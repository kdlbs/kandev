package sqlite

import (
	"context"
	"encoding/json"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

type executorFailureTestStore interface {
	ObserveExecutorFailure(context.Context, models.ExecutorObservationTarget, *models.ExecutorObservation) (*models.ExecutorFailureEpisode, bool, error)
	GetExecutorFailure(context.Context, string) (*models.ExecutorFailureEpisode, error)
	ListExecutorObservationTargets(context.Context, string, int) ([]models.ExecutorObservationTarget, error)
}

func executorFailureStore(t *testing.T, r *Repository) executorFailureTestStore {
	t.Helper()
	store, ok := any(r).(executorFailureTestStore)
	require.True(t, ok, "repository must persist and fence resource failure observations")
	return store
}

// @covers AC-EXECUTORS-FAILURE-VISIBILITY-001.3, .5
func TestExecutorFailureEpisodeSurvivesReloadAndDeduplicates(t *testing.T) {
	repo := newRepoForEntityTests(t)
	store := executorFailureStore(t, repo)
	seedRecoveryClaimEnvironment(t, repo, "task-failure", "env-failure")
	target := models.ExecutorObservationTarget{TaskID: "task-failure", EnvironmentID: "env-failure", OwnershipGeneration: 1, ResourceKey: "container", Runtime: "docker"}
	_, err := repo.db.Exec(`UPDATE task_environments SET container_id = 'container',status='ready' WHERE id='env-failure'`)
	require.NoError(t, err)
	now := time.Now().UTC()
	obs := &models.ExecutorObservation{Outcome: "terminated", Runtime: "docker", ResourceKey: "container", ObservedAt: now, Reason: "ContainerExited", Message: "token=secret-value", Workspace: "unknown"}
	episode, changed, err := store.ObserveExecutorFailure(t.Context(), target, obs)
	require.NoError(t, err)
	require.True(t, changed)
	require.NotNil(t, episode)
	firstID := episode.ID
	again, changed, err := store.ObserveExecutorFailure(t.Context(), target, obs)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, firstID, again.ID)
	reopened := NewWithInitializedDB(repo.db, repo.ro, nil)
	loaded, err := executorFailureStore(t, reopened).GetExecutorFailure(t.Context(), target.TaskID)
	require.NoError(t, err)
	require.Equal(t, firstID, loaded.ID)
	require.NotContains(t, loaded.Observation.Message, "secret-value")
	obs.Outcome = "healthy"
	obs.ObservedAt = now.Add(time.Minute)
	resolved, changed, err := store.ObserveExecutorFailure(t.Context(), target, obs)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "resolved", resolved.State)
	require.Equal(t, "ContainerExited", resolved.Observation.Reason)
	require.NotNil(t, resolved.ResolvedAt)
	obs.Outcome = "terminated"
	obs.ObservedAt = now.Add(2 * time.Minute)
	later, changed, err := store.ObserveExecutorFailure(t.Context(), target, obs)
	require.NoError(t, err)
	require.True(t, changed)
	require.NotEqual(t, firstID, later.ID)
}

func TestExecutorFailureRejectsStaleOwnership(t *testing.T) {
	repo := newRepoForEntityTests(t)
	store := executorFailureStore(t, repo)
	seedRecoveryClaimEnvironment(t, repo, "task-stale", "env-stale")
	_, err := repo.db.Exec(`UPDATE task_environments SET container_id='current',status='ready' WHERE id='env-stale'`)
	require.NoError(t, err)
	now := time.Now().UTC()
	target := models.ExecutorObservationTarget{TaskID: "task-stale", EnvironmentID: "env-stale", OwnershipGeneration: 1, ResourceKey: "current", Runtime: "docker"}
	obs := &models.ExecutorObservation{Outcome: "terminated", ResourceKey: "current", ObservedAt: now, Runtime: "docker", Workspace: "unknown"}
	first, _, err := store.ObserveExecutorFailure(t.Context(), target, obs)
	require.NoError(t, err)
	stale := obs.Clone()
	stale.Outcome = "healthy"
	stale.ObservedAt = now.Add(-time.Second)
	_, changed, err := store.ObserveExecutorFailure(t.Context(), target, stale)
	require.NoError(t, err)
	require.False(t, changed)
	got, err := store.GetExecutorFailure(t.Context(), target.TaskID)
	require.NoError(t, err)
	require.Equal(t, first.ID, got.ID)
	require.Equal(t, "active", got.State)
	_, err = repo.db.Exec(`UPDATE task_environments SET ownership_generation=2 WHERE id='env-stale'`)
	require.NoError(t, err)
	_, _, err = store.ObserveExecutorFailure(t.Context(), target, obs)
	require.Error(t, err, "old generation must not write against a replacement")
}

