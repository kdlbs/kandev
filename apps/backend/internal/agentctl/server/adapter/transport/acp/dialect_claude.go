package acp

import (
	"strings"

	"github.com/kandev/kandev/internal/agentctl/types"
)

const claudePermissionOtherActionType = "other"

// newClaudeACPDialect enables only the compatibility translations verified
// against Claude ACP frames. Other providers retain their normal identity path.
func newClaudeACPDialect() acpDialect {
	return acpDialect{permissionToolName: normalizeClaudePermissionToolName}
}

func normalizeClaudePermissionToolName(
	programmaticName *string,
	meta map[string]any,
	title string,
	actionType string,
) *string {
	metadataName, metadataPresent, metadataValid := claudePermissionMetadataName(meta)
	if !metadataValid {
		return emptyPermissionToolName()
	}
	if metadataPresent {
		if programmaticName != nil && *programmaticName != metadataName {
			return emptyPermissionToolName()
		}
		return &metadataName
	}

	// A provider-supplied programmatic name is authoritative when no Claude
	// metadata identity is present. In particular, a nonmatching name must not
	// be replaced with a matching display title.
	if programmaticName != nil {
		return programmaticName
	}

	if actionType != claudePermissionOtherActionType {
		return nil
	}
	candidate := strings.TrimSpace(title)
	server, _, ok := types.ParseQualifiedMCPToolName(candidate)
	if !ok || server != kandevMCPServerName {
		return nil
	}
	return &candidate
}

func claudePermissionMetadataName(meta map[string]any) (name string, present, valid bool) {
	claudeMeta, ok := meta["claudeCode"]
	if !ok {
		return "", false, true
	}
	fields, ok := claudeMeta.(map[string]any)
	if !ok {
		return "", false, false
	}
	rawName, ok := fields["toolName"]
	if !ok {
		return "", false, true
	}
	name, ok = rawName.(string)
	if !ok {
		return "", false, false
	}
	return name, true, true
}

func emptyPermissionToolName() *string {
	name := ""
	return &name
}
