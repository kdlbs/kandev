package streams

import (
	"encoding/json"
	"testing"
	"time"

	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

func builtinAttachmentAttempt(t *testing.T, id string) MCPAttachmentAttempt {
	t.Helper()
	manifest := &p.BuiltinManifest{Version: 1, CatalogVersion: 1, WorkspaceID: "workspace", TaskID: "task", Surface: p.SurfaceKanbanTask, Packages: []p.PackageID{p.PackageCore, p.PackageConfiguration}, Origins: map[p.PackageID]string{p.PackageCore: "required", p.PackageConfiguration: "task"}}
	encoded, err := json.Marshal(map[string]any{"attachment_attempt_id": id, "task_id": "task", "session_id": "session", "execution_id": "execution", "started_at": time.Unix(10, 0).UTC(), "builtin_packages": manifest})
	require.NoError(t, err)
	var attempt MCPAttachmentAttempt
	require.NoError(t, json.Unmarshal(encoded, &attempt))
	return attempt
}

func builtinAttachmentEvidence(t *testing.T, attemptID, serverName, packageID string, kind MCPAttachmentEvidenceKind) MCPAttachmentEvidence {
	t.Helper()
	encoded, err := json.Marshal(map[string]any{"attachment_attempt_id": attemptID, "server_name": serverName, "builtin_package": packageID, "source": "kandev", "kind": kind, "occurred_at": time.Unix(20, 0).UTC()})
	require.NoError(t, err)
	var evidence MCPAttachmentEvidence
	require.NoError(t, json.Unmarshal(encoded, &evidence))
	return evidence
}

func builtinAcceptedJSON(t *testing.T, history MCPAttachmentHistory) any {
	t.Helper()
	encoded, err := json.Marshal(history)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	return decoded["builtin_binding"]
}

// @covers AC-AGENTS-BUILTIN-MCP-004.2
func TestBuiltinAttachmentAcceptanceRequiresEverySelectedPackage(t *testing.T) {
	var history MCPAttachmentHistory
	history.StartAttempt(builtinAttachmentAttempt(t, "attempt"))
	for _, name := range []string{"kandev", "kandev-configuration"} {
		id := string(p.BuiltinPackageForRuntimeName(name))
		require.True(t, history.Apply(builtinAttachmentEvidence(t, "attempt", name, id, MCPAttachmentEvidenceConfigured)))
	}
	require.Nil(t, builtinAcceptedJSON(t, history), "configuration is not accepted process evidence")
	require.True(t, history.Apply(builtinAttachmentEvidence(t, "attempt", "kandev", "core", MCPAttachmentEvidenceSessionAccepted)))
	require.Nil(t, builtinAcceptedJSON(t, history), "Core acceptance cannot stand in for another package")
	require.True(t, history.Apply(builtinAttachmentEvidence(t, "attempt", "kandev-configuration", "configuration", MCPAttachmentEvidenceSessionAccepted)))
	accepted := builtinAcceptedJSON(t, history)
	require.NotNil(t, accepted)
	for _, server := range history.Current.Servers {
		require.Equal(t, MCPAttachmentStatusDelivered, server.Status)
		require.Nil(t, server.ConnectedAt)
		require.Nil(t, server.ToolsListedAt)
	}
	encoded, err := json.Marshal(history)
	require.NoError(t, err)
	var restored MCPAttachmentHistory
	require.NoError(t, json.Unmarshal(encoded, &restored))
	require.Equal(t, accepted, builtinAcceptedJSON(t, restored))
	restored.StartAttempt(builtinAttachmentAttempt(t, "failed-next-attempt"))
	restored.Apply(builtinAttachmentEvidence(t, "failed-next-attempt", "kandev", "core", MCPAttachmentEvidenceExplicitError))
	require.Equal(t, accepted, builtinAcceptedJSON(t, restored), "failed replacement preserves the last accepted binding")
	require.False(t, restored.Apply(builtinAttachmentEvidence(t, "attempt", "kandev", "core", MCPAttachmentEvidenceSessionAccepted)))
}

func TestBuiltinAttachmentRejectsMissingOrFalseProvenance(t *testing.T) {
	for _, id := range []string{"", "configuration", "unknown"} {
		var history MCPAttachmentHistory
		history.StartAttempt(builtinAttachmentAttempt(t, "attempt"))
		history.Apply(builtinAttachmentEvidence(t, "attempt", "kandev", id, MCPAttachmentEvidenceSessionAccepted))
		history.Apply(builtinAttachmentEvidence(t, "attempt", "kandev-configuration", "configuration", MCPAttachmentEvidenceSessionAccepted))
		require.Nil(t, builtinAcceptedJSON(t, history))
		for _, evidence := range history.Current.Evidence {
			if evidence.ServerName == "kandev" {
				require.Empty(t, evidence.BuiltinPackage)
			}
		}
	}
}

func TestBuiltinAttachmentHistoryRejectsInvalidPersistedBinding(t *testing.T) {
	var history MCPAttachmentHistory
	history.StartAttempt(builtinAttachmentAttempt(t, "attempt"))
	history.Apply(builtinAttachmentEvidence(t, "attempt", "kandev", "core", MCPAttachmentEvidenceSessionAccepted))
	history.Apply(builtinAttachmentEvidence(t, "attempt", "kandev-configuration", "configuration", MCPAttachmentEvidenceSessionAccepted))
	require.True(t, history.Valid())
	for _, mutation := range []string{"version", "session", "task"} {
		encoded, err := json.Marshal(history)
		require.NoError(t, err)
		var corrupted MCPAttachmentHistory
		require.NoError(t, json.Unmarshal(encoded, &corrupted))
		switch mutation {
		case "version":
			corrupted.BuiltinBinding.Manifest.Version = 99
		case "session":
			corrupted.BuiltinBinding.SessionID = "foreign"
		case "task":
			corrupted.BuiltinBinding.Manifest.TaskID = "foreign"
		}
		require.False(t, corrupted.Valid(), mutation)
	}
}
