package app

import (
	"net/http"
	"strconv"
	"work2api/internal/store"
)

func (s *Server) portalKey(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	owned, err := s.o.db.UserAppOwned(id, portalCtx(r).ID)
	if err != nil {
		writeAPIErr(w, errBody(500, "查询失败", "server_error"))
		return
	}
	if owned == nil {
		writeAPIErr(w, errBody(404, "Key 不存在", "invalid_request_error"))
		return
	}
	enc, _ := owned["key_enc"].(string)
	hash, _ := owned["key_hash"].(string)
	key := s.o.crypto.DecryptVerified(enc, hash)
	if key == "" {
		writeJSON(w, 200, map[string]any{"ok": false, "unavailable": true, "message": "该 Key 未保存可恢复密文，或主密钥验证失败；请联系管理员检查备份，无法恢复时重新创建 Key"})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "key": key})
}

func logPage(r *http.Request) (int, int) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 || page > 1000000 {
		page = 1
	}
	size, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if size < 1 || size > 100 {
		size = 20
	}
	return page, size
}

// Explicit public projection: no private pool identities, request/response text,
// raw upstream errors, credentials, IPs or account configuration.
func portalLog(row map[string]any) map[string]any {
	out := map[string]any{}
	for _, key := range []string{"id", "ts", "model", "protocol", "input_tokens", "output_tokens", "total_tokens", "latency_ms", "status", "app_name", "reasoning_effort", "tokens_known", "cached_tokens", "logged_attempts"} {
		out[key] = row[key]
	}
	if diag, ok := row["diagnostics"].(store.UsageDiagnostics); ok {
		out["diagnostics"] = diag
	}
	if row["status"] == "error" {
		out["error"] = "调用失败，请向管理员提供请求标识以定位原因"
	}
	if history, ok := row["attempt_records"].([]map[string]any); ok {
		items := []map[string]any{}
		for _, item := range history {
			items = append(items, portalLog(item))
		}
		out["attempt_records"] = items
	}
	return out
}

func (s *Server) portalUsageRecords(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	user := portalCtx(r).ID
	page, size := logPage(r)
	q := r.URL.Query()
	rows, total, err := s.o.db.UsageRequests(&user, size, (page-1)*size, q.Get("protocol"), q.Get("model"), nil, q.Get("status"), "")
	if err != nil {
		writeAPIErr(w, errBody(500, "查询调用日志失败", "server_error"))
		return
	}
	items := []map[string]any{}
	for _, row := range rows {
		items = append(items, portalLog(row))
	}
	writeJSON(w, 200, map[string]any{"records": items, "total": total, "page": page, "page_size": size})
}
func (s *Server) portalUsageRecord(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	user := portalCtx(r).ID
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	row, err := s.o.db.UsageRequest(id, &user)
	if err != nil {
		writeAPIErr(w, errBody(500, "查询调用详情失败", "server_error"))
		return
	}
	if row == nil {
		writeAPIErr(w, errBody(404, "调用记录不存在", "invalid_request_error"))
		return
	}
	writeJSON(w, 200, map[string]any{"record": portalLog(row)})
}