func TestExecutorFailureInventoryIncludesZeroSessionEnvironment(t *testing.T) {
	repo := newRepoForEntityTests(t)
	store := executorFailureStore(t, repo)
	seedRecoveryClaimEnvironment(t, repo, "task-idle", "env-idle")
	_, err := repo.db.Exec(`UPDATE task_environments SET container_id='idle-container',executor_type='docker',status='ready' WHERE id='env-idle'`)
	require.NoError(t, err)
	targets, err := store.ListExecutorObservationTargets(t.Context(), "", 1)
	require.NoError(t, err)
	require.Len(t, targets, 1)
	require.Equal(t, "env-idle", targets[0].EnvironmentID)
	require.Empty(t, targets[0].SessionID)
	next, err := store.ListExecutorObservationTargets(t.Context(), targets[0].EnvironmentID, 1)
	require.NoError(t, err)
	require.Empty(t, next)
}

func TestExecutorFailureCapturesExactAffectedTurn(t *testing.T) {
	repo := newRepoForEntityTests(t)
	store := executorFailureStore(t, repo)
	pending, ok := any(repo).(interface {
		ListExecutorFailureAffectedSessions(context.Context, string) ([]models.ExecutorFailureAffectedSession, error)
	})
	require.True(t, ok, "affected turn identities must survive partial settlement")
	seedRecoveryClaimEnvironment(t, repo, "task-active", "env-active")
	env, err := repo.GetTaskEnvironment(t.Context(), "env-active")
	require.NoError(t, err)
	env.ContainerID = "owned"
	env.ExecutorType = "local_docker"
	env.Status = models.TaskEnvironmentStatusReady
	require.NoError(t, repo.UpdateTaskEnvironment(t.Context(), env))
	require.NoError(t, repo.CreateTaskSession(t.Context(), &models.TaskSession{ID: "session-active", TaskID: "task-active", TaskEnvironmentID: "env-active", State: models.TaskSessionStateRunning}))
	require.NoError(t, repo.UpsertExecutorRunning(t.Context(), &models.ExecutorRunning{ID: "running", TaskID: "task-active", SessionID: "session-active", AgentExecutionID: "lost", ContainerID: "owned", Status: "running"}))
	require.NoError(t, repo.CreateTurn(t.Context(), &models.Turn{ID: "turn-active", TaskID: "task-active", TaskSessionID: "session-active", StartedAt: time.Now().UTC()}))
	target := models.ExecutorObservationTarget{TaskID: "task-active", EnvironmentID: "env-active", OwnershipGeneration: 1, ResourceKey: "owned", Runtime: "docker"}
	episode, _, err := store.ObserveExecutorFailure(t.Context(), target, &models.ExecutorObservation{Outcome: "terminated", ResourceKey: "owned", ObservedAt: time.Now().UTC(), Workspace: "unknown"})
	require.NoError(t, err)
	refs, err := pending.ListExecutorFailureAffectedSessions(t.Context(), "task-active")
	require.NoError(t, err)
	require.Len(t, refs, 1)
	require.Equal(t, episode.ID, refs[0].EpisodeID)
	require.Equal(t, "lost", refs[0].Candidate.ExpectedExecutorAgentExecutionID)
	require.Equal(t, "turn-active", refs[0].Candidate.ExpectedTurnID)
	history, historyErr := repo.ListMessages(t.Context(), "session-active")
	require.NoError(t, historyErr)
	require.Len(t, history, 1, "failure history must commit with the episode before settlement can race")
	require.Equal(t, episode.ID, history[0].Metadata["executor_failure_id"])
	require.NoError(t, repo.UpsertExecutorRunning(t.Context(), &models.ExecutorRunning{ID: "running", TaskID: "task-active", SessionID: "session-active", AgentExecutionID: "successor", ContainerID: "owned", Status: "running"}))
	recovered, err := repo.RecoverTaskSessionByCandidate(t.Context(), refs[0].Candidate, time.Time{})
	require.NoError(t, err)
	require.Nil(t, recovered, "settlement cannot overwrite a successor execution")
}

