package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"sort"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/improvekandev"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

type workflowStepCreatorFunc func(context.Context, string, string) error

func (f workflowStepCreatorFunc) CreateStepsFromTemplate(ctx context.Context, workflowID, templateID string) error {
	return f(ctx, workflowID, templateID)
}

func wireImproveKandevWorkflowPolicies(ts *OrchestratorTestServer) {
	ts.WorkflowSvc.SetSessionAccessChecker(ts.TaskSvc.AuthorizeSessionAccess)
	ts.WorkflowSvc.SetWorkflowAccessChecker(ts.TaskSvc.AuthorizeWorkflowAccess)
	ts.WorkflowSvc.SetWorkflowAuthorizationRequiredChecker(ts.TaskSvc.WorkflowAuthorizationRequired)
	ts.WorkflowSvc.SetWorkflowMutationReadOnlyChecker(ts.TaskSvc.WorkflowMutationReadOnly)
	ts.WorkflowSvc.SetWorkspaceMutationReadOnlyChecker(ts.TaskSvc.WorkspaceWorkflowMutationReadOnly)
	ts.WorkflowSvc.SetExecutorMutationChecker(ts.TaskSvc.AuthorizeWorkflowExecutorMutation)
	ts.WorkflowSvc.SetWorkspaceAccessChecker(ts.TaskSvc.AuthorizeWorkspaceAccess)
	ts.WorkflowSvc.SetTaskAccessChecker(ts.TaskSvc.AuthorizeTaskAccess)
}

func issueImproveBootstrap(t *testing.T, router http.Handler) (*httptest.ResponseRecorder, improvekandev.BootstrapResponse) {
	t.Helper()
	response, result, err := postImproveBootstrap(router)
	require.NoError(t, err)
	return response, result
}

func postImproveBootstrap(router http.Handler) (*httptest.ResponseRecorder, improvekandev.BootstrapResponse, error) {
	body, err := json.Marshal(improvekandev.BootstrapRequest{CreateWorkspace: true})
	if err != nil {
		return nil, improvekandev.BootstrapResponse{}, err
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/improve-kandev/bootstrap", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	var result improvekandev.BootstrapResponse
	if response.Code == http.StatusOK {
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			return response, improvekandev.BootstrapResponse{}, err
		}
	}
	return response, result, nil
}

func stepIDsByName(t *testing.T, ts *OrchestratorTestServer, workflowID string, expectedCount int) map[string]string {
	t.Helper()
	steps, err := ts.WorkflowSvc.ListStepsByWorkflow(context.Background(), workflowID)
	require.NoError(t, err)
	require.Len(t, steps, expectedCount, "workflow %s raw step row count", workflowID)
	ids := make(map[string]string, len(steps))
	for _, step := range steps {
		ids[step.Name] = step.ID
	}
	require.Len(t, ids, expectedCount, "workflow %s step names must be unique", workflowID)
	return ids
}

