package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// @covers AC-TASKS-COORDINATION-002.1, AC-TASKS-COORDINATION-002.2, AC-TASKS-COORDINATION-002.3
func TestCompletionGateEntryPoints(t *testing.T) {
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "user-1", Role: authn.RoleAdmin, Synthetic: true})
	svc, _, repo := createTestService(t)
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	svc.workflowStepGetter.(*fakeWorkflowStepGetter).steps["step-done"] = &wfmodels.WorkflowStep{
		ID: "step-done", WorkflowID: "wf-source", Name: "Done", Position: 2, CompleteTaskOnEnter: true,
	}
	createMoveTask(t, ctx, repo, "task-without-gate", "wf-source", "step-source", nil)
	if _, err := svc.MoveTask(ctx, "task-without-gate", "wf-source", "step-done", 0); err != nil {
		t.Fatalf("completion without criteria should remain compatible: %v", err)
	}
	createMoveTask(t, ctx, repo, "task-gated", "wf-source", "step-source", nil)
	if _, err := repo.DB().ExecContext(ctx, `CREATE TABLE github_task_prs (id TEXT PRIMARY KEY, task_id TEXT NOT NULL, head_sha TEXT NOT NULL)`); err != nil {
		t.Fatalf("create PR evidence fixture: %v", err)
	}
	if _, err := repo.DB().ExecContext(ctx, `INSERT INTO github_task_prs (id, task_id, head_sha) VALUES ('pr-row-1', 'task-gated', 'head-1')`); err != nil {
		t.Fatalf("insert PR evidence fixture: %v", err)
	}

	criteria, err := svc.SetTaskCompletionCriteria(ctx, "task-gated", SetTaskCompletionCriteriaRequest{
		ExpectedRevision: 0,
		Criteria: []models.TaskCompletionCriterion{{
			ID: "checks-pass", Description: "Required checks pass",
			EvidenceSubject: models.TaskCompletionEvidenceSubject{Kind: models.TaskCompletionEvidenceGitHubPRHead, ID: "pr-row-1"},
		}},
	})
	if err != nil {
		t.Fatalf("set criteria: %v", err)
	}

	if _, err := svc.MoveTask(ctx, "task-gated", "wf-source", "step-done", 0); !errors.Is(err, repoerrors.ErrTaskCompletionGateBlocked) {
		t.Fatalf("move with unmet criterion = %v, want completion gate blocked", err)
	}
	if _, err := svc.UpdateTaskState(ctx, "task-gated", v1.TaskStateCompleted); !errors.Is(err, repoerrors.ErrTaskCompletionGateBlocked) {
		t.Fatalf("direct completion = %v, want completion gate blocked", err)
	}
	if _, err := svc.SetTaskCompletionCriteria(ctx, "task-gated", SetTaskCompletionCriteriaRequest{
		ExpectedRevision: criteria.Revision,
	}); !errors.Is(err, repoerrors.ErrTaskCompletionHumanConfirmationRequired) {
		t.Fatalf("removing unmet criterion without human confirmation = %v, want confirmation required", err)
	}

	verified, err := svc.VerifyTaskCompletionCriterion(ctx, "task-gated", VerifyTaskCompletionCriterionRequest{
		ExpectedRevision: criteria.Revision,
		CriterionID:      "checks-pass",
		Evidence: models.TaskCompletionEvidence{
			Subject: models.TaskCompletionEvidenceSubject{Kind: models.TaskCompletionEvidenceGitHubPRHead, ID: "pr-row-1", Revision: "head-1"},
			Summary: "Checks passed for the pull request head.",
		},
	})
	if err != nil {
		t.Fatalf("verify criteria: %v", err)
	}
	if verified.Blocked {
		t.Fatal("criteria remain blocked immediately after verification")
	}
	if _, err := repo.DB().ExecContext(ctx, `UPDATE github_task_prs SET head_sha = 'head-2' WHERE id = 'pr-row-1'`); err != nil {
		t.Fatalf("advance PR head: %v", err)
	}
	if _, err := svc.MoveTask(ctx, "task-gated", "wf-source", "step-done", 0); !errors.Is(err, repoerrors.ErrTaskCompletionGateBlocked) {
		t.Fatalf("move with stale PR evidence = %v, want completion gate blocked", err)
	}

	verified, err = svc.VerifyTaskCompletionCriterion(ctx, "task-gated", VerifyTaskCompletionCriterionRequest{
		ExpectedRevision: criteria.Revision,
		CriterionID:      "checks-pass",
		Evidence: models.TaskCompletionEvidence{
			Subject: models.TaskCompletionEvidenceSubject{Kind: models.TaskCompletionEvidenceGitHubPRHead, ID: "pr-row-1", Revision: "head-2"},
			Summary: "Checks passed for the current pull request head.",
		},
	})
	if err != nil {
		t.Fatalf("refresh PR evidence: %v", err)
	}
	if verified.Blocked {
		t.Fatal("criteria remain blocked after verifying the current PR head")
	}
	if _, err := svc.SetTaskCompletionCriteria(ctx, "task-gated", SetTaskCompletionCriteriaRequest{
		ExpectedRevision: criteria.Revision,
		Criteria: []models.TaskCompletionCriterion{{
			ID: "checks-pass", Description: "Required CI checks pass",
			EvidenceSubject: models.TaskCompletionEvidenceSubject{Kind: models.TaskCompletionEvidenceGitHubPRHead, ID: "pr-row-1"},
		}},
	}); err != nil {
		t.Fatalf("change criterion description: %v", err)
	}
	if _, err := svc.MoveTask(ctx, "task-gated", "wf-source", "step-done", 0); !errors.Is(err, repoerrors.ErrTaskCompletionGateBlocked) {
		t.Fatalf("move after criterion change = %v, want completion gate blocked", err)
	}
	verified, err = svc.VerifyTaskCompletionCriterion(ctx, "task-gated", VerifyTaskCompletionCriterionRequest{
		ExpectedRevision: criteria.Revision + 1,
		CriterionID:      "checks-pass",
		Evidence: models.TaskCompletionEvidence{
			Subject: models.TaskCompletionEvidenceSubject{Kind: models.TaskCompletionEvidenceGitHubPRHead, ID: "pr-row-1", Revision: "head-2"},
			Summary: "CI passed for the updated criterion.",
		},
	})
	if err != nil || verified.Blocked {
		t.Fatalf("reverify changed criterion: snapshot=%+v err=%v", verified, err)
	}
	if _, err := svc.MoveTask(ctx, "task-gated", "wf-source", "step-done", 0); err != nil {
		t.Fatalf("move with current evidence: %v", err)
	}

	createMoveTask(t, ctx, repo, "task-reopened", "wf-source", "step-source", nil)
	if _, err := svc.SetTaskCompletionCriteria(ctx, "task-reopened", SetTaskCompletionCriteriaRequest{
		Criteria: []models.TaskCompletionCriterion{{ID: "tests", Description: "Tests pass", EvidenceSubject: models.TaskCompletionEvidenceSubject{Kind: models.TaskCompletionEvidenceGitHubPRHead, ID: "pr-row-1"}}},
	}); err != nil {
		t.Fatalf("set reopen criteria: %v", err)
	}
	if _, err := svc.MoveTaskWithOptions(ctx, "task-reopened", "wf-source", "step-done", 0, MoveTaskOptions{
		CompletionOverride: &TaskCompletionMoveOverrideRequest{ExpectedRevision: 1, Reason: "Release accepted by product owner"},
	}); err != nil {
		t.Fatalf("reasoned human override: %v", err)
	}
	history, err := svc.ListTaskCompletionGateHistory(ctx, "task-reopened")
	if err != nil {
		t.Fatalf("read completion history: %v", err)
	}
	if len(history) != 2 || history[1].Action != "completion_overridden" || history[1].ActorID != "user-1" || history[1].Reason != "Release accepted by product owner" {
		t.Fatalf("completion override audit = %+v", history)
	}

	createMoveTask(t, ctx, repo, "task-criteria-removal", "wf-source", "step-source", nil)
	criteriaToRemove, err := svc.SetTaskCompletionCriteria(ctx, "task-criteria-removal", SetTaskCompletionCriteriaRequest{
		Criteria: []models.TaskCompletionCriterion{{ID: "review", Description: "Review is resolved", EvidenceSubject: models.TaskCompletionEvidenceSubject{Kind: models.TaskCompletionEvidenceArtifact, ID: "review-1"}}},
	})
	if err != nil {
		t.Fatalf("set criteria for removal confirmation: %v", err)
	}
	removed, err := svc.SetTaskCompletionCriteria(ctx, "task-criteria-removal", SetTaskCompletionCriteriaRequest{
		ExpectedRevision:  criteriaToRemove.Revision,
		HumanConfirmation: &TaskCompletionHumanConfirmation{ExpectedRevision: criteriaToRemove.Revision, Reason: "The review was withdrawn."},
	})
	if err != nil {
		t.Fatalf("remove unmet criteria with human confirmation: %v", err)
	}
	if removed.Blocked || len(removed.Criteria) != 0 {
		t.Fatalf("removed criteria snapshot = %+v", removed)
	}
}

