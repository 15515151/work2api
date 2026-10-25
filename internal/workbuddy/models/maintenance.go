package models

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"work2api/internal/core/provider"
	"work2api/internal/workbuddy/pool"
)

type catalogResponseError struct {
	status int
	code   int
}

func (e *catalogResponseError) Error() string {
	return fmt.Sprintf("模型目录响应异常（HTTP %d，code=%d）", e.status, e.code)
}

type credentialIdentity interface{ MaintenanceIdentity() (string, error) }
type credentialAuthorization interface{ AuthorizationInvalid() bool }
type contextCatalogClient interface {
	CatalogHeadersContext(context.Context) (map[string]string, error)
}

// SetMaintenanceGuard is configured before the registry becomes available.
func (r *Registry) SetMaintenanceGuard(g *provider.MaintenanceGuard) { r.maintenance = g }
func catalogIdentity(a *pool.Account) string {
	if m, ok := a.Mgr.(credentialIdentity); ok {
		identity, _ := m.MaintenanceIdentity()
		return identity
	}
	return a.UID
}
func catalogPrefix(a *pool.Account) string { return "catalog:" + a.UID + ":" + a.Profile + ":" }
func (r *Registry) CatalogMaintenance(a *pool.Account) []map[string]any {
	if a == nil || r.maintenance == nil {
		return nil
	}
	pending := r.maintenance.Pending(catalogPrefix(a), catalogIdentity(a))
	keys := make([]string, 0, len(pending))
	for key := range pending {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := []map[string]any{}
	for _, key := range keys {
		scope := "模型目录"
		if strings.Contains(key, ":desktop:") {
			scope = "活动信息"
		} else if strings.HasSuffix(key, "/v3/config") {
			scope = "模型目录 · 主来源"
		} else {
			scope = "模型目录 · 补充来源"
		}
		out = append(out, pending[key].Public(scope))
	}
	return out
}
func (r *Registry) fetchMaintainedCatalog(ctx context.Context, url string, headers map[string]string, a *pool.Account, scope string) map[string]any {
	if m, ok := a.Mgr.(credentialAuthorization); ok && m.AuthorizationInvalid() {
		return nil
	}
	if r.maintenance == nil {
		data, _ := r.fetchCatalogDocumentResult(ctx, url, headers)
		return data
	}
	identity := catalogIdentity(a)
	key := catalogPrefix(a) + scope + ":" + url
	var data map[string]any
	fetch := func(ctx context.Context) error {
		var err error
		data, err = r.fetchCatalogDocumentResult(ctx, url, headers)
		var response *catalogResponseError
		if errors.As(err, &response) {
			log.Printf("模型目录请求失败 account=%s scope=%s status=%d code=%d；保留可用快照并按来源恢复", a.UID, scope, response.status, response.code)
		}
		if catalogIdentity(a) != identity {
			data = nil
			return provider.ErrMaintenancePaused
		}
		if !errors.Is(err, context.Canceled) {
			previous := r.maintenance.State(key, identity)
			kind := ""
			if err != nil {
				kind = "temporary"
				var response *catalogResponseError
				if errors.As(err, &response) && (response.status == 401 || response.code == 12153) {
					kind = "authorization_unverified"
				}
			}
			// Ordinary directory failures keep the established catalog retry policy.
			// Only authorization failures (and their recovery) use this extra gate.
			if kind == "" || kind == "authorization_unverified" || previous.Kind != "" {
				r.maintenance.Observe(key, identity, kind, false)
			}
		}
		return err
	}
	ran, err := r.maintenance.Recover(ctx, key, identity, fetch)
	if err != nil {
		return nil
	}
	// Healthy requests keep the registry's normal refresh behavior.
	if !ran && r.maintenance.State(key, identity).Kind == "" {
		_ = fetch(ctx)
	}
	return data
}
