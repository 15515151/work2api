package upstream

import "testing"

func TestClassifyLongContext5xxAsInputTooLarge(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
	}{
		{502, `{"error":"input too large"}`},
		{500, `{"message":"context length exceeds limit"}`},
		{503, `{"message":"上下文过长，请压缩后重试"}`},
		{413, `too large`},
	} {
		if got := Classify(tc.status, tc.body); got != ErrInputTooLarge {
			t.Fatalf("status=%d body=%q: got %v, want ErrInputTooLarge", tc.status, tc.body, got)
		}
	}
	if got := Classify(502, `{"error":"bad gateway"}`); got != ErrServer {
		t.Fatalf("plain 502: got %v, want ErrServer", got)
	}
}
