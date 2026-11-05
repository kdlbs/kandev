package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func newPreparedSessionRepo(t *testing.T, postgres bool) *Repository {
	t.Helper()
	if !postgres {
		return newRepoForSessionTests(t)
	}
	conn := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	// The concurrency case needs independent connections so the task-lock
	// election can race instead of serializing on a single-connection pool.
	conn.SetMaxOpenConns(4)
	conn.SetMaxIdleConns(4)
	repo, err := NewWithDB(conn, conn, nil)
	require.NoError(t, err)
	return repo
}

func TestPreparedSessionPrimaryElection(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		name := "SQLite"
		if postgres {
			name = "Postgres"
		}
		t.Run(name, func(t *testing.T) {
			runPreparedSessionPrimaryElection(t, newPreparedSessionRepo(t, postgres))
		})
	}
}

func runPreparedSessionPrimaryElection(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	const taskID = "task-prepared-election"
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Prepared election"}))

	first := &models.TaskSession{ID: "prepared-first", TaskID: taskID, IsPrimary: true}
	require.NoError(t, repo.CreateTaskSession(ctx, first))
	require.True(t, first.IsPrimary, "first prepared session must commit as primary")

	second := &models.TaskSession{ID: "prepared-second", TaskID: taskID, IsPrimary: true}
	require.NoError(t, repo.CreateTaskSession(ctx, second))
	require.False(t, second.IsPrimary, "a second prepared session must not demote the primary")

	stored, err := repo.ListTaskSessions(ctx, taskID)
	require.NoError(t, err)
	primary := 0
	for _, session := range stored {
		if session.IsPrimary {
			primary++
			require.Equal(t, first.ID, session.ID)
		}
	}
	require.Equal(t, 1, primary)
}

func TestPreparedSessionConcurrentPrimaryElection(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		name := "SQLite"
		if postgres {
			name = "Postgres"
		}
		t.Run(name, func(t *testing.T) {
			repo := newPreparedSessionRepo(t, postgres)
			ctx := context.Background()
			const taskID = "task-prepared-concurrent"
			require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Concurrent election"}))

			const creators = 4
			start := make(chan struct{})
			errs := make([]error, creators)
			sessions := make([]*models.TaskSession, creators)
			var wg sync.WaitGroup
			for i := 0; i < creators; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					session := &models.TaskSession{
						ID: fmt.Sprintf("prepared-concurrent-%d", i), TaskID: taskID, IsPrimary: true,
					}
					sessions[i] = session
					<-start
					errs[i] = repo.CreateTaskSession(ctx, session)
				}(i)
			}
			close(start)
			wg.Wait()
			for i, err := range errs {
				require.NoErrorf(t, err, "creator %d", i)
			}
			stored, err := repo.ListTaskSessions(ctx, taskID)
			require.NoError(t, err)
			primary := 0
			for _, session := range stored {
				if session.IsPrimary {
					primary++
				}
			}
			require.Equal(t, 1, primary)
			committed := 0
			for _, session := range sessions {
				storedSession, err := repo.GetTaskSession(ctx, session.ID)
				require.NoError(t, err)
				if storedSession.IsPrimary {
					committed++
				}
			}
			require.Equal(t, 1, committed)
		})
	}
}

func TestPreparedSessionSeedAtomicity(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		name := "SQLite"
		if postgres {
			name = "Postgres"
		}
		t.Run(name, func(t *testing.T) {
			runPreparedSessionSeedAtomicity(t, newPreparedSessionRepo(t, postgres))
		})
	}
}

func runPreparedSessionSeedAtomicity(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	const taskID = "task-prepared-seed"
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Prepared seed"}))
	preview := models.NewInitialPromptPreview("original text", nil)
	submission, err := models.NewInitialPromptSubmission("original text", false, nil)
	require.NoError(t, err)
	seed := &models.PreparedSessionSeed{
		InitialPromptPreview:    preview,
		InitialPromptSubmission: submission,
	}
	session := &models.TaskSession{
		ID: "prepared-seed-session", TaskID: taskID,
		Metadata: seed.ApplyToMetadata(nil),
	}
	require.NoError(t, repo.CreateTaskSession(ctx, session))

	replacementPreview := models.NewInitialPromptPreview("replacement text", nil)
	replacementSubmission, err := models.NewInitialPromptSubmission("replacement text", false, nil)
	require.NoError(t, err)
	duplicate := &models.TaskSession{
		ID: session.ID, TaskID: taskID,
		Metadata: (&models.PreparedSessionSeed{
			InitialPromptPreview: replacementPreview, InitialPromptSubmission: replacementSubmission,
		}).ApplyToMetadata(nil),
	}
	require.Error(t, repo.CreateTaskSession(ctx, duplicate), "duplicate session insert must fail")

	stored, err := repo.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	loadedPreview, ok := stored.Metadata[models.SessionMetaKeyInitialPromptPreview]
	require.True(t, ok)
	encodedPreview, err := json.Marshal(loadedPreview)
	require.NoError(t, err)
	var decodedPreview models.InitialPromptPreview
	require.NoError(t, json.Unmarshal(encodedPreview, &decodedPreview))
	require.Equal(t, "original text", decodedPreview.Content)
	loaded, found, err := models.LoadInitialPromptSubmission(stored.Metadata)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "original text", loaded.Content)

	sessions, err := repo.ListTaskSessions(ctx, taskID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.Equal(t, session.ID, sessions[0].ID)
}

func TestPreparedSessionCancellationWritesNothing(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForSessionTests(t)
	const taskID = "task-prepared-cancel"
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Prepared cancel"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	err := repo.CreateTaskSession(cancelled, &models.TaskSession{
		ID: "prepared-cancel-session", TaskID: taskID, IsPrimary: true,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled insert error = %v, want context.Canceled", err)
	}
	sessions, err := repo.ListTaskSessions(ctx, taskID)
	if err != nil {
		t.Fatalf("ListTaskSessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("cancelled insert persisted sessions: %+v", sessions)
	}
}
