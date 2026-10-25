package checkin

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"work2api/internal/qoder/account"
)

func TestInvalidCheckinAuthStopsFurtherRequestsUntilNewToken(t *testing.T) {
	root := account.DataRoot()
	account.SetDataRoot(t.TempDir())
	t.Cleanup(func() { account.SetDataRoot(root) })
	old := checkinClient
	t.Cleanup(func() { checkinClient = old })
	calls := 0
	checkinClient = &http.Client{Transport: statsTestTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}, nil
	})}
	for i := 0; i < 3; i++ {
		res := CheckinWithTokenContext(context.Background(), "fixture", "disabled account", "expired")
		if res.Status != "reauthorize" {
			t.Fatal(res)
		}
	}
	if calls != 1 {
		t.Fatalf("invalid auth produced %d requests, including optional statistics", calls)
	}
	maintenance.Lock()
	maintenance.guard = nil
	maintenance.Unlock()
	CheckinWithTokenContext(context.Background(), "fixture", "disabled account", "expired")
	if calls != 1 {
		t.Fatal("restart bypassed authorization pause")
	}
	CheckinWithTokenContext(context.Background(), "fixture", "disabled account", "new-token")
	if calls != 2 {
		t.Fatal("new credential did not resume verification")
	}
}
