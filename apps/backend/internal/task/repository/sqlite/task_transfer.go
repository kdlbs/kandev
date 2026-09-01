package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/steptelemetry"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

type transferWorkspaceProjection struct {
	table          string
	taskColumn     string
	identityColumn string
	receiptKey     string
}

const (
	taskTransferResultDenied   = "denied"
	taskTransferResultConflict = "conflict"
	taskTransferResultFailed   = "failed"
	taskTransferOperationIndex = "idx_task_transfer_operations_idempotency"
)

const sqliteTaskTransferOperationViolation = "UNIQUE constraint failed: task_transfer_operations.source_workspace_id, task_transfer_operations.idempotency_key"
const postgresForUpdateClause = " FOR UPDATE"

var transferWorkspaceProjections = []transferWorkspaceProjection{
	{table: "task_status_summaries", taskColumn: "task_id"},
	{table: "task_message_attachments", taskColumn: "task_id"},
	{table: "github_pr_watches", taskColumn: "task_id"},
	{table: "github_task_prs", taskColumn: "task_id"},
	{table: "task_delivery_ledger", taskColumn: "task_id"},
	{table: "azure_devops_task_work_items", taskColumn: "task_id"},
	{table: "automation_task_cleanup_jobs", taskColumn: "task_id"},
	{table: "storage_quarantine_entries", taskColumn: "task_id"},
	{table: "office_channels", taskColumn: "task_id"},
	{table: "task_workspace_groups", taskColumn: "owner_task_id"},
}

var transferPreservationTables = []transferWorkspaceProjection{
	{table: "task_plans", taskColumn: "task_id", identityColumn: "id"},
	{table: "task_plan_revisions", taskColumn: "task_id", identityColumn: "id"},
	{table: "task_walkthroughs", taskColumn: "task_id", identityColumn: "id"},
	{table: "task_documents", taskColumn: "task_id", identityColumn: "id"},
	{table: "task_document_revisions", taskColumn: "task_id", identityColumn: "id"},
	{table: "task_session_messages", taskColumn: "task_id", identityColumn: "id"},
	{table: "task_session_turns", taskColumn: "task_id", identityColumn: "id"},
	{table: "task_sessions", taskColumn: "task_id", identityColumn: "id"},
	{table: "task_repositories", taskColumn: "task_id", identityColumn: "id"},
	{table: "task_workspace_folders", taskColumn: "task_id", identityColumn: "id"},
	{table: "task_environments", taskColumn: "task_id", identityColumn: "id"},
	{table: "task_review_runs", taskColumn: "task_id", identityColumn: "id"},
	{table: "task_review_findings", taskColumn: "task_id", identityColumn: "id"},
	{table: "task_status_summaries", taskColumn: "task_id", identityColumn: "task_id"},
	{table: "task_message_attachments", taskColumn: "task_id", identityColumn: "id"},
	{table: "github_pr_watches", taskColumn: "task_id", identityColumn: "id"},
	{table: "github_task_prs", taskColumn: "task_id", identityColumn: "id"},
	{table: "task_delivery_ledger", taskColumn: "task_id", identityColumn: "id"},
	{table: "azure_devops_task_prs", taskColumn: "task_id", identityColumn: "id"},
	{table: "azure_devops_task_work_items", taskColumn: "task_id", identityColumn: "id"},
	{table: "task_usage_events", taskColumn: "task_id", identityColumn: "id"},
	{table: "office_cost_events", taskColumn: "task_id", identityColumn: "id"},
	{table: "task_comments", taskColumn: "task_id", identityColumn: "id"},
	{table: "pending_moves", taskColumn: "task_id", identityColumn: "id"},
	{table: "workflow_step_participants", taskColumn: "task_id", identityColumn: "id"},
	{table: "workflow_step_decisions", taskColumn: "task_id", identityColumn: "id"},
	{table: "office_task_labels", taskColumn: "task_id", identityColumn: "label_id"},
	{table: "task_workspace_group_members", taskColumn: "task_id", identityColumn: "workspace_group_id"},
	{table: "task_blockers", taskColumn: "task_id", identityColumn: "blocker_task_id"},
	{table: "task_blockers", taskColumn: "blocker_task_id", identityColumn: "task_id", receiptKey: "task_blockers_as_blocker"},
}

type persistedTaskTransfer struct {
	RequestDigest  string `db:"request_digest"`
	ReceiptJSON    string `db:"receipt_json"`
	ActorKind      string `db:"actor_kind"`
	ActorID        string `db:"actor_id"`
	ActorSessionID string `db:"actor_session_id"`
}

