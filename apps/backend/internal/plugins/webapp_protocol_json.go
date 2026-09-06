package plugins

import (
	"encoding/json"
	"net/http"

	"github.com/kandev/kandev/pkg/pluginsdk"
)

type webAppPageInfo struct {
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    bool   `json:"has_more"`
}

type webAppPage[T any] struct {
	Items    []T            `json:"items"`
	PageInfo webAppPageInfo `json:"page_info"`
}

func webAppPageFromSDK[T any](items []T, info *pluginsdk.PageInfo) webAppPage[T] {
	page := webAppPage[T]{Items: items}
	if info != nil {
		page.PageInfo = webAppPageInfo{NextCursor: info.NextCursor, HasMore: info.HasMore}
	}
	return page
}

func writeWebAppJSON(w http.ResponseWriter, r *http.Request, status int, value any) {
	body, err := json.Marshal(value)
	if err != nil {
		writeWebAppError(w, http.StatusInternalServerError, webAppRuntimeUnavailable)
		return
	}
	if len(body) > webAppResponseLimit {
		writeWebAppError(w, http.StatusInternalServerError, "response_too_large")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if r == nil || r.Method != http.MethodHead {
		_, _ = w.Write(body)
	}
}

func writeWebAppError(w http.ResponseWriter, status int, code string) {
	body, err := json.Marshal(struct {
		Error string `json:"error"`
	}{Error: code})
	if err != nil {
		body = []byte(`{"error":"runtime_unavailable"}`)
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

type webAppTaskRepository struct {
	ID             string `json:"id"`
	RepositoryID   string `json:"repository_id"`
	BaseBranch     string `json:"base_branch"`
	Position       int32  `json:"position"`
	CheckoutBranch string `json:"checkout_branch"`
}

type webAppPullRequest struct {
	Number                  int64   `json:"number"`
	URL                     string  `json:"url"`
	Title                   string  `json:"title"`
	State                   string  `json:"state"`
	HeadBranch              string  `json:"head_branch"`
	BaseBranch              string  `json:"base_branch"`
	IsDraft                 bool    `json:"is_draft"`
	Provider                string  `json:"provider"`
	MergedAt                *string `json:"merged_at,omitempty"`
	ClosedAt                *string `json:"closed_at,omitempty"`
	ReviewState             string  `json:"review_state"`
	ChecksState             string  `json:"checks_state"`
	MergeableState          string  `json:"mergeable_state"`
	UnresolvedReviewThreads int32   `json:"unresolved_review_threads"`
	ChecksTotal             int32   `json:"checks_total"`
	ChecksPassing           int32   `json:"checks_passing"`
	Additions               int32   `json:"additions"`
	Deletions               int32   `json:"deletions"`
	AuthorLogin             string  `json:"author_login"`
}

type webAppTask struct {
	ID                     string                 `json:"id"`
	WorkspaceID            string                 `json:"workspace_id"`
	WorkflowID             string                 `json:"workflow_id"`
	Title                  string                 `json:"title"`
	Description            string                 `json:"description"`
	State                  string                 `json:"state"`
	Priority               string                 `json:"priority"`
	CreatedBy              string                 `json:"created_by"`
	CreatedAt              string                 `json:"created_at"`
	UpdatedAt              string                 `json:"updated_at"`
	StartedAt              *string                `json:"started_at,omitempty"`
	CompletedAt            *string                `json:"completed_at,omitempty"`
	ParentID               *string                `json:"parent_id,omitempty"`
	Identifier             string                 `json:"identifier"`
	IsEphemeral            bool                   `json:"is_ephemeral"`
	Repositories           []webAppTaskRepository `json:"repositories,omitempty"`
	Metadata               map[string]any         `json:"metadata,omitempty"`
	ArchivedAt             *string                `json:"archived_at,omitempty"`
	PullRequests           []webAppPullRequest    `json:"pull_requests,omitempty"`
	WorkflowStepID         string                 `json:"workflow_step_id"`
	Position               int32                  `json:"position"`
	AssigneeAgentProfileID string                 `json:"assignee_agent_profile_id"`
	Labels                 []string               `json:"labels,omitempty"`
	Autopilot              bool                   `json:"autopilot"`
	WIPAdmitted            bool                   `json:"wip_admitted"`
	QueuedForStepID        string                 `json:"queued_for_step_id,omitempty"`
	QueuedAt               *string                `json:"queued_at,omitempty"`
	ProjectID              string                 `json:"project_id,omitempty"`
	ExternalID             string                 `json:"external_id,omitempty"`
}

func webAppTaskFromSDK(task pluginsdk.Task) webAppTask {
	result := webAppTask{
		ID:                     task.ID,
		WorkspaceID:            task.WorkspaceID,
		WorkflowID:             task.WorkflowID,
		Title:                  task.Title,
		Description:            task.Description,
		State:                  task.State,
		Priority:               task.Priority,
		CreatedBy:              task.CreatedBy,
		CreatedAt:              task.CreatedAt,
		UpdatedAt:              task.UpdatedAt,
		StartedAt:              task.StartedAt,
		CompletedAt:            task.CompletedAt,
		ParentID:               task.ParentID,
		Identifier:             task.Identifier,
		IsEphemeral:            task.IsEphemeral,
		Metadata:               task.Metadata,
		ArchivedAt:             task.ArchivedAt,
		WorkflowStepID:         task.WorkflowStepID,
		Position:               task.Position,
		AssigneeAgentProfileID: task.AssigneeAgentProfileID,
		Labels:                 task.Labels,
		Autopilot:              task.Autopilot,
		WIPAdmitted:            task.WIPAdmitted,
		QueuedForStepID:        task.QueuedForStepID,
		QueuedAt:               task.QueuedAt,
		ProjectID:              task.ProjectID,
		ExternalID:             task.ExternalID,
	}
	if len(task.Repositories) > 0 {
		result.Repositories = make([]webAppTaskRepository, len(task.Repositories))
		for i, repository := range task.Repositories {
			result.Repositories[i] = webAppTaskRepository{
				ID: repository.ID, RepositoryID: repository.RepositoryID,
				BaseBranch: repository.BaseBranch, Position: repository.Position,
				CheckoutBranch: repository.CheckoutBranch,
			}
		}
	}
	if len(task.PullRequests) > 0 {
		result.PullRequests = make([]webAppPullRequest, len(task.PullRequests))
		for i, pullRequest := range task.PullRequests {
			result.PullRequests[i] = webAppPullRequest{
				Number: pullRequest.Number, URL: pullRequest.URL, Title: pullRequest.Title,
				State: pullRequest.State, HeadBranch: pullRequest.HeadBranch,
				BaseBranch: pullRequest.BaseBranch, IsDraft: pullRequest.IsDraft,
				Provider: pullRequest.Provider, MergedAt: pullRequest.MergedAt,
				ClosedAt: pullRequest.ClosedAt, ReviewState: pullRequest.ReviewState,
				ChecksState: pullRequest.ChecksState, MergeableState: pullRequest.MergeableState,
				UnresolvedReviewThreads: pullRequest.UnresolvedReviewThreads,
				ChecksTotal:             pullRequest.ChecksTotal, ChecksPassing: pullRequest.ChecksPassing,
				Additions: pullRequest.Additions, Deletions: pullRequest.Deletions,
				AuthorLogin: pullRequest.AuthorLogin,
			}
		}
	}
	return result
}

type webAppWorkflow struct {
	ID          string  `json:"id"`
	WorkspaceID string  `json:"workspace_id"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	SortOrder   int32   `json:"sort_order"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

func webAppWorkflowFromSDK(workflow pluginsdk.Workflow) webAppWorkflow {
	return webAppWorkflow{
		ID: workflow.ID, WorkspaceID: workflow.WorkspaceID, Name: workflow.Name,
		Description: workflow.Description, SortOrder: workflow.SortOrder,
		CreatedAt: workflow.CreatedAt, UpdatedAt: workflow.UpdatedAt,
	}
}

type webAppWorkflowStep struct {
	ID                 string   `json:"id"`
	WorkflowID         string   `json:"workflow_id"`
	Name               string   `json:"name"`
	Position           int32    `json:"position"`
	StageType          string   `json:"stage_type"`
	Color              string   `json:"color"`
	IsStartStep        bool     `json:"is_start_step"`
	WIPLimit           int32    `json:"wip_limit"`
	AgentProfileID     string   `json:"agent_profile_id"`
	OnEnterActionTypes []string `json:"on_enter_action_types,omitempty"`
}

func webAppWorkflowStepFromSDK(step pluginsdk.WorkflowStep) webAppWorkflowStep {
	return webAppWorkflowStep{
		ID: step.ID, WorkflowID: step.WorkflowID, Name: step.Name,
		Position: step.Position, StageType: step.StageType, Color: step.Color,
		IsStartStep: step.IsStartStep, WIPLimit: step.WIPLimit,
		AgentProfileID: step.AgentProfileID, OnEnterActionTypes: step.OnEnterActionTypes,
	}
}
