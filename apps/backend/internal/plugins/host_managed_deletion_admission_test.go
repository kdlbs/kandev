package plugins

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/canvas"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/plugins/instances"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/state"
	pluginstore "github.com/kandev/kandev/internal/plugins/store"
	"github.com/kandev/kandev/internal/task/models"
	managed "github.com/kandev/kandev/internal/task/repository/managedconversation"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/testutil"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"github.com/stretchr/testify/require"
)

var managedHostDeletionTemplate = testutil.NewSQLiteTemplate(func(database *sqlx.DB) error {
	if _, err := tasksqlite.NewWithDB(database, database, nil); err != nil {
		return err
	}
	if _, err := officesqlite.NewWithDB(database, database, nil); err != nil {
		return err
	}
	if _, err := workflowrepo.NewWithDB(database, database, nil); err != nil {
		return err
	}
	if _, _, err := settingsstore.Provide(database, database, nil); err != nil {
		return err
	}
	_, err := messagequeue.NewSQLiteRepository(database, database)
	return err
})

// Record the actual publication boundary while delegating to the real bus.
type managedDeletionHostBus struct {
	bus.EventBus
	mu      sync.Mutex
	deleted int
}

func (b *managedDeletionHostBus) Publish(ctx context.Context, subject string, event *bus.Event) error {
	if event.Type == events.TaskDeleted {
		b.mu.Lock()
		b.deleted++
		b.mu.Unlock()
	}
	return b.EventBus.Publish(ctx, subject, event)
}
func (b *managedDeletionHostBus) count() int { b.mu.Lock(); defer b.mu.Unlock(); return b.deleted }

type managedDeletionHostFixture struct {
	database                   *sqlx.DB
	repo, other                *tasksqlite.Repository
	commands                   *state.CommandStore
	conversations, independent *taskservice.AgentConversationService
	lifecycle                  *taskservice.Service
	host                       *Service
	manager                    pluginsdk.ManagedAgentConversationManager
	descriptor                 pluginsdk.ManagedAgentConversationDescriptor
	spec                       pluginsdk.ManagedAgentConversationSpec
	input                      pluginsdk.ManagedAgentConversationDelete
	canvas                     *canvas.Repository
	canvasID                   string
	events                     *managedDeletionHostBus
}