func TestCompletionGateExactCommandsUseTaskVersionClaimFenceAndReplay(t *testing.T) {
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "user-1", Role: authn.RoleAdmin, Synthetic: true})
	svc, _, repo := createTestService(t)
	createMoveTask(t, ctx, repo, "task-exact-gate", "wf-source", "step-source", nil)
	task, err := repo.GetTask(ctx, "task-exact-gate")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	version := task.UpdatedAt.UTC().Format(time.RFC3339Nano)
	claim, err := svc.ChangeTaskManagementClaim(ctx, task.ID, models.TaskManagementClaimChange{
		Action: models.TaskManagementClaimAcquire, TaskID: task.ID, WorkspaceID: task.WorkspaceID,
		ExpectedTaskResourceVersion: version, InstallationID: "plugin-installation", InstanceKey: "manager",
		ActorID: "plugin:plugin-installation",
	})
	if err != nil {
		t.Fatalf("acquire task management claim: %v", err)
	}
	fence := models.TaskManagementClaimFence{InstallationID: "plugin-installation", InstanceKey: "manager", Generation: claim.Generation}
	criteriaRequest := ExactTaskCompletionCriteriaRequest{
		WorkspaceID: "ws-1", ExpectedTaskResourceVersion: version, ExpectedRevision: 0,
		OperationID: "completion-operation-1", PayloadDigest: "sha256:" + strings.Repeat("a", 64),
		ClaimFence: fence, ActorID: "plugin:plugin-installation",
		Criteria: []models.TaskCompletionCriterion{{
			ID: "tests", Description: "Tests pass",
			EvidenceSubject: models.TaskCompletionEvidenceSubject{Kind: models.TaskCompletionEvidenceArtifact, ID: "artifact-1"},
		}},
	}
	set, err := svc.SetTaskCompletionCriteriaExact(ctx, task.ID, criteriaRequest)
	if err != nil || set.Snapshot.Revision != 1 || !set.Snapshot.Blocked || set.AlreadyApplied {
		t.Fatalf("set exact criteria = %+v err=%v", set, err)
	}

	replayed, err := svc.SetTaskCompletionCriteriaExact(ctx, task.ID, criteriaRequest)
	if err != nil || !replayed.AlreadyApplied || replayed.Snapshot.Revision != set.Snapshot.Revision {
		t.Fatalf("replay exact criteria = %+v err=%v", replayed, err)
	}

	conflicting := criteriaRequest
	conflicting.PayloadDigest = "sha256:" + strings.Repeat("b", 64)
	if _, err := svc.SetTaskCompletionCriteriaExact(ctx, task.ID, conflicting); !errors.Is(err, repoerrors.ErrTaskOperationConflict) {
		t.Fatalf("operation digest conflict = %v, want task operation conflict", err)
	}

	staleClaim := criteriaRequest
	staleClaim.OperationID = "completion-operation-stale-claim"
	staleClaim.PayloadDigest = "sha256:" + strings.Repeat("c", 64)
	staleClaim.ClaimFence = models.TaskManagementClaimFence{InstallationID: "plugin-installation", InstanceKey: "manager", Generation: claim.Generation + 1}
	if _, err := svc.SetTaskCompletionCriteriaExact(ctx, task.ID, staleClaim); !errors.Is(err, repoerrors.ErrTaskManagementClaimConflict) {
		t.Fatalf("stale claim fence = %v, want claim conflict", err)
	}

	bumpedTask, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("load task before version change: %v", err)
	}
	bumpedTask.Title = "A newer task version"
	if err := repo.UpdateTask(ctx, bumpedTask); err != nil {
		t.Fatalf("advance task resource version: %v", err)
	}
	staleTask := criteriaRequest
	staleTask.OperationID = "completion-operation-stale-task"
	staleTask.PayloadDigest = "sha256:" + strings.Repeat("d", 64)
	if _, err := svc.SetTaskCompletionCriteriaExact(ctx, task.ID, staleTask); !errors.Is(err, repoerrors.ErrTaskVersionConflict) {
		t.Fatalf("stale task version = %v, want task version conflict", err)
	}

	currentTask, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	verify := ExactTaskCompletionEvidenceRequest{
		WorkspaceID: "ws-1", ExpectedTaskResourceVersion: currentTask.UpdatedAt.UTC().Format(time.RFC3339Nano),
		ExpectedRevision: 1, OperationID: "completion-operation-verify", PayloadDigest: "sha256:" + strings.Repeat("e", 64),
		ClaimFence: fence, ActorID: "plugin:plugin-installation", CriterionID: "tests",
		Evidence: models.TaskCompletionEvidence{
			Subject: models.TaskCompletionEvidenceSubject{Kind: models.TaskCompletionEvidenceArtifact, ID: "artifact-1", Revision: "artifact-v1"},
			Summary: "The required test run passed.", Reference: "run-42",
		},
	}
	verified, err := svc.VerifyTaskCompletionCriterionExact(ctx, task.ID, verify)
	if err != nil || verified.Snapshot.Blocked || verified.AlreadyApplied {
		t.Fatalf("verify exact evidence = %+v err=%v", verified, err)
	}
	verifiedReplay, err := svc.VerifyTaskCompletionCriterionExact(ctx, task.ID, verify)
	if err != nil || !verifiedReplay.AlreadyApplied || verifiedReplay.Snapshot.Criteria[0].Evidence.Subject.Revision != "artifact-v1" {
		t.Fatalf("replay exact evidence = %+v err=%v", verifiedReplay, err)
	}
}
