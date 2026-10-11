package trae

import (
	"context"
	"errors"
	"time"

	"work2api/internal/core/provider"
	"work2api/internal/trae/upstream"
)

func (r *Runtime) AdminRefreshCredits(ctx context.Context) (map[string]any, error) {
	r.mu.Lock()
	ids := []string{}
	for id := range r.accounts {
		ids = append(ids, id)
	}
	r.mu.Unlock()
	results := []map[string]any{}
	for _, id := range ids {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		result := map[string]any{"uid": id, "status": "error"}
		if r.refreshQuota(ctx, id, true) == nil {
			result["status"] = "ok"
		}
		results = append(results, result)
	}
	return map[string]any{"results": results}, nil
}

// One in-flight quota read per resource, even when several Agents discover an
// expired zero balance together. Callers can cancel without detached probes.
func (r *Runtime) refreshQuota(ctx context.Context, id string, force bool) error {
	r.mu.Lock()
	a := r.accounts[id]
	if a == nil {
		r.mu.Unlock()
		return errors.New("账号不存在")
	}
	if done := a.QuotaProbeDone; done != nil {
		r.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-done:
		}
		r.mu.Lock()
		failed := r.accounts[id] != a || a.QuotaFailed
		r.mu.Unlock()
		if failed {
			return errors.New("Trae 额度刷新失败")
		}
		return nil
	}
	if !force && time.Since(a.QuotaProbeAt) < time.Minute {
		r.mu.Unlock()
		return nil
	}
	done := make(chan struct{})
	a.QuotaProbeDone, a.QuotaProbeAt = done, time.Now()
	identity := a.Auth
	r.mu.Unlock()
	err := r.ensure(ctx, identity)
	var usage *upstream.EntUsageResult
	if err == nil {
		usage, err = r.client(ctx).UserEntUsage(identity)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	defer close(done)
	a.QuotaProbeDone = nil
	if r.accounts[id] != a {
		return errors.New("Trae 账号已变更")
	}
	a.QuotaFailed = err != nil
	if err == nil {
		pool := upstream.CreditsPoolUsage(usage.UserEntitlementPackList, usage.IsCreditsBilling)
		remaining := 0.0
		total := 0.0
		known := false
		var expiry *float64
		packages := []map[string]any{}
		for _, pack := range usage.UserEntitlementPackList {
			if pack.Excluded() {
				continue
			}
			q := pack.EffectiveQuota()
			var creditRemain *float64
			if q.CreditsLimit != nil && *q.CreditsLimit >= 0 {
				used := 0.0 // Same absent-usage convention as the upstream credits pool.
				if pack.Usage.CreditsAmount != nil {
					used = *pack.Usage.CreditsAmount
				}
				left := max(0.0, float64(*q.CreditsLimit)-used)
				creditRemain = &left
				remaining += left
			}
			if q.CreditsLimit != nil {
				name := pack.DisplayDesc
				if name == "" {
					name = "积分资源包"
				}
				item := map[string]any{"name": name, "remain": nil, "used": pack.Usage.CreditsAmount, "total": nil, "expire_at": nil, "unlimited": *q.CreditsLimit == -1}
				if *q.CreditsLimit >= 0 {
					item["total"] = *q.CreditsLimit
					item["remain"] = creditRemain
				}
				end := pack.EntitlementBaseInfo.EndTime
				if end > 1e12 {
					end /= 1000
				}
				if end > 0 && end < 4102444800 {
					item["expire_at"] = end
				}
				packages = append(packages, item)
			}
			if q.CreditsLimit != nil && *q.CreditsLimit >= 0 {
				total += float64(*q.CreditsLimit)
				known = true
			}
			end := float64(pack.EntitlementBaseInfo.EndTime)
			if end > 1e12 {
				end /= 1000
			}
			if creditRemain != nil && *creditRemain > 0 && end > 0 && end < 4102444800 && (expiry == nil || end < *expiry) {
				v := end
				expiry = &v
			}
		}
		if pool.Known && !pool.Unlimited {
			// Preserve fractional credits so a usable balance is not rounded to zero.
			a.Remaining = &remaining
		} else {
			a.Remaining = nil
		}
		if known && !pool.Unlimited {
			a.Total = &total
		} else {
			a.Total = nil
		}
		a.Expiry = expiry
		a.QuotaAt = time.Now()
		a.CreditPackages = packages
		if a.CooldownKind == upstream.ErrPlanLimit && (pool.Unlimited || pool.Known && remaining > 0) {
			// This allows a bounded chat recheck, not a promise that positive
			// credits imply remaining model quota. A rejection re-freezes it.
			a.Cooldown, a.CooldownKind = time.Time{}, upstream.ErrNone
		}
	}
	if err != nil {
		return errors.New("Trae 额度刷新失败")
	}
	return nil
}

