package streamwatch

import (
	"context"
	"errors"
	"io"
)

type responseLimitKey struct{}
type bufferedResponseLimitKey struct{}

const DefaultMaxLineBytes = 40 << 20
const DefaultResponseBytes = 128 << 20

var ErrResponseTooLarge = errors.New("upstream response exceeds configured byte limit")

func WithResponseLimit(ctx context.Context, limit int64) context.Context {
	if limit == 0 {
		buffered := ResponseLimit(ctx)
		if buffered == 0 {
			buffered, _ = ctx.Value(bufferedResponseLimitKey{}).(int64)
		}
		if buffered <= 0 {
			buffered = DefaultResponseBytes
		}
		ctx = context.WithValue(ctx, bufferedResponseLimitKey{}, buffered)
	}
	return context.WithValue(ctx, responseLimitKey{}, limit)
}

func ResponseLimit(ctx context.Context) int64 {
	if n, ok := ctx.Value(responseLimitKey{}).(int64); ok && n >= 0 {
		return n
	}
	return DefaultResponseBytes
}

func LimitReader(ctx context.Context, r io.Reader) io.Reader {
	limit := ResponseLimit(ctx)
	if limit == 0 {
		return r
	}
	return &responseReader{r: r, left: limit}
}

// ReadBody always bounds buffered JSON/error bodies, even in an unbounded SSE
// context. A smaller diagnostic limit may be supplied by callers.
func ReadBody(ctx context.Context, body io.ReadCloser, limit int64) ([]byte, error) {
	current := ResponseLimit(ctx)
	if current == 0 {
		current, _ = ctx.Value(bufferedResponseLimitKey{}).(int64)
		if current <= 0 {
			current = DefaultResponseBytes
		}
	}
	if limit > 0 && limit < current {
		current = limit
	}
	ctx = WithResponseLimit(ctx, current)
	watch := NewWatch(body, ctx, 0, 0)
	defer watch.Close()
	data, err := io.ReadAll(watch.Reader(body))
	if breach := watch.Err(); breach != nil {
		return nil, breach
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return data, err
}

type responseReader struct {
	r    io.Reader
	left int64
}

func (r *responseReader) Read(p []byte) (int, error) {
	if int64(len(p)) > r.left+1 {
		p = p[:r.left+1]
	}
	n, err := r.r.Read(p)
	if int64(n) > r.left {
		return 0, ErrResponseTooLarge
	}
	r.left -= int64(n)
	return n, err
}
