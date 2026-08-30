package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type CIRunFailureClass string

const (
	CIRunFailureNotAuthorized            CIRunFailureClass = "not_authorized"
	CIRunFailureCrossWorkspace           CIRunFailureClass = "cross_workspace"
	CIRunFailureTaskMismatch             CIRunFailureClass = "task_mismatch"
	CIRunFailureIdempotencyConflict      CIRunFailureClass = "idempotency_conflict"
	CIRunFailureWorkflowStepMismatch     CIRunFailureClass = "workflow_step_mismatch"
	CIRunFailureUnlinkedPR               CIRunFailureClass = "unlinked_pr"
	CIRunFailureRepositoryMismatch       CIRunFailureClass = "repository_mismatch"
	CIRunFailureHeadDrift                CIRunFailureClass = "head_drift"
	CIRunFailureSourceRunMismatch        CIRunFailureClass = "source_run_mismatch"
	CIRunFailureRerunIneligible          CIRunFailureClass = "rerun_ineligible"
	CIRunFailureDispatchDenied           CIRunFailureClass = "workflow_dispatch_denied"
	CIRunFailureMergeEvidenceUnavailable CIRunFailureClass = "merge_evidence_unavailable"
	CIRunFailureInstallationPermission   CIRunFailureClass = "installation_permission_denied"
	CIRunFailureProviderRateLimited      CIRunFailureClass = "provider_rate_limited"
	CIRunFailureProviderUnavailable      CIRunFailureClass = "provider_unavailable"
	CIRunFailureProviderCallAmbiguous    CIRunFailureClass = "provider_call_ambiguous"
	CIRunFailureProviderRejected         CIRunFailureClass = "provider_rejected"
)

type CIRunProviderError struct {
	Class      CIRunFailureClass
	StatusCode int
	Retryable  bool
}

func (e *CIRunProviderError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("GitHub Actions request failed (%s, HTTP %d)", e.Class, e.StatusCode)
	}
	return fmt.Sprintf("GitHub Actions request failed (%s)", e.Class)
}

type GitHubActionsRun struct {
	ID             int64
	Attempt        int
	WorkflowID     int64
	WorkflowName   string
	WorkflowPath   string
	Event          string
	Status         string
	Conclusion     string
	HeadSHA        string
	HeadBranch     string
	Repository     string
	HeadRepository string
	PullRequests   []int
}

type GitHubActionsWorkflow struct {
	ID    int64
	Name  string
	Path  string
	State string
}

type actionsRunResponse struct {
	ID         int64  `json:"id"`
	RunAttempt int    `json:"run_attempt"`
	WorkflowID int64  `json:"workflow_id"`
	Name       string `json:"name"`
	Path       string `json:"path"`
	Event      string `json:"event"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	HeadSHA    string `json:"head_sha"`
	HeadBranch string `json:"head_branch"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
	HeadRepository struct {
		FullName string `json:"full_name"`
	} `json:"head_repository"`
	PullRequests []struct {
		Number int `json:"number"`
	} `json:"pull_requests"`
}

func projectActionsRun(raw actionsRunResponse) *GitHubActionsRun {
	run := &GitHubActionsRun{
		ID: raw.ID, Attempt: raw.RunAttempt, WorkflowID: raw.WorkflowID,
		WorkflowName: raw.Name, WorkflowPath: raw.Path, Event: raw.Event,
		Status: raw.Status, Conclusion: raw.Conclusion, HeadSHA: raw.HeadSHA,
		HeadBranch: raw.HeadBranch, Repository: raw.Repository.FullName,
		HeadRepository: raw.HeadRepository.FullName,
		PullRequests:   make([]int, 0, len(raw.PullRequests)),
	}
	for _, pr := range raw.PullRequests {
		run.PullRequests = append(run.PullRequests, pr.Number)
	}
	return run
}

func (c *TokenClient) GetActionsRun(
	ctx context.Context, owner, repo string, runID int64,
) (*GitHubActionsRun, error) {
	var raw actionsRunResponse
	endpoint := fmt.Sprintf("/repos/%s/%s/actions/runs/%d", owner, repo, runID)
	if err := c.get(ctx, endpoint, &raw); err != nil {
		return nil, classifyCIRunProviderError(err, false, false)
	}
	return projectActionsRun(raw), nil
}

