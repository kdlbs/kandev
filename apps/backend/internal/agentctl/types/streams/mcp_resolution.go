package streams

import (
	"context"
	"strings"
	"time"
)

// MCPResolutionEntry carries catalog identity and host exclusions separately
// from executable configuration. It has no fields for values or diagnostics.
type MCPResolutionEntry struct {
	RuntimeName        string               `json:"runtime_name"`
	DefinitionID       string               `json:"definition_id,omitempty"`
	DefinitionRevision int64                `json:"definition_revision,omitempty"`
	Origins            []MCPSelectionOrigin `json:"origins,omitempty"`
	ReasonCode         string               `json:"reason_code,omitempty"`
}

type MCPSelectionOrigin struct {
	Scope       string `json:"scope"`
	WorkspaceID string `json:"workspace_id"`
	OwnerID     string `json:"owner_id"`
}

type mcpResolutionContextKey struct{}

// WithMCPResolution freezes safe metadata for one authenticated session call.
func WithMCPResolution(ctx context.Context, entries []MCPResolutionEntry) context.Context {
	return context.WithValue(ctx, mcpResolutionContextKey{}, NormalizeMCPResolution(entries))
}

func MCPResolutionFromContext(ctx context.Context) []MCPResolutionEntry {
	entries, _ := ctx.Value(mcpResolutionContextKey{}).([]MCPResolutionEntry)
	return NormalizeMCPResolution(entries)
}

// NormalizeMCPResolution bounds the authenticated host projection. Unknown
// reasons and caller-provided summaries cannot enter attachment history.
func NormalizeMCPResolution(entries []MCPResolutionEntry) []MCPResolutionEntry {
	if len(entries) > 1024 {
		entries = entries[:1024]
	}
	result := make([]MCPResolutionEntry, 0, len(entries))
	for _, entry := range entries {
		entry.RuntimeName = boundedMCPIdentity(entry.RuntimeName)
		entry.DefinitionID = boundedMCPIdentity(entry.DefinitionID)
		if entry.RuntimeName == "" {
			entry.RuntimeName = entry.DefinitionID
		}
		if entry.RuntimeName == "" {
			continue
		}
		switch entry.ReasonCode {
		case "", "definition_missing", "definition_disabled", "session_excluded", "secret_resolution_failed", "transport_policy", "server_policy", "invalid_transport", "filtered":
		default:
			continue
		}
		if entry.DefinitionRevision < 0 {
			entry.DefinitionRevision = 0
		}
		entry.Origins = normalizeMCPOrigins(entry.Origins)
		result = append(result, entry)
	}
	return result
}

func boundedMCPIdentity(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 256 {
		return ""
	}
	for _, r := range value {
		if r < 32 || r == 127 {
			return ""
		}
	}
	return value
}

func normalizeMCPOrigins(origins []MCPSelectionOrigin) []MCPSelectionOrigin {
	result := make([]MCPSelectionOrigin, 0, min(len(origins), 64))
	for _, origin := range origins {
		switch origin.Scope {
		case "profile", "repository", "task", "task_session":
		default:
			continue
		}
		origin.WorkspaceID = boundedMCPIdentity(origin.WorkspaceID)
		origin.OwnerID = boundedMCPIdentity(origin.OwnerID)
		result = append(result, origin)
		if len(result) == 64 {
			break
		}
	}
	return result
}

// MCPResolutionEvidence produces only known host exclusions. Delivered rows
// get their identity from the same request at the configuration boundary.
func MCPResolutionEvidence(attemptID string, entries []MCPResolutionEntry, at time.Time) []MCPAttachmentEvidence {
	var result []MCPAttachmentEvidence
	for _, entry := range NormalizeMCPResolution(entries) {
		if entry.ReasonCode == "" {
			continue
		}
		kind := MCPAttachmentEvidenceFiltered
		if entry.ReasonCode == "secret_resolution_failed" || entry.ReasonCode == "definition_missing" {
			kind = MCPAttachmentEvidenceUnavailable
		}
		result = append(result, MCPAttachmentEvidence{AttemptID: attemptID, ServerName: entry.RuntimeName,
			DefinitionID: entry.DefinitionID, DefinitionRevision: entry.DefinitionRevision, Origins: entry.Origins,
			Source: MCPServerSourceProfile, Kind: kind, ReasonCode: entry.ReasonCode, OccurredAt: at})
	}
	return result
}
