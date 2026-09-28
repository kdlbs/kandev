package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

func TestTaskManagementClaimSchema(t *testing.T) {
	_, _, repo := createTestServiceWithSessionsRepo(t, func(repo *sqliterepo.Repository) taskrepo.SessionRepository {
		return repo
	})
	for _, table := range []string{"task_management_claims", "task_management_claim_history"} {
		var count int
		if err := repo.DB().QueryRowContext(context.Background(),
			"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?", table,
		).Scan(&count); err != nil {
			t.Fatalf("query %s migration: %v", table, err)
		}
		if count != 1 {
			t.Fatalf("table %s exists %d times, want one task-owned claim table", table, count)
		}
	}
}

func TestTaskManagementClaimFencing(t *testing.T) {
	ctx := context.Background()
	_, _, repo := createTestServiceWithSessionsRepo(t, func(repo *sqliterepo.Repository) taskrepo.SessionRepository {
		return repo
	})
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "claim-task", WorkspaceID: "claim-workspace", Title: "initial", Priority: "medium",
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	current, err := repo.GetTask(ctx, "claim-task")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	claims, ok := any(repo).(taskrepo.TaskManagementClaimRepository)
	if !ok {
		t.Fatal("sqlite repository does not implement task management claims")
	}
	claim, err := claims.ChangeTaskManagementClaim(ctx, models.TaskManagementClaimChange{
		Action: models.TaskManagementClaimAcquire, TaskID: current.ID, WorkspaceID: current.WorkspaceID,
		ExpectedTaskResourceVersion: current.UpdatedAt.UTC().Format(time.RFC3339Nano),
		InstallationID:              "installation-a", InstanceKey: "instance-a", ActorID: "plugin:installation-a",
	})
	if err != nil {
		t.Fatalf("acquire claim: %v", err)
	}
	if claim.Generation != 1 || claim.InstallationID != "installation-a" {
		t.Fatalf("unexpected acquired claim: %+v", claim)
	}
	if _, err := claims.ChangeTaskManagementClaim(ctx, models.TaskManagementClaimChange{
		Action: models.TaskManagementClaimAcquire, TaskID: current.ID, WorkspaceID: current.WorkspaceID,
		ExpectedTaskResourceVersion: current.UpdatedAt.UTC().Format(time.RFC3339Nano),
		InstallationID:              "installation-b", InstanceKey: "instance-b", ActorID: "plugin:installation-b",
	}); !errors.Is(err, repoerrors.ErrTaskManagementClaimConflict) && !errors.Is(err, repoerrors.ErrTaskManagementClaimOwned) {
		t.Fatalf("competing acquisition error = %v, want claim conflict", err)
	}

	current.Title = "managed update"
	_, err = repo.UpdateTaskExactOperation(ctx, current,
		current.WorkspaceID, current.UpdatedAt.UTC().Format(time.RFC3339Nano), "op-a", "digest-a",
		taskrepo.TaskManagementClaimFence{InstallationID: "installation-a", InstanceKey: "instance-a", Generation: claim.Generation},
	)
	if err != nil {
		t.Fatalf("current owner exact update: %v", err)
	}
	current, err = repo.GetTask(ctx, current.ID)
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	transferred, err := claims.ChangeTaskManagementClaim(ctx, models.TaskManagementClaimChange{
		Action: models.TaskManagementClaimTransfer, TaskID: current.ID, WorkspaceID: current.WorkspaceID,
		ExpectedTaskResourceVersion:  current.UpdatedAt.UTC().Format(time.RFC3339Nano),
		ExpectedClaimResourceVersion: claim.ResourceVersion,
		InstallationID:               "installation-b", InstanceKey: "instance-b", ActorID: "human:user-1", Reason: "handoff",
	})
	if err != nil {
		t.Fatalf("transfer claim: %v", err)
	}
	if transferred.Generation != claim.Generation+1 || transferred.InstallationID != "installation-b" {
		t.Fatalf("transfer did not fence prior owner: %+v", transferred)
	}

	current, err = repo.GetTask(ctx, current.ID)
	if err != nil {
		t.Fatalf("reload task after transfer: %v", err)
	}
	current.Title = "stale owner update"
	if _, err := repo.UpdateTaskExactOperation(ctx, current,
		current.WorkspaceID, current.UpdatedAt.UTC().Format(time.RFC3339Nano), "op-old", "digest-old",
		taskrepo.TaskManagementClaimFence{InstallationID: "installation-a", InstanceKey: "instance-a", Generation: claim.Generation},
	); err == nil || !errors.Is(err, repoerrors.ErrTaskManagementClaimConflict) {
		t.Fatalf("former owner update error = %v, want claim conflict", err)
	}
	stored, err := repo.GetTask(ctx, current.ID)
	if err != nil {
		t.Fatalf("reload task after stale write: %v", err)
	}
	if stored.Title != "managed update" {
		t.Fatalf("former owner changed task title to %q", stored.Title)
	}
	current = stored
	current.Title = "new manager update"
	if _, err := repo.UpdateTaskExactOperation(ctx, current,
		current.WorkspaceID, current.UpdatedAt.UTC().Format(time.RFC3339Nano), "op-b", "digest-b",
		taskrepo.TaskManagementClaimFence{InstallationID: "installation-b", InstanceKey: "instance-b", Generation: transferred.Generation},
	); err != nil {
		t.Fatalf("new owner exact update: %v", err)
	}
	current, err = repo.GetTask(ctx, current.ID)
	if err != nil {
		t.Fatalf("reload task before human takeover: %v", err)
	}
	humanOwner, err := claims.ChangeTaskManagementClaim(ctx, models.TaskManagementClaimChange{
		Action: models.TaskManagementClaimTransfer, TaskID: current.ID, WorkspaceID: current.WorkspaceID,
		ExpectedTaskResourceVersion:  current.UpdatedAt.UTC().Format(time.RFC3339Nano),
		ExpectedClaimResourceVersion: transferred.ResourceVersion,
		OwnerKind:                    "human", OwnerActorID: "user-1", ActorID: "user-1", Reason: "take over unavailable manager",
	})
	if err != nil {
		t.Fatalf("human takeover of unavailable manager: %v", err)
	}
	if humanOwner.Generation != transferred.Generation+1 || humanOwner.OwnerKind != "human" || humanOwner.OwnerActorID != "user-1" {
		t.Fatalf("human takeover did not advance ownership generation: %+v", humanOwner)
	}
	current, err = repo.GetTask(ctx, current.ID)
	if err != nil {
		t.Fatalf("reload task after human takeover: %v", err)
	}
	current.Title = "former manager update"
	if _, err := repo.UpdateTaskExactOperation(ctx, current,
		current.WorkspaceID, current.UpdatedAt.UTC().Format(time.RFC3339Nano), "op-former", "digest-former",
		taskrepo.TaskManagementClaimFence{InstallationID: "installation-b", InstanceKey: "instance-b", Generation: transferred.Generation},
	); err == nil || !errors.Is(err, repoerrors.ErrTaskManagementClaimConflict) {
		t.Fatalf("former manager write after human takeover error = %v, want claim conflict", err)
	}
	current, err = repo.GetTask(ctx, current.ID)
	if err != nil {
		t.Fatalf("reload task before human release: %v", err)
	}
	released, err := claims.ChangeTaskManagementClaim(ctx, models.TaskManagementClaimChange{
		Action: models.TaskManagementClaimRelease, TaskID: current.ID, WorkspaceID: current.WorkspaceID,
		ExpectedTaskResourceVersion:  current.UpdatedAt.UTC().Format(time.RFC3339Nano),
		ExpectedClaimResourceVersion: humanOwner.ResourceVersion,
		ActorID:                      "user-1", Reason: "manager unavailable",
	})
	if err != nil {
		t.Fatalf("human release unavailable manager: %v", err)
	}
	if released.Generation != humanOwner.Generation+1 || released.InstallationID != "" || released.OwnerKind != "" {
		t.Fatalf("release did not invalidate old generation: %+v", released)
	}
	current.Title = "stale unclaimed generation"
	if _, err := repo.UpdateTaskExactOperation(ctx, current,
		current.WorkspaceID, current.UpdatedAt.UTC().Format(time.RFC3339Nano), "op-unclaimed-stale", "digest-unclaimed-stale",
		taskrepo.TaskManagementClaimFence{},
	); !errors.Is(err, repoerrors.ErrTaskManagementClaimConflict) {
		t.Fatalf("generation-less write after release error = %v, want claim conflict", err)
	}
	current, err = repo.GetTask(ctx, current.ID)
	if err != nil {
		t.Fatalf("reload task before unclaimed write: %v", err)
	}
	current.Title = "unclaimed after release"
	if _, err := repo.UpdateTaskExactOperation(ctx, current,
		current.WorkspaceID, current.UpdatedAt.UTC().Format(time.RFC3339Nano), "op-unclaimed-current", "digest-unclaimed-current",
		taskrepo.TaskManagementClaimFence{Generation: released.Generation},
	); err != nil {
		t.Fatalf("write fenced by released generation: %v", err)
	}
	current, err = repo.GetTask(ctx, current.ID)
	if err != nil {
		t.Fatalf("reload task before reacquisition: %v", err)
	}
	if _, err := claims.ChangeTaskManagementClaim(ctx, models.TaskManagementClaimChange{
		Action: models.TaskManagementClaimAcquire, TaskID: current.ID, WorkspaceID: current.WorkspaceID,
		ExpectedTaskResourceVersion:  current.UpdatedAt.UTC().Format(time.RFC3339Nano),
		ExpectedClaimResourceVersion: released.ResourceVersion,
		InstallationID:               "installation-c", InstanceKey: "instance-c", ActorID: "plugin:installation-c",
	}); err != nil {
		t.Fatalf("reacquire after human release: %v", err)
	}
	history, err := claims.ListTaskManagementClaimHistory(ctx, current.ID)
	if err != nil {
		t.Fatalf("read claim history: %v", err)
	}
	if len(history) != 5 {
		t.Fatalf("claim history has %d entries, want acquire, plugin transfer, human takeover, release, reacquire", len(history))
	}
	if history[1].Action != models.TaskManagementClaimReleased || history[1].Reason != "manager unavailable" {
		t.Fatalf("release audit record missing actor action and reason: %+v", history[1])
	}
	if history[2].Action != models.TaskManagementClaimTransferred || history[2].OwnerKind != "human" || history[2].OwnerActorID != "user-1" {
		t.Fatalf("human takeover audit record is incomplete: %+v", history[2])
	}
}

