package automation

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

var (
	ErrRetryDelayOverflow      = errors.New("automation retry delay overflow")
	ErrNoDueRetry              = errors.New("no due automation retry")
	ErrRetryGenerationMismatch = errors.New("automation retry generation mismatch")
	ErrRetryOutboxLeaseHeld    = errors.New("automation retry outbox lease is held")
)

const (
	retryIntentAdmitted     = "admitted"
	retryIntentCreating     = "creating"
	retryIntentCreated      = "created"
	retryIntentBound        = "bound"
	retryIntentAbandoned    = "abandoned"
	retryOperationRequested = "requested"
	retryOperationLeased    = "leased"
	retryOperationCommitted = "committed"
	retryOperationAbandoned = "abandoned"
	retryOperationAmbiguous = "ambiguous"
	retryOutboxPending      = "pending"
	retryOutboxLeased       = "leased"
	retryOutboxRevoked      = "revoked"
)

// RetryDelay returns the checked delay for retry number one-based.
func RetryDelay(policy RetryPolicy, retryNumber int64) (time.Duration, error) {
	if retryNumber < 1 {
		return 0, fmt.Errorf("retry number must be positive")
	}
	seconds, err := parseRetryDecimal(policy.DelaySeconds)
	if err != nil {
		return 0, err
	}
	if seconds == 0 || policy.Backoff != RetryBackoffExponential || retryNumber == 1 {
		if seconds > math.MaxInt64/int64(time.Second) {
			return 0, ErrRetryDelayOverflow
		}
		return time.Duration(seconds) * time.Second, nil
	}

	result := seconds
	for power := retryNumber - 1; power > 0; power-- {
		if result > math.MaxInt64/2 {
			return 0, ErrRetryDelayOverflow
		}
		result *= 2
	}
	if result > math.MaxInt64/int64(time.Second) {
		return 0, ErrRetryDelayOverflow
	}
	return time.Duration(result) * time.Second, nil
}

// FormatRetryTitle prefixes an immutable root title with its attempt number.
func FormatRetryTitle(root string, attempt int64) string {
	prefix := "Retry " + strconv.FormatInt(attempt, 10) + ": "
	budget := 60 - utf8.RuneCountInString(prefix)
	if budget <= 0 {
		return prefix
	}
	rootRunes := []rune(root)
	if len(rootRunes) <= budget {
		return prefix + root
	}
	if budget == 1 {
		return prefix + "…"
	}
	return prefix + string(rootRunes[:budget-1]) + "…"
}

func runeCount(value string) int { return utf8.RuneCountInString(value) }

// RetryFailure describes the safe, stable representation stored for a failure.
type RetryFailure struct {
	Message      string `json:"message"`
	FailureClass string `json:"failure_class"`
	FailurePhase string `json:"failure_phase"`
}

// SanitizeAutomationFailure stores only an allowlisted phase message.
func SanitizeAutomationFailure(raw error, phase string, _ map[string]string) RetryFailure {
	failureClass := "completion"
	message := "automation attempt failed"
	switch phase {
	case "admission":
		failureClass, message = phase, "automation admission failed"
	case "launch":
		failureClass, message = phase, "automation launch failed"
	case "permission":
		failureClass, message = phase, "automation permission was denied"
	case "cancellation":
		failureClass, message = phase, "automation was cancelled"
	case "retry_schedule":
		failureClass, message = phase, "automation retry could not be scheduled"
		if raw != nil && strings.Contains(strings.ToLower(raw.Error()), "overflow") {
			failureClass, message = "delay_overflow", "automation retry delay is outside the supported range"
		}
	case "launch_snapshot_version":
		failureClass, message = phase, "automation launch configuration is incompatible"
	case "trigger_projection":
		failureClass, message = phase, "automation trigger data could not be projected safely"
	}
	return RetryFailure{Message: message, FailureClass: failureClass, FailurePhase: phase}
}

