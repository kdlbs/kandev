package service_test

import (
	"strings"
	"testing"

	officeruntime "github.com/kandev/kandev/internal/office/runtime"
	"github.com/kandev/kandev/internal/office/service"
)

// TestBuildPrompt_TaskCommentReviewStageVerdictPrompt pins
// AC-OFFICE-GATE-COMMENT-003.1/.2: a task_comment run whose StageType is
// review frames the run as a review, quotes the comment under a From:/Comment:
// pair, and asks for a verdict — instead of the generic "Address this
// comment" prompt.
func TestBuildPrompt_TaskCommentReviewStageVerdictPrompt(t *testing.T) {
	pc := &service.PromptContext{
		Reason:            service.RunReasonTaskComment,
		StageType:         "review",
		TaskIdentifier:    "KAN-5",
		TaskTitle:         "Fix bug",
		CommentBody:       "Please add a test for the edge case.",
		CommentAuthor:     "Alice",
		CommentAuthorType: "user",
	}
	prompt := service.BuildPrompt(pc)

	if !strings.HasPrefix(prompt, "You are reviewing") {
		t.Fatalf("prompt = %q, want reviewer framing", prompt)
	}
	for _, want := range []string{"[KAN-5]", "Fix bug", "From: Alice (user)", "Comment: Please add a test for the edge case.", "verdict"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "Address this comment") {
		t.Errorf("gate comment prompt must not fall back to the generic comment prompt:\n%s", prompt)
	}
}

// TestBuildPrompt_TaskCommentApprovalStageVerdictPrompt mirrors the review
// case for the approval stage.
func TestBuildPrompt_TaskCommentApprovalStageVerdictPrompt(t *testing.T) {
	pc := &service.PromptContext{
		Reason:            service.RunReasonTaskComment,
		StageType:         "approval",
		TaskIdentifier:    "KAN-6",
		TaskTitle:         "Ship release",
		CommentBody:       "Confirm the rollout plan first.",
		CommentAuthor:     "Bob",
		CommentAuthorType: "user",
	}
	prompt := service.BuildPrompt(pc)

	if !strings.HasPrefix(prompt, "You are approving") {
		t.Fatalf("prompt = %q, want approver framing", prompt)
	}
	if !strings.Contains(prompt, "Comment: Confirm the rollout plan first.") {
		t.Errorf("prompt missing quoted comment:\n%s", prompt)
	}
}

// TestBuildPrompt_TaskCommentGateStageUnloadableCommentFallback pins
// AC-OFFICE-GATE-COMMENT-003.6: when the triggering comment failed to load
// (CommentBody empty), the prompt keeps the review/approval framing but
// states the comment is no longer available instead of quoting nothing.
func TestBuildPrompt_TaskCommentGateStageUnloadableCommentFallback(t *testing.T) {
	pc := &service.PromptContext{
		Reason:         service.RunReasonTaskComment,
		StageType:      "review",
		TaskIdentifier: "KAN-7",
		TaskTitle:      "Investigate flake",
	}
	prompt := service.BuildPrompt(pc)

	if !strings.HasPrefix(prompt, "You are reviewing") {
		t.Fatalf("prompt = %q, want reviewer framing even without a loaded comment", prompt)
	}
	if !strings.Contains(prompt, "no longer available") {
		t.Errorf("prompt missing no-longer-available sentence:\n%s", prompt)
	}
	if strings.Contains(prompt, "Comment:") {
		t.Errorf("prompt must not render a Comment: heading with no body:\n%s", prompt)
	}
}

// TestBuildPrompt_TaskCommentGateStageEndsWithDecisionContract pins
// AC-OFFICE-GATE-COMMENT-003.3: buildGateCommentPrompt's own output ends with
// the decision contract exactly when record_step_decision is among
// AllowedActions.
func TestBuildPrompt_TaskCommentGateStageEndsWithDecisionContract(t *testing.T) {
	pc := &service.PromptContext{
		Reason:            service.RunReasonTaskComment,
		StageType:         "review",
		TaskIdentifier:    "KAN-8",
		TaskTitle:         "Fix bug",
		CommentBody:       "Looks off.",
		CommentAuthor:     "Alice",
		CommentAuthorType: "user",
		AllowedActions:    []string{officeruntime.AvailableActionRecordStepDecision},
	}
	prompt := service.BuildPrompt(pc)

	if !strings.Contains(prompt, `$KANDEV_CLI kandev task decision --decision approved --reason "..."`) {
		t.Fatalf("prompt missing decision CLI contract:\n%s", prompt)
	}
}

// TestBuildPrompt_TaskCommentGateStageOmitsDecisionContractWithoutAction pins
// the other half of AC-003.3: no decision-seat action means no decision
// contract text at all.
func TestBuildPrompt_TaskCommentGateStageOmitsDecisionContractWithoutAction(t *testing.T) {
	pc := &service.PromptContext{
		Reason:            service.RunReasonTaskComment,
		StageType:         "approval",
		TaskIdentifier:    "KAN-9",
		TaskTitle:         "Ship release",
		CommentBody:       "Looks good.",
		CommentAuthor:     "Bob",
		CommentAuthorType: "user",
		AllowedActions:    []string{officeruntime.CapabilityPostComment},
	}
	prompt := service.BuildPrompt(pc)

	for _, fragment := range []string{
		"$KANDEV_CLI kandev task decision",
		"record_step_decision",
	} {
		if strings.Contains(prompt, fragment) {
			t.Fatalf("prompt must not contain decision contract %q without the decision action:\n%s", fragment, prompt)
		}
	}
}

// TestBuildPrompt_TaskCommentWorkStageUsesPlainCommentPrompt pins that a
// task_comment run resolved to the "work" stage (or any non-gate stage)
// keeps rendering the existing plain comment prompt, never the verdict
// framing.
func TestBuildPrompt_TaskCommentWorkStageUsesPlainCommentPrompt(t *testing.T) {
	pc := &service.PromptContext{
		Reason:            service.RunReasonTaskComment,
		StageType:         "work",
		TaskIdentifier:    "KAN-10",
		TaskTitle:         "Build feature",
		CommentBody:       "Please add a test.",
		CommentAuthor:     "Alice",
		CommentAuthorType: "user",
	}
	prompt := service.BuildPrompt(pc)

	if !strings.Contains(prompt, "New comment") || !strings.Contains(prompt, "Address this comment") {
		t.Fatalf("work-stage task_comment prompt should stay the plain comment prompt, got:\n%s", prompt)
	}
}
