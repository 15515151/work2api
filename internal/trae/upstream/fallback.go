// Adapted from mmqz/cpa-multi-plugins, MIT; see NOTICE.md.
package upstream

import "strings"

var cnAlternateOrigins = []string{
	"https://api.trae.com.cn",
}

func exchangeHosts(primary, oauth string) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(h string) {
		h = strings.TrimRight(strings.TrimSpace(h), "/")
		if h == "" {
			return
		}
		if !strings.HasPrefix(h, "http://") && !strings.HasPrefix(h, "https://") {
			h = "https://" + h
		}
		if _, ok := seen[h]; ok {
			return
		}
		seen[h] = struct{}{}
		out = append(out, h)
	}
	add(primary)
	add(oauth)
	for _, h := range cnAlternateOrigins {
		add(h)
	}
	return out
}
