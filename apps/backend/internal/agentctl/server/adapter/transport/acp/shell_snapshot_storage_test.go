package acp

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-BOUNDED-SESSION-HISTORY-002.4
// @covers AC-PLATFORM-DELIVERY-RETENTION-005.7
func TestShellSnapshotDurableStorage(t *testing.T) {
	events, want := collectStorageSnapshots(t)
	require.LessOrEqual(t, len(events), 12)
	dir := t.TempDir()
	logPath := filepath.Join(dir, "delivery.bbolt")
	j, err := journal.Open(journal.Config{Path: logPath})
	require.NoError(t, err)
	t.Cleanup(func() { _ = j.Close() })
	var entries []journal.Event
	var bytes int
	for _, event := range events {
		payload, marshalErr := json.Marshal(event)
		require.NoError(t, marshalErr)
		bytes += len(payload)
		entries = append(entries, journal.Event{SessionID: "snapshot-session", IncarnationID: "snapshot-incarnation",
			HarnessGeneration: 1, StreamID: "snapshot-stream", Type: event.Type, Payload: payload})
	}
	require.Less(t, bytes, 4<<20)
	committed, err := j.AppendBatch(t.Context(), entries)
	require.NoError(t, err)
	require.NoError(t, j.Close())
	j, err = journal.Open(journal.Config{Path: logPath})
	require.NoError(t, err)
	replayed, stream, err := j.Replay(t.Context(), "snapshot-stream", 0, 100)
	require.NoError(t, err)
	require.Equal(t, committed, replayed, "committed snapshots must survive journal restart unchanged")
	require.Equal(t, uint64(len(events)), stream.HighWater)
	repo, writer := snapshotStorageRepository(t, dir)
	projectStorageSnapshots(t, repo, replayed, events)
	before := readStorageShellOutput(t, repo)
	require.Equal(t, want, before.Stdout)
	require.Equal(t, "warning", before.Stderr)
	require.Equal(t, 0, *before.ExitCode)
	retireStorageSnapshots(t, repo, writer, len(events))
	require.Equal(t, before, readStorageShellOutput(t, repo), "transport cleanup must preserve canonical command output")
	cursor, err := repo.GetAgentDeliveryCursor(t.Context(), "snapshot-stream")
	require.NoError(t, err)
	require.Equal(t, int64(len(events)), cursor.RetiredSequence)
	old := storageInboxEvent(replayed[0])
	_, err = repo.ReceiveAgentDeliveryEvent(t.Context(), old, int64(stream.HighWater))
	require.ErrorIs(t, err, repoerrors.ErrAgentDeliveryReplayExpired)
	t.Logf("source_updates=2000 persisted_events=%d persisted_bytes=%d retired_events=%d", len(events), bytes, cursor.RetiredSequence)
}

func retireStorageSnapshots(t *testing.T, repo *sqlite.Repository, writer *sqlx.DB, count int) {
	t.Helper()
	var retired int64
	for index := range count * 3 {
		options := models.AgentDeliveryRetentionBatchOptions{
			PayloadGrace: time.Nanosecond, ReplayAge: time.Nanosecond, MaxCandidates: 1,
		}
		if index == 0 {
			options.WorkTimeout = time.Nanosecond
		}
		result, batchErr := repo.RunAgentDeliveryRetentionBatch(t.Context(), options)
		if index == 0 {
			require.ErrorIs(t, batchErr, context.DeadlineExceeded)
			require.Zero(t, result.EventRowsRetired)
		}
		if batchErr != nil {
			require.ErrorIs(t, batchErr, context.DeadlineExceeded)
			require.NoError(t, t.Context().Err())
		}
		previous := retired
		var remaining int
		retired, remaining = settledStorageRetirement(t, writer)
		require.Equal(t, int64(count), retired+int64(remaining))
		if index == 0 {
			require.Zero(t, retired)
		}
		if batchErr == nil {
			require.Equal(t, retired-previous, int64(result.EventRowsRetired))
		}
		if retired == int64(count) {
			break
		}
	}
	require.Equal(t, int64(count), retired)
}

