package instances

import (
	"context"
	"testing"
	"time"
)

func TestRemoveArtifactIfUnreferencedRechecksReleaseOwnership(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if err := store.Create(ctx, Instance{
		ID: "instance-1", PluginID: "canvas-board", SourceKind: SourceLocalCanvas,
		ScopeKind: ScopeWorkspace, WorkspaceID: "workspace-1", Status: StatusActive,
	}); err != nil {
		t.Fatalf("create instance: %v", err)
	}
	if err := store.AddCleanupJob(ctx, CleanupJob{ID: "cleanup-1", WorkspaceID: "workspace-1", InstanceID: "instance-1", ArtifactPath: "releases/shared"}); err != nil {
		t.Fatalf("add cleanup job: %v", err)
	}
	job, claimed, err := store.ClaimCleanupJob(ctx, nowForCleanupTest())
	if err != nil || !claimed {
		t.Fatalf("claim cleanup job = %#v, %v, want claimed", job, err)
	}
	if err := store.CreateRelease(ctx, Release{
		ID: "release-1", PluginID: "canvas-board", InstanceID: "instance-1", PackageDigest: "digest-1",
		SourceKind: SourceLocalCanvas, ArtifactPath: "releases/shared", ValidationStatus: ValidationValid,
	}); err != nil {
		t.Fatalf("republish artifact: %v", err)
	}
	removed := false
	if got, err := store.RemoveArtifactIfUnreferenced(ctx, job.ID, job.ArtifactPath, func() error {
		removed = true
		return nil
	}); err != nil {
		t.Fatalf("remove artifact: %v", err)
	} else if got {
		t.Fatal("remove artifact reported removal for a referenced path")
	}
	if removed {
		t.Fatal("remove callback ran for a referenced path")
	}
	jobs, err := store.ListCleanupJobs(ctx)
	if err != nil {
		t.Fatalf("list cleanup jobs: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Status != CleanupCompleted {
		t.Fatalf("cleanup jobs = %#v, want completed job", jobs)
	}
}

func nowForCleanupTest() (now time.Time) {
	return time.Now().UTC().Add(time.Second)
}