func TestExecutorFailureRepeatedTerminalObservationRejectsOlderRecovery(t *testing.T) {
	repo := newRepoForEntityTests(t)
	store := executorFailureStore(t, repo)
	seedRecoveryClaimEnvironment(t, repo, "task-clock", "env-clock")
	_, err := repo.db.Exec(`UPDATE task_environments SET container_id='owned',status='ready' WHERE id='env-clock'`)
	require.NoError(t, err)
	target := models.ExecutorObservationTarget{TaskID: "task-clock", EnvironmentID: "env-clock", OwnershipGeneration: 1, ResourceKey: "owned", Runtime: "docker"}
	now := time.Now().UTC()
	obs := &models.ExecutorObservation{Outcome: "terminated", ResourceKey: "owned", Runtime: "docker", ObservedAt: now, Reason: "ContainerExited", Workspace: "unknown"}
	_, _, err = store.ObserveExecutorFailure(t.Context(), target, obs)
	require.NoError(t, err)
	obs.ObservedAt = now.Add(2 * time.Minute)
	_, changed, err := store.ObserveExecutorFailure(t.Context(), target, obs)
	require.NoError(t, err)
	require.False(t, changed)
	obs.Outcome = "healthy"
	obs.ObservedAt = now.Add(time.Minute)
	_, changed, err = store.ObserveExecutorFailure(t.Context(), target, obs)
	require.NoError(t, err)
	require.False(t, changed, "a delayed healthy response cannot clear a more recent confirmed loss")
	latest, err := store.GetExecutorFailure(t.Context(), target.TaskID)
	require.NoError(t, err)
	require.Equal(t, "active", latest.State)
}

func TestPostgresExecutorFailureConcurrentAdmission(t *testing.T) {
	first, second, _ := newTaskPostgresRepoPair(t)
	seedRecoveryClaimEnvironment(t, first, "task-pg-failure", "env-pg-failure")
	_, err := first.db.Exec(`UPDATE task_environments SET container_id='owned',status='ready' WHERE id='env-pg-failure'`)
	require.NoError(t, err)
	target := models.ExecutorObservationTarget{TaskID: "task-pg-failure", EnvironmentID: "env-pg-failure", OwnershipGeneration: 1, ResourceKey: "owned", Runtime: "docker"}
	observation := &models.ExecutorObservation{Outcome: "terminated", ResourceKey: "owned", Runtime: "docker", ObservedAt: time.Now().UTC(), Reason: "ContainerExited", Workspace: "unknown"}
	type result struct {
		episode *models.ExecutorFailureEpisode
		err     error
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	for _, repo := range []*Repository{first, second} {
		go func(r *Repository) {
			<-start
			e, _, err := r.ObserveExecutorFailure(t.Context(), target, observation)
			results <- result{e, err}
		}(repo)
	}
	close(start)
	a, b := <-results, <-results
	require.NoError(t, a.err)
	require.NoError(t, b.err)
	require.Equal(t, a.episode.ID, b.episode.ID)
	var count int
	require.NoError(t, first.db.QueryRow(`SELECT count(*) FROM executor_failure_episodes WHERE task_id='task-pg-failure'`).Scan(&count))
	require.Equal(t, 1, count)
}

func TestExecutorFailureUncertaintyPreservesCauseAndResolvedOrdering(t *testing.T) {
	repo := newRepoForEntityTests(t)
	store := executorFailureStore(t, repo)
	seedRecoveryClaimEnvironment(t, repo, "task-order", "env-order")
	_, err := repo.db.Exec(`UPDATE task_environments SET container_id='owned',status='ready' WHERE id='env-order'`)
	require.NoError(t, err)
	target := models.ExecutorObservationTarget{TaskID: "task-order", EnvironmentID: "env-order", OwnershipGeneration: 1, ResourceKey: "owned", Runtime: "docker"}
	now := time.Now().UTC()
	obs := &models.ExecutorObservation{Outcome: "terminated", ResourceKey: "owned", Runtime: "docker", ObservedAt: now, Reason: "ContainerExited", Workspace: "unknown", Containers: []models.ExecutorContainerEvidence{{Name: "agent", Reason: "password=private-value"}}}
	initial, _, err := store.ObserveExecutorFailure(t.Context(), target, obs)
	require.NoError(t, err)
	raw, err := json.Marshal(initial)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "private-value")
	unknown := obs.Clone()
	unknown.Outcome = "unknown"
	unknown.ObservedAt = now.Add(time.Minute)
	uncertain, changed, err := store.ObserveExecutorFailure(t.Context(), target, unknown)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "unknown", uncertain.CurrentOutcome)
	require.Equal(t, "ContainerExited", uncertain.Observation.Reason)
	obs.Outcome = "healthy"
	obs.ObservedAt = now.Add(3 * time.Minute)
	resolved, _, err := store.ObserveExecutorFailure(t.Context(), target, obs)
	require.NoError(t, err)
	require.Equal(t, "resolved", resolved.State)
	obs.Outcome = "terminated"
	obs.ObservedAt = now.Add(2 * time.Minute)
	latest, changed, err := store.ObserveExecutorFailure(t.Context(), target, obs)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, initial.ID, latest.ID)
	require.Equal(t, "resolved", latest.State, "old terminal response cannot reopen a resolved episode")
}