func TestTaskManagementClaimAcquisitionRace(t *testing.T) {
	ctx := context.Background()
	_, _, repo := createTestServiceWithSessionsRepo(t, func(repo *sqliterepo.Repository) taskrepo.SessionRepository {
		return repo
	})
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "claim-race-task", WorkspaceID: "claim-race-workspace", Title: "race", Priority: "medium",
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	task, err := repo.GetTask(ctx, "claim-race-task")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	claims := any(repo).(taskrepo.TaskManagementClaimRepository)
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, owner := range []struct{ installation, instance string }{{"installation-a", "instance-a"}, {"installation-b", "instance-b"}} {
		owner := owner
		go func() {
			<-start
			_, err := claims.ChangeTaskManagementClaim(ctx, models.TaskManagementClaimChange{
				Action: models.TaskManagementClaimAcquire, TaskID: task.ID, WorkspaceID: task.WorkspaceID,
				ExpectedTaskResourceVersion: task.UpdatedAt.UTC().Format(time.RFC3339Nano),
				InstallationID:              owner.installation, InstanceKey: owner.instance, ActorID: "plugin:" + owner.installation,
			})
			results <- err
		}()
	}
	close(start)
	first, second := <-results, <-results
	if (first == nil) == (second == nil) {
		t.Fatalf("concurrent acquisitions returned %v and %v, want exactly one winner", first, second)
	}
	loser := first
	if loser == nil {
		loser = second
	}
	if !errors.Is(loser, repoerrors.ErrTaskManagementClaimConflict) && !errors.Is(loser, repoerrors.ErrTaskManagementClaimOwned) {
		t.Fatalf("losing acquisition error = %v, want claim conflict", loser)
	}
}

