package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// @covers AC-PLUGINS-MANAGED-COORDINATION-002.6, AC-PLUGINS-MANAGED-COORDINATION-002.7, AC-PLUGINS-MANAGED-COORDINATION-002.8
func TestManagedConversationAdmissionControls(t *testing.T) {
	t.Run("failed_effects", func(t *testing.T) {
		services, repos, _ := managedAdmissionPair(t)
		ctx := context.Background()
		events := NewMockEventBus()
		services[0].eventer = events
		wakes := 0
		services[0].SetManagedInputNotifier(func(context.Context, string, string) { wakes++ })
		spec := managedAdmissionSpec()
		first, _, err := services[0].EnsureManaged(ctx, "plugin", "installation", spec, "create", "digest")
		require.NoError(t, err)
		require.Len(t, events.GetPublishedEvents(), 1)
		events.ClearEvents()
		_, err = services[0].SetManagedPaused(ctx, "installation", spec.WorkspaceID, spec.InstanceKey, first.Revision, false, "wake", "wake-digest")
		require.NoError(t, err)
		require.Equal(t, 1, wakes)
		wakes = 0
		require.NoError(t, repos[1].UpdateTaskSessionState(ctx, first.SessionID, models.TaskSessionStateRunning, ""))
		stops := 0
		services[0].SetManagedExecutionStopper(func(context.Context, string) error { stops++; return nil })
		require.NoError(t, services[0].PauseManagedForInstallation(ctx, "installation"))
		require.Equal(t, 1, stops)
		events.ClearEvents()
		stops = 0
		_, err = repos[1].DB().ExecContext(ctx, `CREATE TRIGGER fail_effect_metadata BEFORE UPDATE OF metadata ON tasks BEGIN SELECT RAISE(ABORT, 'metadata unavailable'); END`)
		require.NoError(t, err)
		_, err = services[0].SetManagedPaused(ctx, "installation", spec.WorkspaceID, spec.InstanceKey, first.Revision+1, false, "failed-wake", "failed-wake-digest")
		require.Error(t, err)
		require.Zero(t, wakes)
		require.Error(t, services[0].InvalidateManagedForInstallationWorkspace(ctx, "installation", spec.WorkspaceID))
		require.Zero(t, stops)
		_, err = repos[1].DB().ExecContext(ctx, `CREATE TRIGGER fail_effect_create BEFORE INSERT ON task_sessions BEGIN SELECT RAISE(ABORT, 'session unavailable'); END`)
		require.NoError(t, err)
		spec.InstanceKey = "failed-create"
		_, _, err = services[0].EnsureManaged(ctx, "plugin", "installation", spec, "failed-create", "failed-digest")
		require.Error(t, err)
		require.Empty(t, events.GetPublishedEvents())
	})
	t.Run("pause_missing_primary", func(t *testing.T) {
		services, repos, _ := managedAdmissionPair(t)
		ctx := context.Background()
		spec := managedAdmissionSpec()
		first, _, err := services[0].EnsureManaged(ctx, "plugin", "installation", spec, "create", "digest")
		require.NoError(t, err)
		primary, err := repos[1].GetTaskSession(ctx, first.SessionID)
		require.NoError(t, err)
		require.NoError(t, repos[1].DeleteTaskSession(ctx, primary))
		_, err = services[0].SetManagedPaused(ctx, "installation", spec.WorkspaceID, spec.InstanceKey, first.Revision, true, "pause", "pause-digest")
		require.Error(t, err)
		stored, err := repos[1].GetTask(ctx, first.TaskID)
		require.NoError(t, err)
		require.Equal(t, "1", stored.Metadata[models.MetaKeyManagedConversationRevision])
		require.Equal(t, false, stored.Metadata[models.MetaKeyManagedConversationPaused])
	})
	for _, mode := range []string{"nochange_busy", "execution_only", "empty_settings", "replay_repair", "raw_metadata", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			services, repos, _ := managedAdmissionPair(t)
			ctx := context.Background()
			spec := managedAdmissionSpec()
			first, _, err := services[0].EnsureManaged(ctx, "plugin", "installation", spec, "create", "create-digest")
			require.NoError(t, err)
			spec.ExpectedRevision = first.Revision
			operation, digest := "update", "update-digest"
			switch mode {
			case "nochange_busy":
				require.NoError(t, repos[1].UpdateTaskSessionState(ctx, first.SessionID, models.TaskSessionStateRunning, ""))
			case "execution_only":
				require.NoError(t, repos[1].UpsertExecutorRunning(ctx, &models.ExecutorRunning{TaskID: first.TaskID, SessionID: first.SessionID, AgentExecutionID: "live", Status: models.ExecutorRunningStatusRunning}))
				spec.BasePrompt = "changed"
			case "empty_settings":
				spec.BasePrompt, spec.InstructionVersion = "", ""
			case "replay_repair":
				primary, err := repos[1].GetTaskSession(ctx, first.SessionID)
				require.NoError(t, err)
				require.NoError(t, repos[1].DeleteTaskSession(ctx, primary))
				operation, digest = "create", "create-digest"
				spec.ExpectedRevision, spec.BasePrompt = 0, "untrusted replay settings"
			case "raw_metadata":
				require.NoError(t, repos[1].CreateTurn(ctx, &models.Turn{ID: "retained-turn", TaskID: first.TaskID, TaskSessionID: first.SessionID}))
				require.NoError(t, repos[1].CreateMessage(ctx, &models.Message{ID: "retained-message", TurnID: "retained-turn", TaskID: first.TaskID, TaskSessionID: first.SessionID, AuthorType: models.MessageAuthorAgent, Content: "retained transcript"}))
				_, err := repos[1].DB().ExecContext(ctx, `UPDATE tasks SET metadata=json_set(metadata,'$.precise',json('9007199254740993'),'$.unrelated',json('{"keep":[1,true]}')), title='retained title' WHERE id=?`, first.TaskID)
				require.NoError(t, err)
				_, err = repos[1].DB().ExecContext(ctx, `UPDATE task_sessions SET metadata='{"keep":9007199254740993}', review_status='approved' WHERE id=?`, first.SessionID)
				require.NoError(t, err)
				spec.BasePrompt = "current settings"
			case "cancelled":
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelled
				spec.BasePrompt = "cancelled settings"
			}
			current, _, err := services[0].EnsureManaged(ctx, "plugin", "installation", spec, operation, digest)
			if mode == "execution_only" {
				require.Equal(t, codes.FailedPrecondition, status.Code(err))
				return
			}
			if mode == "cancelled" {
				require.ErrorIs(t, err, context.Canceled)
				stored, err := repos[1].GetTask(context.Background(), first.TaskID)
				require.NoError(t, err)
				require.Equal(t, first.BasePrompt, stored.Metadata["kandev.base_prompt"])
				return
			}
			require.NoError(t, err)
			switch mode {
			case "nochange_busy":
				require.Equal(t, first.Revision, current.Revision)
			case "empty_settings":
				require.Empty(t, current.BasePrompt)
				require.Equal(t, first.Revision+1, current.Revision)
			case "replay_repair":
				require.Equal(t, first.SessionID, current.SessionID)
				require.Equal(t, first.Revision, current.Revision)
				require.Equal(t, first.BasePrompt, current.BasePrompt)
			case "raw_metadata":
				var raw, title, sessionRaw, review string
				require.NoError(t, repos[1].DB().QueryRowContext(ctx, `SELECT metadata,title FROM tasks WHERE id=?`, first.TaskID).Scan(&raw, &title))
				require.Contains(t, raw, "9007199254740993")
				require.Contains(t, raw, `"unrelated":{"keep":[1,true]}`)
				require.Equal(t, "retained title", title)
				require.NoError(t, repos[1].DB().QueryRowContext(ctx, `SELECT metadata,review_status FROM task_sessions WHERE id=?`, first.SessionID).Scan(&sessionRaw, &review))
				require.Equal(t, `{"keep":9007199254740993}`, sessionRaw)
				require.Equal(t, "approved", review)
				messages, err := repos[1].ListMessages(ctx, first.SessionID)
				require.NoError(t, err)
				require.Len(t, messages, 1)
				require.Equal(t, "retained transcript", messages[0].Content)
			}
		})
	}
}

