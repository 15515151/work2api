package app

import (
	"net/http"
	"os"
	"runtime"
	"runtime/metrics"
	"sync"
	"time"
)

type resourceMonitor struct {
	mu    sync.Mutex
	at    time.Time
	value map[string]any
}

// Sampling contains no prompts, credentials, account IDs or external I/O.
// Fixed-size runtime counters and file metadata are cached for ten seconds.
func (s *Server) resourceSnapshot() map[string]any {
	m := &s.resources
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.value != nil && time.Since(m.at) < 10*time.Second {
		return m.value
	}
	samples := []metrics.Sample{{Name: "/memory/classes/heap/objects:bytes"}, {Name: "/gc/heap/goal:bytes"}, {Name: "/gc/cycles/total:gc-cycles"}}
	metrics.Read(samples)
	values := make([]any, len(samples))
	for i, sample := range samples {
		if sample.Value.Kind() == metrics.KindUint64 {
			values[i] = sample.Value.Uint64()
		}
	}
	fileBytes := func(path string) any {
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			return info.Size()
		}
		return nil
	}
	m.at = time.Now()
	m.value = map[string]any{"sampled_at": m.at.Unix(), "heap_objects_bytes": values[0], "heap_goal_bytes": values[1], "gc_cycles": values[2], "goroutines": runtime.NumGoroutine(),
		"go_memory_in_use_bytes": goMemoryUsage(), "request_body_bytes": s.bodies.usage(),
		"database_bytes": fileBytes(s.o.cfg.DBPath), "wal_bytes": fileBytes(s.o.cfg.DBPath + "-wal"), "usage": s.o.db.UsageRuntimeStats()}
	return m.value
}

func (s *Server) adminResources(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.resourceSnapshot())
}
