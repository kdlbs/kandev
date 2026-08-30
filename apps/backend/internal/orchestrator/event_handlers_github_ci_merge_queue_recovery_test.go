package orchestrator

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/task/models"
)

type mergeQueueRecoveryGitHubService struct {
	*mockGitHubService
}

func (s *mergeQueueRecoveryGitHubService) RecordTaskCIMergeQueueObservation(
	_ context.Context,
	observation github.TaskCIMergeQueueObservation,
) error {
	if s.ciPRState == nil {
		s.ciPRState = &github.TaskCIPRAutomationState{
			TaskID: observation.TaskID, RepositoryID: observation.RepositoryID, PRNumber: observation.PRNumber,
		}
	}
	if observation.ActiveQueueHeadSHA != "" {
		s.ciPRState.LastQueueAttemptHeadSHA = observation.ActiveQueueHeadSHA
		if observation.MergeSignature != "" {
			s.ciPRState.LastMergeSignature = observation.MergeSignature
		}
	} else if s.ciPRState.LastQueueAttemptHeadSHA == "" {
		s.ciPRState.LastQueueAttemptHeadSHA = observation.RemovalObservedHeadSHA
	}
	if observation.RemovalCause != "" {
		s.ciPRState.LastQueueRemovalCause = observation.RemovalCause
	}
	return nil
}

func (s *mergeQueueRecoveryGitHubService) RecordTaskCIFixAttempt(
	ctx context.Context,
	attempt github.TaskCIFixAttempt,
) error {
	if err := s.mockGitHubService.RecordTaskCIFixAttempt(ctx, attempt); err != nil {
		return err
	}
	if attempt.QueueRemovalEventID != "" {
		if s.ciPRState == nil {
			s.ciPRState = &github.TaskCIPRAutomationState{
				TaskID: attempt.TaskID, RepositoryID: attempt.RepositoryID, PRNumber: attempt.PRNumber,
			}
		}
		s.ciPRState.LastQueueFixEventID = attempt.QueueRemovalEventID
		s.ciPRState.LastQueueRemovalCause = attempt.QueueRemovalCause
	}
	return nil
}

func (s *mergeQueueRecoveryGitHubService) RecordTaskCIMergeAttempt(
	ctx context.Context,
	attempt github.TaskCIMergeAttempt,
) error {
	if err := s.mockGitHubService.RecordTaskCIMergeAttempt(ctx, attempt); err != nil {
		return err
	}
	if s.ciPRState == nil {
		s.ciPRState = &github.TaskCIPRAutomationState{
			TaskID: attempt.TaskID, RepositoryID: attempt.RepositoryID, PRNumber: attempt.PRNumber,
		}
	}
	s.ciPRState.LastQueueAttemptHeadSHA = attempt.AttemptedHeadSHA
	s.ciPRState.LastMergeSignature = attempt.Signature
	return nil
}

func TestCIAutomationMergeQueueRecoveryClassifiesReviewedReasons(t *testing.T) {
	tests := []struct {
		name   string
		reason string
		want   string
	}{
		{name: "checks failed enum", reason: "CHECKS_FAILED", want: ciAutomationQueueRemovalCauseChecksFailed},
		{name: "CI checks failed text", reason: "CI checks failed", want: ciAutomationQueueRemovalCauseChecksFailed},
		{name: "checks timed out text", reason: "Checks timed out", want: ciAutomationQueueRemovalCauseChecksTimedOut},
		{name: "merge conflict text", reason: "merge conflict", want: ciAutomationQueueRemovalCauseConflict},
		{name: "manual", reason: "MANUAL", want: ciAutomationQueueRemovalCauseManual},
		{name: "branch protection", reason: "BRANCH_PROTECTION", want: ciAutomationQueueRemovalCauseBranchProtection},
		{name: "unknown", reason: "provider changed this", want: ciAutomationQueueRemovalCauseUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ciAutomationQueueRemovalCause(tt.reason, &github.TaskPR{}); got != tt.want {
				t.Fatalf("ciAutomationQueueRemovalCause(%q) = %q, want %q", tt.reason, got, tt.want)
			}
		})
	}
}

