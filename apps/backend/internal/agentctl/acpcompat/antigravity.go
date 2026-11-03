package acpcompat

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// AntigravityAgentID is the registry identity of Google's standalone
// Antigravity ACP server.
const AntigravityAgentID = "antigravity-acp"

const (
	// AntigravityClientName and AntigravityClientVersion are the wire identity
	// the vendor's model-discovery allowlist expects. Product records continue
	// to identify Kandev normally.
	AntigravityClientName    = "zed"
	AntigravityClientVersion = "0.0.0"

	// AntigravityPersonalAuthMethod selects the personal Google subscription.
	AntigravityPersonalAuthMethod = "oauth-personal"

	// AntigravityHumanInputPrefix marks permission requests that carry a
	// question only the user can answer.
	AntigravityHumanInputPrefix = "interaction_"

	// AntigravityBrowserHelperCommand is the private agentctl subcommand used
	// as a no-op browser so a background operation can never open consent.
	AntigravityBrowserHelperCommand = "antigravity-noop-browser"

	// AntigravityHarnessPathEnv overrides the sibling harness lookup.
	AntigravityHarnessPathEnv = "ANTIGRAVITY_HARNESS_PATH"

	// AntigravityPersonalAuthCapability is advertised by agentctl builds that
	// understand AuthPolicy and transient auth status.
	AntigravityPersonalAuthCapability = "antigravity_personal_auth_v1"
)

// AuthPolicy is the internal child-process authentication contract. It is
// never persisted as a profile option.
type AuthPolicy struct {
	MethodID         string `json:"method_id"`
	AllowInteractive bool   `json:"allow_interactive,omitempty"`
	AttemptID        string `json:"attempt_id,omitempty"`
}

// ErrAuthRequired marks an operation that needs an interactive Google sign-in.
var ErrAuthRequired = errors.New("antigravity authentication required")

// ErrInvalidHarnessOverride marks a harness path that is not a usable regular
// executable after environment merge.
var ErrInvalidHarnessOverride = errors.New("antigravity harness override is invalid")

// AuthRequiredError carries a sanitized cause and, only for the active
// operator attempt, the transient consent URL.
type AuthRequiredError struct {
	MethodID   string
	Message    string
	ConsentURL string
}

func (e *AuthRequiredError) Error() string {
	if e == nil {
		return ErrAuthRequired.Error()
	}
	if e.Message != "" {
		return e.Message
	}
	return ErrAuthRequired.Error()
}

func (e *AuthRequiredError) Unwrap() error { return ErrAuthRequired }

// AntigravityAuthPolicy returns the policy for an eligible local Antigravity
// launch. Interactive permission is reserved for the dedicated operator setup
// instance.
func AntigravityAuthPolicy(allowInteractive bool) *AuthPolicy {
	return &AuthPolicy{
		MethodID:         AntigravityPersonalAuthMethod,
		AllowInteractive: allowInteractive,
	}
}

// IsAntigravity reports whether agentID names the Antigravity ACP agent.
func IsAntigravity(agentID string) bool {
	return agentID == AntigravityAgentID
}

// ClientInfoForAgent returns the ACP clientInfo identity for an agent. Only
// Antigravity overrides the canonical Kandev identity.
func ClientInfoForAgent(agentID string) (name, version string) {
	if IsAntigravity(agentID) {
		return AntigravityClientName, AntigravityClientVersion
	}
	return "", ""
}

// IsHumanInputPermission reports whether an ACP permission request is an
// Antigravity native question. Recognition is limited to the provider's
// tool-call prefix; prose and option counts are never inspected.
func IsHumanInputPermission(agentID, toolCallID string) bool {
	return IsAntigravity(agentID) && strings.HasPrefix(toolCallID, AntigravityHumanInputPrefix)
}

// HasAuthMethod reports whether methods advertises the personal OAuth method.
func HasAuthMethod(methods []string) bool {
	return slices.Contains(methods, AntigravityPersonalAuthMethod)
}

