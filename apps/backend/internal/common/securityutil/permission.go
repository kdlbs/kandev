package securityutil

import (
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

const maxPermissionPresentationBytes = 4096

const permissionRedactionMask = "[redacted]"

// Display categories ProjectPermissionAction's switch understands.
const (
	permissionTypeCommand   = "command"
	permissionTypeFileWrite = "file_write"
	permissionTypeFileRead  = "file_read"
	permissionTypeNetwork   = "network"
	permissionTypeMCPTool   = "mcp_tool"
)

var permissionRedactions = []*regexp.Regexp{
	regexp.MustCompile(`sk-[A-Za-z0-9_-]{12,}`),
	regexp.MustCompile(`github_pat_[A-Za-z0-9_]{20,}`),
	regexp.MustCompile(`ghp_[A-Za-z0-9]{20,}`),
	regexp.MustCompile(`kandev_pat_[A-Za-z0-9_]+`),
	regexp.MustCompile(`(?i)Bearer\s+[A-Za-z0-9._\-+/=]{12,}`),
	regexp.MustCompile(`(?i)Authorization:\s*[^\r\n]+`),
	regexp.MustCompile(`(?i)--api-key(?:=|\s+)\S+`),
	regexp.MustCompile(`(?i)(?:password|secret|token|api[_-]?key)\s*[:=]\s*\S+`),
}

// PermissionActionProjection is the allowlisted, presentation-only view of a
// provider action. It intentionally has no generic metadata or argument map.
type PermissionActionProjection struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Command     string `json:"command,omitempty"`
	CWD         string `json:"cwd,omitempty"`
	Path        string `json:"path,omitempty"`
	Destination string `json:"destination,omitempty"`
	Server      string `json:"server,omitempty"`
	Tool        string `json:"tool,omitempty"`
	Redacted    bool   `json:"redacted"`
}

// SanitizePermissionText removes common credential shapes from provider text
// and bounds its size before it crosses a public or durable boundary.
func SanitizePermissionText(value string) (string, bool) {
	safe := value
	for _, pattern := range permissionRedactions {
		safe = pattern.ReplaceAllString(safe, permissionRedactionMask)
	}
	if len(safe) > maxPermissionPresentationBytes {
		safe = safe[:maxPermissionPresentationBytes]
		for !utf8.ValidString(safe) {
			safe = safe[:len(safe)-1]
		}
	}
	return safe, safe != value
}

// ProjectPermissionAction extracts only action-type-specific display fields.
// Unknown fields, environment values, headers, and MCP arguments are ignored.
func ProjectPermissionAction(actionType, title string, details map[string]any) PermissionActionProjection {
	projected := PermissionActionProjection{Type: normalizePermissionActionType(actionType)}
	projected.Description, projected.Redacted = sanitizePermissionField(firstString(details, "description"))
	if projected.Description == "" {
		projected.Description, projected.Redacted = sanitizePermissionField(title)
	}

	rawInput, _ := details["raw_input"].(map[string]any)
	field := func(keys ...string) string {
		if value := firstString(rawInput, keys...); value != "" {
			return value
		}
		return firstString(details, keys...)
	}

	switch projected.Type {
	case permissionTypeCommand:
		projected.Command, projected.Redacted = mergeSanitized(projected.Redacted, field("command"))
		projected.CWD, projected.Redacted = mergeSanitized(projected.Redacted, field("cwd", "workdir", "work_dir"))
	case permissionTypeFileWrite, permissionTypeFileRead:
		projected.Path, projected.Redacted = mergeSanitized(projected.Redacted, field("path", "file_path"))
	case permissionTypeNetwork:
		destination := field("destination", "url", "host")
		stripped, ok := stripURLCredentials(destination)
		if !ok {
			// Could not confidently strip embedded credentials — fail closed
			// rather than risk leaking them across the permission boundary.
			projected.Destination = ""
			projected.Redacted = true
			break
		}
		projected.Redacted = projected.Redacted || stripped != destination
		projected.Destination, projected.Redacted = mergeSanitized(projected.Redacted, stripped)
	case permissionTypeMCPTool:
		projected.Server, projected.Redacted = mergeSanitized(projected.Redacted, field("server", "server_name"))
		projected.Tool, projected.Redacted = mergeSanitized(projected.Redacted, field("tool", "tool_name", "name"))
	}

	return projected
}

