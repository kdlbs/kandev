package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-INCREMENTAL-SHELL-001.1, AC-PLATFORM-INCREMENTAL-SHELL-001.3
func TestIncrementalShellStorageEndurance(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedIncrementalShellMessage(t, repo)
	_, err := repo.db.Exec(`CREATE TABLE shell_write_measurement (bytes INTEGER NOT NULL);
INSERT INTO shell_write_measurement VALUES (0);
CREATE TRIGGER measure_shell_insert AFTER INSERT ON task_message_shell_output_chunks BEGIN
 UPDATE shell_write_measurement SET bytes=bytes+length(NEW.content); END;
CREATE TRIGGER measure_shell_update AFTER UPDATE ON task_message_shell_output_chunks BEGIN
 UPDATE shell_write_measurement SET bytes=bytes+length(NEW.content); END;
CREATE TABLE shell_metadata_measurement (head_bytes INTEGER NOT NULL, message_bytes INTEGER NOT NULL);
INSERT INTO shell_metadata_measurement VALUES (0,0);
CREATE TRIGGER measure_shell_head_insert AFTER INSERT ON task_message_shell_output BEGIN
 UPDATE shell_metadata_measurement SET head_bytes=head_bytes+length(NEW.header)+length(NEW.identity); END;
CREATE TRIGGER measure_shell_head_update AFTER UPDATE ON task_message_shell_output BEGIN
 UPDATE shell_metadata_measurement SET head_bytes=head_bytes+length(NEW.header)+length(NEW.identity); END;
CREATE TRIGGER measure_shell_message AFTER UPDATE OF metadata ON task_session_messages BEGIN
 UPDATE shell_metadata_measurement SET message_bytes=message_bytes+length(NEW.metadata); END;`)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "endurance.bbolt")
	store, err := journal.Open(journal.Config{Path: path})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	ctx := context.Background()
	output := streams.ShellExecOutput{Stdout: strings.Repeat("x", streams.MaxShellOutputFieldBytes), Truncated: true, StdoutTruncated: true}
	var state *streams.ShellOutputState
	var journalBytes, inboxBytes, envelopeBytes int64
	var last *models.AgentDeliveryEvent
	for index := range 7200 {
		delta := fmt.Sprintf("%016d", index)
		output.Stdout = output.Stdout[len(delta):] + delta
		patch, encodeErr := streams.EncodeShellOutputPatch(state, output, "endurance", index%512 == 0)
		require.NoError(t, encodeErr)
		state, err = streams.ApplyShellOutputPatch(state, patch)
		require.NoError(t, err)
		last = incrementalShellTestEvent(t, int64(index+1), patch)
		record, appendErr := store.Append(ctx, journal.Event{SessionID: last.SessionID, IncarnationID: last.IncarnationID, HarnessGeneration: 1, StreamID: last.StreamID, Type: last.EventType, Payload: last.Payload})
		require.NoError(t, appendErr)
		encoded, marshalErr := json.Marshal(record)
		require.NoError(t, marshalErr)
		journalBytes += int64(len(encoded))
		inboxBytes += int64(len(last.Payload))
		_, err = repo.ReceiveAgentDeliveryEvent(ctx, last, last.Sequence)
		require.NoError(t, err)
		_, err = repo.ProjectCanonicalAgentDeliveryEvent(ctx, last, completedRetentionEventEffect(last))
		require.NoError(t, err)
		lean := *patch
		lean.Stdout, lean.Stderr = nil, nil
		envelope := incrementalShellTestEvent(t, last.Sequence, &lean)
		encoded, err = json.Marshal(journal.Event{SessionID: last.SessionID, IncarnationID: last.IncarnationID, HarnessGeneration: 1, StreamID: last.StreamID, Sequence: record.Sequence, Type: last.EventType, Payload: envelope.Payload, CreatedAt: record.CreatedAt})
		require.NoError(t, err)
		envelopeBytes += int64(len(encoded))
	}
	assertShellEnduranceReplay(t, repo, store, last, output.Stdout)
	var canonicalBytes int64
	require.NoError(t, repo.db.Get(&canonicalBytes, `SELECT bytes FROM shell_write_measurement`))
	const newBytes = 7200 * 16
	const bound = 12*newBytes + 2*(512<<10)*(1+7200/512)
	require.LessOrEqual(t, journalBytes, int64(bound)+envelopeBytes)
	require.Less(t, journalBytes, int64(32<<20))
	require.Less(t, inboxBytes, int64(32<<20))
	require.LessOrEqual(t, canonicalBytes, int64(2*newBytes+8192*7200+2*(512<<10)))
	var dbPath string
	require.NoError(t, repo.db.QueryRow(`SELECT file FROM pragma_database_list WHERE name='main'`).Scan(&dbPath))
	t.Logf("new_text_bytes=%d journal_serialized_bytes=%d inbox_serialized_bytes=%d canonical_chunk_write_bytes=%d envelope_bytes=%d", newBytes, journalBytes, inboxBytes, canonicalBytes, envelopeBytes)
	var headBytes, messageBytes, effects int64
	require.NoError(t, repo.db.QueryRow(`SELECT head_bytes,message_bytes FROM shell_metadata_measurement`).Scan(&headBytes, &messageBytes))
	require.NoError(t, repo.db.Get(&effects, `SELECT COUNT(*) FROM agent_delivery_effects`))
	t.Logf("canonical_head_metadata_write_bytes=%d message_metadata_write_bytes=%d effect_rows=%d", headBytes, messageBytes, effects)
	for _, file := range []string{path, dbPath, dbPath + "-wal"} {
		info, statErr := os.Stat(file)
		if os.IsNotExist(statErr) {
			t.Logf("fixture_file=%s size_bytes=0", filepath.Base(file))
			continue
		}
		require.NoError(t, statErr)
		t.Logf("fixture_file=%s size_bytes=%d (size, not cumulative physical writes)", filepath.Base(file), info.Size())
	}
}

func assertShellEnduranceReplay(t *testing.T, repo *Repository, store *journal.Journal, last *models.AgentDeliveryEvent, expected string) {
	t.Helper()
	ctx := context.Background()
	// A new repository has no in-memory receipt cache. Replay the lost-ACK prefix.
	cold := NewWithInitializedDB(repo.db, repo.db, nil)
	records, _, err := store.Replay(ctx, last.StreamID, uint64(last.Sequence-1), 1)
	require.NoError(t, err)
	require.Len(t, records, 1)
	replay := *last
	replay.Payload = records[0].Payload
	_, err = cold.ReceiveAgentDeliveryEvent(ctx, &replay, last.Sequence)
	require.NoError(t, err)
	_, err = cold.ProjectCanonicalAgentDeliveryEvent(ctx, &replay, completedRetentionEventEffect(last))
	require.NoError(t, err)
	message, err := cold.GetMessage(ctx, "shell-message")
	require.NoError(t, err)
	require.NoError(t, cold.RehydrateMessagePayload(ctx, message))
	output, ok := models.ExtractShellExecOutput(message.Metadata)
	require.True(t, ok)
	require.True(t, expected == output.Stdout)
	replay.Payload = []byte(`{"changed":"duplicate"}`)
	_, err = cold.ReceiveAgentDeliveryEvent(ctx, &replay, last.Sequence)
	require.Error(t, err, "changed replay must never replace accepted bytes")
}
