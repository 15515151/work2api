package profiles

import (
	"strings"
	"testing"

	"work2api/internal/workbuddy/siterouting"
)

func testAuth() siterouting.Auth {
	return siterouting.Auth{"accessToken": "tok", "domain": "copilot.tencent.com"}
}

// Chat 保留三个共享请求标识，并额外发送独立的 UUID 会话头；不发送 X-Session-ID。
func TestChatHeadersRequestIDs(t *testing.T) {
	h := ChatHeaders(testAuth(), Account{"uid": "u1"})
	rid := h["X-Request-ID"]
	if rid == "" || len(rid) != 32 {
		t.Fatalf("X-Request-ID 应为 32 位 hex, got %q", rid)
	}
	if h["X-Conversation-Message-ID"] != rid || h["X-Conversation-Request-ID"] != rid {
		t.Fatal("三个 id 头应共享同一值（对账用）")
	}
	conversation := h["X-Conversation-ID"]
	if len(conversation) != 36 || conversation[14] != '4' || !strings.ContainsRune("89ab", rune(conversation[19])) {
		t.Fatal("conversation id must be UUID v4")
	}
	for _, k := range []string{"X-Session-ID"} {
		if _, ok := h[k]; ok {
			t.Fatalf("%s 不应发送（疑似缓存归属键）", k)
		}
	}
	// 每次调用生成新 id
	h2 := ChatHeaders(testAuth(), Account{"uid": "u1"})
	if h2["X-Request-ID"] == rid {
		t.Fatal("request id 应每次重新生成")
	}
	if h2["X-Conversation-ID"] == conversation {
		t.Fatal("conversation transport id should be fresh")
	}
	// 基础身份头仍在
	if !strings.Contains(h["User-Agent"], "CLI/") {
		t.Fatalf("UA 缺失或异常: %q", h["User-Agent"])
	}
	if h["Authorization"] != "Bearer tok" {
		t.Fatal("Authorization 缺失")
	}
}
