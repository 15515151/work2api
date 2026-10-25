package opencode

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
	"work2api/internal/core/protocol"
)

// Opt-in evaluation of upstream's complete free-tier request shape. Assets
// remain reference-only, never enter production prompts, and use synthetic
// content rather than a user's conversation or the gateway's filesystem path.
func TestLiveOpenCodeUpstreamShape(t *testing.T) {
	if os.Getenv("WORK2API_LIVE_OPENCODE_SHAPE") != "1" {
		t.Skip("explicit upstream shape evaluation")
	}
	cfg, err := LoadConfig(filepath.Join("..", "..", "data", "opencode", "opencode.json"))
	if err != nil {
		t.Fatal("local config unavailable")
	}
	pool, err := newTransportPool(cfg.RuntimeProxies(), cfg.Performance, 35*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		for _, p := range pool.items {
			p.client.CloseIdleConnections()
		}
	}()
	if len(pool.items) == 0 {
		t.Fatal("no transport")
	}
	read := func(name string) []byte {
		cmd := exec.Command("git", "-C", filepath.Join("..", "..", ".reference", "opencode2api"), "show", "b8b1454:internal/gateway/"+name)
		b, e := cmd.Output()
		if e != nil {
			t.Fatal("reference asset unavailable")
		}
		return b
	}
	prompt := string(read("opencode_prompt_v2.txt"))
	var definitions map[string]map[string]any
	if json.Unmarshal(read("opencode_tools.json"), &definitions) != nil {
		t.Fatal("invalid reference tools")
	}
	tools := []any{}
	for _, name := range []string{"edit", "execute", "glob", "grep", "question", "read", "shell", "skill", "subagent", "webfetch", "websearch", "write", "patch"} {
		d := definitions[name]
		if d == nil {
			t.Fatal("missing official definition")
		}
		tools = append(tools, map[string]any{"type": "function", "function": map[string]any{"name": name, "description": d["description"], "parameters": d["parameters"], "strict": false}})
	}
	body, _ := json.Marshal(map[string]any{"model": "deepseek-v4.1-flash-free", "stream": false, "max_tokens": 128, "reasoning_effort": "low", "tools": tools, "tool_choice": "none", "messages": []any{map[string]any{"role": "system", "content": prompt}, map[string]any{"role": "user", "content": "Reply with just OK. Do not use tools."}}})
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	base := cfg.Upstream.Anonymous
	if base == "" {
		base = cfg.Upstream.Zen
	}
	req, err := newUpstreamRequest(ctx, base, protocol.Chat, body, RequestIDs{Session: CanonicalSessionID("upstream-shape-evaluation"), Request: RandomID("req", 16)}, "public")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	response, err := pool.items[0].client.Do(req)
	if err != nil {
		t.Fatal("upstream transport failed")
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		t.Fatal("response read failed")
	}
	t.Logf("official prompt+13 tools+recent session HTTP=%d duration=%s bytes=%d", response.StatusCode, time.Since(start).Round(time.Millisecond), len(data))
	if response.StatusCode != 200 {
		t.Fatalf("candidate did not restore availability: HTTP %d", response.StatusCode)
	}
}
