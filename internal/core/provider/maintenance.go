package provider

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// MaintenanceGuard suppresses known-invalid background operations, never model
// dispatch. State is keyed by the channel's credential fingerprint and scope.
type MaintenanceGuard struct {
	mu      sync.Mutex
	path    string
	states  map[string]MaintenanceState
	flights map[string]*RefreshGate
}
type MaintenanceState struct {
	Identity   string `json:"identity"`
	Kind       string `json:"kind,omitempty"`
	NeedsLogin bool   `json:"needs_login,omitempty"`
	Failures   int    `json:"failures,omitempty"`
	RetryAt    int64  `json:"retry_at,omitempty"`
}

var ErrMaintenancePaused = errors.New("维护已暂停，等待授权恢复验证")
var ErrMaintenanceLogin = errors.New("维护已暂停，请重新授权")

func NewMaintenanceGuard(path string) *MaintenanceGuard {
	g := &MaintenanceGuard{path: path, states: map[string]MaintenanceState{}, flights: map[string]*RefreshGate{}}
	if path != "" {
		if raw, e := os.ReadFile(path); e == nil && len(raw) <= 1<<20 {
			var states map[string]MaintenanceState
			if json.Unmarshal(raw, &states) == nil && states != nil && len(states) <= 4096 {
				g.states = states
			}
		}
	}
	return g
}
func (g *MaintenanceGuard) State(key, identity string) MaintenanceState {
	g.mu.Lock()
	defer g.mu.Unlock()
	s := g.states[key]
	if s.Identity != identity {
		changed := s.Kind != ""
		s = MaintenanceState{Identity: identity}
		g.states[key] = s
		if changed {
			g.saveLocked()
		}
	}
	return s
}
func (g *MaintenanceGuard) Observe(key, identity, kind string, needsLogin bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	s, exists := g.states[key]
	if !exists || s.Identity != identity {
		return
	}
	if s.NeedsLogin {
		return
	}
	if kind == "" {
		if s.Kind == "" {
			return
		}
		delete(g.states, key)
		g.states[key] = MaintenanceState{Identity: identity}
		g.saveLocked()
		return
	}
	s.Kind = kind
	s.NeedsLogin = needsLogin
	s.Failures++
	delay := 5 * time.Minute
	if s.Failures == 2 {
		delay = 15 * time.Minute
	}
	if s.Failures >= 3 {
		delay = time.Hour
	}
	s.RetryAt = time.Now().Add(delay).Unix()
	g.states[key] = s
	g.saveLocked()
}

// Recovery is shared across maintenance branches. Healthy credentials incur no
// probe. A failed identity cannot be retried by merely restarting the process.
func (g *MaintenanceGuard) Recover(ctx context.Context, key, identity string, probe func(context.Context) error) (bool, error) {
	s := g.State(key, identity)
	if s.Kind == "" {
		return false, nil
	}
	if s.NeedsLogin {
		return false, ErrMaintenanceLogin
	}
	if time.Now().Unix() < s.RetryAt {
		return false, ErrMaintenancePaused
	}
	g.mu.Lock()
	flight := g.flights[key]
	if flight == nil {
		flight = &RefreshGate{}
		g.flights[key] = flight
	}
	g.mu.Unlock()
	ran := false
	err := flight.Do(ctx, identity, func(ctx context.Context) error {
		g.mu.Lock()
		s := g.states[key]
		g.mu.Unlock()
		if s.Identity != identity {
			return ErrMaintenancePaused
		}
		if s.Kind == "" {
			return nil
		}
		if s.NeedsLogin {
			return ErrMaintenanceLogin
		}
		if time.Now().Unix() < s.RetryAt {
			return ErrMaintenancePaused
		}
		ran = true
		return probe(ctx)
	})
	return ran, err
}
func (g *MaintenanceGuard) saveLocked() {
	if g.path == "" {
		return
	}
	stored := map[string]MaintenanceState{}
	for key, s := range g.states {
		if s.Kind != "" {
			stored[key] = s
		}
	}
	raw, err := json.Marshal(stored)
	if err != nil {
		return
	}
	file, err := os.CreateTemp(filepath.Dir(g.path), ".maintenance-*")
	if err != nil {
		log.Print("维护状态保存失败；本次运行继续暂停异常任务")
		return
	}
	name := file.Name()
	defer os.Remove(name)
	if err = file.Chmod(0600); err == nil {
		_, err = file.Write(raw)
	}
	if err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(name, g.path)
	}
	if err != nil {
		log.Print("维护状态保存失败；本次运行继续暂停异常任务")
	}
}

func (s MaintenanceState) Public(scope string) map[string]any {
	return map[string]any{"scope": scope, "kind": s.Kind, "needs_login": s.NeedsLogin, "retry_at": s.RetryAt}
}

// Pending returns a copy of failed scopes for this exact credential generation.
func (g *MaintenanceGuard) Pending(prefix, identity string) map[string]MaintenanceState {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := map[string]MaintenanceState{}
	for key, state := range g.states {
		if strings.HasPrefix(key, prefix) && state.Identity == identity && state.Kind != "" {
			out[key] = state
		}
	}
	return out
}
