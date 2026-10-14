package upstream

import "testing"

func TestParseSOLOLineStringPayload(t *testing.T) {
	// Bare JSON string on an output event carries the text delta.
	ev, err := ParseSOLOLine("output", `"hello"`)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Response != "hello" {
		t.Fatalf("response = %q, want hello", ev.Response)
	}
	// String-wrapped JSON object is unwrapped one layer.
	ev, err = ParseSOLOLine("output", `"{\"response\":\"hi\"}"`)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Response != "hi" {
		t.Fatalf("response = %q, want hi", ev.Response)
	}
	// Control events with string data must not fail the stream.
	for _, event := range []string{"metadata", "timing_cost", ""} {
		if _, err := ParseSOLOLine(event, `"ok"`); err != nil {
			t.Fatalf("event %q: unexpected error %v", event, err)
		}
	}
	// Bare [DONE] sentinel without an event name.
	ev, err = ParseSOLOLine("", "[DONE]")
	if err != nil {
		t.Fatal(err)
	}
	if ev.Event != "done" {
		t.Fatalf("event = %q, want done", ev.Event)
	}
	// Object payloads keep working.
	ev, err = ParseSOLOLine("output", `{"response":"x"}`)
	if err != nil || ev.Response != "x" {
		t.Fatalf("object payload broken: %+v %v", ev, err)
	}
	// Error string payload maps to the message.
	ev, err = ParseSOLOLine("error", `"boom"`)
	if err != nil || ev.ErrorMessage != "boom" {
		t.Fatalf("error payload broken: %+v %v", ev, err)
	}
}
