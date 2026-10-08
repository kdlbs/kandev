package sqlite

import (
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestPostgresExecutorFailureConcurrentSettlement(t *testing.T) {
	first, second, _ := newTaskPostgresRepoPair(t)
	seedRecoveryClaimEnvironment(t, first, "settle-task", "settle-env")
	_, err := first.db.Exec(`UPDATE task_environments SET container_id='owned',status='ready' WHERE id='settle-env'`)
	require.NoError(t, err)
	require.NoError(t, first.CreateTaskSession(t.Context(), &models.TaskSession{ID: "settle-session", TaskID: "settle-task", TaskEnvironmentID: "settle-env", State: models.TaskSessionStateRunning}))
	require.NoError(t, first.UpsertExecutorRunning(t.Context(), &models.ExecutorRunning{ID: "settle-row", TaskID: "settle-task", SessionID: "settle-session", AgentExecutionID: "lost", ContainerID: "owned", Status: "running"}))
	require.NoError(t, first.CreateTurn(t.Context(), &models.Turn{ID: "settle-turn", TaskID: "settle-task", TaskSessionID: "settle-session", StartedAt: time.Now().UTC()}))
	_, _, err = first.ObserveExecutorFailure(t.Context(), models.ExecutorObservationTarget{TaskID: "settle-task", EnvironmentID: "settle-env", OwnershipGeneration: 1, Runtime: "docker", ResourceKey: "owned"}, &models.ExecutorObservation{Outcome: "terminated", ResourceKey: "owned", ObservedAt: time.Now().UTC()})
	require.NoError(t, err)
	refs, err := first.ListExecutorFailureAffectedSessions(t.Context(), "settle-task")
	require.NoError(t, err)
	require.Len(t, refs, 1)
	type result struct {
		session *models.TaskSession
		err     error
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	for _, repo := range []*Repository{first, second} {
		go func(r *Repository) {
			<-start
			session, err := r.RecoverTaskSessionByCandidate(t.Context(), refs[0].Candidate, time.Time{})
			results <- result{session, err}
		}(repo)
	}
	close(start)
	a, b := <-results, <-results
	require.NoError(t, a.err)
	require.NoError(t, b.err)
	recovered := 0
	for _, response := range []result{a, b} {
		if response.session != nil {
			recovered++
		}
	}
	require.Equal(t, 1, recovered, "only one observer may settle the exact interrupted session")
	history, err := first.ListMessages(t.Context(), "settle-session")
	require.NoError(t, err)
	require.Len(t, history, 1, "concurrent recovery keeps one incident history marker")
}