// PermissionActionDetailsForEvent preserves the existing frontend detail
// shape while dropping provider-specific fields that are unsafe to stream.
func PermissionActionDetailsForEvent(action PermissionActionProjection) map[string]any {
	details := make(map[string]any)
	if action.Description != "" {
		details["description"] = action.Description
	}
	rawInput := make(map[string]any)
	if action.Command != "" {
		rawInput["command"] = action.Command
	}
	if action.CWD != "" {
		rawInput["cwd"] = action.CWD
	}
	if action.Path != "" {
		rawInput["path"] = action.Path
	}
	if action.Destination != "" {
		rawInput["destination"] = action.Destination
	}
	if action.Server != "" {
		rawInput["server"] = action.Server
	}
	if action.Tool != "" {
		rawInput["tool"] = action.Tool
	}
	if len(rawInput) > 0 {
		details["raw_input"] = rawInput
	}
	return details
}

// normalizePermissionActionType maps a permission action type onto the fixed
// set of display categories ProjectPermissionAction's switch understands. It
// accepts both the internal display names (used by non-ACP sources) and the
// raw ACP ToolKind values agentctl forwards unchanged from the agent
// (acp.RequestPermissionRequest.ToolCall.Kind — "execute", "edit", "read",
// "fetch", ...). Without this mapping every ACP-sourced request normalized to
// "other" and the switch below never populated command/cwd/path/destination.
func normalizePermissionActionType(actionType string) string {
	switch actionType {
	case permissionTypeCommand, permissionTypeFileWrite, permissionTypeFileRead, permissionTypeNetwork, permissionTypeMCPTool:
		return actionType
	case "execute":
		return permissionTypeCommand
	case "edit":
		return permissionTypeFileWrite
	case "read":
		return permissionTypeFileRead
	case "fetch":
		return permissionTypeNetwork
	default:
		return "other"
	}
}

func sanitizePermissionField(value string) (string, bool) {
	if value == "" {
		return "", false
	}
	return SanitizePermissionText(value)
}

func mergeSanitized(previouslyRedacted bool, value string) (string, bool) {
	safe, changed := sanitizePermissionField(value)
	return safe, previouslyRedacted || changed
}

func firstString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := values[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case string:
			return typed
		case []string:
			return strings.Join(typed, " ")
		case []any:
			parts := make([]string, 0, len(typed))
			for _, part := range typed {
				text, ok := part.(string)
				if !ok {
					return ""
				}
				parts = append(parts, text)
			}
			return strings.Join(parts, " ")
		}
	}
	return ""
}

// stripURLCredentials removes embedded userinfo, query, and fragment data
// from a network destination. It treats values without a scheme as network
// path references so query credentials cannot pass through unchanged. It
// reports ok=false when parsing cannot produce a host, so callers fail closed.
func stripURLCredentials(value string) (string, bool) {
	schemeless := !strings.Contains(value, "://")
	parseValue := value
	if schemeless && !strings.HasPrefix(parseValue, "//") {
		parseValue = "//" + value
	}
	parsed, err := url.Parse(parseValue)
	if err != nil {
		return "", false
	}
	if schemeless {
		if parsed.Host == "" {
			return "", false
		}
	} else if parsed.Scheme == "" || parsed.Host == "" {
		return "", false
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	stripped := parsed.String()
	if schemeless {
		return strings.TrimPrefix(stripped, "//"), true
	}
	return stripped, true
}
