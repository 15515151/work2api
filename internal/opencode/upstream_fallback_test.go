package opencode

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"work2api/internal/core/protocol"
)

type fallbackTransport func(*http.Request) (*http.Response, error)

func (f fallbackTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestInferenceMessagesBetaOnlyForKnownV2Endpoint(t *testing.T) {
	for _, tc := range []struct {
		base  string
		proto protocol.Protocol
		beta  string
	}{
		{"https://opencode.ai/inference", protocol.Anthropic, "true"},
		{"https://opencode.ai/inference/", protocol.Anthropic, "true"},
		{"https://opencode.ai/inference", protocol.Chat, ""},
		{"https://opencode.ai/inference", protocol.Responses, ""},
		{"https://opencode.ai/zen", protocol.Anthropic, ""},
		{"https://custom.invalid/inference", protocol.Anthropic, ""},
	} {
		req, err := newUpstreamRequest(context.Background(), tc.base, tc.proto, []byte(`{}`), RequestIDs{}, "synthetic-key")
		if err != nil || req.URL.Query().Get("beta") != tc.beta {
			t.Fatalf("unexpected beta flag for %s %s: %v %v", tc.base, tc.proto, req, err)
		}
	}
}

func TestTierTransportFailurePreservesRealUpstreamError(t *testing.T) {
	for _, status := range []int{429, 503} {
		cfg := Config{Retry: RetryConfig{MaxAttempts: 1}}
		cfg.Upstream.Anonymous = "https://anonymous.invalid"
		cfg.Upstream.Zen = "https://paid.invalid"
		transports, err := newTransportPool([]string{"direct"}, PerformanceConfig{}, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		// Keep background proxy health probes out of this deterministic lane test.
		transports.items[0].checking.Store(true)
		calls := 0
		transports.items[0].client = &http.Client{Transport: fallbackTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.URL.Host == "anonymous.invalid" {
				return &http.Response{StatusCode: status, Header: http.Header{"Retry-After": {"37"}}, Body: io.NopCloser(strings.NewReader(`{"error":"busy"}`)), Request: r}, nil
			}
			return nil, errors.New("synthetic connection failure")
		})}
		keys, _ := newNodePool([]string{"synthetic-key"}, transports, time.Second)
		rt := &Runtime{cfg: cfg, logger: slog.New(slog.NewTextHandler(io.Discard, nil)), transports: transports, zenNodes: keys, goNodes: &nodePool{}, anonymous: newAnonymousPool(true, transports, time.Second), catalog: NewCatalog(TierZen, nil)}
		uid := ""
		route := Route{ID: "probe", Tier: TierZen, Protocol: protocol.Chat, Anonymous: true, KeyTiers: []Tier{TierZen}}
		resp, effective, _, err := rt.doUpstreamTiers(context.Background(), route, map[Tier][]byte{TierZen: []byte(`{"model":"probe","messages":[]}`)}, RequestIDs{Session: "probe", AccountUID: &uid}, 0)
		if err != nil || resp == nil {
			t.Fatal("real error replaced by transport failure", err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if calls != 2 || resp.StatusCode != status || resp.Header.Get("Retry-After") != "37" || string(body) != `{"error":"busy"}` || !effective.Anonymous || uid != "opencode:anonymous" {
			t.Fatalf("wrong retained response calls=%d status=%d route=%+v uid=%s body=%s", calls, resp.StatusCode, effective, uid, body)
		}
	}
}
