package streams

import (
	"strings"
	"testing"
	"time"
)

func TestMCPAttachmentHistoryUsesStrongestEvidenceForServerStatus(t *testing.T) {
	history := MCPAttachmentHistory{}
	history.StartAttempt(MCPAttachmentAttempt{AttemptID: "attempt-1", StartedAt: time.Unix(1, 0).UTC()})

	for _, tc := range []struct {
		name string
		kind MCPAttachmentEvidenceKind
		want MCPAttachmentStatus
	}{
		{name: "filtered", kind: MCPAttachmentEvidenceFiltered, want: MCPAttachmentStatusFiltered},
		{name: "delivered", kind: MCPAttachmentEvidenceDelivered, want: MCPAttachmentStatusDelivered},
		{name: "connected", kind: MCPAttachmentEvidenceInitializeObserved, want: MCPAttachmentStatusConnected},
		{name: "active", kind: MCPAttachmentEvidenceToolsListObserved, want: MCPAttachmentStatusActive},
		{name: "failed", kind: MCPAttachmentEvidenceExplicitError, want: MCPAttachmentStatusFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate := MCPAttachmentHistory{}
			candidate.StartAttempt(MCPAttachmentAttempt{AttemptID: tc.name})
			if !candidate.Apply(MCPAttachmentEvidence{AttemptID: tc.name, ServerName: "kandev", Kind: tc.kind}) {
				t.Fatal("Apply() rejected current evidence")
			}
			server, ok := candidate.CurrentServer("kandev")
			if !ok {
				t.Fatal("CurrentServer() did not retain server")
			}
			if server.Status != tc.want {
				t.Fatalf("status = %q, want %q", server.Status, tc.want)
			}
		})
	}

	if !history.Apply(MCPAttachmentEvidence{AttemptID: "attempt-1", ServerName: "kandev", Kind: MCPAttachmentEvidenceToolsListObserved}) {
		t.Fatal("Apply() rejected tools-list evidence")
	}
	if !history.Apply(MCPAttachmentEvidence{AttemptID: "attempt-1", ServerName: "kandev", Kind: MCPAttachmentEvidenceExplicitError}) {
		t.Fatal("Apply() rejected explicit-error evidence")
	}
	server, ok := history.CurrentServer("kandev")
	if !ok || server.Status != MCPAttachmentStatusFailed {
		t.Fatalf("later explicit error did not win: %+v", server)
	}
}

func TestMCPAttachmentHistoryDoesNotRestoreFailedServerFromLateObservations(t *testing.T) {
	history := MCPAttachmentHistory{}
	history.StartAttempt(MCPAttachmentAttempt{AttemptID: "attempt-1"})

	for _, evidence := range []MCPAttachmentEvidence{
		{AttemptID: "attempt-1", ServerName: "kandev", Kind: MCPAttachmentEvidenceExplicitError},
		{AttemptID: "attempt-1", ServerName: "kandev", Kind: MCPAttachmentEvidenceInitializeObserved},
		{AttemptID: "attempt-1", ServerName: "kandev", Kind: MCPAttachmentEvidenceToolsListObserved, ToolCount: 2},
	} {
		if !history.Apply(evidence) {
			t.Fatal("Apply() rejected current evidence")
		}
	}

	server, ok := history.CurrentServer("kandev")
	if !ok || server.Status != MCPAttachmentStatusFailed {
		t.Fatalf("server = %+v, want failed", server)
	}
}

