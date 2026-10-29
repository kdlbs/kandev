package streams

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestMCPResolutionProjectionIsBoundedAndFrozen(t *testing.T) {
	entries := []MCPResolutionEntry{{RuntimeName: "protected", DefinitionID: "definition", DefinitionRevision: 2, ReasonCode: "secret_resolution_failed", Origins: []MCPSelectionOrigin{{Scope: "task_session", WorkspaceID: "workspace", OwnerID: "session"}}}}
	ctx := WithMCPResolution(context.Background(), entries)
	entries[0].Origins[0].OwnerID = "newer"
	frozen := MCPResolutionFromContext(ctx)
	if frozen[0].Origins[0].OwnerID != "session" {
		t.Fatal("lost or aliased session origin")
	}
	evidence := MCPResolutionEvidence("attempt-a", frozen, time.Now())
	history := MCPAttachmentHistory{}
	history.StartAttempt(MCPAttachmentAttempt{AttemptID: "attempt-b"})
	if history.Apply(evidence[0]) {
		t.Fatal("old resolution changed a newer attachment")
	}
	if got := NormalizeMCPResolution([]MCPResolutionEntry{{RuntimeName: strings.Repeat("x", 257)}, {RuntimeName: "unknown", ReasonCode: "untrusted reason"}}); len(got) != 0 {
		t.Fatal("unbounded identity or unknown reason was accepted")
	}
}

// @covers AC-AGENTS-SESSION-MCP-EXCLUSIONS-002.6
func TestMCPResolutionSessionExclusionEvidence(t *testing.T) {
	entries := []MCPResolutionEntry{{RuntimeName: "docs", DefinitionID: "definition", ReasonCode: "session_excluded", Origins: []MCPSelectionOrigin{{Scope: "repository", WorkspaceID: "workspace", OwnerID: "repo"}}}}
	frozen := MCPResolutionFromContext(WithMCPResolution(context.Background(), entries))
	evidence := MCPResolutionEvidence("attempt", frozen, time.Now())
	if len(evidence) != 1 || evidence[0].ReasonCode != "session_excluded" || evidence[0].Kind != MCPAttachmentEvidenceFiltered || len(evidence[0].Origins) != 1 {
		t.Fatalf("session exclusion evidence was lost: %#v", evidence)
	}
}
