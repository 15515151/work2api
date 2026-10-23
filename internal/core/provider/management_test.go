package provider

import (
	"testing"
	"time"
)

func TestResourceStatusSeparatesCapacityFromAuthorization(t *testing.T) {
	tests := []struct {
		name       string
		row        map[string]any
		kind, want string
	}{
		{"no secret beats optimistic flag", map[string]any{"has_secret": false, "healthy": true}, "account", "unauthorized"},
		{"free remains available", map[string]any{"credits_remaining": float64(0), "healthy": true}, "account", "healthy"},
		{"paid exhausted", map[string]any{"credits_remaining": float64(0), "healthy": false}, "account", "paid_exhausted"},
		{"cooling", map[string]any{"cooldown_until": float64(time.Now().Add(time.Hour).Unix()), "healthy": false}, "account", "cooling"},
		{"partial tier", map[string]any{"key_count": 3, "available_count": 2, "total_count": 3}, "key_tier", "degraded"},
		{"unavailable tier", map[string]any{"key_count": 3, "available_count": 0, "total_count": 3}, "key_tier", "unavailable"},
		{"unconfigured tier", map[string]any{"key_count": 0, "available_count": 0, "total_count": 0}, "key_tier", "unconfigured"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResourceMetadata(tc.row, tc.kind, nil).Status; got != tc.want {
				t.Fatalf("got %s want %s", got, tc.want)
			}
		})
	}
}