type transferTaskPlacement struct {
	WorkspaceID  string    `db:"workspace_id"`
	WorkflowID   string    `db:"workflow_id"`
	StepID       string    `db:"workflow_step_id"`
	QueuedStepID string    `db:"queued_for_step_id"`
	ProjectID    string    `db:"project_id"`
	WIPAdmitted  int       `db:"wip_admitted"`
	UpdatedAt    time.Time `db:"updated_at"`
}

type transferRelationInventory struct {
	labels, groups, pendingMoves, participants, decisions, treeHolds, agentProfiles bool
}

type transferStepSemantics struct {
	Name                       string `db:"name"`
	Prompt                     string `db:"prompt"`
	WorkflowPrompt             string `db:"workflow_prompt"`
	Events                     string `db:"events"`
	EffectiveAgentProfileID    string `db:"effective_agent_profile_id"`
	StageType                  string `db:"stage_type"`
	PullFromName               string `db:"pull_from_name"`
	AllowManualMove            int    `db:"allow_manual_move"`
	IsStartStep                int    `db:"is_start_step"`
	AutoArchiveAfterHours      int    `db:"auto_archive_after_hours"`
	AutoAdvanceRequiresSignal  int    `db:"auto_advance_requires_signal"`
	CancelTriggersTurnComplete int    `db:"cancel_triggers_turn_complete"`
	WIPLimit                   int    `db:"wip_limit"`
}

type transferStepParticipantSemantics struct {
	Role             string `db:"role"`
	AgentProfileID   string `db:"agent_profile_id"`
	DecisionRequired int    `db:"decision_required"`
	Position         int    `db:"position"`
	Provenance       string `db:"provenance"`
}

// TransferTask atomically rebinds one exact task placement while preserving
// every task-keyed durable/runtime association.
func (r *Repository) TransferTask(
	ctx context.Context,
	command models.TaskTransferCommand,
) (*models.TaskTransferReceipt, error) {
	command.DestinationStepName = strings.TrimSpace(command.DestinationStepName)
	if err := validateTaskTransferCommand(command); err != nil {
		return nil, errors.Join(err, r.RecordTaskTransferAttempt(ctx, command, taskTransferResultConflict))
	}
	requestDigest, err := taskTransferRequestDigest(command)
	if err != nil {
		return nil, errors.Join(err, r.RecordTaskTransferAttempt(ctx, command, taskTransferResultFailed))
	}
	if receipt, found, replayErr := r.replayTaskTransferCommitted(ctx, command, requestDigest); replayErr != nil {
		return nil, errors.Join(replayErr, r.RecordTaskTransferAttempt(ctx, command, taskTransferResultForError(replayErr)))
	} else if found {
		return receipt, nil
	}
	projections, counts, inventory, err := r.inspectTaskTransferRelations(ctx)
	if err != nil {
		return nil, errors.Join(err, r.RecordTaskTransferAttempt(ctx, command, taskTransferResultFailed))
	}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, errors.Join(err, r.RecordTaskTransferAttempt(ctx, command, taskTransferResultFailed))
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.lockTaskTransferRelations(ctx, tx, projections, counts, inventory); err != nil {
		_ = tx.Rollback()
		return nil, errors.Join(err, r.RecordTaskTransferAttempt(ctx, command, taskTransferResultFailed))
	}
	receipt, err := r.applyTaskTransfer(ctx, tx, command, requestDigest, projections, counts, inventory)
	if err != nil {
		_ = tx.Rollback()
		return r.handleTaskTransferApplyError(ctx, command, requestDigest, err)
	}
	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		return nil, errors.Join(err, r.RecordTaskTransferAttempt(ctx, command, taskTransferResultFailed))
	}
	return receipt, nil
}

func (r *Repository) handleTaskTransferApplyError(
	ctx context.Context,
	command models.TaskTransferCommand,
	requestDigest string,
	applyErr error,
) (*models.TaskTransferReceipt, error) {
	if errors.Is(applyErr, repoerrors.ErrTaskTransferConflict) || isTaskTransferOperationViolation(applyErr) {
		replay, found, replayErr := r.replayTaskTransferCommitted(ctx, command, requestDigest)
		switch {
		case replayErr != nil:
			return nil, errors.Join(replayErr, r.RecordTaskTransferAttempt(ctx, command, taskTransferResultConflict))
		case found:
			return replay, nil
		case isTaskTransferOperationViolation(applyErr):
			applyErr = fmt.Errorf("%w: idempotency key request differs", repoerrors.ErrTaskTransferConflict)
		}
	}
	result := taskTransferResultFailed
	if errors.Is(applyErr, repoerrors.ErrTaskTransferConflict) {
		result = taskTransferResultConflict
	}
	return nil, errors.Join(applyErr, r.RecordTaskTransferAttempt(ctx, command, result))
}

