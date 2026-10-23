package qoder

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"work2api/internal/qoder/account"
	"work2api/internal/qoder/bridge"
	"work2api/internal/qoder/cosy"
	"work2api/internal/qoder/logger"
	"work2api/internal/statebackup"
)

var exchangeRenewal = cosy.ExchangeJobTokenContext

func (r *Runtime) RenewAccount(ctx context.Context, id string) error {
	if !r.maintenanceMu.TryLock() {
		return errors.New("账号维护正在进行，请稍后重试")
	}
	defer r.maintenanceMu.Unlock()
	accounts, e := account.List()
	if e != nil {
		return errors.New("读取账号失败")
	}
	for _, a := range accounts {
		if a.ID == id {
			if a.AuthMode != "oauth" || account.IsGatewayHidden(id) {
				return errors.New("该账号不支持续期")
			}
			if e = r.renewToken(ctx, &a); e != nil {
				return errors.New("续期未完成，请核实账号授权状态后重试")
			}
			return nil
		}
	}
	return errors.New("账号不存在")
}

func (r *Runtime) renewTokens(ctx context.Context) {
	accounts, e := account.List()
	if e != nil {
		return
	}
	for _, a := range accounts {
		if ctx.Err() != nil {
			return
		}
		if a.AuthMode != "oauth" || account.IsGatewayHidden(a.ID) {
			continue
		}
		age, e := account.SecretAge(a.ID)
		if e != nil || age < 12*24*time.Hour {
			continue
		}
		if e = r.renewToken(ctx, &a); e != nil {
			logger.Error("Qoder授权续期未完成，请检查账号状态")
		}
	}
}

func (r *Runtime) renewToken(ctx context.Context, a *account.Account) error {
	leave := statebackup.Enter()
	defer leave()
	old, e := account.GetSecret(a.ID)
	if e != nil {
		return e
	}
	dt, rt := bridge.ParseOAuthSecret(old)
	if !strings.HasPrefix(dt, "dt-") || rt != "" && !strings.HasPrefix(rt, "drt-") {
		return errors.New("OAuth凭据格式无效")
	}
	credential := rt
	if credential == "" {
		credential = dt
	}
	seed := cosy.FingerprintSeed("", credential)
	mid, mt, typ := cosy.DeriveMachineID(seed), cosy.DeriveMachineToken(seed), cosy.DeriveMachineType(seed)
	if fp := a.CheckinFingerprint; fp != nil {
		mid, mt, typ = fp.MachineID, fp.MachineToken, fp.MachineType
	}
	result, e := exchangeRenewal(ctx, credential, mid, mt, typ, account.GetEndpoints(a.Region).JobTokenURL)
	if e != nil && credential == rt && dt != "" && strings.HasPrefix(e.Error(), "HTTP 401 at jobToken") && ctx.Err() == nil {
		seed = cosy.FingerprintSeed("", dt)
		if a.CheckinFingerprint == nil {
			mid, mt, typ = cosy.DeriveMachineID(seed), cosy.DeriveMachineToken(seed), cosy.DeriveMachineType(seed)
		}
		result, e = exchangeRenewal(ctx, dt, mid, mt, typ, account.GetEndpoints(a.Region).JobTokenURL)
	}
	if e != nil {
		return e
	}
	if nested, ok := result["data"].(map[string]any); ok {
		result = nested
	}
	get := func(keys ...string) string {
		for _, k := range keys {
			if s, ok := result[k].(string); ok && s != "" {
				return s
			}
		}
		return ""
	}
	nextDT := get("device_token", "deviceToken", "access_token", "accessToken", "token", "securityOauthToken")
	nextRT := get("refresh_token", "refreshToken")
	if nextDT == "" {
		nextDT = dt
	}
	if nextRT == "" {
		nextRT = rt
	}
	if !strings.HasPrefix(nextDT, "dt-") || nextRT != "" && !strings.HasPrefix(nextRT, "drt-") || (nextDT == dt && nextRT == rt) {
		return errors.New("上游未返回可保存的新授权")
	}
	raw, _ := json.Marshal(map[string]string{"device_token": nextDT, "refresh_token": nextRT})
	if e = account.RotateSecret(a.ID, old, string(raw)); e != nil {
		return e
	}
	r.mu.Lock()
	delete(r.bridges, a.ID)
	r.bridgeGeneration[a.ID]++
	r.modelCacheStale = true
	r.mu.Unlock()
	r.quotaMu.Lock()
	delete(r.quotaCache, a.ID)
	r.quotaMu.Unlock()
	return nil
}
