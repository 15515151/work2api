package provider

import (
	"context"
	"strings"
	"unicode/utf8"
)

// LogCapturePolicy applies only to diagnostic copies, never protocol output.
type LogCapturePolicy struct {
	Enabled bool
	Limit   int // zero keeps the deployment's explicit unlimited setting
}

type LogCapture struct {
	text   strings.Builder
	policy LogCapturePolicy
	full   bool
}

func NewLogCapture(ctx context.Context) *LogCapture {
	p := LogCapturePolicy{Enabled: true, Limit: 32768}
	if policy := CurrentInvocation(ctx).LogCapture; policy != nil {
		p = *policy
	}
	return NewLogCaptureWithPolicy(p)
}

func NewLogCaptureWithPolicy(p LogCapturePolicy) *LogCapture { return &LogCapture{policy: p} }

func (b *LogCapture) WriteString(text string) {
	if !b.policy.Enabled || b.full {
		return
	}
	if b.policy.Limit > 0 && len(text) > b.policy.Limit-b.text.Len() {
		cut := b.policy.Limit - b.text.Len()
		for cut > 0 && !utf8.RuneStart(text[cut]) {
			cut--
		}
		text = text[:cut]
		b.full = true
	}
	b.text.WriteString(text)
}

func (b *LogCapture) String() string { return b.text.String() }
func (b *LogCapture) Full() bool     { return b.full || !b.policy.Enabled }
