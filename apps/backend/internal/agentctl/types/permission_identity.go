package types

import "strings"

// ParseQualifiedMCPToolName parses an unambiguous protocol-qualified MCP tool
// name used by ACP clients, such as mcp__kandev__update_task_plan_kandev.
// The flattened format cannot identify a boundary when either side contains
// a delimiter-like underscore sequence, so those names fail closed.
func ParseQualifiedMCPToolName(name string) (server, tool string, ok bool) {
	if !strings.HasPrefix(name, "mcp__") {
		return "", "", false
	}

	qualified := strings.TrimPrefix(name, "mcp__")
	separator := strings.Index(qualified, "__")
	if !unambiguousQualifiedMCPSeparator(qualified, separator) {
		return "", "", false
	}
	server, tool = qualified[:separator], qualified[separator+2:]
	if !permissionIdentifier(server) || !permissionIdentifier(tool) {
		return "", "", false
	}
	return server, tool, true
}

func unambiguousQualifiedMCPSeparator(qualified string, separator int) bool {
	if separator <= 0 || separator+2 >= len(qualified) {
		return false
	}
	if qualified[separator-1] == '_' || qualified[separator+2] == '_' {
		return false
	}
	return !strings.Contains(qualified[separator+2:], "__")
}

func permissionIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if (char < 'a' || char > 'z') &&
			(char < 'A' || char > 'Z') &&
			(char < '0' || char > '9') &&
			char != '_' && char != '-' {
			return false
		}
	}
	return true
}