func TestTaskManagementClaimFenceSerializesTransferAndReleaseEffects(t *testing.T) {
	for _, action := range []string{
		models.TaskManagementClaimTransfer,
		models.TaskManagementClaimRelease,
	} {
		t.Run(action, func(t *testing.T) {
			ctx := context.Background()
			svc, _, repo := createTestService(t)
			setupTestTask(t, repo)
			task, err := repo.GetTask(ctx, "task-123")
			if err != nil {
				t.Fatalf("get task: %v", err)
			}
			claim, err := svc.ChangeTaskManagementClaim(ctx, task.ID, models.TaskManagementClaimChange{
				Action: models.TaskManagementClaimAcquire, TaskID: task.ID, WorkspaceID: task.WorkspaceID,
				ExpectedTaskResourceVersion: task.UpdatedAt.UTC().Format(time.RFC3339Nano),
				InstallationID:              "installation-a", InstanceKey: "delivery-lead", ActorID: "plugin:installation-a",
			})
			if err != nil {
				t.Fatalf("acquire claim: %v", err)
			}

			effectStarted := make(chan struct{})
			effectRelease := make(chan struct{})
			guardResult := make(chan error, 1)
			go func() {
				guardResult <- svc.WithTaskManagementClaimFence(ctx, task.ID, "installation-a", "delivery-lead", claim.Generation, func() error {
					close(effectStarted)
					<-effectRelease
					return nil
				})
			}()
			<-effectStarted

			transferTask, err := repo.GetTask(ctx, task.ID)
			if err != nil {
				t.Fatalf("reload task before claim change: %v", err)
			}
			change := models.TaskManagementClaimChange{
				Action: action, TaskID: task.ID, WorkspaceID: task.WorkspaceID,
				ExpectedTaskResourceVersion:  transferTask.UpdatedAt.UTC().Format(time.RFC3339Nano),
				ExpectedClaimResourceVersion: claim.ResourceVersion,
				ActorID:                      "human:user-1", Reason: "ownership race regression",
			}
			if action == models.TaskManagementClaimTransfer {
				change.InstallationID, change.InstanceKey = "installation-b", "reviewer"
			}
			changeResult := make(chan error, 1)
			go func() {
				_, changeErr := svc.ChangeTaskManagementClaim(ctx, task.ID, change)
				changeResult <- changeErr
			}()
			select {
			case err := <-changeResult:
				t.Fatalf("claim %s completed while the previous owner's effect held the fence: %v", action, err)
			case <-time.After(25 * time.Millisecond):
			}
			close(effectRelease)
			if err := <-guardResult; err != nil {
				t.Fatalf("protected effect: %v", err)
			}
			if err := <-changeResult; err != nil {
				t.Fatalf("claim %s after protected effect: %v", action, err)
			}
			if err := svc.WithTaskManagementClaimFence(ctx, task.ID, "installation-a", "delivery-lead", claim.Generation, func() error { return nil }); !errors.Is(err, taskrepo.ErrTaskManagementClaimConflict) {
				t.Fatalf("old owner effect after %s error = %v, want claim conflict", action, err)
			}
			if action == models.TaskManagementClaimRelease {
				released, err := svc.GetTaskManagementClaim(ctx, task.ID)
				if err != nil || released.OwnerKind != "" || released.Generation != claim.Generation+1 {
					t.Fatalf("released ownership = %+v err=%v, want unowned generation %d", released, err, claim.Generation+1)
				}
				if err := svc.WithTaskManagementClaimFence(ctx, task.ID, "installation-c", "", released.Generation, func() error { return nil }); err != nil {
					t.Fatalf("unclaimed task at current released generation: %v", err)
				}
			}
		})
	}
}
