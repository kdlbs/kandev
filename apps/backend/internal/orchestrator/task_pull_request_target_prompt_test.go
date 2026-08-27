package orchestrator

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

// @covers AC-WORKSPACES-BRANCH-POLICIES-004.6
func TestStartCreatedSession_InstructsAgentToUseSnapshottedPullRequestTarget(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateCreated)
	seedExecutorRunning(t, repo, "session1", "task1", "exec-1")
	seedPolicyBackedTaskRepository(t, repo, "task1")

	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task1"] = &v1.Task{ID: "task1", Title: "Release", State: v1.TaskStateInProgress}
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.promptTargets = repo
	messages := &mockMessageCreator{}
	svc.messageCreator = messages

	_, err := svc.StartCreatedSession(
		ctx, "task1", "session1", "profile1", "Create the release", false, false, false, nil, nil,
	)
	require.NoError(t, err)
	require.Len(t, messages.userMessages, 1)

	content := messages.userMessages[0].content
	require.Contains(t, content, "BRANCH POLICY PULL REQUEST TARGETS")
	require.Contains(t, content, `Repository "kandev"`)
	require.Contains(t, content, `working branch "release/next"`)
	require.Contains(t, content, `target branch "main"`)
	require.Contains(t, content, `gh pr create --base "main"`)
	require.Equal(t, "Create the release", sysprompt.StripSystemContent(content))
}

// @covers AC-WORKSPACES-BRANCH-POLICIES-004.6
func TestStartCreatedSession_GivesPassthroughAgentPullRequestTargetInstruction(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateCreated)
	seedExecutorRunning(t, repo, "session1", "task1", "exec-1")
	seedPolicyBackedTaskRepository(t, repo, "task1")

	session, err := repo.GetTaskSession(ctx, "session1")
	require.NoError(t, err)
	session.IsPassthrough = true
	require.NoError(t, repo.UpdateTaskSession(ctx, session))

	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task1"] = &v1.Task{ID: "task1", Title: "Release", State: v1.TaskStateInProgress}
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.promptTargets = repo
	messages := &mockMessageCreator{}
	svc.messageCreator = messages

	_, err = svc.StartCreatedSession(
		ctx, "task1", "session1", "profile1", "Create the release", false, false, false, nil, nil,
	)
	require.NoError(t, err)
	require.Len(t, messages.userMessages, 1)

	content := messages.userMessages[0].content
	require.NotContains(t, content, sysprompt.TagStart)
	require.True(t, strings.HasSuffix(content, "Create the release"))
	require.Contains(t, content, `gh pr create --base "main"`)
}

// @covers AC-WORKSPACES-BRANCH-POLICIES-004.6
func TestAutoStartStepPrompt_ResetContextReinjectsSnapshottedPullRequestTarget(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)
	seedPolicyBackedTaskRepository(t, repo, "task1")

	session, err := repo.GetTaskSession(ctx, "session1")
	require.NoError(t, err)
	session.AgentExecutionID = "execution-1"
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	seedExecutorRunning(t, repo, session.ID, session.TaskID, session.AgentExecutionID)

	step := &wfmodels.WorkflowStep{
		ID: "step-review", WorkflowID: "wf1", Name: "Review",
		Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{
			{Type: wfmodels.OnEnterResetAgentContext},
			{Type: wfmodels.OnEnterAutoStartAgent},
		}},
	}
	stepGetter := newMockStepGetter()
	stepGetter.steps[step.ID] = step
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo, isAgentRunning: true}
	svc := createTestServiceWithScheduler(repo, stepGetter, newMockTaskRepo(), agentMgr)
	svc.promptTargets = repo
	messages := &mockMessageCreator{}
	svc.messageCreator = messages

	require.NoError(t, svc.autoStartStepPrompt(
		ctx, "task1", session, step, "Review the release", false, false,
	))
	require.Len(t, messages.userMessages, 1)
	require.Contains(t, messages.userMessages[0].content, `gh pr create --base "main"`)
	require.Len(t, agentMgr.capturedPromptCalls, 1)
	require.Contains(t, agentMgr.capturedPromptCalls[0].Prompt, `gh pr create --base "main"`)
}

func seedPolicyBackedTaskRepository(t *testing.T, repo interface {
	CreateRepository(context.Context, *models.Repository) error
	CreateTaskRepository(context.Context, *models.TaskRepository) error
}, taskID string) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-1", WorkspaceID: "ws1", Name: "kandev", SourceType: "local",
	}))
	require.NoError(t, repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID:                            "task-repo-1",
		TaskID:                        taskID,
		RepositoryID:                  "repo-1",
		BaseBranch:                    "develop",
		CheckoutBranch:                "release/next",
		BranchPolicyID:                "policy-release",
		BranchPolicyName:              "Release",
		BranchPolicyBaseBranch:        "develop",
		BranchPolicyBranchTemplate:    "release/{title}-{suffix}",
		BranchPolicyPullRequestTarget: "main",
	}))
}
