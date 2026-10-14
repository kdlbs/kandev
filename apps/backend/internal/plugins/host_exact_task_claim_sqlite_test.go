package plugins

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/state"
	pluginstore "github.com/kandev/kandev/internal/plugins/store"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/pkg/pluginsdk"
	_ "github.com/mattn/go-sqlite3"
)

type sqliteExactTaskUpdateWriter struct {
	*fakeTaskWriter
	service *taskservice.Service
	fences  []taskrepo.TaskManagementClaimFence
}

func (w *sqliteExactTaskUpdateWriter) UpdateTaskExact(ctx context.Context, input ExactTaskUpdateInput) (*taskmodels.Task, bool, error) {
	w.fences = append(w.fences, input.ClaimFence)
	updated, err := w.service.UpdateTaskExact(ctx, input.TaskID, taskservice.ExactTaskUpdateRequest{
		WorkspaceID: input.WorkspaceID, ExpectedResourceVersion: input.ExpectedResourceVersion,
		OperationID: input.OperationID, PayloadDigest: input.PayloadDigest,
		Title: input.Title, Description: input.Description, Priority: input.Priority,
		Labels: input.Labels, AssigneeUserID: input.AssigneeUserID, ClaimFence: input.ClaimFence,
	})
	if err != nil {
		return nil, false, err
	}
	return updated.Task, updated.AlreadyApplied, nil
}

