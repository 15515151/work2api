package qoder

import (
	"testing"
	"work2api/internal/qoder/bridge"
)

func TestWorkspaceQuotaAndQueueDoNotCoolOrSwitchAccounts(t *testing.T) {
	for _, raw := range []string{`{"error":{"message":"Workspace allocated quota exceeded"}}`, `{"subCode":"RESOURCE_LIMIT","details":{"isQueued":true}}`, `{"subCode":"RESOURCE_LIMIT","serviceAvailable":false}`} {
		for _, err := range []error{bridge.NewUpstreamError(429, raw), bridge.NewStreamBusinessError(raw)} {
			if canFailover(err) {
				t.Fatal("resource failure must not switch/cool healthy accounts", err)
			}
		}
	}
	if !canFailover(bridge.NewUpstreamError(429, `{"error":{"message":"rate limited"}}`)) {
		t.Fatal("ordinary account rate limiting lost failover")
	}
	if !canFailover(bridge.NewUpstreamError(503, `{"error":{"message":"unavailable"}}`)) {
		t.Fatal("transient fallback regressed")
	}
}
