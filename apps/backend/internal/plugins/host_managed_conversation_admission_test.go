package plugins

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/state"
	pluginstore "github.com/kandev/kandev/internal/plugins/store"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/testutil"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"github.com/stretchr/testify/require"
)

var managedHostAdmissionTemplate = testutil.NewSQLiteTemplate(func(database *sqlx.DB) error {
	if _, err := tasksqlite.NewWithDB(database, database, nil); err != nil {
		return err
	}
	if _, err := officesqlite.NewWithDB(database, database, nil); err != nil {
		return err
	}
	_, err := workflowrepo.NewWithDB(database, database, nil)
	return err
})

type managedHostPrimaryGate struct {
	*tasksqlite.Repository
	armed         atomic.Bool
	seen, release chan struct{}
}

func (g *managedHostPrimaryGate) GetPrimarySessionByTaskID(ctx context.Context, id string) (*models.TaskSession, error) {
	row, err := g.Repository.GetPrimarySessionByTaskID(ctx, id)
	if err == nil && g.armed.CompareAndSwap(true, false) {
		close(g.seen)
		select {
		case <-g.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return row, err
}

// @covers AC-PLUGINS-MANAGED-COORDINATION-002.1, AC-PLUGINS-MANAGED-COORDINATION-002.4, AC-PLUGINS-MANAGED-COORDINATION-002.7, AC-PLUGINS-MANAGED-COORDINATION-002.8
func TestManagedConversationHostAdmissionReceipts(t *testing.T) {
	database, path := managedHostAdmissionTemplate.Open(t)
	raw, err := db.OpenSQLite(path)
	require.NoError(t, err)
	independent := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { require.NoError(t, independent.Close()) })
	repo := tasksqlite.NewWithInitializedDB(database, database, nil)
	other := tasksqlite.NewWithInitializedDB(independent, independent, nil)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace-one", Name: "Workspace"}))
	commands, err := state.NewCommandStore(db.NewPool(database, database))
	require.NoError(t, err)
	pluginState, err := state.NewStore(db.NewPool(database, database))
	require.NoError(t, err)
	record := &pluginstore.Record{Manifest: manifest.Manifest{ID: "coordinator", Capabilities: manifest.Capabilities{APIRead: []string{"managed_agent_conversations"}, APIWrite: []string{"managed_agent_conversations"}}}, InstallationID: "installation-one", Status: StatusActive}
	registry := NewRegistry()
	registry.Add(record)
	svc := NewService(pluginstore.NewFSStore(t.TempDir()), registry, nil, testLogger(t))
	require.NoError(t, svc.SetPluginsDir(t.TempDir()))
	svc.SetExactCommandStore(commands)
	t.Cleanup(func() { require.NoError(t, svc.Close()) })
	digest := ManifestCapabilityDigest(record.Manifest)
	_, err = svc.approvalGrant(record.InstallationID, "workspace-one", 1, digest, []string{"host.v2.read:managed_agent_conversations", "host.v2.write:managed_agent_conversations"}, "human", "grant", "approval-one")
	require.NoError(t, err)
	svc.SetManagedAgentConversations(taskservice.NewAgentConversationService(repo, repo, nil, pluginState, nil))
	manager := svc.hostForPlugin(record.ID).(pluginsdk.ExactHost).ManagedAgentConversations()
	input := pluginsdk.ManagedAgentConversationSpec{RequestID: "create-request", IdempotencyKey: "create", WorkspaceID: "workspace-one", InstanceKey: "lead", ApprovalRevision: 1, ManifestDigest: digest, AgentProfileID: "profile", BasePrompt: "original"}
	result, first, err := manager.Ensure(ctx, input)
	require.NoError(t, err)
	require.Equal(t, pluginsdk.CommandApplied, result.Status)
	require.NotNil(t, result.Receipt)
	require.Equal(t, uint64(1), first.Revision)
	replay, same, err := manager.Ensure(ctx, input)
	require.NoError(t, err)
	require.Equal(t, result.Status, replay.Status)
	require.Equal(t, first, same)
	input.RequestID, input.IdempotencyKey, input.ExpectedRevision = "nochange-request", "nochange", first.Revision
	result, current, err := manager.Ensure(ctx, input)
	require.NoError(t, err)
	require.Equal(t, pluginsdk.CommandNoChange, result.Status)
	require.Equal(t, first.Revision, current.Revision)
	require.NoError(t, other.UpdateTaskSessionState(ctx, first.SessionID, models.TaskSessionStateRunning, ""))
	input.RequestID, input.IdempotencyKey, input.BasePrompt = "busy-request", "busy", "rejected"
	result, _, err = manager.Ensure(ctx, input)
	require.NoError(t, err)
	require.Equal(t, pluginsdk.CommandConflict, result.Status)
	require.NotNil(t, result.Receipt)
	primary, err := other.GetTaskSession(ctx, first.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateRunning, primary.State)
	require.NoError(t, other.UpdateTaskSessionState(ctx, first.SessionID, models.TaskSessionStateWaitingForInput, ""))
	input.RequestID, input.IdempotencyKey, input.BasePrompt = "winner-request", "winner", "winner"
	result, winner, err := manager.Ensure(ctx, input)
	require.NoError(t, err)
	require.Equal(t, pluginsdk.CommandApplied, result.Status)
	require.Equal(t, uint64(2), winner.Revision)
	input.RequestID, input.IdempotencyKey, input.BasePrompt = "stale-request", "stale", "loser"
	result, _, err = manager.Ensure(ctx, input)
	require.NoError(t, err)
	require.Equal(t, pluginsdk.CommandConflict, result.Status)
	task, err := other.GetTask(ctx, first.TaskID)
	require.NoError(t, err)
	require.Equal(t, "winner", task.Metadata["kandev.base_prompt"])
	_, err = database.Exec(`CREATE TRIGGER fail_managed_receipt BEFORE UPDATE ON plugin_host_command_receipts BEGIN SELECT RAISE(ABORT,'receipt unavailable'); END`)
	require.NoError(t, err)
	input.RequestID, input.IdempotencyKey, input.BasePrompt, input.ExpectedRevision = "ack-request", "ack", "committed without acknowledgement", winner.Revision
	result, committed, err := manager.Ensure(ctx, input)
	require.NoError(t, err)
	require.Equal(t, pluginsdk.CommandUnavailable, result.Status)
	require.Equal(t, "command_receipt_unavailable", result.Reason)
	require.Equal(t, uint64(3), committed.Revision)
	task, err = other.GetTask(ctx, first.TaskID)
	require.NoError(t, err)
	require.Equal(t, input.BasePrompt, task.Metadata["kandev.base_prompt"])
	_, err = database.Exec(`DROP TRIGGER fail_managed_receipt`)
	require.NoError(t, err)
	result, recovered, err := manager.Ensure(ctx, input)
	require.NoError(t, err)
	require.Equal(t, pluginsdk.CommandAlreadyApplied, result.Status)
	require.NotNil(t, result.Receipt)
	require.Equal(t, committed, recovered)
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	gate := &managedHostPrimaryGate{Repository: repo, seen: make(chan struct{}), release: make(chan struct{})}
	var once sync.Once
	var workers sync.WaitGroup
	release := func() { once.Do(func() { close(gate.release) }) }
	t.Cleanup(func() { cancel(); release(); workers.Wait() })
	svc.SetManagedAgentConversations(taskservice.NewAgentConversationService(repo, gate, nil, pluginState, nil))
	input.RequestID, input.IdempotencyKey, input.ExpectedRevision, input.BasePrompt = "delayed-request", "delayed", committed.Revision, "delayed Host settings"
	gate.armed.Store(true)
	type hostResult struct {
		result *pluginsdk.CommandResult
		err    error
	}
	done := make(chan hostResult, 1)
	workers.Add(1)
	go func() {
		defer workers.Done()
		result, _, err := manager.Ensure(ctx, input)
		done <- hostResult{result, err}
	}()
	select {
	case <-gate.seen:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	winnerSpec := input
	winnerSpec.BasePrompt = "independently accepted winner"
	independentSvc := taskservice.NewAgentConversationService(other, other, nil, pluginState, nil)
	accepted, _, err := independentSvc.EnsureManaged(ctx, record.ID, record.InstallationID, winnerSpec, "independent-winner", "independent-winner-digest")
	require.NoError(t, err)
	require.Equal(t, committed.Revision+1, accepted.Revision)
	release()
	loser := <-done
	workers.Wait()
	require.NoError(t, loser.err)
	require.Equal(t, pluginsdk.CommandConflict, loser.result.Status)
	require.NotNil(t, loser.result.Receipt)
	task, err = other.GetTask(ctx, first.TaskID)
	require.NoError(t, err)
	require.Equal(t, winnerSpec.BasePrompt, task.Metadata["kandev.base_prompt"])
}
