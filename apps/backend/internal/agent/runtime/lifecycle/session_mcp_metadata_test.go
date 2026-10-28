package lifecycle

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// The token nonce and broker fingerprint are what lets adoption re-derive and
// verify a retained instance's token, so they must survive every same-session
// metadata writer and never reach a sibling session.
func TestSessionMCPMetadataKeysFollowTheSameSessionRule(t *testing.T) {
	for _, key := range []string{MetadataKeyMCPTokenNonce, MetadataKeyBrokerFingerprint} {
		t.Run(key, func(t *testing.T) {
			if !ShouldPersistMetadataKey(key) {
				t.Fatalf("%s must be carried across a same-session resume", key)
			}
			if !IsSessionScopedMetadataKey(key) {
				t.Fatalf("%s must not be copied into a sibling session", key)
			}
			got := FilterPersistentMetadata(map[string]interface{}{key: "v", "task_description": "x"})
			if got[key] != "v" || len(got) != 1 {
				t.Fatalf("filtered metadata = %v", got)
			}
		})
	}
}

func TestRunningRecordNonceComesFromTheExecutionNotAPriorRow(t *testing.T) {
	exec := &AgentExecution{ID: "e1", TaskID: "t", SessionID: "s"}
	exec.setMetadataValue(MetadataKeyMCPTokenNonce, "n-new")
	exec.setMetadataValue(MetadataKeyBrokerFingerprint, "b-new")
	prior := &models.ExecutorRunning{Metadata: map[string]interface{}{
		MetadataKeyMCPTokenNonce: "n-old", MetadataKeyBrokerFingerprint: "b-old", "context_window": 1,
	}}
	got := buildRunningFromExecution(exec, prior).Metadata
	if got[MetadataKeyMCPTokenNonce] != "n-new" || got[MetadataKeyBrokerFingerprint] != "b-new" {
		t.Fatalf("a prior row's value replaced the minted one: %v", got)
	}
	if got["context_window"] != 1 {
		t.Fatalf("orchestrator-owned prior metadata dropped: %v", got)
	}
	// An execution without a nonce must not inherit a stale one either.
	bare := buildRunningFromExecution(&AgentExecution{ID: "e2", TaskID: "t", SessionID: "s"}, prior).Metadata
	if _, ok := bare[MetadataKeyMCPTokenNonce]; ok {
		t.Fatalf("stale nonce inherited: %v", bare)
	}
}
