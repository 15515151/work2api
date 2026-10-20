package upstream_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"work2api/internal/workbuddy/credentials"
	"work2api/internal/workbuddy/reasoning"
	"work2api/internal/workbuddy/siterouting"
	"work2api/internal/workbuddy/upstream"
)

// TestLiveInternationalConversationHeaders uses synthetic content only and
// reports no credentials or account identifiers. Opt in explicitly.
func TestLiveInternationalConversationHeaders(t *testing.T) {
	if os.Getenv("WORK2API_LIVE_INTL_HEADERS") != "1" {
		t.Skip("explicit live verification only")
	}
	var mgr *credentials.Manager
	for _, file := range credentials.FindAuthFiles("", filepath.Join("..", "..", "..", "auths")) {
		candidate := credentials.NewManager(file)
		if siterouting.ProfileSite(candidate.Profile()) == siterouting.International {
			mgr = candidate
			break
		}
	}
	if mgr == nil {
		t.Fatal("international credential unavailable")
	}
	url, err := siterouting.ChatURLForProfile(mgr.Profile())
	if err != nil {
		t.Fatal(err)
	}
	prefix := strings.Repeat("Reference item: amber birch cedar delta elm fern.\n", 300)
	body := upstream.BuildUpstreamBody(map[string]any{
		"model": "deepseek-v4.1-flash", "max_tokens": 1024, "reasoning_effort": "off",
		"messages": []any{map[string]any{"role": "system", "content": "Answer briefly."}, map[string]any{"role": "user", "content": prefix + "Reply with just OK."}},
	})
	body = reasoning.InjectThinking(body)
	for _, mode := range []string{"legacy", "current", "current-repeat"} {
		ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
		headers, err := mgr.GetChatHeadersContext(ctx)
		if err != nil {
			cancel()
			t.Fatal("credential preparation failed")
		}
		if mode == "legacy" {
			delete(headers, "X-Conversation-ID")
		}
		start := time.Now()
		var usage map[string]any
		content := 0
		finish := ""
		first := time.Duration(0)
		err = upstream.Shared().StreamUpstream(ctx, headers, body, url, func(line string) error {
			if !strings.HasPrefix(line, "data: ") {
				return nil
			}
			var chunk map[string]any
			if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &chunk) != nil {
				return nil
			}
			if u, ok := chunk["usage"].(map[string]any); ok {
				usage = upstream.MergeUsage(usage, u)
			}
			choices, _ := chunk["choices"].([]any)
			for _, raw := range choices {
				choice, _ := raw.(map[string]any)
				delta, _ := choice["delta"].(map[string]any)
				if text, _ := delta["content"].(string); text != "" {
					content += len(text)
					if first == 0 {
						first = time.Since(start)
					}
				}
				if reason, _ := choice["finish_reason"].(string); reason != "" {
					finish = reason
				}
			}
			return nil
		})
		cancel()
		if err != nil {
			var remote *upstream.UpstreamError
			if errors.As(err, &remote) {
				var payload map[string]any
				_ = json.Unmarshal(remote.Raw, &payload)
				t.Logf("mode=%s status=%d code=%v msg=%v duration=%s", mode, remote.StatusCode, payload["code"], payload["msg"], time.Since(start).Round(time.Millisecond))
				if remote.StatusCode == 429 {
					t.Fatal("rate limited; stop live requests")
				}
			} else {
				t.Logf("mode=%s transport failure: %v", mode, err)
			}
			if mode != "legacy" {
				t.Fatal("current headers did not complete generation")
			}
			continue
		}
		t.Logf("mode=%s finish=%s bytes=%d first=%s duration=%s usage=%v", mode, finish, content, first.Round(time.Millisecond), time.Since(start).Round(time.Millisecond), usage)
		if content == 0 || finish != "stop" {
			t.Error("incomplete synthetic response")
		}
	}
}
