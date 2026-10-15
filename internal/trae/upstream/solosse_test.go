package upstream

import "testing"

func TestParseSOLOLineKnownHeartbeatOnly(t *testing.T) {
	for _, data := range []string{`"working"`, `{"stage":"thinking"}`, `null`, `42`} {
		ev, err := ParseSOLOLine("progress_notice", data)
		if err != nil || ev.Response != "" || ev.Reasoning != "" || ev.Event != "progress_notice" {
			t.Fatalf("heartbeat: %+v %v", ev, err)
		}
	}
	for _, event := range []string{"output", "error", "done", "token_usage", "unknown"} {
		for _, data := range []string{`"text"`, `not-json`, `[DONE]`, `{"broken":`} {
			if _, err := ParseSOLOLine(event, data); err == nil {
				t.Fatalf("accepted malformed %s: %s", event, data)
			}
		}
	}
	if _, err := ParseSOLOLine("progress_notice", `"unfinished`); err == nil {
		t.Fatal("malformed heartbeat accepted")
	}
	ev, err := ParseSOLOLine("output", `{"response":"answer"}`)
	if err != nil || ev.Response != "answer" {
		t.Fatalf("output lost: %+v %v", ev, err)
	}
}