func retryPolicyFromSnapshot(raw string) (RetryPolicy, error) {
	var policy RetryPolicy
	if raw == "" || raw == "{}" {
		return NormalizeRetryPolicy(policy)
	}
	if err := json.Unmarshal([]byte(raw), &policy); err != nil {
		return RetryPolicy{}, err
	}
	return NormalizeRetryPolicy(policy)
}

func (s *Store) CreateRetryGroup(ctx context.Context, group *RetryGroup) error {
	if group == nil || group.AutomationID == "" {
		return errors.New("retry group automation is required")
	}
	if group.ID == "" {
		group.ID = uuid.NewString()
	}
	if group.Generation == 0 {
		group.Generation = 1
	}
	if group.State == "" {
		group.State = RetryGroupLive
	}
	if group.TriggerIDsJSON == "" {
		if group.TriggerID == "" {
			group.TriggerIDsJSON = "[]"
		} else {
			encoded, _ := json.Marshal([]string{group.TriggerID})
			group.TriggerIDsJSON = string(encoded)
		}
	}
	group.CreatedAt = time.Now().UTC()
	group.UpdatedAt = group.CreatedAt
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO automation_retry_groups
			(id, automation_id, trigger_id, trigger_ids, generation, state, superseded_by_run_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`), group.ID, group.AutomationID, group.TriggerID,
		group.TriggerIDsJSON, group.Generation, group.State, group.SupersededByRunID, group.CreatedAt, group.UpdatedAt)
	return err
}

func (s *Store) GetRetryGroup(ctx context.Context, id string) (*RetryGroup, error) {
	var group RetryGroup
	if err := s.ro.GetContext(ctx, &group, s.ro.Rebind(`SELECT * FROM automation_retry_groups WHERE id = ?`), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &group, nil
}

// FinalizeRetryFailure settles an attempt and inserts its unique child.
//
//nolint:gocognit,cyclop,funlen,nestif,maintidx // One transaction owns all parent, child, and outbox CAS.
func (s *Store) FinalizeRetryFailure(ctx context.Context, runID string, generation int64, raw error, phase string) (*AutomationRun, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var parent AutomationRun
	if err := tx.GetContext(ctx, &parent, tx.Rebind(`SELECT * FROM automation_runs WHERE id = ?`), runID); err != nil {
		return nil, err
	}
	if parent.RetryGroupID == "" {
		return nil, errors.New("run is not retry-enabled")
	}
	if parent.RetryGroupGeneration != generation {
		return nil, ErrRetryGenerationMismatch
	}
	var group RetryGroup
	if err := tx.GetContext(ctx, &group, tx.Rebind(`SELECT * FROM automation_retry_groups WHERE id = ?`), parent.RetryGroupID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE automation_retry_outbox
		SET state = ?, lease_token = '', lease_expires_at = NULL, updated_at = ?
		WHERE run_id = ? AND state IN (?, ?)`),
		retryOutboxRevoked, time.Now().UTC(), runID, retryOutboxPending, retryOutboxLeased); err != nil {
		return nil, err
	}
	if group.State == RetryGroupSuperseded && group.Generation != generation {
		failure := SanitizeAutomationFailure(raw, phase, nil)
		result, execErr := tx.ExecContext(ctx, tx.Rebind(`
			UPDATE automation_runs
			SET status = ?, retry_state = ?, retry_failure_phase = ?,
				retry_failure_class = ?, error_message = ?
			WHERE id = ? AND retry_group_generation = ?
				AND status IN (?, ?) AND retry_state IN (?, ?)`),
			RunStatusFailed, RetryStateCompleted, failure.FailurePhase,
			failure.FailureClass, failure.Message, runID, generation,
			RunStatusTriggered, RunStatusTaskCreated, RetryStateTriggered, RetryStateNone)
		if execErr != nil {
			return nil, execErr
		}
		if affected, _ := result.RowsAffected(); affected != 1 {
			return nil, ErrRetryGenerationMismatch
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return nil, nil
	}
	if group.State != RetryGroupLive || group.Generation != generation {
		return nil, ErrRetryGenerationMismatch
	}
	if parent.Status == RunStatusFailed && parent.RetryState == RetryStateCompleted {
		var existing AutomationRun
		err := tx.GetContext(ctx, &existing, tx.Rebind(`
			SELECT * FROM automation_runs
			WHERE retry_group_id = ? AND retry_parent_run_id = ? AND attempt_number = ?`),
			parent.RetryGroupID, parent.ID, parent.AttemptNumber+1)
		if err == nil {
			if err := tx.Commit(); err != nil {
				return nil, err
			}
			return &existing, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		return nil, ErrRetryGenerationMismatch
	}
	failure := SanitizeAutomationFailure(raw, phase, nil)
	policy, err := retryPolicyFromSnapshot(parent.RetryPolicySnapshot)
	if err != nil {
		return nil, err
	}
	nextNumber := parent.AttemptNumber + 1
	maxRetries, _ := parseRetryDecimal(policy.MaxRetries)
	if policy.Mode == RetryModeDisabled || (policy.Mode == RetryModeFinite && parent.AttemptNumber > maxRetries) {
		parentResult, execErr := tx.ExecContext(ctx, tx.Rebind(`UPDATE automation_runs SET status = ?, retry_state = ?, retry_failure_phase = ?, retry_failure_class = ?, error_message = ? WHERE id = ? AND retry_group_generation = ?`),
			RunStatusFailed, RetryStateExhausted, failure.FailurePhase, failure.FailureClass, failure.Message, runID, generation)
		if execErr != nil {
			return nil, execErr
		}
		if affected, _ := parentResult.RowsAffected(); affected != 1 {
			return nil, ErrRetryGenerationMismatch
		}
		groupResult, execErr := tx.ExecContext(ctx, tx.Rebind(`UPDATE automation_retry_groups SET state = ?, updated_at = ? WHERE id = ? AND generation = ?`), RetryGroupCompleted, time.Now().UTC(), group.ID, generation)
		if execErr != nil {
			return nil, execErr
		}
		if affected, _ := groupResult.RowsAffected(); affected != 1 {
			return nil, ErrRetryGenerationMismatch
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return nil, nil
	}
	delay, delayErr := RetryDelay(policy, nextNumber-1)
	due := time.Now().UTC().Add(delay)
	childState := RetryStateScheduled
	childStatus := RunStatusScheduledRetry
	if delayErr != nil {
		childState = RetryStateSchedulingFailed
		childStatus = RunStatusRetrySchedulingFailed
		due = time.Time{}
		failure = SanitizeAutomationFailure(delayErr, "retry_schedule", nil)
	}
	child := parent
	child.ID = uuid.NewString()
	child.RetryParentRunID = parent.ID
	child.AttemptNumber = nextNumber
	child.Status = childStatus
	child.RetryState = childState
	child.RetryScheduledAt = nil
	if !due.IsZero() {
		child.RetryScheduledAt = &due
	}
	child.RetryClaimedAt = nil
	child.RetryClaimExpiresAt = nil
	child.RetryClaimToken = ""
	child.RetryFailurePhase = failure.FailurePhase
	child.RetryFailureClass = failure.FailureClass
	child.ErrorMessage = failure.Message
	child.TaskID, child.SessionID, child.TurnID = "", "", ""
	child.DedupKey = ""
	child.RetryTaskIntentID = uuid.NewString()
	if child.RetryBaseTitle == "" {
		child.RetryBaseTitle = parent.DisplayTitle
	}
	child.DisplayTitle = FormatRetryTitle(child.RetryBaseTitle, nextNumber)
	child.CreatedAt = time.Now().UTC()
	parentResult, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE automation_runs SET status = ?, retry_state = ?, retry_failure_phase = ?, retry_failure_class = ?, error_message = ? WHERE id = ? AND retry_state IN (?, ?)`),
		RunStatusFailed, RetryStateCompleted, failure.FailurePhase, failure.FailureClass, failure.Message, runID, RetryStateTriggered, RetryStateNone)
	if err != nil {
		return nil, err
	}
	if affected, _ := parentResult.RowsAffected(); affected != 1 {
		return nil, ErrRetryGenerationMismatch
	}
	if _, err = tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO automation_runs (id, automation_id, trigger_id, trigger_type, task_id, status, dedup_key, trigger_data,
			error_message, session_id, turn_id, thread_action, thread_reason, display_title, retry_group_id, retry_parent_run_id,
			attempt_number, retry_state, retry_scheduled_at, retry_claimed_at, retry_claim_expires_at, retry_claim_token,
			retry_group_generation, retry_cancelled_at, retry_base_title, retry_failure_phase, retry_failure_class, retry_task_intent_id,
			retry_policy_snapshot, retry_trigger_snapshot, retry_launch_config_snapshot, retry_launch_config_version,
			retry_resolved_prompt, retry_resolved_title, retry_resolved_trigger_timestamp, retry_continuation_snapshot,
			automation_revision, trigger_revision, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		child.ID, child.AutomationID, child.TriggerID, child.TriggerType, child.TaskID, child.Status, child.DedupKey,
		child.TriggerDataJSON, child.ErrorMessage, child.SessionID, child.TurnID, child.ThreadAction, child.ThreadReason,
		child.DisplayTitle, child.RetryGroupID, child.RetryParentRunID, child.AttemptNumber, child.RetryState,
		child.RetryScheduledAt, child.RetryClaimedAt, child.RetryClaimExpiresAt, child.RetryClaimToken,
		child.RetryGroupGeneration, child.RetryCancelledAt, child.RetryBaseTitle, child.RetryFailurePhase,
		child.RetryFailureClass, child.RetryTaskIntentID, child.RetryPolicySnapshot, child.RetryTriggerSnapshot,
		child.RetryLaunchConfigSnapshot, child.RetryLaunchConfigVersion, child.RetryResolvedPrompt, child.RetryResolvedTitle,
		child.RetryResolvedTriggerAt, child.RetryContinuationSnapshot, child.AutomationRevision, child.TriggerRevision, child.CreatedAt); err != nil {
		var existing AutomationRun
		if !isUniqueConstraint(err) || tx.GetContext(ctx, &existing, tx.Rebind(`SELECT * FROM automation_runs WHERE retry_group_id = ? AND attempt_number = ?`), parent.RetryGroupID, nextNumber) != nil {
			return nil, err
		}
		child = existing
		if existing.RetryPolicySnapshot != parent.RetryPolicySnapshot ||
			existing.RetryTriggerSnapshot != parent.RetryTriggerSnapshot ||
			existing.RetryGroupGeneration != generation ||
			existing.RetryParentRunID != parent.ID {
			return nil, errors.New("retry child consistency mismatch")
		}
	}
	if child.RetryTaskIntentID == "" {
		child.RetryTaskIntentID = uuid.NewString()
		if _, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE automation_runs SET retry_task_intent_id = ? WHERE id = ?`), child.RetryTaskIntentID, child.ID); err != nil {
			return nil, err
		}
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO automation_run_task_intents
			(intent_id, run_id, task_id, state, group_generation, automation_deleted_at, created_at, updated_at)
		VALUES (?, ?, NULL, ?, ?, NULL, ?, ?) ON CONFLICT DO NOTHING`),
		child.RetryTaskIntentID, child.ID, retryIntentAdmitted, child.RetryGroupGeneration, now, now); err != nil {
		return nil, err
	}
	var intentID string
	if err := tx.GetContext(ctx, &intentID, tx.Rebind(
		`SELECT intent_id FROM automation_run_task_intents WHERE run_id = ?`), child.ID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO automation_run_operations
			(operation_id, intent_id, run_id, group_generation, operation_kind, state,
			 lease_token, lease_expires_at, external_task_id, external_session_id,
			 external_turn_id, result_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, '', NULL, '', '', '', '{}', ?, ?)
		ON CONFLICT DO NOTHING`),
		retryOperationID(intentID, child.RetryGroupGeneration, retryTaskOperationKind),
		intentID, child.ID, child.RetryGroupGeneration, retryTaskOperationKind,
		retryOperationRequested, now, now); err != nil {
		return nil, err
	}
	if delayErr != nil {
		groupResult, execErr := tx.ExecContext(ctx, tx.Rebind(`UPDATE automation_retry_groups SET state = ?, updated_at = ? WHERE id = ? AND generation = ?`), RetryGroupCompleted, time.Now().UTC(), group.ID, generation)
		if execErr != nil {
			return nil, execErr
		}
		if affected, _ := groupResult.RowsAffected(); affected != 1 {
			return nil, ErrRetryGenerationMismatch
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return &child, nil
	}
	payload, marshalErr := json.Marshal(&AutomationTriggeredEvent{RunID: child.ID, SnapshotVersion: child.RetryLaunchConfigVersion})
	if marshalErr != nil {
		return nil, marshalErr
	}
	now = time.Now().UTC()
	if _, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO automation_retry_outbox (event_id, run_id, snapshot_version, payload_hash, state, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT(event_id) DO NOTHING`), fmt.Sprintf("%s:%d", child.ID, child.RetryLaunchConfigVersion), child.ID, child.RetryLaunchConfigVersion, retryPayloadHash(payload), retryOutboxPending, now, now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &child, nil
}

func isUniqueConstraint(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "unique")
}

func (s *Store) ClaimDueRetry(ctx context.Context, now time.Time, lease time.Duration) (*AutomationRun, string, error) {
	return s.claimDueRetry(ctx, now, lease, "")
}

func (s *Store) ClaimDueRetryForAutomation(ctx context.Context, now time.Time, lease time.Duration, automationID string) (*AutomationRun, string, error) {
	return s.claimDueRetry(ctx, now, lease, automationID)
}

//nolint:funlen // The claim query and its poison-row repair are one lease boundary.
func (s *Store) claimDueRetry(ctx context.Context, now time.Time, lease time.Duration, automationID string) (*AutomationRun, string, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = tx.Rollback() }()
	filter := ""
	var filterArgs []any
	if automationID != "" {
		filter = " AND ar.automation_id = ?"
		filterArgs = append(filterArgs, automationID)
	}
	var run AutomationRun
	var poisonID string
	poisonQuery := `
		SELECT ar.id FROM automation_runs ar
		JOIN automation_retry_groups rg ON rg.id = ar.retry_group_id
			AND rg.generation = ar.retry_group_generation AND rg.state = ?
		JOIN automations a ON a.id = ar.automation_id AND a.enabled = TRUE
		WHERE ar.status = ? AND ar.retry_state = ? AND ar.retry_scheduled_at IS NOT NULL
			AND ar.retry_scheduled_at <= ?
			AND (NOT EXISTS (
				SELECT 1 FROM automation_run_task_intents i
				WHERE i.run_id = ar.id AND i.group_generation = ar.retry_group_generation
					AND i.state = ?
			) OR NOT EXISTS (
				SELECT 1 FROM automation_run_operations o
				WHERE o.run_id = ar.id AND o.group_generation = ar.retry_group_generation
					AND o.operation_kind = ? AND o.state = ?
			))` + filter + ` ORDER BY ar.retry_scheduled_at ASC, ar.id ASC LIMIT 1`
	poisonArgs := []any{RetryGroupLive, RunStatusScheduledRetry, RetryStateScheduled, now,
		retryIntentAdmitted, retryTaskOperationKind, retryOperationRequested}
	poisonArgs = append(poisonArgs, filterArgs...)
	poisonErr := tx.GetContext(ctx, &poisonID, tx.Rebind(poisonQuery), poisonArgs...)
	if poisonErr == nil {
		if _, err := tx.ExecContext(ctx, tx.Rebind(`
			UPDATE automation_runs SET status = ?, retry_state = ?, error_message = ?
			WHERE id = ? AND retry_state = ?`),
			RunStatusRetrySchedulingFailed, RetryStateSchedulingFailed,
			"retry task operation is missing", poisonID, RetryStateScheduled); err != nil {
			return nil, "", err
		}
		groupResult, groupErr := tx.ExecContext(ctx, tx.Rebind(`
			UPDATE automation_retry_groups
			SET state = ?, updated_at = ?
			WHERE id = (SELECT retry_group_id FROM automation_runs WHERE id = ?)
				AND generation = (SELECT retry_group_generation FROM automation_runs WHERE id = ?)
				AND state = ?`),
			RetryGroupCompleted, now, poisonID, poisonID, RetryGroupLive)
		if groupErr != nil {
			return nil, "", groupErr
		}
		if affected, _ := groupResult.RowsAffected(); affected != 1 {
			return nil, "", ErrRetryGenerationMismatch
		}
		if err := tx.Commit(); err != nil {
			return nil, "", err
		}
		return nil, "", ErrNoDueRetry
	}
	if !errors.Is(poisonErr, sql.ErrNoRows) {
		return nil, "", poisonErr
	}
	runQuery := `
		SELECT ar.* FROM automation_runs ar
		JOIN automation_retry_groups rg ON rg.id = ar.retry_group_id AND rg.generation = ar.retry_group_generation AND rg.state = ?
		JOIN automations a ON a.id = ar.automation_id AND a.enabled = TRUE
		JOIN automation_run_task_intents i ON i.run_id = ar.id AND i.group_generation = ar.retry_group_generation AND i.state = ?
		JOIN automation_run_operations o ON o.run_id = ar.id AND o.group_generation = ar.retry_group_generation AND o.operation_kind = ? AND o.state = ?
		WHERE ar.status = ? AND ar.retry_state = ? AND ar.retry_scheduled_at IS NOT NULL AND ar.retry_scheduled_at <= ?` + filter + `
		ORDER BY ar.retry_scheduled_at ASC, ar.id ASC LIMIT 1`
	runArgs := []any{RetryGroupLive, retryIntentAdmitted, retryTaskOperationKind, retryOperationRequested,
		RunStatusScheduledRetry, RetryStateScheduled, now}
	runArgs = append(runArgs, filterArgs...)
	err = tx.GetContext(ctx, &run, tx.Rebind(runQuery), runArgs...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", ErrNoDueRetry
	}
	if err != nil {
		return nil, "", err
	}
	token := uuid.NewString()
	expires := now.Add(lease)
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE automation_runs SET retry_state = ?, retry_claimed_at = ?, retry_claim_expires_at = ?, retry_claim_token = ? WHERE id = ? AND status = ? AND retry_state = ? AND retry_group_generation = ?`), RetryStateClaimed, now, expires, token, run.ID, RunStatusScheduledRetry, RetryStateScheduled, run.RetryGroupGeneration)
	if err != nil {
		return nil, "", err
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return nil, "", ErrNoDueRetry
	}
	if err := tx.Commit(); err != nil {
		return nil, "", err
	}
	run.RetryState, run.RetryClaimedAt, run.RetryClaimExpiresAt, run.RetryClaimToken = RetryStateClaimed, &now, &expires, token
	return &run, token, nil
}

