package orchestrator

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/task/models"
)

func setupCIAutomationFreshnessTest(t *testing.T) (*Service, *github.TaskPR) {
	t.Helper()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	t.Cleanup(svc.stopCIAutomationWorkers)
	now := time.Now().UTC()
	return svc, &github.TaskPR{
		TaskID: "task-1", RepositoryID: "repo-1", Owner: "acme", Repo: "widget",
		PRNumber: 42, State: "open", ChecksState: "success", ReviewState: "approved",
		MergeableState: "clean", HeadSHA: "observed-head", LastSyncedAt: &now,
	}
}

// @covers AC-INTEGRATIONS-GITHUB-CI-PR-FRESHNESS-001.1
func TestCIAutomationFreshStatusSkipsRefresh(t *testing.T) {
	for _, mode := range []string{"auto_fix", "auto_merge", "both"} {
		t.Run(mode, func(t *testing.T) {
			svc, pr := setupCIAutomationFreshnessTest(t)
			ghSvc := &mockGitHubService{
				ciOptionsResp: &github.TaskCIOptionsResponse{
					TaskID: "task-1", AutoFixEnabled: mode != "auto_merge", AutoMergeEnabled: mode != "auto_fix",
				},
				triggerPRSyncAllPRs: []*github.TaskPR{pr},
				prFeedback:          &github.PRFeedback{},
			}
			svc.SetGitHubService(ghSvc)
			if err := svc.handleTaskPRCIAutomation(context.Background(), pr); err != nil {
				t.Fatal(err)
			}
			if ghSvc.triggerPRSyncAllCalls != 0 {
				t.Fatalf("fresh PR forced %d sync calls, want 0", ghSvc.triggerPRSyncAllCalls)
			}
		})
	}
}

// @covers AC-INTEGRATIONS-GITHUB-CI-PR-FRESHNESS-001.3
// @covers AC-INTEGRATIONS-GITHUB-CI-PR-FRESHNESS-001.4
func TestCIAutomationFreshStatusContinuesAutomation(t *testing.T) {
	for _, mode := range []string{"auto_fix", "auto_merge"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			svc, pr := setupCIAutomationFreshnessTest(t)
			ghSvc := &mockGitHubService{
				ciOptionsResp: &github.TaskCIOptionsResponse{
					TaskID: "task-1", AutoFixEnabled: mode == "auto_fix", AutoMergeEnabled: mode == "auto_merge",
					EffectiveAutoFixPrompt: "Fix the PR\n\n{{pr.feedback}}",
				},
				triggerPRSyncAllPRs: []*github.TaskPR{pr},
				prFeedback: &github.PRFeedback{
					Comments: []github.PRComment{{ID: 99, Body: "Handle the missing validation"}},
				},
			}
			svc.SetGitHubService(ghSvc)
			if err := svc.handleTaskPRCIAutomation(ctx, pr); err != nil {
				t.Fatal(err)
			}
			if ghSvc.triggerPRSyncAllCalls != 0 {
				t.Fatalf("fresh PR forced %d sync calls, want 0", ghSvc.triggerPRSyncAllCalls)
			}
			if mode == "auto_fix" {
				if ghSvc.prFeedbackCalls != 1 {
					t.Fatalf("feedback reads = %d, want 1", ghSvc.prFeedbackCalls)
				}
				status := svc.messageQueue.GetStatus(ctx, "session-1")
				if status.Count != 1 || !strings.Contains(status.Entries[0].Content, "Handle the missing validation") {
					t.Fatalf("queued feedback prompt = %+v, want actionable comment", status)
				}
			} else if ghSvc.mergeCalls != 1 || ghSvc.mergeExpectedHeadSHA != "observed-head" {
				t.Fatalf("merge calls = %d, expected head = %q, want 1 and observed-head", ghSvc.mergeCalls, ghSvc.mergeExpectedHeadSHA)
			}
		})
	}
}

