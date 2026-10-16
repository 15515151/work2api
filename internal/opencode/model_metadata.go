package opencode

import "math"

func publishedCost(raw map[string]any) *ModelCost {
	number := func(key string) *float64 {
		var value float64
		switch v := raw[key].(type) {
		case float64:
			value = v
		case *float64:
			if v == nil {
				return nil
			}
			value = *v
		default:
			return nil
		}
		if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			return nil
		}
		return &value
	}
	c := &ModelCost{number("input"), number("output"), number("cache_read"), number("cache_write")}
	if c.Input == nil && c.Output == nil && c.CacheRead == nil && c.CacheWrite == nil {
		return nil
	}
	return c
}

func cloneMetadata(md Metadata) Metadata {
	md.InputModalities = append([]string(nil), md.InputModalities...)
	md.OutputModalities = append([]string(nil), md.OutputModalities...)
	if md.Cost != nil {
		c := md.Cost
		md.Cost = publishedCost(map[string]any{"input": c.Input, "output": c.Output, "cache_read": c.CacheRead, "cache_write": c.CacheWrite})
	}
	options := make([]ReasoningOption, 0, len(md.ReasoningOptions))
	for _, option := range md.ReasoningOptions {
		option.Values = append([]string(nil), option.Values...)
		if option.Min != nil {
			v := *option.Min
			option.Min = &v
		}
		if option.Max != nil {
			v := *option.Max
			option.Max = &v
		}
		options = append(options, option)
	}
	md.ReasoningOptions = options
	return md
}

// Do not turn reasoning:true, toggle or budget controls into invented effort
// levels. Only the model's published effort values are discoverable as levels.
func (rt *Runtime) addDiscoveryMetadata(extra map[string]any, model string, md Metadata, tier Tier) {
	if md.Cost != nil {
		cost := map[string]float64{}
		for _, f := range []struct {
			key, alias string
			value      *float64
		}{
			{"input", "", md.Cost.Input}, {"output", "", md.Cost.Output},
			{"cache_read", "cacheRead", md.Cost.CacheRead}, {"cache_write", "cacheWrite", md.Cost.CacheWrite},
		} {
			if f.value != nil {
				cost[f.key] = *f.value
				if f.alias != "" {
					cost[f.alias] = *f.value
				}
			}
		}
		extra["cost"], extra["cost_tier"] = cost, string(tier)
	}
	if !md.Reasoning {
		return
	}
	if len(md.ReasoningOptions) > 0 {
		extra["reasoning_options"] = md.ReasoningOptions
	}
	efforts := []string{}
	seen := map[string]bool{}
	for _, option := range md.ReasoningOptions {
		if option.Type != "effort" {
			continue
		}
		for _, value := range option.Values {
			if value == "" || seen[value] {
				continue
			}
			seen[value] = true
			efforts = append(efforts, value)
		}
	}
	if forced := rt.cfg.ForcedEffort(model); forced != "" {
		// A configured override removes the client's ability to choose another
		// level. An unverified override must not become a capability claim.
		if seen[forced] {
			efforts = []string{forced}
			extra["default_reasoning_effort"] = forced
		} else {
			efforts = nil
		}
	}
	if len(efforts) > 0 {
		extra["reasoning_efforts"] = efforts
		extra["compat"] = map[string]any{"supportsReasoningEffort": true, "supportedReasoningEfforts": append([]string(nil), efforts...)}
	}
}