func (r *Runtime) refreshBlockedQuotas(ctx context.Context, req provider.ServeRequest, model string, image bool) {
	excluded := map[string]bool{}
	if req.Caller.UserID <= 0 {
		var err error
		excluded, err = r.db.PrivatePoolExcludedUIDs()
		if err != nil {
			return
		}
	}
	r.mu.Lock()
	ids := []string{}
	for id, a := range r.accounts {
		if !a.Enabled || req.Caller.UserID > 0 && !req.Caller.AccountScope[id] || req.Caller.UserID <= 0 && excluded[id] {
			continue
		}
		if identity, _ := a.modelIdentityFor(model, image); identity == nil {
			continue
		}
		// Healthy resources should serve immediately; recovery reads must not
		// delay a working pool because a different resource is frozen.
		if !time.Now().Before(a.Cooldown) && (a.Remaining == nil || *a.Remaining > 0) {
			r.mu.Unlock()
			return
		}
		zero := a.Remaining != nil && *a.Remaining == 0 && time.Since(a.QuotaAt) >= time.Minute
		frozen := a.CooldownKind == upstream.ErrPlanLimit && time.Since(a.QuotaProbeAt) >= 5*time.Minute
		if zero || frozen {
			ids = append(ids, id)
		}
	}
	r.mu.Unlock()
	// Cap the recovery round: startup/large pools must not add N network reads
	// before a request. Subsequent calls can probe the remaining stale resources.
	probeCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	for i, id := range ids {
		if i == 3 || probeCtx.Err() != nil {
			break
		}
		if provider.CheckDispatch(probeCtx, provider.AccountRef{Provider: "trae", LocalID: id}, "trae/"+model) == nil {
			_ = r.refreshQuota(probeCtx, id, false)
		}
	}
}
func (r *Runtime) AdminCheckin(ctx context.Context) (map[string]any, error) {
	r.mu.Lock()
	ids := []string{}
	for id := range r.accounts {
		ids = append(ids, id)
	}
	r.mu.Unlock()
	results := []map[string]any{}
	today := time.Now().In(time.FixedZone("Asia/Shanghai", 8*3600)).Format("2006-01-02")
	for _, id := range ids {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		r.mu.Lock()
		a := r.accounts[id]
		r.mu.Unlock()
		if a == nil {
			continue
		}
		result := map[string]any{"uid": id, "status": "error"}
		err := r.ensure(ctx, a.Auth)
		if err == nil {
			c := r.client(ctx)
			status, e := c.CheckinStatus(a.Auth)
			err = e
			if err == nil {
				if !status.Enable {
					result["status"] = "no_campaign"
				} else if status.CheckedIn {
					result["status"] = "already"
				} else if status.DidCheckedIn {
					result["status"] = "device_checked"
					result["message"] = "设备已参与，未确认该账号签到成功"
				} else {
					_, err = c.CheckinClaim(a.Auth)
					if err == nil {
						result["status"] = "ok"
					}
				}
			}
		}
		if err != nil {
			result["message"] = "上游未确认签到成功，请稍后重试"
			var rejection *upstream.Error
			if errors.As(err, &rejection) {
				result["code"] = rejection.BizCode
			}
		}
		if result["status"] == "ok" || result["status"] == "already" {
			if err := r.db.RecordCheckin(id, today, "trae"); err != nil {
				return nil, err
			}
			r.mu.Lock()
			a.CheckinDate = today
			r.mu.Unlock()
		}
		results = append(results, result)
	}
	return map[string]any{"results": results}, nil
}
func (r *Runtime) CheckinHistory(ctx context.Context, days int) (provider.CheckinCalendar, error) {
	if days < 1 || days > 366 {
		days = 30
	}
	now := time.Now().In(time.FixedZone("Asia/Shanghai", 8*3600))
	history, err := r.db.CheckinHistory(now.AddDate(0, 0, -days).Format("2006-01-02"))
	if err != nil {
		return provider.CheckinCalendar{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	filtered := map[string][]string{}
	for id, dates := range history {
		if r.accounts[id] != nil {
			filtered[id] = dates
		}
	}
	today := now.Format("2006-01-02")
	return provider.CheckinCalendar{History: filtered, Today: today, WindowDate: today, Timezone: "Asia/Shanghai"}, nil
}
