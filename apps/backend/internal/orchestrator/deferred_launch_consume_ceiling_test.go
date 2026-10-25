package orchestrator

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	commonlogger "github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
)

func newServiceWithRealRepo(t *testing.T) (*Service, *tasksqlite.Repository) {
	t.Helper()
	conn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "deferred-launch.db"))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	sqlxDB := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	repo, err := tasksqlite.NewWithDB(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("NewWithDB: %v", err)
	}
	log, err := commonlogger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("NewFromZap: %v", err)
	}
	return &Service{repo: repo, logger: log}, repo
}

func deferredLaunchOf(t *testing.T, s *Service, taskID string) map[string]interface{} {
	t.Helper()
	task, err := s.repo.GetTask(context.Background(), taskID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.Metadata == nil {
		return nil
	}
	record, _ := task.Metadata[models.MetaKeyDeferredLaunch].(map[string]interface{})
	return record
}

// TestClaimForStartLeavesACeilingRecordInPlace pins the claim half: a direct
// start takes the launch intent it owns and must not delete the ceiling
// deferral sharing the record. Deleting it is a silently dropped launch that the
// sweep can no longer find.
func TestClaimForStartLeavesACeilingRecordInPlace(t *testing.T) {
	svc, repo := newServiceWithRealRepo(t)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "claim-mixed", Title: "Mixed",
		Metadata: map[string]interface{}{models.MetaKeyDeferredLaunch: map[string]interface{}{
			models.DeferredLaunchStartWhenUnblockedKey: true,
			models.CeilingDeferredKey:                  true,
			models.CeilingLaunchKindKey:                "start_task",
		}},
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	claim := svc.claimDeferredLaunchForStart(ctx, "claim-mixed")
	if !claim.held {
		t.Fatal("claim was not held; the WIP intent should have been claimable")
	}

	after := deferredLaunchOf(t, svc, "claim-mixed")
	if after == nil {
		t.Fatal("the whole deferred_launch record was deleted by the claim")
	}
	if after[models.CeilingDeferredKey] != true || after[models.CeilingLaunchKindKey] != "start_task" {
		t.Fatalf("ceiling keys did not survive the claim: %+v", after)
	}
	if _, still := after[models.DeferredLaunchStartWhenUnblockedKey]; still {
		t.Fatalf("the claim did not take its own key: %+v", after)
	}
}

// TestReleaseIfHeldMergesRatherThanOverwriting pins the release half: a ceiling
// record written while the launch was in flight must survive the restore. The
// pre-claim snapshot predates it, so writing that snapshot back would erase it.
func TestReleaseIfHeldMergesRatherThanOverwriting(t *testing.T) {
	svc, repo := newServiceWithRealRepo(t)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "claim-release", Title: "Release",
		Metadata: map[string]interface{}{models.MetaKeyDeferredLaunch: map[string]interface{}{
			models.DeferredLaunchStartWhenUnblockedKey: true,
			models.DeferredLaunchUserIDKey:             "user-7",
		}},
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	claim := svc.claimDeferredLaunchForStart(ctx, "claim-release")
	if !claim.held {
		t.Fatal("claim was not held")
	}
	if record := deferredLaunchOf(t, svc, "claim-release"); record != nil {
		t.Fatalf("a WIP-only record should have been removed entirely, got %+v", record)
	}

	// A ceiling refusal lands while the launch is in flight.
	if err := repo.SetTaskMetadataKey(ctx, "claim-release", models.MetaKeyDeferredLaunch, map[string]interface{}{
		models.CeilingDeferredKey:   true,
		models.CeilingLaunchKindKey: "start_task",
	}); err != nil {
		t.Fatalf("SetTaskMetadataKey: %v", err)
	}

	claim.releaseIfHeld(ctx)

	after := deferredLaunchOf(t, svc, "claim-release")
	if after[models.CeilingDeferredKey] != true || after[models.CeilingLaunchKindKey] != "start_task" {
		t.Fatalf("release overwrote the ceiling record: %+v", after)
	}
	if after[models.DeferredLaunchStartWhenUnblockedKey] != true || after[models.DeferredLaunchUserIDKey] != "user-7" {
		t.Fatalf("release did not put the claimed keys back: %+v", after)
	}
}

// TestClaimForStartDoesNotClaimACeilingOnlyRecord keeps the two meanings
// separate in the other direction: a ceiling deferral is replayed by its own
// sweep, so a direct start has nothing to claim and must leave it untouched.
func TestClaimForStartDoesNotClaimACeilingOnlyRecord(t *testing.T) {
	svc, repo := newServiceWithRealRepo(t)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "claim-ceiling-only", Title: "CeilingOnly",
		Metadata: map[string]interface{}{models.MetaKeyDeferredLaunch: map[string]interface{}{
			models.CeilingDeferredKey: true,
		}},
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	claim := svc.claimDeferredLaunchForStart(ctx, "claim-ceiling-only")
	if claim.held {
		t.Fatal("a ceiling-only record must not be claimed by the direct-start path")
	}
	if after := deferredLaunchOf(t, svc, "claim-ceiling-only"); after[models.CeilingDeferredKey] != true {
		t.Fatalf("ceiling record disturbed: %+v", after)
	}
}