func isTaskTransferOperationViolation(err error) bool {
	if err == nil {
		return false
	}
	if strings.Contains(err.Error(), sqliteTaskTransferOperationViolation) {
		return true
	}
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == taskTransferOperationIndex
}

func taskTransferResultForError(err error) string {
	if errors.Is(err, repoerrors.ErrTaskTransferConflict) {
		return taskTransferResultConflict
	}
	return taskTransferResultFailed
}

func validateTaskTransferCommand(command models.TaskTransferCommand) error {
	if taskTransferCommandMissingPredicate(command) {
		return fmt.Errorf("%w: required transfer predicate is missing", repoerrors.ErrTaskTransferConflict)
	}
	if taskTransferCommandUnsupported(command) {
		return fmt.Errorf("%w: unsupported transfer request", repoerrors.ErrTaskTransferConflict)
	}
	if command.Actor.Kind != models.TaskTransferActorHuman && command.Actor.Kind != models.TaskTransferActorCoordinator {
		return fmt.Errorf("%w: actor is not attested", repoerrors.ErrTaskTransferConflict)
	}
	if command.Actor.Kind == models.TaskTransferActorCoordinator && command.Actor.CallerTaskID == "" {
		return fmt.Errorf("%w: coordinator caller task is missing", repoerrors.ErrTaskTransferConflict)
	}
	return nil
}

func taskTransferCommandMissingPredicate(command models.TaskTransferCommand) bool {
	return command.TaskID == "" || command.ExpectedSourceWorkspaceID == "" ||
		command.ExpectedSourceWorkflowID == "" || command.ExpectedSourceStepID == "" ||
		command.ExpectedTaskUpdatedAt.IsZero() || command.DestinationWorkspaceID == "" ||
		command.DestinationWorkflowID == "" || command.IdempotencyKey == ""
}

func taskTransferCommandUnsupported(command models.TaskTransferCommand) bool {
	return command.ExpectedSourceWorkspaceID == command.DestinationWorkspaceID ||
		(command.DestinationStepID == "" && command.DestinationStepName == "") ||
		command.PreservationPolicy != models.TaskTransferPreservationPolicyV1 || !command.OwnerPredicateSet
}

func taskTransferRequestDigest(command models.TaskTransferCommand) (string, error) {
	command.Actor = models.TaskTransferActor{}
	command.AuthorizedOwnerID = ""
	command.OwnerPredicateSet = false
	command.DestinationStepName = strings.TrimSpace(command.DestinationStepName)
	encoded, err := json.Marshal(command)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

// ResolveTaskTransferReplayActor returns the actor bound to an exact committed
// request. It never authorizes a new transfer.
func (r *Repository) ResolveTaskTransferReplayActor(
	ctx context.Context,
	command models.TaskTransferCommand,
) (models.TaskTransferActor, bool, error) {
	digest, err := taskTransferRequestDigest(command)
	if err != nil {
		return models.TaskTransferActor{}, false, err
	}
	var persisted persistedTaskTransfer
	err = r.ro.GetContext(ctx, &persisted, r.ro.Rebind(`
		SELECT request_digest, receipt_json, actor_kind, actor_id, actor_session_id
		FROM task_transfer_operations WHERE source_workspace_id = ? AND idempotency_key = ?`),
		command.ExpectedSourceWorkspaceID, command.IdempotencyKey)
	if errors.Is(err, sql.ErrNoRows) {
		return models.TaskTransferActor{}, false, nil
	}
	if err != nil {
		return models.TaskTransferActor{}, false, err
	}
	if persisted.RequestDigest != digest {
		return models.TaskTransferActor{}, false, repoerrors.ErrTaskTransferConflict
	}
	return models.TaskTransferActor{
		Kind: models.TaskTransferActorKind(persisted.ActorKind), ID: persisted.ActorID,
		SessionID: persisted.ActorSessionID,
	}, true, nil
}

// ReplayTaskTransfer returns a committed receipt without consulting mutable
// task, workflow, or lane state. The persisted actor binding remains required.
func (r *Repository) ReplayTaskTransfer(
	ctx context.Context,
	command models.TaskTransferCommand,
) (*models.TaskTransferReceipt, bool, error) {
	digest, err := taskTransferRequestDigest(command)
	if err != nil {
		return nil, false, err
	}
	receipt, found, err := r.replayTaskTransferCommitted(ctx, command, digest)
	if err == nil {
		return receipt, found, nil
	}
	return nil, found, errors.Join(err, r.RecordTaskTransferAttempt(ctx, command, taskTransferResultForError(err)))
}

func (r *Repository) replayTaskTransfer(
	ctx context.Context,
	tx *sqlx.Tx,
	command models.TaskTransferCommand,
	requestDigest string,
) (*models.TaskTransferReceipt, bool, error) {
	var persisted persistedTaskTransfer
	err := tx.GetContext(ctx, &persisted, r.db.Rebind(`
		SELECT request_digest, receipt_json, actor_kind, actor_id, actor_session_id FROM task_transfer_operations
		WHERE source_workspace_id = ? AND idempotency_key = ?`),
		command.ExpectedSourceWorkspaceID, command.IdempotencyKey)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if persisted.RequestDigest != requestDigest {
		return nil, true, fmt.Errorf("%w: idempotency key request differs", repoerrors.ErrTaskTransferConflict)
	}
	if persisted.ActorKind != string(command.Actor.Kind) || persisted.ActorID != command.Actor.ID ||
		persisted.ActorSessionID != command.Actor.SessionID {
		return nil, true, fmt.Errorf("%w: idempotency actor differs", repoerrors.ErrTaskTransferConflict)
	}
	var receipt models.TaskTransferReceipt
	if err := json.Unmarshal([]byte(persisted.ReceiptJSON), &receipt); err != nil {
		return nil, true, fmt.Errorf("decode task transfer receipt: %w", err)
	}
	receipt.IdempotentReplay = true
	return &receipt, true, nil
}

func (r *Repository) replayTaskTransferCommitted(
	ctx context.Context,
	command models.TaskTransferCommand,
	requestDigest string,
) (*models.TaskTransferReceipt, bool, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback() }()
	receipt, found, err := r.replayTaskTransfer(ctx, tx, command, requestDigest)
	if err != nil || !found {
		return nil, found, err
	}
	if err := r.insertTaskTransferAudit(ctx, tx, command, receipt, "idempotent_replay"); err != nil {
		return nil, true, err
	}
	if err := tx.Commit(); err != nil {
		return nil, true, err
	}
	return receipt, true, nil
}

