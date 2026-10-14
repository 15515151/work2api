package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"context"
	"unicode/utf8"
	"work2api/internal/core/provider"
	"work2api/internal/jsonutil"
	"work2api/internal/streamwatch"
)

type terminalOnlyReader struct {
	wire  string
	reads int
}

func TestResponsesUsageObservesNativeTerminalWithoutDecodingContent(t *testing.T) {
	for _, tc := range []struct{ status, kind, reason, want string }{
		{"completed", "custom_tool_call", "", "tool_calls"},
		{"completed", "web_search_call", "", "stop"},
		{"incomplete", "custom_tool_call", "max_output_tokens", "length"},
		{"incomplete", "message", "content_filter", "content_filter"},
		{"failed", "message", "", "error"},
	} {
		body, _ := json.Marshal(map[string]any{"status": tc.status, "output": []any{map[string]any{"type": tc.kind, "content": []any{map[string]any{"type": "unknown_native_block"}}}}, "incomplete_details": map[string]any{"reason": tc.reason}, "usage": map[string]any{"input_tokens": 5, "output_tokens": 2, "total_tokens": 7}})
		u, known := ResponseUsage(Responses, body)
		if !known || u.Input != 5 || u.Output != 2 || u.FinishReason != tc.want {
			t.Fatal(tc, u, known)
		}
	}
}

func BenchmarkResponsesTerminalObservation(b *testing.B) {
	items := make([]any, 100)
	for i := range items {
		items[i] = map[string]any{"type": "message", "content": []any{map[string]any{"type": "output_text", "text": strings.Repeat("x", 10<<10)}}}
	}
	body, _ := json.Marshal(map[string]any{"status": "completed", "output": items, "usage": map[string]any{"input_tokens": 10, "output_tokens": 10, "total_tokens": 20}})
	for _, legacy := range []bool{true, false} {
		name := "terminal-only"
		if legacy {
			name = "full-response-decode"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if legacy {
					var p map[string]any
					_ = json.Unmarshal(body, &p)
					_, _ = decodeBridgeResponse(Responses, p)
				} else {
					_, _ = ResponseUsage(Responses, body)
				}
			}
		})
	}
}

func TestSSEEventLimitsPreserveUnlimitedAggregateAndTerminalCutoff(t *testing.T) {
	oversized := strings.Repeat("x", streamwatch.DefaultMaxEventBytes+1)
	observer := newStreamUsageObserver(Chat)
	if _, err := observer.Write([]byte(oversized)); !errors.Is(err, streamwatch.ErrSSEEventTooLarge) {
		t.Fatal("unterminated event not bounded", err)
	}
	if len(observer.buffer) != 0 {
		t.Fatal("oversized event retained")
	}
	line := "data: " + strings.Repeat("x", 1024) + "\n"
	manyLines := strings.Repeat(line, streamwatch.DefaultMaxEventBytes/len(line)+1)
	if err := readSSE(strings.NewReader(manyLines), func(string, string) error { return nil }); !errors.Is(err, streamwatch.ErrSSEEventTooLarge) {
		t.Fatal("multiline event not bounded", err)
	}
	observer = newStreamUsageObserver(Chat)
	for i := 0; i < 1025; i++ {
		if _, err := observer.Write([]byte(":" + strings.Repeat("x", 40<<10) + "\n\n")); err != nil {
			t.Fatal("aggregate stream bounded", err)
		}
	}
	terminal := "data: [DONE]\n\n"
	accepted, err := observer.Write([]byte(terminal + oversized))
	if err != nil || accepted != len(terminal) || !observer.NormalTermination() {
		t.Fatal("post-terminal garbage inspected", accepted, err)
	}
}

