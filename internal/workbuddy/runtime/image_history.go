package runtime

import (
	"encoding/base64"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

var imageParamPath = regexp.MustCompile(`^messages\[([0-9]+)\]\.content\[([0-9]+)\](?:\.image_url(?:\.url)?)?$`)

// Accept only a structured, unambiguous path into the actual outgoing body.
// A free-text error message or an out-of-bounds index is not image evidence.
func rejectedImagePosition(raw []byte) (int, int, bool) {
	root := JsonRoot(string(raw))
	nodes := []map[string]any{root}
	for _, key := range []string{"error", "extError"} {
		if node, ok := root[key].(map[string]any); ok {
			nodes = append(nodes, node)
		}
	}
	mi, pi, found := 0, 0, false
	for _, node := range nodes {
		param, _ := node["param"].(string)
		match := imageParamPath.FindStringSubmatch(param)
		if match == nil {
			continue
		}
		m, e1 := strconv.Atoi(match[1])
		p, e2 := strconv.Atoi(match[2])
		if e1 != nil || e2 != nil {
			continue
		}
		if found && (mi != m || pi != p) {
			return 0, 0, false
		}
		mi, pi, found = m, p, true
	}
	return mi, pi, found
}

// Check decoded bytes with a fixed buffer, never a full decoded-image copy.
// Do not fetch URLs or infer that an unfamiliar image format is invalid.
func compatibleImageURL(url string) (string, bool, bool) {
	trimmed := strings.TrimSpace(url)
	if trimmed == "" {
		return url, false, true
	}
	if !strings.HasPrefix(strings.ToLower(trimmed), "data:") {
		return url, false, false
	}
	header, data, ok := strings.Cut(trimmed[5:], ",")
	if !ok {
		return url, false, true
	}
	meta := strings.Split(header, ";")
	mime := strings.ToLower(strings.TrimSpace(meta[0]))
	if !strings.HasPrefix(mime, "image/") || mime == "image/" {
		return url, false, true
	}
	// Non-base64 data URLs may be valid; leave unsupported encodings alone.
	if len(meta) != 2 || !strings.EqualFold(strings.TrimSpace(meta[1]), "base64") {
		return url, false, false
	}
	cleaned := data
	if strings.ContainsAny(data, " \t\r\n") {
		cleaned = strings.Map(func(r rune) rune {
			if r == ' ' || r == '\t' || r == '\r' || r == '\n' {
				return -1
			}
			return r
		}, data)
	}
	if cleaned == "" {
		return url, false, true
	}
	encoding := base64.StdEncoding
	if !strings.Contains(cleaned, "=") {
		encoding = base64.RawStdEncoding
	}
	decoder := base64.NewDecoder(encoding, strings.NewReader(cleaned))
	var prefix [512]byte
	n, err := io.ReadFull(decoder, prefix[:])
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return url, false, true
	}
	if _, err := io.Copy(io.Discard, decoder); err != nil || n == 0 {
		return url, false, true
	}
	detected := http.DetectContentType(prefix[:n])
	if strings.HasPrefix(detected, "image/") {
		mime = detected
	}
	padding := (4 - len(cleaned)%4) % 4
	// Keep valid data byte-for-byte, unless a concrete correction is available.
	if trimmed == url && header == mime+";base64" && cleaned == data && padding == 0 {
		return url, false, false
	}
	return "data:" + mime + ";base64," + cleaned + strings.Repeat("=", padding), true, false
}

func locallyCompatibleImages(body map[string]any, raw []byte) (map[string]any, int, int) {
	mi, pi, located := rejectedImagePosition(raw)
	if located {
		out, _, removed := transformRequestImages(body, func(i, j int, part map[string]any) (map[string]any, bool, bool) {
			return nil, false, i == mi && j == pi
		})
		if removed > 0 {
			return out, 0, removed
		}
	}
	return transformRequestImages(body, func(_, _ int, part map[string]any) (map[string]any, bool, bool) {
		// WorkBuddy's protocol adapters normalize images to Chat image_url.
		if part["type"] != "image_url" {
			return nil, false, false
		}
		var url string
		switch image := part["image_url"].(type) {
		case string:
			url = image
		case map[string]any:
			url, _ = image["url"].(string)
		}
		value, repaired, invalid := compatibleImageURL(url)
		if !repaired {
			return nil, false, invalid
		}
		updated := copyImageMap(part)
		image := map[string]any{}
		if old, ok := part["image_url"].(map[string]any); ok {
			image = copyImageMap(old)
		}
		image["url"] = value
		updated["image_url"] = image
		return updated, true, false
	})
}

func copyImageMap(source map[string]any) map[string]any {
	out := make(map[string]any, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}

// withoutRequestImages is used only after an explicit upstream image rejection.
// Copy only changed containers. The original request and future calls retain
// every image, including newly uploaded images.
func withoutRequestImages(body map[string]any) (map[string]any, int) {
	out, _, removed := transformRequestImages(body, func(_, _ int, _ map[string]any) (map[string]any, bool, bool) { return nil, false, true })
	return out, removed
}

func transformRequestImages(body map[string]any, transform func(int, int, map[string]any) (map[string]any, bool, bool)) (map[string]any, int, int) {
	messages, _ := body["messages"].([]any)
	var copied []any
	removed := 0
	repaired := 0
	for i := 0; i < len(messages); i++ {
		message, _ := messages[i].(map[string]any)
		role, _ := message["role"].(string)
		if role != "user" && role != "assistant" && role != "tool" {
			continue
		}
		parts, _ := message["content"].([]any)
		var content []any
		for j, value := range parts {
			part, _ := value.(map[string]any)
			kind, _ := part["type"].(string)
			if kind != "image_url" && kind != "image" && kind != "input_image" {
				continue
			}
			updated, fixed, drop := transform(i, j, part)
			if !fixed && !drop {
				continue
			}
			if content == nil {
				content = append([]any(nil), parts...)
			}
			if drop {
				content[j] = map[string]any{"type": "text", "text": "[本次图片解析失败，已降级为文字请求；此处图片内容未提供。]"}
				removed++
			} else {
				content[j] = updated
				repaired++
			}
		}
		if content != nil {
			if copied == nil {
				copied = append([]any(nil), messages...)
			}
			updated := make(map[string]any, len(message))
			for key, value := range message {
				updated[key] = value
			}
			updated["content"] = content
			copied[i] = updated
		}
	}
	if removed == 0 && repaired == 0 {
		return body, 0, 0
	}
	out := make(map[string]any, len(body))
	for key, value := range body {
		out[key] = value
	}
	out["messages"] = copied
	return out, repaired, removed
}
