package protocol

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"work2api/internal/jsonutil"
)

type NamespaceName struct{ Namespace, Name string }

// PrepareClientTools lowers request-local identities without mutating history.
func PrepareClientTools(body map[string]any) (map[string]any, map[string]bool, map[string]NamespaceName, error) {
	needsNamespace := false
	for _, raw := range jsonutil.SliceAt(body, "tools") {
		item, _ := raw.(map[string]any)
		needsNamespace = needsNamespace || jsonutil.StringAt(item, "type") == "namespace"
	}
	for _, raw := range jsonutil.SliceAt(body, "input") {
		item, _ := raw.(map[string]any)
		kind := jsonutil.StringAt(item, "type")
		needsNamespace = needsNamespace || (kind == "function_call" || kind == "custom_tool_call") && jsonutil.StringAt(item, "namespace") != ""
	}
	choice := jsonutil.MapAt(body, "tool_choice")
	needsNamespace = needsNamespace || jsonutil.StringAt(choice, "namespace") != "" || jsonutil.StringAt(choice, "type") == "namespace"
	if !needsNamespace {
		prepared, custom, err := PrepareCustomTools(body)
		return prepared, custom, nil, err
	}
	out := copyCustomMap(body)
	mapping := map[string]NamespaceName{}
	used := map[string]bool{}
	for _, raw := range jsonutil.SliceAt(body, "tools") {
		tool, _ := raw.(map[string]any)
		if jsonutil.StringAt(tool, "type") != "namespace" {
			used[jsonutil.StringAt(tool, "name")] = true
		}
	}
	bind := func(namespace, name string) (string, error) {
		if strings.TrimSpace(namespace) == "" || strings.TrimSpace(name) == "" {
			return "", fmt.Errorf("namespace 工具必须指定名称")
		}
		sum := sha256.Sum256([]byte(namespace + "\x00" + name))
		flat := "ns_" + hex.EncodeToString(sum[:16])
		entry := NamespaceName{namespace, name}
		if used[flat] {
			return "", fmt.Errorf("namespace 工具与普通工具名称冲突")
		}
		if old, ok := mapping[flat]; ok && old != entry {
			return "", fmt.Errorf("namespace 工具名称冲突")
		}
		mapping[flat] = entry
		return flat, nil
	}
	tools := []any{}
	declared := map[string]bool{}
	for _, raw := range jsonutil.SliceAt(body, "tools") {
		tool, _ := raw.(map[string]any)
		if jsonutil.StringAt(tool, "type") != "namespace" {
			tools = append(tools, raw)
			continue
		}
		namespace := jsonutil.StringAt(tool, "name")
		children := jsonutil.SliceAt(tool, "tools")
		if len(children) == 0 {
			children = jsonutil.SliceAt(tool, "children")
		}
		if len(children) == 0 {
			return nil, nil, nil, fmt.Errorf("namespace 工具不能为空")
		}
		for _, childRaw := range children {
			child, ok := childRaw.(map[string]any)
			kind := jsonutil.StringAt(child, "type")
			if !ok || kind != "function" && kind != "custom" {
				return nil, nil, nil, fmt.Errorf("namespace 仅支持 function/custom 子工具")
			}
			flat, err := bind(namespace, jsonutil.StringAt(child, "name"))
			if err != nil {
				return nil, nil, nil, err
			}
			if declared[flat] {
				return nil, nil, nil, fmt.Errorf("namespace 子工具重复")
			}
			declared[flat] = true
			copy := copyCustomMap(child)
			copy["name"] = flat
			copy["description"] = strings.TrimSpace("Tool identity: " + namespace + "." + jsonutil.StringAt(child, "name") + ".\n" + jsonutil.StringAt(tool, "description") + "\n" + jsonutil.StringAt(child, "description"))
			tools = append(tools, copy)
		}
	}
	if _, ok := body["tools"]; ok {
		out["tools"] = tools
	}
	if history, ok := body["input"].([]any); ok {
		var cloned []any
		for i, raw := range history {
			item, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			kind := jsonutil.StringAt(item, "type")
			if (kind == "function_call" || kind == "custom_tool_call") && jsonutil.StringAt(item, "namespace") != "" {
				flat, err := bind(jsonutil.StringAt(item, "namespace"), jsonutil.StringAt(item, "name"))
				if err != nil {
					return nil, nil, nil, err
				}
				copy := copyCustomMap(item)
				if cloned == nil {
					cloned = append([]any(nil), history...)
				}
				copy["name"] = flat
				delete(copy, "namespace")
				cloned[i] = copy
			}
		}
		if cloned != nil {
			out["input"] = cloned
		}
	}
	if choice, ok := body["tool_choice"].(map[string]any); ok {
		if namespace := jsonutil.StringAt(choice, "namespace"); namespace != "" {
			flat, err := bind(namespace, jsonutil.StringAt(choice, "name"))
			if err != nil {
				return nil, nil, nil, err
			}
			if !declared[flat] {
				return nil, nil, nil, fmt.Errorf("tool_choice 指向未声明的 namespace 工具")
			}
			copy := copyCustomMap(choice)
			copy["name"] = flat
			delete(copy, "namespace")
			out["tool_choice"] = copy
		} else if jsonutil.StringAt(choice, "type") == "namespace" {
			if strings.TrimSpace(jsonutil.StringAt(choice, "name")) == "" {
				return nil, nil, nil, fmt.Errorf("tool_choice namespace 名称不能为空")
			}
			selected := []any{}
			for _, raw := range tools {
				tool, _ := raw.(map[string]any)
				if mapping[jsonutil.StringAt(tool, "name")].Namespace == jsonutil.StringAt(choice, "name") {
					selected = append(selected, raw)
				}
			}
			if len(selected) == 0 {
				return nil, nil, nil, fmt.Errorf("tool_choice 指向未声明的 namespace")
			}
			out["tools"] = selected
			out["tool_choice"] = "auto"
		}
	}
	prepared, custom, err := PrepareCustomTools(out)
	return prepared, custom, mapping, err
}
