package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
)

func (r *Repository) initExecutorFailureSchema() error {
	_, err := r.db.ExecContext(r.migrationContext(), `CREATE TABLE IF NOT EXISTS executor_failure_episodes (
 id TEXT PRIMARY KEY, task_id TEXT NOT NULL, environment_id TEXT NOT NULL DEFAULT '', session_id TEXT NOT NULL DEFAULT '',
 ownership_generation BIGINT NOT NULL, resource_key TEXT NOT NULL, revision BIGINT NOT NULL,
 state TEXT NOT NULL, current_outcome TEXT NOT NULL, first_observed_at TIMESTAMP NOT NULL, last_observed_at TIMESTAMP NOT NULL,
 evidence TEXT NOT NULL, resolved_at TIMESTAMP,
 FOREIGN KEY(task_id) REFERENCES tasks(id) ON DELETE CASCADE
 );
 CREATE UNIQUE INDEX IF NOT EXISTS idx_executor_failure_active ON executor_failure_episodes(task_id,environment_id,session_id,ownership_generation,resource_key) WHERE state='active';
 CREATE INDEX IF NOT EXISTS idx_executor_failure_task ON executor_failure_episodes(task_id,last_observed_at);
 CREATE TABLE IF NOT EXISTS executor_failure_sessions (
 episode_id TEXT NOT NULL, session_id TEXT NOT NULL, execution_id TEXT NOT NULL, candidate TEXT NOT NULL, settled INTEGER NOT NULL DEFAULT 0,
 PRIMARY KEY(episode_id,session_id,execution_id),
 FOREIGN KEY(episode_id) REFERENCES executor_failure_episodes(id) ON DELETE CASCADE
 );`)
	return err
}

const executorFailureSelect = `SELECT id,task_id,environment_id,session_id,ownership_generation,resource_key,revision,state,current_outcome,first_observed_at,last_observed_at,evidence,resolved_at FROM executor_failure_episodes`

