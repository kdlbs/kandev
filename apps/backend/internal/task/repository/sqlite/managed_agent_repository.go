package sqlite

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

const managedAgentBindingColumns = `id, session_id, task_id, workspace_id, user_id, execution_id,
	provider_kind, executor_id, executor_profile_id, credential_ref, remote_agent_id, lifecycle,
	repository_id, repository_url, starting_ref, model, callback_url, auto_create_pr,
	revision, dispatch_generation, dispatch_owner, dispatch_lease_until, created_at, updated_at`

const managedAgentOperationColumns = `id, binding_id, prompt_turn_id, operation_kind, request_digest,
	request_snapshot, submission_state, remote_run_id, pre_submit_run_id, dispatch_generation,
	revision, created_at, dispatch_started_at, accepted_at, settled_at, updated_at, sanitized_error, result_snapshot,
	completion_pending`

type managedAgentScanner interface {
	Scan(dest ...any) error
}

func scanManagedAgentBinding(row managedAgentScanner) (*models.ManagedAgentBinding, error) {
	binding := &models.ManagedAgentBinding{}
	var (
		autoCreatePR int
		leaseUntil   sql.NullTime
	)
	if err := row.Scan(
		&binding.ID, &binding.SessionID, &binding.TaskID, &binding.WorkspaceID, &binding.UserID,
		&binding.ExecutionID, &binding.ProviderKind, &binding.ExecutorID, &binding.ExecutorProfileID,
		&binding.CredentialRef, &binding.RemoteAgentID, &binding.Lifecycle,
		&binding.Launch.RepositoryID, &binding.Launch.RepositoryURL, &binding.Launch.StartingRef,
		&binding.Launch.Model, &binding.Launch.CallbackURL, &autoCreatePR, &binding.Revision,
		&binding.DispatchGeneration, &binding.DispatchOwner, &leaseUntil, &binding.CreatedAt, &binding.UpdatedAt,
	); err != nil {
		return nil, err
	}
	binding.Launch.AutoCreatePR = autoCreatePR != 0
	if leaseUntil.Valid {
		binding.DispatchLeaseUntil = &leaseUntil.Time
	}
	return binding, nil
}

func scanManagedAgentOperation(row managedAgentScanner) (*models.ManagedAgentOperation, error) {
	operation := &models.ManagedAgentOperation{}
	var (
		requestSnapshot   string
		resultSnapshot    string
		completionPending int
		dispatchStarted   sql.NullTime
		acceptedAt        sql.NullTime
		settledAt         sql.NullTime
	)
	if err := row.Scan(
		&operation.ID, &operation.BindingID, &operation.PromptTurnID, &operation.Kind,
		&operation.RequestDigest, &requestSnapshot, &operation.State, &operation.RemoteRunID,
		&operation.PreSubmitRunID, &operation.DispatchGeneration, &operation.Revision, &operation.CreatedAt,
		&dispatchStarted, &acceptedAt, &settledAt, &operation.UpdatedAt, &operation.SanitizedError, &resultSnapshot,
		&completionPending,
	); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(requestSnapshot), &operation.RequestSnapshot); err != nil {
		return nil, fmt.Errorf("decode managed agent request snapshot: %w", err)
	}
	if err := json.Unmarshal([]byte(resultSnapshot), &operation.ResultSnapshot); err != nil {
		return nil, fmt.Errorf("decode managed agent result snapshot: %w", err)
	}
	operation.DispatchStartedAt = managedAgentNullableTime(dispatchStarted)
	operation.AcceptedAt = managedAgentNullableTime(acceptedAt)
	operation.SettledAt = managedAgentNullableTime(settledAt)
	operation.CompletionPending = completionPending != 0
	return operation, nil
}

func managedAgentNullableTime(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	return &value.Time
}

func managedAgentBool(value bool) int {
	if value {
		return 1
	}
	return 0
}

func managedAgentLockSuffix(driver string) string {
	if dialect.IsPostgres(driver) {
		return forUpdateClause
	}
	return ""
}

