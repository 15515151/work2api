package streamwatch

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"
)

type timingKey struct{}

// Timing carries bounded, content-free observations across provider boundaries.
type Timing struct {
	mu                              sync.Mutex
	Start                           time.Time
	Queue, Account                  time.Duration
	FirstByte                       time.Duration
	FirstGeneration, LastGeneration time.Duration
	GenerationEvents                int
	BytesSeen                       bool
	Attempts, Upstream429           int
	Outcome                         string
	RequestID                       string
	FirstAttempt, WaitAtAttempt     time.Duration
	AttemptStages                   []AttemptStage
	lastAttempt                     time.Time
}

// AttemptStage contains no URL, account identifier, headers or request content.
type AttemptStage struct {
	UpstreamRequestID string `json:"upstream_request_id,omitempty"`
	Number            int    `json:"number"`
	HeaderWaitMS      int64  `json:"header_wait_ms"`
	HTTPStatus        int    `json:"http_status"`
	Finished          bool   `json:"headers_finished"`
}

func WithTiming(ctx context.Context) (context.Context, *Timing) {
	t := &Timing{Start: time.Now()}
	return context.WithValue(ctx, timingKey{}, t), t
}
func TimingFrom(ctx context.Context) *Timing { t, _ := ctx.Value(timingKey{}).(*Timing); return t }
func QueueWait(ctx context.Context, d time.Duration, account bool) {
	if t := TimingFrom(ctx); t != nil {
		t.mu.Lock()
		if account {
			t.Account += d
		} else {
			t.Queue += d
		}
		t.mu.Unlock()
	}
}
func StartAttempt(ctx context.Context) {
	if t := TimingFrom(ctx); t != nil {
		t.mu.Lock()
		if t.Attempts == 0 {
			t.FirstAttempt = time.Since(t.Start)
			t.WaitAtAttempt = t.Queue + t.Account
		}
		t.Attempts++
		t.FirstGeneration, t.LastGeneration, t.GenerationEvents = 0, 0, 0
		t.lastAttempt = time.Now()
		if len(t.AttemptStages) < 16 {
			t.AttemptStages = append(t.AttemptStages, AttemptStage{Number: t.Attempts})
		}
		t.mu.Unlock()
	}
}

// Generation records one nonempty model delta, never role, heartbeat, usage,
// signature or terminal snapshots. It retains timestamps only, not content.
func Generation(ctx context.Context) {
	if t := TimingFrom(ctx); t != nil {
		t.mu.Lock()
		defer t.mu.Unlock()
		elapsed := time.Since(t.Start)
		if t.GenerationEvents == 0 {
			t.FirstGeneration = elapsed
		}
		t.LastGeneration = elapsed
		t.GenerationEvents++
	}
}

func HasToolArguments(calls []any) bool {
	for _, raw := range calls {
		call, _ := raw.(map[string]any)
		fn, _ := call["function"].(map[string]any)
		if args, _ := fn["arguments"].(string); args != "" {
			return true
		}
	}
	return false
}

// ChatGeneration uses the already decoded chunk; no second JSON parse.
func ChatGeneration(chunk map[string]any) bool {
	choices, _ := chunk["choices"].([]any)
	for _, raw := range choices {
		choice, _ := raw.(map[string]any)
		delta, _ := choice["delta"].(map[string]any)
		for _, key := range []string{"content", "reasoning_content", "refusal"} {
			if text, _ := delta[key].(string); text != "" {
				return true
			}
		}
		calls, _ := delta["tool_calls"].([]any)
		if HasToolArguments(calls) {
			return true
		}
	}
	return false
}

// AttemptHeaders records only connection/header wait, not body generation time.
func AttemptHeaders(ctx context.Context, status int, headers ...http.Header) {
	if t := TimingFrom(ctx); t != nil {
		t.mu.Lock()
		defer t.mu.Unlock()
		if t.Attempts == 0 || t.Attempts > len(t.AttemptStages) {
			return
		}
		stage := &t.AttemptStages[t.Attempts-1]
		if stage.Finished {
			return
		}
		stage.HeaderWaitMS = time.Since(t.lastAttempt).Milliseconds()
		stage.HTTPStatus = status
		stage.Finished = true
		if len(headers) > 0 {
			stage.UpstreamRequestID = ResponseRequestID(headers[0])
		}
	}
}

// Store only recognizable, bounded correlation IDs; never arbitrary headers.
func ResponseRequestID(header http.Header) string {
	for _, key := range []string{"X-Request-Id", "Request-Id", "X-Tt-Logid"} {
		value := strings.TrimSpace(header.Get(key))
		if value == "" || len(value) > 128 {
			continue
		}
		safe := true
		for _, c := range value {
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || strings.ContainsRune("-_:.", c)) {
				safe = false
				break
			}
		}
		if safe {
			return value
		}
	}
	return ""
}
func AttemptHTTP(ctx context.Context, response *http.Response) {
	if response == nil {
		AttemptHeaders(ctx, 0)
		return
	}
	AttemptHeaders(ctx, response.StatusCode, response.Header)
}
func AttemptResult(ctx context.Context, status int) {
	if status != 429 {
		return
	}
	if t := TimingFrom(ctx); t != nil {
		t.mu.Lock()
		t.Upstream429++
		t.mu.Unlock()
	}
}
func Outcome(ctx context.Context, status string) {
	if t := TimingFrom(ctx); t != nil {
		t.mu.Lock()
		t.Outcome = status
		t.mu.Unlock()
	}
}
func (t *Timing) FirstWrite() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.BytesSeen {
		t.BytesSeen = true
		t.FirstByte = time.Since(t.Start)
	}
}

type TimingSnapshot struct {
	Start                           time.Time
	Queue, Account, FirstByte       time.Duration
	BytesSeen                       bool
	FirstGeneration, LastGeneration time.Duration
	GenerationEvents                int
	Attempts, Upstream429           int
	Outcome                         string
	RequestID                       string
	FirstAttempt, WaitAtAttempt     time.Duration
	AttemptStages                   []AttemptStage
}

func (t *Timing) Snapshot() TimingSnapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	return TimingSnapshot{Start: t.Start, Queue: t.Queue, Account: t.Account, FirstByte: t.FirstByte, BytesSeen: t.BytesSeen, FirstGeneration: t.FirstGeneration, LastGeneration: t.LastGeneration, GenerationEvents: t.GenerationEvents, Attempts: t.Attempts, Upstream429: t.Upstream429, Outcome: t.Outcome, RequestID: t.RequestID, FirstAttempt: t.FirstAttempt, WaitAtAttempt: t.WaitAtAttempt, AttemptStages: append([]AttemptStage(nil), t.AttemptStages...)}
}
