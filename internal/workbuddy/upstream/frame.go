package upstream

import (
	"context"
	"encoding/json"
	"strings"
)

type frameKey struct{}
type parsedFrame struct {
	line  string
	chunk map[string]any
}

// One synchronous request pipeline owns this cursor. Nothing is cached across
// requests or accounts; consumers finish with a frame before the next arrives.
func WithParsedFrames(ctx context.Context) context.Context {
	return context.WithValue(ctx, frameKey{}, &parsedFrame{})
}

func RememberFrame(ctx context.Context, line string, chunk map[string]any) {
	if frame, _ := ctx.Value(frameKey{}).(*parsedFrame); frame != nil {
		frame.line, frame.chunk = line, chunk
	}
}

func DecodeFrame(ctx context.Context, line string) map[string]any {
	if ctx != nil {
		if frame, _ := ctx.Value(frameKey{}).(*parsedFrame); frame != nil && frame.line == line {
			return frame.chunk
		}
	}
	if !strings.HasPrefix(line, "data:") {
		return nil
	}
	data := strings.TrimSpace(line[5:])
	if data == "" || data == "[DONE]" {
		return nil
	}
	var chunk map[string]any
	if json.Unmarshal([]byte(data), &chunk) != nil {
		return nil
	}
	if ctx != nil {
		RememberFrame(ctx, line, chunk)
	}
	return chunk
}
