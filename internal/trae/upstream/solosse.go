// Adapted from mmqz/cpa-multi-plugins, MIT; see NOTICE.md.
package upstream

import (
	"encoding/json"
	"fmt"
	"strings"
)

type SOLOEvent struct {
	Event        string          // metadata | timing_cost | output | extra_info | token_usage | done | error
	Response     string          // output: content 增量
	Reasoning    string          // output: 思考链增量
	ToolCalls    json.RawMessage // output: 工具调用（null 或对象/数组）
	Usage        map[string]any  // token_usage
	FinishReason string          // done
	ErrorCode    int64           // error
	ErrorMessage string          // error
}
type SOLOStreamError struct {
	Code int64
	Msg  string
}

func (e *SOLOStreamError) Error() string {
	return fmt.Sprintf("solo error code=%d msg=%s", e.Code, e.Msg)
}

var soloPlanLimitCodes = map[int64]struct{}{1005: {}, 4008: {}}
var soloModelMismatchCodes = map[int64]struct{}{4001: {}}

func IsPlanLimitCode(code int64) bool {
	_, ok := soloPlanLimitCodes[code]
	return ok
}
func IsModelMismatchCode(code int64) bool {
	_, ok := soloModelMismatchCodes[code]
	return ok
}
func (e *SOLOStreamError) Kind() ErrKind {
	if _, ok := soloPlanLimitCodes[e.Code]; ok {
		return ErrPlanLimit
	}
	if MsgIndicatesInputTooLarge(e.Msg) {
		return ErrInputTooLarge
	}
	if _, ok := soloModelMismatchCodes[e.Code]; ok {
		return ErrModelUnavailable
	}
	if e.Code == 429 {
		return ErrSoftRate
	}
	if e.Code == 401 {
		return ErrSessionDead
	}
	if e.Code >= 500 && e.Code <= 599 {
		return ErrServer
	}
	return ErrClient
}
func ParseSOLOLine(eventName, dataLine string) (*SOLOEvent, error) {
	ev := &SOLOEvent{Event: strings.TrimSpace(eventName)}
	trimmed := strings.TrimSpace(dataLine)
	if trimmed == "" {
		return ev, nil
	}
	if trimmed == "[DONE]" {
		if ev.Event == "" {
			ev.Event = "done"
		}
		if ev.Event == "done" && ev.FinishReason == "" {
			ev.FinishReason = "stop"
		}
		return ev, nil
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
		// Payloads that open with { or [ meant to be structured data: a
		// parse failure there is corruption, not a shape variant, and must
		// still fail the stream. Bare strings and other primitives fall
		// through to the tolerant path below.
		if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
			return nil, err
		}
		return parseNonObjectSOLOData(ev, trimmed), nil
	}
	applySOLOData(ev, raw)
	return ev, nil
}

// parseNonObjectSOLOData handles SSE data payloads that are not JSON objects.
// Some models (e.g. doubao-seed-2.1-turbo) emit a bare JSON string on control
// events; failing the whole stream on that shape mismatch turns a healthy
// long-context reply into a 502. Unwrap one string layer and map the result
// onto the event; unknown shapes are skipped without an error so the reader
// advances to the next event.
func parseNonObjectSOLOData(ev *SOLOEvent, trimmed string) *SOLOEvent {
	var s string
	if err := json.Unmarshal([]byte(trimmed), &s); err != nil {
		plain := strings.TrimSpace(trimmed)
		if plain == "[DONE]" {
			if ev.Event == "" {
				ev.Event = "done"
			}
			if ev.Event == "done" {
				ev.FinishReason = "stop"
			}
			return ev
		}
		switch ev.Event {
		case "output":
			ev.Response = plain
		case "error":
			ev.ErrorMessage = plain
		}
		return ev
	}
	inner := strings.TrimSpace(s)
	if inner == "" {
		return ev
	}
	if inner == "[DONE]" {
		if ev.Event == "" {
			ev.Event = "done"
		}
		if ev.Event == "done" {
			ev.FinishReason = "stop"
		}
		return ev
	}
	var innerRaw map[string]any
	if err := json.Unmarshal([]byte(inner), &innerRaw); err == nil {
		applySOLOData(ev, innerRaw)
		return ev
	}
	switch ev.Event {
	case "output":
		ev.Response = s
	case "error":
		ev.ErrorMessage = s
	case "done":
		ev.FinishReason = s
		if ev.FinishReason == "" {
			ev.FinishReason = "stop"
		}
	}
	return ev
}

func applySOLOData(ev *SOLOEvent, raw map[string]any) {
	switch ev.Event {
	case "output":
		if v, ok := raw["response"].(string); ok {
			ev.Response = v
		}
		if v, ok := raw["reasoning_content"].(string); ok {
			ev.Reasoning = v
		}
		if tc, ok := raw["tool_calls"]; ok {
			ev.ToolCalls, _ = json.Marshal(tc)
		}
	case "token_usage":
		ev.Usage = raw
	case "extra_info":
		ev.Usage = raw
	case "done":
		if v, ok := raw["finish_reason"].(string); ok {
			ev.FinishReason = v
		}
	case "error":
		if v, ok := raw["code"].(float64); ok {
			ev.ErrorCode = int64(v)
		}
		if v, ok := raw["message"].(string); ok {
			ev.ErrorMessage = v
		}
		if ev.ErrorMessage == "" {
			ev.ErrorMessage, _ = raw["msg"].(string)
		}
	}
}
