package app

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"work2api/internal/store"
)

func TestAccountHealthEvidenceIsBoundedAndCredentialFree(t *testing.T) {
	var h accountHealthObservations
	h.observe("workbuddy", store.UsageParams{AccountUID: "a", Status: "error", Error: `{"code":11128,"msg":"secret-marker"}`, InputContent: "private-input"})
	row, ok := h.snapshot("workbuddy", "a")
	if !ok || row.BusinessCode != "11128" || row.LastFailure == 0 || row.LastSuccess != 0 {
		t.Fatal(row)
	}
	encoded, _ := json.Marshal(row)
	if strings.Contains(string(encoded), "secret-marker") || strings.Contains(string(encoded), "private-input") {
		t.Fatal("stored raw content")
	}
	if _, ok := h.snapshot("trae", "a"); ok {
		t.Fatal("cross-provider collision")
	}
	h.observe("workbuddy", store.UsageParams{AccountUID: "a", Status: "ok"})
	row, _ = h.snapshot("workbuddy", "a")
	if row.LastSuccess == 0 || row.LastFailure == 0 {
		t.Fatal("lost history", row)
	}
	for i := 0; i < 4100; i++ {
		h.observe("qoder", store.UsageParams{AccountUID: fmt.Sprint(i), Status: "ok"})
	}
	if len(h.rows) != 4096 {
		t.Fatal("unbounded observations", len(h.rows))
	}
}

func TestAccountHealthDoesNotInventIdentityOrSuccess(t *testing.T) {
	var h accountHealthObservations
	h.observe("qoder", store.UsageParams{Status: "ok"})
	if len(h.rows) != 0 {
		t.Fatal("missing account accepted")
	}
	h.observe("qoder", store.UsageParams{AccountUID: "a", Status: "ok"})
	if _, ok := h.snapshot("qoder", "qoder:a"); !ok {
		t.Fatal("Qoder display identity lost its observation")
	}
	h.observe("trae", store.UsageParams{AccountUID: "a", Status: "incomplete"})
	row, _ := h.snapshot("trae", "a")
	if row.LastSuccess != 0 || row.FailureKind != "incomplete" {
		t.Fatal("incomplete mistaken for success", row)
	}
}
