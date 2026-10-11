package app

import (
	"math"
	"sort"
	"strings"

	"work2api/internal/core/provider"
	"work2api/internal/store"
)

type budgetEvidence struct {
	credits, tokens float64
	calls           int
	legacyCalls     int
}

func budgetModel(providerName, model string) (string, bool) {
	if name, id, ok := strings.Cut(model, "/"); ok {
		return id, name == providerName
	}
	return model, providerName == "workbuddy"
}

// Prediction sums finite token budgets, never credits from different billing
// domains. Each account balance is used once. Free access is an independent
// capability, not infinite capacity or an amplification of the paid estimate.
func (s *Server) creditPrediction(data map[string]provider.AdminData, now float64) map[string]any {
	samples, err := s.o.db.UsageBudgetSamples(now)
	if err != nil {
		return map[string]any{"predicted_tokens": nil, "providers": []any{}, "error": "用量统计暂不可用"}
	}
	return estimateBudgets(data, samples)
}

func estimateBudgets(data map[string]provider.AdminData, samples []store.BudgetSample) map[string]any {
	providers := []map[string]any{}
	names := make([]string, 0, len(data))
	for name := range data {
		names = append(names, name)
	}
	sort.Strings(names)
	var total float64
	estimated, poolCount := 0, 0
	for _, name := range names {
		d := data[name]
		pools := []map[string]any{}
		freeSet := map[string]bool{}
		seen := map[string]bool{}
		var subtotal float64
		covered := 0
		for _, pool := range d.Budgets {
			if pool.AccountID == "" || seen[pool.AccountID] {
				continue
			}
			seen[pool.AccountID] = true
			poolCount++
			allowed, free := map[string]bool{}, map[string]bool{}
			for _, model := range pool.Models {
				id, ok := budgetModel(name, model)
				if ok {
					allowed[id] = true
				}
			}
			for _, model := range pool.FreeModels {
				id, ok := budgetModel(name, model)
				if ok {
					free[id] = true
					if pool.Reason == "" && !pool.Deferred {
						freeSet[id] = true
					}
				}
			}
			peers := map[string]bool{}
			for _, other := range d.Budgets {
				if other.Domain == pool.Domain && other.Unit == pool.Unit && other.Reason == "" {
					peers[other.AccountID] = true
				}
			}
			evidence := func(recent, own bool) budgetEvidence {
				e := budgetEvidence{}
				for _, sample := range samples {
					if recent && !sample.Recent {
						continue
					}
					if own {
						if sample.AccountUID != pool.AccountID {
							continue
						}
					} else if !peers[sample.AccountUID] {
						continue
					}
					id, ok := budgetModel(name, sample.Model)
					if !ok || !allowed[id] || free[id] || sample.Credits <= 0 || sample.Tokens <= 0 || math.IsNaN(sample.Credits) || math.IsInf(sample.Credits, 0) {
						continue
					}
					e.credits += sample.Credits
					e.tokens += sample.Tokens
					e.calls += sample.Calls
					e.legacyCalls += sample.LegacyCalls
				}
				return e
			}
			row := map[string]any{"label": pool.Label, "remaining": pool.Remaining, "unit": pool.Unit, "stale": pool.Stale, "deferred": pool.Deferred, "excluded_credits": pool.Excluded, "predicted_tokens": nil, "status": "unknown", "reason": pool.Reason, "window_days": 7, "samples": 0, "scope": "account"}
			var observedFree, reportedZero float64
			unknown := 0
			for _, sample := range samples {
				id, ok := budgetModel(name, sample.Model)
				if ok && allowed[id] && sample.Recent && sample.AccountUID == pool.AccountID {
					if free[id] {
						observedFree += sample.ZeroTokens
					} else {
						reportedZero += sample.ZeroTokens
					}
					unknown += sample.UnknownCalls
				}
			}
			row["free_tokens_observed"] = observedFree
			row["zero_cost_unconfirmed_tokens"] = reportedZero
			row["unknown_cost_calls"] = unknown
			e := evidence(true, true)
			good := func(e budgetEvidence) bool {
				return e.calls >= 5 && e.tokens >= 1000 && e.credits > 0 && !math.IsInf(e.credits, 0) && !math.IsInf(e.tokens, 0)
			}
			if !good(e) {
				e = evidence(false, true)
				row["window_days"] = 30
			}
			if !good(e) {
				e = evidence(true, false)
				row["window_days"] = 7
				row["scope"] = "billing_domain"
			}
			if !good(e) {
				e = evidence(false, false)
				row["window_days"] = 30
			}
			row["samples"] = e.calls
			row["legacy_samples"] = e.legacyCalls
			switch {
			case pool.Reason != "":
				row["status"] = "unavailable"
			case pool.Remaining == nil:
				row["reason"] = "余额未知"
			case *pool.Remaining == 0:
				row["status"] = "empty"
				row["reason"] = "没有可估算的付费余额"
				row["predicted_tokens"] = int64(0)
			case !good(e):
				row["reason"] = "有效付费样本不足（至少5次、1000 Tokens）"
			default:
				// Σ(token_share × credits_per_token) = total_credits / total_tokens.
				value := *pool.Remaining * (e.tokens / e.credits)
				if !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value < float64(math.MaxInt64) && total+value < float64(math.MaxInt64) {
					row["predicted_tokens"] = int64(value)
					row["status"] = "estimated"
					row["tokens_per_unit"] = e.tokens / e.credits
					subtotal += value
					total += value
					covered++
					estimated++
				} else {
					row["reason"] = "超出可估算范围"
				}
			}
			pools = append(pools, row)
		}
		freeModels := []string{}
		for id := range freeSet {
			freeModels = append(freeModels, id)
		}
		sort.Strings(freeModels)
		row := map[string]any{"provider": name, "label": d.DisplayName, "predicted_tokens": nil, "pools": pools, "free_models": freeModels, "estimated_pools": covered}
		if covered > 0 {
			row["predicted_tokens"] = int64(subtotal)
		}
		if len(pools) == 0 {
			row["reason"] = "余额或计费依据未知"
		}
		providers = append(providers, row)
	}
	out := map[string]any{"predicted_tokens": nil, "providers": providers, "estimated_pools": estimated, "total_pools": poolCount}
	if estimated > 0 {
		out["predicted_tokens"] = int64(total)
	}
	return out
}
