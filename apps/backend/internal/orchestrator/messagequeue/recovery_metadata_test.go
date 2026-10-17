package messagequeue

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

// QR-REVIEW-001: lossless metadata evidence must detect adjacent large integers.
func TestRecoverySnapshotLargeIntegerSettingsChange(t *testing.T) {
	var changedDigest string
	for _, factory := range recoveryFactories() {
		t.Run(factory.name, func(t *testing.T) {
			repo := factory.new(t)
			identity := QueueSessionIdentity{"task", "session", "incarnation"}
			seedQueueSessionIdentity(t, repo, identity)
			entry := &QueuedMessage{ID: "entry", TaskID: "task", SessionID: "session", Content: strings.Repeat("x", 2048), QueuedAt: time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC), Metadata: map[string]interface{}{"setting": json.Number("9007199254740992")}}
			require.NoError(t, repo.InsertForSession(context.Background(), identity, entry, 0))
			queue := NewService(repo, 0, logger.Default())
			first, err := queue.RecoverySnapshotPage(context.Background(), identity, "workspace", "", 1024, "")
			require.NoError(t, err)
			switch r := repo.(type) {
			case *memoryRepository:
				r.entries["session"][0].Metadata["setting"] = json.Number("9007199254740993")
			case *sqliteRepository:
				_, err := r.db.Exec(r.db.Rebind(`UPDATE queued_messages SET metadata_json = ? WHERE id = ?`), `{"setting":9007199254740993}`, "entry")
				require.NoError(t, err)
			}
			for _, cursor := range []string{"", first.NextCursor} {
				page, err := queue.RecoverySnapshotPage(context.Background(), identity, "workspace", cursor, 1024, first.SnapshotSHA256)
				require.ErrorIs(t, err, ErrRecoveryChanged)
				require.Nil(t, page)
			}
			changed, err := queue.RecoverySnapshotManifest(context.Background(), identity, "workspace")
			require.NoError(t, err)
			if changedDigest == "" {
				changedDigest = changed.SnapshotSHA256
			}
			require.Equal(t, changedDigest, changed.SnapshotSHA256, "equivalent large-number metadata must hash identically across repositories")
		})
	}
}