func TestExactHostTaskUpdatesUseSQLiteManagementClaims(t *testing.T) {
	ctx := context.Background()
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatalf("open task database: %v", err)
	}
	taskDB := sqlx.NewDb(dbConn, "sqlite3")
	taskRepository, closeRepository, err := taskrepo.Provide(taskDB, taskDB, nil)
	if err != nil {
		t.Fatalf("provide task repository: %v", err)
	}
	t.Cleanup(func() {
		_ = taskDB.Close()
		_ = closeRepository()
	})
	if err := taskRepository.CreateWorkspace(ctx, &taskmodels.Workspace{ID: "claim-host-workspace", Name: "Claims"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := taskRepository.CreateWorkflow(ctx, &taskmodels.Workflow{ID: "claim-host-workflow", WorkspaceID: "claim-host-workspace", Name: "Workflow"}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	taskService := taskservice.NewService(taskservice.Repos{
		Workspaces: taskRepository, Tasks: taskRepository, TaskRepos: taskRepository, Workflows: taskRepository,
	}, nil, testLogger(t), taskservice.RepositoryDiscoveryConfig{})

	commandDB, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open command database: %v", err)
	}
	commandDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = commandDB.Close() })
	commandStore, err := state.NewCommandStore(db.NewPool(commandDB, commandDB))
	if err != nil {
		t.Fatalf("create command store: %v", err)
	}
	record := &pluginstore.Record{
		Manifest:       manifest.Manifest{ID: "claim-host-plugin", Capabilities: manifest.Capabilities{APIWrite: []string{"tasks"}}},
		InstallationID: "host-installation",
	}
	registry := NewRegistry()
	registry.Add(record)
	pluginService := NewService(pluginstore.NewFSStore(t.TempDir()), registry, nil, testLogger(t))
	if err := pluginService.SetPluginsDir(t.TempDir()); err != nil {
		t.Fatalf("set plugin directory: %v", err)
	}
	pluginService.SetExactCommandStore(commandStore)
	t.Cleanup(func() { _ = pluginService.Close() })
	digest := ManifestCapabilityDigest(record.Manifest)
	if _, err := pluginService.approvalGrant(record.InstallationID, "claim-host-workspace", 1, digest,
		[]string{"host.v2.write:tasks"}, "human", "grant", "claim-host-approval"); err != nil {
		t.Fatalf("grant task update capability: %v", err)
	}
	host := pluginService.hostForPlugin(record.ID).(*pluginHost)
	writer := &sqliteExactTaskUpdateWriter{fakeTaskWriter: &fakeTaskWriter{}, service: taskService}
	host.taskWriter = writer

	createTask := func(id, title string) *taskmodels.Task {
		t.Helper()
		if err := taskRepository.CreateTask(ctx, &taskmodels.Task{
			ID: id, WorkspaceID: "claim-host-workspace", WorkflowID: "claim-host-workflow",
			Title: title, Priority: "medium",
		}); err != nil {
			t.Fatalf("create task %s: %v", id, err)
		}
		task, err := taskRepository.GetTask(ctx, id)
		if err != nil {
			t.Fatalf("get task %s: %v", id, err)
		}
		return task
	}
	update := func(task *taskmodels.Task, idempotencyKey, instanceKey string, generation int64, title string) *pluginsdk.CommandResult {
		t.Helper()
		result, _, err := host.UpdateTaskExact(ctx, pluginsdk.ExactTaskUpdate{
			RequestID: "request-" + idempotencyKey, WorkspaceID: task.WorkspaceID, TaskID: task.ID,
			IdempotencyKey: idempotencyKey, ExpectedResourceVersion: task.UpdatedAt.UTC().Format(time.RFC3339Nano),
			ManagementInstanceKey: instanceKey, ExpectedClaimGeneration: generation,
			ApprovalRevision: 1, ManifestDigest: digest, Title: &title,
		})
		if err != nil {
			t.Fatalf("Host UpdateTaskExact %s: %v", idempotencyKey, err)
		}
		return result
	}
	acquire := func(task *taskmodels.Task, installationID, instanceKey string) *taskmodels.TaskManagementClaim {
		t.Helper()
		claim, err := taskService.ChangeTaskManagementClaim(ctx, task.ID, taskmodels.TaskManagementClaimChange{
			Action: taskmodels.TaskManagementClaimAcquire, TaskID: task.ID, WorkspaceID: task.WorkspaceID,
			ExpectedTaskResourceVersion: task.UpdatedAt.UTC().Format(time.RFC3339Nano),
			InstallationID:              installationID, InstanceKey: instanceKey, ActorID: "plugin:" + installationID,
		})
		if err != nil {
			t.Fatalf("acquire claim for %s: %v", task.ID, err)
		}
		return claim
	}

	t.Run("never claimed task accepts authenticated installation attribution", func(t *testing.T) {
		task := createTask("claim-host-unclaimed", "before")
		result := update(task, "unclaimed-update", "", 0, "after")
		if result.Status != pluginsdk.CommandApplied {
			t.Fatalf("unclaimed Host update status = %s, want APPLIED", result.Status)
		}
		if fence := writer.fences[len(writer.fences)-1]; fence.InstallationID != record.InstallationID || fence.InstanceKey != "" || fence.Generation != 0 {
			t.Fatalf("Host attribution fence = %+v, want installation attribution with no ownership assertion", fence)
		}
	})

	t.Run("released task accepts its current unowned generation", func(t *testing.T) {
		task := createTask("claim-host-released", "before")
		claim := acquire(task, "other-installation", "owner")
		claimedTask, err := taskRepository.GetTask(ctx, task.ID)
		if err != nil {
			t.Fatalf("reload claimed task: %v", err)
		}
		released, err := taskService.ChangeTaskManagementClaim(ctx, task.ID, taskmodels.TaskManagementClaimChange{
			Action: taskmodels.TaskManagementClaimRelease, TaskID: task.ID, WorkspaceID: task.WorkspaceID,
			ExpectedTaskResourceVersion:  claimedTask.UpdatedAt.UTC().Format(time.RFC3339Nano),
			ExpectedClaimResourceVersion: claim.ResourceVersion, ActorID: "human:user-1", Reason: "release for plugin update",
		})
		if err != nil {
			t.Fatalf("release claim: %v", err)
		}
		result := update(claimedTask, "released-update", "", released.Generation, "after release")
		if result.Status != pluginsdk.CommandApplied {
			t.Fatalf("released Host update status = %s, want APPLIED", result.Status)
		}
	})

	t.Run("active competing installation remains fenced", func(t *testing.T) {
		task := createTask("claim-host-competing", "before")
		claim := acquire(task, "owner-installation", "owner-instance")
		result := update(task, "competing-update", "", 0, "must not apply")
		if result.Status != pluginsdk.CommandConflict {
			t.Fatalf("unowned Host update against active owner status = %s, want CONFLICT", result.Status)
		}
		staleOwnerAssertion := update(task, "forged-owner-update", "owner-instance", claim.Generation, "must still not apply")
		if staleOwnerAssertion.Status != pluginsdk.CommandConflict {
			t.Fatalf("forged active owner assertion status = %s, want CONFLICT", staleOwnerAssertion.Status)
		}
		stored, err := taskRepository.GetTask(ctx, task.ID)
		if err != nil {
			t.Fatalf("reload task after rejected writes: %v", err)
		}
		if stored.Title != "before" {
			t.Fatalf("rejected updates changed title to %q", stored.Title)
		}
		for _, fence := range writer.fences[len(writer.fences)-2:] {
			if fence.InstallationID != record.InstallationID {
				t.Fatalf("Host attribution fence = %+v, want installation %q", fence, record.InstallationID)
			}
		}
	})
}
