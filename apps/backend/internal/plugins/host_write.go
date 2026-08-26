// host_write.go implements pluginHost's Host data API WRITE surface (ADR 0043
// phase 2): the CreateTask/UpdateTask methods on the task accessor
// (Host.Tasks()) and the Send method on the message accessor (Host.Messages()).
// Writes are gated on api_write:<resource> — undeclared → gRPC PermissionDenied,
// exactly mirroring the read gating in host_data.go.
//
// Every write routes through the same first-party layer the REST/MCP API uses,
// never a repository — that is how task.* events fire and how a message reaches
// the agent through the orchestrator's real delivery path (queue when the
// session is running, resume/start it otherwise). The service types can't be
// referenced here without an import cycle (see SetDataSources' doc), so task
// writes go through a narrow taskWriter interface and message delivery through a
// narrow taskMessenger interface, both satisfied by backendapp adapters.
package plugins

import (
	"context"
	"errors"
	"fmt"
	"strings"

	agentsettingsdto "github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/repoclone"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ── Narrow write data-source interfaces ─────────────────────────────────
//
// Both are satisfied by backendapp adapters (the service request types would
// create an import cycle here): taskWriter over internal/task/service,
// taskMessenger over the task service + orchestrator delivery path.

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
	Repositories   []pluginsdk.PluginTaskRepository
	Launch         TaskLaunchInput
	Metadata       map[string]any
	PlanMode       bool
	// StartAgent reports that the caller will launch an agent for this task
	// right after creation. The adapter maps it onto CreateTaskRequest so step
	// resolution sends the task to the first step that runs agents rather than
	// to the workflow's parking start step.
	StartAgent bool
}