func TestExecutorFailureProviderOutcomeIsDurableAndFenced(t *testing.T) {
	repo := newRepoForEntityTests(t)
	seedRecoveryClaimEnvironment(t, repo, "outcome-task", "outcome-env")
	require.NoError(t, repo.CreateTaskSession(t.Context(), &models.TaskSession{ID: "outcome-session", TaskID: "outcome-task", State: models.TaskSessionStateRunning}))
	require.NoError(t, repo.UpsertExecutorRunning(t.Context(), &models.ExecutorRunning{ID: "outcome-row", SessionID: "outcome-session", TaskID: "outcome-task", AgentExecutionID: "current", ResumeToken: "fresh-token", Status: "ready"}))
	writer, ok := any(repo).(interface {
		RecordProviderRecovery(context.Context, string, string, string, string, string) (*models.Message, error)
	})
	require.True(t, ok, "actual provider recovery must persist independently of session state")
	marker, err := writer.RecordProviderRecovery(t.Context(), "outcome-task", "outcome-session", "current", "fresh-token", "fresh")
	require.NoError(t, err)
	require.NotNil(t, marker)
	duplicate, err := writer.RecordProviderRecovery(t.Context(), "outcome-task", "outcome-session", "current", "fresh-token", "fresh")
	require.NoError(t, err)
	require.Equal(t, marker.ID, duplicate.ID)
	reopened := NewWithInitializedDB(repo.db, repo.ro, nil)
	loaded, err := reopened.GetMessage(t.Context(), marker.ID)
	require.NoError(t, err)
	require.Equal(t, "fresh", loaded.Metadata["provider_conversation"])
	_, err = writer.RecordProviderRecovery(t.Context(), "outcome-task", "outcome-session", "old", "fresh-token", "restored")
	require.Error(t, err, "stale recovery cannot overwrite a successor")
}

func TestExecutorFailureInventoryIncludesLegacySessionOnlyRuntime(t *testing.T) {
	repo := newRepoForEntityTests(t)
	seedRecoveryClaimEnvironment(t, repo, "legacy-task", "legacy-env")
	require.NoError(t, repo.CreateTaskSession(t.Context(), &models.TaskSession{ID: "legacy-session", TaskID: "legacy-task", State: models.TaskSessionStateRunning}))
	require.NoError(t, repo.UpsertExecutorRunning(t.Context(), &models.ExecutorRunning{ID: "legacy-row", SessionID: "legacy-session", TaskID: "legacy-task", AgentExecutionID: "legacy-execution", Runtime: "docker", ContainerID: "legacy-container", Status: "running"}))
	targets, err := repo.ListExecutorObservationTargets(t.Context(), "", 1)
	require.NoError(t, err)
	require.Len(t, targets, 1, "session-only runtimes must be observed after backend restart")
	require.Equal(t, "legacy-session", targets[0].SessionID)
	require.False(t, targets[0].ExpectedExecutorUpdatedAt.IsZero())
}