// AntigravityCredentialEnvNames lists environment inputs that select a
// non-personal credential mode. They are removed only from the Antigravity
// child process.
func AntigravityCredentialEnvNames() []string {
	return []string{
		"GEMINI_API_KEY",
		"GOOGLE_API_KEY",
		"GOOGLE_APPLICATION_CREDENTIALS",
		"GOOGLE_CLOUD_PROJECT",
		"GOOGLE_CLOUD_LOCATION",
		"GOOGLE_CLOUD_QUOTA_PROJECT",
		"GOOGLE_GENAI_USE_VERTEXAI",
		"AGY_ACP_CCPA_PROJECT",
		"AGY_ACP_ENABLE_OAUTH",
	}
}

// ValidateLocalAntigravityEnvironment rejects profile inputs that would create
// a task-specific native store. The expectedHome is the host user's home.
func ValidateLocalAntigravityEnvironment(env map[string]string, expectedHome string) error {
	if raw, ok := env["HOME"]; ok {
		if raw == "" || !samePath(raw, expectedHome) {
			return fmt.Errorf("antigravity personal subscription requires the shared user home %q, got HOME=%q", expectedHome, raw)
		}
	}
	if raw := strings.TrimSpace(env["GEMINI_HOME"]); raw != "" {
		return fmt.Errorf("antigravity personal subscription does not support GEMINI_HOME=%q; remove it to use the shared native store", raw)
	}
	return nil
}

func samePath(a, b string) bool {
	aa, errA := filepath.Abs(a)
	bb, errB := filepath.Abs(b)
	if errA != nil || errB != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(aa), filepath.Clean(bb))
	}
	return filepath.Clean(aa) == filepath.Clean(bb)
}

// ValidateLocalAntigravityEnvSlice validates the final ambient-plus-profile
// environment before a local Antigravity child starts.
func ValidateLocalAntigravityEnvSlice(env []string, expectedHome string) error {
	values := make(map[string]string, len(env))
	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			values[key] = value
		}
	}
	return ValidateLocalAntigravityEnvironment(values, expectedHome)
}

// PrepareLocalAntigravityEnv applies the local personal-subscription policy to
// a final child environment. It validates the shared home, removes conflicting
// credential-mode inputs, pins the validated harness, and replaces BROWSER with
// the private no-op helper. Unrelated variables are preserved.
func PrepareLocalAntigravityEnv(env []string, harnessPath string) ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil, fmt.Errorf("resolve the shared Antigravity home: %w", err)
	}
	if err := ValidateLocalAntigravityEnvSlice(env, home); err != nil {
		return nil, err
	}
	out := append([]string(nil), env...)
	for _, key := range AntigravityCredentialEnvNames() {
		out = removeEnvEntry(out, key)
	}
	out = removeEnvEntry(out, AntigravityHarnessPathEnv)
	if harnessPath != "" {
		info, err := os.Stat(harnessPath)
		if err != nil || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("%w: %s is not a regular executable", ErrInvalidHarnessOverride, harnessPath)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
			return nil, fmt.Errorf("%w: %s is not executable", ErrInvalidHarnessOverride, harnessPath)
		}
		out = append(out, AntigravityHarnessPathEnv+"="+harnessPath)
	}
	exe, err := AgentctlExecutableForBrowser()
	if err != nil {
		return nil, err
	}
	out = removeEnvEntry(out, "BROWSER")
	out = append(out, "BROWSER="+BrowserEnvValue(exe))
	return out, nil
}

func removeEnvEntry(env []string, key string) []string {
	prefix := key + "="
	next := make([]string, 0, len(env))
	for _, entry := range env {
		if !strings.HasPrefix(entry, prefix) {
			next = append(next, entry)
		}
	}
	return next
}

// EnvValue returns the last value for key in a final environment slice.
func EnvValue(env []string, key string) string {
	prefix := key + "="
	value := ""
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			value = strings.TrimPrefix(entry, prefix)
		}
	}
	return value
}

// MergeEnvMap overlays values on a final environment slice, removing prior
// entries for the same key.
func MergeEnvMap(env []string, values map[string]string) []string {
	out := append([]string(nil), env...)
	for key, value := range values {
		out = removeEnvEntry(out, key)
		out = append(out, key+"="+value)
	}
	return out
}

