package app

import (
	"testing"
	"time"
	"work2api/internal/store"
)

// 预热已经刷过一遍额度；若 lastCredit 未被预热同步，首个 tick 的周期判据
// (now-lastCredit >= interval) 因零值必然命中，刚启动就再对全部账号白刷一遍
// （Workbuddy2API #34 同款问题）。这里锁住 lastCredit 必须由预热更新的约定。
func TestWarmupUpdatesLastCredit(t *testing.T) {
	s := &Scheduler{o: nil, stop: make(chan struct{}), done: make(chan struct{})}
	// 模拟 run() 的预热两行：刷新后立即同步 lastCredit（与 scheduler.go 保持一致）
	s.lastCredit = float64(time.Now().Unix())
	interval := 30 // 默认 CREDIT_REFRESH_MIN=30 分钟

	// 首个 tick（预热后 1 秒内）不得再触发周期刷新
	now := time.Now().Add(time.Second)
	if float64(now.Unix())-s.lastCredit >= float64(interval)*60 {
		t.Fatal("首个 tick 不应再次刷新额度：预热未同步 lastCredit 会导致启动双刷")
	}
}

func TestIndependentUsageMaintenanceAndResourceSnapshot(t *testing.T) {
	server, _ := portalFixture(t)
	server.o.cfg.UsageContentBudgetBytes = 1
	id, err := server.o.db.LogUsageWithID(store.UsageParams{Model: "test", InputTokens: 10, InputContent: "saved diagnostic text"})
	if err != nil {
		t.Fatal(err)
	}
	scheduler := NewScheduler(server.o)
	done := make(chan struct{})
	go scheduler.runUsageMaintenance(done)
	defer func() {
		scheduler.cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("maintenance did not stop")
		}
	}()
	deadline := time.Now().Add(3 * time.Second)
	for {
		row, err := server.o.db.GetUsage(int(id))
		if err != nil {
			t.Fatal(err)
		}
		if row["input_content"] == "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("maintenance required upstream scheduler tick")
		}
		time.Sleep(time.Millisecond)
	}
	snapshot := server.resourceSnapshot()
	if snapshot["goroutines"].(int) < 1 || snapshot["usage"].(map[string]any)["write_attempts"].(uint64) < 1 {
		t.Fatal("resource counters missing", snapshot)
	}
	if server.resourceSnapshot()["sampled_at"] != snapshot["sampled_at"] {
		t.Fatal("snapshot cache missed")
	}
}

func TestScheduledMaintenanceClearsOnlyDiagnosticText(t *testing.T) {
	server, _ := portalFixture(t)
	server.o.cfg.UsageContentBudgetBytes = 1
	server.o.cfg.UsageContentRetentionDays = 0
	id, err := server.o.db.LogUsageWithID(store.UsageParams{Model: "test", InputTokens: 10, OutputTokens: 5, InputContent: "large saved text"})
	if err != nil {
		t.Fatal(err)
	}
	scheduler := NewScheduler(server.o)
	defer scheduler.cancel()
	scheduler.cleanupUsage()
	row, err := server.o.db.GetUsage(int(id))
	if err != nil {
		t.Fatal(err)
	}
	if row["input_content"] != "" || row["input_tokens"] != int64(10) || !row["diagnostics"].(store.UsageDiagnostics).ContentCleared {
		t.Fatal("scheduled cleanup changed accounting or retained text", row)
	}
	if scheduler.usageMaintenancePending || scheduler.lastUsageMaintenance.IsZero() {
		t.Fatal("maintenance completion not tracked")
	}
}
