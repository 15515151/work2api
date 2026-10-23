package billing

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreditFailuresRetainSafeClassification(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     int
		body, kind string
	}{
		{"HTML bearer denial", 401, "<html>401 Authorization Required private-marker</html>", "unauthorized"},
		{"explicit expired session", 200, `{"code":12153}`, "login_expired"},
		{"provider outage", 503, `{"private":"private-marker"}`, "upstream"},
		{"access denied", 403, `{}`, "forbidden"},
		{"invalid success payload", 200, "<html>private-marker</html>", "invalid_response"},
		{"empty successful document", 200, `{"code":0,"data":{}}`, "unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); w.Write([]byte(tc.body)) }))
			defer s.Close()
			_, e := FetchCredits(context.Background(), checkinCredential{s.URL})
			var q *QueryError
			if !errors.As(e, &q) || q.Kind != tc.kind {
				t.Fatalf("got %v", e)
			}
			if strings.Contains(e.Error(), "private-marker") {
				t.Fatal("raw upstream response leaked")
			}
		})
	}
}