// @covers AC-INTEGRATIONS-GITHUB-CI-PR-FRESHNESS-001.2
// @covers AC-INTEGRATIONS-GITHUB-CI-PR-FRESHNESS-001.4
func TestCIAutomationStaleStatusRefreshes(t *testing.T) {
	for _, mode := range []string{"auto_fix", "auto_merge", "both"} {
		for _, timestamp := range []string{"stale", "missing"} {
			t.Run(mode+"/"+timestamp, func(t *testing.T) {
				svc, pr := setupCIAutomationFreshnessTest(t)
				refreshed := *pr
				refreshed.HeadSHA = "refreshed-head"
				pr.ChecksState = "pending"
				if timestamp == "missing" {
					pr.LastSyncedAt = nil
				} else {
					stale := time.Now().Add(-2 * github.PRSyncFreshnessWindow)
					pr.LastSyncedAt = &stale
				}
				ghSvc := &mockGitHubService{
					ciOptionsResp: &github.TaskCIOptionsResponse{
						TaskID: "task-1", AutoFixEnabled: mode != "auto_merge", AutoMergeEnabled: mode != "auto_fix",
					},
					triggerPRSyncAllPRs: []*github.TaskPR{&refreshed},
					prFeedback:          &github.PRFeedback{},
				}
				svc.SetGitHubService(ghSvc)
				if err := svc.handleTaskPRCIAutomation(context.Background(), pr); err != nil {
					t.Fatal(err)
				}
				if ghSvc.triggerPRSyncAllCalls != 1 {
					t.Fatalf("sync calls = %d, want 1", ghSvc.triggerPRSyncAllCalls)
				}
				if mode != "auto_merge" && ghSvc.prFeedbackCalls != 1 {
					t.Fatalf("feedback reads = %d, want 1 from refreshed settled checks", ghSvc.prFeedbackCalls)
				}
				if mode != "auto_fix" && (ghSvc.mergeCalls != 1 || ghSvc.mergeExpectedHeadSHA != "refreshed-head") {
					t.Fatalf("merge calls = %d, expected head = %q, want 1 and refreshed-head", ghSvc.mergeCalls, ghSvc.mergeExpectedHeadSHA)
				}
			})
		}
	}
}

// @covers AC-INTEGRATIONS-GITHUB-CI-PR-FRESHNESS-001.2
// @covers AC-INTEGRATIONS-GITHUB-CI-PR-FRESHNESS-001.5
func TestCIAutomationRefreshAdmissionPreservesScope(t *testing.T) {
	tests := []struct {
		name         string
		refresh      bool
		aggregate    bool
		perPR        bool
		target       bool
		wantSync     int
		wantFeedback int
	}{
		{name: "no refresh", aggregate: true, wantFeedback: 1},
		{name: "disabled", refresh: true},
		{name: "disabled target enabled sibling", refresh: true, aggregate: true, perPR: true},
		{name: "enabled target disabled sibling", refresh: true, perPR: true, target: true, wantSync: 1, wantFeedback: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, pr := setupCIAutomationFreshnessTest(t)
			pr.LastSyncedAt = nil
			options := &github.TaskCIOptionsResponse{TaskID: "task-1", AutoFixEnabled: tt.aggregate}
			if tt.perPR {
				options.PROptions = []*github.TaskPRAutomationOptions{
					{RepositoryID: "repo-1", PRNumber: 42, AutoFixEnabled: tt.target},
					{RepositoryID: "repo-2", PRNumber: 43, AutoFixEnabled: !tt.target},
				}
			}
			ghSvc := &mockGitHubService{
				ciOptionsResp: options, triggerPRSyncAllPRs: []*github.TaskPR{pr}, prFeedback: &github.PRFeedback{},
			}
			svc.SetGitHubService(ghSvc)
			if err := svc.handleTaskPRCIAutomationWithRefresh(context.Background(), pr, tt.refresh); err != nil {
				t.Fatal(err)
			}
			if ghSvc.triggerPRSyncAllCalls != tt.wantSync || ghSvc.prFeedbackCalls != tt.wantFeedback {
				t.Fatalf("sync/feedback calls = %d/%d, want %d/%d", ghSvc.triggerPRSyncAllCalls, ghSvc.prFeedbackCalls, tt.wantSync, tt.wantFeedback)
			}
		})
	}
}