func newManagedDeletionHostFixture(t *testing.T, ctx context.Context) *managedDeletionHostFixture {
	t.Helper()
	database, path := managedHostDeletionTemplate.Open(t)
	open := func(reader bool) *sqlx.DB {
		var raw *sql.DB
		var err error
		if reader {
			raw, err = db.OpenSQLiteReader(path)
		} else {
			raw, err = db.OpenSQLite(path)
		}
		require.NoError(t, err)
		connection := sqlx.NewDb(raw, "sqlite3")
		t.Cleanup(func() { require.NoError(t, connection.Close()) })
		require.NoError(t, connection.PingContext(ctx))
		return connection
	}
	reader, second, secondReader := open(true), open(false), open(true)
	pool := db.NewPool(database, reader)
	repo := tasksqlite.NewWithInitializedDB(database, reader, nil)
	other := tasksqlite.NewWithInitializedDB(second, secondReader, nil)
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace-one", Name: "Workspace"}))
	commands, err := state.NewCommandStore(pool)
	require.NoError(t, err)
	pluginState, err := state.NewStore(pool)
	require.NoError(t, err)
	independentState, err := state.NewStore(db.NewPool(second, secondReader))
	require.NoError(t, err)
	record := &pluginstore.Record{Manifest: manifest.Manifest{ID: "coordinator", Capabilities: manifest.Capabilities{APIRead: []string{"managed_agent_conversations"}, APIWrite: []string{"managed_agent_conversations"}}}, InstallationID: "installation-one", Status: StatusActive}
	registry := NewRegistry()
	registry.Add(record)
	host := NewService(pluginstore.NewFSStore(t.TempDir()), registry, nil, testLogger(t))
	t.Cleanup(func() { require.NoError(t, host.Close()) })
	require.NoError(t, host.SetPluginsDir(t.TempDir()))
	host.SetExactCommandStore(commands)
	digest := ManifestCapabilityDigest(record.Manifest)
	_, err = host.approvalGrant(record.InstallationID, "workspace-one", 1, digest, []string{"host.v2.read:managed_agent_conversations", "host.v2.write:managed_agent_conversations"}, "human", "grant", "approval-one")
	require.NoError(t, err)
	eventBus := &managedDeletionHostBus{EventBus: bus.NewMemoryEventBus(testLogger(t))}
	t.Cleanup(eventBus.Close)
	lifecycle := taskservice.NewService(taskservice.Repos{Workspaces: repo, Tasks: repo, TaskRepos: repo, Messages: repo, Turns: repo, Sessions: repo, Attachments: repo, GitSnapshots: repo, RepoEntities: repo, Executors: repo, Environments: repo, TaskEnvironments: repo, ResourceCleanups: repo, Reviews: repo, BackgroundWork: repo}, eventBus, testLogger(t), taskservice.RepositoryDiscoveryConfig{})
	require.NoError(t, lifecycle.StartTaskResourceCleanupWorker(ctx))
	t.Cleanup(lifecycle.StopTaskResourceCleanupWorker)
	conversations := taskservice.NewAgentConversationService(repo, repo, nil, pluginState, nil)
	conversations.SetTaskDeleter(lifecycle)
	independent := taskservice.NewAgentConversationService(other, other, nil, independentState, nil)
	independent.SetTaskDeleter(lifecycle)
	host.SetManagedAgentConversations(conversations)
	manager := host.hostForPlugin(record.ID).(pluginsdk.ExactHost).ManagedAgentConversations()
	spec := pluginsdk.ManagedAgentConversationSpec{RequestID: "create-request", IdempotencyKey: "create", WorkspaceID: "workspace-one", InstanceKey: "lead", ApprovalRevision: 1, ManifestDigest: digest, AgentProfileID: "profile", BasePrompt: "original"}
	result, descriptor, err := manager.Ensure(ctx, spec)
	require.NoError(t, err)
	require.Equal(t, pluginsdk.CommandApplied, result.Status)
	require.NoError(t, repo.CreateTurn(ctx, &models.Turn{ID: "retained-turn", TaskID: descriptor.TaskID, TaskSessionID: descriptor.SessionID}))
	require.NoError(t, repo.CreateMessage(ctx, &models.Message{ID: "retained-message", TaskID: descriptor.TaskID, TaskSessionID: descriptor.SessionID, TurnID: "retained-turn", AuthorType: models.MessageAuthorUser, Content: "host retained transcript"}))
	canvasRepo, err := canvas.NewRepository(pool)
	require.NoError(t, err)
	instanceStore, err := instances.NewStore(pool)
	require.NoError(t, err)
	canvasSvc := canvas.NewService(canvasRepo, instanceStore)
	created, err := canvasSvc.Create(ctx, canvas.CreateCanvasRequest{WorkspaceID: spec.WorkspaceID, TaskID: descriptor.TaskID, Title: "Managed canvas", CreatedBySessionID: descriptor.SessionID})
	require.NoError(t, err)
	lifecycle.SetCanvasCleanup(canvasSvc)
	return &managedDeletionHostFixture{database: database, repo: repo, other: other, commands: commands, conversations: conversations, independent: independent, lifecycle: lifecycle, host: host, manager: manager, descriptor: descriptor, spec: spec, input: pluginsdk.ManagedAgentConversationDelete{RequestID: "delete-request", IdempotencyKey: "delete", WorkspaceID: spec.WorkspaceID, InstanceKey: spec.InstanceKey, ExpectedRevision: descriptor.Revision, ApprovalRevision: 1, ManifestDigest: digest}, canvas: canvasRepo, canvasID: created.ID, events: eventBus}
}