func (r *Repository) applyTaskTransfer(
	ctx context.Context,
	tx *sqlx.Tx,
	command models.TaskTransferCommand,
	requestDigest string,
	projections []transferWorkspaceProjection,
	countTables []transferWorkspaceProjection,
	inventory transferRelationInventory,
) (*models.TaskTransferReceipt, error) {
	placement, err := r.lockAndValidateTaskTransferSource(ctx, tx, command)
	if err != nil {
		return nil, err
	}
	destinationStepID, destinationStepName, err := r.resolveTransferDestination(ctx, tx, command)
	if err != nil {
		return nil, err
	}
	agentProfileMappings, err := r.resolveTransferAgentProfileMappings(ctx, tx, command, inventory)
	if err != nil {
		return nil, err
	}
	if err := r.validateTransferInvariants(ctx, tx, placement, command.TaskID, command.ExpectedSourceStepID,
		destinationStepID, command.DestinationWorkspaceID, command.DestinationWorkflowID,
		command.AuthorizedOwnerID, inventory); err != nil {
		return nil, err
	}
	queuedStepID, pendingStepIDs, err := r.resolveTransferPendingState(ctx, tx, command, placement, inventory)
	if err != nil {
		return nil, err
	}

	sessions, err := r.taskTransferSessions(ctx, tx, command.TaskID)
	if err != nil {
		return nil, err
	}
	_, beforeDigest, err := r.taskTransferPreservation(ctx, tx, command.TaskID, sessions, countTables)
	if err != nil {
		return nil, err
	}
	transferredAt := r.nowUTC()
	transitionID, err := r.updateTaskTransferPlacement(ctx, tx, command, destinationStepID, queuedStepID,
		pendingStepIDs, agentProfileMappings, transferredAt, projections, inventory)
	if err != nil {
		return nil, err
	}
	afterSessions, err := r.taskTransferSessions(ctx, tx, command.TaskID)
	if err != nil {
		return nil, err
	}
	afterCounts, afterDigest, err := r.taskTransferPreservation(ctx, tx, command.TaskID, afterSessions, countTables)
	if err != nil {
		return nil, err
	}
	if beforeDigest != afterDigest {
		return nil, fmt.Errorf("%w: preservation census changed", repoerrors.ErrTaskTransferConflict)
	}
	var transitionCount int
	if err := tx.GetContext(ctx, &transitionCount,
		r.db.Rebind(`SELECT COUNT(*) FROM task_step_transitions WHERE task_id = ?`), command.TaskID); err != nil {
		return nil, err
	}
	afterCounts["task_step_transitions"] = transitionCount

	receipt := &models.TaskTransferReceipt{
		OperationID: uuid.NewString(), TaskID: command.TaskID,
		SourceWorkspaceID: command.ExpectedSourceWorkspaceID, SourceWorkflowID: command.ExpectedSourceWorkflowID,
		SourceStepID: command.ExpectedSourceStepID, DestinationWorkspaceID: command.DestinationWorkspaceID,
		DestinationWorkflowID: command.DestinationWorkflowID, DestinationStepID: destinationStepID,
		DestinationStepName: destinationStepName, TaskGeneration: placement.UpdatedAt, StepTransitionID: transitionID,
		Sessions:           afterSessions,
		PreservationCounts: afterCounts, PreservationDigest: afterDigest, IdempotencyKey: command.IdempotencyKey,
		PreservationPolicy: command.PreservationPolicy, TransferredAt: transferredAt,
	}
	if err := r.persistTaskTransfer(ctx, tx, command, requestDigest, receipt); err != nil {
		return nil, err
	}
	return receipt, nil
}