func TestMCPServerAttachmentConnectedAtIsWriteOnce(t *testing.T) {
	history := MCPAttachmentHistory{}
	history.StartAttempt(MCPAttachmentAttempt{AttemptID: "attempt-1"})
	first := time.Unix(10, 0).UTC()
	second := time.Unix(20, 0).UTC()

	for _, when := range []time.Time{first, second} {
		if !history.Apply(MCPAttachmentEvidence{
			AttemptID:  "attempt-1",
			ServerName: "kandev",
			Kind:       MCPAttachmentEvidenceProtocolAccepted,
			OccurredAt: when,
		}) {
			t.Fatal("Apply() rejected protocol acceptance evidence")
		}
	}

	server, ok := history.CurrentServer("kandev")
	if !ok || server.ConnectedAt == nil {
		t.Fatalf("server = %+v, want connected timestamp", server)
	}
	if !server.ConnectedAt.Equal(first) {
		t.Fatalf("connected_at = %v, want first acceptance %v", server.ConnectedAt, first)
	}
}

func TestMCPAttachmentHistoryUpdatesToolCountToZero(t *testing.T) {
	history := MCPAttachmentHistory{}
	history.StartAttempt(MCPAttachmentAttempt{AttemptID: "attempt-1"})
	for _, toolCount := range []int{2, 0} {
		if !history.Apply(MCPAttachmentEvidence{
			AttemptID: "attempt-1", ServerName: "kandev", Kind: MCPAttachmentEvidenceToolsListObserved, ToolCount: toolCount,
		}) {
			t.Fatal("Apply() rejected current evidence")
		}
	}

	server, ok := history.CurrentServer("kandev")
	if !ok || server.ToolCount != 0 {
		t.Fatalf("server = %+v, want tool_count 0", server)
	}
}

func TestMCPAttachmentHistoryConnectionClosedOnlyClearsMatchingConnection(t *testing.T) {
	history := MCPAttachmentHistory{}
	history.StartAttempt(MCPAttachmentAttempt{AttemptID: "attempt-1"})
	history.Apply(MCPAttachmentEvidence{AttemptID: "attempt-1", ServerName: "kandev", Kind: MCPAttachmentEvidenceToolsListObserved, ConnectionID: "new"})
	history.Apply(MCPAttachmentEvidence{AttemptID: "attempt-1", ServerName: "kandev", Kind: MCPAttachmentEvidenceConnectionClosed, ConnectionID: "old"})

	server, _ := history.CurrentServer("kandev")
	if server.Status != MCPAttachmentStatusActive || server.ConnectionID != "new" || server.DisconnectedAt != nil {
		t.Fatalf("old connection closure changed current server: %+v", server)
	}

	history.Apply(MCPAttachmentEvidence{AttemptID: "attempt-1", ServerName: "kandev", Kind: MCPAttachmentEvidenceConnectionClosed, ConnectionID: "new"})
	server, _ = history.CurrentServer("kandev")
	if server.Status != MCPAttachmentStatusConnected || server.DisconnectedAt == nil {
		t.Fatalf("matching connection closure did not downgrade server: %+v", server)
	}
}

func TestMCPAttachmentHistorySupersedesEarlierAttemptInSameExecution(t *testing.T) {
	history := MCPAttachmentHistory{}
	history.StartAttempt(MCPAttachmentAttempt{AttemptID: "attempt-1", ExecutionID: "execution-1"})
	if !history.Apply(MCPAttachmentEvidence{AttemptID: "attempt-1", ServerName: "kandev", Kind: MCPAttachmentEvidenceToolsListObserved}) {
		t.Fatal("Apply() rejected first attempt")
	}

	history.StartAttempt(MCPAttachmentAttempt{AttemptID: "attempt-2", ExecutionID: "execution-1"})
	if history.Current.AttemptID != "attempt-2" {
		t.Fatalf("current attempt = %q, want attempt-2", history.Current.AttemptID)
	}
	if _, ok := history.CurrentServer("kandev"); ok {
		t.Fatal("new attempt inherited active server evidence")
	}
	if len(history.Previous) != 1 || history.Previous[0].AttemptID != "attempt-1" {
		t.Fatalf("previous attempts = %+v, want attempt-1", history.Previous)
	}
	if history.Previous[0].SupersededAt == nil {
		t.Fatal("previous attempt was not marked superseded")
	}
	if history.Apply(MCPAttachmentEvidence{AttemptID: "attempt-1", ServerName: "kandev", Kind: MCPAttachmentEvidenceToolsListObserved}) {
		t.Fatal("Apply() accepted evidence for superseded attempt")
	}
}

