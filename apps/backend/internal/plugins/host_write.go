// host_write.go implements pluginHost's Host data API WRITE surface (ADR 0043
// phase 2): the CreateTask/UpdateTask methods on the task accessor
// (Host.Tasks()) and the CreateComment method behind Host.Comments(). Writes
// are gated on api_write:<resource> — undeclared → gRPC PermissionDenied,
// exactly mirroring the read gating in host_data.go.
//
// Every write routes through the same first-party service layer the REST/MCP
// API uses (internal/task/service for tasks, internal/office/service for
// comments), never a repository directly — that is how task.* / comment-created
// events fire and WS-driven UI stays in sync (apps/backend/CLAUDE.md: "any code
// path that mutates a task row must publish via the event bus"). The service
// types can't be referenced here without an import cycle (see SetDataSources'
// doc), so task writes go through a narrow taskWriter interface satisfied by a
// backendapp adapter, while comment writes use office/models.TaskComment
// directly (a leaf model package, no cycle) and are satisfied structurally by
// internal/office/service.Service.
package plugins

import (
	"context"
	"errors"
	"time"

	orchmodels "github.com/kandev/kandev/internal/office/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

// commentAuthorType is the author_type stamped on plugin-created comments.
// kandev's comment author vocabulary is user | agent; a plugin is a
// non-human author, so it posts as "agent" with a "plugin:<id>" source/author
// id carrying the real provenance.
const commentAuthorType = "agent"

// ── Narrow write data-source interfaces ─────────────────────────────────
//
// taskWriter is satisfied by a backendapp adapter over internal/task/service
// (the service request types would create an import cycle here, so the adapter
// translates plugins-local inputs into service.CreateTaskRequest/
// UpdateTaskRequest). commentDataSource is satisfied structurally by
// internal/office/service.Service.CreateComment. taskStarter is satisfied by a
// backendapp adapter over the orchestrator's StartTask.

// TaskCreateInput is the plugins-local task-create request a taskWriter adapter
// translates into internal/task/service.CreateTaskRequest. Source is the
// "plugin:<id>" provenance the host stamps (a plugin cannot set it).
type TaskCreateInput struct {
	WorkspaceID    string
	WorkflowID     string
	WorkflowStepID string
	Title          string
	Description    string
	ParentID       string
	Source         string
}

// TaskUpdateInput is the plugins-local task-update request a taskWriter adapter
// translates into internal/task/service.UpdateTaskRequest. A nil field means
// "leave unchanged".
type TaskUpdateInput struct {
	ID             string
	Title          *string
	Description    *string
	State          *string
	WorkflowStepID *string
}

// taskWriter is the narrow slice of the task service the CreateTask/UpdateTask
// RPCs need, adapted by backendapp to avoid an internal/task/service import
// cycle. Both methods return the persisted *taskmodels.Task so the reader can
// map it to the wire DTO.
type taskWriter interface {
	CreateTask(ctx context.Context, in TaskCreateInput) (*taskmodels.Task, error)
	UpdateTask(ctx context.Context, in TaskUpdateInput) (*taskmodels.Task, error)
}

// commentDataSource is the narrow slice of internal/office/service.Service the
// CreateComment RPC needs. CreateComment persists and publishes the
// comment-created event; it mutates the passed *TaskComment in place with the
// server-assigned id and created_at.
type commentDataSource interface {
	CreateComment(ctx context.Context, comment *orchmodels.TaskComment) error
}

// taskStarter is the narrow slice of the orchestrator the CreateTask RPC needs
// to honor start_agent, adapted by backendapp. Best-effort: a launch failure
// never fails task creation.
type taskStarter interface {
	StartTask(ctx context.Context, taskID string) error
}

// pluginSource is the "plugin:<id>" provenance stamped on rows this plugin
// creates, so a created task/comment is attributable and a plugin can never
// spoof another origin.
func (h *pluginHost) pluginSource() string {
	return "plugin:" + h.pluginID
}

// writeDependencies returns the live comment writer and task starter (wired via
// SetWriteDeps). Read live rather than snapshotted at hostForPlugin time for
// the same reason as the utility agent (ADR 0048): the office service and
// orchestrator are constructed after StartActivePlugins has spawned boot-active
// plugins, so a snapshot would strand those hosts. nil on a bare test host.
func (h *pluginHost) writeDependencies() (commentDataSource, taskStarter) {
	if h.writeDeps == nil {
		return nil, nil
	}
	return h.writeDeps()
}

// ── Task writes (api_write:tasks) ───────────────────────────────────────

func (r taskReader) Create(ctx context.Context, in pluginsdk.CreateTaskInput) (*pluginsdk.Task, error) {
	if !r.host.capabilities.CanWrite(resourceTasks) {
		return nil, permissionDenied(apiWriteCapability(resourceTasks))
	}
	if r.host.taskWriter == nil {
		return r.host.UnimplementedHostData.Tasks().Create(ctx, in)
	}
	if in.Title == "" {
		return nil, invalidArgument("title is required")
	}
	workspaceID, workflowID, err := r.host.resolveCreatePlacement(ctx, in)
	if err != nil {
		return nil, err
	}
	created, err := r.host.taskWriter.CreateTask(ctx, TaskCreateInput{
		WorkspaceID:    workspaceID,
		WorkflowID:     workflowID,
		WorkflowStepID: strDeref(in.WorkflowStepID),
		Title:          in.Title,
		Description:    in.Description,
		ParentID:       strDeref(in.ParentID),
		Source:         r.host.pluginSource(),
	})
	if err != nil {
		return nil, err
	}
	if in.StartAgent {
		r.host.startTaskBestEffort(ctx, created.ID)
	}
	dto := taskModelToDTO(created)
	return &dto, nil
}

func (r taskReader) Update(ctx context.Context, in pluginsdk.UpdateTaskInput) (*pluginsdk.Task, error) {
	if !r.host.capabilities.CanWrite(resourceTasks) {
		return nil, permissionDenied(apiWriteCapability(resourceTasks))
	}
	if r.host.taskWriter == nil {
		return r.host.UnimplementedHostData.Tasks().Update(ctx, in)
	}
	if in.ID == "" {
		return nil, invalidArgument("id is required")
	}
	updated, err := r.host.taskWriter.UpdateTask(ctx, TaskUpdateInput{
		ID:             in.ID,
		Title:          in.Title,
		Description:    in.Description,
		State:          in.State,
		WorkflowStepID: in.WorkflowStepID,
	})
	if err != nil {
		if errors.Is(err, repoerrors.ErrTaskNotFound) {
			return nil, taskNotFound(in.ID)
		}
		return nil, err
	}
	dto := taskModelToDTO(updated)
	return &dto, nil
}

// resolveCreatePlacement fills the workspace and workflow a created task lands
// in when the plugin leaves them empty, mirroring the REST/MCP "sane default"
// behavior: an empty workspace_id resolves to the single workspace (ambiguous
// when there are zero or many → InvalidArgument), and an empty workflow_id
// resolves to that workspace's first workflow. Both defaulters need the read
// data sources; if those aren't wired the plugin must pass the ids explicitly.
func (h *pluginHost) resolveCreatePlacement(ctx context.Context, in pluginsdk.CreateTaskInput) (string, string, error) {
	workspaceID := in.WorkspaceID
	if workspaceID == "" {
		resolved, err := h.defaultWorkspaceID(ctx)
		if err != nil {
			return "", "", err
		}
		workspaceID = resolved
	}
	workflowID := in.WorkflowID
	if workflowID == "" {
		resolved, err := h.defaultWorkflowID(ctx, workspaceID)
		if err != nil {
			return "", "", err
		}
		workflowID = resolved
	}
	return workspaceID, workflowID, nil
}

func (h *pluginHost) defaultWorkspaceID(ctx context.Context) (string, error) {
	if h.taskData == nil {
		return "", invalidArgument("workspace_id is required")
	}
	workspaces, err := h.taskData.ListWorkspaces(ctx)
	if err != nil {
		return "", err
	}
	if len(workspaces) != 1 {
		return "", invalidArgument("workspace_id is required: cannot resolve a default among the instance's workspaces")
	}
	return workspaces[0].ID, nil
}

func (h *pluginHost) defaultWorkflowID(ctx context.Context, workspaceID string) (string, error) {
	if h.workflows == nil {
		return "", invalidArgument("workflow_id is required")
	}
	workflows, err := h.workflows.ListWorkflows(ctx, workspaceID, false)
	if err != nil {
		return "", err
	}
	if len(workflows) == 0 {
		return "", invalidArgument("workflow_id is required: workspace has no workflow to default to")
	}
	// ListWorkflows returns workflows in sort order, so the first is the
	// workspace's default landing workflow.
	return workflows[0].ID, nil
}

// startTaskBestEffort launches an agent on the freshly created task when the
// plugin requested start_agent. Best-effort and non-fatal, matching the
// REST/MCP path's asynchronous auto-start: a missing starter (orchestrator not
// wired) or a launch error leaves the task on the board for a manual start.
func (h *pluginHost) startTaskBestEffort(ctx context.Context, taskID string) {
	_, starter := h.writeDependencies()
	if starter == nil {
		return
	}
	_ = starter.StartTask(ctx, taskID)
}

// ── Comment writes (api_write:comments) ─────────────────────────────────

func (h *pluginHost) Comments() pluginsdk.CommentWriter {
	if !h.capabilities.CanWrite(resourceComments) {
		return deniedCommentWriter{}
	}
	if comments, _ := h.writeDependencies(); comments == nil {
		return h.UnimplementedHostData.Comments()
	}
	return commentWriter{host: h}
}

type deniedCommentWriter struct{}

func (deniedCommentWriter) Create(context.Context, string, string) (*pluginsdk.Comment, error) {
	return nil, permissionDenied(apiWriteCapability(resourceComments))
}

type commentWriter struct{ host *pluginHost }

func (w commentWriter) Create(ctx context.Context, taskID, body string) (*pluginsdk.Comment, error) {
	if taskID == "" {
		return nil, invalidArgument("task_id is required")
	}
	if body == "" {
		return nil, invalidArgument("body is required")
	}
	comments, _ := w.host.writeDependencies()
	source := w.host.pluginSource()
	comment := &orchmodels.TaskComment{
		TaskID:     taskID,
		AuthorType: commentAuthorType,
		AuthorID:   source,
		Body:       body,
		Source:     source,
	}
	if err := comments.CreateComment(ctx, comment); err != nil {
		return nil, err
	}
	return &pluginsdk.Comment{
		ID:        comment.ID,
		TaskID:    comment.TaskID,
		Body:      comment.Body,
		Source:    comment.Source,
		CreatedAt: comment.CreatedAt.UTC().Format(time.RFC3339),
	}, nil
}

// strDeref returns the pointed-to string, or "" when the pointer is nil —
// translating the SDK's *string "absent" convention into the plain strings the
// task service's CreateTaskRequest uses.
func strDeref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
