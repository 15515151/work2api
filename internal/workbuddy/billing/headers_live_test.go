package billing

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
	"work2api/internal/workbuddy/credentials"
)

// Read-only billing probe. No check-in or inference request is sent and no
// account identifiers, authorization headers or raw response bodies are logged.
func TestLiveBillingIdentity(t *testing.T) {
	if os.Getenv("WORK2API_LIVE_BILLING_HEADERS") != "1" {
		t.Skip("explicit read-only upstream verification")
	}
	files := credentials.FindAuthFiles("", filepath.Join("..", "..", "..", "auths"))
	seen := map[string]bool{}
	success := 0
	for _, file := range files {
		mgr := credentials.NewManager(file)
		profile := mgr.Profile()
		if seen[profile] {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		_, err := mgr.GetHeadersContext(ctx)
		cancel()
		if err != nil {
			t.Logf("profile=%s credential unavailable", profile)
			continue
		}
		seen[profile] = true
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
		start := time.Now()
		credit, err := FetchCredits(ctx, mgr)
		cancel()
		if err != nil {
			t.Logf("profile=%s billing unavailable: %v", profile, err)
			continue
		}
		success++
		t.Logf("profile=%s success packages=%d duration=%s", profile, len(credit.Packages), time.Since(start).Round(time.Millisecond))
	}
	if success == 0 {
		t.Fatal("no live billing response verified")
	}
}