func (r *Repository) lockAndValidateTaskTransferSource(
	ctx context.Context,
	tx *sqlx.Tx,
	command models.TaskTransferCommand,
) (*transferTaskPlacement, error) {
	placement, err := r.lockTransferTask(ctx, tx, command.TaskID)
	if err != nil || !taskTransferSourceMatches(placement, command) {
		return nil, taskTransferConflict(err, "source predicate changed")
	}
	if err := r.validateTransferCoordinatorActor(ctx, tx, command); err != nil {
		return nil, err
	}
	return placement, nil
}

func (r *Repository) lockTransferTask(ctx context.Context, tx *sqlx.Tx, taskID string) (*transferTaskPlacement, error) {
	query := `SELECT workspace_id, workflow_id, workflow_step_id, COALESCE(queued_for_step_id, '') AS queued_for_step_id,
		COALESCE(project_id, '') AS project_id, wip_admitted, updated_at FROM tasks WHERE id = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += postgresForUpdateClause
	}
	var placement transferTaskPlacement
	if err := tx.GetContext(ctx, &placement, r.db.Rebind(query), taskID); err != nil {
		return nil, err
	}
	return &placement, nil
}

func taskTransferSourceMatches(placement *transferTaskPlacement, command models.TaskTransferCommand) bool {
	return placement != nil && placement.WorkspaceID == command.ExpectedSourceWorkspaceID &&
		placement.WorkflowID == command.ExpectedSourceWorkflowID && placement.StepID == command.ExpectedSourceStepID &&
		placement.UpdatedAt.Equal(command.ExpectedTaskUpdatedAt)
}

func taskTransferConflict(err error, reason string) error {
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	return fmt.Errorf("%w: %s", repoerrors.ErrTaskTransferConflict, reason)
}

func (r *Repository) resolveTransferDestination(
	ctx context.Context,
	tx *sqlx.Tx,
	command models.TaskTransferCommand,
) (string, string, error) {
	var workflowWorkspaceID string
	err := tx.GetContext(ctx, &workflowWorkspaceID,
		r.db.Rebind(`SELECT workspace_id FROM workflows WHERE id = ?`), command.DestinationWorkflowID)
	if err != nil || workflowWorkspaceID != command.DestinationWorkspaceID {
		return "", "", taskTransferConflict(err, "destination unavailable")
	}
	if command.DestinationStepID != "" {
		var step struct {
			ID   string `db:"id"`
			Name string `db:"name"`
		}
		err := tx.GetContext(ctx, &step, r.db.Rebind(
			`SELECT id, name FROM workflow_steps WHERE id = ? AND workflow_id = ?`),
			command.DestinationStepID, command.DestinationWorkflowID)
		if err != nil || (command.DestinationStepName != "" && step.Name != command.DestinationStepName) {
			return "", "", taskTransferConflict(err, "destination lane unavailable")
		}
		return step.ID, step.Name, nil
	}
	var steps []struct {
		ID   string `db:"id"`
		Name string `db:"name"`
	}
	if err := tx.SelectContext(ctx, &steps, r.db.Rebind(
		`SELECT id, name FROM workflow_steps WHERE workflow_id = ? AND name = ? ORDER BY id`),
		command.DestinationWorkflowID, command.DestinationStepName); err != nil {
		return "", "", err
	}
	if len(steps) != 1 {
		return "", "", fmt.Errorf("%w: destination lane mapping is not unique", repoerrors.ErrTaskTransferConflict)
	}
	return steps[0].ID, steps[0].Name, nil
}

func (r *Repository) validateTransferInvariants(
	ctx context.Context,
	tx *sqlx.Tx,
	placement *transferTaskPlacement,
	taskID, sourceStepID, destinationStepID, destinationWorkspaceID, destinationWorkflowID, authorizedOwnerID string,
	inventory transferRelationInventory,
) error {
	if err := r.validateTransferWorkspaceBoundary(ctx, tx, destinationWorkspaceID, destinationWorkflowID,
		placement, authorizedOwnerID); err != nil {
		return err
	}
	if err := r.validateTransferLane(ctx, tx, placement, taskID, sourceStepID, destinationStepID); err != nil {
		return err
	}
	if placement.ProjectID != "" {
		return fmt.Errorf("%w: destination project mapping is required", repoerrors.ErrTaskTransferConflict)
	}
	if err := r.validateTransferCleanup(ctx, tx, taskID); err != nil {
		return err
	}
	return r.validateTransferWorkspaceRelations(ctx, tx, taskID, destinationWorkspaceID, inventory)
}

func (r *Repository) validateTransferWorkspaceBoundary(
	ctx context.Context,
	tx *sqlx.Tx,
	destinationWorkspaceID, destinationWorkflowID string,
	placement *transferTaskPlacement,
	authorizedOwnerID string,
) error {
	var compatible int
	if err := tx.GetContext(ctx, &compatible, r.db.Rebind(`
		SELECT COUNT(*) FROM workspaces source
		JOIN workspaces destination ON destination.id = ?
		JOIN workflows source_workflow ON source_workflow.id = ? AND source_workflow.workspace_id = source.id
		JOIN workflows destination_workflow ON destination_workflow.id = ?
			AND destination_workflow.workspace_id = destination.id
		WHERE source.id = ? AND source.owner_id = ? AND destination.owner_id = ?`),
		destinationWorkspaceID, placement.WorkflowID, destinationWorkflowID, placement.WorkspaceID,
		authorizedOwnerID, authorizedOwnerID); err != nil {
		return err
	}
	if compatible != 1 {
		return fmt.Errorf("%w: workspace boundary changed", repoerrors.ErrTaskTransferConflict)
	}
	return nil
}

func (r *Repository) validateTransferLane(
	ctx context.Context,
	tx *sqlx.Tx,
	placement *transferTaskPlacement,
	taskID, sourceStepID, destinationStepID string,
) error {
	if dialect.IsPostgres(r.db.DriverName()) {
		var lockedStepID string
		query := `SELECT id FROM workflow_steps WHERE id = ?` + postgresForUpdateClause
		if err := tx.GetContext(ctx, &lockedStepID, r.db.Rebind(query), destinationStepID); err != nil {
			return taskTransferConflict(err, "destination lane unavailable")
		}
	}
	equivalent, destination, err := r.transferStepsEquivalent(ctx, tx, sourceStepID, destinationStepID)
	if err != nil {
		return err
	}
	if !equivalent {
		return fmt.Errorf("%w: destination lane changes task semantics", repoerrors.ErrTaskTransferConflict)
	}
	if placement.WIPAdmitted != 0 && destination.WIPLimit > 0 {
		var occupants int
		if err := tx.GetContext(ctx, &occupants, r.db.Rebind(`
			SELECT COUNT(*) FROM tasks WHERE workflow_step_id = ? AND id <> ?
				AND archived_at IS NULL AND is_ephemeral = 0 AND wip_admitted = 1
				AND (queued_for_step_id = '' OR queued_for_step_id IS NULL)`), destinationStepID, taskID); err != nil {
			return err
		}
		if occupants >= destination.WIPLimit {
			return fmt.Errorf("%w: destination lane is at capacity", repoerrors.ErrTaskTransferConflict)
		}
	}
	return nil
}

func (r *Repository) validateTransferCleanup(ctx context.Context, tx *sqlx.Tx, taskID string) error {
	var cleanupCount int
	if err := tx.GetContext(ctx, &cleanupCount, r.db.Rebind(`
		SELECT COUNT(*) FROM task_resource_cleanup_jobs
		WHERE task_id = ? AND state NOT IN ('completed', 'failed')`), taskID); err != nil {
		return err
	}
	if cleanupCount != 0 {
		return fmt.Errorf("%w: incompatible lifecycle mutation is active", repoerrors.ErrTaskTransferConflict)
	}
	return nil
}

func (r *Repository) validateTransferWorkspaceRelations(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID, destinationWorkspaceID string,
	inventory transferRelationInventory,
) error {
	if inventory.labels {
		var incompatibleLabelCount int
		if err := tx.GetContext(ctx, &incompatibleLabelCount, r.db.Rebind(`
			SELECT COUNT(*) FROM office_task_labels task_labels
			JOIN office_labels labels ON labels.id = task_labels.label_id
			WHERE task_labels.task_id = ? AND labels.workspace_id <> ?`), taskID, destinationWorkspaceID); err != nil {
			return err
		}
		if incompatibleLabelCount != 0 {
			return fmt.Errorf("%w: destination label mapping is required", repoerrors.ErrTaskTransferConflict)
		}
	}
	if inventory.groups {
		var groupCount int
		if err := tx.GetContext(ctx, &groupCount, r.db.Rebind(`
			SELECT (SELECT COUNT(*) FROM task_workspace_group_members WHERE task_id = ?) +
				(SELECT COUNT(*) FROM task_workspace_groups WHERE owner_task_id = ?)`), taskID, taskID); err != nil {
			return err
		}
		if groupCount != 0 {
			return fmt.Errorf("%w: workspace group mapping is incompatible", repoerrors.ErrTaskTransferConflict)
		}
	}
	if inventory.treeHolds {
		var holdCount int
		if err := tx.GetContext(ctx, &holdCount, r.db.Rebind(`
			SELECT (SELECT COUNT(*) FROM office_task_tree_holds WHERE root_task_id = ?) +
				(SELECT COUNT(*) FROM office_task_tree_hold_members WHERE task_id = ?)`), taskID, taskID); err != nil {
			return err
		}
		if holdCount != 0 {
			return fmt.Errorf("%w: destination tree-hold mapping is required", repoerrors.ErrTaskTransferConflict)
		}
	}
	return nil
}

func (r *Repository) transferStepsEquivalent(
	ctx context.Context, tx *sqlx.Tx, sourceStepID, destinationStepID string,
) (bool, transferStepSemantics, error) {
	read := func(stepID string) (transferStepSemantics, error) {
		var step transferStepSemantics
		err := tx.GetContext(ctx, &step, r.db.Rebind(`
			SELECT s.name, COALESCE(s.prompt, '') AS prompt, COALESCE(w.prompt, '') AS workflow_prompt,
				COALESCE(s.events, '') AS events,
				COALESCE(NULLIF(s.agent_profile_id, ''), w.agent_profile_id, '') AS effective_agent_profile_id,
				COALESCE(s.stage_type, 'custom') AS stage_type,
				COALESCE((SELECT p.name FROM workflow_steps p WHERE p.id = s.pull_from_step_id), '') AS pull_from_name,
				COALESCE(s.allow_manual_move, 1) AS allow_manual_move, COALESCE(s.is_start_step, 0) AS is_start_step,
				COALESCE(s.auto_archive_after_hours, 0) AS auto_archive_after_hours,
				COALESCE(s.auto_advance_requires_signal, 0) AS auto_advance_requires_signal,
				COALESCE(s.cancel_triggers_turn_complete, 0) AS cancel_triggers_turn_complete,
				COALESCE(s.wip_limit, 0) AS wip_limit
			FROM workflow_steps s JOIN workflows w ON w.id = s.workflow_id WHERE s.id = ?`), stepID)
		if err != nil {
			return step, taskTransferConflict(err, "lane unavailable")
		}
		return step, nil
	}
	source, err := read(sourceStepID)
	if err != nil {
		return false, transferStepSemantics{}, err
	}
	destination, err := read(destinationStepID)
	if err != nil {
		return false, transferStepSemantics{}, err
	}
	if source != destination {
		return false, destination, nil
	}
	participantsEqual, err := r.transferStepParticipantsEquivalent(ctx, tx, sourceStepID, destinationStepID)
	return participantsEqual, destination, err
}

func (r *Repository) transferStepParticipantsEquivalent(
	ctx context.Context, tx *sqlx.Tx, sourceStepID, destinationStepID string,
) (bool, error) {
	read := func(stepID string) ([]transferStepParticipantSemantics, error) {
		var rows []transferStepParticipantSemantics
		err := tx.SelectContext(ctx, &rows, r.db.Rebind(`
			SELECT role, agent_profile_id, decision_required, position, COALESCE(provenance, 'manual') AS provenance
			FROM workflow_step_participants WHERE step_id = ? AND task_id = ''
			ORDER BY role, position, agent_profile_id, decision_required, provenance`), stepID)
		return rows, err
	}
	source, err := read(sourceStepID)
	if err != nil {
		return false, err
	}
	destination, err := read(destinationStepID)
	if err != nil || len(source) != len(destination) {
		return false, err
	}
	for index := range source {
		if source[index] != destination[index] {
			return false, nil
		}
	}
	return true, nil
}

func (r *Repository) taskTransferSessions(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID string,
) ([]models.TaskTransferSessionReceipt, error) {
	var rows []struct {
		ID                string `db:"id"`
		State             string `db:"state"`
		IsPrimary         int    `db:"is_primary"`
		TaskEnvironmentID string `db:"task_environment_id"`
		TurnID            string `db:"turn_id"`
	}
	err := tx.SelectContext(ctx, &rows, r.db.Rebind(`
		SELECT s.id, s.state, s.is_primary, COALESCE(s.task_environment_id, '') AS task_environment_id,
			COALESCE((SELECT t.id FROM task_session_turns t
				WHERE t.task_session_id = s.id AND t.completed_at IS NULL
				ORDER BY t.started_at DESC, t.id DESC LIMIT 1), '') AS turn_id
		FROM task_sessions s WHERE s.task_id = ? ORDER BY s.id`), taskID)
	if err != nil {
		return nil, err
	}
	receipts := make([]models.TaskTransferSessionReceipt, 0, len(rows))
	for _, row := range rows {
		receipts = append(receipts, models.TaskTransferSessionReceipt{
			ID: row.ID, State: models.TaskSessionState(row.State), IsPrimary: row.IsPrimary != 0,
			TaskEnvironmentID: row.TaskEnvironmentID, TurnID: row.TurnID,
		})
	}
	return receipts, nil
}

func (r *Repository) taskTransferPreservation(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID string,
	sessions []models.TaskTransferSessionReceipt,
	tables []transferWorkspaceProjection,
) (map[string]int, string, error) {
	counts := make(map[string]int, len(tables)+1)
	identities := make(map[string][]string, len(tables))
	for _, table := range tables {
		query := fmt.Sprintf(`SELECT CAST(%s AS TEXT) FROM %s WHERE %s = ? ORDER BY CAST(%s AS TEXT)`,
			table.identityColumn, table.table, table.taskColumn, table.identityColumn)
		var ids []string
		if err := tx.SelectContext(ctx, &ids, r.db.Rebind(query), taskID); err != nil {
			return nil, "", err
		}
		key := table.receiptKey
		if key == "" {
			key = table.table
		}
		counts[key] = len(ids)
		identities[key] = ids
	}
	var childCount int
	if err := tx.GetContext(ctx, &childCount,
		r.db.Rebind(`SELECT COUNT(*) FROM tasks WHERE parent_id = ?`), taskID); err != nil {
		return nil, "", err
	}
	counts["task_children"] = childCount
	payload := struct {
		TaskID       string                              `json:"task_id"`
		Sessions     []models.TaskTransferSessionReceipt `json:"sessions"`
		Counts       map[string]int                      `json:"counts"`
		Associations map[string][]string                 `json:"associations"`
	}{TaskID: taskID, Sessions: sessions, Counts: counts, Associations: identities}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, "", err
	}
	digest := sha256.Sum256(encoded)
	return counts, hex.EncodeToString(digest[:]), nil
}

func (r *Repository) updateTaskTransferPlacement(
	ctx context.Context,
	tx *sqlx.Tx,
	command models.TaskTransferCommand,
	destinationStepID string,
	queuedStepID string,
	pendingStepIDs, agentProfileMappings map[string]string,
	transferredAt time.Time,
	projections []transferWorkspaceProjection,
	inventory transferRelationInventory,
) (int64, error) {
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE tasks SET workspace_id = ?, workflow_id = ?, workflow_step_id = ?, queued_for_step_id = ?
		WHERE id = ? AND workspace_id = ? AND workflow_id = ? AND workflow_step_id = ? AND updated_at = ?`),
		command.DestinationWorkspaceID, command.DestinationWorkflowID, destinationStepID, queuedStepID,
		command.TaskID, command.ExpectedSourceWorkspaceID, command.ExpectedSourceWorkflowID,
		command.ExpectedSourceStepID, command.ExpectedTaskUpdatedAt)
	if err != nil {
		return 0, err
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return 0, fmt.Errorf("%w: source predicate changed", repoerrors.ErrTaskTransferConflict)
	}
	attribution := steptelemetry.Attribution{
		Trigger: steptelemetry.TriggerTaskTransfer, ActorID: command.Actor.ID, SessionID: command.Actor.SessionID,
	}
	if command.Actor.Kind == models.TaskTransferActorHuman {
		attribution.ActorKind = steptelemetry.ActorHuman
	} else {
		attribution.ActorKind = steptelemetry.ActorAgent
	}
	transitionCtx := steptelemetry.WithAttribution(ctx, attribution)
	transitionID, err := r.recordStepTransition(transitionCtx, tx, stepTransitionInput{
		taskID: command.TaskID, fromWorkflowID: command.ExpectedSourceWorkflowID,
		fromWorkflowStepID: command.ExpectedSourceStepID, toWorkflowID: command.DestinationWorkflowID,
		toWorkflowStepID: destinationStepID, occurredAt: transferredAt,
	})
	if err != nil {
		return 0, err
	}
	if err := r.updateTaskTransferRelations(ctx, tx, command, pendingStepIDs, agentProfileMappings,
		projections, inventory); err != nil {
		return 0, err
	}
	return transitionID, nil
}