// @covers AC-PLUGINS-MANAGED-COORDINATION-002.5, AC-PLUGINS-MANAGED-COORDINATION-002.8
func TestManagedConversationAdmissionLifecycle(t *testing.T) {
	for _, mode := range []string{"exact_pause", "installation_pause", "invalidation", "detach"} {
		t.Run(mode, func(t *testing.T) {
			services, repos, gate := managedAdmissionPair(t)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			var workers sync.WaitGroup
			var once sync.Once
			release := func() { once.Do(func() { close(gate.release) }) }
			t.Cleanup(func() { cancel(); release(); workers.Wait() })
			spec := managedAdmissionSpec()
			first, _, err := services[0].EnsureManaged(ctx, "plugin", "installation", spec, "create", "create-digest")
			require.NoError(t, err)
			require.NoError(t, repos[1].CreateTurn(ctx, &models.Turn{ID: "lifecycle-turn", TaskID: first.TaskID, TaskSessionID: first.SessionID}))
			require.NoError(t, repos[1].CreateMessage(ctx, &models.Message{ID: "lifecycle-message", TurnID: "lifecycle-turn", TaskID: first.TaskID, TaskSessionID: first.SessionID, AuthorType: models.MessageAuthorAgent, Content: "lifecycle retained transcript"}))
			spec.ExpectedRevision, spec.BasePrompt = first.Revision, "delayed settings"
			gate.armed.Store(true)
			done := make(chan error, 1)
			workers.Add(1)
			go func() {
				defer workers.Done()
				_, _, err := services[0].EnsureManaged(ctx, "plugin", "installation", spec, "delayed", "delayed-digest")
				done <- err
			}()
			select {
			case <-gate.seen:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			switch mode {
			case "exact_pause":
				_, err = services[1].SetManagedPaused(ctx, "installation", spec.WorkspaceID, spec.InstanceKey, first.Revision, true, "pause", "pause-digest")
			case "installation_pause":
				err = services[1].PauseManagedForInstallation(ctx, "installation")
			case "invalidation":
				err = services[1].InvalidateManagedForInstallationWorkspace(ctx, "installation", spec.WorkspaceID)
			case "detach":
				err = services[1].DetachManagedForInstallation(ctx, "installation")
			}
			require.NoError(t, err)
			release()
			err = <-done
			workers.Wait()
			if mode == "detach" {
				require.Equal(t, codes.NotFound, status.Code(err))
			} else {
				require.Equal(t, codes.Aborted, status.Code(err))
			}
			task, err := repos[0].GetTask(ctx, first.TaskID)
			require.NoError(t, err)
			require.Equal(t, first.BasePrompt, task.Metadata["kandev.base_prompt"])
			require.Equal(t, "2", task.Metadata[models.MetaKeyManagedConversationRevision])
			primary, err := repos[0].GetTaskSession(ctx, first.SessionID)
			require.NoError(t, err)
			require.Equal(t, models.TaskSessionStateCreated, primary.State)
			messages, err := repos[0].ListMessages(ctx, first.SessionID)
			require.NoError(t, err)
			require.Len(t, messages, 1)
			require.Equal(t, "lifecycle retained transcript", messages[0].Content)
		})
	}
	t.Run("postcommit_stop_failure", func(t *testing.T) {
		services, repos, _ := managedAdmissionPair(t)
		ctx := context.Background()
		spec := managedAdmissionSpec()
		first, _, err := services[0].EnsureManaged(ctx, "plugin", "installation", spec, "create", "digest")
		require.NoError(t, err)
		require.NoError(t, repos[1].UpdateTaskSessionState(ctx, first.SessionID, models.TaskSessionStateRunning, ""))
		stopped := 0
		failure := errors.New("transport stop failed")
		services[0].SetManagedExecutionStopper(func(context.Context, string) error { stopped++; return failure })
		require.ErrorIs(t, services[0].PauseManagedForInstallation(ctx, "installation"), failure)
		require.Equal(t, 1, stopped)
		current, err := services[1].GetManaged(ctx, "installation", spec.WorkspaceID, spec.InstanceKey)
		require.NoError(t, err)
		require.True(t, current.DesiredPaused)
		require.Equal(t, first.Revision+1, current.Revision)
	})
}

// @covers AC-PLUGINS-MANAGED-COORDINATION-002.6, AC-PLUGINS-MANAGED-COORDINATION-002.8
func TestManagedConversationAdmissionPublication(t *testing.T) {
	t.Run("creation_defaults", func(t *testing.T) {
		services, repos, _ := managedAdmissionPair(t)
		spec := managedAdmissionSpec()
		first, _, err := services[0].EnsureManaged(context.Background(), "plugin", "installation", spec, "create", "digest")
		require.NoError(t, err)
		stored, err := repos[1].GetTask(context.Background(), first.TaskID)
		require.NoError(t, err)
		require.Equal(t, "[]", stored.Labels)
	})
	for _, mode := range []string{"configuration", "exact_pause", "installation_pause", "invalidation", "detach"} {
		t.Run(mode, func(t *testing.T) {
			services, repos, _ := managedAdmissionPair(t)
			ctx := context.Background()
			eventBus := NewMockEventBus()
			services[0].eventer = managedAdmissionEventObserver{MockEventBus: eventBus, observe: func(event *bus.Event) {
				if event.Type != events.TaskUpdated {
					return
				}
				data := event.Data.(map[string]interface{})
				stored, err := repos[1].GetTask(ctx, data["task_id"].(string))
				require.NoError(t, err)
				require.Equal(t, stored.UpdatedAt.Format(time.RFC3339Nano), data["updated_at"])
			}}
			spec := managedAdmissionSpec()
			first, _, err := services[0].EnsureManaged(ctx, "plugin", "installation", spec, "create", "digest")
			require.NoError(t, err)
			eventBus.ClearEvents()
			switch mode {
			case "configuration":
				spec.ExpectedRevision, spec.BasePrompt = first.Revision, "updated instructions"
				_, _, err = services[0].EnsureManaged(ctx, "plugin", "installation", spec, "update", "update-digest")
			case "exact_pause":
				_, err = services[0].SetManagedPaused(ctx, "installation", spec.WorkspaceID, spec.InstanceKey, first.Revision, true, "pause", "pause-digest")
			case "installation_pause":
				err = services[0].PauseManagedForInstallation(ctx, "installation")
			case "invalidation":
				err = services[0].InvalidateManagedForInstallationWorkspace(ctx, "installation", spec.WorkspaceID)
			case "detach":
				err = services[0].DetachManagedForInstallation(ctx, "installation")
			}
			require.NoError(t, err)
			published := eventBus.GetPublishedEvents()
			require.Len(t, published, 1)
			require.Equal(t, events.TaskUpdated, published[0].Type)
			stored, err := repos[1].GetTask(ctx, first.TaskID)
			require.NoError(t, err)
			data := published[0].Data.(map[string]interface{})
			require.Equal(t, first.TaskID, data["task_id"])
			require.Equal(t, stored.UpdatedAt.Format(time.RFC3339Nano), data["updated_at"])
			eventBus.ClearEvents()
			if mode == "configuration" {
				_, _, err = services[0].EnsureManaged(ctx, "plugin", "installation", spec, "update", "update-digest")
				require.NoError(t, err)
				require.Empty(t, eventBus.GetPublishedEvents())
			}
		})
	}
}

type managedAdmissionEventObserver struct {
	*MockEventBus
	observe func(*bus.Event)
}

func (b managedAdmissionEventObserver) Publish(ctx context.Context, subject string, event *bus.Event) error {
	b.observe(event)
	return b.MockEventBus.Publish(ctx, subject, event)
}
