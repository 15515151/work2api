package provider

import (
	"math"
	"math/rand"
)

// Candidate carries comparable, channel-owned facts. A caller must filter
// ownership, model capability and cooldown before invoking SelectCandidate.
// Costs are comparable only within the same billing domain.
type Candidate struct {
	ID                 string
	Domestic           bool
	International      bool
	Cost               *float64
	Remaining, Total   *float64
	ExpiryDays         float64
	ExpiringAmount     float64
	Priority, Failures int
	LastUsed           float64
}

// SelectCandidate preserves WorkBuddy's established ordering: cheapest known
// cost, domestic paid candidates, expiring balance, then weighted rotation.
// Sticky binding belongs to the request/session owner, outside this function.
func SelectCandidate(candidates []Candidate, costAware bool, windowDays, now float64) string {
	if len(candidates) == 0 {
		return ""
	}
	if windowDays <= 0 {
		windowDays = 7
	}
	group := append([]Candidate(nil), candidates...)
	validCost := func(c Candidate) bool {
		return c.Cost != nil && *c.Cost >= 0 && !math.IsNaN(*c.Cost) && !math.IsInf(*c.Cost, 0)
	}
	if costAware {
		minimum := math.Inf(1)
		for _, c := range group {
			if validCost(c) && *c.Cost < minimum {
				minimum = *c.Cost
			}
		}
		if !math.IsInf(minimum, 1) {
			filtered := make([]Candidate, 0, len(group))
			for _, c := range group {
				if validCost(c) && *c.Cost <= minimum+1e-9 {
					filtered = append(filtered, c)
				}
			}
			group = filtered
		}
		domestic := make([]Candidate, 0, len(group))
		for _, c := range group {
			if validCost(c) && *c.Cost > 0 && c.Domestic {
				domestic = append(domestic, c)
			}
		}
		if len(domestic) > 0 {
			group = domestic
		}
	}
	urgent := make([]Candidate, 0, len(group))
	for _, c := range group {
		if c.ExpiryDays <= windowDays {
			urgent = append(urgent, c)
		}
	}
	applyIdle := len(urgent) == 0
	if !applyIdle {
		group = urgent
	}
	maxExpiring := 0.0
	if !applyIdle {
		for _, c := range group {
			if c.ExpiringAmount > maxExpiring {
				maxExpiring = c.ExpiringAmount
			}
		}
	}
	domestic, foreign, sameCost := false, false, costAware
	var reference float64
	for i, c := range group {
		if !validCost(c) {
			sameCost = false
		} else if i == 0 {
			reference = *c.Cost
		} else if math.Abs(*c.Cost-reference) > 1e-9 {
			sameCost = false
		}
		if c.Domestic {
			domestic = true
		} else if c.International {
			foreign = true
		}
	}
	weights := make([]float64, len(group))
	total := 0.0
	for i, c := range group {
		balance := 1.0
		if c.Total != nil && *c.Total != 0 {
			remain := 0.0
			if c.Remaining != nil && *c.Remaining > 0 {
				remain = *c.Remaining
			}
			balance = 0.5 + 0.5*remain / *c.Total
		}
		expiry := 1.0
		switch {
		case c.ExpiryDays <= 1:
			expiry = 8
		case c.ExpiryDays <= 3:
			expiry = 6
		case c.ExpiryDays <= 7:
			expiry = 4
		case c.ExpiryDays <= 30:
			expiry = 2
		}
		idle := 1.0
		if applyIdle {
			hours := 0.0
			if c.LastUsed > 0 {
				hours = (now - c.LastUsed) / 3600
			}
			idle = math.Min(1+hours*0.3, 3)
		}
		w := float64(1+max(c.Priority, 0)) * balance * expiry / (1 + float64(c.Failures)) * idle
		if sameCost && domestic && foreign && c.Domestic {
			w *= 1.25
		}
		if maxExpiring > 0 {
			w *= 1 + 2*c.ExpiringAmount/maxExpiring
		}
		if w < 0 || math.IsNaN(w) || math.IsInf(w, 0) {
			w = 1
		}
		weights[i] = w
		total += w
	}
	pick := rand.Float64() * total
	for i, c := range group {
		pick -= weights[i]
		if pick <= 0 {
			return c.ID
		}
	}
	return group[len(group)-1].ID
}
