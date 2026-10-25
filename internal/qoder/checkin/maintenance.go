package checkin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"sync"
	"work2api/internal/core/provider"
	"work2api/internal/qoder/account"
)

var maintenance struct {
	sync.Mutex
	path  string
	guard *provider.MaintenanceGuard
}

func maintenanceGuard() *provider.MaintenanceGuard {
	path := filepath.Join(account.DataRoot(), ".checkin-maintenance.json")
	maintenance.Lock()
	defer maintenance.Unlock()
	if maintenance.guard == nil || maintenance.path != path {
		maintenance.path = path
		maintenance.guard = provider.NewMaintenanceGuard(path)
	}
	return maintenance.guard
}
func campaignsCheckin(ctx context.Context, token string, res *CheckinResult) CheckinResult {
	sum := sha256.Sum256([]byte(token))
	identity := hex.EncodeToString(sum[:])
	key := res.AccountID
	g := maintenanceGuard()
	g.State(key, identity)
	observe := func(status int) {
		if ctx.Err() != nil {
			return
		}
		kind := ""
		hard := status == 401
		if hard {
			kind = "authorization_expired"
		} else if status != 200 {
			kind = "temporary"
		}
		g.Observe(key, identity, kind, hard)
	}
	_, err := g.Recover(ctx, key, identity, func(ctx context.Context) error {
		status, body, _ := doCheckinRequest(ctx, "GET", "/sash/api/v1/me/campaigns", token, nil)
		if _, ok := body.(map[string]interface{}); status == 200 && !ok {
			status = 502
		}
		observe(status)
		if status != 200 {
			return provider.ErrMaintenancePaused
		}
		return nil
	})
	if err != nil {
		res.Status = "paused"
		res.Message = "签到已暂停，等待授权恢复验证"
		if errors.Is(err, provider.ErrMaintenanceLogin) || g.State(key, identity).NeedsLogin {
			res.Status = "reauthorize"
			res.Message = "签到已暂停，请重新授权"
		}
		return *res
	}
	result := campaignsCheckinRaw(ctx, token, res)
	status := result.httpStatus
	if status == 200 && result.Status == checkinStatusError {
		status = 502
	}
	observe(status)
	if status == 401 {
		result.Status = "reauthorize"
		result.Message = "签到已暂停，请重新授权"
	}
	return result
}

// PendingMaintenance exposes only display-safe state, never the credential hash.
func PendingMaintenance(id, token string) []map[string]any {
	if token == "" {
		return nil
	}
	sum := sha256.Sum256([]byte(token))
	state := maintenanceGuard().State(id, hex.EncodeToString(sum[:]))
	if state.Kind == "" {
		return nil
	}
	return []map[string]any{state.Public("签到")}
}
