package provider

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMaintenanceGuardCredentialRecovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "maintenance.json")
	g := NewMaintenanceGuard(path)
	g.State("disabled-account", "old")
	g.Observe("disabled-account", "old", "authorization_expired", true)
	g.Observe("disabled-account", "old", "", false)
	calls := 0
	probe := func(context.Context) error { calls++; return nil }
	g = NewMaintenanceGuard(path)
	if _, err := g.Recover(context.Background(), "disabled-account", "old", probe); !errors.Is(err, ErrMaintenanceLogin) || calls != 0 {
		t.Fatalf("invalid credentials were retried: %v, %d", err, calls)
	}
	if _, err := g.Recover(context.Background(), "disabled-account", "new", probe); err != nil || calls != 0 {
		t.Fatalf("new credentials should leave recovery gate: %v", err)
	}
	g.Observe("disabled-account", "old", "authorization_expired", true)
	if g.State("disabled-account", "new").Kind != "" {
		t.Fatal("stale callback blocked new credentials")
	}
	if NewMaintenanceGuard(path).State("disabled-account", "new").Kind != "" {
		t.Fatal("stale state persisted")
	}
}

func TestMaintenanceGuardBackoffAndSharedProbe(t *testing.T) {
	g := NewMaintenanceGuard("")
	g.State("a", "credential")
	g.Observe("a", "credential", "network", false)
	if _, err := g.Recover(context.Background(), "a", "credential", func(context.Context) error { t.Fatal("probe ran before backoff"); return nil }); !errors.Is(err, ErrMaintenancePaused) {
		t.Fatal(err)
	}
	g.mu.Lock()
	s := g.states["a"]
	s.RetryAt = time.Now().Add(-time.Second).Unix()
	g.states["a"] = s
	g.mu.Unlock()
	var calls atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := g.Recover(context.Background(), "a", "credential", func(context.Context) error { calls.Add(1); g.Observe("a", "credential", "", false); return nil })
			if err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("recovery probes: %d", calls.Load())
	}
	if g.State("a", "credential").Kind != "" {
		t.Fatal("successful verification did not resume maintenance")
	}
}
