package githubauth

import (
	"encoding/json"
	"slices"
	"testing"
)

// TestCredentialBrokerRequestWireFields pins the JSON contract the managed
// helper and the broker routes share. KANDEV_GITHUB_CREDENTIAL_SCOPES is a
// persisted form of this struct, so every key is a compatibility promise.
func TestCredentialBrokerRequestWireFields(t *testing.T) {
	request := CredentialBrokerRequest{
		Lease:             "lease-token",
		ReissueCapability: "capability-token",
		TaskID:            "task-1",
		SessionID:         "session-1",
		RepositoryID:      "repository-1",
		Owner:             "kdlbs",
		Repo:              "kandev",
		Host:              "github.com",
		Path:              "/kdlbs/kandev.git",
		ProviderID:        "provider-1",
		ParentProviderID:  "provider-parent-1",
	}
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal CredentialBrokerRequest: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("decode marshaled request: %v", err)
	}
	want := []string{
		"host", "lease", "owner", "parent_provider_id", "path", "provider_id",
		"reissue_capability", "repo", "repository_id", "session_id", "task_id",
	}
	got := make([]string, 0, len(fields))
	for key := range fields {
		got = append(got, key)
	}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf("wire fields = %v, want %v", got, want)
	}
}
