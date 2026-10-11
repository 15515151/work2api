package trae

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Desktop storage decoding follows buddy-proxy, MIT (see NOTICE.md). Read-only:
// the explicit management action imports a verified copy and never rewrites IDE files.
func decodeDesktop(value string) ([]byte, error) {
	if strings.HasPrefix(strings.TrimSpace(value), "{") {
		return []byte(value), nil
	}
	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(raw) < 38+64 || (len(raw)-38)%aes.BlockSize != 0 {
		return nil, errors.New("不支持的桌面凭据格式")
	}
	saltHex := ""
	switch {
	case bytes.Equal(raw[:6], []byte{0x74, 0x63, 5, 16, 0, 0}):
		saltHex = "4DD4C2E6B83162090E52B3C7A6733BA41CB2462B829AB58A196B39DB57177524F49BAF7F08E8D68D26A72E37C1A95A2F1F05A51892AEF2949732B62A38AADD58"
	case bytes.Equal(raw[:6], []byte{0x12, 0x39, 0x20, 0x20, 2, 3}):
		saltHex = "490CC21292B05D0CC06CCBE91FB92E21B5A7C538811DC85E27A96E28104E1CF69C7166EFAB232807390DA78A8B91E49C44B49A43FEB941A1E46F2656E613E47B"
	default:
		return nil, errors.New("不支持的桌面凭据版本")
	}
	salt, _ := hex.DecodeString(saltHex)
	randomHash := sha512.Sum512(raw[6:38])
	keyHash := sha512.Sum512(append(randomHash[:], salt...))
	block, err := aes.NewCipher(keyHash[:16])
	if err != nil {
		return nil, err
	}
	decoded := make([]byte, len(raw)-38)
	cipher.NewCBCDecrypter(block, keyHash[16:32]).CryptBlocks(decoded, raw[38:])
	digest := sha512.Sum512(decoded[64:])
	if subtle.ConstantTimeCompare(decoded[:64], digest[:]) != 1 {
		return nil, errors.New("桌面凭据完整性验证失败")
	}
	return decoded[64:], nil
}

func (r *Runtime) ImportLocalAccounts(ctx context.Context) (map[string]any, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	root := filepath.Join(home, ".config")
	if runtime.GOOS == "windows" {
		root = os.Getenv("APPDATA")
		if root == "" {
			root = filepath.Join(home, "AppData", "Roaming")
		}
	} else if runtime.GOOS == "darwin" {
		root = filepath.Join(home, "Library", "Application Support")
	}
	results := []map[string]any{}
	for _, source := range []struct{ dir, product string }{{"Trae CN", "cn"}, {"Trae SOLO", "solo"}} {
		path := filepath.Join(root, source.dir, "User", "globalStorage", "storage.json")
		info, e := os.Lstat(path)
		if errors.Is(e, os.ErrNotExist) {
			continue
		}
		result := map[string]any{"product": source.product, "status": "error"}
		if e != nil || !info.Mode().IsRegular() || info.Size() > 8<<20 {
			result["message"] = "桌面凭据文件不可读取"
			results = append(results, result)
			continue
		}
		f, e := os.Open(path)
		if e != nil {
			result["message"] = "桌面凭据读取失败"
			results = append(results, result)
			continue
		}
		raw, e := io.ReadAll(io.LimitReader(f, 8<<20))
		f.Close()
		var storage map[string]any
		if e != nil || json.Unmarshal(raw, &storage) != nil {
			result["message"] = "桌面存储格式无效"
			results = append(results, result)
			continue
		}
		plain, e := decodeDesktop(text(storage["iCubeAuthInfo://icube.cloudide"]))
		var data map[string]any
		if e != nil || json.Unmarshal(plain, &data) != nil {
			result["message"] = "未检测到可验证的登录态，请使用网页登录"
			results = append(results, result)
			continue
		}
		token := ""
		for _, key := range []string{"token", "Token", "access_token"} {
			if token = text(data[key]); token != "" {
				break
			}
		}
		if token == "" {
			if nested, ok := data["data"].(map[string]any); ok {
				token = text(nested["token"])
			}
		}
		var expiry any
		if parts := strings.Split(token, "."); len(parts) == 3 {
			if payload, e := base64.RawURLEncoding.DecodeString(parts[1]); e == nil {
				var claims map[string]any
				if json.Unmarshal(payload, &claims) == nil {
					expiry = claims["exp"]
				}
			}
		}
		credential, _ := json.Marshal(map[string]any{"accessToken": token, "expiresAt": expiry, "domain": "trae.cn", "variant": source.product})
		account, e := r.AddAccountByToken(ctx, string(credential), map[string]any{"product": source.product})
		if e != nil {
			result["message"] = e.Error()
		} else {
			result["status"] = "ok"
			result["account"] = account
			result["message"] = "已导入当前登录态；持续刷新建议使用网页登录"
		}
		results = append(results, result)
	}
	return map[string]any{"results": results}, nil
}
