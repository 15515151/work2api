package trae

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
	"work2api/internal/trae/upstream"
)

func displayPricing(a *accountState, m upstream.ModelInfo) upstream.ModelInfo {
	m.Pricing = append([]upstream.ModelPrice(nil), m.Pricing...)
	if len(m.Pricing) == 0 {
		product := ""
		if a.Auth != nil {
			product = a.Auth.Variant
		}
		m.Pricing = []upstream.ModelPrice{{Product: product, Stale: true}}
	}
	for i := range m.Pricing {
		q := &m.Pricing[i]
		q.Stale = a.CatalogErrors[q.Product] || time.Since(a.CatalogTimes[q.Product]) > 30*time.Minute
	}
	return m
}

func pricingMetadata(quotes []upstream.ModelPrice) map[string]any {
	prices := map[string]upstream.ModelPrice{}
	groups := map[string]upstream.ModelPromotion{}
	counts := map[string]int{}
	matched := map[string]int{}
	stale := false
	for _, q := range quotes {
		counts[q.Product]++
		stale = stale || q.Stale
		encoded, _ := json.Marshal(q)
		prices[string(encoded)] = q
		if q.Stale {
			continue
		}
		seen := map[string]bool{}
		for _, p := range q.Promotions {
			p.Region = q.Product
			p.Partial = false
			encoded, _ := json.Marshal(p)
			key := string(encoded)
			groups[key] = p
			if !seen[key] {
				matched[key]++
				seen[key] = true
			}
		}
	}
	keys := make([]string, 0, len(prices))
	for key := range prices {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	values := make([]upstream.ModelPrice, 0, len(keys))
	for _, key := range keys {
		values = append(values, prices[key])
	}
	keys = keys[:0]
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	promotions := []upstream.ModelPromotion{}
	for _, key := range keys {
		p := groups[key]
		p.Partial = matched[key] < counts[p.Region]
		promotions = append(promotions, p)
	}
	return map[string]any{"price_factors": values, "promotions": promotions, "pricing_stale": stale}
}

func (r *Runtime) ModelMetadataForAccounts(model string, allowed map[string]bool) map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	quotes := []upstream.ModelPrice{}
	for id, a := range r.accounts {
		if !allowed[id] || !a.Enabled {
			continue
		}
		for _, m := range a.Models {
			if strings.EqualFold("trae/"+m.ID, model) {
				quotes = append(quotes, displayPricing(a, m).Pricing...)
			}
		}
	}
	return pricingMetadata(quotes)
}
