// Package acpprovider carries the OpenAI-compatible provider primitive across
// the backend/agentctl tier boundary as plain data. The backend resolves an
// agent profile's provider fields (base URL + revealed bearer key) into a
// GatewayAuth value; agentctl's ACP adapter replays it as an ACP
// `authenticate` request with methodId "gateway". Neither tier imports the
// other. See docs/specs/agents/system-design/openai-compatible-providers.md.
package acpprovider

import (
	"fmt"
	"net/url"
	"strings"
)

// GatewayAuth is the ACP `authenticate` request an agent's OpenAI-compatible
// gateway provider expects. Meta is placed verbatim into
// AuthenticateRequest._meta.
type GatewayAuth struct {
	// MethodID is the ACP auth methodId to authenticate with (e.g. "gateway").
	MethodID string `json:"method_id"`
	// Meta is the ACP `_meta` payload for the authenticate request. For the
	// gateway method it is {"gateway": {"baseUrl": ..., "headers": {...},
	// "providerName": ...}}.
	Meta map[string]any `json:"meta,omitempty"`
}

// ClientAuthMeta is the `clientCapabilities.auth._meta` a Kandev client sends in
// `initialize` so a gateway-capable agent (codex-acp >= 1.7) advertises its
// gateway auth method.
func ClientAuthMeta() map[string]any {
	return map[string]any{"gateway": true}
}

// BuildGatewayAuth assembles the authenticate params for an OpenAI-compatible
// gateway. baseURL must already be validated with ValidateBaseURL. apiKey may be
// empty, in which case no Authorization header is sent and the agent falls back
// to its own credential lookup (for example OPENAI_API_KEY in its environment).
func BuildGatewayAuth(methodID, providerName, baseURL, apiKey string) GatewayAuth {
	gateway := map[string]any{"baseUrl": baseURL}
	if providerName != "" {
		gateway["providerName"] = providerName
	}
	if apiKey != "" {
		gateway["headers"] = map[string]any{"Authorization": "Bearer " + apiKey}
	}
	return GatewayAuth{
		MethodID: methodID,
		Meta:     map[string]any{"gateway": gateway},
	}
}

// ValidateBaseURL requires a non-empty absolute http(s) URL with a host. Shared
// by save-time (settings controller) and launch-time validation so the two
// cannot drift.
func ValidateBaseURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("base URL is required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("base URL is not a valid URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("base URL must be http or https")
	}
	if u.Host == "" {
		return fmt.Errorf("base URL must include a host")
	}
	return nil
}
