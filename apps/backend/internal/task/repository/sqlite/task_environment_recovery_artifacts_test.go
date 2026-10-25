package sqlite

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/task/recoveryartifact"
)

func TestTaskEnvironmentRecoveryArtifactsMigrationIsIdempotent(t *testing.T) {
	repo := newRepoForEntityTests(t)
	for range 2 {
		if err := repo.initSchema(); err != nil {
			t.Fatalf("initSchema: %v", err)
		}
	}
	var count int
	if err := repo.db.QueryRowContext(context.Background(), `
		SELECT COUNT(1) FROM sqlite_master WHERE type = 'table' AND name = 'task_environment_recovery_artifacts'
	`).Scan(&count); err != nil {
		t.Fatalf("query artifact registry table: %v", err)
	}
	if count != 1 {
		t.Fatalf("artifact registry table count = %d, want 1", count)
	}
}

func TestTaskEnvironmentRecoveryArtifactsRequireCurrentClaimAndSlot(t *testing.T) {
	repo := newRepoForEntityTests(t)
	env, _ := seedWorkspaceInventoryRecovery(t, repo)
	ctx := context.Background()
	claim, err := repo.AcquireTaskEnvironmentRecoveryClaim(ctx, recoveryClaimRequest(
		env.ID, env.TaskID, "session-recovery", "artifact-operation", env.OwnershipGeneration,
	))
	if err != nil {
		t.Fatalf("AcquireTaskEnvironmentRecoveryClaim: %v", err)
	}
	defer func() { _ = repo.ReleaseTaskEnvironmentRecoveryClaim(ctx, claim) }()
	artifactRoot := t.TempDir()
	relocationRecord := filepath.Join(artifactRoot, "relocation.json")
	recoveryRecord := filepath.Join(artifactRoot, "recovery.json")
	for _, path := range []string{relocationRecord, recoveryRecord} {
		if err := os.WriteFile(path, []byte("verified legacy record"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok := recoveryartifact.FilesystemIdentity(relocationRecord); !ok {
		t.Skip("filesystem does not expose stable object identity")
	}
	registration := recoveryartifact.Registration{
		TaskEnvironmentID: env.ID, OwnerTaskID: env.TaskID, OwnershipGeneration: env.OwnershipGeneration,
		SessionID: claim.SessionID, OperationID: claim.OperationID, ExecutorType: claim.ExecutorType,
		WorktreeID: "worktree-recovery", RepositoryID: "repository-recovery",
		OriginalPath: "/synthetic/worktree", ReplacementID: "replacement-recovery",
		ReplacementPath: "/synthetic/worktree.relocated", LayoutVersion: 1,
		Provenance:    recoveryartifact.ProvenanceLegacyPublished,
		ArtifactPaths: []string{relocationRecord},
	}
	if err := repo.RegisterTaskEnvironmentRecoveryArtifacts(ctx, registration); err != nil {
		t.Fatalf("RegisterTaskEnvironmentRecoveryArtifacts: %v", err)
	}
	registration.ArtifactPaths = []string{recoveryRecord}
	if err := repo.RegisterTaskEnvironmentRecoveryArtifacts(ctx, registration); err != nil {
		t.Fatalf("append snapshot record path: %v", err)
	}
	registered, err := repo.ListTaskEnvironmentRecoveryArtifacts(ctx, env.ID)
	if err != nil || len(registered) != 1 || len(registered[0].ArtifactPaths) != 2 {
		t.Fatalf("ListTaskEnvironmentRecoveryArtifacts = %+v, %v", registered, err)
	}
	for _, path := range []string{relocationRecord, recoveryRecord} {
		if registered[0].ArtifactIdentities[path] == "" {
			t.Fatalf("registered artifact %s has no persisted filesystem identity: %+v", path, registered[0].ArtifactIdentities)
		}
	}
	if got := recoveryartifact.VerifiedLegacyArtifactPaths(registered); len(got) != 2 {
		t.Fatalf("verified legacy paths = %v, want both current records", got)
	}
	oldRecord := relocationRecord + ".old"
	if err := os.Rename(relocationRecord, oldRecord); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(relocationRecord, []byte("user replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	replacementRegistration := registration
	replacementRegistration.ArtifactPaths = []string{relocationRecord}
	if err := repo.RegisterTaskEnvironmentRecoveryArtifacts(ctx, replacementRegistration); err != nil {
		t.Fatalf("re-register existing path after replacement: %v", err)
	}
	registered, err = repo.ListTaskEnvironmentRecoveryArtifacts(ctx, env.ID)
	if err != nil || len(registered) != 1 {
		t.Fatalf("read registered artifacts after replacement = %+v, %v", registered, err)
	}
	if got := recoveryartifact.VerifiedLegacyArtifactPaths(registered); len(got) != 1 || got[0] != recoveryRecord {
		t.Fatalf("replacement path was still excluded: %v", got)
	}

	substituted := registration
	substituted.ReplacementPath = "/synthetic/other-replacement"
	if err := repo.RegisterTaskEnvironmentRecoveryArtifacts(ctx, substituted); !errors.Is(err, recoveryartifact.ErrIdentityMismatch) {
		t.Fatalf("substituted replacement path error = %v, want identity mismatch", err)
	}
	if err := repo.ReleaseTaskEnvironmentRecoveryClaim(ctx, claim); err != nil {
		t.Fatalf("ReleaseTaskEnvironmentRecoveryClaim: %v", err)
	}
	if err := repo.RegisterTaskEnvironmentRecoveryArtifacts(ctx, registration); !errors.Is(err, recoveryartifact.ErrIdentityMismatch) {
		t.Fatalf("registration after claim release error = %v, want identity mismatch", err)
	}
}

func TestTaskEnvironmentRecoveryArtifactsRejectStaleOwnerGeneration(t *testing.T) {
	repo := newRepoForEntityTests(t)
	env, _ := seedWorkspaceInventoryRecovery(t, repo)
	ctx := context.Background()
	claim, err := repo.AcquireTaskEnvironmentRecoveryClaim(ctx, recoveryClaimRequest(
		env.ID, env.TaskID, "session-recovery", "artifact-stale-operation", env.OwnershipGeneration,
	))
	if err != nil {
		t.Fatalf("AcquireTaskEnvironmentRecoveryClaim: %v", err)
	}
	defer func() { _ = repo.ReleaseTaskEnvironmentRecoveryClaim(ctx, claim) }()
	registration := recoveryartifact.Registration{
		TaskEnvironmentID: env.ID, OwnerTaskID: env.TaskID, OwnershipGeneration: env.OwnershipGeneration + 1,
		SessionID: claim.SessionID, OperationID: claim.OperationID, ExecutorType: claim.ExecutorType,
		WorktreeID: "worktree-recovery", RepositoryID: "repository-recovery",
		OriginalPath: "/synthetic/worktree", ReplacementID: "replacement-recovery",
		ReplacementPath: "/synthetic/worktree.relocated", LayoutVersion: 2,
		Provenance:    recoveryartifact.ProvenanceV2Operation,
		ArtifactPaths: []string{filepath.Join("/private", "records", "relocation.json")},
	}
	if err := repo.RegisterTaskEnvironmentRecoveryArtifacts(ctx, registration); !errors.Is(err, recoveryartifact.ErrIdentityMismatch) {
		t.Fatalf("stale owner generation error = %v, want identity mismatch", err)
	}
}