func TestExecutorFailureRejectsDelayedObservationAcrossResourceOperation(t *testing.T) {
	repo := newRepoForEntityTests(t)
	seedRecoveryClaimEnvironment(t, repo, "operation-task", "operation-env")
	_, err := repo.db.Exec(`UPDATE task_environments SET executor_type='k8s',status='ready' WHERE id='operation-env'`)
	require.NoError(t, err)
	_, err = repo.db.Exec(`INSERT INTO task_environment_kubernetes(environment_id,task_id,ownership_generation,revision,metadata) VALUES('operation-env','operation-task',1,2,'{"kubernetes_pod_uid":"same-pod"}')`)
	require.NoError(t, err)
	target := models.ExecutorObservationTarget{TaskID: "operation-task", EnvironmentID: "operation-env", OwnershipGeneration: 1, Runtime: "k8s", ResourceKey: "same-pod", InventoryRevision: 1}
	_, _, err = repo.ObserveExecutorFailure(t.Context(), target, &models.ExecutorObservation{Outcome: "terminated", ResourceKey: "same-pod", ObservedAt: time.Now().UTC()})
	require.Error(t, err, "a completed operation changes authority even when Pod UID is unchanged")
}

func TestExecutorFailureHealthyReplacementResolvesPriorResource(t *testing.T) {
	repo := newRepoForEntityTests(t)
	seedRecoveryClaimEnvironment(t, repo, "replace-task", "replace-env")
	_, err := repo.db.Exec(`UPDATE task_environments SET container_id='lost',status='ready' WHERE id='replace-env'`)
	require.NoError(t, err)
	target := models.ExecutorObservationTarget{TaskID: "replace-task", EnvironmentID: "replace-env", OwnershipGeneration: 1, Runtime: "docker", ResourceKey: "lost"}
	now := time.Now().UTC()
	prior, _, err := repo.ObserveExecutorFailure(t.Context(), target, &models.ExecutorObservation{Outcome: "terminated", ResourceKey: "lost", ObservedAt: now})
	require.NoError(t, err)
	_, err = repo.db.Exec(`UPDATE task_environments SET container_id='replacement' WHERE id='replace-env'`)
	require.NoError(t, err)
	target.ResourceKey = "replacement"
	_, changed, err := repo.ObserveExecutorFailure(t.Context(), target, &models.ExecutorObservation{Outcome: "healthy", ResourceKey: "replacement", ObservedAt: now.Add(time.Minute)})
	require.NoError(t, err)
	require.True(t, changed)
	resolved, err := repo.GetExecutorFailure(t.Context(), target.TaskID)
	require.NoError(t, err)
	require.Equal(t, prior.ID, resolved.ID)
	require.Equal(t, "resolved", resolved.State, "verified replacement must clear stale failure without erasing the incident")
}

func TestExecutorFailureInventoryIncludesRetainedLocalController(t *testing.T) {
	repo := newRepoForEntityTests(t)
	seedRecoveryClaimEnvironment(t, repo, "local-task", "local-env")
	_, err := repo.db.Exec(`UPDATE task_environments SET executor_type='worktree',status='ready' WHERE id='local-env'`)
	require.NoError(t, err)
	require.NoError(t, repo.CreateTaskSession(t.Context(), &models.TaskSession{ID: "local-session", TaskID: "local-task", TaskEnvironmentID: "local-env", State: models.TaskSessionStateWaitingForInput}))
	require.NoError(t, repo.UpsertExecutorRunning(t.Context(), &models.ExecutorRunning{ID: "local-row", TaskID: "local-task", SessionID: "local-session", AgentExecutionID: "local-exec", Runtime: "standalone", LocalPID: 42, Status: "ready"}))
	targets, err := repo.ListExecutorObservationTargets(t.Context(), "", 100)
	require.NoError(t, err)
	require.Len(t, targets, 1)
	require.Equal(t, "local-pid:42", targets[0].ResourceKey)
	require.Equal(t, 42, targets[0].LocalPID)
	require.Equal(t, "local-session", targets[0].AuthoritySessionID)
}