func (s *Store) ListDueRetryAutomationIDs(ctx context.Context, now time.Time) ([]string, error) {
	var ids []string
	err := s.ro.SelectContext(ctx, &ids, s.ro.Rebind(`
		SELECT DISTINCT ar.automation_id
		FROM automation_runs ar
		JOIN automation_retry_groups rg ON rg.id = ar.retry_group_id
			AND rg.generation = ar.retry_group_generation AND rg.state = ?
		JOIN automations a ON a.id = ar.automation_id AND a.enabled = TRUE
		WHERE ar.status = ? AND ar.retry_state = ?
			AND ar.retry_scheduled_at IS NOT NULL AND ar.retry_scheduled_at <= ?
		ORDER BY ar.automation_id ASC`),
		RetryGroupLive, RunStatusScheduledRetry, RetryStateScheduled, now)
	return ids, err
}

func (s *Store) ReleaseRetryClaim(ctx context.Context, runID, token string, generation int64) error {
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`UPDATE automation_runs SET retry_state = ?, retry_claimed_at = NULL, retry_claim_expires_at = NULL, retry_claim_token = '' WHERE id = ? AND retry_group_generation = ? AND retry_state = ? AND retry_claim_token = ?`), RetryStateScheduled, runID, generation, RetryStateClaimed, token)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrRetryGenerationMismatch
	}
	return nil
}

