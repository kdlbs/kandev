package streams

import "testing"

func TestMCPAttachmentHistoryTreatsModernProtocolAcceptanceAsConnected(t *testing.T) {
	history := MCPAttachmentHistory{}
	history.StartAttempt(MCPAttachmentAttempt{AttemptID: "attempt-modern"})

	if !history.Apply(MCPAttachmentEvidence{
		AttemptID:  "attempt-modern",
		ServerName: "kandev",
		Kind:       MCPAttachmentEvidenceProtocolAccepted,
	}) {
		t.Fatal("Apply() rejected modern protocol evidence")
	}

	server, ok := history.CurrentServer("kandev")
	if !ok {
		t.Fatal("CurrentServer() did not retain kandev")
	}
	if server.Status != MCPAttachmentStatusConnected {
		t.Fatalf("status = %q, want connected", server.Status)
	}
}