func TestExecutorFailureHealthyReplacementResolvesAllEarlierSharedResources(t *testing.T) {
	repo := newRepoForEntityTests(t)
	seedRecoveryClaimEnvironment(t, repo, "many-task", "many-env")
	target := models.ExecutorObservationTarget{TaskID: "many-task", EnvironmentID: "many-env", OwnershipGeneration: 1, Runtime: "docker"}
	now := time.Now().UTC()
	for index, resource := range []string{"lost-first", "lost-second", "healthy"} {
		_, err := repo.db.Exec(`UPDATE task_environments SET container_id=?,status='ready' WHERE id='many-env'`, resource)
		require.NoError(t, err)
		target.ResourceKey = resource
		outcome := "terminated"
		if index == 2 {
			outcome = "healthy"
		}
		_, _, err = repo.ObserveExecutorFailure(t.Context(), target, &models.ExecutorObservation{Outcome: outcome, ResourceKey: resource, ObservedAt: now.Add(time.Duration(index) * time.Minute)})
		require.NoError(t, err)
	}
	visible, err := repo.GetExecutorFailure(t.Context(), "many-task")
	require.NoError(t, err)
	require.Equal(t, "resolved", visible.State, "successful current recovery must not leave an older active episode visible")
	var active int
	require.NoError(t, repo.db.QueryRow(`SELECT count(*) FROM executor_failure_episodes WHERE task_id='many-task' AND state='active'`).Scan(&active))
	require.Zero(t, active)
}

func TestExecutorFailureInventoryCursorAdvancesPastUnknownResource(t *testing.T) {
	repo := newRepoForEntityTests(t)
	seedRecoveryClaimEnvironment(t, repo, "page-task", "a-unknown")
	_, err := repo.db.Exec(`UPDATE task_environments SET executor_type='k8s',status='ready' WHERE id='a-unknown'`)
	require.NoError(t, err)
	_, err = repo.db.Exec(`INSERT INTO task_environment_kubernetes(environment_id,task_id,ownership_generation,revision,metadata) VALUES('a-unknown','page-task',1,1,'{"kubernetes_pod_name":"unverified"}')`)
	require.NoError(t, err)
	seedRecoveryClaimEnvironment(t, repo, "page-known-task", "b-known")
	_, err = repo.db.Exec(`UPDATE task_environments SET executor_type='local_docker',container_id='owned',status='ready' WHERE id='b-known'`)
	require.NoError(t, err)
	first, err := repo.ListExecutorObservationTargets(t.Context(), "", 1)
	require.NoError(t, err)
	require.Len(t, first, 1, "an unverifiable row must not truncate the inventory page and starve later resources")
	require.Equal(t, "a-unknown", first[0].Cursor)
	require.Empty(t, first[0].ResourceKey)
	next, err := repo.ListExecutorObservationTargets(t.Context(), first[0].Cursor, 1)
	require.NoError(t, err)
	require.Len(t, next, 1)
	require.Equal(t, "owned", next[0].ResourceKey)
}

func TestExecutorFailureProviderRecoveryKeepsWorkspaceEvidenceSeparate(t *testing.T) {
	repo := newRepoForEntityTests(t)
	seedRecoveryClaimEnvironment(t, repo, "facts-task", "facts-env")
	_, err := repo.db.Exec(`UPDATE task_environments SET container_id='owned',status='ready' WHERE id='facts-env'`)
	require.NoError(t, err)
	require.NoError(t, repo.CreateTaskSession(t.Context(), &models.TaskSession{ID: "facts-session", TaskID: "facts-task", TaskEnvironmentID: "facts-env", State: models.TaskSessionStateRunning}))
	require.NoError(t, repo.UpsertExecutorRunning(t.Context(), &models.ExecutorRunning{ID: "facts-row", TaskID: "facts-task", SessionID: "facts-session", AgentExecutionID: "current", ContainerID: "owned", ResumeToken: "fresh-token", Status: "ready"}))
	observed := time.Now().UTC()
	_, _, err = repo.ObserveExecutorFailure(t.Context(), models.ExecutorObservationTarget{TaskID: "facts-task", EnvironmentID: "facts-env", OwnershipGeneration: 1, Runtime: "docker", ResourceKey: "owned"}, &models.ExecutorObservation{Outcome: "terminated", ResourceKey: "owned", ObservedAt: observed, Workspace: "retained"})
	require.NoError(t, err)
	marker, err := repo.RecordProviderRecovery(t.Context(), "facts-task", "facts-session", "current", "fresh-token", "fresh")
	require.NoError(t, err)
	require.Equal(t, "fresh", marker.Metadata["provider_conversation"])
	require.Equal(t, "retained", marker.Metadata["workspace"], "workspace evidence does not imply provider continuity")
	require.Equal(t, observed.Format(time.RFC3339Nano), marker.Metadata["workspace_observed_at"], "retention is explicitly the recorded check, not a claim that recovery checked the files again")
}