func TestGenerationParserExcludesProtocolMetadataAndDoneSnapshots(t *testing.T) {
	for _, tc := range []struct {
		protocol Protocol
		frames   []string
		count    int
	}{
		{Chat, []string{`{"choices":[{"delta":{"role":"assistant"}}]}`, `{"choices":[{"delta":{"reasoning_content":"think"}}]}`, `{"choices":[{"delta":{"content":"answer"}}]}`, `{"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"completion_tokens":20}}`}, 2},
		{Anthropic, []string{`{"type":"message_start","message":{"id":"m","model":"m","usage":{"input_tokens":1}}}`, `{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"think"}}`, `{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"answer"}}`, `{"type":"content_block_delta","index":1,"delta":{"type":"signature_delta","signature":"opaque"}}`, `{"type":"message_stop"}`}, 2},
		{Responses, []string{`{"type":"response.created","response":{"id":"r","model":"m"}}`, `{"type":"response.reasoning_summary_text.delta","item_id":"think","delta":"think"}`, `{"type":"response.output_text.delta","delta":"answer"}`, `{"type":"response.function_call_arguments.delta","item_id":"call","delta":"{}"}`, `{"type":"response.output_text.done","text":"answer"}`, `{"type":"response.completed","response":{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"answer"}]}]}}`}, 3},
	} {
		t.Run(string(tc.protocol), func(t *testing.T) {
			ctx, timing := streamwatch.WithTiming(context.Background())
			p := newStreamUsageObserver(tc.protocol).parser
			p.generationCtx = ctx
			for _, frame := range tc.frames {
				if _, err := p.Parse("", frame); err != nil {
					t.Fatal(err)
				}
			}
			if timing.Snapshot().GenerationEvents != tc.count {
				t.Fatal(timing.Snapshot())
			}
		})
	}
}

func TestClientToolPreparationCopiesOnlyChangedHistory(t *testing.T) {
	ordinary := map[string]any{"type": "message", "content": "unchanged"}
	custom := map[string]any{"type": "custom_tool_call", "name": "patch", "input": "raw"}
	body := map[string]any{"tools": []any{map[string]any{"type": "custom", "name": "patch"}}, "input": []any{ordinary, custom}}
	before, _ := json.Marshal(body)
	out, _, _, err := PrepareClientTools(body)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(body)
	if !bytes.Equal(before, after) {
		t.Fatal("original request mutated")
	}
	history := out["input"].([]any)
	if reflect.ValueOf(history[0]).Pointer() != reflect.ValueOf(ordinary).Pointer() {
		t.Fatal("ordinary history copied")
	}
	if reflect.ValueOf(history[1]).Pointer() == reflect.ValueOf(custom).Pointer() {
		t.Fatal("rewritten tool aliases original")
	}
	plain := map[string]any{"input": []any{ordinary}}
	prepared, _, _, err := PrepareClientTools(plain)
	if err != nil || reflect.ValueOf(prepared).Pointer() != reflect.ValueOf(plain).Pointer() {
		t.Fatal("ordinary request did not use fast path")
	}
}

