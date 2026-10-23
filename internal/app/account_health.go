package app

import (
	"strings"
	"sync"
	"time"
	"work2api/internal/store"
	wbruntime "work2api/internal/workbuddy/runtime"
)

// Observations describe actual traffic, never grant permission or change
// scheduling. Bounded, memory-only, and free of request/response/credential text.
type accountHealthEvidence struct {
	LastSuccess  float64 `json:"last_success,omitempty"`
	LastFailure  float64 `json:"last_failure,omitempty"`
	LastObserved float64 `json:"-"`
	FailureKind  string  `json:"failure_kind,omitempty"`
	BusinessCode string  `json:"business_code,omitempty"`
}
type accountHealthObservations struct {
	mu   sync.Mutex
	rows map[string]accountHealthEvidence
}

func accountHealthKey(channel, uid string) string {
	if channel == "qoder" {
		uid = strings.TrimPrefix(uid, "qoder:")
	}
	return channel + "\x00" + uid
}

func (o *accountHealthObservations) observe(channel string, p store.UsageParams) {
	if p.AccountUID == "" || (p.Status != "ok" && p.Status != "incomplete" && p.Status != "error") {
		return
	}
	now := float64(time.Now().UnixMilli()) / 1000
	key := accountHealthKey(channel, p.AccountUID)
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.rows == nil {
		o.rows = make(map[string]accountHealthEvidence)
	}
	row, exists := o.rows[key]
	if !exists && len(o.rows) >= 4096 {
		oldest := ""
		at := now
		for k, v := range o.rows {
			if oldest == "" || v.LastObserved < at {
				oldest, at = k, v.LastObserved
			}
		}
		delete(o.rows, oldest)
	}
	row.LastObserved = now
	if p.Status == "ok" {
		row.LastSuccess = now
	} else {
		row.LastFailure = now
		row.FailureKind = "request_failed"
		row.BusinessCode = ""
		if p.Status == "incomplete" {
			row.FailureKind = "incomplete"
		}
		if p.Diagnostics != nil {
			switch p.Diagnostics.ErrorKind {
			case "auth_error", "account_fault", "channel_denied", "soft_rate", "plan_limit", "transport", "upstream_error", "content_policy", "timeout", "local_network", "response_limit", "gateway", "hard_credit", "session_dead", "not_found", "server", "content_blocked", "bad_params", "model_blocked", "waf_block", "prompt_too_long":
				row.FailureKind = p.Diagnostics.ErrorKind
			}
		}
		// Structured upstream codes only; never retain raw provider prose.
		detail := p.Error
		if len(detail) > 16384 {
			detail = detail[:16384]
		}
		if channel == "workbuddy" {
			for _, code := range []string{"11128", "11135", "11140", "14003", "14017"} {
				if wbruntime.BusinessCode(detail, code) {
					row.BusinessCode = code
					break
				}
			}
		}
	}
	o.rows[key] = row
}
func (o *accountHealthObservations) snapshot(channel, uid string) (accountHealthEvidence, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	row, ok := o.rows[accountHealthKey(channel, uid)]
	return row, ok
}
