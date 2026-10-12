package app

import (
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
	"work2api/internal/store"
)

func inviteSettings(body map[string]any, create bool) (int64, float64, bool) {
	maxUses := int64(1)
	if v, exists := body["max_uses"]; exists {
		n, ok := v.(float64)
		if !ok || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n > 9007199254740991 || math.Trunc(n) != n {
			return 0, 0, false
		}
		maxUses = int64(n)
	} else if !create {
		return 0, 0, false
	}
	expires := float64(time.Now().Unix()) + 7*86400
	if v, exists := body["expires_at"]; exists {
		if _, both := body["valid_days"]; both {
			return 0, 0, false
		}
		n, ok := v.(float64)
		if !ok || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n > 253402300799 || (create && n != 0 && n <= float64(time.Now().Unix())) {
			return 0, 0, false
		}
		expires = n
	} else if v, exists := body["valid_days"]; exists && create {
		n, ok := v.(float64)
		if !ok || n <= 0 || n > 365 || math.IsNaN(n) || math.IsInf(n, 0) {
			return 0, 0, false
		}
		expires = float64(time.Now().Unix()) + n*86400
	} else if !create {
		return 0, 0, false
	}
	return maxUses, expires, true
}

func (s *Server) adminPortalUpdateInvite(w http.ResponseWriter, r *http.Request) {
	body, e := readJSON(r)
	maxUses, expires, valid := inviteSettings(body, false)
	enabled, ok := body["enabled"].(bool)
	revision, isRevision := body["revision"].(float64)
	if e != nil || !valid || !ok || !isRevision || revision < 1 || revision > 9007199254740991 || math.Trunc(revision) != revision {
		writeAPIErr(w, errBody(400, "请填写有效次数、到期时间、启用状态和配置版本", "invalid_request_error"))
		return
	}
	code := strings.ToUpper(r.PathValue("code"))
	if !inviteCodeRe.MatchString(code) {
		writeAPIErr(w, errBody(400, "无效邀请码", "invalid_request_error"))
		return
	}
	if e = s.o.db.UpdateInviteCode(code, maxUses, expires, enabled, int64(revision)); e != nil {
		status := 503
		message := "保存邀请码失败"
		if e == store.ErrConflict {
			status = 409
			message = "配置已变更，或次数上限低于已用次数，请刷新后重试"
		}
		writeAPIErr(w, errBody(status, message, "invite_update_failed"))
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) adminPortalInviteHistory(w http.ResponseWriter, r *http.Request) {
	code := strings.ToUpper(r.PathValue("code"))
	item, e := s.o.db.FindInviteCode(code)
	if e != nil {
		writeAPIErr(w, errBody(503, "邀请码查询失败", "server_error"))
		return
	}
	if item == nil {
		writeAPIErr(w, errBody(404, "邀请码不存在", "not_found"))
		return
	}
	offset := 0
	if raw := r.URL.Query().Get("offset"); raw != "" {
		var err error
		offset, err = strconv.Atoi(raw)
		if err != nil || offset < 0 {
			writeAPIErr(w, errBody(400, "无效记录页", "invalid_request_error"))
			return
		}
	}
	records, e := s.o.db.InviteRedemptions(code, offset)
	if e != nil {
		writeAPIErr(w, errBody(503, "使用记录查询失败", "server_error"))
		return
	}
	next := -1
	if len(records) > 50 {
		records = records[:50]
		next = offset + 50
	}
	writeJSON(w, 200, map[string]any{"records": records, "next_offset": next})
}

func inviteInt(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case bool:
		if n {
			return 1
		}
	case float64:
		return int64(n)
	}
	return 0
}
