// Adapted from mmqz/cpa-multi-plugins, MIT; see NOTICE.md.
package upstream

import "strings"

var (
	clientIDByVariant = map[string]string{
		"cn":        "ono9krqynydwx5", // non-solo (Trae Code CN = TRAE CN)
		"solo":      "en1oxy7wnw8j9n", // SOLO stable (Trae SOLO CN)
		"intl":      "ono9krqynydwx5", // TRAE intl — same non-solo client id (cockpit TRAE_AUTH_CLIENT_ID)
		"solo-intl": "en1oxy7wnw8j9n", // TRAE SOLO intl — same solo client id (cockpit TRAE_SOLO_AUTH_CLIENT_ID)
	}
	functionByVariant = map[string]string{
		"cn":        "solo_work_lite", // v0.12.79 (issue #9): llm_utils_chat's only live function
		"solo":      "solo_work_lite",
		"intl":      "solo_work_lite", // unreachable from chat (intlupstream chat_sessions); kept live
		"solo-intl": "solo_work_lite",
	}
)
var (
	platformIDByVariant = map[string]string{
		"cn":        "trae_cn",
		"solo":      "trae_solo_cn",
		"intl":      "trae",
		"solo-intl": "trae_solo_intl",
	}
	platformNameByVariant = map[string]string{
		"cn":        "TRAE CN",
		"solo":      "TRAE SOLO CN",
		"intl":      "TRAE",
		"solo-intl": "TRAE SOLO",
	}
)

func PlatformIDFor(variant string) string {
	if v, ok := platformIDByVariant[variant]; ok {
		return v
	}
	return platformIDByVariant["cn"]
}
func PlatformNameFor(variant string) string {
	if v, ok := platformNameByVariant[variant]; ok {
		return v
	}
	return platformNameByVariant["cn"]
}
func IsSoloVariant(variant string) bool {
	switch strings.ToLower(strings.TrimSpace(variant)) {
	case "solo", "solo-intl":
		return true
	}
	return false
}
func IsIntlVariant(variant string) bool {
	switch strings.ToLower(strings.TrimSpace(variant)) {
	case "intl", "solo-intl":
		return true
	}
	return false
}
func ClientIDFor(variant string) string {
	if v, ok := clientIDByVariant[variant]; ok {
		return v
	}
	return clientIDByVariant["cn"]
}
func FunctionFor(variant string) string {
	if v, ok := functionByVariant[variant]; ok {
		return v
	}
	return functionByVariant["cn"]
}
