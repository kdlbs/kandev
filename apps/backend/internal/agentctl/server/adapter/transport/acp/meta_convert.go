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
		id, name, desc, meta := flattenAuthMethod(m)
		if id == "" && name == "" {
			continue
		}
		info := streams.AuthMethodInfo{
			ID:          id,
			Name:        name,
			Description: derefStr(desc),
			Meta:        toStringMap(meta),
		}
		// Normalize _meta.terminal-auth → TerminalAuth
		info.TerminalAuth = extractTerminalAuth(info.Meta)
		result = append(result, info)
	}
	return result
}

// flattenAuthMethod collapses upstream's tagged-union AuthMethod into the
// (id, name, description, meta) tuple our streams layer expects.
func flattenAuthMethod(m acp.AuthMethod) (string, string, *string, map[string]any) {
	switch {
	case m.Agent != nil:
		return m.Agent.Id, m.Agent.Name, m.Agent.Description, m.Agent.Meta
	case m.Terminal != nil:
		return m.Terminal.Id, m.Terminal.Name, m.Terminal.Description, m.Terminal.Meta
	case m.EnvVar != nil:
		return m.EnvVar.Id, m.EnvVar.Name, m.EnvVar.Description, m.EnvVar.Meta
	}
	return "", "", nil, nil
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

// convertACPConfigOptions converts typed ACP SessionConfigOption to stream ConfigOption.
// This is the preferred path when the SDK properly parses configOptions from the response.
func convertACPConfigOptions(opts []acp.SessionConfigOption) []streams.ConfigOption {
	if len(opts) == 0 {
		return nil
	}
	result := make([]streams.ConfigOption, 0, len(opts))
	for _, opt := range opts {
		if opt.Select == nil {
			continue
		}
		s := opt.Select
		co := streams.ConfigOption{
			Type:         s.Type,
			ID:           string(s.Id),
			Name:         s.Name,
			CurrentValue: string(s.CurrentValue),
		}
		if s.Category != nil {
			co.Category = string(*s.Category)
		}
		// Extract options from ungrouped list (most common case)
		if s.Options.Ungrouped != nil {
			for _, o := range *s.Options.Ungrouped {
				co.Options = append(co.Options, streams.ConfigOptionValue{
					Value: string(o.Value),
					Name:  o.Name,
				})
			}
		}
		result = append(result, co)
	}
	return result
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

// extractUsage pulls token usage out of a prompt response, walking the
// three CLI-specific shapes observed in /tmp/acp-probe-*.jsonl in order:
//
//  1. Typed Usage on the prompt response (the ACP SDK parses claude-acp
//     / opencode-acp's `result.usage` field directly). This is the
//     authoritative source when present.
//  2. `_meta.quota.token_count` (gemini). snake_case, no cached, no cost.
//  3. `_meta.usage` (legacy / forward-compat). camelCase or snake_case.
//
// Returns nil when no usable counts are found.
func extractUsage(resp *acp.PromptResponse) *streams.PromptUsage {
	if resp == nil {
		return nil
	}
	if u := fromTypedUsage(resp.Usage); u != nil {
		return u
	}
	if u := fromGeminiQuotaMeta(resp.Meta); u != nil {
		return u
	}
	return extractPromptUsage(resp.Meta)
}

// fromTypedUsage converts the SDK's typed Usage (parsed from
// result.usage on claude-acp / opencode-acp) to streams.PromptUsage.
// Returns nil when every count is zero.
func fromTypedUsage(u *acp.Usage) *streams.PromptUsage {
	if u == nil {
		return nil
	}
	usage := &streams.PromptUsage{
		InputTokens:  int64(u.InputTokens),
		OutputTokens: int64(u.OutputTokens),
		TotalTokens:  int64(u.TotalTokens),
	}
	if u.CachedReadTokens != nil {
		usage.CachedReadTokens = int64(*u.CachedReadTokens)
	}
	if u.CachedWriteTokens != nil {
		usage.CachedWriteTokens = int64(*u.CachedWriteTokens)
	}
	if u.ThoughtTokens != nil {
		usage.ThoughtTokens = int64(*u.ThoughtTokens)
	}
	if usage.InputTokens == 0 && usage.OutputTokens == 0 && usage.TotalTokens == 0 {
		return nil
	}
	return usage
}

// fromGeminiQuotaMeta extracts gemini's `_meta.quota.token_count` shape.
// The captured frame at /tmp/acp-probe-gemini.jsonl shows:
//
//	"_meta":{"quota":{"model_usage":[{
//	   "model":"gemini-3-flash-preview",
//	   "token_count":{"input_tokens":9796,"output_tokens":2}}]}}
//
// Tokens may also appear directly under `quota.token_count` without the
// model_usage[] envelope (older builds); handle both.
func fromGeminiQuotaMeta(meta map[string]any) *streams.PromptUsage {
	if meta == nil {
		return nil
	}
	quotaAny, ok := meta["quota"]
	if !ok {
		return nil
	}
	quota, ok := quotaAny.(map[string]any)
	if !ok {
		return nil
	}
	if u := geminiTokenCount(quota["token_count"]); u != nil {
		return u
	}
	usages, _ := quota["model_usage"].([]any)
	for _, item := range usages {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if u := geminiTokenCount(entry["token_count"]); u != nil {
			return u
		}
	}
	return nil
}

func geminiTokenCount(raw any) *streams.PromptUsage {
	tc, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	usage := &streams.PromptUsage{
		InputTokens:  getInt64(tc, "input_tokens"),
		OutputTokens: getInt64(tc, "output_tokens"),
	}
	if usage.InputTokens == 0 && usage.OutputTokens == 0 {
		return nil
	}
	usage.TotalTokens = usage.InputTokens + usage.OutputTokens
	return usage
}

// extractPromptUsage is the legacy `_meta.usage` reader retained for
// forward compatibility. Most production CLIs put usage at `result.usage`
// (parsed into the typed `resp.Usage` field) — extractUsage prefers that
// path and only falls through to this one.
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
	// Also check camelCase variants.
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
	if usage.ThoughtTokens == 0 {
		usage.ThoughtTokens = getInt64(u, "thoughtTokens")
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
