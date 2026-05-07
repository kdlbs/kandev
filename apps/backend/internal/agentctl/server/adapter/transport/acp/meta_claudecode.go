package acp

import "strings"

// buildClaudeCodeMeta translates argv-style CLI flags
// (e.g. ["--plugin-dir", "/path", "--debug"]) into the
// _meta.claudeCode.options.extraArgs payload understood by the
// @agentclientprotocol/claude-agent-acp bridge (≥0.32.0). The bridge passes
// extraArgs to the Claude Agent SDK, which surfaces them as CLI flags on the
// underlying claude binary.
//
// Parsing rules (intentionally minimal — same conventions Kandev's command
// builder produces from AgentProfile.CLIFlags):
//   - "--key=value" → {key: "value"}
//   - "--key" followed by a non-flag token   → {key: <token>}
//   - "--key" followed by another --flag/EOF → {key: ""} (bare flag)
//   - tokens not starting with "--" are skipped (treated as orphaned values).
//
// Returns nil for empty input or when no flag is recognised so callers can
// pass the result directly into NewSessionRequest.Meta.
func buildClaudeCodeMeta(tokens []string) map[string]any {
	if len(tokens) == 0 {
		return nil
	}
	extraArgs := map[string]any{}
	for i := 0; i < len(tokens); i++ {
		raw := tokens[i]
		if !strings.HasPrefix(raw, "--") {
			continue
		}
		key := strings.TrimPrefix(raw, "--")
		if key == "" {
			continue
		}
		if eq := strings.IndexByte(key, '='); eq >= 0 {
			extraArgs[key[:eq]] = key[eq+1:]
			continue
		}
		// Look ahead: if the next token isn't another flag, consume it as the value.
		if i+1 < len(tokens) && !strings.HasPrefix(tokens[i+1], "--") {
			extraArgs[key] = tokens[i+1]
			i++
			continue
		}
		extraArgs[key] = "" // bare flag
	}
	if len(extraArgs) == 0 {
		return nil
	}
	return map[string]any{
		"claudeCode": map[string]any{
			"options": map[string]any{
				"extraArgs": extraArgs,
			},
		},
	}
}
