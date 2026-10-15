package models

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"work2api/internal/workbuddy/pool"
	"work2api/internal/workbuddy/profiles"
	"work2api/internal/workbuddy/siterouting"
)

// Promotion is display-only upstream metadata. It never participates in model
// authorization, tariff calculation, or routing. Account identities stay private.
type Promotion struct {
	Label       string            `json:"label"`
	Description string            `json:"description,omitempty"`
	Region      string            `json:"region"`
	Partial     bool              `json:"partial,omitempty"`
	ValidFrom   string            `json:"valid_from,omitempty"`
	ValidUntil  string            `json:"valid_until,omitempty"`
	Timezone    string            `json:"timezone,omitempty"`
	Daily       []PromotionWindow `json:"daily,omitempty"`
	TrialDays   int               `json:"trial_days,omitempty"`
	priority    int
}

type PromotionWindow struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

type promotionSnapshot struct {
	models  map[string][]Promotion
	expires time.Time
}

// CLI catalogs omit full promotion rules. Fetch only display metadata using the
// existing desktop identity, at the SAME allowlisted endpoint. Never import its
// models, whitelist, credits, or credentials into the inference catalog.
func (r *Registry) refreshPromotionMetadata(ctx context.Context, account *pool.Account, url string, headers map[string]string) {
	if !strings.EqualFold(headers["X-IDE-Type"], "CLI") {
		return
	}
	region, err := siterouting.ProfileRegion(account.Profile)
	if err != nil {
		return
	}
	desktop := make(map[string]string, len(headers))
	for k, v := range headers {
		if !strings.EqualFold(k, "x-client-platform") {
			desktop[k] = v
		}
	}
	for k, v := range profiles.IdentityHeaders(region + "-work") {
		desktop[k] = v
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if data := r.fetchCatalogDocument(ctx, url, desktop); data != nil {
		r.savePromotionMetadata(account, data)
	}
}

func (r *Registry) savePromotionMetadata(account *pool.Account, data map[string]any) {
	snapshot := promotionSnapshot{models: parsePromotions(data), expires: time.Now().Add(time.Duration(r.ttl()) * time.Second)}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.promotions == nil {
		r.promotions = map[[2]string]promotionSnapshot{}
	}
	r.promotions[[2]string{account.Profile, account.UID}] = snapshot
}

func parsePromotions(data map[string]any) map[string][]Promotion {
	out := map[string][]Promotion{}
	features, _ := data["productFeaturesConfig"].(map[string]any)
	banners, _ := features["ModelTrialBanner"].(map[string]any)
	trials := map[string]int{}
	if list, ok := banners["banners"].([]any); ok {
		for _, raw := range list {
			b, _ := raw.(map[string]any)
			if id, days := str(b["modelId"]), intOf(b["trialDays"]); id != "" && days > 0 {
				trials[id] = days
			}
		}
	}
	list, _ := data["modelPromotions"].([]any)
	for _, raw := range list {
		v, _ := raw.(map[string]any)
		if !boolOf(v["enabled"]) {
			continue
		}
		badge, _ := v["badge"].(map[string]any)
		hover, _ := v["hover"].(map[string]any)
		schedule, _ := v["schedule"].(map[string]any)
		p := Promotion{Label: str(badge["label"]), Description: str(hover["textZh"]),
			ValidFrom: str(schedule["validFrom"]), ValidUntil: str(schedule["validUntil"]),
			Timezone: str(schedule["timezone"]), priority: intOf(v["priority"])}
		if p.Label == "" {
			continue
		}
		windows, _ := schedule["daily"].([]any)
		for _, raw := range windows {
			w, _ := raw.(map[string]any)
			if start, end := str(w["start"]), str(w["end"]); start != "" && end != "" {
				p.Daily = append(p.Daily, PromotionWindow{Start: start, End: end})
			}
		}
		ids, _ := v["modelIds"].([]any)
		for _, rawID := range ids {
			id := str(rawID)
			if id == "" {
				continue
			}
			item := p
			item.TrialDays = trials[id]
			// This window is trial enrollment, not the account's expiry. The
			// catalog still has to explicitly allow this model for the account.
			out[id] = append(out[id], item)
		}
	}
	for id, days := range trials {
		if len(out[id]) == 0 {
			out[id] = []Promotion{{Label: "限时试用", TrialDays: days}}
		}
	}
	return out
}

// Promotions uses only enabled accounts that the current model catalog confirms
// support this model. Expired snapshots and disabled accounts cannot advertise
// an offer. A trial's enrollment deadline is never its personal expiry date.
func (r *Registry) Promotions(model string, supporters []map[string]any) []Promotion {
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	counts := map[string]int{}
	groups := map[string]Promotion{}
	seen := map[string]map[string]bool{}
	for _, a := range supporters {
		if a["enabled"] != true {
			continue
		}
		profile, uid := str(a["profile"]), str(a["uid"])
		region := siterouting.ProfileSite(profile)
		counts[region]++
		snapshot := r.promotions[[2]string{profile, uid}]
		if !now.Before(snapshot.expires) {
			continue
		}
		for _, p := range snapshot.models[model] {
			if p.TrialDays == 0 && !promotionInDate(p, now) {
				continue
			}
			p.Region = region
			// Daytime reminder and nighttime price records can have identical
			// prose. Display that rule once rather than duplicate the same badge.
			keyRecord := p
			if p.Description != "" {
				keyRecord.Daily = nil
			}
			keyBytes, _ := json.Marshal(keyRecord)
			key := string(keyBytes)
			if prev, ok := groups[key]; ok && prev.priority > p.priority {
				p.priority = prev.priority
			}
			groups[key] = p
			if seen[key] == nil {
				seen[key] = map[string]bool{}
			}
			seen[key][uid] = true
		}
	}
	out := make([]Promotion, 0, len(groups))
	for key, p := range groups {
		p.Partial = len(seen[key]) < counts[p.Region]
		p.Daily = append([]PromotionWindow(nil), p.Daily...)
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].priority != out[j].priority {
			return out[i].priority > out[j].priority
		}
		a, _ := json.Marshal(out[i])
		b, _ := json.Marshal(out[j])
		return string(a) < string(b)
	})
	return out
}

func promotionInDate(p Promotion, now time.Time) bool {
	for _, bound := range []struct {
		value string
		start bool
	}{{p.ValidFrom, true}, {p.ValidUntil, false}} {
		if bound.value == "" {
			continue
		}
		at, err := time.Parse(time.RFC3339, bound.value)
		if err != nil || bound.start && now.Before(at) || !bound.start && !now.Before(at) {
			return false
		}
	}
	return true
}