func scanExecutorFailure(row rowScanner) (*models.ExecutorFailureEpisode, error) {
	var e models.ExecutorFailureEpisode
	var raw string
	err := row.Scan(&e.ID, &e.TaskID, &e.EnvironmentID, &e.SessionID, &e.OwnershipGeneration, &e.ResourceKey, &e.Revision, &e.State, &e.CurrentOutcome, &e.FirstObservedAt, &e.LastObservedAt, &raw, &e.ResolvedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal([]byte(raw), &e.Observation); err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *Repository) GetExecutorFailure(ctx context.Context, taskID string) (*models.ExecutorFailureEpisode, error) {
	return scanExecutorFailure(r.ro.QueryRowContext(ctx, r.ro.Rebind(executorFailureSelect+` WHERE task_id=? ORDER BY CASE WHEN state='active' THEN 0 ELSE 1 END,last_observed_at DESC,id DESC LIMIT 1`), taskID))
}

func (r *Repository) GetActiveExecutorFailure(ctx context.Context, target models.ExecutorObservationTarget) (*models.ExecutorFailureEpisode, error) {
	return scanExecutorFailure(r.ro.QueryRowContext(ctx, r.ro.Rebind(executorFailureSelect+` WHERE task_id=? AND environment_id=? AND ownership_generation=? AND state='active' AND (environment_id!='' OR session_id=?) ORDER BY last_observed_at DESC,id DESC LIMIT 1`), target.TaskID, target.EnvironmentID, target.OwnershipGeneration, target.SessionID))
}

func (r *Repository) ObserveExecutorFailure(ctx context.Context, target models.ExecutorObservationTarget, observation *models.ExecutorObservation) (*models.ExecutorFailureEpisode, bool, error) {
	if observation == nil || observation.ObservedAt.IsZero() || target.TaskID == "" || target.ResourceKey == "" || observation.ResourceKey != target.ResourceKey {
		return nil, false, fmt.Errorf("executor observation identity unavailable")
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err = r.validateExecutorObservationTarget(ctx, tx, target); err != nil {
		return nil, false, err
	}
	prior, err := scanExecutorFailure(tx.QueryRowContext(ctx, r.db.Rebind(executorFailureSelect+` WHERE task_id=? AND environment_id=? AND session_id=? AND ownership_generation=? AND resource_key=? ORDER BY first_observed_at DESC,id DESC LIMIT 1`), target.TaskID, target.EnvironmentID, target.SessionID, target.OwnershipGeneration, target.ResourceKey))
	if err != nil {
		return nil, false, err
	}
	episode, changed, err := r.applyExecutorFailureObservation(ctx, tx, target, observation, prior)
	if err != nil {
		return nil, false, err
	}
	if err = tx.Commit(); err != nil {
		return nil, false, err
	}
	return episode, changed, nil
}

func (r *Repository) applyExecutorFailureObservation(ctx context.Context, tx *sqlx.Tx, target models.ExecutorObservationTarget, observation *models.ExecutorObservation, prior *models.ExecutorFailureEpisode) (*models.ExecutorFailureEpisode, bool, error) {
	var err error
	if observation.Outcome == models.ExecutorOutcomeHealthy && (prior == nil || prior.State == executorFailureResolved) {
		prior, err = r.findSupersededExecutorFailure(ctx, tx, target, observation.ObservedAt)
		if err != nil {
			return nil, false, err
		}
	}
	if prior != nil && !observation.ObservedAt.After(prior.LastObservedAt) {
		return prior, false, nil
	}
	switch observation.Outcome {
	case models.ExecutorOutcomeUnknown:
		if observation.ReportedUnavailable() && (prior == nil || prior.State == executorFailureResolved || prior.Observation.Outcome == models.ExecutorOutcomeUnknown) {
			return r.observeExecutorFailureEpisode(ctx, tx, target, observation, prior)
		}
		if prior == nil {
			return nil, false, nil
		}
		changed := prior.CurrentOutcome != observation.Outcome
		err = r.advanceExecutorFailureCheck(ctx, tx, prior, observation, changed)
		return prior, changed, err
	case models.ExecutorOutcomeHealthy:
		return r.observeHealthyExecutorFailure(ctx, tx, target, observation, prior)
	default:
		return r.observeExecutorFailureEpisode(ctx, tx, target, observation, prior)
	}
}

func (r *Repository) observeHealthyExecutorFailure(ctx context.Context, tx *sqlx.Tx, target models.ExecutorObservationTarget, observation *models.ExecutorObservation, prior *models.ExecutorFailureEpisode) (*models.ExecutorFailureEpisode, bool, error) {
	if prior == nil {
		return nil, false, nil
	}
	if prior.State == executorFailureResolved {
		changed := prior.CurrentOutcome != observation.Outcome
		err := r.advanceExecutorFailureCheck(ctx, tx, prior, observation, changed)
		return prior, changed, err
	}
	if err := r.resolveOwnedExecutorFailures(ctx, tx, target, observation.ObservedAt); err != nil {
		return nil, false, err
	}
	prior.State = executorFailureResolved
	prior.CurrentOutcome = models.ExecutorOutcomeHealthy
	prior.Revision++
	prior.LastObservedAt = observation.ObservedAt
	prior.ResolvedAt = &observation.ObservedAt
	return prior, true, nil
}

func withoutObservationTime(o *models.ExecutorObservation) *models.ExecutorObservation {
	c := o.Clone()
	if c != nil {
		c.ObservedAt = time.Time{}
	}
	return c
}

func (r *Repository) validateExecutorObservationTarget(ctx context.Context, tx *sqlx.Tx, t models.ExecutorObservationTarget) error {
	if err := r.lockTaskRowInTx(ctx, tx, t.TaskID); err != nil {
		return err
	}
	if err := r.taskCleanupBarrierLocked(ctx, tx, t.TaskID); err != nil {
		return err
	}
	var archived sql.NullTime
	if err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT archived_at FROM tasks WHERE id=?`), t.TaskID).Scan(&archived); err != nil {
		return err
	}
	if archived.Valid {
		return fmt.Errorf("executor owner is archived")
	}
	if t.EnvironmentID != "" {
		return r.validateExecutorEnvironmentTarget(ctx, tx, t)
	}

	return r.validateExecutorSessionTarget(ctx, tx, t)
}

func (r *Repository) validateExecutorSessionTarget(ctx context.Context, tx *sqlx.Tx, t models.ExecutorObservationTarget) error {
	var taskID, executionID, containerID, status, raw string
	var updatedAt time.Time
	var localPID int
	if err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT task_id,agent_execution_id,container_id,status,metadata,updated_at,local_pid FROM executors_running WHERE session_id=?`), t.SessionID).Scan(&taskID, &executionID, &containerID, &status, &raw, &updatedAt, &localPID); err != nil {
		return err
	}
	if t.Runtime == string(models.ExecutorTypeKubernetes.Runtime()) {
		var metadata map[string]any
		if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
			return err
		}
		containerID, _ = metadata["kubernetes_pod_uid"].(string)
	}
	if t.Runtime == string(models.ExecutorTypeWorktree.Runtime()) {
		containerID = fmt.Sprintf("local-pid:%d", localPID)
	}
	if taskID != t.TaskID || executionID != t.ExecutionID || containerID != t.ResourceKey || status == taskEnvironmentStatusStopped || (!t.ExpectedExecutorUpdatedAt.IsZero() && !updatedAt.Equal(t.ExpectedExecutorUpdatedAt)) {
		return fmt.Errorf("executor session ownership changed")
	}
	return nil
}

