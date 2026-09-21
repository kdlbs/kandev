package backendapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/authz"
	shared "github.com/kandev/kandev/internal/orchestration/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	workflowservice "github.com/kandev/kandev/internal/workflow/service"
	"github.com/kandev/kandev/internal/workflow/stepevents"
)

type workspaceAdminAdapter struct {
	*taskCreatorAdapter
	workflows  *workflowservice.Service
	stepEvents *stepevents.Publisher
}

type workspaceAdminCommand struct {
	Resource      string          `json:"resource"`
	Action        string          `json:"action"`
	ID            string          `json:"id,omitempty"`
	WorkflowID    string          `json:"workflow_id,omitempty"`
	IDs           []string        `json:"ids,omitempty"`
	Configuration json.RawMessage `json:"configuration,omitempty"`
}

const (
	adminWorkspace  = "workspace"
	adminWorkflow   = "workflow"
	adminStep       = "step"
	adminRepository = "repository"
	adminCreate     = "create"
	adminUpdate     = "update"
	adminDelete     = "delete"
	adminReorder    = "reorder"
)

func (a *workspaceAdminAdapter) ManageWorkspace(ctx context.Context, workspace string, raw json.RawMessage) (any, error) {
	var command workspaceAdminCommand
	if err := decodeWorkspaceConfiguration(raw, &command); err != nil {
		return nil, err
	}
	if err := a.taskSvc.AuthorizeWorkspaceScope(ctx, workspace, authz.ScopeWorkspaceManage); err != nil {
		return nil, err
	}
	ws, err := a.taskSvc.GetWorkspace(ctx, workspace)
	if err != nil {
		return nil, err
	}
	if ws.IsImproveKandev() {
		return nil, workflowservice.ErrWorkflowWorkspaceReadOnly
	}
	if err := shared.CheckWorkspaceEffect(ctx); err != nil {
		return nil, err
	}
	switch command.Resource {
	case adminWorkspace:
		return a.updateWorkspaceConfiguration(ctx, workspace, command)
	case adminWorkflow:
		return a.manageWorkflowConfiguration(ctx, workspace, command)
	case adminStep:
		return a.manageStepConfiguration(ctx, workspace, command)
	case adminRepository:
		return a.manageRepositoryConfiguration(ctx, workspace, command)
	default:
		return nil, fmt.Errorf("resource must be workspace, workflow, step or repository")
	}
}

func decodeWorkspaceConfiguration(raw json.RawMessage, target any) error {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.TrimSpace(raw)[0] != '{' {
		return fmt.Errorf("configuration must be a JSON object")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func (a *workspaceAdminAdapter) updateWorkspaceConfiguration(ctx context.Context, workspace string, cmd workspaceAdminCommand) (any, error) {
	if cmd.Action != adminUpdate || (cmd.ID != "" && cmd.ID != workspace) {
		return nil, fmt.Errorf("only update of the assigned workspace is supported")
	}
	var req taskservice.UpdateWorkspaceRequest
	if err := decodeWorkspaceConfiguration(cmd.Configuration, &req); err != nil {
		return nil, err
	}
	if req.UnitID != nil || req.Visibility != nil {
		return nil, fmt.Errorf("workspace access changes require a human administrator")
	}
	if err := a.validateWorkspaceDefaults(ctx, workspace, req); err != nil {
		return nil, err
	}
	if req.Name != nil && strings.TrimSpace(*req.Name) == "" {
		return nil, fmt.Errorf("name must not be empty")
	}
	return a.taskSvc.UpdateWorkspace(ctx, workspace, &req)
}

func (a *workspaceAdminAdapter) validateWorkspaceDefaults(ctx context.Context, workspace string, req taskservice.UpdateWorkspaceRequest) error {
	for _, profile := range []*string{req.DefaultAgentProfileID, req.DefaultConfigAgentProfileID} {
		if err := a.validateWorkspaceProfile(ctx, workspace, profile); err != nil {
			return err
		}
	}
	if req.DefaultExecutorID != nil && *req.DefaultExecutorID != "" {
		if _, err := a.taskSvc.GetExecutor(ctx, *req.DefaultExecutorID); err != nil {
			return err
		}
	}
	if req.DefaultEnvironmentID != nil && *req.DefaultEnvironmentID != "" {
		if _, err := a.taskSvc.GetEnvironment(ctx, *req.DefaultEnvironmentID); err != nil {
			return err
		}
	}
	return nil
}

func (a *workspaceAdminAdapter) validateWorkspaceProfile(ctx context.Context, workspace string, id *string) error {
	if id == nil || strings.TrimSpace(*id) == "" {
		return nil
	}
	_, err := a.directWorkerProfile(ctx, shared.WorkspaceTaskSpec{WorkspaceID: workspace, AssigneeID: *id}, map[string]interface{}{})
	return err
}

func (a *workspaceAdminAdapter) WorkspaceCatalog(ctx context.Context, workspace string) (any, error) {
	result, err := a.taskCreatorAdapter.WorkspaceCatalog(ctx, workspace)
	if err != nil {
		return nil, err
	}
	ws, err := a.taskSvc.GetWorkspace(ctx, workspace)
	if err != nil {
		return nil, err
	}
	templates, err := a.workflows.ListTemplates(ctx)
	if err != nil {
		return nil, err
	}
	catalog := result.(map[string]any)
	catalog["workspace"] = ws
	catalog["workflow_templates"] = templates
	return catalog, nil
}
