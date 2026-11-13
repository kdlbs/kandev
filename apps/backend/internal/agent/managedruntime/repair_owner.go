package managedruntime

import "strings"

// RepairOwnerInput carries trusted identity evidence for one reusable-repair
// owner decision. SingleUserFallbackAllowed must be set only when the live auth
// service explicitly reports disabled authentication.
type RepairOwnerInput struct {
	ActorUserID               string
	ActorSynthetic            bool
	WorkspaceOwnerUserID      string
	SingleUserFallbackUserID  string
	SingleUserFallbackAllowed bool
}

// ResolveRepairOwner applies the canonical precedence: real authenticated
// actor, explicit workspace owner, then the permitted single-user fallback.
// An empty result disables reuse rather than sharing an owner implicitly.
func ResolveRepairOwner(input RepairOwnerInput) string {
	if !input.ActorSynthetic {
		if owner := strings.TrimSpace(input.ActorUserID); owner != "" {
			return owner
		}
	}
	if owner := strings.TrimSpace(input.WorkspaceOwnerUserID); owner != "" {
		return owner
	}
	if !input.SingleUserFallbackAllowed {
		return ""
	}
	return strings.TrimSpace(input.SingleUserFallbackUserID)
}