func (r *Repository) captureExecutorFailureSessions(ctx context.Context, tx *sqlx.Tx, e *models.ExecutorFailureEpisode, t models.ExecutorObservationTarget) error {
	rows, err := tx.QueryContext(ctx, r.db.Rebind(`SELECT r.id,r.session_id,r.agent_execution_id,r.updated_at,s.state,s.updated_at,COALESCE(active.id,'') FROM executors_running r JOIN task_sessions s ON s.id=r.session_id LEFT JOIN task_session_turns active ON active.task_session_id=s.id AND active.completed_at IS NULL WHERE r.task_id=? AND r.status!='stopped' AND ((?!='' AND s.task_environment_id=?) OR (?='' AND r.session_id=? AND r.agent_execution_id=?)) AND (?=0 OR r.local_pid=?)`), t.TaskID, t.EnvironmentID, t.EnvironmentID, t.EnvironmentID, t.SessionID, t.ExecutionID, t.LocalPID, t.LocalPID)
	if err != nil {
		return err
	}
	var candidates []models.ActiveSessionRecoveryCandidate
	for rows.Next() {
		c := models.ActiveSessionRecoveryCandidate{TaskID: t.TaskID}
		if err = rows.Scan(&c.ExpectedExecutorID, &c.SessionID, &c.ExpectedExecutorAgentExecutionID, &c.ExpectedExecutorUpdatedAt, &c.ExpectedState, &c.ExpectedUpdatedAt, &c.ExpectedTurnID); err != nil {
			_ = rows.Close()
			return err
		}
		candidates = append(candidates, c)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	for _, c := range candidates {
		raw, err := json.Marshal(c)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO executor_failure_sessions(episode_id,session_id,execution_id,candidate) VALUES(?,?,?,?) ON CONFLICT DO NOTHING`), e.ID, c.SessionID, c.ExpectedExecutorAgentExecutionID, string(raw))
		if err != nil {
			return err
		}
		metadata := map[string]interface{}{"executor_failure_id": e.ID, "executor_failure": e.Observation}
		if err = r.insertExecutorStatusHistoryTx(ctx, tx, models.ExecutorFailureMessageID(e.ID, c.SessionID, c.ExpectedExecutorAgentExecutionID), t.TaskID, c.SessionID, c.ExpectedTurnID, metadata, e.FirstObservedAt); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) ListExecutorFailureAffectedSessions(ctx context.Context, taskID string) ([]models.ExecutorFailureAffectedSession, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`SELECT s.episode_id,s.candidate,e.environment_id,e.resource_key,e.ownership_generation FROM executor_failure_sessions s JOIN executor_failure_episodes e ON e.id=s.episode_id WHERE e.task_id=? AND s.settled=0 ORDER BY e.first_observed_at,s.session_id LIMIT 100`), taskID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var refs []models.ExecutorFailureAffectedSession
	for rows.Next() {
		var ref models.ExecutorFailureAffectedSession
		var raw string
		if err = rows.Scan(&ref.EpisodeID, &raw, &ref.EnvironmentID, &ref.ResourceKey, &ref.OwnershipGeneration); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &ref.Candidate); err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, rows.Err()
}

func (r *Repository) CompleteExecutorFailureSettlement(ctx context.Context, ref models.ExecutorFailureAffectedSession) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE executor_failure_sessions SET settled=1 WHERE episode_id=? AND session_id=? AND execution_id=?`), ref.EpisodeID, ref.Candidate.SessionID, ref.Candidate.ExpectedExecutorAgentExecutionID)
	return err
}

