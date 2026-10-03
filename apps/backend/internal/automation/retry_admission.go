package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
)

type retryGroupTriggerIdentity struct {
	ID             string `db:"id"`
	TriggerID      string `db:"trigger_id"`
	TriggerIDsJSON string `db:"trigger_ids"`
	DedupKey       string `db:"dedup_key"`
}

func canonicalRetryTriggerIDs(raw, fallback string) []string {
	var ids []string
	if json.Unmarshal([]byte(raw), &ids) != nil || len(ids) == 0 {
		if fallback != "" {
			ids = []string{fallback}
		}
	}
	sort.Strings(ids)
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != "" && (len(result) == 0 || result[len(result)-1] != id) {
			result = append(result, id)
		}
	}
	return result
}

func sameRetryTriggerSet(left, leftFallback, right, rightFallback string) bool {
	leftIDs := canonicalRetryTriggerIDs(left, leftFallback)
	rightIDs := canonicalRetryTriggerIDs(right, rightFallback)
	if len(leftIDs) != len(rightIDs) {
		return false
	}
	for i := range leftIDs {
		if leftIDs[i] != rightIDs[i] {
			return false
		}
	}
	return true
}

func (s *Store) SetRetryTaskIntentID(ctx context.Context, runID, intentID string) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`UPDATE automation_runs SET retry_task_intent_id = ? WHERE id = ?`), intentID, runID)
	return err
}
func (s *Store) bindRetryIntentTask(ctx context.Context, runID, taskID, state string) error {

	result, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE automation_run_task_intents
		SET task_id = ?, state = ?, updated_at = CURRENT_TIMESTAMP
		WHERE run_id = ? AND (
			EXISTS (
				SELECT 1 FROM automation_runs ar
				JOIN automation_retry_groups rg ON rg.id = ar.retry_group_id
				JOIN automations a ON a.id = ar.automation_id
				WHERE ar.id = ? AND rg.generation = ar.retry_group_generation
					AND rg.state = ? AND a.enabled = TRUE
			) OR EXISTS (
				SELECT 1 FROM automation_runs ar
				WHERE ar.id = ? AND ar.retry_group_id = ''
			)
		)`), taskID, state, runID, runID, RetryGroupLive, runID)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		var groupID string
		if lookupErr := s.db.GetContext(ctx, &groupID, s.db.Rebind(`SELECT retry_group_id FROM automation_runs WHERE id = ?`), runID); lookupErr == nil && groupID != "" {
			return ErrRetryGenerationMismatch
		}
	}
	return nil
}

//nolint:cyclop,funlen // One transaction fences the group, run, intent, operation, and outbox together.
func (s *Store) CreateRetryAdmission(ctx context.Context, run *AutomationRun, group *RetryGroup) error {
	if run == nil || group == nil || run.RetryGroupID == "" || group.ID == "" {
		return fmt.Errorf("retry admission identity is incomplete")
	}
	if run.ID == "" {
		run.ID = uuid.NewString()
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC()
	triggerIDs := canonicalRetryTriggerIDs(group.TriggerIDsJSON, group.TriggerID)
	encodedTriggerIDs, _ := json.Marshal(triggerIDs)
	group.TriggerIDsJSON = string(encodedTriggerIDs)

	if run.AttemptNumber == 0 {
		run.AttemptNumber = 1
	}
	if run.RetryState == "" {
		run.RetryState = RetryStateNone
	}
	if run.RetryLaunchConfigVersion == 0 {
		run.RetryLaunchConfigVersion = RetryLaunchConfigVersion
	}
	run.TriggerDataJSON = string(run.TriggerData)
	run.CreatedAt = now
	intent := &RetryTaskIntent{ID: uuid.NewString(), RunID: run.ID, State: retryIntentAdmitted, GroupGeneration: run.RetryGroupGeneration}
	run.RetryTaskIntentID = intent.ID
	op := &RetryOperation{ID: retryOperationID(intent.ID, run.RetryGroupGeneration, retryTaskOperationKind), IntentID: intent.ID, RunID: run.ID, GroupGeneration: run.RetryGroupGeneration, Kind: retryTaskOperationKind, State: retryOperationRequested}
	payload, err := json.Marshal(&AutomationTriggeredEvent{RunID: run.ID, SnapshotVersion: run.RetryLaunchConfigVersion, RetryExternalID: RetryTaskExternalID(run.ID, run.RetryGroupGeneration)})
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO automation_runs (
			id, automation_id, trigger_id, trigger_type, task_id, status, dedup_key, trigger_data,
			error_message, session_id, turn_id, thread_action, thread_reason, display_title,
			retry_group_id, retry_parent_run_id, attempt_number, retry_state, retry_scheduled_at,
			retry_claimed_at, retry_claim_expires_at, retry_claim_token, retry_group_generation,
			retry_cancelled_at, retry_base_title, retry_failure_phase, retry_failure_class,
			retry_task_intent_id, retry_policy_snapshot, retry_trigger_snapshot,
			retry_launch_config_snapshot, retry_launch_config_version, retry_resolved_prompt,
			retry_resolved_title, retry_resolved_trigger_timestamp, retry_continuation_snapshot,
			automation_revision, trigger_revision, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		run.ID, run.AutomationID, run.TriggerID, run.TriggerType, run.TaskID, run.Status, run.DedupKey,
		run.TriggerDataJSON, run.ErrorMessage, run.SessionID, run.TurnID, run.ThreadAction, run.ThreadReason,
		run.DisplayTitle, run.RetryGroupID, run.RetryParentRunID, run.AttemptNumber, run.RetryState,
		run.RetryScheduledAt, run.RetryClaimedAt, run.RetryClaimExpiresAt, run.RetryClaimToken,
		run.RetryGroupGeneration, run.RetryCancelledAt, run.RetryBaseTitle, run.RetryFailurePhase,
		run.RetryFailureClass, run.RetryTaskIntentID, run.RetryPolicySnapshot, run.RetryTriggerSnapshot,
		run.RetryLaunchConfigSnapshot, run.RetryLaunchConfigVersion, run.RetryResolvedPrompt,
		run.RetryResolvedTitle, run.RetryResolvedTriggerAt, run.RetryContinuationSnapshot,
		run.AutomationRevision, run.TriggerRevision, run.CreatedAt); err != nil {
		return err
	}
	group.CreatedAt, group.UpdatedAt = now, now
	if group.Generation == 0 {
		group.Generation = 1
	}
	if group.State == "" {
		group.State = RetryGroupLive
	}
	var existingGroups []retryGroupTriggerIdentity
	if err := tx.SelectContext(ctx, &existingGroups, tx.Rebind(`
		SELECT rg.id, rg.trigger_id, rg.trigger_ids,
			COALESCE((
				SELECT ar.dedup_key FROM automation_runs ar
				WHERE ar.retry_group_id = rg.id AND ar.attempt_number = 1
				LIMIT 1
			), '') AS dedup_key
		FROM automation_retry_groups rg
		WHERE rg.automation_id = ? AND rg.state = ? AND rg.id != ?`),
		group.AutomationID, RetryGroupLive, group.ID); err != nil {
		return err
	}
	for _, existing := range existingGroups {
		if existing.DedupKey != run.DedupKey ||
			!sameRetryTriggerSet(existing.TriggerIDsJSON, existing.TriggerID, group.TriggerIDsJSON, group.TriggerID) {
			continue
		}
		if _, err := tx.ExecContext(ctx, tx.Rebind(`
			UPDATE automation_retry_groups
			SET generation = generation + 1, state = ?, superseded_by_run_id = ?, updated_at = ?
			WHERE id = ? AND state = ?`),
			RetryGroupSuperseded, run.ID, now, existing.ID, RetryGroupLive); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE automation_runs
		SET status = ?, retry_state = ?, retry_cancelled_at = ?,
			retry_claim_token = '', retry_claimed_at = NULL, retry_claim_expires_at = NULL
		WHERE retry_group_id IN (
			SELECT id FROM automation_retry_groups WHERE superseded_by_run_id = ?
		) AND retry_state IN (?, ?)`),
		RunStatusFailed, RetryStateSuperseded, now, run.ID,
		RetryStateScheduled, RetryStateClaimed); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE automation_runs
		SET status = ?, retry_state = ?, retry_cancelled_at = ?
		WHERE retry_group_id IN (
			SELECT id FROM automation_retry_groups WHERE superseded_by_run_id = ?
		) AND status = ? AND retry_state = ? AND task_id = ''
		AND EXISTS (
			SELECT 1 FROM automation_run_operations o
			WHERE o.run_id = automation_runs.id
				AND o.group_generation = automation_runs.retry_group_generation
				AND o.operation_kind = ? AND o.state = ?
				AND o.external_task_id != ''
		)`),
		RunStatusFailed, RetryStateSuperseded, now, run.ID,
		RunStatusTriggered, RetryStateTriggered, retryTaskOperationKind, retryOperationCommitted); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE automation_runs
		SET status = ?, retry_state = ?, retry_cancelled_at = ?
		WHERE retry_group_id IN (
			SELECT id FROM automation_retry_groups WHERE superseded_by_run_id = ?
		) AND status = ? AND retry_state = ? AND task_id = ''
		AND EXISTS (
			SELECT 1 FROM automation_run_operations o
			WHERE o.run_id = automation_runs.id
				AND o.group_generation = automation_runs.retry_group_generation
				AND o.operation_kind = ? AND o.state = ?
		)`),
		RunStatusFailed, RetryStateSuperseded, now, run.ID,
		RunStatusTriggered, RetryStateTriggered, retryTaskOperationKind, retryOperationRequested); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE automation_run_operations
		SET state = ?, lease_token = '', lease_expires_at = NULL, updated_at = ?
		WHERE run_id IN (
			SELECT id FROM automation_runs WHERE retry_group_id IN (
				SELECT id FROM automation_retry_groups WHERE superseded_by_run_id = ?
			)
		) AND state != ?`), retryOperationAbandoned, now, run.ID, retryOperationCommitted); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE automation_retry_outbox
		SET state = ?, lease_token = '', lease_expires_at = NULL, updated_at = ?
		WHERE run_id IN (
			SELECT id FROM automation_runs WHERE retry_group_id IN (
				SELECT id FROM automation_retry_groups WHERE superseded_by_run_id = ?
			)
		) AND state IN (?, ?)`), retryOutboxRevoked, now, run.ID, retryOutboxPending, retryOutboxLeased); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO automation_retry_groups
			(id, automation_id, trigger_id, trigger_ids, generation, state, superseded_by_run_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`), group.ID, group.AutomationID, group.TriggerID,
		group.TriggerIDsJSON, group.Generation, group.State, group.SupersededByRunID, group.CreatedAt, group.UpdatedAt); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO automation_run_task_intents
			(intent_id, run_id, task_id, state, group_generation, automation_deleted_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`), intent.ID, run.ID, nullableString(intent.TaskID), intent.State,
		intent.GroupGeneration, intent.AutomationDeletedAt, now, now); err != nil {
		return err
	}
	op.CreatedAt, op.UpdatedAt = now, now
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO automation_run_operations
			(operation_id, intent_id, run_id, group_generation, operation_kind, state, lease_token,
			 lease_expires_at, external_task_id, external_session_id, external_turn_id, result_json,
			 created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`), op.ID, op.IntentID, nullableString(op.RunID),
		op.GroupGeneration, op.Kind, op.State, op.LeaseToken, op.LeaseExpiresAt, op.ExternalTaskID,
		op.ExternalSessionID, op.ExternalTurnID, op.ResultJSON, op.CreatedAt, op.UpdatedAt); err != nil {
		return err
	}
	outbox := &RetryOutbox{
		EventID: fmt.Sprintf("%s:%d", run.ID, run.RetryLaunchConfigVersion), RunID: run.ID,
		SnapshotVersion: run.RetryLaunchConfigVersion, PayloadHash: retryPayloadHash(payload),
		State: retryOutboxPending, CreatedAt: now, UpdatedAt: now,
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO automation_retry_outbox
			(event_id, run_id, snapshot_version, payload_hash, state, lease_token, lease_expires_at,
			 enqueued_at, acknowledged_at, attempts, safe_error, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`), outbox.EventID, nullableString(outbox.RunID),
		outbox.SnapshotVersion, outbox.PayloadHash, outbox.State, outbox.LeaseToken, outbox.LeaseExpiresAt,
		outbox.EnqueuedAt, outbox.AcknowledgedAt, outbox.Attempts, outbox.SafeError, outbox.CreatedAt,
		outbox.UpdatedAt); err != nil {
		return err
	}
	return tx.Commit()
}
