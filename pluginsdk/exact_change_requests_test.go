package pluginsdk

import "testing"

func TestTaskChangeRequestProtoRoundTrip(t *testing.T) {
	mergedAt := "2026-10-29T00:00:00Z"
	closedAt := "2026-10-29T01:00:00Z"
	original := TaskChangeRequest{
		ID: "change-1", WorkspaceID: "workspace-1", TaskID: "task-1",
		ProviderID: "forgejo", ProviderHost: "forge.example.test", RepositoryID: "repo-1",
		Number: 7, URL: "https://forge.example.test/pulls/7", Title: "Fix it", State: "merged",
		HeadBranch: "fix", BaseBranch: "main", CreatedAt: "2026-10-29T00:00:00Z",
		MergedAt: &mergedAt, ClosedAt: &closedAt, ResourceVersion: "sha256:version",
	}
	roundTrip := taskChangeRequestFromProto(taskChangeRequestToProto(original))
	if roundTrip.ID != original.ID || roundTrip.ProviderID != original.ProviderID ||
		roundTrip.Number != original.Number || roundTrip.State != original.State ||
		roundTrip.ResourceVersion != original.ResourceVersion ||
		roundTrip.MergedAt == nil || *roundTrip.MergedAt != mergedAt ||
		roundTrip.ClosedAt == nil || *roundTrip.ClosedAt != closedAt {
		t.Fatalf("round trip = %+v, want %+v", roundTrip, original)
	}
}