func (r *Repository) ListExecutorObservationTargets(ctx context.Context, after string, limit int) ([]models.ExecutorObservationTarget, error) {
	if limit < 1 || limit > 100 {
		return nil, fmt.Errorf("executor inventory page must contain 1 to 100 records")
	}
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`WITH inventory AS (
 SELECT e.id AS cursor,e.id AS environment_id,e.task_id,e.ownership_generation,e.executor_type AS runtime,e.container_id,COALESCE(k.metadata,'{}') AS metadata,COALESCE(k.revision,0) AS revision,'' AS session_id,'' AS execution_id,local.updated_at AS updated_at,COALESCE(local.local_pid,0) AS local_pid,COALESCE(local.session_id,'') AS authority_session_id
 FROM task_environments e JOIN tasks t ON t.id=e.task_id LEFT JOIN task_environment_kubernetes k ON k.environment_id=e.id
 LEFT JOIN executors_running local ON local.session_id=(SELECT rr.session_id FROM executors_running rr JOIN task_sessions ss ON ss.id=rr.session_id WHERE ss.task_environment_id=e.id AND rr.runtime='standalone' AND rr.local_pid>0 AND rr.status!='stopped' ORDER BY rr.updated_at DESC,rr.session_id LIMIT 1)
 WHERE t.archived_at IS NULL AND e.status IN ('ready','failed','creating') AND (e.container_id!='' OR k.metadata!='{}' OR local.local_pid>0) AND COALESCE(k.operation_id,'')=''
 AND NOT EXISTS(SELECT 1 FROM task_environment_recovery_claims c WHERE c.task_environment_id=e.id)
 UNION ALL
 SELECT '~session:'||r.session_id,'',r.task_id,0,r.runtime,r.container_id,r.metadata,0,r.session_id,r.agent_execution_id,r.updated_at,r.local_pid,r.session_id
 FROM executors_running r JOIN task_sessions s ON s.id=r.session_id JOIN tasks t ON t.id=r.task_id
 WHERE t.archived_at IS NULL AND COALESCE(s.task_environment_id,'')='' AND r.status!='stopped' AND r.runtime!='' AND (r.container_id!='' OR (r.runtime='standalone' AND r.local_pid>0) OR (r.runtime='k8s' AND r.metadata!='{}'))
 ) SELECT cursor,environment_id,task_id,ownership_generation,runtime,container_id,metadata,revision,session_id,execution_id,updated_at,local_pid,authority_session_id FROM inventory WHERE cursor>? ORDER BY cursor LIMIT ?`), after, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var result []models.ExecutorObservationTarget
	for rows.Next() {
		var target models.ExecutorObservationTarget
		var typ, raw string
		var updatedAt sql.NullTime
		if err = rows.Scan(&target.Cursor, &target.EnvironmentID, &target.TaskID, &target.OwnershipGeneration, &typ, &target.ContainerID, &raw, &target.InventoryRevision, &target.SessionID, &target.ExecutionID, &updatedAt, &target.LocalPID, &target.AuthoritySessionID); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &target.Metadata); err != nil {
			return nil, err
		}
		target.Runtime = string(models.ExecutorType(typ).Runtime())
		if target.SessionID != "" {
			target.Runtime = typ
		}
		target.ExpectedExecutorUpdatedAt = updatedAt.Time
		target.ResourceKey = target.ContainerID
		if target.Runtime == string(models.ExecutorTypeWorktree.Runtime()) && target.LocalPID > 0 {
			target.ResourceKey = fmt.Sprintf("local-pid:%d", target.LocalPID)
		}
		if target.Runtime == string(models.ExecutorTypeKubernetes.Runtime()) {
			target.ResourceKey, _ = target.Metadata["kubernetes_pod_uid"].(string)
		}
		result = append(result, target)
	}
	return result, rows.Err()
}