func (c *TokenClient) GetActionsWorkflow(
	ctx context.Context, owner, repo string, workflowID int64,
) (*GitHubActionsWorkflow, error) {
	var raw struct {
		ID    int64  `json:"id"`
		Name  string `json:"name"`
		Path  string `json:"path"`
		State string `json:"state"`
	}
	endpoint := fmt.Sprintf("/repos/%s/%s/actions/workflows/%d", owner, repo, workflowID)
	if err := c.get(ctx, endpoint, &raw); err != nil {
		return nil, classifyCIRunProviderError(err, false, false)
	}
	return &GitHubActionsWorkflow{ID: raw.ID, Name: raw.Name, Path: raw.Path, State: raw.State}, nil
}

func (c *TokenClient) RerunFailedActionsJobs(
	ctx context.Context, owner, repo string, runID int64,
) error {
	endpoint := fmt.Sprintf("/repos/%s/%s/actions/runs/%d/rerun-failed-jobs", owner, repo, runID)
	err := c.postJSON(ctx, endpoint, []byte(`{}`), nil)
	return classifyCIRunProviderError(err, true, true)
}

func (c *TokenClient) DispatchActionsWorkflow(
	ctx context.Context,
	owner, repo string,
	workflowID int64,
	ref string,
	inputs map[string]string,
) error {
	payload, err := json.Marshal(struct {
		Ref    string            `json:"ref"`
		Inputs map[string]string `json:"inputs,omitempty"`
	}{Ref: ref, Inputs: inputs})
	if err != nil {
		return err
	}
	endpoint := fmt.Sprintf("/repos/%s/%s/actions/workflows/%d/dispatches", owner, repo, workflowID)
	return classifyCIRunProviderError(c.postJSON(ctx, endpoint, payload, nil), true, false)
}

func (c *TokenClient) ListActionsWorkflowRuns(
	ctx context.Context,
	owner, repo string,
	workflowID int64,
	headSHA string,
) ([]GitHubActionsRun, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/actions/workflows/%d/runs?head_sha=%s&per_page=20",
		owner, repo, workflowID, url.QueryEscape(headSHA))
	var raw struct {
		Runs []actionsRunResponse `json:"workflow_runs"`
	}
	if err := c.get(ctx, endpoint, &raw); err != nil {
		return nil, classifyCIRunProviderError(err, false, false)
	}
	runs := make([]GitHubActionsRun, 0, len(raw.Runs))
	for _, item := range raw.Runs {
		runs = append(runs, *projectActionsRun(item))
	}
	return runs, nil
}

func classifyCIRunProviderError(err error, mutation, rerun bool) error {
	if err == nil {
		return nil
	}
	var apiErr *GitHubAPIError
	if !errors.As(err, &apiErr) {
		if mutation {
			return &CIRunProviderError{Class: CIRunFailureProviderCallAmbiguous, Retryable: false}
		}
		return &CIRunProviderError{Class: CIRunFailureProviderUnavailable, Retryable: true}
	}
	body := strings.ToLower(apiErr.Body)
	switch {
	case rerun && apiErr.StatusCode == http.StatusUnprocessableEntity:
		return &CIRunProviderError{Class: CIRunFailureRerunIneligible, StatusCode: apiErr.StatusCode}
	case apiErr.StatusCode == http.StatusTooManyRequests ||
		(apiErr.StatusCode == http.StatusForbidden && strings.Contains(body, "rate limit")):
		return &CIRunProviderError{Class: CIRunFailureProviderRateLimited, StatusCode: apiErr.StatusCode, Retryable: true}
	case apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden:
		return &CIRunProviderError{Class: CIRunFailureInstallationPermission, StatusCode: apiErr.StatusCode}
	case apiErr.StatusCode >= http.StatusInternalServerError:
		return &CIRunProviderError{Class: CIRunFailureProviderUnavailable, StatusCode: apiErr.StatusCode, Retryable: true}
	default:
		return &CIRunProviderError{Class: CIRunFailureProviderRejected, StatusCode: apiErr.StatusCode}
	}
}