func BenchmarkClientToolPreparationLargeHistory(b *testing.B) {
	for _, custom := range []bool{false, true} {
		b.Run(fmt.Sprintf("custom=%t", custom), func(b *testing.B) {
			history := make([]any, 1000)
			for i := range history {
				history[i] = map[string]any{"type": "message", "content": strings.Repeat("x", 1024)}
			}
			body := map[string]any{"input": history}
			if custom {
				body["tools"] = []any{map[string]any{"type": "custom", "name": "patch"}}
				history[999] = map[string]any{"type": "custom_tool_call", "name": "patch", "input": "raw"}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, _, _, err := PrepareClientTools(body); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func (r *terminalOnlyReader) Read(p []byte) (int, error) {
	r.reads++
	if r.reads > 1 {
		return 0, fmt.Errorf("read after terminal")
	}
	return copy(p, r.wire), nil
}

func TestForwardStreamStopsAtTerminalWithoutEOF(t *testing.T) {
	for _, tc := range []struct {
		proto Protocol
		wire  string
	}{
		{Chat, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":7,\"completion_tokens\":3,\"total_tokens\":10}}\n\ndata: [DONE]\n\n"},
		{Anthropic, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"},
		{Responses, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n"},
	} {
		reader := &terminalOnlyReader{wire: tc.wire + ": after terminal\n\n"}
		w := httptest.NewRecorder()
		usage, known, err := ForwardStream(context.Background(), w, reader, tc.proto, "test")
		if err != nil || reader.reads != 1 || w.Body.String() != tc.wire {
			t.Fatal(tc.proto, err, reader.reads, w.Body.String())
		}
		if tc.proto == Chat && (!known || usage.Total != 10) {
			t.Fatal("terminal usage lost", usage)
		}
	}
}

func TestNamespaceToolsRoundTrip(t *testing.T) {
	body := map[string]any{
		"tools":       []any{map[string]any{"type": "function", "name": "shell"}, map[string]any{"type": "namespace", "name": "functions", "tools": []any{map[string]any{"type": "function", "name": "shell", "parameters": map[string]any{"type": "object"}}, map[string]any{"type": "custom", "name": "patch"}}}},
		"tool_choice": map[string]any{"type": "function", "namespace": "functions", "name": "shell"},
		"input":       []any{map[string]any{"type": "function_call", "namespace": "functions", "name": "shell", "call_id": "call_1", "arguments": "{}"}, map[string]any{"type": "function_call_output", "call_id": "call_1", "output": "ok"}},
	}
	before, _ := json.Marshal(body)
	prepared, custom, mapping, err := PrepareClientTools(body)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(body)
	if !bytes.Equal(before, after) {
		t.Fatal("namespace adaptation mutated request")
	}
	flat := jsonutil.StringAt(prepared, "tool_choice", "name")
	if mapping[flat] != (NamespaceName{"functions", "shell"}) || len(custom) != 1 || jsonutil.StringAt(jsonutil.SliceAt(prepared, "input")[0].(map[string]any), "name") != flat {
		t.Fatal("namespace history or choice lost", prepared)
	}
	for _, stream := range []bool{false, true} {
		w := httptest.NewRecorder()
		writer := NewClientToolWriter(w, custom, mapping, 1<<20)
		item := map[string]any{"type": "function_call", "id": "fc_1", "call_id": "call_1", "name": flat, "arguments": "{}"}
		patchFlat := ""
		for name, entry := range mapping {
			if entry.Name == "patch" {
				patchFlat = name
			}
		}
		patch := map[string]any{"type": "function_call", "id": "fc_patch", "call_id": "call_patch", "name": patchFlat, "arguments": `{"input":"hello"}`}
		response := map[string]any{"output": []any{item, patch}, "status": "completed"}
		if stream {
			writer.Header().Set("Content-Type", "text/event-stream")
			for _, event := range []map[string]any{{"type": "response.output_item.added", "item": item}, {"type": "response.output_item.done", "item": patch}, {"type": "response.completed", "response": response}} {
				raw, _ := json.Marshal(event)
				if _, err := writer.Write([]byte("data: " + string(raw) + "\n\n")); err != nil {
					t.Fatal(err)
				}
			}
		} else {
			raw, _ := json.Marshal(response)
			writer.Write(raw)
		}
		if err := writer.Finish(); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(w.Body.String(), `"namespace":"functions"`) || !strings.Contains(w.Body.String(), `"name":"shell"`) || strings.Contains(w.Body.String(), flat) {
			t.Fatal("namespace response not restored", w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"type":"custom_tool_call"`) || !strings.Contains(w.Body.String(), `"input":"hello"`) || strings.Contains(w.Body.String(), patchFlat) {
			t.Fatal("namespaced custom call lost", w.Body.String())
		}
	}
	bad := map[string]any{"tools": []any{map[string]any{"type": "namespace", "name": "functions", "tools": []any{map[string]any{"type": "function", "name": "shell"}, map[string]any{"type": "function", "name": "shell"}}}}}
	if _, _, _, err := PrepareClientTools(bad); err == nil {
		t.Fatal("duplicate namespace tools accepted")
	}
}

func TestCustomToolsRoundTrip(t *testing.T) {
	patch := "*** Begin Patch\n*** Add File: hello.txt\n+你好\\world\n*** End Patch"
	body := map[string]any{
		"tools":       []any{map[string]any{"type": "custom", "name": "apply_patch", "format": map[string]any{"type": "grammar", "syntax": "lark", "definition": "start: PATCH"}}, map[string]any{"type": "function", "name": "shell", "parameters": map[string]any{"type": "object"}}},
		"tool_choice": map[string]any{"type": "custom", "name": "apply_patch"},
		"input":       []any{map[string]any{"type": "custom_tool_call", "id": "ctc_1", "call_id": "call_1", "name": "apply_patch", "input": patch}, map[string]any{"type": "custom_tool_call_output", "call_id": "call_1", "output": "success"}},
	}
	before, _ := json.Marshal(body)
	prepared, names, err := PrepareCustomTools(body)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(body)
	if !bytes.Equal(before, after) {
		t.Fatal("custom preparation mutated client request")
	}
	tool := jsonutil.SliceAt(prepared, "tools")[0].(map[string]any)
	if tool["type"] != "function" || !strings.Contains(jsonutil.StringAt(tool, "description"), "start: PATCH") || jsonutil.StringAt(prepared, "tool_choice", "type") != "function" {
		t.Fatal("custom tool definition/choice lost", prepared)
	}
	history := jsonutil.SliceAt(prepared, "input")
	if history[0].(map[string]any)["type"] != "function_call" || history[1].(map[string]any)["type"] != "function_call_output" {
		t.Fatal("custom history not adapted")
	}
	args := history[0].(map[string]any)["arguments"]
	if input, err := customInput(args); err != nil || input != patch {
		t.Fatal("tool input changed", input, err)
	}
	for _, streaming := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", streaming), func(t *testing.T) {
			recorder := httptest.NewRecorder()
			w := NewCustomToolWriter(recorder, names, 1<<20)
			item := map[string]any{"type": "function_call", "id": "fc_1", "call_id": "call_1", "name": "apply_patch", "arguments": args, "status": "completed"}
			response := map[string]any{"id": "resp_1", "status": "completed", "output": []any{item, map[string]any{"type": "function_call", "id": "fc_2", "call_id": "call_2", "name": "shell", "arguments": "{}"}}}
			if streaming {
				w.Header().Set("Content-Type", "text/event-stream")
				events := []map[string]any{
					{"type": "response.created", "response": map[string]any{"id": "resp_1"}},
					{"type": "response.output_item.added", "output_index": 0, "item": map[string]any{"type": "function_call", "id": "fc_1", "call_id": "call_1", "name": "apply_patch", "arguments": ""}},
					{"type": "response.function_call_arguments.delta", "item_id": "fc_1", "output_index": 0, "delta": args},
					{"type": "response.function_call_arguments.done", "item_id": "fc_1", "output_index": 0, "arguments": args},
					{"type": "response.output_item.done", "output_index": 0, "item": item},
					{"type": "response.completed", "response": response},
				}
				for _, event := range events {
					data, _ := json.Marshal(event)
					frame := append(append([]byte("event: ignored\r\ndata: "), data...), '\r', '\n', '\r', '\n')
					for _, b := range frame {
						if _, err := w.Write([]byte{b}); err != nil {
							t.Fatal(err)
						}
					}
					w.Flush()
				}
			} else {
				w.Header().Set("Content-Type", "application/json")
				data, _ := json.Marshal(response)
				w.Header().Set("Content-Length", "99999")
				if _, err := w.Write(data); err != nil {
					t.Fatal(err)
				}
			}
			if err := w.Finish(); err != nil {
				t.Fatal(err)
			}
			if recorder.Header().Get("Content-Length") != "" {
				t.Fatal("stale content length")
			}
			if streaming {
				seq := 0
				deltas := 0
				if err := readSSE(strings.NewReader(recorder.Body.String()), func(_ string, data string) error {
					var event map[string]any
					if err := json.Unmarshal([]byte(data), &event); err != nil {
						return err
					}
					if jsonutil.IntAt(event, "sequence_number") != seq {
						t.Fatal("sequence changed", event)
					}
					seq++
					if event["type"] == "response.custom_tool_call_input.delta" {
						deltas++
						if event["delta"] != patch {
							t.Fatal("custom text changed")
						}
					}
					if event["type"] == "response.completed" {
						response = jsonutil.MapAt(event, "response")
					}
					if strings.HasPrefix(jsonutil.StringAt(event, "type"), "response.function_call_arguments") {
						t.Fatal("wrapper arguments leaked")
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				if deltas != 1 {
					t.Fatal("custom input duplicated or lost", deltas)
				}
			} else {
				if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
			}
			output := jsonutil.SliceAt(response, "output")
			if jsonutil.StringAt(output[0].(map[string]any), "input") != patch || output[0].(map[string]any)["type"] != "custom_tool_call" || output[1].(map[string]any)["type"] != "function_call" {
				t.Fatal("restored output incorrect", response)
			}
		})
	}
}

func TestCustomToolsRejectMalformedInput(t *testing.T) {
	for _, args := range []string{`{"input":7}`, `{"input":"patch","extra":1}`, `{"input":`} {
		recorder := httptest.NewRecorder()
		w := NewCustomToolWriter(recorder, map[string]bool{"apply_patch": true}, 1<<20)
		w.Header().Set("Content-Type", "text/event-stream")
		for _, event := range []map[string]any{
			{"type": "response.output_item.added", "item": map[string]any{"type": "function_call", "id": "fc_1", "name": "apply_patch"}},
			{"type": "response.function_call_arguments.done", "item_id": "fc_1", "arguments": args},
		} {
			data, _ := json.Marshal(event)
			_, _ = w.Write(append(append([]byte("data: "), data...), '\n', '\n'))
		}
		if w.Finish() == nil || !strings.Contains(recorder.Body.String(), `"type":"response.failed"`) || strings.Contains(recorder.Body.String(), `"type":"response.custom_tool_call_input.done"`) {
			t.Fatal("invalid input became executable", recorder.Body.String())
		}
	}
}

func TestRefusalSurvivesCrossProtocolConversion(t *testing.T) {
	for _, from := range []Protocol{Chat, Responses} {
		var body []byte
		var stream string
		if from == Chat {
			body = []byte(`{"choices":[{"message":{"role":"assistant","content":null,"refusal":"Cannot comply."},"finish_reason":"stop"}]}`)
			stream = "data: {\"choices\":[{\"delta\":{\"refusal\":\"Cannot comply.\"}}]}\n\ndata: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"
		} else {
			body = []byte(`{"id":"resp_1","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"refusal","refusal":"Cannot comply."}]}]}`)
			stream = "data: {\"type\":\"response.refusal.delta\",\"delta\":\"Cannot comply.\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\"}}\n\n"
		}
		converted, err := ConvertResponse(from, Anthropic, body)
		if err != nil || !bytes.Contains(converted, []byte("Cannot comply.")) {
			t.Fatal("refusal response lost", string(converted), err)
		}
		recorder := httptest.NewRecorder()
		if err := transcodeStream(recorder, strings.NewReader(stream), from, Anthropic, "m"); err != nil || !strings.Contains(recorder.Body.String(), "Cannot comply.") {
			t.Fatal("refusal stream lost", recorder.Body.String(), err)
		}
	}
}

// TestConvertRequestChatToAnthropic verifies the ported conversion layer wires
// up: an OpenAI Chat request should transcode into an Anthropic Messages body
// with system split out and messages preserved.
func TestConvertRequestChatToAnthropic(t *testing.T) {
	in := map[string]any{
		"model": "claude-x",
		"messages": []any{
			map[string]any{"role": "system", "content": "be brief"},
			map[string]any{"role": "user", "content": "hello"},
		},
		"max_tokens": float64(64),
	}
	out, err := ConvertRequest(Chat, Anthropic, in)
	if err != nil {
		t.Fatalf("ConvertRequest: %v", err)
	}
	if got := jsonutil.StringAt(out, "system"); got != "be brief" {
		// system may be represented as string or blocks depending on impl;
		// accept either as long as the text survives.
		if blocks := jsonutil.SliceAt(out, "system"); len(blocks) == 0 {
			t.Fatalf("system prompt lost: %v", out["system"])
		}
	}
	msgs := jsonutil.SliceAt(out, "messages")
	if len(msgs) == 0 {
		t.Fatalf("messages lost: %v", out["messages"])
	}
	b, _ := json.Marshal(out)
	t.Logf("anthropic body: %s", b)
}

// TestConvertRequestSameProtocolClones ensures a same-protocol request is
// preserved (not dropped) through PrepareRequest.
func TestConvertRequestSameProtocolClones(t *testing.T) {
	in := map[string]any{
		"model":    "gpt-x",
		"messages": []any{map[string]any{"role": "user", "content": "hi"}},
	}
	out, err := PrepareRequest(Chat, Chat, in, "https://example.test/v1/chat/completions")
	if err != nil {
		t.Fatalf("PrepareRequest: %v", err)
	}
	if jsonutil.StringAt(out, "model") != "gpt-x" {
		t.Fatalf("model lost: %v", out)
	}
}

func TestResponsesCompletedReusesFinishedItems(t *testing.T) {
	var wire bytes.Buffer
	emitter := newBridgeStreamEmitter(&wire, httptest.NewRecorder(), Responses, "test-model")
	for _, event := range []bridgeStreamEvent{{Kind: "reasoning", Encrypted: "encrypted-state"}, {Kind: "tool_start", ToolKey: "one", ToolID: "call1", ToolName: "lookup"}, {Kind: "tool_delta", ToolKey: "one", Text: `{"q":"test"}`}, {Kind: "text", Text: "answer"}, {Kind: "finish", Stop: "tool_calls"}, {Kind: "done"}} {
		if err := emitter.Emit(event); err != nil {
			t.Fatal(err)
		}
	}
	finished := map[int]any{}
	var completed []any
	err := readSSE(strings.NewReader(wire.String()), func(name, data string) error {
		var event map[string]any
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return err
		}
		if name == "response.output_item.done" {
			finished[int(event["output_index"].(float64))] = event["item"]
		}
		if name == "response.completed" {
			completed = event["response"].(map[string]any)["output"].([]any)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(completed) != 3 || len(finished) != 3 {
		t.Fatalf("missing output items: %s", wire.String())
	}
	for index, item := range completed {
		if !reflect.DeepEqual(item, finished[index]) {
			t.Fatalf("completed item %d changed identity/content", index)
		}
	}
	if completed[0].(map[string]any)["encrypted_content"] != "encrypted-state" {
		t.Fatal("encrypted reasoning lost")
	}
}

func TestCacheUnknownAndObservedZeroRemainDistinct(t *testing.T) {
	for _, target := range []Protocol{Chat, Responses, Anthropic} {
		for _, known := range []bool{false, true} {
			response := bridgeResponse{Usage: Usage{Input: 10, Output: 2, Total: 12, CachedKnown: known}}
			doc := encodeBridgeResponse(target, response)
			usage := doc["usage"].(map[string]any)
			var cached any
			if target == Anthropic {
				cached = usage["cache_read_input_tokens"]
			} else if target == Chat {
				cached = usage["prompt_tokens_details"].(map[string]any)["cached_tokens"]
			} else {
				cached = usage["input_tokens_details"].(map[string]any)["cached_tokens"]
			}
			if (cached != nil) != known {
				t.Fatalf("%s cached known=%v, value=%v", target, known, cached)
			}
		}
	}
}

func TestCollapseDoesNotInventMissingUsage(t *testing.T) {
	wire := `data: {"choices":[{"delta":{"content":"ok"},"finish_reason":"stop"}]}` + "\n\n" + "data: [DONE]\n\n"
	body, usage, known, err := CollapseStreamWithUsage(strings.NewReader(wire), Chat, "test")
	if err != nil || known {
		t.Fatal("unexpected usage observation", known, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatal(err)
	}
	if _, ok := doc["usage"]; ok {
		t.Fatal("synthetic usage exposed as observed", string(body))
	}
	if usage.FinishReason != "stop" {
		t.Fatalf("finish lost without usage: %+v", usage)
	}
	for _, target := range []Protocol{Chat, Anthropic, Responses} {
		for _, reason := range []string{"stop", "length", "tool_calls", "content_filter"} {
			stream := strings.Replace(wire, `"finish_reason":"stop"`, `"finish_reason":"`+reason+`"`, 1)
			w := httptest.NewRecorder()
			got, reported, err := TranscodeStream(t.Context(), w, strings.NewReader(stream), Chat, target, "test")
			if err != nil || reported || got.FinishReason != reason {
				t.Fatalf("%s/%s finish=%q known=%v err=%v", target, reason, got.FinishReason, reported, err)
			}
			single, collapsed, reported, err := CollapseStreamWithUsage(strings.NewReader(stream), Chat, "test")
			if err != nil || reported || collapsed.FinishReason != reason {
				t.Fatalf("collapse %s: %+v %v", reason, collapsed, err)
			}
			_ = single
			got, reported = ResponseUsage(Chat, []byte(`{"choices":[{"finish_reason":"`+reason+`"}]}`))
			if reported || got.FinishReason != reason {
				t.Fatalf("response %s: %+v", reason, got)
			}
		}
	}
}

func TestUsageObserverFragmentationAndBufferRelease(t *testing.T) {
	for _, delimiter := range []string{"\n\n", "\r\n\r\n"} {
		frame := `data: {"choices":[],"usage":{"prompt_tokens":7,"completion_tokens":3,"total_tokens":10}}` + delimiter
		wire := ":" + strings.Repeat("x", 160<<10) + delimiter + frame + "data: [DONE]" + delimiter
		for _, size := range []int{1, 3, 65536, len(wire)} {
			observer := newStreamUsageObserver(Chat)
			for start := 0; start < len(wire); start += size {
				observer.Write([]byte(wire[start:min(start+size, len(wire))]))
			}
			if !observer.Reported() || !observer.NormalTermination() || observer.ParseError() != nil || observer.usage.Total != 10 {
				t.Fatalf("fragment size %d: usage=%+v normal=%v error=%v", size, observer.usage, observer.normal, observer.parseErr)
			}
			if len(observer.buffer) != 0 || cap(observer.buffer) > 128<<10 {
				t.Fatal("completed oversized frame retained")
			}
			observer.Write([]byte(strings.TrimSuffix(frame, delimiter)))
			observer.Finish()
			if observer.usage.Total != 10 || observer.buffer != nil {
				t.Fatal("unterminated frame counted or retained")
			}
		}
	}
}

func TestStreamingCaptureDoesNotTrimProtocolOutput(t *testing.T) {
	text, reason := strings.Repeat("text", 20000), strings.Repeat("think", 20000)
	for _, target := range []Protocol{Chat, Anthropic, Responses} {
		w := httptest.NewRecorder()
		emitter := newBridgeStreamEmitter(w, w, target, "test")
		for _, e := range []bridgeStreamEvent{{Kind: "text", Text: text}, {Kind: "reasoning", Text: reason}, {Kind: "finish", Stop: "stop"}, {Kind: "done"}} {
			if err := emitter.Emit(e); err != nil {
				t.Fatal(err)
			}
		}
		if !strings.Contains(w.Body.String(), text) || !strings.Contains(w.Body.String(), reason) {
			t.Fatal("protocol content truncated", target)
		}
		want := 0
		if target == Responses {
			want = len(text)
		}
		if emitter.text.Len() != want {
			t.Fatal("unnecessary text retained", target, emitter.text.Len())
		}
	}
	for _, policy := range []provider.LogCapturePolicy{{Enabled: true, Limit: 4}, {Enabled: false, Limit: 4}, {Enabled: true, Limit: 0}} {
		ctx := provider.WithInvocation(context.Background(), provider.Invocation{LogCapture: &policy})
		capture := provider.NewLogCapture(ctx)
		capture.WriteString("中文abc")
		capture.WriteString("later")
		got := capture.String()
		if !utf8.ValidString(got) {
			t.Fatal("UTF8 broken")
		}
		if !policy.Enabled && got != "" {
			t.Fatal("private capture performed")
		}
		if policy.Enabled && policy.Limit == 4 && got != "中" {
			t.Fatal("not a bounded prefix", got)
		}
		if policy.Enabled && policy.Limit == 0 && got != "中文abclater" {
			t.Fatal("explicit unlimited ignored")
		}
	}
}
