package models

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestReduceCleanupSnapshotKeepsUnreducibleSnapshots(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name, snapshot, reason string
	}{
		{"empty", ``, payloadMalformed},
		{"invalid json", `{"workspace_id":`, payloadMalformed},
		{"not an object", `[1,2]`, payloadMalformed},
		{"null", `null`, payloadMalformed},
		{"numeric workspace", `{"workspace_id":7,"progress":"x"}`, payloadMalformed},
		{"manifest object", `{"archive_source_manifest":{"manifest":1},"progress":"x"}`, payloadMalformed},
		{"worktrees string", `{"worktrees":"w1","progress":"x"}`, payloadMalformed},
		{"already reduced", `{"workspace_id":"ws","snapshot_reduced":{"version":1},"progress":"x"}`, "already_removed"},
		{"only retained keys", `{"archive_source_manifest":[],"workspace_id":"ws","worktrees":[]}`, "no_payload"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReduceCleanupSnapshot(tt.snapshot, at)
			require.NoError(t, err)
			require.Equal(t, tt.reason, got.Reason)
			require.Equal(t, tt.snapshot, got.Snapshot)
			require.Zero(t, got.RemovedBytes)
		})
	}
}

func TestReduceCleanupSnapshotKeepsEvidenceBytes(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	snapshot := `{"progress":"` + strings.Repeat("x", 512) + `","worktrees":[ {"id":"w1"} ],` +
		`"workspace_id":"ws-1","archive_source_manifest":[{"task_id":"t", "cleanup_job_id":"j"}],"sessions":[{"id":"s"}]}`
	got, err := ReduceCleanupSnapshot(snapshot, at)
	require.NoError(t, err)
	require.Empty(t, got.Reason)
	require.Equal(t, int64(len(snapshot)-len(got.Snapshot)), got.RemovedBytes)
	require.Positive(t, got.RemovedBytes)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(got.Snapshot), &fields))
	require.Equal(t, `[ {"id":"w1"} ]`, string(fields["worktrees"]))
	require.Equal(t, `"ws-1"`, string(fields["workspace_id"]))
	require.Equal(t, `[{"task_id":"t", "cleanup_job_id":"j"}]`, string(fields["archive_source_manifest"]))
	require.NotContains(t, fields, "progress")
	require.NotContains(t, fields, "sessions")
	require.Contains(t, string(fields[snapshotReducedKey]), `"removed_fields":["progress","sessions"]`)

	again, err := ReduceCleanupSnapshot(snapshot, at)
	require.NoError(t, err)
	require.Equal(t, got, again, "the same input must encode to the same bytes")

	second, err := ReduceCleanupSnapshot(got.Snapshot, at.Add(time.Hour))
	require.NoError(t, err)
	require.Equal(t, "already_removed", second.Reason)
	require.Equal(t, got.Snapshot, second.Snapshot)
}
