package trae

import (
	"context"
	"strings"
	"time"
	"work2api/internal/core/provider"
)

func (r *Runtime) CheckAccountIDs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	ids := []string{}
	for id := range r.accounts {
		ids = append(ids, id)
	}
	return ids
}
func (r *Runtime) CheckAccountIdentity(id string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	a := r.accounts[id]
	if a == nil {
		return ""
	}
	parts := []string{}
	for _, p := range []string{"cn", "solo"} {
		if identity := a.Identities[p]; identity != nil {
			fp, ready := identity.TryMaintenanceIdentity()
			if !ready {
				return ""
			}
			parts = append(parts, fp)
		}
	}
	return strings.Join(parts, ":")
}
func (r *Runtime) CheckAccount(ctx context.Context, id string, force bool) provider.AccountCheck {
	r.authGuard()
	r.mu.Lock()
	a := r.accounts[id]
	fresh := a != nil
	invalid, all := 0, 0
	oldest := time.Now()
	if a != nil {
		for product, identity := range a.Identities {
			all++
			if at := a.CatalogTimes[product]; at.Before(oldest) {
				oldest = at
			}
			if !r.identityAvailableLocked(identity) {
				invalid++
			}
			fresh = fresh && !a.CatalogErrors[product] && time.Since(a.CatalogTimes[product]) < provider.AccountCheckMaxAge(ctx)
		}
	}
	r.mu.Unlock()
	if a == nil {
		return provider.Checked("unknown", "账号", "账号已移除")
	}
	if invalid == all {
		return provider.Checked("invalid", "登录授权", "所有产品身份均已失效，请重新授权")
	}
	var err error
	if force || !fresh {
		err = r.RefreshAccount(ctx, id)
	}
	r.mu.Lock()
	invalid = 0
	for _, identity := range a.Identities {
		if !r.identityAvailableLocked(identity) {
			invalid++
		}
	}
	r.mu.Unlock()
	if invalid == all {
		return provider.Checked("invalid", "登录授权", "所有产品身份均已失效，请重新授权")
	}
	if err != nil || invalid > 0 {
		return provider.Checked("warning", "产品目录与授权", "部分身份或目录检查异常，其他有效身份仍可调用")
	}
	result := provider.Checked("valid", "产品目录与授权", "产品目录验证通过；未测试模型调用")
	if !force && fresh {
		result.CheckedAt = oldest.Unix()
	}
	return result
}
