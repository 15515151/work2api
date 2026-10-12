package streamwatch

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestResponseLimitRejectsOverflowWithoutSilentTruncation(t *testing.T) {
	ctx := WithResponseLimit(context.Background(), 4)
	for _, tc := range []struct {
		input    string
		overflow bool
	}{{"abcd", false}, {"abcde", true}} {
		data, err := io.ReadAll(LimitReader(ctx, strings.NewReader(tc.input)))
		if tc.overflow {
			if !errors.Is(err, ErrResponseTooLarge) {
				t.Fatal("overflow silently truncated", err)
			}
		} else if err != nil || string(data) != tc.input {
			t.Fatal("exact limit rejected", err)
		}
	}
}

func TestStreamingResponseHasNoCumulativeCapButJSONStillBounded(t *testing.T) {
	ctx := WithResponseLimit(context.Background(), 4)
	ctx = WithResponseLimit(ctx, 0)
	raw := "longer than the non-streaming byte budget"
	data, err := io.ReadAll(LimitReader(ctx, strings.NewReader(raw)))
	if err != nil || string(data) != raw {
		t.Fatal("stream was cumulatively limited", err)
	}
	if _, err := ReadBody(ctx, io.NopCloser(strings.NewReader("abcde")), 0); !errors.Is(err, ErrResponseTooLarge) {
		t.Fatal("stream context removed JSON/configured safety bound", err)
	}
	if _, err := ReadBody(ctx, io.NopCloser(strings.NewReader("abc")), 2); !errors.Is(err, ErrResponseTooLarge) {
		t.Fatal("error body limit lost", err)
	}
}
