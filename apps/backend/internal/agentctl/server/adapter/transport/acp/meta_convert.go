package acp

import (
	"fmt"

	"github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// convertAuthMethods converts ACP auth methods to stream types,
// normalizing known _meta patterns (terminal-auth) while preserving raw _meta.
func convertAuthMethods(methods []acp.AuthMethod) []streams.AuthMethodInfo {
	if len(methods) == 0 {
		return nil
	}
	result := make([]streams.AuthMethodInfo, 0, len(methods))
	for _, m := range methods {
		info := streams.AuthMethodInfo{
			ID:          string(m.Id),
			Name:        m.Name,
			Description: derefStr(m.Description),
			Meta:        toStringMap(m.Meta),
		}
		// Normalize _meta.terminal-auth → TerminalAuth
		info.TerminalAuth = extractTerminalAuth(info.Meta)
		result = append(result, info)
	}
	return result
}

// extractTerminalAuth normalizes the terminal-auth pattern from _meta.
// Example _meta: {"terminal-auth": {"command": "copilot", "args": ["auth", "login"], "label": "Login with GitHub"}}
func extractTerminalAuth(meta map[string]any) *streams.TerminalAuth {
	if meta == nil {
		return nil
	}
	raw, ok := meta["terminal-auth"]
	if !ok {
		return nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	ta := &streams.TerminalAuth{}
	if cmd, ok := m["command"].(string); ok {
		ta.Command = cmd
	}
	if label, ok := m["label"].(string); ok {
		ta.Label = label
	}
	if args, ok := m["args"].([]any); ok {
		for _, a := range args {
			if s, ok := a.(string); ok {
				ta.Args = append(ta.Args, s)
			}
		}
	}
	if ta.Command == "" {
		return nil
	}
	return ta
}

// convertSessionModels converts ACP model info to stream types,
// normalizing known _meta patterns (copilotUsage) while preserving raw _meta.
func convertSessionModels(models []acp.ModelInfo) []streams.SessionModelInfo {
	if len(models) == 0 {
		return nil
	}
	result := make([]streams.SessionModelInfo, 0, len(models))
	for _, m := range models {
		info := streams.SessionModelInfo{
			ModelID:     string(m.ModelId),
			Name:        m.Name,
			Description: derefStr(m.Description),
			Meta:        toStringMap(m.Meta),
		}
		// Normalize _meta.copilotUsage → UsageMultiplier
		info.UsageMultiplier = extractUsageMultiplier(info.Meta)
		result = append(result, info)
	}
	return result
}

// extractUsageMultiplier normalizes the copilotUsage pattern from model _meta.
// Example _meta: {"copilotUsage": "3x"} or {"copilotUsage": 3}
func extractUsageMultiplier(meta map[string]any) string {
	if meta == nil {
		return ""
	}
	raw, ok := meta["copilotUsage"]
	if !ok {
		return ""
	}
	switch v := raw.(type) {
	case string:
		return v
	case float64:
		if v == float64(int64(v)) {
			return fmt.Sprintf("%dx", int64(v))
		}
		return fmt.Sprintf("%.2fx", v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// extractConfigOptions extracts config options from ACP session _meta.
// Example _meta: {"configOptions": [{"type": "select", "id": "mode", "name": "Mode", ...}]}
func extractConfigOptions(meta any) []streams.ConfigOption {
	m, ok := toAnyMap(meta)
	if !ok {
		return nil
	}
	raw, ok := m["configOptions"]
	if !ok {
		return nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	result := make([]streams.ConfigOption, 0, len(items))
	for _, item := range items {
		opt, ok := item.(map[string]any)
		if !ok {
			continue
		}
		co := streams.ConfigOption{
			Type:         getString(opt, "type"),
			ID:           getString(opt, "id"),
			Name:         getString(opt, "name"),
			CurrentValue: getString(opt, "currentValue"),
			Category:     getString(opt, "category"),
		}
		if options, ok := opt["options"].([]any); ok {
			for _, o := range options {
				if om, ok := o.(map[string]any); ok {
					co.Options = append(co.Options, streams.ConfigOptionValue{
						Value: getString(om, "value"),
						Name:  getString(om, "name"),
					})
				}
			}
		}
		result = append(result, co)
	}
	return result
}

// extractPromptUsage extracts token usage from ACP prompt response _meta.
// Example _meta: {"usage": {"input_tokens": 100, "output_tokens": 50, "total_tokens": 150}}
func extractPromptUsage(meta any) *streams.PromptUsage {
	m, ok := toAnyMap(meta)
	if !ok {
		return nil
	}
	raw, ok := m["usage"]
	if !ok {
		return nil
	}
	u, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	usage := &streams.PromptUsage{
		InputTokens:       getInt64(u, "input_tokens"),
		OutputTokens:      getInt64(u, "output_tokens"),
		CachedReadTokens:  getInt64(u, "cached_read_tokens"),
		CachedWriteTokens: getInt64(u, "cached_write_tokens"),
		TotalTokens:       getInt64(u, "total_tokens"),
	}
	// Also check camelCase variants
	if usage.InputTokens == 0 {
		usage.InputTokens = getInt64(u, "inputTokens")
	}
	if usage.OutputTokens == 0 {
		usage.OutputTokens = getInt64(u, "outputTokens")
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = getInt64(u, "totalTokens")
	}
	if usage.CachedReadTokens == 0 {
		usage.CachedReadTokens = getInt64(u, "cachedReadTokens")
	}
	if usage.CachedWriteTokens == 0 {
		usage.CachedWriteTokens = getInt64(u, "cachedWriteTokens")
	}
	if usage.InputTokens == 0 && usage.OutputTokens == 0 && usage.TotalTokens == 0 {
		return nil
	}
	return usage
}

// toStringMap converts any to map[string]any, handling the common JSON unmarshal case.
func toStringMap(v any) map[string]any {
	m, _ := toAnyMap(v)
	return m
}

// toAnyMap converts any to map[string]any.
func toAnyMap(v any) (map[string]any, bool) {
	if v == nil {
		return nil, false
	}
	m, ok := v.(map[string]any)
	return m, ok
}

// getString safely extracts a string from a map.
func getString(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

// getInt64 safely extracts an int64 from a map (JSON numbers are float64).
func getInt64(m map[string]any, key string) int64 {
	switch v := m[key].(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	default:
		return 0
	}
}