func (r *Repository) advanceExecutorFailureCheck(ctx context.Context, tx *sqlx.Tx, prior *models.ExecutorFailureEpisode, observation *models.ExecutorObservation, changed bool) error {
	delta := 0
	if changed {
		delta = 1
	}
	_, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE executor_failure_episodes SET last_observed_at=?,current_outcome=?,revision=revision+? WHERE id=?`), observation.ObservedAt, observation.Outcome, delta, prior.ID)
	if err != nil {
		return err
	}
	prior.LastObservedAt = observation.ObservedAt
	prior.CurrentOutcome = observation.Outcome
	prior.Revision += int64(delta)
	return nil
}

func boundedExecutorDiagnostic(value string, limit int) string {
	value = routingerr.Sanitize(value)
	if len(value) <= limit {
		return value
	}
	value = value[:limit]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}

func (r *Repository) observeExecutorFailureEpisode(ctx context.Context, tx *sqlx.Tx, target models.ExecutorObservationTarget, observation *models.ExecutorObservation, prior *models.ExecutorFailureEpisode) (*models.ExecutorFailureEpisode, bool, error) {
	var err error
	if !observation.ConfirmedLoss() && !observation.ReportedUnavailable() {
		return nil, false, fmt.Errorf("unknown executor outcome")
	}
	if prior != nil && prior.State == executorFailureResolved {
		prior = nil
	}
	safe := safeExecutorFailureObservation(observation, prior)
	if prior != nil && prior.CurrentOutcome == observation.Outcome && reflect.DeepEqual(withoutObservationTime(prior.Observation), withoutObservationTime(safe)) {
		_, err = tx.ExecContext(ctx, r.db.Rebind(`UPDATE executor_failure_episodes SET last_observed_at=? WHERE id=?`), observation.ObservedAt, prior.ID)
		if err != nil {
			return nil, false, err
		}
		prior.LastObservedAt = observation.ObservedAt
		return prior, false, nil
	}
	raw, encodeErr := json.Marshal(safe)
	if encodeErr != nil {
		return nil, false, encodeErr
	}
	if len(raw) > 4096 {
		return nil, false, fmt.Errorf("executor evidence exceeds diagnostic budget")
	}
	if prior == nil {
		prior = &models.ExecutorFailureEpisode{ID: uuid.NewString(), TaskID: target.TaskID, EnvironmentID: target.EnvironmentID, SessionID: target.SessionID, OwnershipGeneration: target.OwnershipGeneration, ResourceKey: target.ResourceKey, Revision: 1, State: "active", CurrentOutcome: observation.Outcome, FirstObservedAt: observation.ObservedAt, LastObservedAt: observation.ObservedAt, Observation: safe}
		_, err = tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO executor_failure_episodes(id,task_id,environment_id,session_id,ownership_generation,resource_key,revision,state,current_outcome,first_observed_at,last_observed_at,evidence) VALUES(?,?,?,?,?,?,1,'active',?,?,?,?)`), prior.ID, target.TaskID, target.EnvironmentID, target.SessionID, target.OwnershipGeneration, target.ResourceKey, observation.Outcome, observation.ObservedAt, observation.ObservedAt, string(raw))
		if err == nil {
			err = r.captureExecutorFailureSessions(ctx, tx, prior, target)
		}
	} else {
		_, err = tx.ExecContext(ctx, r.db.Rebind(`UPDATE executor_failure_episodes SET revision=revision+1,current_outcome=?,last_observed_at=?,evidence=? WHERE id=? AND state='active'`), observation.Outcome, observation.ObservedAt, string(raw), prior.ID)
		prior.Revision++
		prior.LastObservedAt = observation.ObservedAt
		prior.Observation = safe
		prior.CurrentOutcome = observation.Outcome
	}
	if err != nil {
		return nil, false, err
	}
	return prior, true, nil
}

func (r *Repository) validateExecutorEnvironmentTarget(ctx context.Context, tx *sqlx.Tx, t models.ExecutorObservationTarget) error {
	if err := recoveryclaim.EnsureAvailableTx(ctx, r.db, tx, t.EnvironmentID); err != nil {
		return err
	}
	var taskID, containerID, status string
	var generation int64
	if err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT task_id,ownership_generation,container_id,status FROM task_environments WHERE id=?`), t.EnvironmentID).Scan(&taskID, &generation, &containerID, &status); err != nil {
		return err
	}
	if taskID != t.TaskID || generation != t.OwnershipGeneration || status == taskEnvironmentStatusStopped {
		return fmt.Errorf("executor ownership changed")
	}

	switch t.Runtime {
	case "k8s":
		return r.validateKubernetesExecutorTarget(ctx, tx, t)
	case "standalone":
		return r.validateLocalExecutorAuthority(ctx, tx, t)
	default:
		if containerID != t.ResourceKey {
			return fmt.Errorf("executor container ownership changed")
		}
	}

	return nil
}

func (r *Repository) validateKubernetesExecutorTarget(ctx context.Context, tx *sqlx.Tx, t models.ExecutorObservationTarget) error {

	var raw, operation string
	var revision int64
	if err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT metadata,operation_id,revision FROM task_environment_kubernetes WHERE environment_id=? AND task_id=? AND ownership_generation=?`), t.EnvironmentID, t.TaskID, t.OwnershipGeneration).Scan(&raw, &operation, &revision); err != nil {
		return err
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return err
	}
	if operation != "" || revision != t.InventoryRevision || metadata["kubernetes_pod_uid"] != t.ResourceKey {
		return fmt.Errorf("executor resource ownership changed")
	}
	return nil
}

const executorFailureResolved = "resolved"