func TestCIAutomationMergeQueueRecoveryRequiresDurableAttemptEvidence(t *testing.T) {
	pr := &github.TaskPR{HeadSHA: "head-a"}
	tests := []struct {
		name  string
		state *github.TaskCIPRAutomationState
		want  bool
	}{
		{name: "no state", state: nil, want: false},
		{
			name:  "passive baseline has no attempt signature",
			state: &github.TaskCIPRAutomationState{LastQueueAttemptHeadSHA: "head-a"},
			want:  false,
		},
		{
			name:  "empty attempt head fails closed",
			state: &github.TaskCIPRAutomationState{LastMergeSignature: "merge-a"},
			want:  false,
		},
		{
			name: "matching attempted head and signature",
			state: &github.TaskCIPRAutomationState{
				LastQueueAttemptHeadSHA: "head-a",
				LastMergeSignature:      "merge-a",
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ciAutomationQueueRemovalBelongsToCurrentHead(pr, tt.state); got != tt.want {
				t.Fatalf("queue removal evidence = %v, want %v for state %+v", got, tt.want, tt.state)
			}
		})
	}
}

func TestCIAutomationMergeQueueRecoveryUsesConflictEvidenceForUnknownReason(t *testing.T) {
	pr := &github.TaskPR{MergeableState: "dirty"}
	if got := ciAutomationQueueRemovalCause("provider changed this", pr); got != ciAutomationQueueRemovalCauseConflict {
		t.Fatalf("conflict evidence cause = %q, want %q", got, ciAutomationQueueRemovalCauseConflict)
	}
	if got := ciAutomationQueueRemovalCause("provider changed this", &github.TaskPR{MergeQueueState: "UNMERGEABLE"}); got != ciAutomationQueueRemovalCauseConflict {
		t.Fatalf("unmergeable queue cause = %q, want %q", got, ciAutomationQueueRemovalCauseConflict)
	}
}

func TestCIAutomationActiveQueueCheckIgnoresStaleIdentifiers(t *testing.T) {
	if ciAutomationHasActiveMergeQueueEntry(&github.TaskPR{
		State: "open", MergeQueueEntryID: "stale-entry", MergeQueueEntryHeadSHA: "stale-head",
	}) {
		t.Fatal("stale queue identifiers were treated as an active queue entry")
	}
	if !ciAutomationHasActiveMergeQueueEntry(&github.TaskPR{State: "open", MergeQueueState: "queued"}) {
		t.Fatal("queued merge queue state was not treated as active")
	}
	if ciAutomationHasActiveMergeQueueEntry(&github.TaskPR{State: "closed", MergeQueueState: "queued"}) {
		t.Fatal("closed PR was treated as an active queue entry")
	}
}

func TestCIAutomationMergeQueueRecoveryDoesNotTreatBeforeCommitAsCheckIdentity(t *testing.T) {
	pr := &github.TaskPR{
		HeadSHA: "head-a", MergeQueueLastRemovalID: "removal-a", MergeQueueLastRemovalBeforeSHA: "merge-group-a",
		MergeQueueLastRemovalReason: "CHECKS_FAILED", State: "open",
	}
	snapshot, actionable := ciAutomationQueueRemovalSnapshot(pr)
	if !actionable || snapshot == nil {
		t.Fatalf("expected actionable queue removal, snapshot=%+v actionable=%v", snapshot, actionable)
	}
	if snapshot.BeforeCommit != "merge-group-a" || snapshot.CheckCommit != "" {
		t.Fatalf("beforeCommit was treated as a check identity: %+v", snapshot)
	}
}

func TestCIAutomationMergeQueueRecoveryQueuesOneRepairPerRemoval(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	ghSvc := &mergeQueueRecoveryGitHubService{mockGitHubService: &mockGitHubService{
		ciOptionsResp: &github.TaskCIOptionsResponse{
			TaskID: "task-1", AutoFixEnabled: true, EffectiveAutoFixPrompt: "Repair the PR\n\n{{pr.feedback}}",
		},
		prFeedback: &github.PRFeedback{},
	}}
	ghSvc.ciPRState = &github.TaskCIPRAutomationState{
		TaskID: "task-1", RepositoryID: "repo-1", PRNumber: 42,
		LastQueueAttemptHeadSHA: "head-a", LastMergeSignature: "merge-a",
	}
	svc.SetGitHubService(ghSvc)
	pr := &github.TaskPR{
		TaskID: "task-1", RepositoryID: "repo-1", Owner: "acme", Repo: "widget", PRNumber: 42,
		State: "open", ChecksState: "success", HeadSHA: "head-a",
		MergeQueueLastRemovalID: "removal-a", MergeQueueLastRemovalReason: "CHECKS_FAILED",
		MergeQueueLastRemovedAt: func() *time.Time { value := time.Now().UTC(); return &value }(),
	}

	if err := svc.handleTaskPRCIAutomationWithRefresh(ctx, pr, false); err != nil {
		t.Fatalf("first queue recovery evaluation: %v", err)
	}
	if got := svc.messageQueue.GetStatus(ctx, "session-1"); got.Count != 1 || !strings.Contains(strings.ToLower(got.Entries[0].Content), "merge queue") {
		t.Fatalf("queue recovery prompt = %+v, want one merge-queue prompt", got)
	}
	if len(ghSvc.fixAttempts) != 1 || ghSvc.fixAttempts[0].QueueRemovalEventID != "removal-a" || !ghSvc.fixAttempts[0].IncrementRound {
		t.Fatalf("queue recovery attempts = %+v, want one round-consuming removal attempt", ghSvc.fixAttempts)
	}

	if err := svc.handleTaskPRCIAutomationWithRefresh(ctx, pr, false); err != nil {
		t.Fatalf("duplicate queue recovery evaluation: %v", err)
	}
	if got := svc.messageQueue.GetStatus(ctx, "session-1"); got.Count != 1 || len(ghSvc.fixAttempts) != 1 {
		t.Fatalf("duplicate queue recovery was dispatched: status=%+v attempts=%+v", got, ghSvc.fixAttempts)
	}
}

