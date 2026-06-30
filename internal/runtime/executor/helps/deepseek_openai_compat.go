package helps

import (
	"encoding/json"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// NormalizeDeepSeekOpenAICompatPayload applies final DeepSeek v4 request fixes
// after OpenAI-compatible translation, thinking application, and payload rules.
func NormalizeDeepSeekOpenAICompatPayload(model string, body []byte) []byte {
	if len(body) == 0 || !looksLikeJSONObject(body) || !isDeepSeekV4Model(model, body) {
		return body
	}

	var err error
	if body, err = ensureDeepSeekRequiredArrays(body); err != nil {
		return body
	}
	if body, err = normalizeDeepSeekReasoningEffortNone(body); err != nil {
		return body
	}
	if body, err = applyDeepSeekToolChoiceSplit(body); err != nil {
		return body
	}
	return body
}

func isDeepSeekV4Model(model string, body []byte) bool {
	for _, candidate := range []string{
		model,
		gjson.GetBytes(body, "model").String(),
	} {
		if deepSeekV4ModelName(candidate) {
			return true
		}
	}
	return false
}

func deepSeekV4ModelName(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	if slash := strings.LastIndex(model, "/"); slash >= 0 {
		model = strings.TrimSpace(model[slash+1:])
	}
	return model == "deepseek-v4" || strings.HasPrefix(model, "deepseek-v4-")
}

func looksLikeJSONObject(body []byte) bool {
	for _, b := range body {
		switch b {
		case ' ', '\t', '\n', '\r':
			continue
		case '{':
			return true
		default:
			return false
		}
	}
	return false
}

func ensureDeepSeekRequiredArrays(body []byte) ([]byte, error) {
	if !gjson.GetBytes(body, "tools").IsArray() {
		return body, nil
	}
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		return body, err
	}
	tools, _ := root["tools"].([]any)
	for _, t := range tools {
		tool, _ := t.(map[string]any)
		fn, _ := tool["function"].(map[string]any)
		if fn == nil {
			continue
		}
		params, _ := fn["parameters"].(map[string]any)
		if params == nil {
			continue
		}
		walkDeepSeekSchema(params)
	}
	return json.Marshal(root)
}

func walkDeepSeekSchema(node map[string]any) {
	if node == nil {
		return
	}
	if deepSeekObjectSchema(node) {
		if _, ok := node["required"]; !ok {
			node["required"] = []any{}
		}
	}
	if props, ok := node["properties"].(map[string]any); ok {
		for _, p := range props {
			if child, ok := p.(map[string]any); ok {
				walkDeepSeekSchema(child)
			}
		}
	}
	if defs, ok := node["$defs"].(map[string]any); ok {
		for _, d := range defs {
			if child, ok := d.(map[string]any); ok {
				walkDeepSeekSchema(child)
			}
		}
	}
	if items, ok := node["items"].(map[string]any); ok {
		walkDeepSeekSchema(items)
	}
	if ap, ok := node["additionalProperties"].(map[string]any); ok {
		walkDeepSeekSchema(ap)
	}
}

func deepSeekObjectSchema(node map[string]any) bool {
	if t, ok := node["type"].(string); ok {
		return t == "object"
	}
	_, hasProps := node["properties"]
	_, hasDefs := node["$defs"]
	_, hasAP := node["additionalProperties"]
	return hasProps || hasDefs || hasAP
}

func normalizeDeepSeekReasoningEffortNone(body []byte) ([]byte, error) {
	if !deepSeekReasoningEffortNone(body) {
		return body, nil
	}
	return applyDeepSeekThinkingDisabled(body)
}

func applyDeepSeekToolChoiceSplit(body []byte) ([]byte, error) {
	toolChoice := gjson.GetBytes(body, "tool_choice")
	if !toolChoice.Exists() {
		return body, nil
	}
	if deepSeekToolChoiceForcesToolUse(toolChoice) {
		if deepSeekThinkingDisabled(body) && !deepSeekReasoningEffortNone(body) {
			return body, nil
		}
		return applyDeepSeekThinkingDisabled(body)
	}
	if deepSeekThinkingEnabled(body) {
		return sjson.DeleteBytes(body, "tool_choice")
	}
	return body, nil
}

func deepSeekThinkingEnabled(body []byte) bool {
	thinkingType := strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "thinking.type").String()))
	switch thinkingType {
	case "disabled":
		return false
	case "enabled", "adaptive", "auto":
		return true
	}
	effort := strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "reasoning_effort").String()))
	return effort != "" && effort != "none"
}

func deepSeekThinkingDisabled(body []byte) bool {
	thinkingType := strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "thinking.type").String()))
	if thinkingType == "disabled" {
		return true
	}
	return deepSeekReasoningEffortNone(body)
}

func deepSeekReasoningEffortNone(body []byte) bool {
	effort := strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "reasoning_effort").String()))
	return effort == "none"
}

func deepSeekToolChoiceForcesToolUse(choice gjson.Result) bool {
	switch choice.Type {
	case gjson.String:
		switch strings.ToLower(strings.TrimSpace(choice.String())) {
		case "", "auto", "none":
			return false
		default:
			return true
		}
	case gjson.JSON:
		choiceType := strings.ToLower(strings.TrimSpace(choice.Get("type").String()))
		switch choiceType {
		case "", "auto", "none":
			return false
		default:
			return true
		}
	default:
		return true
	}
}

func applyDeepSeekThinkingDisabled(body []byte) ([]byte, error) {
	result, errDelete := sjson.DeleteBytes(body, "reasoning_effort")
	if errDelete != nil {
		return body, errDelete
	}
	result, errSet := sjson.SetRawBytes(result, "thinking", []byte(`{"type":"disabled"}`))
	if errSet != nil {
		return body, errSet
	}
	return result, nil
}
