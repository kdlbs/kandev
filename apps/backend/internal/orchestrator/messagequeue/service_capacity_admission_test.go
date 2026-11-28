package messagequeue

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

// @covers AC-AGENTS-SESSION-CEILING-003.10
func TestSnapshotProjectsKnownCapacityAdmissionReason(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	ctx := t.Context()
	identity := QueueSessionIdentity{TaskID: "task", SessionID: "session", SessionIncarnationID: "incarnation"}
	seedQueueSessionIdentity(t, repo, identity)
	for _, reason := range []string{"session_capacity", "", "other_reason"} {
		metadata := map[string]interface{}{}
		if reason != "" {
			metadata["admission_reason"] = reason
		}
		insertTestEntry(t, repo, identity.SessionID, identity.TaskID, "pending", QueuedByUser, nil, metadata)
	}

	svc := NewService(repo, 10, logger.Default())
	snapshot, err := svc.Snapshot(ctx, identity)
	require.NoError(t, err)
	for _, status := range []*QueueStatus{snapshot, svc.GetStatus(ctx, identity.SessionID)} {
		payload, err := json.Marshal(status)
		require.NoError(t, err)
		var wire struct {
			Entries []map[string]json.RawMessage `json:"entries"`
		}
		require.NoError(t, json.Unmarshal(payload, &wire))
		require.Len(t, wire.Entries, 3)
		require.JSONEq(t, `"session_capacity"`, string(wire.Entries[0]["admission_reason"]))
		require.NotContains(t, wire.Entries[1], "admission_reason")
		require.NotContains(t, wire.Entries[2], "admission_reason")
	}
}

// @covers AC-AGENTS-SESSION-CEILING-003.10
func TestCapacityAdmissionReasonSurvivesEditAndDispatchRelease(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	ctx := t.Context()
	identity := QueueSessionIdentity{TaskID: "task", SessionID: "session", SessionIncarnationID: "incarnation"}
	seedQueueSessionIdentity(t, repo, identity)
	entry := insertTestEntry(t, repo, identity.SessionID, identity.TaskID, "pending", QueuedByUser, nil,
		map[string]interface{}{MetadataAdmissionReason: AdmissionReasonSessionCapacity})
	svc := NewService(repo, 10, logger.Default())
	require.NoError(t, svc.UpdateMessageWithMetadataForSession(ctx, identity, entry.ID, "edited", nil, nil, QueuedByUser))

	reserved, ok, _, err := svc.ReserveQueuedForDeliveryWithAutoRunForSession(ctx, identity)
	require.NoError(t, err)
	require.True(t, ok)
	status, err := svc.Snapshot(ctx, identity)
	require.NoError(t, err)
	require.Empty(t, status.Entries)

	require.NoError(t, svc.ReleaseQueuedDeliveryForSession(ctx, identity, reserved))
	status, err = svc.Snapshot(ctx, identity)
	require.NoError(t, err)
	entries := status.ProjectedEntries()
	require.Len(t, entries, 1)
	require.Equal(t, entry.ID, entries[0].ID)
	require.Equal(t, "edited", entries[0].Content)
	require.Equal(t, "session_capacity", entries[0].AdmissionReason)

	reserved, ok, _, err = svc.ReserveQueuedForDeliveryWithAutoRunForSession(ctx, identity)
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, svc.AcknowledgeQueuedForSession(ctx, identity, reserved))
	status, err = svc.Snapshot(ctx, identity)
	require.NoError(t, err)
	require.Empty(t, status.Entries)
}
