package sqlite

import (
	"context"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestIncrementalShellPostgresProjection(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	require.NoError(t, repo.initSchema())
	seedIncrementalShellMessage(t, repo)
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(2)
	first, err := db.Conn(context.Background())
	require.NoError(t, err)
	second, err := db.Conn(context.Background())
	require.NoError(t, err)
	var firstPID, secondPID int
	require.NoError(t, first.QueryRowContext(context.Background(), `SELECT pg_backend_pid()`).Scan(&firstPID))
	require.NoError(t, second.QueryRowContext(context.Background(), `SELECT pg_backend_pid()`).Scan(&secondPID))
	require.NotEqual(t, firstPID, secondPID)
	require.NoError(t, first.Close())
	require.NoError(t, second.Close())
	patch, err := streams.EncodeShellOutputPatch(nil, streams.ShellExecOutput{Stdout: "雪\noutput"}, "epoch", true)
	require.NoError(t, err)
	event := incrementalShellTestEvent(t, 1, patch)
	_, err = repo.ReceiveAgentDeliveryEvent(context.Background(), event, 1)
	require.NoError(t, err)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Go(func() {
			_, e := repo.ProjectCanonicalAgentDeliveryEvent(context.Background(), event, completedRetentionEventEffect(event))
			errs <- e
		})
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		require.NoError(t, e)
	}
	message, err := repo.GetMessage(context.Background(), "shell-message")
	require.NoError(t, err)
	require.NoError(t, repo.RehydrateMessagePayload(context.Background(), message))
	snapshot, ok := models.ExtractShellExecOutput(message.Metadata)
	require.True(t, ok)
	require.Equal(t, "雪\noutput", snapshot.Stdout)
}