func (s *Store) CancelRetryGroup(ctx context.Context, groupID string, generation int64) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE automation_retry_groups SET generation = generation + 1, state = ?, updated_at = ? WHERE id = ? AND generation = ? AND state = ?`), RetryGroupCancelled, now, groupID, generation, RetryGroupLive)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return ErrRetryGenerationMismatch
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE automation_runs SET status = ?, retry_state = ?, retry_cancelled_at = ?, retry_claim_token = '', retry_claimed_at = NULL, retry_claim_expires_at = NULL WHERE retry_group_id = ? AND retry_group_generation = ? AND retry_state NOT IN (?, ?, ?, ?, ?)`), RunStatusFailed, RetryStateCancelled, now, groupID, generation, RetryStateCompleted, RetryStateExhausted, RetryStateCancelled, RetryStateSuperseded, RetryStateSchedulingFailed); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE automation_run_task_intents
		SET state = ?, automation_deleted_at = ?, updated_at = ?
		WHERE run_id IN (
			SELECT id FROM automation_runs
			WHERE retry_group_id = ? AND retry_group_generation = ?
		) AND state != ?`),
		retryIntentAbandoned, now, now, groupID, generation, retryIntentAbandoned); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE automation_run_operations SET state = ?, updated_at = ? WHERE run_id IN (SELECT id FROM automation_runs WHERE retry_group_id = ? AND retry_group_generation = ?) AND state NOT IN (?, ?)`), retryOperationAbandoned, now, groupID, generation, retryOperationCommitted, retryOperationAbandoned); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE automation_retry_outbox SET state = ?, updated_at = ? WHERE run_id IN (SELECT id FROM automation_runs WHERE retry_group_id = ? AND retry_group_generation = ?) AND state IN (?, ?)`), retryOutboxRevoked, now, groupID, generation, retryOutboxPending, retryOutboxLeased); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) MarkRetrySucceeded(ctx context.Context, runID string, generation int64) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE automation_runs SET status = ?, retry_state = ? WHERE id = ? AND retry_group_generation = ? AND status IN (?, ?) AND retry_state IN (?, ?, ?)`), RunStatusSucceeded, RetryStateCompleted, runID, generation, RunStatusTriggered, RunStatusTaskCreated, RetryStateTriggered, RetryStateClaimed, RetryStateNone)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return nil
	}
	var groupID string
	if err := tx.GetContext(ctx, &groupID, tx.Rebind(`SELECT retry_group_id FROM automation_runs WHERE id = ?`), runID); err != nil {
		return err
	}
	var group RetryGroup
	if err := tx.GetContext(ctx, &group, tx.Rebind(`SELECT * FROM automation_retry_groups WHERE id = ?`), groupID); err != nil {
		return err
	}
	if group.State == RetryGroupSuperseded && group.Generation != generation {
		return tx.Commit()
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE automation_retry_groups SET state = ?, updated_at = ? WHERE id = ? AND generation = ?`), RetryGroupCompleted, time.Now().UTC(), groupID, generation); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE automation_runs SET retry_state = ?, retry_cancelled_at = ? WHERE retry_group_id = ? AND retry_group_generation = ? AND retry_state IN (?, ?)`), RetryStateSuperseded, time.Now().UTC(), groupID, generation, RetryStateScheduled, RetryStateClaimed); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) CreateRetryIntent(ctx context.Context, intent *RetryTaskIntent) error {
	if intent.ID == "" {
		intent.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	intent.CreatedAt, intent.UpdatedAt = now, now
	if intent.State == "" {
		intent.State = retryIntentAdmitted
	}
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`INSERT INTO automation_run_task_intents (intent_id, run_id, task_id, state, group_generation, automation_deleted_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`), intent.ID, nullableString(intent.RunID), nullableString(intent.TaskID), intent.State, intent.GroupGeneration, intent.AutomationDeletedAt, intent.CreatedAt, intent.UpdatedAt)
	return err
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func (s *Store) CreateRetryOperation(ctx context.Context, op *RetryOperation) error {
	if op.ID == "" {
		op.ID = retryOperationID(op.IntentID, op.GroupGeneration, op.Kind)
	}
	now := time.Now().UTC()
	op.CreatedAt, op.UpdatedAt = now, now
	if op.State == "" {
		op.State = retryOperationRequested
	}
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`INSERT INTO automation_run_operations (operation_id, intent_id, run_id, group_generation, operation_kind, state, lease_token, lease_expires_at, external_task_id, external_session_id, external_turn_id, result_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`), op.ID, op.IntentID, nullableString(op.RunID), op.GroupGeneration, op.Kind, op.State, op.LeaseToken, op.LeaseExpiresAt, op.ExternalTaskID, op.ExternalSessionID, op.ExternalTurnID, op.ResultJSON, op.CreatedAt, op.UpdatedAt)
	return err
}

