package messagequeue

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

func TestGetStatusReturnsDetachedEntrySnapshots(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stderr"})
	require.NoError(t, err)
	svc := NewServiceMemory(log)
	ctx := context.Background()
	_, err = svc.QueueMessageWithMetadata(ctx, "session", "task", "content", "", QueuedByUser, false,
		[]MessageAttachment{{AttachmentID: "attachment-1", Name: "original.txt"}},
		map[string]interface{}{"nested": map[string]interface{}{"value": "original"}},
	)
	require.NoError(t, err)

	status := svc.GetStatus(ctx, "session")
	require.Len(t, status.Entries, 1)
	status.Entries[0].Attachments[0].Name = "mutated.txt"
	status.Entries[0].Metadata["nested"].(map[string]interface{})["value"] = "mutated"

	fresh := svc.GetStatus(ctx, "session")
	require.Equal(t, "original.txt", fresh.Entries[0].Attachments[0].Name)
	require.Equal(t, "original", fresh.Entries[0].Metadata["nested"].(map[string]interface{})["value"])
}
func TestGetStatusReturnsDetachedTypedMetadataSnapshots(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stderr"})
	require.NoError(t, err)
	svc := NewServiceMemory(log)
	ctx := context.Background()
	_, err = svc.QueueMessageWithMetadata(ctx, "session-typed", "task", "content", "", QueuedByUser, false, nil,
		map[string]interface{}{
			"tags":   []string{"original"},
			"labels": map[string]string{"name": "original"},
		},
	)
	require.NoError(t, err)

	status := svc.GetStatus(ctx, "session-typed")
	require.Len(t, status.Entries, 1)
	status.Entries[0].Metadata["tags"].([]string)[0] = "mutated"
	status.Entries[0].Metadata["labels"].(map[string]string)["name"] = "mutated"

	fresh := svc.GetStatus(ctx, "session-typed")
	require.Equal(t, []string{"original"}, fresh.Entries[0].Metadata["tags"])
	require.Equal(t, map[string]string{"name": "original"}, fresh.Entries[0].Metadata["labels"])
}
func TestSnapshotSessionReturnsDetachedEntrySnapshots(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stderr"})
	require.NoError(t, err)
	svc := NewServiceMemory(log)
	ctx := context.Background()
	_, err = svc.QueueMessageWithMetadata(ctx, "session-snapshot", "task", "content", "", QueuedByUser, false,
		[]MessageAttachment{{AttachmentID: "attachment-1", Name: "original.txt"}},
		map[string]interface{}{"nested": map[string]interface{}{"value": "original"}},
	)
	require.NoError(t, err)

	entries, _, err := svc.SnapshotSession(ctx, "session-snapshot")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	entries[0].Attachments[0].Name = "mutated.txt"
	entries[0].Metadata["nested"].(map[string]interface{})["value"] = "mutated"

	fresh, _, err := svc.SnapshotSession(ctx, "session-snapshot")
	require.NoError(t, err)
	require.Equal(t, "original.txt", fresh[0].Attachments[0].Name)
	require.Equal(t, "original", fresh[0].Metadata["nested"].(map[string]interface{})["value"])
}
