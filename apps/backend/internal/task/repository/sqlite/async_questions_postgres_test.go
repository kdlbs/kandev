package sqlite

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/clarification/protocol"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestAsyncQuestionsPostgresResolutionRace(t *testing.T) {
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 4)
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	ctx := context.Background()
	question := newAsyncQuestionForTest(t, "q-postgres-race", "session-postgres-race")
	created, isNew := createAsyncQuestionForTest(t, repo, question)
	require.True(t, isNew)

	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		wins  int
		fails []error
	)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			answer := "a"
			if index == 1 {
				answer = "b"
			}
			_, won, resolveErr := repo.ResolveAsyncQuestion(ctx, created.ID, models.AsyncQuestionOutcome{
				Answers: []protocol.Answer{{QuestionID: "q1", SelectedOptions: []string{answer}}},
			}, nil)
			mu.Lock()
			defer mu.Unlock()
			if resolveErr != nil {
				fails = append(fails, resolveErr)
				return
			}
			if won {
				wins++
			}
		}(i)
	}
	wg.Wait()
	require.Empty(t, fails)
	require.Equal(t, 1, wins, "exactly one concurrent resolution must win")

	winner, err := repo.GetAsyncQuestion(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, models.AsyncQuestionStatusAnswered, winner.Status)
	require.Equal(t, int64(2), winner.Version, "a racing loser must not bump the version")
	require.NotNil(t, winner.Outcome)
	require.Len(t, winner.Outcome.Answers, 1)
}

func TestAsyncQuestionsPostgresCreateReplay(t *testing.T) {
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 2)
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	ctx := context.Background()
	question := newAsyncQuestionForTest(t, "q-postgres-replay", "session-postgres-replay")

	created, isNew := createAsyncQuestionForTest(t, repo, question)
	require.True(t, isNew)
	replayed, isNew, err := repo.CreateAsyncQuestion(ctx, newAsyncQuestionForTest(t, "q-postgres-replay", "session-postgres-replay"), nil)
	require.NoError(t, err)
	require.False(t, isNew)
	require.Equal(t, created.ID, replayed.ID)

	conflicting := newAsyncQuestionForTest(t, "q-postgres-replay", "session-postgres-replay")
	conflicting.PayloadDigest = "other"
	_, _, err = repo.CreateAsyncQuestion(ctx, conflicting, nil)
	require.ErrorIs(t, err, ErrAsyncQuestionRetryConflict)

	wait, err := repo.CreateAsyncQuestionWait(ctx, &models.AsyncQuestionWait{
		TaskID: created.TaskID, SessionID: created.SessionID, DependencyIDs: []string{created.ID},
		Status: models.AsyncQuestionWaitRequested,
	})
	require.NoError(t, err)
	withdrawn, won, released, err := repo.WithdrawAsyncQuestion(ctx, created.ID, created.SessionID, "done", nil, nil)
	require.NoError(t, err)
	require.True(t, won)
	require.Equal(t, models.AsyncQuestionStatusWithdrawn, withdrawn.Status)
	require.Len(t, released, 1)
	require.Equal(t, wait.ID, released[0].ID)
	require.False(t, withdrawn.UpdatedAt.IsZero())
}

func TestAsyncQuestionsPostgresConcurrentCreateAndWait(t *testing.T) {
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 4)
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	ctx := context.Background()
	question := newAsyncQuestionForTest(t, "q-postgres-concurrent", "session-postgres-concurrent")
	seedAsyncQuestionBinding(t, repo, question)

	var wg sync.WaitGroup
	ids := make([]string, 2)
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			created, _, createErr := repo.CreateAsyncQuestion(ctx, newAsyncQuestionForTest(t,
				"q-postgres-concurrent", "session-postgres-concurrent"), nil)
			errs[index] = createErr
			if created != nil {
				ids[index] = created.ID
			}
		}(i)
	}
	wg.Wait()
	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	require.Equal(t, ids[0], ids[1], "concurrent exact retries must return one request")

	waitErr := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			_, waitErr[index] = repo.CreateAsyncQuestionWait(ctx, &models.AsyncQuestionWait{
				TaskID: question.TaskID, SessionID: question.SessionID,
				DependencyIDs: []string{question.ID}, Status: models.AsyncQuestionWaitRequested,
			})
		}(i)
	}
	wg.Wait()
	wins := 0
	for _, err := range waitErr {
		if err == nil {
			wins++
		} else {
			require.ErrorIs(t, err, ErrAsyncQuestionWaitAlreadyActive)
		}
	}
	require.Equal(t, 1, wins, "only one concurrent wait may be active per session")
}