func settledStorageRetirement(t *testing.T, writer *sqlx.DB) (int64, int) {
	t.Helper()
	tx, err := writer.BeginTxx(t.Context(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	// The writer lease settles a possible commit whose acknowledgement expired.
	_, err = tx.ExecContext(t.Context(), `UPDATE agent_delivery_retention_state SET updated_at=updated_at WHERE id=1`)
	require.NoError(t, err)
	var retired int64
	var remaining int
	require.NoError(t, tx.GetContext(t.Context(), &retired, `SELECT retired_sequence FROM agent_delivery_cursors WHERE stream_id='snapshot-stream'`))
	require.NoError(t, tx.GetContext(t.Context(), &remaining, `SELECT COUNT(*) FROM agent_delivery_inbox WHERE stream_id='snapshot-stream'`))
	require.NoError(t, tx.Rollback())
	return retired, remaining
}

func collectStorageSnapshots(t *testing.T) ([]AgentEvent, string) {
	t.Helper()
	var events []AgentEvent
	var want string
	synctest.Test(t, func(t *testing.T) {
		a := newTestAdapter()
		defer func() { require.NoError(t, a.Close()) }()
		a.sendUpdate(AgentEvent{Type: streams.EventTypeAgentCapabilities, SessionID: "snapshot-session", SupportsImage: true})
		seedSnapshotTool(a, "shell")
		a.activeToolCalls["shell"].ShellExec().Output = &streams.ShellExecOutput{Stdout: strings.Repeat("x", maxShellOutputBytes)}
		for range 2000 {
			a.handleACPUpdate(shellSnapshotNotification(t, "shell", shellDelta("new output\n")), 1)
			events = append(events, drainEvents(a)...)
			time.Sleep(time.Millisecond)
		}
		want = a.activeToolCalls["shell"].ShellExec().Output.Stdout
		final := shellSnapshotNotification(t, "shell", nil)
		final.Update.ToolCallUpdate.RawOutput = map[string]any{"stdout": want, "stderr": "warning", "exit_code": 0}
		a.handleACPUpdate(final, 1)
		require.True(t, a.syncNotifQueue())
		events = append(events, drainEvents(a)...)
	})
	return events, want
}

func snapshotStorageRepository(t *testing.T, dir string) (*sqlite.Repository, *sqlx.DB) {
	t.Helper()
	w, err := db.OpenSQLite(filepath.Join(dir, "backend.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = w.Close() })
	r, err := db.OpenSQLiteReader(filepath.Join(dir, "backend.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })
	writer := sqlx.NewDb(w, "sqlite3")
	repo, err := sqlite.NewWithDB(writer, sqlx.NewDb(r, "sqlite3"), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo.Close() })
	require.NoError(t, repo.CreateWorkspace(t.Context(), &models.Workspace{ID: "snapshot-workspace", Name: "Snapshot fixture"}))
	require.NoError(t, repo.CreateTask(t.Context(), &models.Task{ID: "snapshot-task", WorkspaceID: "snapshot-workspace", Title: "Snapshot fixture"}))
	require.NoError(t, repo.CreateTaskSession(t.Context(), &models.TaskSession{ID: "snapshot-session", TaskID: "snapshot-task", QueueIncarnationID: "snapshot-incarnation"}))
	require.NoError(t, repo.CreateTurn(t.Context(), &models.Turn{ID: "snapshot-turn", TaskID: "snapshot-task", TaskSessionID: "snapshot-session"}))
	return repo, writer
}

func projectStorageSnapshots(t *testing.T, repo *sqlite.Repository, entries []journal.Event, events []AgentEvent) {
	t.Helper()
	created := false
	for index, entry := range entries {
		require.Equal(t, uint64(index+1), entry.Sequence)
		inbox := storageInboxEvent(entry)
		inserted, err := repo.ReceiveAgentDeliveryEvent(t.Context(), inbox, int64(len(entries)))
		require.NoError(t, err)
		require.True(t, inserted)
		if events[index].Type == streams.EventTypeToolUpdate {
			raw, marshalErr := json.Marshal(events[index].NormalizedPayload)
			require.NoError(t, marshalErr)
			var normalized map[string]any
			require.NoError(t, json.Unmarshal(raw, &normalized))
			message := &models.Message{ID: "snapshot-message", TaskID: "snapshot-task", TaskSessionID: "snapshot-session",
				TurnID: "snapshot-turn", Type: models.MessageTypeToolExecute, AuthorType: models.MessageAuthorAgent,
				Metadata: map[string]any{"normalized": normalized, "status": events[index].ToolStatus, "tool_call_id": "shell"}}
			if created {
				err = repo.UpdateMessage(t.Context(), message)
			} else {
				err = repo.CreateMessage(t.Context(), message)
				created = true
			}
			require.NoError(t, err)
		}
		now := time.Now().UTC()
		_, err = repo.ProjectAgentDeliveryEvent(t.Context(), inbox, &models.AgentDeliveryEffect{
			EffectKey: "agent_delivery.event:" + entry.StreamID + ":" + strconv.FormatUint(entry.Sequence, 10),
			StreamID:  entry.StreamID, Sequence: int64(entry.Sequence), EffectType: "agent_delivery.event",
			State: models.DeliveryEffectCompleted, CreatedAt: now, CompletedAt: &now,
		})
		require.NoError(t, err)
		inserted, err = repo.ReceiveAgentDeliveryEvent(t.Context(), inbox, int64(len(entries)))
		require.NoError(t, err)
		require.False(t, inserted)
	}
}

func storageInboxEvent(entry journal.Event) *models.AgentDeliveryEvent {
	return &models.AgentDeliveryEvent{SessionID: entry.SessionID, IncarnationID: entry.IncarnationID,
		HarnessGeneration: int64(entry.HarnessGeneration), StreamID: entry.StreamID, Sequence: int64(entry.Sequence),
		EventType: entry.Type, Payload: entry.Payload, ReceivedAt: entry.CreatedAt}
}

func readStorageShellOutput(t *testing.T, repo *sqlite.Repository) models.ShellExecOutputSnapshot {
	t.Helper()
	message, err := repo.GetMessage(t.Context(), "snapshot-message")
	require.NoError(t, err)
	require.NoError(t, repo.RehydrateMessagePayload(t.Context(), message))
	output, ok := models.ExtractShellExecOutput(message.Metadata)
	require.True(t, ok)
	return output
}
