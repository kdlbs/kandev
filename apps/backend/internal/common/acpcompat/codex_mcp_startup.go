package acpcompat

import (
	"net/url"
	"strings"

	"github.com/google/uuid"
)

const CodexAgentID = "codex-acp"

// IsCodexMCPStartupReport recognizes the Codex ACP reporter's synthetic MCP
// startup failure call. All four protocol markers are required because a
// genuine tool call can have a similar title.
func IsCodexMCPStartupReport(agentID, toolCallID, title, kind, status string) bool {
	if agentID != CodexAgentID || kind != "other" || status != "failed" {
		return false
	}

	const (
		idPrefix    = "mcp_startup."
		titlePrefix = "mcp__"
		titleSuffix = "__startup"
	)
	if !strings.HasPrefix(toolCallID, idPrefix) || !strings.HasPrefix(title, titlePrefix) ||
		!strings.HasSuffix(title, titleSuffix) {
		return false
	}

	idRemainder := strings.TrimPrefix(toolCallID, idPrefix)
	idSeparator := strings.LastIndexByte(idRemainder, '.')
	if idSeparator <= 0 {
		return false
	}
	encodedServer := idRemainder[:idSeparator]
	reportUUID := idRemainder[idSeparator+1:]
	parsedUUID, err := uuid.Parse(reportUUID)
	if err != nil || parsedUUID.String() != reportUUID {
		return false
	}
	serverName, err := url.PathUnescape(encodedServer)
	if err != nil || serverName == "" {
		return false
	}

	titleServer := strings.TrimSuffix(strings.TrimPrefix(title, titlePrefix), titleSuffix)
	return titleServer != "" && titleServer == serverName
}
