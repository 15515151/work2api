// Adapted from mmqz/cpa-multi-plugins, MIT; see NOTICE.md.
package upstream

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"work2api/internal/trae/auth"
)

const epExchangeBound = "/trae/api/v3/oauth/ExchangeToken"

type exchangeIdentity struct {
	ClientID     string
	PlatformCode string // SOLO_PC | IDE_PC
	Version      string
}

func exchangeIdentitiesFor(variant string) []exchangeIdentity {
	return []exchangeIdentity{{ClientID: ClientIDFor(variant), PlatformCode: PlatformCodeFor(variant), Version: IdeVersion}}
}
func PlatformCodeFor(variant string) string {
	if IsSoloVariant(variant) {
		return "SOLO_PC"
	}
	return "IDE_PC"
}
func deviceProofPayload(path, clientID, refreshToken string, ts int64, nonce string) string {
	return strings.Join([]string{"POST", path, clientID, refreshToken, fmt.Sprintf("%d", ts), nonce}, "\n")
}
func newProofNonce() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%032x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
func boundDeviceInfo(a *auth.Auth, platformCode, clientVersion string) map[string]any {
	deviceType := "windows"
	osVersion := OSVersion
	return map[string]any{
		"DeviceID":        a.DeviceID,
		"MachineID":       a.MachineID,
		"PlatformCode":    platformCode,
		"DeviceType":      "PC",
		"DeviceName":      "",
		"DeviceModel":     DeviceBrand,
		"ClientVersion":   clientVersion,
		"DevicePublicKey": a.DevicePublicKey,
		"DeviceBrand":     DeviceBrand,
		"DeviceCPU":       "",
		"OSInfo":          deviceType,
		"OSVersion":       osVersion,
	}
}
func ensureDeviceKeyLocked(a *auth.Auth) {
	if strings.TrimSpace(a.DevicePublicKey) != "" && strings.TrimSpace(a.DevicePrivateKey) != "" {
		return
	}
	pubPEM, privPEM, err := auth.GenerateDeviceKeyPair()
	if err != nil {
		return // bound exchange will fail without keys; legacy path still renews
	}
	a.DevicePublicKey = pubPEM
	a.DevicePrivateKey = privPEM
}
func (c *Client) boundExchangeLocked(a *auth.Auth) error {
	if strings.TrimSpace(a.RefreshToken) == "" {
		return fmt.Errorf("no refreshToken")
	}
	if a.DevicePublicKey == "" || a.DevicePrivateKey == "" {
		return fmt.Errorf("no device key pair")
	}
	rt := a.RefreshToken // held write lock
	hosts := exchangeHosts(a.APIHost, c.OAuthHost)
	var lastErr error
	for _, id := range exchangeIdentitiesFor(a.Variant) {
		for _, host := range hosts {
			ts := time.Now().Unix()
			nonce := newProofNonce()
			proof, err := auth.SignDeviceProof(a.DevicePrivateKey, deviceProofPayload(epExchangeBound, id.ClientID, rt, ts, nonce))
			if err != nil {
				return err // key material problem: no point trying other hosts
			}
			body := map[string]any{
				"ClientID":     id.ClientID,
				"ClientSecret": "",
				"RefreshToken": rt,
				"DeviceInfo":   boundDeviceInfo(a, id.PlatformCode, id.Version),
				"DeviceProof": map[string]any{
					"Signature": proof,
					"Timestamp": ts,
					"Nonce":     nonce,
				},
				"IDEVersion": id.Version,
			}
			raw, _ := json.Marshal(body)
			req, err := http.NewRequest(http.MethodPost, host+epExchangeBound, bytes.NewReader(raw))
			if err != nil {
				return err
			}
			OAuthHeaders(req)
			data, err := c.doJSON(req)
			if err != nil {
				lastErr = err
				continue
			}
			token, refreshToken, expiresAt, ok := parseBoundExchange(data)
			if !ok {
				lastErr = fmt.Errorf("bound exchange: no token in response")
				continue
			}
			a.AccessToken = token
			if refreshToken != "" {
				a.RefreshToken = refreshToken
			}
			if expiresAt > 0 {
				a.ExpiresAt = expiresAt
			}
			log.Printf("exchange refresh (bound): uid=%s variant=%q client=%s platform=%s", a.UID, a.Variant, id.ClientID, id.PlatformCode)
			return nil
		}
	}
	return lastErr
}
func parseBoundExchange(data []byte) (token, refreshToken string, expiresAt int64, ok bool) {
	var resp struct {
		Result struct {
			Token               string `json:"Token"`
			TokenExpireAt       int64  `json:"TokenExpireAt"`
			TokenExpireDuration int64  `json:"TokenExpireDuration"`
			RefreshToken        string `json:"RefreshToken"`
		} `json:"Result"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", "", 0, false
	}
	if resp.Result.Token == "" {
		return "", "", 0, false
	}
	exp := int64(0)
	if resp.Result.TokenExpireAt > 0 {
		exp = normalizeExpiresAt(resp.Result.TokenExpireAt)
	} else if resp.Result.TokenExpireDuration > 0 {
		exp = time.Now().Add(time.Duration(resp.Result.TokenExpireDuration) * time.Second).Unix()
	}
	return resp.Result.Token, resp.Result.RefreshToken, exp, true
}
