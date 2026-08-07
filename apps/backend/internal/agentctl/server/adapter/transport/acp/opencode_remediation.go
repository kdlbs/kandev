package acp

import (
	"net/url"
	"strings"
)

const (
	// maxOpenCodeActionURLBytes bounds the complete validated remediation URL.
	// The allowlisted route plus a bounded workspace identifier leaves ample
	// headroom while keeping the field a bounded diagnostic.
	maxOpenCodeActionURLBytes = 256
	openCodeActionURLPrefix   = "/workspace/"
	openCodeActionURLSuffix   = "/go"
	openCodeActionURLHost     = "opencode.ai"
)

// NormalizeOpenCodeActionURL is the single allowlist validator shared by the
// OpenCode stderr parser and the structured ACP error projection. It accepts
// only `https://opencode.ai/workspace/<safe-workspace-id>/go` with an exact
// host, HTTPS scheme, no userinfo, query, fragment, percent-encoding, or port,
// a bounded workspace identifier, and a bounded complete URL. Any other input
// — including arbitrary provider URLs — returns "".
func NormalizeOpenCodeActionURL(raw string) string {
	if raw == "" || len(raw) > maxOpenCodeActionURLBytes {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if u.Scheme != "https" || u.Host != openCodeActionURLHost {
		return ""
	}
	if u.User != nil || u.Opaque != "" ||
		u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" {
		return ""
	}
	if !strings.HasPrefix(u.Path, openCodeActionURLPrefix) ||
		!strings.HasSuffix(u.Path, openCodeActionURLSuffix) {
		return ""
	}
	workspaceID := strings.TrimSuffix(strings.TrimPrefix(u.Path, openCodeActionURLPrefix), openCodeActionURLSuffix)
	if !safeOpenCodeWorkspaceID(workspaceID) {
		return ""
	}
	return "https://" + openCodeActionURLHost + openCodeActionURLPrefix + workspaceID + openCodeActionURLSuffix
}

// safeOpenCodeWorkspaceID bounds the workspace identifier to the same
// alphanumeric/underscore/hyphen charset OpenCode uses for its identifiers.
// The dot-colon-slash tolerance of safeOpenCodeField is intentionally not
// reused here so a `..`-style segment cannot pass as a workspace ID.
func safeOpenCodeWorkspaceID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

// extractOpenCodeActionURL finds the first allowlisted OpenCode action URL in
// the raw provider message, before sanitization strips URLs and identifiers.
// Strictness is intentional: a URL followed by punctuation or prose does not
// validate, and the sanitizer still removes the token from the message.
func extractOpenCodeActionURL(raw string) string {
	for _, token := range strings.Fields(raw) {
		if !strings.HasPrefix(token, "https://") {
			continue
		}
		if normalized := NormalizeOpenCodeActionURL(token); normalized != "" {
			return normalized
		}
	}
	return ""
}
