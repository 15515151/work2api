package app

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"
	"work2api/internal/core/provider"
)

type accountCheckEntry struct {
	identity string
	result   provider.AccountCheck
	retryAt  time.Time
	failures int
}

func (e accountCheckEntry) due(period time.Duration) time.Time {
	if e.result.Status == "valid" || e.result.Status == "unknown" {
		return time.Unix(e.result.CheckedAt, 0).Add(period)
	}
	return e.retryAt
}

type accountCheckFlight struct {
	identity string
	done     chan struct{}
	result   provider.AccountCheck
}
type accountCheckBook struct {
	mu      sync.Mutex
	entries map[string]accountCheckEntry
	tests   map[string]accountCheckEntry
	flights map[string]*accountCheckFlight
	slots   chan struct{}
}

func checkIdentity(rt provider.Runtime, p provider.AccountChecker, id string) string {
	return fmt.Sprintf("%p:", rt) + p.CheckAccountIdentity(id)
}
func checkKey(rt provider.Runtime, id string) string {
	return provider.AccountRef{Provider: rt.Name(), LocalID: id}.ResourceKey()
}
func (b *accountCheckBook) run(ctx context.Context, rt provider.Runtime, p provider.AccountChecker, id string, force bool, period time.Duration) provider.AccountCheck {
	key := checkKey(rt, id)
	identity := checkIdentity(rt, p, id)
	b.mu.Lock()
	if b.entries == nil {
		b.entries = map[string]accountCheckEntry{}
		if b.tests == nil {
			b.tests = map[string]accountCheckEntry{}
		}
		b.flights = map[string]*accountCheckFlight{}
		b.slots = make(chan struct{}, 2)
	}
	if f := b.flights[key]; f != nil {
		b.mu.Unlock()
		select {
		case <-ctx.Done():
			return provider.Checked("unknown", "检查", "检查已取消")
		case <-f.done:
			if f.identity != identity || identity != checkIdentity(rt, p, id) {
				return provider.Checked("unknown", "检查", "凭据已变化，请重新检查")
			}
			return f.result
		}
	}
	if entry, ok := b.entries[key]; !force && ok && entry.identity == identity && time.Now().Before(entry.due(period)) {
		b.mu.Unlock()
		return entry.result
	}
	flight := &accountCheckFlight{identity: identity, done: make(chan struct{})}
	b.flights[key] = flight
	slots := b.slots
	b.mu.Unlock()
	result := provider.Checked("unknown", "检查", "检查已取消")
	publish := false
	start := time.Now()
	defer func() { b.mu.Lock(); flight.result = result; delete(b.flights, key); close(flight.done); b.mu.Unlock() }()
	select {
	case slots <- struct{}{}:
		defer func() { <-slots }()
	case <-ctx.Done():
		return result
	}
	if p.CheckAccountIdentity(id) == "" {
		result = provider.Checked("unknown", "账号", "账号已移除或凭据正在更新")
		return result
	}
	result = p.CheckAccount(provider.WithAccountCheckMaxAge(ctx, period), id, force)
	result.LatencyMS = time.Since(start).Milliseconds()
	if ctx.Err() != nil {
		result = provider.Checked("warning", "检查", "检查超时或取消，未判定授权失效")
	}
	if identity != checkIdentity(rt, p, id) {
		result = provider.Checked("unknown", "检查", "凭据已变化，将使用新凭据重新检查")
		return result
	}
	if result.CheckedAt == 0 {
		result.CheckedAt = time.Now().Unix()
	}
	if result.ExpiresAt == 0 {
		result.ExpiresAt = time.Unix(result.CheckedAt, 0).Add(period).Unix()
	}
	publish = !errors.Is(ctx.Err(), context.Canceled)
	if publish {
		b.mu.Lock()
		old := b.entries[key]
		next := time.Now().Add(period)
		failures := 0
		if expiry := time.Unix(result.ExpiresAt, 0); expiry.Before(next) {
			next = expiry
		}
		if result.Status == "warning" {
			failures = old.failures + 1
			delay := 5 * time.Minute
			if failures == 2 {
				delay = 15 * time.Minute
			}
			if failures >= 3 {
				delay = time.Hour
			}
			next = time.Now().Add(delay)
		}
		if result.Status == "invalid" {
			next = time.Now().Add(24 * time.Hour)
		}
		b.entries[key] = accountCheckEntry{identity: identity, result: result, retryAt: next, failures: failures}
		b.mu.Unlock()
	}
	return result
}
func (b *accountCheckBook) snapshot(rt provider.Runtime, p provider.AccountChecker, id string) (*provider.AccountCheck, *provider.AccountCheck) {
	identity := checkIdentity(rt, p, id)
	key := checkKey(rt, id)
	b.mu.Lock()
	defer b.mu.Unlock()
	var check, test *provider.AccountCheck
	if e, ok := b.entries[key]; ok && e.identity == identity {
		v := e.result
		check = &v
	}
	if e, ok := b.tests[key]; ok && e.identity == identity {
		v := e.result
		test = &v
	}
	return check, test
}
func (b *accountCheckBook) saveTest(rt provider.Runtime, p provider.AccountChecker, id, identity string, result provider.AccountCheck) {
	if identity != checkIdentity(rt, p, id) {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.tests == nil {
		b.tests = map[string]accountCheckEntry{}
	}
	b.tests[checkKey(rt, id)] = accountCheckEntry{identity: identity, result: result}
}
func checkPeriod(settings map[string]string) time.Duration {
	n, e := strconv.Atoi(settings["account_check_interval_min"])
	if e != nil || n < 5 || n > 1440 {
		n = 30
	}
	return time.Duration(n) * time.Minute
}
func (o *Orchestrator) checkAccountsTick(ctx context.Context, settings map[string]string) {
	if settings["account_check_enabled"] == "0" {
		return
	}
	type task struct {
		rt provider.Runtime
		p  provider.AccountChecker
		id string
		at time.Time
	}
	tasks := []task{}
	live := map[string]bool{}
	b := &o.accountChecks
	for _, rt := range o.runtimes.Runtimes() {
		p, ok := rt.(provider.AccountChecker)
		if !ok {
			continue
		}
		for _, id := range p.CheckAccountIDs() {
			live[checkKey(rt, id)] = true
			identity := checkIdentity(rt, p, id)
			b.mu.Lock()
			e := b.entries[checkKey(rt, id)]
			b.mu.Unlock()
			if e.identity == identity && time.Now().Before(e.due(checkPeriod(settings))) {
				continue
			}
			tasks = append(tasks, task{rt, p, id, e.due(checkPeriod(settings))})
		}
	}
	b.mu.Lock()
	for key := range b.entries {
		if !live[key] && b.flights[key] == nil {
			delete(b.entries, key)
			delete(b.tests, key)
		}
	}
	for key := range b.tests {
		if !live[key] && b.flights[key] == nil {
			delete(b.tests, key)
		}
	}
	b.mu.Unlock()
	sort.SliceStable(tasks, func(i, j int) bool { return tasks[i].at.Before(tasks[j].at) })
	// A round uses at most two workers, never an unbounded background fan-out.
	if len(tasks) > 2 {
		tasks = tasks[:2]
	}
	var wg sync.WaitGroup
	for _, t := range tasks {
		wg.Add(1)
		go func(t task) {
			defer wg.Done()
			probe, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			b.run(probe, t.rt, t.p, t.id, false, checkPeriod(settings))
		}(t)
	}
	wg.Wait()
}