func (f *managedDeletionHostFixture) receipt(t *testing.T, ctx context.Context) state.CommandRecord {
	t.Helper()
	var operation string
	require.NoError(t, f.database.QueryRowContext(ctx, `SELECT operation_id FROM plugin_host_command_intents WHERE method='DeleteManagedAgentConversationExact' AND idempotency_key='delete'`).Scan(&operation))
	record, err := f.commands.Get(ctx, operation)
	require.NoError(t, err)
	return record
}
func (f *managedDeletionHostFixture) retained(t *testing.T, ctx context.Context) {
	t.Helper()
	_, err := f.other.GetTask(ctx, f.descriptor.TaskID)
	require.NoError(t, err)
	primary, err := f.other.GetPrimarySessionByTaskID(ctx, f.descriptor.TaskID)
	require.NoError(t, err)
	require.Equal(t, f.descriptor.SessionID, primary.ID)
	messages, err := f.other.ListMessages(ctx, primary.ID)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, "host retained transcript", messages[0].Content)
}

// @covers AC-PLUGINS-MANAGED-COORDINATION-013.1, AC-PLUGINS-MANAGED-COORDINATION-013.2, AC-PLUGINS-MANAGED-COORDINATION-013.5, AC-PLUGINS-MANAGED-COORDINATION-013.6
func TestManagedDeletionHostReceipts(t *testing.T) {
	for _, mode := range []string{"stale", "detached", "missing", "current", "completed_replay", "ack_failure", "unavailable", "payload_mismatch"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			f := newManagedDeletionHostFixture(t, ctx)
			originalTask, taskErr := f.other.GetTask(ctx, f.descriptor.TaskID)
			require.NoError(t, taskErr)
			want := pluginsdk.CommandApplied
			switch mode {
			case "stale", "payload_mismatch":
				spec := f.spec
				spec.ExpectedRevision = 1
				spec.BasePrompt = "independent winner"
				accepted, _, err := f.independent.EnsureManaged(ctx, "coordinator", "installation-one", spec, "winner", "winner-digest")
				require.NoError(t, err)
				require.Equal(t, uint64(2), accepted.Revision)
				want = pluginsdk.CommandConflict
			case "detached":
				require.NoError(t, f.independent.DetachManagedForInstallation(ctx, "installation-one"))
				want = pluginsdk.CommandNotFound
			case "missing":
				f.input.InstanceKey = "missing"
				want = pluginsdk.CommandNotFound
			case "unavailable":
				f.conversations.SetTaskDeleter(nil)
				want = pluginsdk.CommandUnavailable
			case "ack_failure":
				_, err := f.database.ExecContext(ctx, `CREATE TRIGGER fail_delete_receipt BEFORE UPDATE ON plugin_host_command_receipts BEGIN SELECT RAISE(ABORT,'receipt unavailable'); END`)
				require.NoError(t, err)
				want = pluginsdk.CommandUnavailable
			}
			result, err := f.manager.Delete(ctx, f.input)
			require.NoError(t, err)
			require.Equal(t, want, result.Status)
			removed := mode == "current" || mode == "completed_replay" || mode == "ack_failure"
			if !removed {
				f.retained(t, ctx)
				require.Zero(t, f.events.count())
				_, err = f.canvas.Get(ctx, f.canvasID)
				require.NoError(t, err)
				jobs, err := f.other.ListTaskResourceCleanupJobs(ctx, f.descriptor.TaskID)
				require.NoError(t, err)
				require.Empty(t, jobs)
				record := f.receipt(t, ctx)
				if mode == "unavailable" {
					require.Equal(t, "accepted", record.Receipt.State)
					require.Nil(t, result.Receipt)
				} else {
					require.Equal(t, "completed", record.Receipt.State)
					require.NotNil(t, result.Receipt)
				}
				if mode == "payload_mismatch" {
					f.input.ExpectedRevision = 2
					result, err = f.manager.Delete(ctx, f.input)
					require.NoError(t, err)
					require.Equal(t, pluginsdk.CommandConflict, result.Status)
					require.Equal(t, "idempotency_payload_mismatch", result.Reason)
					f.retained(t, ctx)
				}
				return
			}
			_, err = f.other.GetTask(ctx, f.descriptor.TaskID)
			require.Error(t, err)
			_, err = f.other.GetTaskSession(ctx, f.descriptor.SessionID)
			require.Error(t, err)
			_, err = f.canvas.Get(ctx, f.canvasID)
			require.Error(t, err)
			require.Equal(t, 1, f.events.count())
			record := f.receipt(t, ctx)
			jobs, err := f.other.ListTaskResourceCleanupJobs(ctx, f.descriptor.TaskID)
			require.NoError(t, err)
			require.Len(t, jobs, 1)
			claim, err := managed.DeletionEnvelope(jobs[0].ResourceSnapshot)
			require.NoError(t, err)
			require.Equal(t, managed.DeleteCommitted, claim.Phase)
			require.Equal(t, record.Intent.OperationID, claim.OperationID)
			if mode == "current" {
				require.NotNil(t, result.Receipt)
				require.Equal(t, "completed", record.Receipt.State)
				return
			}
			if mode == "ack_failure" {
				require.Equal(t, "accepted", record.Receipt.State)
				require.Equal(t, "command_receipt_unavailable", result.Reason)
				_, err = f.database.ExecContext(ctx, `DROP TRIGGER fail_delete_receipt`)
				require.NoError(t, err)
			}
			spec := f.spec
			spec.RequestID = "replacement-request"
			spec.IdempotencyKey = "replacement"
			created, replacement, err := f.manager.Ensure(ctx, spec)
			require.NoError(t, err)
			require.Equal(t, pluginsdk.CommandApplied, created.Status)
			replacedTask, err := f.other.GetTask(ctx, replacement.TaskID)
			require.NoError(t, err)
			require.False(t, originalTask.CreatedAt.Equal(replacedTask.CreatedAt), "existing API may reuse ID; current task incarnation must be new")
			require.NoError(t, f.other.CreateTurn(ctx, &models.Turn{ID: "replacement-turn", TaskID: replacement.TaskID, TaskSessionID: replacement.SessionID}))
			require.NoError(t, f.other.CreateMessage(ctx, &models.Message{ID: "replacement-message", TaskID: replacement.TaskID, TaskSessionID: replacement.SessionID, TurnID: "replacement-turn", AuthorType: models.MessageAuthorUser, Content: "replacement transcript"}))
			result, err = f.manager.Delete(ctx, f.input)
			require.NoError(t, err)
			if mode == "ack_failure" {
				require.Equal(t, pluginsdk.CommandAlreadyApplied, result.Status)
			} else {
				require.Equal(t, pluginsdk.CommandApplied, result.Status)
			}
			require.NotNil(t, result.Receipt)
			_, err = f.other.GetTask(ctx, replacement.TaskID)
			require.NoError(t, err)
			primary, err := f.other.GetPrimarySessionByTaskID(ctx, replacement.TaskID)
			require.NoError(t, err)
			require.Equal(t, replacement.SessionID, primary.ID)
			messages, err := f.other.ListMessages(ctx, primary.ID)
			require.NoError(t, err)
			require.Len(t, messages, 1)
			require.Equal(t, "replacement transcript", messages[0].Content)
			require.Equal(t, 1, f.events.count(), "replay must not delete replacement or publish again")
		})
	}
}

