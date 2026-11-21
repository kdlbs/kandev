package plugins

import (
	"context"
	"testing"

	"github.com/kdlbs/kandev/pluginsdk"
)

func TestExactChangeRequestProviderHostQualifiesIdentity(t *testing.T) {
	host := newExactChangeRequestHost(t, []string{"forgejo"})
	manager, ok := pluginsdk.HostTaskChangeRequests(host)
	if !ok {
		t.Fatal("plugin Host does not expose exact task change requests")
	}
	ctx := context.Background()
	digest := changeRequestManifestDigest(t, host)
	report := func(requestID, providerHost, url string) (*pluginsdk.CommandResult, pluginsdk.TaskChangeRequest) {
		t.Helper()
		result, change, err := manager.Report(ctx, pluginsdk.ExactTaskChangeRequestReport{
			RequestID: requestID, WorkspaceID: "workspace-change-requests", TaskID: "task-1",
			ProviderID: "forgejo", ProviderHost: providerHost, RepositoryID: "repo-1", Number: 4,
			URL: url, State: "open", IdempotencyKey: requestID, ApprovalRevision: 1, ManifestDigest: digest,
		})
		if err != nil {
			t.Fatalf("Report %s: %v", requestID, err)
		}
		return result, change
	}

	// The host comes from the explicit field (canonicalized) or, without one,
	// from the change request URL; the two forges stay two rows.
	first, firstChange := report("report-a", "Forge-A.example.test", "https://forge-a.example.test/o/r/pulls/4")
	second, secondChange := report("report-b", "", "https://forge-b.example.test/o/r/pulls/4")
	if first.Status != pluginsdk.CommandApplied || second.Status != pluginsdk.CommandApplied {
		t.Fatalf("reports = %+v / %+v, want APPLIED", first, second)
	}
	if firstChange.ProviderHost != "forge-a.example.test" || secondChange.ProviderHost != "forge-b.example.test" ||
		firstChange.ID == secondChange.ID {
		t.Fatalf("changes = %+v / %+v, want two host-qualified rows", firstChange, secondChange)
	}

	// Replaying the first report resolves the row its receipt names, not the
	// other host's row with the same provider, repository, and number.
	replayed, replayedChange := report("report-a", "Forge-A.example.test", "https://forge-a.example.test/o/r/pulls/4")
	if replayed.Status != pluginsdk.CommandApplied || replayedChange.ID != firstChange.ID {
		t.Fatalf("replay = %+v change:%+v, want receipt row %s", replayed, replayedChange, firstChange.ID)
	}

	// A host-less remove names two objects: it is a conflict and deletes nothing.
	removed, err := manager.Remove(ctx, pluginsdk.ExactTaskChangeRequestRemove{
		RequestID: "remove-1", WorkspaceID: "workspace-change-requests",
		ProviderID: "forgejo", RepositoryID: "repo-1", Number: 4,
		IdempotencyKey: "remove-1", ApprovalRevision: 1, ManifestDigest: digest,
	})
	if err != nil || removed == nil || removed.Status != pluginsdk.CommandConflict || removed.Reason != "change_request_identity_ambiguous" {
		t.Fatalf("Remove = %+v err:%v, want CONFLICT change_request_identity_ambiguous", removed, err)
	}
	rows, err := host.changeRequestLedger().ListChangeRequestsByTask(ctx, host.installationID, "workspace-change-requests", "task-1")
	if err != nil || len(rows) != 2 {
		t.Fatalf("rows after ambiguous remove = %+v err:%v, want both kept", rows, err)
	}

	// A host-less report without a URL is ambiguous the same way.
	ambiguous, _ := report("report-hostless", "", "")
	if ambiguous.Status != pluginsdk.CommandConflict || ambiguous.Reason != "change_request_identity_ambiguous" {
		t.Fatalf("host-less report = %+v, want CONFLICT change_request_identity_ambiguous", ambiguous)
	}
}

func TestExactChangeRequestRejectsInvalidProviderHost(t *testing.T) {
	host := newExactChangeRequestHost(t, []string{"forgejo"})
	manager, ok := pluginsdk.HostTaskChangeRequests(host)
	if !ok {
		t.Fatal("plugin Host does not expose exact task change requests")
	}
	result, _, err := manager.Report(context.Background(), pluginsdk.ExactTaskChangeRequestReport{
		RequestID: "report-invalid-host", WorkspaceID: "workspace-change-requests", TaskID: "task-1",
		ProviderID: "forgejo", ProviderHost: "forge.example.test/path", RepositoryID: "repo-1", Number: 4,
		State: "open", IdempotencyKey: "report-invalid-host", ApprovalRevision: 1,
		ManifestDigest: changeRequestManifestDigest(t, host),
	})
	if err != nil || result == nil || result.Status != pluginsdk.CommandInvalid {
		t.Fatalf("Report with path in provider host = %+v err:%v, want INVALID", result, err)
	}
}
