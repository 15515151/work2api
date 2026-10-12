package app

import (
	"context"
	"net/http"
	"strings"
	"time"
	"work2api/internal/selfupdate"
)

func (s *Server) SetUpdater(m *selfupdate.Manager) { s.updater = m }
func (s *Server) adminUpdate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.updater == nil {
		writeAPIErr(w, errBody(503, "当前启动方式未初始化更新功能", "updater_unavailable"))
		return
	}
	if r.Method == "GET" {
		writeJSON(w, 200, s.updater.Status())
		return
	}
	var err error
	switch strings.TrimPrefix(r.URL.Path, "/admin/update/") {
	case "check":
		err = s.updater.Check(r.Context())
	case "download":
		// Continue a bounded download if the browser request times out/disconnects.
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			_ = s.updater.Download(ctx)
		}()
		writeJSON(w, 202, map[string]any{"ok": true})
		return
	case "install":
		body, e := readJSON(r)
		if e != nil {
			err = e
			break
		}
		allow, _ := body["allow_migration"].(bool)
		digest, _ := body["expected_digest"].(string)
		if !selfupdate.ValidHash(digest) {
			writeAPIErr(w, errBody(400, "缺少待安装产物指纹，请刷新状态", "invalid_request_error"))
			return
		}
		err = s.updater.Install(allow, digest)
	case "policy":
		body, e := readJSON(r)
		if e != nil {
			err = e
			break
		}
		mode, _ := body["mode"].(string)
		interval, _ := body["every_hours"].(float64)
		start, _ := body["start_hour"].(float64)
		end, _ := body["end_hour"].(float64)
		republished, _ := body["allow_republished"].(bool)
		if interval != float64(int(interval)) || start != float64(int(start)) || end != float64(int(end)) {
			writeAPIErr(w, errBody(400, "更新时段与间隔需为整数", "invalid_request_error"))
			return
		}
		err = s.updater.SetPolicy(selfupdate.Policy{Mode: mode, EveryHours: int(interval), StartHour: int(start), EndHour: int(end), AllowRepublished: republished})
	default:
		writeAPIErr(w, errBody(404, "更新操作不存在", "not_found"))
		return
	}
	if err != nil {
		writeAPIErr(w, errBody(409, err.Error(), "update_failed"))
		return
	}
	writeJSON(w, 200, s.updater.Status())
}
func (o *Orchestrator) UpdateBackup(ctx context.Context) (string, error) {
	if err := o.runBackup(ctx); err != nil {
		return "", err
	}
	o.backup.mu.Lock()
	defer o.backup.mu.Unlock()
	return o.backup.lastPath, nil
}
func (o *Orchestrator) UpdateReady(ctx context.Context) error {
	if err := o.db.QuickCheck(ctx); err != nil {
		return err
	}
	return o.db.CheckEncryptedApps(func(enc, hash string) bool { return o.crypto.DecryptVerified(enc, hash) != "" })
}