// RedactConsentURLs replaces any Google OAuth consent URL, including a
// malformed one, with a fixed marker. It never returns query values.
func RedactConsentURLs(text string) string {
	const marker = "[antigravity consent URL redacted]"
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return text
	}
	for _, field := range fields {
		candidate := strings.Trim(field, "\"'<>()[]{}.,;")
		if !strings.Contains(candidate, "accounts.google.com") && !strings.Contains(candidate, "/o/oauth2/") {
			continue
		}
		text = strings.ReplaceAll(text, candidate, marker)
	}
	return text
}

// ParseConsentURL extracts the first Google consent URL from a raw stderr
// line. Parsing happens before sanitization so query parameters survive.
func ParseConsentURL(line string) (string, bool) {
	for _, field := range strings.Fields(line) {
		candidate := strings.Trim(field, "\"'<>()[]{}.,;")
		if !strings.HasPrefix(candidate, "https://") {
			continue
		}
		if _, err := ValidateConsentURL(candidate); err == nil {
			return candidate, true
		}
	}
	return "", false
}

// ValidateConsentURL accepts only Google HTTPS authorization URLs whose
// redirect is an ephemeral loopback listener.
func ValidateConsentURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse consent URL: %w", err)
	}
	if u.Scheme != "https" {
		return nil, errors.New("consent URL must use https")
	}
	if !strings.EqualFold(u.Hostname(), "accounts.google.com") {
		return nil, errors.New("consent URL host must be accounts.google.com")
	}
	if u.User != nil {
		return nil, errors.New("consent URL must not contain userinfo")
	}
	if u.Fragment != "" {
		return nil, errors.New("consent URL must not contain a fragment")
	}
	if !strings.HasPrefix(u.Path, "/o/oauth2/") {
		return nil, errors.New("consent URL path is not a Google OAuth authorization path")
	}
	if err := validateConsentQuery(u.Query()); err != nil {
		return nil, err
	}
	return u, nil
}

func validateConsentQuery(query url.Values) error {
	if query.Get("client_id") == "" || query.Get("state") == "" || query.Get("code_challenge") == "" {
		return errors.New("consent URL is missing client_id, state, or code_challenge")
	}
	redirect := query.Get("redirect_uri")
	if redirect == "" {
		return errors.New("consent URL is missing redirect_uri")
	}
	return validateConsentRedirect(redirect)
}

func validateConsentRedirect(raw string) error {
	redirectURL, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("parse consent redirect: %w", err)
	}
	if redirectURL.Scheme != "http" {
		return errors.New("consent redirect must use http loopback")
	}
	if !isLoopbackHost(redirectURL.Hostname()) {
		return errors.New("consent redirect must target a loopback host")
	}
	port := redirectURL.Port()
	if port == "" {
		return errors.New("consent redirect must name an ephemeral port")
	}
	portNumber, err := net.LookupPort("tcp", port)
	if err != nil || portNumber < 1024 || portNumber > 65535 {
		return errors.New("consent redirect port must be an unprivileged ephemeral port")
	}
	return nil
}

func isLoopbackHost(host string) bool {
	switch strings.ToLower(host) {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}

// BrowserEnvValue builds a Python webbrowser BROWSER entry that runs the
// private no-op helper instead of launching a browser. The %s placeholder is
// required by Python's webbrowser module.
func BrowserEnvValue(agentctlPath string) string {
	escaped := strings.ReplaceAll(agentctlPath, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `" ` + AntigravityBrowserHelperCommand + " %s"
}

// IsAntigravityNoopBrowserInvocation reports whether argv names the private
// helper subcommand.
func IsAntigravityNoopBrowserInvocation(args []string) bool {
	return len(args) >= 2 && args[1] == AntigravityBrowserHelperCommand
}

// AgentctlExecutableForBrowser resolves the current agentctl executable for
// BROWSER injection.
func AgentctlExecutableForBrowser() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve agentctl executable: %w", err)
	}
	return filepath.Abs(exe)
}