func TestExecutorFailureSecondaryBlockerSurvivesLaterStatusChecks(t *testing.T) {
	repo := newRepoForEntityTests(t)
	seedRecoveryClaimEnvironment(t, repo, "secondary-task", "secondary-env")
	_, err := repo.db.Exec(`UPDATE task_environments SET container_id='owned',status='ready' WHERE id='secondary-env'`)
	require.NoError(t, err)
	now := time.Now().UTC()
	target := models.ExecutorObservationTarget{TaskID: "secondary-task", EnvironmentID: "secondary-env", OwnershipGeneration: 1, ResourceKey: "owned", Runtime: "docker"}
	obs := &models.ExecutorObservation{Outcome: "terminated", ResourceKey: "owned", ObservedAt: now, Reason: "Evicted", Secondary: []models.ExecutorOperationEvidence{{Operation: "cleanup", Reason: "cleanup_timeout", OccurredAt: now}}}
	_, _, err = repo.ObserveExecutorFailure(t.Context(), target, obs)
	require.NoError(t, err)
	obs.ObservedAt = now.Add(time.Minute)
	obs.Secondary = nil
	_, changed, err := repo.ObserveExecutorFailure(t.Context(), target, obs)
	require.NoError(t, err)
	require.False(t, changed, "polling the same physical cause must not erase a recorded operation blocker")
	retained, err := repo.GetExecutorFailure(t.Context(), target.TaskID)
	require.NoError(t, err)
	require.Len(t, retained.Observation.Secondary, 1)
	require.Equal(t, "cleanup_timeout", retained.Observation.Secondary[0].Reason)
}

func TestExecutorFailureSafeEvidenceBoundsSecondaryAndContainerBudget(t *testing.T) {
	obs := &models.ExecutorObservation{Outcome: "terminated", Runtime: strings.Repeat("x", 100), PodPhase: strings.Repeat("x", 100), Workspace: "token=private", Message: strings.Repeat("x", 768), Secondary: []models.ExecutorOperationEvidence{{Operation: "cleanup", Reason: "token=private"}, {Operation: "cleanup", Reason: "cleanup_timeout", OccurredAt: time.Now().UTC()}}}
	for range 8 {
		obs.Containers = append(obs.Containers, models.ExecutorContainerEvidence{Name: strings.Repeat("x", 64), Reason: strings.Repeat("x", 64), State: strings.Repeat("x", 32), StartedAt: &obs.ObservedAt, FinishedAt: &obs.ObservedAt, LastFinishedAt: &obs.ObservedAt})
	}
	safe := safeExecutorObservation(obs)
	require.Len(t, safe.Secondary, 1)
	require.Equal(t, "cleanup_timeout", safe.Secondary[0].Reason)
	require.Equal(t, "unknown", safe.Workspace)
	raw, err := json.Marshal(safe)
	require.NoError(t, err)
	require.LessOrEqual(t, len(raw), 4096, "bounded diagnostics cannot prevent persistence of the primary failure")
	require.NotContains(t, string(raw), "private")
	require.Len(t, obs.Containers, 8, "sanitizing cannot mutate provider evidence")
}

