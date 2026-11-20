package messagequeue

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

func newSortPolicyRaceRepositories(t *testing.T, backend string) (Repository, Repository) {
	t.Helper()
	if backend == "postgres" {
		first, second, _ := newTestPostgresRepoPair(t)
		return first, second
	}
	path := filepath.Join(t.TempDir(), "sort-policy-race.db")
	open := func() Repository {
		database, err := sqlx.Open("sqlite3", path+"?_foreign_keys=on&_busy_timeout=5000")
		require.NoError(t, err)
		database.SetMaxOpenConns(1)
		t.Cleanup(func() { require.NoError(t, database.Close()) })
		repo, err := NewSQLiteRepository(database, database)
		require.NoError(t, err)
		return repo
	}
	return open(), open()
}

// @covers AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.9, AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.10
func TestAutoMergePolicyAndCompactionRaceAcrossConnections(t *testing.T) {
	for _, backend := range []string{"sqlite", "postgres"} {
		for _, enable := range []bool{false, true} {
			name := map[bool]string{false: "disable", true: "enable-and-compact"}[enable]
			t.Run(backend+"/"+name, func(t *testing.T) {
				first, second := newSortPolicyRaceRepositories(t, backend)
				identity := QueueSessionIdentity{TaskID: "task", SessionID: "session", SessionIncarnationID: "inc"}
				seedQueueSessionIdentity(t, first, identity)
				seedQueueSessionIdentity(t, second, identity)
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				rows := []*QueuedMessage{sortTestEntry("A1", "A"), sortTestEntry("B1", "B")}
				content := "A2"
				expected := []string{"A1", "B1", "A2"}
				if enable {
					rows = append(rows, sortTestEntry("A2", "A"))
					content = "A3"
					expected = []string{"A1\n\nA2\n\nA3", "B1"}
				}
				for _, row := range rows {
					require.NoError(t, first.InsertForSession(ctx, identity, row, 0))
				}
				paused := &pausingSortReadRepository{Repository: first, autoMergeAdmissionRepository: first.(autoMergeAdmissionRepository), captured: make(chan struct{}), release: make(chan struct{})}
				release := sync.OnceFunc(func() { close(paused.release) })
				defer release()
				queue := NewService(paused, 10, logger.Default())
				global := AutoMergePolicy{Enabled: !enable}
				queue.SetGlobalAutoMergePolicy(global)
				writer := NewService(second, 10, logger.Default())
				writer.SetGlobalAutoMergePolicy(global)
				done := make(chan error, 1)
				go func() {
					_, err := queue.QueueMessageWithMetadataForSession(ctx, identity, content, "", QueuedByAgent, false, nil, map[string]any{MetadataSenderTaskID: "A"})
					done <- err
				}()
				select {
				case <-paused.captured:
				case <-ctx.Done():
					t.Fatal("admission did not capture the pre-write policy")
				}
				result, err := writer.SetSessionAutoMerge(ctx, identity, enable)
				require.NoError(t, err)
				require.Equal(t, enable, result.AutoMerge.Enabled)
				if enable {
					require.Equal(t, 1, result.CompactedCount)
				} else {
					require.Zero(t, result.CompactedCount)
				}
				release()
				require.NoError(t, <-done)
				after, err := second.ListBySession(ctx, "session")
				require.NoError(t, err)
				require.Equal(t, expected, sortTestContents(after))
				require.Equal(t, rows[0].ID, after[0].ID)
				require.Equal(t, rows[1].ID, after[1].ID)
			})
		}
	}
}