func retryOperationID(intentID string, generation int64, kind string) string {
	return fmt.Sprintf("%s:%d:%s", intentID, generation, kind)
}

func (s *Store) CreateRetryOutbox(ctx context.Context, outbox *RetryOutbox) error {
	if outbox.EventID == "" {
		return errors.New("retry outbox event id is required")
	}
	now := time.Now().UTC()
	outbox.CreatedAt, outbox.UpdatedAt = now, now
	if outbox.State == "" {
		outbox.State = retryOutboxPending
	}
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`INSERT INTO automation_retry_outbox (event_id, run_id, snapshot_version, payload_hash, state, lease_token, lease_expires_at, enqueued_at, acknowledged_at, attempts, safe_error, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`), outbox.EventID, nullableString(outbox.RunID), outbox.SnapshotVersion, outbox.PayloadHash, outbox.State, outbox.LeaseToken, outbox.LeaseExpiresAt, outbox.EnqueuedAt, outbox.AcknowledgedAt, outbox.Attempts, outbox.SafeError, outbox.CreatedAt, outbox.UpdatedAt)
	return err
}

func (s *Store) AcknowledgeRetryEvent(ctx context.Context, eventID, leaseToken, runID string, version int64) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE automation_retry_outbox
		SET state = ?, acknowledged_at = ?, lease_token = '', lease_expires_at = NULL, updated_at = ?
		WHERE event_id = ? AND state = ? AND lease_token = ?
			AND run_id = ? AND snapshot_version = ?`),
		"published", now, now, eventID, retryOutboxLeased, leaseToken, runID, version)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		var receipt struct {
			RunID           string `db:"run_id"`
			SnapshotVersion int64  `db:"snapshot_version"`
		}
		if err := tx.GetContext(ctx, &receipt, tx.Rebind(
			`SELECT run_id, snapshot_version FROM automation_retry_event_receipts WHERE event_id = ?`),
			eventID); err == nil && receipt.RunID == runID && receipt.SnapshotVersion == version {
			return nil
		}
		return ErrRetryOutboxLeaseHeld
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO automation_retry_event_receipts (event_id, run_id, snapshot_version, accepted_at)
		VALUES (?, ?, ?, ?) ON CONFLICT(event_id) DO NOTHING`),
		eventID, nullableString(runID), version, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) FailRetryOutbox(ctx context.Context, eventID, leaseToken string, raw error) error {
	failure := SanitizeAutomationFailure(raw, "launch", nil)
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE automation_retry_outbox
		SET state = ?, safe_error = ?, lease_token = '', lease_expires_at = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE event_id = ? AND state = ? AND lease_token = ?`),
		retryOutboxRevoked, failure.Message, eventID, retryOutboxLeased, leaseToken)
	return err
}
func (s *Store) RevokePendingRetryOutbox(ctx context.Context, eventID string, raw error) error {
	failure := SanitizeAutomationFailure(raw, "launch", nil)
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE automation_retry_outbox
		SET state = ?, safe_error = ?, updated_at = CURRENT_TIMESTAMP
		WHERE event_id = ? AND state = ? AND lease_token = ''`),
		retryOutboxRevoked, failure.Message, eventID, retryOutboxPending)
	return err
}

func retryPayloadHash(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}