func TestExecutorFailureResourceDisappearancePreservesKnownPhysicalCause(t *testing.T) {
	repo := newRepoForEntityTests(t)
	seedRecoveryClaimEnvironment(t, repo, "missing-cause-task", "missing-cause-env")
	_, err := repo.db.Exec(`UPDATE task_environments SET container_id='owned',status='ready' WHERE id='missing-cause-env'`)
	require.NoError(t, err)
	now := time.Now().UTC()
	target := models.ExecutorObservationTarget{TaskID: "missing-cause-task", EnvironmentID: "missing-cause-env", OwnershipGeneration: 1, ResourceKey: "owned", Runtime: "docker"}
	primary := &models.ExecutorObservation{Outcome: "terminated", ResourceKey: "owned", ObservedAt: now, Reason: "Evicted", Message: "EmptyDir exceeds 12Gi", Workspace: "retained"}
	original, _, err := repo.ObserveExecutorFailure(t.Context(), target, primary)
	require.NoError(t, err)
	missing := &models.ExecutorObservation{Outcome: "missing", ResourceKey: "owned", ObservedAt: now.Add(time.Minute), Reason: "PodMissing", Workspace: "unknown"}
	updated, _, err := repo.ObserveExecutorFailure(t.Context(), target, missing)
	require.NoError(t, err)
	require.Equal(t, original.ID, updated.ID)
	require.Equal(t, "missing", updated.CurrentOutcome)
	reloaded, err := repo.GetExecutorFailure(t.Context(), target.TaskID)
	require.NoError(t, err)
	require.Equal(t, "Evicted", reloaded.Observation.Reason, "resource disappearance cannot erase the actionable physical cause")
	require.Equal(t, primary.Message, reloaded.Observation.Message)
	require.True(t, now.Equal(reloaded.Observation.ObservedAt), "retention evidence remains explicitly as of its original check")
	missing.ObservedAt = now.Add(2 * time.Minute)
	_, changed, err := repo.ObserveExecutorFailure(t.Context(), target, missing)
	require.NoError(t, err)
	require.False(t, changed)
}

func TestExecutorFailureUnavailableWorkerWarningSurvivesReload(t *testing.T) {
	repo := newRepoForEntityTests(t)
	seedRecoveryClaimEnvironment(t, repo, "task-outage", "env-outage")
	_, err := repo.db.Exec(`UPDATE task_environments SET container_id='owned',status='ready' WHERE id='env-outage'`)
	require.NoError(t, err)
	_, err = repo.db.Exec(`INSERT INTO task_environment_kubernetes(environment_id,task_id,ownership_generation,revision,metadata) VALUES('env-outage','task-outage',1,1,'{"kubernetes_pod_uid":"owned"}')`)
	require.NoError(t, err)
	target := models.ExecutorObservationTarget{TaskID: "task-outage", EnvironmentID: "env-outage", OwnershipGeneration: 1, ResourceKey: "owned", Runtime: "k8s", InventoryRevision: 1}
	now := time.Now().UTC()
	obs := &models.ExecutorObservation{Outcome: "unknown", Runtime: "k8s", ResourceKey: "owned", ObservedAt: now, Reason: "WorkerUnavailable", Workspace: "retained"}
	probe := obs.Clone()
	probe.Reason = ""
	absent, changed, err := repo.ObserveExecutorFailure(t.Context(), target, probe)
	require.NoError(t, err)
	require.False(t, changed)
	require.Nil(t, absent, "generic uncertainty cannot create a worker incident")
	episode, changed, err := repo.ObserveExecutorFailure(t.Context(), target, obs)
	require.NoError(t, err)
	require.True(t, changed)
	require.NotNil(t, episode)
	obs.ObservedAt = now.Add(time.Second)
	again, changed, err := repo.ObserveExecutorFailure(t.Context(), target, obs)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, episode.ID, again.ID)
	obs.ObservedAt = now.Add(2 * time.Second)
	obs.Secondary = []models.ExecutorOperationEvidence{{Operation: "cleanup", Reason: "cleanup_failed", OccurredAt: obs.ObservedAt}}
	_, changed, err = repo.ObserveExecutorFailure(t.Context(), target, obs)
	require.NoError(t, err)
	require.True(t, changed)
	reopened := NewWithInitializedDB(repo.db, repo.ro, nil)
	loaded, err := reopened.GetExecutorFailure(t.Context(), target.TaskID)
	require.NoError(t, err)
	require.Equal(t, "active", loaded.State)
	require.Equal(t, "unknown", loaded.CurrentOutcome)
	require.Equal(t, "WorkerUnavailable", loaded.Observation.Reason)
	require.Len(t, loaded.Observation.Secondary, 1)
	probe = obs.Clone()
	probe.Reason = "StatusUnverified"
	probe.ObservedAt = now.Add(3 * time.Second)
	preserved, changed, err := repo.ObserveExecutorFailure(t.Context(), target, probe)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, "WorkerUnavailable", preserved.Observation.Reason)
	require.Len(t, preserved.Observation.Secondary, 1)
	obs.ObservedAt = now.Add(4 * time.Second)
	obs.Outcome = "healthy"
	resolved, changed, err := repo.ObserveExecutorFailure(t.Context(), target, obs)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "resolved", resolved.State)
}
