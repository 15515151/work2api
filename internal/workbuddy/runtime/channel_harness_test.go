package runtime

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestHarnessIdentityCompatibilityIsNarrow(t *testing.T) {
	const intro = "You are an AI agent powered by DeepSeek Harness."
	for _, tail := range []string{"", "\nKeep all skills and permission rules.", " Use the read tool.", "\r\nContinue."} {
		got, changed := CompatibleIdentity(intro + tail)
		if !changed || got != "You are a coding assistant."+tail {
			t.Fatal("lost the instructions after the known opening")
		}
	}
	for _, text := range []string{
		"Quoted: " + intro,
		intro + "example",
		"You are a coding agent powered by the deepseek-v4.1-flash model.",
		"You are an AI assistant built on DeepSeek Harness.",
		"You are an AI agent powered by another application.",
	} {
		got, changed := CompatibleIdentity(text)
		if changed || got != text {
			t.Fatal("rewrote an unverified declaration or quotation")
		}
	}
}

func TestHarnessCompatibilityPreservesToolsAndHistory(t *testing.T) {
	const intro = "You are an AI agent powered by DeepSeek Harness."
	body := map[string]any{
		"messages": []any{
			map[string]any{"role": "system", "content": []any{map[string]any{"type": "text", "text": intro + "\nAvailable skills: keep-this. Never execute unapproved tools."}}},
			map[string]any{"role": "user", "content": intro},
			map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "call1", "type": "function", "function": map[string]any{"name": "custom", "arguments": `{"value":"` + intro + `"}`}}}},
			map[string]any{"role": "tool", "tool_call_id": "call1", "content": intro},
		},
		"tools": []any{map[string]any{"type": "function", "function": map[string]any{"name": "custom", "description": intro, "parameters": map[string]any{"type": "object"}}}},
	}
	before, _ := json.Marshal(body)
	got, changed := ChannelCompatibilityBody(body, 1)
	if !changed {
		t.Fatal("known leading system declaration was missed")
	}
	oldMessages := body["messages"].([]any)
	newMessages := got["messages"].([]any)
	if !reflect.DeepEqual(oldMessages[1:], newMessages[1:]) || !reflect.DeepEqual(body["tools"], got["tools"]) {
		t.Fatal("changed tools, business arguments or conversation history")
	}
	text := newMessages[0].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.HasSuffix(text, "\nAvailable skills: keep-this. Never execute unapproved tools.") {
		t.Fatal("lost skill or permission instructions")
	}
	after, _ := json.Marshal(body)
	if string(before) != string(after) {
		t.Fatal("mutated original body")
	}
	if _, again := ChannelCompatibilityBody(got, 1); again {
		t.Fatal("compatibility is not idempotent")
	}
}