func TestMCPAttachmentHistoryBoundsAttemptsAndEvidence(t *testing.T) {
	history := MCPAttachmentHistory{}
	for i := 0; i < MaxMCPAttachmentAttempts+1; i++ {
		attemptID := string(rune('a' + i))
		history.StartAttempt(MCPAttachmentAttempt{AttemptID: attemptID})
	}
	if len(history.Previous) != MaxMCPAttachmentAttempts-1 {
		t.Fatalf("previous count = %d, want %d", len(history.Previous), MaxMCPAttachmentAttempts-1)
	}
	if history.Previous[0].AttemptID != "c" || history.Previous[1].AttemptID != "b" {
		t.Fatalf("previous retention = %+v, want c then b", history.Previous)
	}

	for i := 0; i < MaxMCPAttachmentEvidenceEvents+1; i++ {
		history.Apply(MCPAttachmentEvidence{AttemptID: history.Current.AttemptID, ServerName: "kandev", Kind: MCPAttachmentEvidenceDelivered})
	}
	if len(history.Current.Evidence) != MaxMCPAttachmentEvidenceEvents {
		t.Fatalf("evidence count = %d, want %d", len(history.Current.Evidence), MaxMCPAttachmentEvidenceEvents)
	}
}

func TestMCPAttachmentSanitizersExcludeSensitiveTargetDetails(t *testing.T) {
	if got, want := SanitizeMCPNetworkTarget("https://alice:secret@example.test:8443/mcp?token=abc#details"), "https://example.test:8443"; got != want {
		t.Fatalf("SanitizeMCPNetworkTarget() = %q, want %q", got, want)
	}
	if got, want := SanitizeMCPStdioTarget("/opt/tools/private-server --token=super-secret"), "private-server"; got != want {
		t.Fatalf("SanitizeMCPStdioTarget() = %q, want %q", got, want)
	}

	summary := SanitizeMCPErrorSummary("connection to https://alice:secret@example.test/mcp?token=abc failed: Bearer token-value-that-must-not-leak")
	for _, forbidden := range []string{"alice", "secret", "/mcp", "token=", "token-value-that-must-not-leak"} {
		if strings.Contains(summary, forbidden) {
			t.Fatalf("SanitizeMCPErrorSummary() leaked %q in %q", forbidden, summary)
		}
	}
	if len(SanitizeMCPErrorSummary(strings.Repeat("x", MaxMCPAttachmentErrorSummaryBytes+1))) > MaxMCPAttachmentErrorSummaryBytes {
		t.Fatal("SanitizeMCPErrorSummary() exceeded maximum bytes")
	}
}

func TestSanitizeMCPErrorSummaryRedactsLabeledConfiguration(t *testing.T) {
	summary := SanitizeMCPErrorSummary("failed: env=API_KEY=short-secret args=--token short-token")
	for _, forbidden := range []string{"API_KEY", "short-secret", "--token", "short-token"} {
		if strings.Contains(summary, forbidden) {
			t.Fatalf("SanitizeMCPErrorSummary() leaked %q in %q", forbidden, summary)
		}
	}
}

func TestSanitizeMCPErrorSummaryRedactsAbsolutePaths(t *testing.T) {
	summary := SanitizeMCPErrorSummary("cannot read /workspace/.env while connecting to https://endpoint.example.test/mcp")
	for _, forbidden := range []string{"/workspace/.env", "/mcp"} {
		if strings.Contains(summary, forbidden) {
			t.Fatalf("SanitizeMCPErrorSummary() leaked %q in %q", forbidden, summary)
		}
	}
	if !strings.Contains(summary, "https://endpoint.example.test") {
		t.Fatalf("SanitizeMCPErrorSummary() removed safe endpoint host: %q", summary)
	}
}
