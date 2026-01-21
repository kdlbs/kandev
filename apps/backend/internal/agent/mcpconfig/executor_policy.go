package mcpconfig

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ExecutorPolicyConfig is a JSON-serializable policy override stored in executor config or metadata.
type ExecutorPolicyConfig struct {
	AllowStdio          *bool             `json:"allow_stdio,omitempty"`
	AllowHTTP           *bool             `json:"allow_http,omitempty"`
	AllowSSE            *bool             `json:"allow_sse,omitempty"`
	AllowStreamableHTTP *bool             `json:"allow_streamable_http,omitempty"`
	URLRewrite          map[string]string `json:"url_rewrite,omitempty"`
	EnvInjection        map[string]string `json:"env_injection,omitempty"`
	AllowlistServers    []string          `json:"allowlist_servers,omitempty"`
	DenylistServers     []string          `json:"denylist_servers,omitempty"`
}

// ApplyExecutorPolicy overlays a policy override onto the base policy.
// The override can be a JSON string, map, or structured config.
func ApplyExecutorPolicy(base Policy, value any) (Policy, []string, error) {
	if value == nil {
		return base, nil, nil
	}

	override, err := parseExecutorPolicy(value)
	if err != nil {
		return base, nil, err
	}
	if override == nil {
		return base, nil, nil
	}

	warnings := []string{}
	if override.AllowStdio != nil {
		base.AllowStdio = *override.AllowStdio
	}
	if override.AllowHTTP != nil {
		base.AllowHTTP = *override.AllowHTTP
	}
	if override.AllowSSE != nil {
		base.AllowSSE = *override.AllowSSE
	}
	if override.AllowStreamableHTTP != nil {
		base.AllowStreamableHTTP = *override.AllowStreamableHTTP
	}
	if override.URLRewrite != nil {
		base.URLRewrite = override.URLRewrite
	}
	if override.EnvInjection != nil {
		base.EnvInjection = override.EnvInjection
	}
	if len(override.AllowlistServers) > 0 {
		base.AllowlistServers = append([]string{}, override.AllowlistServers...)
	}
	if len(override.DenylistServers) > 0 {
		base.DenylistServers = append([]string{}, override.DenylistServers...)
	}
	if len(base.AllowlistServers) > 0 && len(base.DenylistServers) > 0 {
		warnings = append(warnings, "mcp policy: allowlist and denylist both set; allowlist takes precedence")
	}

	return base, warnings, nil
}

func parseExecutorPolicy(value any) (*ExecutorPolicyConfig, error) {
	switch v := value.(type) {
	case ExecutorPolicyConfig:
		return &v, nil
	case *ExecutorPolicyConfig:
		return v, nil
	case json.RawMessage:
		return parseExecutorPolicyJSON(v)
	case string:
		if strings.TrimSpace(v) == "" {
			return nil, nil
		}
		return parseExecutorPolicyJSON([]byte(v))
	case map[string]interface{}:
		return parseExecutorPolicyMap(v)
	default:
		return nil, fmt.Errorf("unsupported mcp policy value type %T", value)
	}
}

func parseExecutorPolicyJSON(payload []byte) (*ExecutorPolicyConfig, error) {
	var cfg ExecutorPolicyConfig
	if err := json.Unmarshal(payload, &cfg); err != nil {
		return nil, fmt.Errorf("invalid mcp policy JSON: %w", err)
	}
	return &cfg, nil
}

func parseExecutorPolicyMap(payload map[string]any) (*ExecutorPolicyConfig, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("invalid mcp policy map: %w", err)
	}
	return parseExecutorPolicyJSON(encoded)
}
