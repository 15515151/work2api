package runtime

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"work2api/internal/workbuddy/upstream"
)

// Some WorkBuddy deployments translate end_turn to stop even after tools.
// Track structured tool calls only; never infer executable tools from prose.
type toolFinishNormalizer struct {
	seen     map[int]bool
	finished map[int]bool
	metadata map[string]any
}

func (n *toolFinishNormalizer) lines(ctx context.Context, line string) []string {
	if !strings.HasPrefix(line, "data:") {
		return []string{line}
	}
	data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
	if n.seen == nil {
		n.seen = map[int]bool{}
		n.finished = map[int]bool{}
		n.metadata = map[string]any{}
	}
	if data == "[DONE]" {
		choices := []any{}
		indices := make([]int, 0, len(n.seen))
		for index := range n.seen {
			indices = append(indices, index)
		}
		sort.Ints(indices)
		for _, index := range indices {
			if n.seen[index] && !n.finished[index] {
				choices = append(choices, map[string]any{"index": index, "delta": map[string]any{}, "finish_reason": "tool_calls"})
			}
		}
		if len(choices) > 0 {
			chunk := make(map[string]any, len(n.metadata)+1)
			for key, value := range n.metadata {
				chunk[key] = value
			}
			chunk["choices"] = choices
			raw, _ := json.Marshal(chunk)
			return []string{"data: " + string(raw), line}
		}
		return []string{line}
	}
	chunk := upstream.DecodeFrame(ctx, line)
	if chunk == nil {
		return []string{line}
	}
	for _, key := range []string{"id", "object", "model", "created", "system_fingerprint"} {
		if value, ok := chunk[key]; ok {
			n.metadata[key] = value
		}
	}
	choices, _ := chunk["choices"].([]any)
	changed := false
	for _, raw := range choices {
		choice, _ := raw.(map[string]any)
		index, _ := choice["index"].(float64)
		delta, _ := choice["delta"].(map[string]any)
		calls, _ := delta["tool_calls"].([]any)
		for _, rawCall := range calls {
			call, _ := rawCall.(map[string]any)
			function, _ := call["function"].(map[string]any)
			if name, _ := function["name"].(string); name != "" {
				n.seen[int(index)] = true
			}
		}
		finish, _ := choice["finish_reason"].(string)
		if finish == "stop" && n.seen[int(index)] {
			choice["finish_reason"] = "tool_calls"
			changed = true
		}
		if finish != "" {
			n.finished[int(index)] = true
		}
	}
	if changed {
		raw, _ := json.Marshal(chunk)
		normalized := "data: " + string(raw)
		upstream.RememberFrame(ctx, normalized, chunk)
		return []string{normalized}
	}
	return []string{line}
}
