package runtime

// withoutRequestImages is used only after an explicit upstream image rejection.
// Copy only changed containers. The original request and future calls retain
// every image, including newly uploaded images.
func withoutRequestImages(body map[string]any) (map[string]any, int) {
	messages, _ := body["messages"].([]any)
	var copied []any
	removed := 0
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
			if content == nil {
				content = append([]any(nil), parts...)
			}
			content[j] = map[string]any{"type": "text", "text": "[本次图片解析失败，已降级为文字请求；此处图片内容未提供。]"}
			removed++
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
	if removed == 0 {
		return body, 0
	}
	out := make(map[string]any, len(body))
	for key, value := range body {
		out[key] = value
	}
	out["messages"] = copied
	return out, removed
}
