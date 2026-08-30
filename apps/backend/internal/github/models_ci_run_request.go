package github

import (
	"errors"
	"time"
)

type CIRunEvidenceKind string

const (
	CIRunEvidencePRHead       CIRunEvidenceKind = "pr_head"
	CIRunEvidenceCurrentMerge CIRunEvidenceKind = "current_merge"
)

type CIRunRequestStatus string

const (
	CIRunRequestPending     CIRunRequestStatus = "pending"
	CIRunRequestReconciling CIRunRequestStatus = "reconciling"
	CIRunRequestSucceeded   CIRunRequestStatus = "succeeded"
	CIRunRequestFailed      CIRunRequestStatus = "failed"
)

type CIRunOperation string

const (
	CIRunOperationRerunFailedJobs  CIRunOperation = "rerun_failed_jobs"
	CIRunOperationWorkflowDispatch CIRunOperation = "workflow_dispatch"
)

var ErrCIRunSemanticConflict = errors.New("CI run source attempt already has a logical request")
var ErrCIRunIdempotencyConflict = errors.New("CI run idempotency key was reused for a different request")

type CIRunGrant struct {
	ID              string     `json:"id" db:"id"`
	WorkspaceID     string     `json:"workspace_id" db:"workspace_id"`
	ActorTaskID     string     `json:"actor_task_id" db:"actor_task_id"`
	TargetTaskID    string     `json:"target_task_id" db:"target_task_id"`
	WorkflowID      string     `json:"workflow_id" db:"workflow_id"`
	WorkflowStepID  string     `json:"workflow_step_id" db:"workflow_step_id"`
	RepositoryID    string     `json:"repository_id" db:"repository_id"`
	CreatedByUserID string     `json:"created_by_user_id" db:"created_by_user_id"`
	RevokedAt       *time.Time `json:"revoked_at,omitempty" db:"revoked_at"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at" db:"updated_at"`
}

type CIRunRequest struct {
	ID                    string             `json:"id" db:"id"`
	GrantID               string             `json:"grant_id" db:"grant_id"`
	WorkspaceID           string             `json:"workspace_id" db:"workspace_id"`
	ActorTaskID           string             `json:"actor_task_id" db:"actor_task_id"`
	ActorSessionID        string             `json:"actor_session_id" db:"actor_session_id"`
	TargetTaskID          string             `json:"target_task_id" db:"target_task_id"`
	WorkflowID            string             `json:"workflow_id" db:"workflow_id"`
	WorkflowStepID        string             `json:"workflow_step_id" db:"workflow_step_id"`
	RepositoryID          string             `json:"repository_id" db:"repository_id"`
	PRNumber              int                `json:"pr_number" db:"pr_number"`
	ExpectedHeadSHA       string             `json:"expected_head_sha" db:"expected_head_sha"`
	SourceRunID           int64              `json:"source_run_id" db:"source_run_id"`
	ExpectedSourceAttempt int                `json:"expected_source_attempt" db:"expected_source_attempt"`
	EvidenceKind          CIRunEvidenceKind  `json:"evidence_kind" db:"evidence_kind"`
	IdempotencyHash       string             `json:"-" db:"idempotency_hash"`
	Status                CIRunRequestStatus `json:"status" db:"status"`
	Operation             CIRunOperation     `json:"operation,omitempty" db:"operation"`
	ProviderCallStartedAt *time.Time         `json:"provider_call_started_at,omitempty" db:"provider_call_started_at"`
	ProviderRunID         int64              `json:"provider_run_id,omitempty" db:"provider_run_id"`
	ProviderWorkflowID    int64              `json:"provider_workflow_id,omitempty" db:"provider_workflow_id"`
	ProviderWorkflowName  string             `json:"provider_workflow_name,omitempty" db:"provider_workflow_name"`
	ProviderWorkflowPath  string             `json:"provider_workflow_path,omitempty" db:"provider_workflow_path"`
	ProviderAttempt       int                `json:"provider_attempt,omitempty" db:"provider_attempt"`
	ProviderHeadRepo      string             `json:"provider_head_repo,omitempty" db:"provider_head_repo"`
	ProviderHeadRef       string             `json:"provider_head_ref,omitempty" db:"provider_head_ref"`
	ProviderHeadSHA       string             `json:"provider_head_sha,omitempty" db:"provider_head_sha"`
	FailureClass          string             `json:"failure_class,omitempty" db:"failure_class"`
	CreatedAt             time.Time          `json:"created_at" db:"created_at"`
	UpdatedAt             time.Time          `json:"updated_at" db:"updated_at"`
}

type CIRunAuditEvent struct {
	ID           string    `json:"id" db:"id"`
	RequestID    string    `json:"request_id" db:"request_id"`
	EventType    string    `json:"event_type" db:"event_type"`
	FailureClass string    `json:"failure_class,omitempty" db:"failure_class"`
	DetailsJSON  string    `json:"details" db:"details_json"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

type CIRunReceipt struct {
	RequestID      string             `json:"request_id"`
	TaskID         string             `json:"task_id"`
	RunID          int64              `json:"run_id"`
	WorkflowID     int64              `json:"workflow_id"`
	WorkflowName   string             `json:"workflow_name,omitempty"`
	WorkflowPath   string             `json:"workflow_path,omitempty"`
	HeadRepository string             `json:"head_repository"`
	HeadRef        string             `json:"head_ref"`
	HeadSHA        string             `json:"head_sha"`
	Attempt        int                `json:"attempt"`
	Operation      CIRunOperation     `json:"operation"`
	EvidenceKind   CIRunEvidenceKind  `json:"evidence_kind"`
	Status         CIRunRequestStatus `json:"status"`
	FailureClass   string             `json:"failure_class,omitempty"`
}