func TestCIAutomationMergeQueueRecoveryBlocksSameHeadAndRequeuesNewHead(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	ghSvc := &mergeQueueRecoveryGitHubService{mockGitHubService: &mockGitHubService{
		ciOptionsResp: &github.TaskCIOptionsResponse{TaskID: "task-1", AutoMergeEnabled: true},
	}}
	svc.SetGitHubService(ghSvc)
	now := time.Now().UTC()
	pr := &github.TaskPR{
		TaskID: "task-1", RepositoryID: "repo-1", Owner: "acme", Repo: "widget", PRNumber: 42,
		State: "open", ChecksState: "success", ReviewState: "approved", MergeableState: "clean",
		HeadSHA: "head-a", LastSyncedAt: &now, MergeQueueLastRemovalID: "removal-a",
		MergeQueueLastRemovalReason: "CHECKS_FAILED",
	}

	if err := svc.handleTaskPRCIAutomationWithRefresh(ctx, pr, false); err != nil {
		t.Fatalf("same-head evaluation: %v", err)
	}
	if ghSvc.mergeCalls != 0 {
		t.Fatalf("same-head removal was requeued: merge calls=%d", ghSvc.mergeCalls)
	}

	pr.HeadSHA = "head-b"
	if err := svc.handleTaskPRCIAutomationWithRefresh(ctx, pr, false); err != nil {
		t.Fatalf("new-head evaluation: %v", err)
	}
	if ghSvc.mergeCalls != 1 || len(ghSvc.mergeAttempts) != 1 || ghSvc.mergeAttempts[0].AttemptedHeadSHA != "head-b" {
		t.Fatalf("new-head queue attempt = calls %d attempts %+v, want one head-b attempt", ghSvc.mergeCalls, ghSvc.mergeAttempts)
	}
}

func TestCIAutomationMergeQueueRecoveryAdoptsActiveQueueEntry(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	ghSvc := &mergeQueueRecoveryGitHubService{mockGitHubService: &mockGitHubService{
		ciOptionsResp: &github.TaskCIOptionsResponse{TaskID: "task-1", AutoMergeEnabled: true},
	}}
	svc.SetGitHubService(ghSvc)
	now := time.Now().UTC()
	pr := &github.TaskPR{
		TaskID: "task-1", RepositoryID: "repo-1", Owner: "acme", Repo: "widget", PRNumber: 42,
		State: "open", ChecksState: "success", MergeableState: "clean", HeadSHA: "head-a", LastSyncedAt: &now,
		MergeQueueState: "QUEUED", MergeQueueEntryID: "entry-a", MergeQueueEntryHeadSHA: "head-a",
	}

	if err := svc.handleTaskPRCIAutomationWithRefresh(ctx, pr, false); err != nil {
		t.Fatalf("active queue adoption: %v", err)
	}
	if ghSvc.mergeCalls != 0 {
		t.Fatalf("active queue entry was duplicated: merge calls=%d", ghSvc.mergeCalls)
	}
	if ghSvc.ciPRState == nil || ghSvc.ciPRState.LastQueueAttemptHeadSHA != "head-a" {
		t.Fatalf("active queue adoption state = %+v, want head-a", ghSvc.ciPRState)
	}
}

func TestCIAutomationMergeQueueRecoveryMergeSignatureIncludesHeadSHA(t *testing.T) {
	left := &github.TaskPR{TaskID: "task-1", RepositoryID: "repo-1", PRNumber: 42, HeadSHA: "head-a"}
	right := *left
	right.HeadSHA = "head-b"
	if ciAutomationMergeSignature(left) == ciAutomationMergeSignature(&right) {
		t.Fatal("merge signature did not change when PR head SHA changed")
	}
}
