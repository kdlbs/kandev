package sqlite

import "testing"

// The abandon marker is a decision to end the launch; a later checkpoint from a
// writer that did not observe it must not clear it.
func TestAbandonMarkerSurvivesALaterCheckpointThatDidNotCarryIt(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedProvisionedForClaims(t, repo, seedDescriptorEnvironmentSQLite)
	_, revision := storedEnvelope(t, repo)
	marked := descriptorFixture(1, "install-a", "provisioned", "/srv/work", "worker-1")
	envelope, _ := marked.Metadata["plugin_executor"].(map[string]interface{})
	envelope["abandon_reason"] = "publish_refused"
	if err := checkpointDescriptor(t, repo, marked, revision); err != nil {
		t.Fatalf("checkpoint with the marker: %v", err)
	}
	stored, revision := storedEnvelope(t, repo)
	if stored["abandon_reason"] != "publish_refused" {
		t.Fatalf("marker not stored: %v", stored["abandon_reason"])
	}
	if err := checkpointDescriptor(t, repo, descriptorFixture(1, "install-a", "provisioned", "/srv/work", "worker-1"), revision); err != nil {
		t.Fatalf("checkpoint without the marker: %v", err)
	}
	if stored, _ := storedEnvelope(t, repo); stored["abandon_reason"] != "publish_refused" {
		t.Fatalf("a checkpoint that did not carry the marker cleared it: %v", stored["abandon_reason"])
	}
}
