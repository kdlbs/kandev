package models

import (
	"context"
	"time"
)

const (
	TaskCompletionEvidenceTaskRevision = "task_revision"
	TaskCompletionEvidenceExecution    = "execution"
	TaskCompletionEvidenceArtifact     = "artifact_revision"
	TaskCompletionEvidenceGitHubPRHead = "github_pr_head"
)

// TaskCompletionEvidenceSubject identifies a typed, revisioned source used to
// verify one task completion criterion. ID is scoped to the owning task.
type TaskCompletionEvidenceSubject struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Revision string `json:"revision,omitempty"`
}

// TaskCompletionEvidence is the current attributed verification for a
// criterion. The subject revision is immutable evidence identity; mutable
// task and pull-request subjects are also checked against their live row at
// the final completion commit.
type TaskCompletionEvidence struct {
	Subject   TaskCompletionEvidenceSubject `json:"subject"`
	Summary   string                        `json:"summary,omitempty"`
	Reference string                        `json:"reference,omitempty"`
}

// TaskCompletionCriterion is one required item in a revisioned task gate.
// CriterionRevision changes only when this criterion's meaning or required
// evidence subject changes, so unrelated edits do not invalidate its proof.
type TaskCompletionCriterion struct {
	ID                string                        `json:"id"`
	Description       string                        `json:"description"`
	EvidenceSubject   TaskCompletionEvidenceSubject `json:"evidence_subject"`
	CriterionRevision int64                         `json:"criterion_revision"`
	VerifiedRevision  int64                         `json:"verified_revision,omitempty"`
	Evidence          *TaskCompletionEvidence       `json:"evidence,omitempty"`
	VerifierKind      string                        `json:"verifier_kind,omitempty"`
	VerifierID        string                        `json:"verifier_id,omitempty"`
	VerifiedAt        *time.Time                    `json:"verified_at,omitempty"`
}

// TaskCompletionBlocker is a bounded reason that one required criterion
// currently prevents completion.
type TaskCompletionBlocker struct {
	CriterionID string `json:"criterion_id"`
	Reason      string `json:"reason"`
}

// TaskCompletionGateSnapshot is the canonical task-owned completion view.
type TaskCompletionGateSnapshot struct {
	TaskID      string                    `json:"task_id"`
	WorkspaceID string                    `json:"workspace_id"`
	Revision    int64                     `json:"revision"`
	Criteria    []TaskCompletionCriterion `json:"criteria"`
	Blockers    []TaskCompletionBlocker   `json:"blockers,omitempty"`
	Blocked     bool                      `json:"blocked"`
}

// TaskCompletionGateHistory is an immutable criteria, evidence, or override
// audit event.
type TaskCompletionGateHistory struct {
	ID          string    `json:"id"`
	TaskID      string    `json:"task_id"`
	WorkspaceID string    `json:"workspace_id"`
	Revision    int64     `json:"revision"`
	Action      string    `json:"action"`
	ActorKind   string    `json:"actor_kind"`
	ActorID     string    `json:"actor_id"`
	Reason      string    `json:"reason,omitempty"`
	Details     string    `json:"details,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// TaskCompletionCriteriaChange is the repository command for replacing the
// current criterion set under an observed set revision.
type TaskCompletionCriteriaChange struct {
	TaskID                      string
	WorkspaceID                 string
	ExpectedTaskResourceVersion string
	ExpectedRevision            int64
	OperationID                 string
	PayloadDigest               string
	ClaimFence                  TaskManagementClaimFence
	Criteria                    []TaskCompletionCriterion
	ActorKind                   string
	ActorID                     string
	HumanConfirmationRevision   int64
	HumanConfirmationReason     string
}

// TaskCompletionEvidenceChange verifies one criterion under both the set and
// criterion revisions observed by the caller.
type TaskCompletionEvidenceChange struct {
	TaskID                      string
	WorkspaceID                 string
	ExpectedTaskResourceVersion string
	ExpectedRevision            int64
	OperationID                 string
	PayloadDigest               string
	ClaimFence                  TaskManagementClaimFence
	CriterionID                 string
	Evidence                    TaskCompletionEvidence
	ActorKind                   string
	ActorID                     string
}

// TaskCompletionMoveOverride authorizes one completion transition. The
// repository checks every observed identity and revision again in the task
// transaction and records the override in the same commit.
type TaskCompletionMoveOverride struct {
	TaskID           string
	WorkspaceID      string
	ExpectedRevision int64
	SourceWorkflowID string
	SourceStepID     string
	TargetWorkflowID string
	TargetStepID     string
	ActorID          string
	Reason           string
}

type completionMoveOverrideContextKey struct{}

// WithTaskCompletionMoveOverride passes a native, user-confirmed override to
// the final task write. Only the task service constructs this value from an
// authenticated human request; plugin task commands do not expose it.
func WithTaskCompletionMoveOverride(ctx context.Context, override TaskCompletionMoveOverride) context.Context {
	return context.WithValue(ctx, completionMoveOverrideContextKey{}, override)
}

// TaskCompletionMoveOverrideFromContext reads the one-move override prepared
// by the native task service.
func TaskCompletionMoveOverrideFromContext(ctx context.Context) (TaskCompletionMoveOverride, bool) {
	value, ok := ctx.Value(completionMoveOverrideContextKey{}).(TaskCompletionMoveOverride)
	return value, ok
}