func TestImproveKandevBootstrapTemplateSteps(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	wireImproveKandevWorkflowPolicies(ts)
	t.Cleanup(ts.Close)

	repoPath := t.TempDir()
	require.NoError(t, exec.Command("git", "init", repoPath).Run())
	handler := improvekandev.NewHandler(ts.TaskSvc, improveKandevTestCloner{path: repoPath}, nil, nil, "test", ts.Logger)
	router := newImproveKandevTestRouter(handler)

	firstResponse, first := issueImproveBootstrap(t, router)
	require.Equal(t, http.StatusOK, firstResponse.Code, firstResponse.Body.String())
	require.NotEmpty(t, first.WorkflowID)
	require.NotEmpty(t, first.IssueWorkflowID)
	firstImproveIDs := stepIDsByName(t, ts, first.WorkflowID, 3)
	firstIssueIDs := stepIDsByName(t, ts, first.IssueWorkflowID, 1)
	require.ElementsMatch(t, []string{"Improve", "Test", "PR"}, mapKeys(firstImproveIDs))
	require.ElementsMatch(t, []string{"Open issue"}, mapKeys(firstIssueIDs))

	// Leave the improve workflow populated and clear the issue workflow to
	// exercise independent repair while preserving both workflow identities.
	_, err := ts.TaskRepo.DB().ExecContext(context.Background(), `DELETE FROM workflow_steps WHERE workflow_id = ?`, first.IssueWorkflowID)
	require.NoError(t, err)
	repairedStepIDs := make(chan string, 4)
	subscription, err := ts.EventBus.Subscribe(events.WorkflowStepCreated, func(_ context.Context, event *bus.Event) error {
		eventData, ok := event.Data.(map[string]interface{})
		if !ok {
			return nil
		}
		stepData, ok := eventData["step"].(map[string]interface{})
		if ok {
			if id, ok := stepData["id"].(string); ok {
				repairedStepIDs <- id
			}
		}
		return nil
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, subscription.Unsubscribe()) })

	secondResponse, second := issueImproveBootstrap(t, router)
	require.Equal(t, http.StatusOK, secondResponse.Code, secondResponse.Body.String())
	require.Equal(t, first.WorkflowID, second.WorkflowID)
	require.Equal(t, first.IssueWorkflowID, second.IssueWorkflowID)
	require.Equal(t, firstImproveIDs, stepIDsByName(t, ts, second.WorkflowID, 3), "populated steps must remain untouched")
	secondIssueIDs := stepIDsByName(t, ts, second.IssueWorkflowID, 1)
	require.ElementsMatch(t, []string{"Open issue"}, mapKeys(secondIssueIDs))
	require.NotEqual(t, firstIssueIDs["Open issue"], secondIssueIDs["Open issue"], "a repaired step receives a new identity")
	require.Equal(t, secondIssueIDs["Open issue"], <-repairedStepIDs, "repair publishes the inserted step")
	require.Empty(t, repairedStepIDs, "one repair publishes one step event")

	thirdResponse, third := issueImproveBootstrap(t, router)
	require.Equal(t, http.StatusOK, thirdResponse.Code, thirdResponse.Body.String())
	require.Equal(t, firstImproveIDs, stepIDsByName(t, ts, third.WorkflowID, 3))
	require.Equal(t, secondIssueIDs, stepIDsByName(t, ts, third.IssueWorkflowID, 1), "repeat bootstrap must not duplicate or replace steps")
}

func TestImproveKandevBootstrapReportsTemplateRepairErrorsAndRetries(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	wireImproveKandevWorkflowPolicies(ts)
	t.Cleanup(ts.Close)

	repoPath := t.TempDir()
	require.NoError(t, exec.Command("git", "init", repoPath).Run())
	handler := improvekandev.NewHandler(ts.TaskSvc, improveKandevTestCloner{path: repoPath}, nil, nil, "test", ts.Logger)
	router := newImproveKandevTestRouter(handler)

	firstResponse, first := issueImproveBootstrap(t, router)
	require.Equal(t, http.StatusOK, firstResponse.Code, firstResponse.Body.String())
	_, err := ts.TaskRepo.DB().ExecContext(context.Background(), `DELETE FROM workflow_steps WHERE workflow_id = ?`, first.IssueWorkflowID)
	require.NoError(t, err)
	wantErr := errors.New("template storage unavailable")
	ts.TaskSvc.SetWorkflowStepCreator(workflowStepCreatorFunc(func(context.Context, string, string) error {
		return wantErr
	}))

	failedResponse, _ := issueImproveBootstrap(t, router)
	require.Equal(t, http.StatusInternalServerError, failedResponse.Code, "bootstrap must not report an empty managed workflow as ready")
	require.Empty(t, stepIDsByName(t, ts, first.IssueWorkflowID, 0))

	ts.TaskSvc.SetWorkflowStepCreator(ts.WorkflowSvc)
	retryResponse, retry := issueImproveBootstrap(t, router)
	require.Equal(t, http.StatusOK, retryResponse.Code, retryResponse.Body.String())
	require.Equal(t, first.WorkflowID, retry.WorkflowID)
	require.Equal(t, first.IssueWorkflowID, retry.IssueWorkflowID)
	require.ElementsMatch(t, []string{"Open issue"}, mapKeys(stepIDsByName(t, ts, retry.IssueWorkflowID, 1)))
}