func validateManagedAgentBinding(binding *models.ManagedAgentBinding) error {
	if binding == nil {
		return fmt.Errorf("managed agent binding is incomplete")
	}
	for _, value := range []string{
		binding.ID, binding.SessionID, binding.TaskID, binding.WorkspaceID, binding.UserID,
		binding.ExecutionID, binding.ProviderKind, binding.ExecutorID, binding.ExecutorProfileID,
		binding.CredentialRef, binding.RemoteAgentID, string(binding.Lifecycle),
		binding.Launch.RepositoryID, binding.Launch.RepositoryURL, binding.Launch.StartingRef,
		binding.Launch.Model, binding.Launch.CallbackURL,
	} {
		if value == "" {
			return fmt.Errorf("managed agent binding is incomplete")
		}
	}
	return nil
}

func validateManagedAgentOperation(operation *models.ManagedAgentOperation) error {
	if operation == nil || operation.ID == "" || operation.BindingID == "" || operation.PromptTurnID == "" ||
		operation.Kind == "" || operation.RequestDigest == "" || operation.RequestSnapshot.Prompt == "" ||
		operation.RequestSnapshot.Model == "" || operation.RequestSnapshot.RepositoryURL == "" ||
		operation.RequestSnapshot.StartingRef == "" || operation.RequestSnapshot.CallbackURL == "" {
		return fmt.Errorf("managed agent operation is incomplete")
	}
	return nil
}

func managedAgentOperationActive(state models.ManagedAgentSubmissionState) bool {
	return models.ManagedAgentOperationActive(state)
}

var managedAgentTransitions = map[models.ManagedAgentSubmissionState][]models.ManagedAgentSubmissionState{
	models.ManagedAgentSubmissionReserved: {
		models.ManagedAgentSubmissionSubmitting, models.ManagedAgentSubmissionAccepted,
		models.ManagedAgentSubmissionUnknown, models.ManagedAgentSubmissionRejected,
	},
	models.ManagedAgentSubmissionSubmitting: {
		models.ManagedAgentSubmissionAccepted, models.ManagedAgentSubmissionUnknown, models.ManagedAgentSubmissionRetryAcked,
		models.ManagedAgentSubmissionRejected,
	},
	models.ManagedAgentSubmissionAccepted: {
		models.ManagedAgentSubmissionCancelling, models.ManagedAgentSubmissionUnknown, models.ManagedAgentSubmissionSucceeded,
		models.ManagedAgentSubmissionFailed, models.ManagedAgentSubmissionCancelled,
	},
	models.ManagedAgentSubmissionCancelling: {
		models.ManagedAgentSubmissionUnknown, models.ManagedAgentSubmissionSucceeded,
		models.ManagedAgentSubmissionFailed, models.ManagedAgentSubmissionCancelled,
	},
	models.ManagedAgentSubmissionUnknown: {
		models.ManagedAgentSubmissionAccepted, models.ManagedAgentSubmissionSucceeded,
		models.ManagedAgentSubmissionFailed, models.ManagedAgentSubmissionCancelled,
		models.ManagedAgentSubmissionRejected, models.ManagedAgentSubmissionRetryAcked,
	},
}

func validManagedAgentTransition(from, to models.ManagedAgentSubmissionState) bool {
	if from == to {
		return true
	}
	for _, allowed := range managedAgentTransitions[from] {
		if to == allowed {
			return true
		}
	}
	return false
}

func managedAgentUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" && strings.HasPrefix(pgErr.ConstraintName, "uniq_managed_agent_")
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed: managed_agent_")
}

func sameManagedAgentLaunch(a, b *models.ManagedAgentBinding) bool {
	return a.SessionID == b.SessionID && a.TaskID == b.TaskID && a.WorkspaceID == b.WorkspaceID &&
		a.UserID == b.UserID && a.ExecutionID == b.ExecutionID && a.ProviderKind == b.ProviderKind &&
		a.ExecutorID == b.ExecutorID && a.ExecutorProfileID == b.ExecutorProfileID &&
		a.CredentialRef == b.CredentialRef && a.RemoteAgentID == b.RemoteAgentID &&
		a.Launch == b.Launch
}
