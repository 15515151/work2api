package billing

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBillingPaginationPreservesAllRows(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var q map[string]any
		json.NewDecoder(r.Body).Decode(&q)
		rows := []any{}
		n := 100
		if q["PageNumber"].(float64) == 2 {
			n = 1
		}
		for i := 0; i < n; i++ {
			rows = append(rows, map[string]any{"index": calls*100 + i})
		}
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"Accounts": rows}})
	}))
	defer s.Close()
	q := map[string]any{"PageNumber": 1, "PageSize": 100}
	rows, e := pagedAccounts(context.Background(), checkinCredential{s.URL}, s.URL, q)
	if e != nil || len(rows) != 101 || calls != 2 || q["PageNumber"] != 1 {
		t.Fatal("pagination failed", len(rows), calls, e)
	}
}