// TaskLaunchInput contains validated launch fields for a freshly-created
// plugin task. The backendapp adapter translates it to the orchestrator's
// StartTask arguments without exposing the task service here.
type TaskLaunchInput struct {
	AgentProfileID    string
	ExecutorProfileID string
	Prompt            string
	PlanMode          bool
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

// PluginMessageResult is the outcome of delivering a message to a task session,
// returned by a taskMessenger adapter. Status is "queued" | "sent" | "started".
type PluginMessageResult struct {
	SessionID string
	Status    string
}

// taskWriter is the narrow slice of the task service the CreateTask/UpdateTask
// RPCs need, adapted by backendapp to avoid an internal/task/service import
// cycle. Both methods return the persisted *taskmodels.Task so the reader can
// map it to the wire DTO.
type taskWriter interface {
	CreateTask(ctx context.Context, in TaskCreateInput) (*taskmodels.Task, error)
	UpdateTask(ctx context.Context, in TaskUpdateInput) (*taskmodels.Task, error)
	DeleteTask(ctx context.Context, id string) error
}

// taskMessenger delivers a prompt to a task session through the orchestrator's
// real message-delivery path (resolve session, record the user message, then
// queue/resume/start by session state — the same behavior as message_task).
// Source is the "plugin:<id>" provenance stamped on the recorded message. An
// empty sessionID targets the task's primary session.
type taskMessenger interface {
	SendMessage(ctx context.Context, taskID, sessionID, text, source string) (PluginMessageResult, error)
}

// taskStarter is the narrow slice of the orchestrator the CreateTask RPC needs
// to honor start_agent, adapted by backendapp. Its StartTask returns promptly:
// the adapter launches the agent asynchronously (fire-and-forget on a detached,
// time-bounded context), so a plugin's CreateTask RPC never blocks on the
// orchestrator's launch path. Best-effort — a launch failure never fails task
// creation.
type taskStarter interface {
	StartTask(ctx context.Context, taskID string, launch TaskLaunchInput) error
}

// pluginSource is the "plugin:<id>" provenance stamped on rows this plugin
// creates, so a created task/message is attributable and a plugin can never
// spoof another origin.
const taskSourceMetadataKey = "source"

func (h *pluginHost) pluginSource() string {
	return "plugin:" + h.pluginID
}

// writeDependencies returns the live task messenger and task starter (wired via
// SetWriteDeps). Read live rather than snapshotted at hostForPlugin time for
// the same reason as the utility agent (ADR 0048): the orchestrator is
// constructed after StartActivePlugins spawns boot-active plugins, so a
// snapshot would strand those hosts. nil on a bare test host.
func (h *pluginHost) writeDependencies() (taskMessenger, taskStarter) {
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
	metadata, err := r.host.pluginTaskMetadata(in.Metadata)
	if err != nil {
		return nil, err
	}
	if err := r.host.validatePluginTaskRepositories(in.Repositories); err != nil {
		return nil, err
	}
	launch, err := taskLaunchInput(in.Launch)
	if err != nil {
		return nil, err
	}
	if in.StartAgent {
		if err := r.host.validateTaskLaunch(ctx, launch); err != nil {
			return nil, err
		}
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
		Repositories:   in.Repositories,
		Launch:         launch,
		Metadata:       metadata,
		PlanMode:       launch.PlanMode,
		StartAgent:     in.StartAgent,
	})
	if err != nil {
		return nil, err
	}
	if in.StartAgent {
		r.host.startTaskBestEffort(ctx, created.ID, launch)
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

// startTaskBestEffort asks the wired starter to launch an agent on the freshly
// created task when the plugin requested start_agent. Best-effort and
// non-fatal, matching the REST/MCP path's asynchronous auto-start: the starter
// returns promptly (it launches on a detached, time-bounded context — see the
// taskStarter doc), and a missing starter or a launch error just leaves the
// task on the board for a manual start.
func (h *pluginHost) startTaskBestEffort(ctx context.Context, taskID string, launch TaskLaunchInput) {
	_, starter := h.writeDependencies()
	if starter == nil {
		return
	}
	_ = starter.StartTask(ctx, taskID, launch)
}

// pluginTaskMetadata stamps immutable provenance and keeps plugin data under
// that provenance key. A plugin cannot replace source or write host metadata
// outside its own namespace.
func (h *pluginHost) pluginTaskMetadata(pluginMetadata map[string]any) (map[string]any, error) {
	if _, exists := pluginMetadata[taskSourceMetadataKey]; exists {
		return nil, invalidArgument("metadata.source is reserved")
	}
	metadata := map[string]any{taskSourceMetadataKey: h.pluginSource()}
	if len(pluginMetadata) > 0 {
		metadata[h.pluginSource()] = pluginMetadata
	}
	return metadata, nil
}

func (h *pluginHost) validatePluginTaskRepositories(repositories []pluginsdk.PluginTaskRepository) error {
	for _, repository := range repositories {
		if repository.RepositoryID != "" && repository.Remote != nil {
			return invalidArgument("repository_id and remote descriptor are mutually exclusive")
		}
		if repository.RepositoryID == "" && repository.Remote == nil {
			return invalidArgument("repository_id or remote descriptor is required")
		}
		if repository.Remote == nil {
			continue
		}
		if err := validateRemoteRepositoryDescriptor(repository.Remote); err != nil {
			return err
		}
		if !h.ownsRepositoryProvider(repository.Remote.ProviderID) {
			return statusPermissionDeniedProvider(repository.Remote.ProviderID)
		}
	}
	return nil
}

func validateRemoteRepositoryDescriptor(descriptor *pluginsdk.RemoteRepositoryDescriptor) error {
	if descriptor.ProviderID == "" || descriptor.ProviderHost == "" || descriptor.OwnerOrProject == "" ||
		descriptor.ProviderRepositoryID == "" || descriptor.Name == "" || descriptor.CloneURL == "" {
		return invalidArgument("remote repository descriptor is incomplete")
	}
	if err := repoclone.ValidateHTTPSCloneOrigin(descriptor.CloneURL, descriptor.ProviderHost); err != nil {
		return invalidArgument("remote repository clone_url must match its credential-free HTTPS provider origin")
	}
	return nil
}

func (h *pluginHost) ownsRepositoryProvider(provider string) bool {
	for _, declared := range h.repositoryProviders {
		if strings.EqualFold(provider, declared) {
			return true
		}
	}
	return false
}

func statusPermissionDeniedProvider(provider string) error {
	return permissionDenied(fmt.Sprintf("repository_provider:%s", provider))
}

func taskLaunchInput(launch *pluginsdk.PluginTaskLaunchOptions) (TaskLaunchInput, error) {
	if launch == nil {
		return TaskLaunchInput{}, nil
	}
	input := TaskLaunchInput{
		AgentProfileID:    strDeref(launch.AgentProfileID),
		ExecutorProfileID: strDeref(launch.ExecutorProfileID),
		Prompt:            strDeref(launch.Prompt),
	}
	if launch.PlanMode == nil || *launch.PlanMode == "" || strings.EqualFold(*launch.PlanMode, "off") {
		return input, nil
	}
	if strings.EqualFold(*launch.PlanMode, "replace") || strings.EqualFold(*launch.PlanMode, "on") {
		input.PlanMode = true
		return input, nil
	}
	return TaskLaunchInput{}, invalidArgument("launch.plan_mode must be one of: off, on, replace")
}

// validateTaskLaunch confirms that auto-start profile references exist before
// the task is created. This avoids a plugin creating a task that is guaranteed
// to fail its requested launch, while keeping launch failures after creation
// best-effort as documented by taskStarter.
func (h *pluginHost) validateTaskLaunch(ctx context.Context, launch TaskLaunchInput) error {
	if err := h.validateLaunchAgentProfile(ctx, launch.AgentProfileID); err != nil {
		return err
	}
	return h.validateLaunchExecutorProfile(ctx, launch.ExecutorProfileID)
}

func (h *pluginHost) validateLaunchAgentProfile(ctx context.Context, id string) error {
	if id == "" {
		return nil
	}
	if h.agentProfiles == nil {
		return status.Error(codes.Unimplemented, "agent profiles are unavailable")
	}
	response, err := h.agentProfiles.ListAgents(ctx)
	if err != nil {
		return err
	}
	if hasAgentProfile(response, id) {
		return nil
	}
	return invalidArgument("launch.agent_profile_id is not available")
}

func hasAgentProfile(response *agentsettingsdto.ListAgentsResponse, id string) bool {
	if response == nil {
		return false
	}
	for _, agent := range response.Agents {
		for _, profile := range agent.Profiles {
			if profile.ID == id {
				return true
			}
		}
	}
	return false
}

func (h *pluginHost) validateLaunchExecutorProfile(ctx context.Context, id string) error {
	if id == "" {
		return nil
	}
	if h.taskData == nil {
		return status.Error(codes.Unimplemented, "executor profiles are unavailable")
	}
	profiles, err := h.taskData.ListAllExecutorProfiles(ctx)
	if err != nil {
		return err
	}
	if hasExecutorProfile(profiles, id) {
		return nil
	}
	return invalidArgument("launch.executor_profile_id is not available")
}

func hasExecutorProfile(profiles []*taskmodels.ExecutorProfile, id string) bool {
	for _, profile := range profiles {
		if profile != nil && profile.ID == id {
			return true
		}
	}
	return false
}

// PluginOwnedTaskTrees returns a manager whose preview and delete operations
// require api_write:tasks and exact source provenance on every traversed node.
func (h *pluginHost) PluginOwnedTaskTrees() pluginsdk.PluginOwnedTaskTreeManager {
	return pluginOwnedTaskTreeManager{host: h}
}

type pluginOwnedTaskTreeManager struct{ host *pluginHost }

func (m pluginOwnedTaskTreeManager) Preview(ctx context.Context, rootTaskID string) ([]pluginsdk.Task, error) {
	tasks, err := m.host.pluginOwnedTaskTree(ctx, rootTaskID)
	if err != nil {
		return nil, err
	}
	return tasksToDTOs(tasks), nil
}

func (m pluginOwnedTaskTreeManager) Delete(ctx context.Context, rootTaskID string) ([]string, error) {
	tasks, err := m.host.pluginOwnedTaskTree(ctx, rootTaskID)
	if err != nil {
		// Deletion is a retry boundary. A prior attempt may have removed the
		// root before its caller persisted progress, so absence is success.
		if status.Code(err) == codes.NotFound {
			return []string{}, nil
		}
		return nil, err
	}
	if m.host.taskWriter == nil {
		return m.host.UnimplementedHostData.PluginOwnedTaskTrees().Delete(ctx, rootTaskID)
	}
	if err := m.host.rejectMixedOwnershipDelete(ctx, tasks); err != nil {
		return nil, err
	}
	deleted := make([]string, 0, len(tasks))
	deleteCtx := context.WithoutCancel(ctx)
	for index := len(tasks) - 1; index >= 0; index-- {
		if err := m.host.taskWriter.DeleteTask(deleteCtx, tasks[index].ID); err != nil {
			return deleted, fmt.Errorf("delete plugin-owned task tree after deleting %d task(s): %w", len(deleted), err)
		}
		deleted = append(deleted, tasks[index].ID)
	}
	return deleted, nil
}

func (h *pluginHost) rejectMixedOwnershipDelete(ctx context.Context, ownedTasks []*taskmodels.Task) error {
	if len(ownedTasks) == 0 {
		return nil
	}
	ownedIDs := make(map[string]struct{}, len(ownedTasks))
	for _, task := range ownedTasks {
		ownedIDs[task.ID] = struct{}{}
	}
	workspaceTasks, err := h.fetchTasksForWorkspaces(ctx, []string{ownedTasks[0].WorkspaceID}, true, true)
	if err != nil {
		return err
	}
	for _, task := range workspaceTasks {
		if _, parentWillBeDeleted := ownedIDs[task.ParentID]; !parentWillBeDeleted {
			continue
		}
		if _, childWillBeDeleted := ownedIDs[task.ID]; childWillBeDeleted {
			continue
		}
		return status.Error(
			codes.FailedPrecondition,
			"plugin-owned task tree contains adopted or user-owned children; detach them before deleting",
		)
	}
	return nil
}

func (h *pluginHost) pluginOwnedTaskTree(ctx context.Context, rootTaskID string) ([]*taskmodels.Task, error) {
	if !h.capabilities.CanWrite(resourceTasks) {
		return nil, permissionDenied(apiWriteCapability(resourceTasks))
	}
	if h.taskData == nil {
		_, err := h.UnimplementedHostData.PluginOwnedTaskTrees().Preview(ctx, rootTaskID)
		return nil, err
	}
	root, err := h.taskData.GetTask(ctx, rootTaskID)
	if err != nil {
		if errors.Is(err, repoerrors.ErrTaskNotFound) {
			return nil, taskNotFound(rootTaskID)
		}
		return nil, err
	}
	if taskSource(root) != h.pluginSource() {
		return nil, permissionDenied("plugin_owned_tasks")
	}
	tasks, err := h.fetchTasksForWorkspaces(ctx, []string{root.WorkspaceID}, true, true)
	if err != nil {
		return nil, err
	}
	children := make(map[string][]*taskmodels.Task)
	for _, task := range tasks {
		children[task.ParentID] = append(children[task.ParentID], task)
	}
	return ownedTaskSubtree(root, children, h.pluginSource()), nil
}

func ownedTaskSubtree(root *taskmodels.Task, children map[string][]*taskmodels.Task, source string) []*taskmodels.Task {
	if root == nil || taskSource(root) != source {
		return nil
	}
	out := []*taskmodels.Task{root}
	for _, child := range children[root.ID] {
		out = append(out, ownedTaskSubtree(child, children, source)...)
	}
	return out
}

func taskSource(task *taskmodels.Task) string {
	if task == nil || task.Metadata == nil {
		return ""
	}
	source, _ := task.Metadata[taskSourceMetadataKey].(string)
	return source
}

// ── Message send (api_write:messages) ───────────────────────────────────

func (r messageReader) Send(ctx context.Context, taskID, sessionID, text string) (*pluginsdk.MessageDispatch, error) {
	if !r.host.capabilities.CanWrite(resourceMessages) {
		return nil, permissionDenied(apiWriteCapability(resourceMessages))
	}
	messenger, _ := r.host.writeDependencies()
	if messenger == nil {
		return r.host.UnimplementedHostData.Messages().Send(ctx, taskID, sessionID, text)
	}
	if taskID == "" {
		return nil, invalidArgument("task_id is required")
	}
	if text == "" {
		return nil, invalidArgument("text is required")
	}
	result, err := messenger.SendMessage(ctx, taskID, sessionID, text, r.host.pluginSource())
	if err != nil {
		return nil, err
	}
	return &pluginsdk.MessageDispatch{SessionID: result.SessionID, Status: result.Status}, nil
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
