package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// newOfficeEnsureService seeds an assigned Office task with no session and
// returns a service whose prepare path can create its session, plus the
// profiles the prepare's workspace launch requested.
func newOfficeEnsureService(t *testing.T) (*Service, interface {
	ListTaskSessions(context.Context, string) ([]*models.TaskSession, error)
}, <-chan string) {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-p", Name: "p", CreatedAt: now, UpdatedAt: now}))
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-p", WorkspaceID: "ws-p", Name: "wf", CreatedAt: now, UpdatedAt: now})
	require.NoError(t, seedWorkflowStep(t, repo, "wfs-p"))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "t-prepare", WorkspaceID: "ws-p", WorkflowID: "wf-p", WorkflowStepID: "wfs-p",
		Title: "Office", State: v1.TaskStateTODO, ProjectID: "p-1",
		AssigneeAgentProfileID: "agent-ceo", CreatedAt: now, UpdatedAt: now,
	}))
	taskRepo := newMockTaskRepo()
	taskRepo.tasks["t-prepare"] = &v1.Task{ID: "t-prepare", WorkspaceID: "ws-p", WorkflowID: "wf-p"}
	launched := make(chan string, 4)
	agentMgr := &mockAgentManager{
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			launched <- req.AgentProfileID
			return &executor.LaunchAgentResponse{AgentExecutionID: "exec-prepared"}, nil
		},
	}
	return createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr), repo, launched
}

// staticProfileStore makes every profile a concrete (non-dynamic) claude
// profile, so the production profile resolver echoes the logical profile back.
type staticProfileStore struct {
	store.Repository
}

func (staticProfileStore) GetAgentProfile(_ context.Context, id string) (*settingsmodels.AgentProfile, error) {
	return &settingsmodels.AgentProfile{ID: id, AgentID: "agent-claude"}, nil
}

func (staticProfileStore) GetAgent(_ context.Context, id string) (*settingsmodels.Agent, error) {
	return &settingsmodels.Agent{ID: id, Name: "claude-acp"}, nil
}

// launchedProfile waits for the prepare's background workspace launch, whose
// request profile is what the session's runtime row records.
func launchedProfile(t *testing.T, launched <-chan string) string {
	t.Helper()
	select {
	case profile := <-launched:
		return profile
	case <-time.After(10 * time.Second):
		t.Fatal("the prepared session's workspace was never launched")
		return ""
	}
}

func preparedOfficeSession(t *testing.T, svc *Service, repo interface {
	ListTaskSessions(context.Context, string) ([]*models.TaskSession, error)
}) *models.TaskSession {
	t.Helper()
	falseVal := false
	_, err := svc.EnsureSession(context.Background(), "t-prepare", EnsureSessionOptions{AutoStart: &falseVal})
	require.NoError(t, err)
	sessions, err := repo.ListTaskSessions(context.Background(), "t-prepare")
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	return sessions[0]
}

func TestEnsureSession_OfficeTaskUsesRoutedProfile(t *testing.T) {
	svc, repo, launched := newOfficeEnsureService(t)
	var previewed [2]string
	svc.SetOfficeLaunchProfilePreview(func(_ context.Context, workspaceID, agentID string) (string, error) {
		previewed = [2]string{workspaceID, agentID}
		return "profile-routed", nil
	})

	session := preparedOfficeSession(t, svc, repo)

	assert.Equal(t, [2]string{"ws-p", "agent-ceo"}, previewed)
	assert.Equal(t, "agent-ceo", session.AgentProfileID, "the Office agent still owns the session")
	assert.Equal(t, "profile-routed", session.ExecutionProfileID,
		"the prepared session binds to the profile Office would launch with")
	assert.Equal(t, "profile-routed", launchedProfile(t, launched),
		"the prepared runtime is launched with the routed profile")
}

// The task workbench's auto-start launch is downgraded to a prepare while the
// task is blocked; it must bind the same routed profile as session.ensure.
func TestPrepareTaskSession_OfficeTaskUsesRoutedProfile(t *testing.T) {
	svc, repo, launched := newOfficeEnsureService(t)
	// The prepare's background launch consults the production resolver, which
	// resolves the session owner. For an Office session that owner is the
	// agent identity, not the routed execution profile.
	svc.profileExecutionResolver = agentruntime.NewProfileExecutionResolver(staticProfileStore{}, nil, false)
	svc.SetOfficeLaunchProfilePreview(func(context.Context, string, string) (string, error) {
		return "profile-routed", nil
	})

	_, err := svc.PrepareTaskSession(context.Background(), "t-prepare", "agent-ceo", "", "", "", true)
	require.NoError(t, err)

	sessions, err := repo.ListTaskSessions(context.Background(), "t-prepare")
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.Equal(t, "agent-ceo", sessions[0].AgentProfileID)
	assert.Equal(t, "profile-routed", sessions[0].ExecutionProfileID)
	assert.Equal(t, "profile-routed", launchedProfile(t, launched))
}

func TestResolvePreparedSessionDynamicRouteUsesLatestProfileForNonOfficeSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-current-profile", "session-current-profile", "step1")
	session, err := repo.GetTaskSession(ctx, "session-current-profile")
	require.NoError(t, err)
	session.AgentProfileID = "profile-current"
	session.ExecutionProfileID = "profile-current"
	require.NoError(t, repo.UpdateTaskSession(ctx, session))

	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
	svc.profileExecutionResolver = agentruntime.NewProfileExecutionResolver(staticProfileStore{}, nil, false)

	route, err := svc.resolvePreparedSessionDynamicRoute(ctx, session.ID, "profile-captured-before-start", false)
	require.NoError(t, err)
	assert.Equal(t, "profile-current", route.launchProfileID,
		"ordinary sessions must use the profile loaded from their current session")
}

func TestEnsureSession_OfficeTaskKeepsAgentProfileWithoutRoutedCandidate(t *testing.T) {
	for name, preview := range map[string]OfficeLaunchProfilePreviewFunc{
		"no candidate": func(context.Context, string, string) (string, error) { return "", nil },
		"preview error": func(context.Context, string, string) (string, error) {
			return "", errors.New("routing unavailable")
		},
	} {
		t.Run(name, func(t *testing.T) {
			svc, repo, launched := newOfficeEnsureService(t)
			svc.SetOfficeLaunchProfilePreview(preview)

			session := preparedOfficeSession(t, svc, repo)

			assert.Equal(t, "agent-ceo", session.AgentProfileID)
			assert.Equal(t, "agent-ceo", session.ExecutionProfileID)
			assert.Equal(t, "agent-ceo", launchedProfile(t, launched))
		})
	}
}
