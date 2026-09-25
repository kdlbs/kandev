// Package acpprovider carries the OpenAI-compatible provider primitive across
// the backend/agentctl tier boundary as plain data. The backend resolves an
// agent profile's provider fields (base URL + revealed bearer key) into a
// GatewayAuth value; agentctl's ACP adapter replays it as an ACP
// `authenticate` request with methodId "gateway". Neither tier imports the
// other. See docs/specs/agents/system-design/openai-compatible-providers.md.
package acpprovider

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// DockerHostGatewayHost is the hostname a container uses to reach a service
// bound to the Docker host's loopback interface. Docker Desktop resolves it
// automatically; on Linux the agent container is created with
// `--add-host host.docker.internal:host-gateway`.
const DockerHostGatewayHost = "host.docker.internal"

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

// ValidateCredentialedBaseURL is ValidateBaseURL plus a transport-safety rule
// for a gateway that carries a bearer credential: cleartext http:// is allowed
// only to a loopback host, so the Authorization header is never sent in the
// clear over the network. https is always allowed. Kandev enforces this on the
// configured URL only; how the agent's own HTTP client follows redirects is out
// of scope here.
func ValidateCredentialedBaseURL(raw string) error {
	if err := ValidateBaseURL(raw); err != nil {
		return err
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("base URL is not a valid URL")
	}
	if u.Scheme == "http" && !isLoopbackHostname(u.Hostname()) {
		return fmt.Errorf("base URL must use https when an API key is set (http is allowed only for a localhost provider)")
	}
	return nil
}

// IsLoopbackBaseURL reports whether raw points at the local loopback interface
// ("localhost", 127.0.0.0/8, or ::1). Such a URL is unreachable from an agent
// running inside a container: the container's own loopback is not the host's.
func IsLoopbackBaseURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return isLoopbackHostname(u.Hostname())
}

// RewriteLoopbackHostForDocker swaps a loopback hostname in raw for
// host.docker.internal, keeping scheme, port, and path. The second return value
// is false (and raw is returned unchanged) when the host is not loopback or raw
// does not parse. Use it when the target agent runs in a local Docker container
// and the provider is expected to be a service on the developer's host.
func RewriteLoopbackHostForDocker(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	u, err := url.Parse(trimmed)
	if err != nil || !isLoopbackHostname(u.Hostname()) {
		return raw, false
	}
	if port := u.Port(); port != "" {
		u.Host = net.JoinHostPort(DockerHostGatewayHost, port)
	} else {
		u.Host = DockerHostGatewayHost
	}
	return u.String(), true
}

func isLoopbackHostname(host string) bool {
	host = strings.TrimSpace(host)
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}
