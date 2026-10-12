package opencode

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"work2api/internal/core/protocol"
	"work2api/internal/streamwatch"
)

func doInferenceAttempt(client *http.Client, req *http.Request, from protocol.Protocol) (*http.Response, error) {
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode/100 != 2 || !strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		return resp, err
	}
	watch := streamwatch.NewWatch(resp.Body, req.Context(), 0, 0)
	reader := bufio.NewReader(watch.Reader(resp.Body))
	prefix, err := protocol.ReadStreamHead(reader, from)
	watch.Close()
	if err != nil {
		resp.Body.Close()
		if req.Context().Err() != nil {
			return nil, req.Context().Err()
		}
		var rejection *protocol.StreamHeadError
		if errors.As(err, &rejection) {
			resp.StatusCode = rejection.Status
			resp.Status = fmt.Sprintf("%d %s", rejection.Status, http.StatusText(rejection.Status))
			resp.Header.Set("Content-Type", "application/json")
			resp.Header.Del("Content-Length")
			resp.ContentLength = int64(len(rejection.Body))
			resp.Body = io.NopCloser(bytes.NewReader(rejection.Body))
			return resp, nil
		}
		return nil, err
	}
	resp.Body = &streamPrefixBody{Reader: io.MultiReader(bytes.NewReader(prefix), reader), body: resp.Body}
	return resp, nil
}

type streamPrefixBody struct {
	io.Reader
	body io.ReadCloser
}

func (b *streamPrefixBody) Close() error { return b.body.Close() }
