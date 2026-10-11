// Adapted from mmqz/cpa-multi-plugins, MIT; see NOTICE.md.
package upstream

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"

	"work2api/internal/trae/auth"
)

const clientUA = "Trae/" + IdeVersion

func SOLOHeaders(req *http.Request, a *auth.Auth, stream bool) {
	req.Header.Set("Content-Type", "application/json")
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	} else {
		req.Header.Set("Accept", "application/json")
	}
	req.Header.Set("User-Agent", clientUA)
	at := a.JWT() // 读锁快照，防与 RefreshToken 写并发竞态
	req.Header.Set("Authorization", "Cloud-IDE-JWT "+at)
	req.Header.Set("X-Cloudide-Token", at)
	req.Header.Set("X-Ide-Token", at)
	if a.UID != "" {
		req.Header.Set("X-Uid", a.UID)
	}
	req.Header.Set("X-App-Id", AppID)
	req.Header.Set("X-App-Version", "default")
	req.Header.Set("X-Ide-Version", IdeVersion)
	req.Header.Set("X-Ide-Version-Code", IdeVersionCode)
	req.Header.Set("X-App-Version-Code", IdeVersionCode)
	req.Header.Set("X-Ide-Version-Type", "stable")
	req.Header.Set("X-Device-Type", "windows")
	req.Header.Set("X-OS-Version", OSVersion)
	req.Header.Set("X-Device-Brand", DeviceBrand)
	req.Header.Set("Request-Traffic-Type", "prod")
	if a.MachineID != "" {
		req.Header.Set("X-Machine-Id", a.MachineID)
	}
	if a.DeviceID != "" {
		req.Header.Set("X-Device-Id", a.DeviceID)
	}
}
func UgHeaders(req *http.Request, a *auth.Auth) {
	ugBaseHeaders(req, a)
	req.Header.Set("Authorization", "Cloud-IDE-JWT "+a.JWT()) // 读锁快照
}

const (
	ugMarketClientID = "VSCode 1.107.1"
	ugIdeVersion     = "0.1.61"
)

func ugUserAgentFor(variant string) string {
	return ugMarketClientID + " (" + PlatformNameFor(variant) + ")"
}
func ugPackageTypeFor(variant string) string {
	if IsIntlVariant(variant) {
		return "stable"
	}
	return "stable_cn"
}
func newRequestID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}
func ttTraceID(requestID string) string {
	t := make([]byte, 16)
	_, _ = rand.Read(t)
	return "00-" + hex.EncodeToString(t) + "-" + strings.ReplaceAll(requestID, "-", "")[:16] + "-01"
}
func ugBaseHeaders(req *http.Request, a *auth.Auth) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("User-Agent", ugUserAgentFor(a.Variant))
	req.Header.Set("X-User-Region", "CN") // 抓包值；intl 维持存量行为（未验证差异）
	req.Header.Set("Accept-Language", "zh-CN")
	req.Header.Set("Package-Type", ugPackageTypeFor(a.Variant))
	req.Header.Set("X-Lgw-Req-Sdk-Type", "3")
	req.Header.Set("X-Market-Client-Id", ugMarketClientID)
	req.Header.Set("X-Device-Brand", DeviceBrand)
	req.Header.Set("X-Device-Type", "windows")
	req.Header.Set("X-OS-Version", OSVersion)
	req.Header.Set("App-Version", ugIdeVersion)
	rid := newRequestID()
	req.Header.Set("X-Request-Id", rid)
	req.Header.Set("X-TT-Trace-Id", ttTraceID(rid))
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "no-cors")
	req.Header.Set("Sec-Fetch-Site", "none")
	if a.DeviceID != "" {
		req.Header.Set("X-Device-Id", a.DeviceID)
	}
}
func OAuthHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", clientUA)
}