func TestImproveKandevBootstrapConcurrentRepairs(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	wireImproveKandevWorkflowPolicies(ts)
	t.Cleanup(ts.Close)

	repoPath := t.TempDir()
	require.NoError(t, exec.Command("git", "init", repoPath).Run())
	handler := improvekandev.NewHandler(ts.TaskSvc, improveKandevTestCloner{path: repoPath}, nil, nil, "test", ts.Logger)
	router := newImproveKandevTestRouter(handler)
	firstResponse, first := issueImproveBootstrap(t, router)
	require.Equal(t, http.StatusOK, firstResponse.Code, firstResponse.Body.String())
	_, err := ts.TaskRepo.DB().ExecContext(context.Background(), `DELETE FROM workflow_steps WHERE workflow_id = ?`, first.IssueWorkflowID)
	require.NoError(t, err)
	creatorStarted := make(chan struct{}, 2)
	creatorRelease := make(chan struct{})
	var creatorCalls atomic.Int32
	ts.TaskSvc.SetWorkflowStepCreator(workflowStepCreatorFunc(func(ctx context.Context, workflowID, templateID string) error {
		creatorCalls.Add(1)
		creatorStarted <- struct{}{}
		<-creatorRelease
		return ts.WorkflowSvc.CreateStepsFromTemplate(ctx, workflowID, templateID)
	}))
	t.Cleanup(func() {
		select {
		case <-creatorRelease:
		default:
			close(creatorRelease)
		}
	})

	type result struct {
		response *httptest.ResponseRecorder
		payload  improvekandev.BootstrapResponse
		err      error
	}
	results := make(chan result, 2)
	var requestCount atomic.Int32
	secondRequestStarted := make(chan struct{})
	overlapRouter := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requestCount.Add(1) == 2 {
			close(secondRequestStarted)
		}
		router.ServeHTTP(w, r)
	})
	runBootstrap := func() {
		response, payload, err := postImproveBootstrap(overlapRouter)
		results <- result{response: response, payload: payload, err: err}
	}
	go runBootstrap()
	<-creatorStarted
	go runBootstrap()
	<-secondRequestStarted
	close(creatorRelease)
	for range 2 {
		got := <-results
		require.NoError(t, got.err)
		require.Equal(t, http.StatusOK, got.response.Code, got.response.Body.String())
		require.Equal(t, first.WorkflowID, got.payload.WorkflowID)
		require.Equal(t, first.IssueWorkflowID, got.payload.IssueWorkflowID)
	}
	require.Equal(t, int32(1), creatorCalls.Load(), "overlapping bootstrap calls should repair the empty workflow once")
	steps, err := ts.WorkflowSvc.ListStepsByWorkflow(context.Background(), first.IssueWorkflowID)
	require.NoError(t, err)
	require.Len(t, steps, 1, "concurrent repairs must create exactly one raw step row")
	require.Equal(t, "Open issue", steps[0].Name)
}

func TestImproveKandevPublicTemplateMutationRemainsReadOnly(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	wireImproveKandevWorkflowPolicies(ts)
	t.Cleanup(ts.Close)
	ctx := context.Background()
	workspace, err := ts.TaskSvc.CreateWorkspace(ctx, &taskservice.CreateWorkspaceRequest{Name: "Improve Kandev"})
	require.NoError(t, err)
	templateID := "improve-kandev"
	workflow, err := ts.TaskSvc.CreateWorkflow(ctx, &taskservice.CreateWorkflowRequest{
		WorkspaceID:        workspace.ID,
		Name:               "Improve Kandev",
		WorkflowTemplateID: &templateID,
	})
	require.NoError(t, err)
	before := stepIDsByName(t, ts, workflow.ID, 3)
	require.ElementsMatch(t, []string{"Improve", "Test", "PR"}, mapKeys(before))

	request, err := http.NewRequest(http.MethodPost, ts.Server.URL+"/api/v1/workflows/"+workflow.ID+"/workflow/steps", bytes.NewBufferString(`{"template_id":"simple"}`))
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	response, err := ts.Server.Client().Do(request)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, response.Body.Close()) })
	require.Equal(t, http.StatusConflict, response.StatusCode)
	require.Equal(t, before, stepIDsByName(t, ts, workflow.ID, 3), "the public controller must keep its read-only guard")
}

func mapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