// @covers AC-PLUGINS-MANAGED-COORDINATION-013.4, AC-PLUGINS-MANAGED-COORDINATION-013.6
func TestManagedDeletionHostAdmittedFailures(t *testing.T) {
	for _, mode := range []string{"canvas_abort", "final_rollback", "revision_after_failure", "detach_after_failure", "missing_after_failure", "owner_uncertain"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			f := newManagedDeletionHostFixture(t, ctx)
			table := "tasks"
			if mode == "canvas_abort" {
				table = "canvas_lifecycle_metadata"
			}
			trigger := "CREATE TRIGGER fail_admitted_delete BEFORE DELETE ON " + table + " BEGIN SELECT RAISE(ABORT,'admitted failure'); END"
			if mode == "owner_uncertain" {
				trigger = `CREATE TRIGGER fail_admitted_delete BEFORE UPDATE ON task_resource_cleanup_jobs BEGIN SELECT RAISE(ABORT,'owner transition unavailable'); END`
			}
			_, err := f.database.ExecContext(ctx, trigger)
			require.NoError(t, err)
			result, err := f.manager.Delete(ctx, f.input)
			require.NoError(t, err)
			require.Equal(t, pluginsdk.CommandUnavailable, result.Status)
			wantReason := "managed_conversation_delete_admitted_failed"
			if mode == "owner_uncertain" {
				wantReason = "managed_conversation_delete_outcome_uncertain"
			}
			require.Equal(t, wantReason, result.Reason)
			require.Nil(t, result.Receipt)
			f.retained(t, ctx)
			require.Zero(t, f.events.count())
			_, canvasErr := f.canvas.Get(ctx, f.canvasID)
			if mode == "canvas_abort" {
				require.NoError(t, canvasErr)
			} else {
				require.Error(t, canvasErr, "admitted canvas preparation is a real partial effect")
			}
			require.Equal(t, "accepted", f.receipt(t, ctx).Receipt.State)
			jobs, err := f.other.ListTaskResourceCleanupJobs(ctx, f.descriptor.TaskID)
			require.NoError(t, err)
			require.Len(t, jobs, 1)
			claim, err := managed.DeletionEnvelope(jobs[0].ResourceSnapshot)
			require.NoError(t, err)
			require.NotEqual(t, managed.DeleteCommitted, claim.Phase)
			_, err = f.database.ExecContext(ctx, `DROP TRIGGER fail_admitted_delete`)
			require.NoError(t, err)
			if mode == "owner_uncertain" {
				require.Equal(t, models.TaskResourceCleanupStatePrepared, jobs[0].State)
				result, err = f.manager.Delete(ctx, f.input)
				require.NoError(t, err)
				require.Equal(t, wantReason, result.Reason)
				after, err := f.other.ListTaskResourceCleanupJobs(ctx, f.descriptor.TaskID)
				require.NoError(t, err)
				require.Equal(t, jobs[0].ResourceSnapshot, after[0].ResourceSnapshot, "same-operation retry never steals invocation owner")
				f.retained(t, ctx)
				return
			}
			require.Equal(t, models.TaskResourceCleanupStateCancelled, jobs[0].State)
			switch mode {
			case "revision_after_failure":
				spec := f.spec
				spec.ExpectedRevision = 1
				spec.BasePrompt = "accepted after admitted failure"
				_, _, err = f.independent.EnsureManaged(ctx, "coordinator", "installation-one", spec, "winner", "winner-digest")
				require.NoError(t, err)
			case "detach_after_failure":
				require.NoError(t, f.independent.DetachManagedForInstallation(ctx, "installation-one"))
			case "missing_after_failure":
				require.NoError(t, f.other.DeleteTask(ctx, f.descriptor.TaskID))
			default:
				return
			}
			result, err = f.manager.Delete(ctx, f.input)
			require.NoError(t, err)
			require.Equal(t, pluginsdk.CommandUnavailable, result.Status)
			require.Equal(t, wantReason, result.Reason)
			require.Nil(t, result.Receipt)
			require.Equal(t, "accepted", f.receipt(t, ctx).Receipt.State)
			require.Zero(t, f.events.count())
			if mode != "missing_after_failure" {
				f.retained(t, ctx)
			}
		})
	}
}
