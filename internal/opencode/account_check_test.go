package opencode

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestReadOnlyAccountCheckDoesNotClaimKeyValidity(t *testing.T) {
	calls := 0
	proxy := &proxyTransport{client: &http.Client{Transport: catalogTestTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.Method != "GET" {
			t.Fatal("read-only check made billed request")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"m"}]}`))}, nil
	})}}
	proxy.healthy.Store(true)
	transports := &transportPool{items: []*proxyTransport{proxy}}
	nodes, err := newNodePool([]string{"fixture-key"}, transports, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	rt := &Runtime{cfg: Config{}, transports: transports, zenNodes: nodes, goNodes: &nodePool{}}
	result := rt.CheckAccount(context.Background(), PortalKeyUID("fixture-key"), true)
	if calls != 1 || result.Status != "unknown" || !proxy.healthy.Load() || nodes.nodes[0].cooldownUntil.Load() != 0 {
		t.Fatal("public directory falsely verified credentials or changed dispatch", result, calls)
	}
}
