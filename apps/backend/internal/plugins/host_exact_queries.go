//nolint:revive // Keep related exact Host query projections together while their shared cursor contract is evolving.
package plugins

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/plugins/state"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/task/statussummary"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	exactReadListWorkspaces          = "ListWorkspacesExact"
	exactReadListWorkflows           = "ListWorkflowsExact"
	exactReadListWorkflowSteps       = "ListWorkflowStepsExact"
	exactReadListTasks               = "ListTasksExact"
	exactReadGetTask                 = "GetTaskExact"
	exactReadListSessions            = "ListSessionsExact"
	exactReadListInteractions        = "ListPendingInteractionsExact"
	exactReadGetInteraction          = "GetInteractionExact"
	exactReadListMessages            = "ListSanitizedMessagesExact"
	exactReadListTaskRelations       = "ListTaskRelationsExact"
	exactReadGetTaskRelations        = "GetTaskRelationsExact"
	exactReadListTaskInbox           = "ListTaskInboxExact"
	exactReadGetTaskDirective        = "GetTaskDirectiveExact"
	exactReadListTaskDirectives      = "ListTaskDirectivesExact"
	exactReadListTaskTransitions     = "ListPendingTaskTransitionsExact"
	exactReadChangeEvidence          = "ListChangeRequestEvidenceExact"
	exactReadTaskUsage               = "ListTaskUsageExact"
	exactReadSourceIssueCapabilities = "GetSourceIssueCapabilitiesExact"
)

var exactReadMethods = []exactMethod{
	{method: exactReadListWorkspaces, capability: "host.v2.read:workspaces", description: "Read one workspace catalog snapshot"},
	{method: exactReadListWorkflows, capability: "host.v2.read:workflows", description: "Read a workspace workflow snapshot"},
	{method: exactReadListWorkflowSteps, capability: "host.v2.read:workflows", description: "Read one workflow step snapshot"},
	{method: exactReadListTasks, capability: "host.v2.read:tasks", description: "Read canonical workspace task projections"},
	{method: exactReadGetTask, capability: "host.v2.read:tasks", description: "Read one workspace task projection"},
	{method: exactReadListSessions, capability: "host.v2.read:sessions", description: "Read workspace session projections"},
	{method: exactReadListInteractions, capability: "host.v2.read:interactions", description: "Read workspace human interactions"},
	{method: exactReadGetInteraction, capability: "host.v2.read:interactions", description: "Read one workspace human interaction"},
	{method: exactReadListMessages, capability: "host.v2.read:messages", description: "Read sanitized workspace messages"},
	{method: exactReadListTaskInbox, capability: "host.v2.read:task_inbox", description: "Read workspace task inbox entries"},
	{method: exactReadGetTaskDirective, capability: "host.v2.read:task_directives", description: "Read one workspace task directive"},
	{method: exactReadListTaskDirectives, capability: "host.v2.read:task_directives", description: "Read workspace task directives"},
	{method: exactReadGetTaskRelations, capability: "host.v2.read:task_relations", description: "Read one task relation snapshot"},
	{method: exactReadListTaskRelations, capability: "host.v2.read:task_relations", description: "Read workspace task relations"},
	{method: exactReadListTaskTransitions, capability: "host.v2.read:task_transitions", description: "Read pending task transitions"},
	{method: exactReadChangeEvidence, capability: "host.v2.read:change_requests", description: "Read linked change-request evidence"},
	{method: exactReadTaskUsage, capability: "host.v2.read:usage", description: "Read task and session usage totals"},
	{method: exactReadSourceIssueCapabilities, capability: "host.v2.read:source_issues", description: "Read operations for one task's linked Jira or Linear issue"},
}

type exactTaskObservationSource interface {
	GetTaskStatusSummaries(context.Context, []string) (map[string]*statussummary.TaskStatusSummary, error)
	GetTaskUsageTotals(context.Context, string) (*taskmodels.TaskUsageTotals, error)
	GetTaskSessionUsageTotals(context.Context, string, string) (*taskmodels.TaskUsageTotals, error)
}

func (h *pluginHost) ExactQueries() pluginsdk.ExactQueryManager { return h }

//nolint:cyclop // Read admission checks approval, caller scope, and snapshot identity together.
func (h *pluginHost) exactReadAdmission(
	ctx context.Context,
	requestID, workspaceID, method, capability string,
	filter any,
) (exactReadIdentity, func(), error) {
	if !isBoundedApprovalIdentifier(requestID) || !isBoundedApprovalIdentifier(workspaceID) {
		return exactReadIdentity{}, nil, status.Error(codes.InvalidArgument, "request_id and workspace_id are required bounded identifiers")
	}
	if h.service == nil || h.installationID == "" {
		return exactReadIdentity{}, nil, status.Error(codes.Unavailable, "exact Host authorization is unavailable")
	}
	h.service.approvalEffectMu.Lock()
	unlock := h.service.approvalEffectMu.Unlock
	installed := h.service.installedRecordByInstallationID(h.installationID)
	if installed == nil || (h.pluginID != "" && installed.ID != h.pluginID) {
		unlock()
		return exactReadIdentity{}, nil, status.Error(codes.PermissionDenied, "plugin installation is unavailable")
	}
	if h.service.approvals == nil {
		unlock()
		return exactReadIdentity{}, nil, status.Error(codes.Unavailable, "capability approval store is unavailable")
	}
	approval, found, err := h.service.approvalCurrent(h.installationID, workspaceID)
	if err != nil {
		unlock()
		return exactReadIdentity{}, nil, status.Error(codes.Unavailable, "capability approval state is unavailable")
	}
	if !found || approval.State != ApprovalStateActive {
		unlock()
		return exactReadIdentity{}, nil, status.Error(codes.PermissionDenied, string(ApprovalDenyMissingApproval))
	}
	filterDigest := exactFilterDigest(filter)
	requestDigest := CanonicalApprovalDigest("host-read", method, workspaceID, filterDigest)
	decision := h.service.authorizePluginCapability(
		h.installationID, workspaceID, capability, approval.Revision,
		requestDigest, CanonicalApprovalDigest("host-method", method, "v2"),
	)
	if !decision.Allowed {
		unlock()
		return exactReadIdentity{}, nil, status.Error(codes.PermissionDenied, string(decision.Reason))
	}
	if h.taskData == nil {
		unlock()
		return exactReadIdentity{}, nil, status.Error(codes.Unavailable, "workspace data service is unavailable")
	}
	workspaces, err := h.taskData.ListWorkspaces(ctx)
	if err != nil {
		unlock()
		return exactReadIdentity{}, nil, status.Error(codes.Unavailable, "workspace data is unavailable")
	}
	visible := false
	for _, workspace := range workspaces {
		if workspace != nil && workspace.ID == workspaceID {
			visible = true
			break
		}
	}
	if !visible {
		unlock()
		return exactReadIdentity{}, nil, status.Error(codes.NotFound, "workspace was not found")
	}
	return exactReadIdentity{
		installationID: h.installationID, workspaceID: workspaceID,
		method: method, filterDigest: filterDigest, capabilityRevision: approval.Revision,
	}, unlock, nil
}

func exactPage[T any](
	h *pluginHost,
	identity exactReadIdentity,
	requestID string,
	page pluginsdk.ExactReadPage,
	load func() ([]T, error),
) ([]T, pluginsdk.ExactReadPageInfo, error) {
	store, err := h.exactReadStore()
	if err != nil {
		return nil, pluginsdk.ExactReadPageInfo{}, err
	}
	rows, info, err := store.page(identity, requestID, page, func() (any, error) { return load() })
	if err != nil {
		return nil, pluginsdk.ExactReadPageInfo{}, err
	}
	items, err := decodeExactSnapshotRows[T](rows)
	return items, info, err
}

func singleReadReceipt(identity exactReadIdentity, requestID string) pluginsdk.HostReadReceipt {
	observedAt := time.Now().UTC().Format(time.RFC3339Nano)
	version := uuid.NewString()
	return pluginsdk.HostReadReceipt{
		InstallationID: identity.installationID, WorkspaceID: identity.workspaceID,
		CapabilityRevision: identity.capabilityRevision, RequestID: requestID,
		SnapshotVersion: version, ObservedAt: observedAt,
	}
}

func (h *pluginHost) ListWorkspaces(ctx context.Context, q pluginsdk.ExactWorkspaceQuery) (pluginsdk.ExactWorkspacePage, error) {
	identity, unlock, err := h.exactReadAdmission(ctx, q.RequestID, q.WorkspaceID, exactReadListWorkspaces, "host.v2.read:workspaces", nil)
	if err != nil {
		return pluginsdk.ExactWorkspacePage{}, err
	}
	defer unlock()
	items, info, err := exactPage(h, identity, q.RequestID, q.Page, func() ([]pluginsdk.ExactWorkspaceObservation, error) {
		workspaces, err := h.taskData.ListWorkspaces(ctx)
		if err != nil {
			return nil, err
		}
		for _, workspace := range workspaces {
			if workspace == nil || workspace.ID != q.WorkspaceID {
				continue
			}
			observed := time.Now().UTC().Format(time.RFC3339Nano)
			dto := workspaceModelToDTO(workspace)
			return []pluginsdk.ExactWorkspaceObservation{{Workspace: dto, ResourceVersion: workspace.UpdatedAt.UTC().Format(time.RFC3339Nano), ObservedAt: observed}}, nil
		}
		return nil, status.Error(codes.NotFound, "workspace was not found")
	})
	return pluginsdk.ExactWorkspacePage{Items: items, PageInfo: info}, err
}

func (h *pluginHost) ListWorkflows(ctx context.Context, q pluginsdk.ExactWorkflowQuery) (pluginsdk.ExactWorkflowPage, error) {
	identity, unlock, err := h.exactReadAdmission(ctx, q.RequestID, q.WorkspaceID, exactReadListWorkflows, "host.v2.read:workflows", nil)
	if err != nil {
		return pluginsdk.ExactWorkflowPage{}, err
	}
	defer unlock()
	if h.workflows == nil {
		return pluginsdk.ExactWorkflowPage{}, status.Error(codes.Unavailable, "workflow data service is unavailable")
	}
	items, info, err := exactPage(h, identity, q.RequestID, q.Page, func() ([]pluginsdk.ExactWorkflowObservation, error) {
		workflows, err := h.workflows.ListWorkflows(ctx, q.WorkspaceID, false)
		if err != nil {
			return nil, err
		}
		observed := time.Now().UTC().Format(time.RFC3339Nano)
		out := make([]pluginsdk.ExactWorkflowObservation, 0, len(workflows))
		for _, workflow := range workflows {
			if workflow == nil || workflow.WorkspaceID != q.WorkspaceID {
				continue
			}
			dto := workflowModelToDTO(workflow)
			out = append(out, pluginsdk.ExactWorkflowObservation{Workflow: dto, ResourceVersion: workflow.UpdatedAt.UTC().Format(time.RFC3339Nano), ObservedAt: observed})
		}
		return out, nil
	})
	return pluginsdk.ExactWorkflowPage{Items: items, PageInfo: info}, err
}

func (h *pluginHost) ListWorkflowSteps(ctx context.Context, q pluginsdk.ExactWorkflowStepQuery) (pluginsdk.ExactWorkflowStepPage, error) {
	identity, unlock, err := h.exactReadAdmission(ctx, q.RequestID, q.WorkspaceID, exactReadListWorkflowSteps, "host.v2.read:workflows", map[string]string{"workflow_id": q.WorkflowID})
	if err != nil {
		return pluginsdk.ExactWorkflowStepPage{}, err
	}
	defer unlock()
	if h.workflows == nil || h.workflowSteps == nil {
		return pluginsdk.ExactWorkflowStepPage{}, status.Error(codes.Unavailable, "workflow data service is unavailable")
	}
	workflows, err := h.workflows.ListWorkflows(ctx, q.WorkspaceID, false)
	if err != nil {
		return pluginsdk.ExactWorkflowStepPage{}, err
	}
	found := false
	for _, workflow := range workflows {
		if workflow != nil && workflow.ID == q.WorkflowID && workflow.WorkspaceID == q.WorkspaceID {
			found = true
			break
		}
	}
	if !found {
		return pluginsdk.ExactWorkflowStepPage{}, status.Error(codes.NotFound, "workflow was not found")
	}
	items, info, err := exactPage(h, identity, q.RequestID, q.Page, func() ([]pluginsdk.ExactWorkflowStepObservation, error) {
		steps, err := h.workflowSteps.ListStepsByWorkflow(ctx, q.WorkflowID)
		if err != nil {
			return nil, err
		}
		observed := time.Now().UTC().Format(time.RFC3339Nano)
		out := make([]pluginsdk.ExactWorkflowStepObservation, 0, len(steps))
		for _, step := range steps {
			if step == nil || step.WorkflowID != q.WorkflowID {
				continue
			}
			dto := workflowStepModelToDTO(step)
			encoded, _ := json.Marshal(dto)
			digest := sha256.Sum256(encoded)
			out = append(out, pluginsdk.ExactWorkflowStepObservation{Step: dto, ResourceVersion: hex.EncodeToString(digest[:]), ObservedAt: observed})
		}
		return out, nil
	})
	return pluginsdk.ExactWorkflowStepPage{Items: items, PageInfo: info}, err
}

func (h *pluginHost) ListTasks(ctx context.Context, q pluginsdk.ExactTaskQuery) (pluginsdk.ExactTaskPage, error) {
	filter := q.Filter
	if len(filter.WorkspaceIDs) != 0 && (len(filter.WorkspaceIDs) != 1 || filter.WorkspaceIDs[0] != q.WorkspaceID) {
		return pluginsdk.ExactTaskPage{}, status.Error(codes.InvalidArgument, "exact task filters cannot widen workspace scope")
	}
	filter.WorkspaceIDs = []string{q.WorkspaceID}
	identity, unlock, err := h.exactReadAdmission(ctx, q.RequestID, q.WorkspaceID, exactReadListTasks, "host.v2.read:tasks", filter)
	if err != nil {
		return pluginsdk.ExactTaskPage{}, err
	}
	defer unlock()
	items, info, err := exactPage(h, identity, q.RequestID, q.Page, func() ([]pluginsdk.ExactTaskObservation, error) {
		models, err := h.fetchTasksForWorkspaces(ctx, []string{q.WorkspaceID}, filter.IncludeEphemeral, filter.IncludeArchived)
		if err != nil {
			return nil, err
		}
		models = filterTasks(models, filter)
		sortTasksNewestFirst(models)
		return h.buildExactTaskObservations(ctx, models, "ListTasksExact")
	})
	return pluginsdk.ExactTaskPage{Items: items, PageInfo: info}, err
}

func (h *pluginHost) GetTask(ctx context.Context, q pluginsdk.ExactTaskGetQuery) (pluginsdk.ExactTaskObservation, pluginsdk.HostReadReceipt, error) {
	identity, unlock, err := h.exactReadAdmission(ctx, q.RequestID, q.WorkspaceID, exactReadGetTask, "host.v2.read:tasks", map[string]string{"task_id": q.TaskID})
	if err != nil {
		return pluginsdk.ExactTaskObservation{}, pluginsdk.HostReadReceipt{}, err
	}
	defer unlock()
	if q.TaskID == "" {
		return pluginsdk.ExactTaskObservation{}, pluginsdk.HostReadReceipt{}, status.Error(codes.InvalidArgument, "task_id is required")
	}
	task, err := h.taskData.GetTask(ctx, q.TaskID)
	if err != nil || task == nil || task.WorkspaceID != q.WorkspaceID {
		if errors.Is(err, repoerrors.ErrTaskNotFound) || err == nil {
			return pluginsdk.ExactTaskObservation{}, pluginsdk.HostReadReceipt{}, status.Error(codes.NotFound, "task was not found")
		}
		return pluginsdk.ExactTaskObservation{}, pluginsdk.HostReadReceipt{}, err
	}
	items, err := h.buildExactTaskObservations(ctx, []*taskmodels.Task{task}, "GetTaskExact")
	if err != nil {
		return pluginsdk.ExactTaskObservation{}, pluginsdk.HostReadReceipt{}, err
	}
	if len(items) != 1 {
		return pluginsdk.ExactTaskObservation{}, pluginsdk.HostReadReceipt{}, status.Error(codes.NotFound, "task was not found")
	}
	return items[0], singleReadReceipt(identity, q.RequestID), nil
}

//nolint:cyclop,funlen,gocognit,nestif // One bounded projection applies task redaction and optional status enrichment.
func (h *pluginHost) buildExactTaskObservations(ctx context.Context, models []*taskmodels.Task, operation string) ([]pluginsdk.ExactTaskObservation, error) {
	if len(models) == 0 {
		return []pluginsdk.ExactTaskObservation{}, nil
	}
	for _, model := range models {
		if model == nil {
			return nil, status.Error(codes.Internal, "task projection is incomplete")
		}
	}
	tasks := tasksToDTOs(models)
	if err := h.attachDependencies(ctx, tasks, models, true, operation); err != nil {
		return nil, err
	}
	source, hasObservationSource := h.taskData.(exactTaskObservationSource)
	taskIDs := make([]string, len(models))
	for i, model := range models {
		taskIDs[i] = model.ID
	}
	var summaries map[string]*statussummary.TaskStatusSummary
	if hasObservationSource {
		var err error
		summaries, err = source.GetTaskStatusSummaries(ctx, taskIDs)
		if err != nil {
			return nil, status.Error(codes.Unavailable, "task status summaries are unavailable")
		}
	}
	observed := time.Now().UTC().Format(time.RFC3339Nano)
	out := make([]pluginsdk.ExactTaskObservation, len(models))
	for i, model := range models {
		summary := summaries[model.ID]
		item := pluginsdk.ExactTaskObservation{
			Task: tasks[i], CanonicalStatus: string(model.State), ResourceVersion: model.UpdatedAt.UTC().Format(time.RFC3339Nano), ObservedAt: observed,
			SemanticActivity: "unknown", ExecutionState: "unknown", StatusKnown: true,
			BlockingReasons: []string{},
		}
		if tasks[i].Blocked {
			reason := tasks[i].BlockedReason
			if reason == "" {
				reason = taskservice.BlockedReasonUnknown
			}
			item.BlockingReasons = appendExactBlockingReason(item.BlockingReasons, reason)
		}
		if summary != nil {
			item.SemanticActivity = summary.ForegroundActivity
			if item.SemanticActivity == "" {
				item.SemanticActivity = "idle"
			}
			item.ExecutionState = "idle"
			if summary.LaunchQueue != nil {
				item.ExecutionState = "queued"
				if summary.LaunchQueue.Reason != "" {
					item.BlockingReasons = appendExactBlockingReason(item.BlockingReasons, "launch_queue:"+summary.LaunchQueue.Reason)
				}
			}
			if summary.PrimarySession != nil {
				item.ExecutionState = summary.PrimarySession.State
			}
			if summary.PendingAction != "" {
				item.BlockingReasons = appendExactBlockingReason(item.BlockingReasons, "pending_action:"+summary.PendingAction)
			}
			if summary.ActiveError != nil {
				reason := summary.ActiveError.Category
				if reason == "" {
					reason = "active_error"
				}
				item.BlockingReasons = appendExactBlockingReason(item.BlockingReasons, reason)
			}
			if summary.TaskError != nil {
				reason := summary.TaskError.Category
				if reason == "" {
					reason = "task_error"
				}
				item.BlockingReasons = appendExactBlockingReason(item.BlockingReasons, reason)
			}
			if summary.LastActivityAt != nil {
				value := summary.LastActivityAt.UTC().Format(time.RFC3339Nano)
				item.LastMeaningfulActivityAt = &value
			}
		}
		out[i] = item
	}
	return out, nil
}

func appendExactBlockingReason(reasons []string, reason string) []string {
	for _, existing := range reasons {
		if existing == reason {
			return reasons
		}
	}
	return append(reasons, reason)
}

func (h *pluginHost) ListSessions(ctx context.Context, q pluginsdk.ExactSessionQuery) (pluginsdk.ExactSessionPage, error) {
	filter := q.Filter
	if len(filter.WorkspaceIDs) != 0 && (len(filter.WorkspaceIDs) != 1 || filter.WorkspaceIDs[0] != q.WorkspaceID) {
		return pluginsdk.ExactSessionPage{}, status.Error(codes.InvalidArgument, "exact session filters cannot widen workspace scope")
	}
	filter.WorkspaceIDs = []string{q.WorkspaceID}
	identity, unlock, err := h.exactReadAdmission(ctx, q.RequestID, q.WorkspaceID, exactReadListSessions, "host.v2.read:sessions", filter)
	if err != nil {
		return pluginsdk.ExactSessionPage{}, err
	}
	defer unlock()
	items, info, err := exactPage(h, identity, q.RequestID, q.Page, func() ([]pluginsdk.ExactSessionObservation, error) {
		models, err := h.fetchSessionsForFilter(ctx, filter)
		if err != nil {
			return nil, err
		}
		models = filterSessionsByState(models, filter.States)
		sortSessionsNewestFirst(models)
		if len(models) > exactSnapshotMaxRows {
			return nil, status.Error(codes.ResourceExhausted, "exact session snapshot exceeds its bounded size")
		}
		observed := time.Now().UTC().Format(time.RFC3339Nano)
		out := make([]pluginsdk.ExactSessionObservation, len(models))
		for i, model := range models {
			dto := h.sessionToDTO(ctx, model)
			out[i] = pluginsdk.ExactSessionObservation{
				Session: dto, ExecutionState: string(model.State), ResourceVersion: model.UpdatedAt.UTC().Format(time.RFC3339Nano),
				ObservedAt: observed, ExecutionID: model.AgentExecutionID,
			}
		}
		return out, nil
	})
	return pluginsdk.ExactSessionPage{Items: items, PageInfo: info}, err
}

//nolint:cyclop,funlen,gocognit // The page applies approval scope, redaction, and cursor rules to interaction data.
func (h *pluginHost) ListPendingInteractions(ctx context.Context, q pluginsdk.ExactInteractionQuery) (pluginsdk.ExactInteractionPage, error) {
	filter := q.Filter
	if len(filter.TaskIDs)+len(filter.SessionIDs)+len(filter.Kinds) > maxMessageFilterValues {
		return pluginsdk.ExactInteractionPage{}, status.Error(codes.ResourceExhausted, "exact interaction filters exceed the bounded size")
	}
	identity, unlock, err := h.exactReadAdmission(ctx, q.RequestID, q.WorkspaceID, exactReadListInteractions, "host.v2.read:interactions", filter)
	if err != nil {
		return pluginsdk.ExactInteractionPage{}, err
	}
	defer unlock()
	if h.interactionData == nil {
		return pluginsdk.ExactInteractionPage{}, status.Error(codes.Unavailable, "interaction data service is unavailable")
	}
	items, info, err := exactPage(h, identity, q.RequestID, q.Page, func() ([]pluginsdk.ExactInteractionObservation, error) {
		tasks, err := h.fetchAllTasksForWorkspace(ctx, q.WorkspaceID, true, true)
		if err != nil {
			return nil, err
		}
		allowedTasks := make(map[string]bool, len(tasks))
		for _, task := range tasks {
			if task != nil {
				allowedTasks[task.ID] = true
			}
		}
		if len(filter.TaskIDs) > 0 {
			for _, id := range filter.TaskIDs {
				if !allowedTasks[id] {
					return nil, status.Error(codes.InvalidArgument, "interaction filter contains a task outside the workspace")
				}
			}
		} else {
			filter.TaskIDs = make([]string, 0, len(allowedTasks))
			for id := range allowedTasks {
				filter.TaskIDs = append(filter.TaskIDs, id)
			}
			sort.Strings(filter.TaskIDs)
		}
		allowedSessions := make(map[string]bool)
		for _, task := range tasks {
			if task == nil || (len(filter.TaskIDs) > 0 && !exactContainsString(filter.TaskIDs, task.ID)) {
				continue
			}
			sessions, listErr := h.taskData.ListTaskSessions(ctx, task.ID)
			if listErr != nil {
				return nil, listErr
			}
			for _, session := range sessions {
				if session != nil {
					allowedSessions[session.ID] = true
				}
			}
		}
		if len(filter.SessionIDs) > 0 {
			for _, id := range filter.SessionIDs {
				if !allowedSessions[id] {
					return nil, status.Error(codes.InvalidArgument, "interaction filter contains a session outside the workspace")
				}
			}
		} else {
			filter.SessionIDs = make([]string, 0, len(allowedSessions))
			for id := range allowedSessions {
				filter.SessionIDs = append(filter.SessionIDs, id)
			}
			sort.Strings(filter.SessionIDs)
		}
		if err := validateInteractionKinds(filter.Kinds); err != nil {
			return nil, err
		}
		models, err := h.interactionData.ListPendingInteractions(ctx, taskmodels.PendingInteractionFilter{TaskIDs: filter.TaskIDs, SessionIDs: filter.SessionIDs, Kinds: filter.Kinds})
		if err != nil {
			return nil, err
		}
		observed := time.Now().UTC().Format(time.RFC3339Nano)
		out := make([]pluginsdk.ExactInteractionObservation, 0, len(models))
		for _, model := range models {
			if model == nil {
				continue
			}
			dto := interactionModelToDTO(model)
			version := digestPublicValue(dto)
			out = append(out, pluginsdk.ExactInteractionObservation{Interaction: dto, ResourceVersion: version, ObservedAt: observed})
		}
		return out, nil
	})
	return pluginsdk.ExactInteractionPage{Items: items, PageInfo: info}, err
}

func (h *pluginHost) GetInteraction(ctx context.Context, q pluginsdk.ExactInteractionGetQuery) (pluginsdk.ExactInteractionObservation, pluginsdk.HostReadReceipt, error) {
	identity, unlock, err := h.exactReadAdmission(ctx, q.RequestID, q.WorkspaceID, exactReadGetInteraction, "host.v2.read:interactions", map[string]string{"interaction_id": q.InteractionID})
	if err != nil {
		return pluginsdk.ExactInteractionObservation{}, pluginsdk.HostReadReceipt{}, err
	}
	defer unlock()
	if h.interactionData == nil {
		return pluginsdk.ExactInteractionObservation{}, pluginsdk.HostReadReceipt{}, status.Error(codes.Unavailable, "interaction data service is unavailable")
	}
	model, err := h.resolveInteraction(ctx, q.InteractionID)
	if err != nil {
		return pluginsdk.ExactInteractionObservation{}, pluginsdk.HostReadReceipt{}, err
	}
	task, err := h.taskData.GetTask(ctx, model.TaskID)
	if err != nil || task == nil || task.WorkspaceID != q.WorkspaceID {
		return pluginsdk.ExactInteractionObservation{}, pluginsdk.HostReadReceipt{}, status.Error(codes.NotFound, "interaction was not found")
	}
	dto := interactionModelToDTO(model)
	return pluginsdk.ExactInteractionObservation{Interaction: dto, ResourceVersion: digestPublicValue(dto), ObservedAt: time.Now().UTC().Format(time.RFC3339Nano)}, singleReadReceipt(identity, q.RequestID), nil
}

//nolint:cyclop,funlen,gocognit // The page applies approval scope, redaction, and cursor rules to message data.
func (h *pluginHost) ListSanitizedMessages(ctx context.Context, q pluginsdk.ExactMessageQuery) (pluginsdk.ExactMessagePage, error) {
	filter := q.Filter
	identity, unlock, err := h.exactReadAdmission(ctx, q.RequestID, q.WorkspaceID, exactReadListMessages, "host.v2.read:messages", filter)
	if err != nil {
		return pluginsdk.ExactMessagePage{}, err
	}
	defer unlock()
	if h.messageData == nil {
		return pluginsdk.ExactMessagePage{}, status.Error(codes.Unavailable, "message data service is unavailable")
	}
	items, info, err := exactPage(h, identity, q.RequestID, q.Page, func() ([]pluginsdk.ExactMessageObservation, error) {
		tasks, err := h.fetchAllTasksForWorkspace(ctx, q.WorkspaceID, true, true)
		if err != nil {
			return nil, err
		}
		allowedTasks := make(map[string]bool, len(tasks))
		for _, task := range tasks {
			if task != nil {
				allowedTasks[task.ID] = true
			}
		}
		taskIDs := append([]string(nil), filter.TaskIDs...)
		if len(taskIDs) == 0 {
			for id := range allowedTasks {
				taskIDs = append(taskIDs, id)
			}
		}
		if len(taskIDs) > maxMessageFilterValues {
			return nil, status.Error(codes.ResourceExhausted, "workspace message query requires a narrower task filter")
		}
		for _, id := range taskIDs {
			if !allowedTasks[id] {
				return nil, status.Error(codes.InvalidArgument, "message filter contains a task outside the workspace")
			}
		}
		if len(filter.SessionIDs)+len(filter.Types) > maxMessageFilterValues {
			return nil, status.Error(codes.ResourceExhausted, "message filter exceeds the bounded size")
		}
		allowedSessions := make(map[string]bool)
		for _, task := range tasks {
			if task == nil || !exactContainsString(taskIDs, task.ID) {
				continue
			}
			sessions, listErr := h.taskData.ListTaskSessions(ctx, task.ID)
			if listErr != nil {
				return nil, listErr
			}
			for _, session := range sessions {
				if session != nil {
					allowedSessions[session.ID] = true
				}
			}
		}
		for _, id := range filter.SessionIDs {
			if !allowedSessions[id] {
				return nil, status.Error(codes.InvalidArgument, "message filter contains a session outside the workspace")
			}
		}
		since, err := parseFilterTime(filter.Since, "since")
		if err != nil {
			return nil, err
		}
		until, err := parseFilterTime(filter.Until, "until")
		if err != nil {
			return nil, err
		}
		rows, err := h.messageData.ListMessagesForPlugin(ctx, taskmodels.PluginMessageFilter{TaskIDs: taskIDs, SessionIDs: filter.SessionIDs, Types: filter.Types, Since: since, Until: until, Limit: exactSnapshotMaxRows + 1})
		if err != nil {
			return nil, err
		}
		if len(rows) > exactSnapshotMaxRows {
			return nil, status.Error(codes.ResourceExhausted, "exact message snapshot exceeds its bounded size")
		}
		observed := time.Now().UTC().Format(time.RFC3339Nano)
		out := make([]pluginsdk.ExactMessageObservation, 0, len(rows))
		for _, row := range rows {
			if row == nil {
				continue
			}
			dto := messageModelToDTO(row)
			out = append(out, pluginsdk.ExactMessageObservation{Message: dto, ResourceVersion: digestPublicValue(map[string]string{"id": dto.ID, "created_at": dto.CreatedAt, "task_id": dto.TaskID, "session_id": dto.SessionID}), ObservedAt: observed, Sanitization: "system_context_removed"})
		}
		return out, nil
	})
	return pluginsdk.ExactMessagePage{Items: items, PageInfo: info}, err
}

func (h *pluginHost) ListTaskRelations(ctx context.Context, q pluginsdk.ExactTaskRelationsQuery) (pluginsdk.ExactTaskRelationsPage, error) {
	return h.listExactTaskRelations(ctx, q, exactReadListTaskRelations)
}

func (h *pluginHost) GetTaskRelations(ctx context.Context, q pluginsdk.ExactTaskRelationsQuery) (pluginsdk.ExactTaskRelationsPage, error) {
	if q.TaskID == "" {
		return pluginsdk.ExactTaskRelationsPage{}, status.Error(codes.InvalidArgument, "task_id is required")
	}
	return h.listExactTaskRelations(ctx, q, exactReadGetTaskRelations)
}

//nolint:cyclop,gocognit // The page binds relation projection and pagination to one authorized task snapshot.
func (h *pluginHost) listExactTaskRelations(ctx context.Context, q pluginsdk.ExactTaskRelationsQuery, method string) (pluginsdk.ExactTaskRelationsPage, error) {
	identity, unlock, err := h.exactReadAdmission(ctx, q.RequestID, q.WorkspaceID, method, "host.v2.read:task_relations", map[string]string{"task_id": q.TaskID})
	if err != nil {
		return pluginsdk.ExactTaskRelationsPage{}, err
	}
	defer unlock()
	items, info, err := exactPage(h, identity, q.RequestID, q.Page, func() ([]pluginsdk.TaskRelation, error) {
		models, err := h.fetchTasksForWorkspaces(ctx, []string{q.WorkspaceID}, true, true)
		if err != nil {
			return nil, err
		}
		if len(models) > exactSnapshotMaxRows {
			return nil, status.Error(codes.ResourceExhausted, "exact relation snapshot exceeds its bounded size")
		}
		versions := make(map[string]string, len(models))
		taskFound := q.TaskID == ""
		for _, model := range models {
			if model != nil {
				versions[model.ID] = model.UpdatedAt.UTC().Format(time.RFC3339Nano)
				if model.ID == q.TaskID {
					taskFound = true
				}
			}
		}
		if !taskFound {
			return nil, status.Error(codes.InvalidArgument, "task relation filter contains a task outside the workspace")
		}
		out := make([]pluginsdk.TaskRelation, 0)
		seen := make(map[string]bool)
		for _, model := range models {
			if model == nil {
				continue
			}
			if model.ParentID != "" && versions[model.ParentID] != "" {
				out = appendRelation(out, seen, q.WorkspaceID, model.ParentID, model.ID, "parent", versions[model.ParentID], versions[model.ID])
			}
		}
		observations, err := h.buildExactTaskObservations(ctx, models, "ListTaskRelationsExact")
		if err != nil {
			return nil, err
		}
		for _, observation := range observations {
			for _, predecessor := range observation.Task.DependsOn {
				if versions[predecessor.ID] == "" {
					continue
				}
				out = appendRelation(out, seen, q.WorkspaceID, predecessor.ID, observation.Task.ID, "dependency", versions[predecessor.ID], observation.ResourceVersion)
			}
		}
		if q.TaskID != "" {
			filtered := out[:0]
			for _, relation := range out {
				if relation.SourceTaskID == q.TaskID || relation.TargetTaskID == q.TaskID {
					filtered = append(filtered, relation)
				}
			}
			out = filtered
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].SourceTaskID == out[j].SourceTaskID {
				if out[i].Kind == out[j].Kind {
					return out[i].TargetTaskID < out[j].TargetTaskID
				}
				return out[i].Kind < out[j].Kind
			}
			return out[i].SourceTaskID < out[j].SourceTaskID
		})
		return out, nil
	})
	return pluginsdk.ExactTaskRelationsPage{Items: items, PageInfo: info}, err
}

func appendRelation(items []pluginsdk.TaskRelation, seen map[string]bool, workspaceID, source, target, kind, sourceVersion, targetVersion string) []pluginsdk.TaskRelation {
	key := source + "\x00" + target + "\x00" + kind
	if seen[key] {
		return items
	}
	seen[key] = true
	version := digestPublicValue(map[string]string{"workspace": workspaceID, "source": source, "target": target, "kind": kind, "source_version": sourceVersion, "target_version": targetVersion})
	return append(items, pluginsdk.TaskRelation{WorkspaceID: workspaceID, SourceTaskID: source, TargetTaskID: target, Kind: kind, SourceResourceVersion: sourceVersion, TargetResourceVersion: targetVersion, ResourceVersion: version})
}

//nolint:cyclop,gocognit // The page projects authorized change evidence with stable pagination.
func (h *pluginHost) ListChangeRequestEvidence(ctx context.Context, q pluginsdk.ExactChangeRequestEvidenceQuery) (pluginsdk.ExactChangeRequestEvidencePage, error) {
	taskIDs := append([]string(nil), q.TaskIDs...)
	identity, unlock, err := h.exactReadAdmission(ctx, q.RequestID, q.WorkspaceID, exactReadChangeEvidence, "host.v2.read:change_requests", taskIDs)
	if err != nil {
		return pluginsdk.ExactChangeRequestEvidencePage{}, err
	}
	defer unlock()
	items, info, err := exactPage(h, identity, q.RequestID, q.Page, func() ([]pluginsdk.ChangeRequestEvidence, error) {
		models, err := h.fetchTasksForWorkspaces(ctx, []string{q.WorkspaceID}, true, true)
		if err != nil {
			return nil, err
		}
		byID := make(map[string]*taskmodels.Task, len(models))
		for _, task := range models {
			if task != nil {
				byID[task.ID] = task
			}
		}
		if len(taskIDs) == 0 {
			for id := range byID {
				taskIDs = append(taskIDs, id)
			}
		}
		sort.Strings(taskIDs)
		if len(taskIDs) > maxPageLimit {
			return nil, status.Error(codes.ResourceExhausted, "change evidence query exceeds the bounded task count")
		}
		for _, id := range taskIDs {
			if byID[id] == nil {
				return nil, status.Error(codes.InvalidArgument, "change evidence filter contains a task outside the workspace")
			}
		}
		observed := time.Now().UTC().Format(time.RFC3339Nano)
		source := h.taskPRs
		if h.taskPRsDep != nil {
			source = h.taskPRsDep()
		}
		if source == nil {
			out := make([]pluginsdk.ChangeRequestEvidence, 0, len(taskIDs))
			for _, id := range taskIDs {
				out = append(out, pluginsdk.ChangeRequestEvidence{TaskID: id, ProviderState: "unsupported", ResourceVersion: byID[id].UpdatedAt.UTC().Format(time.RFC3339Nano), ObservedAt: observed})
			}
			return out, nil
		}
		prsByTask, err := source.ListTaskPRsByTaskIDs(ctx, taskIDs)
		providerFailed := err != nil
		out := make([]pluginsdk.ChangeRequestEvidence, 0)
		for _, id := range taskIDs {
			prs := prsByTask[id]
			if providerFailed {
				reason := "provider_read_failed"
				out = append(out, pluginsdk.ChangeRequestEvidence{TaskID: id, Provider: "github", ProviderState: "unavailable", ProviderError: &reason, ResourceVersion: byID[id].UpdatedAt.UTC().Format(time.RFC3339Nano), ObservedAt: observed})
				continue
			}
			if len(prs) == 0 {
				out = append(out, pluginsdk.ChangeRequestEvidence{TaskID: id, Provider: "github", ProviderState: "no_evidence", ResourceVersion: byID[id].UpdatedAt.UTC().Format(time.RFC3339Nano), ObservedAt: observed})
				continue
			}
			for _, pr := range prs {
				if pr == nil {
					continue
				}
				fetchedAt := timePtrToRFC3339(pr.LastSyncedAt)
				version := digestPublicValue(map[string]string{"task_id": id, "pr_id": pr.ID, "head": pr.HeadSHA, "updated_at": pr.UpdatedAt.UTC().Format(time.RFC3339Nano)})
				out = append(out, pluginsdk.ChangeRequestEvidence{TaskID: id, Provider: "github", RepositoryID: pr.RepositoryID, RepositoryOwner: pr.Owner, RepositoryName: pr.Repo, Number: int64(pr.PRNumber), URL: pr.PRURL, HeadRevision: pr.HeadSHA, ReviewState: pr.ReviewState, ChecksState: pr.ChecksState, MergeableState: pr.MergeableState, UnresolvedReviewThreads: int32(pr.UnresolvedReviewThreads), ChecksTotal: int32(pr.ChecksTotal), ChecksPassing: int32(pr.ChecksPassing), ProviderState: "available", FetchedAt: fetchedAt, ResourceVersion: version, ObservedAt: observed})
			}
		}
		return out, nil
	})
	return pluginsdk.ExactChangeRequestEvidencePage{Items: items, PageInfo: info}, err
}

//nolint:cyclop,gocognit // The page projects authorized usage records with stable pagination.
func (h *pluginHost) ListTaskUsage(ctx context.Context, q pluginsdk.ExactTaskUsageQuery) (pluginsdk.ExactTaskUsagePage, error) {
	taskIDs := append([]string(nil), q.TaskIDs...)
	identity, unlock, err := h.exactReadAdmission(ctx, q.RequestID, q.WorkspaceID, exactReadTaskUsage, "host.v2.read:usage", taskIDs)
	if err != nil {
		return pluginsdk.ExactTaskUsagePage{}, err
	}
	defer unlock()
	source, ok := h.taskData.(exactTaskObservationSource)
	if !ok {
		return pluginsdk.ExactTaskUsagePage{}, status.Error(codes.Unavailable, "task usage service is unavailable")
	}
	items, info, err := exactPage(h, identity, q.RequestID, q.Page, func() ([]pluginsdk.TaskUsageObservation, error) {
		models, err := h.fetchTasksForWorkspaces(ctx, []string{q.WorkspaceID}, true, true)
		if err != nil {
			return nil, err
		}
		byID := make(map[string]*taskmodels.Task, len(models))
		for _, task := range models {
			if task != nil {
				byID[task.ID] = task
			}
		}
		if len(taskIDs) == 0 {
			for id := range byID {
				taskIDs = append(taskIDs, id)
			}
		}
		sort.Strings(taskIDs)
		if len(taskIDs) > exactSnapshotMaxRows {
			return nil, status.Error(codes.ResourceExhausted, "exact usage query exceeds the bounded task count")
		}
		observed := time.Now().UTC().Format(time.RFC3339Nano)
		out := make([]pluginsdk.TaskUsageObservation, 0, len(taskIDs))
		for _, taskID := range taskIDs {
			model := byID[taskID]
			if model == nil {
				return nil, status.Error(codes.InvalidArgument, "usage filter contains a task outside the workspace")
			}
			totals, err := source.GetTaskUsageTotals(ctx, taskID)
			if err != nil || totals == nil {
				return nil, status.Error(codes.Unavailable, "task usage totals are unavailable")
			}
			out = append(out, usageObservation(taskID, nil, model.UpdatedAt, observed, totals))
			sessions, err := h.taskData.ListTaskSessions(ctx, taskID)
			if err != nil {
				return nil, err
			}
			for _, session := range sessions {
				if session == nil {
					continue
				}
				totals, err := source.GetTaskSessionUsageTotals(ctx, taskID, session.ID)
				if err != nil || totals == nil {
					return nil, status.Error(codes.Unavailable, "session usage totals are unavailable")
				}
				sessionID := session.ID
				out = append(out, usageObservation(taskID, &sessionID, session.UpdatedAt, observed, totals))
			}
		}
		return out, nil
	})
	return pluginsdk.ExactTaskUsagePage{Items: items, PageInfo: info}, err
}

func usageObservation(taskID string, sessionID *string, resourceTime time.Time, observed string, totals *taskmodels.TaskUsageTotals) pluginsdk.TaskUsageObservation {
	item := pluginsdk.TaskUsageObservation{TaskID: taskID, SessionID: sessionID, Currency: "USD", ObservationState: "observed", OutputTokensComplete: totals.OutputTokensComplete, EventCount: totals.EventCount, EstimatedEventCount: totals.EstimatedEventCount, UnpricedEventCount: totals.UnpricedEventCount, TokensIn: totals.TokensIn, TokensCachedRead: totals.TokensCachedRead, TokensCachedWrite: totals.TokensCachedWrite, TokensOut: totals.TokensOut, TokensThought: totals.TokensThought, TokensTotal: totals.TokensTotal, ResourceVersion: resourceTime.UTC().Format(time.RFC3339Nano), ObservedAt: observed}
	if totals.EventCount == 0 {
		item.ObservationState = "no_observations"
		reason := "no_usage_events"
		item.CostUnknownReason = &reason
		return item
	}
	cost := totals.CostSubcents
	item.CostSubcents = &cost
	item.CostComplete = totals.UnpricedEventCount == 0
	if !item.CostComplete {
		reason := "unpriced_events"
		item.CostUnknownReason = &reason
	}
	item.FirstEventAt = timePtrToRFC3339(totals.FirstEventAt)
	item.LastEventAt = timePtrToRFC3339(totals.LastEventAt)
	item.ElapsedMilliseconds = exactElapsedMilliseconds(item.FirstEventAt, item.LastEventAt)
	return item
}

type exactPendingTransitionObservation struct {
	transition pluginsdk.PendingTaskTransition
	sessionID  string
}

//nolint:cyclop,gocognit // The page resolves and projects the authorized task inbox in one read boundary.
func (h *pluginHost) ListTaskInbox(ctx context.Context, q pluginsdk.ExactTaskInboxQuery) (pluginsdk.ExactTaskInboxPage, error) {
	filter := map[string]string{"task_id": q.TaskID, "session_id": q.SessionID}
	identity, unlock, err := h.exactReadAdmission(ctx, q.RequestID, q.WorkspaceID, exactReadListTaskInbox, "host.v2.read:task_inbox", filter)
	if err != nil {
		return pluginsdk.ExactTaskInboxPage{}, err
	}
	defer unlock()
	items, info, err := exactPage(h, identity, q.RequestID, q.Page, func() ([]pluginsdk.TaskInboxItem, error) {
		taskByID, taskIDs, sessionIDs, err := h.exactInboxScope(ctx, q.WorkspaceID, q.TaskID, q.SessionID)
		if err != nil {
			return nil, err
		}
		out := make([]pluginsdk.TaskInboxItem, 0)
		if h.interactionData != nil && len(taskIDs) > 0 {
			interactions, listErr := h.interactionData.ListPendingInteractions(ctx, taskmodels.PendingInteractionFilter{TaskIDs: taskIDs, SessionIDs: sessionIDs})
			if listErr != nil {
				return nil, status.Error(codes.Unavailable, "pending task interactions are unavailable")
			}
			for _, interaction := range interactions {
				if interaction == nil || interaction.ID == "" || interaction.Status != taskmodels.InteractionStatusPending ||
					taskByID[interaction.TaskID] == nil || !exactContainsString(taskIDs, interaction.TaskID) ||
					(len(sessionIDs) > 0 && !exactContainsString(sessionIDs, interaction.SessionID)) {
					continue
				}
				createdAt := interaction.CreatedAt.UTC().Format(time.RFC3339Nano)
				item := pluginsdk.TaskInboxItem{ID: interaction.ID, WorkspaceID: q.WorkspaceID, TaskID: interaction.TaskID, SessionID: interaction.SessionID, Kind: string(interaction.Kind), State: string(interaction.Status), CreatedAt: createdAt}
				item.ResourceVersion = digestPublicValue(item)
				out = append(out, item)
			}
		}
		if source := h.pendingTaskTransitionSource(); source != nil {
			transitions, loadErr := h.loadExactPendingTaskTransitions(ctx, q.WorkspaceID, q.TaskID, taskByID)
			if loadErr != nil {
				return nil, loadErr
			}
			for _, observed := range transitions {
				if len(sessionIDs) > 0 && !exactContainsString(sessionIDs, observed.sessionID) {
					continue
				}
				transition := observed.transition
				item := pluginsdk.TaskInboxItem{ID: transition.ID, WorkspaceID: transition.WorkspaceID, TaskID: transition.TaskID, SessionID: observed.sessionID, Kind: "workflow_transition", State: transition.State, CreatedAt: transition.CreatedAt, ResourceVersion: transition.ResourceVersion}
				out = append(out, item)
			}
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].CreatedAt != out[j].CreatedAt {
				return out[i].CreatedAt < out[j].CreatedAt
			}
			if out[i].Kind != out[j].Kind {
				return out[i].Kind < out[j].Kind
			}
			return out[i].ID < out[j].ID
		})
		return out, nil
	})
	return pluginsdk.ExactTaskInboxPage{Items: items, PageInfo: info}, err
}

//nolint:cyclop // Inbox scope resolution verifies the task and optional session relationship.
func (h *pluginHost) exactInboxScope(ctx context.Context, workspaceID, taskID, sessionID string) (
	map[string]*taskmodels.Task, []string, []string, error,
) {
	tasks, err := h.fetchAllTasksForWorkspace(ctx, workspaceID, true, true)
	if err != nil {
		return nil, nil, nil, status.Error(codes.Unavailable, "workspace tasks are unavailable")
	}
	taskByID := make(map[string]*taskmodels.Task, len(tasks))
	for _, task := range tasks {
		if task != nil {
			taskByID[task.ID] = task
		}
	}
	if taskID != "" && taskByID[taskID] == nil {
		return nil, nil, nil, status.Error(codes.InvalidArgument, "task inbox filter contains a task outside the workspace")
	}
	taskIDs := make([]string, 0, len(taskByID))
	for id := range taskByID {
		if taskID == "" || taskID == id {
			taskIDs = append(taskIDs, id)
		}
	}
	sort.Strings(taskIDs)
	if len(taskIDs) > maxMessageFilterValues {
		return nil, nil, nil, status.Error(codes.ResourceExhausted, "task inbox query requires a narrower task filter")
	}
	allowedSessions := make(map[string]bool)
	for _, id := range taskIDs {
		sessions, listErr := h.taskData.ListTaskSessions(ctx, id)
		if listErr != nil {
			return nil, nil, nil, status.Error(codes.Unavailable, "workspace sessions are unavailable")
		}
		for _, session := range sessions {
			if session != nil {
				allowedSessions[session.ID] = true
			}
		}
	}
	sessionIDs := make([]string, 0, len(allowedSessions))
	if sessionID != "" {
		if !allowedSessions[sessionID] {
			return nil, nil, nil, status.Error(codes.InvalidArgument, "task inbox filter contains a session outside the workspace")
		}
		sessionIDs = append(sessionIDs, sessionID)
	} else {
		for id := range allowedSessions {
			sessionIDs = append(sessionIDs, id)
		}
		sort.Strings(sessionIDs)
	}
	return taskByID, taskIDs, sessionIDs, nil
}

func (h *pluginHost) pendingTaskTransitionSource() pendingTaskTransitionSource {
	if h.pendingTaskTransitions == nil {
		return nil
	}
	return h.pendingTaskTransitions()
}

func (h *pluginHost) loadExactPendingTaskTransitions(
	ctx context.Context,
	workspaceID, taskID string,
	taskByID map[string]*taskmodels.Task,
) ([]exactPendingTransitionObservation, error) {
	source := h.pendingTaskTransitionSource()
	if source == nil {
		return nil, status.Error(codes.Unavailable, "pending task transition service is unavailable")
	}
	records, err := source.ListPendingTaskTransitions(ctx)
	if err != nil {
		return nil, status.Error(codes.Unavailable, "pending task transitions are unavailable")
	}
	out := make([]exactPendingTransitionObservation, 0)
	for _, record := range records {
		task := taskByID[record.TaskID]
		if task == nil || (taskID != "" && task.ID != taskID) {
			continue
		}
		id := record.ID
		if id == "" {
			id = digestPublicValue(map[string]string{"task_id": record.TaskID, "session_id": record.SessionID, "workflow_id": record.WorkflowID, "step_id": record.WorkflowStepID, "queued_at": record.QueuedAt.UTC().Format(time.RFC3339Nano)})
		}
		createdAt := record.QueuedAt.UTC().Format(time.RFC3339Nano)
		version := digestPublicValue(map[string]string{"id": id, "task_version": task.UpdatedAt.UTC().Format(time.RFC3339Nano), "workflow_id": record.WorkflowID, "step_id": record.WorkflowStepID, "queued_at": createdAt})
		transition := pluginsdk.PendingTaskTransition{
			ID: id, WorkspaceID: workspaceID, TaskID: task.ID, FromWorkflowID: task.WorkflowID, FromStepID: task.WorkflowStepID,
			ToWorkflowID: record.WorkflowID, ToStepID: record.WorkflowStepID, State: "pending", ResourceVersion: version, CreatedAt: createdAt,
		}
		out = append(out, exactPendingTransitionObservation{transition: transition, sessionID: record.SessionID})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].transition.CreatedAt != out[j].transition.CreatedAt {
			return out[i].transition.CreatedAt < out[j].transition.CreatedAt
		}
		return out[i].transition.ID < out[j].transition.ID
	})
	if len(out) > exactSnapshotMaxRows {
		return nil, status.Error(codes.ResourceExhausted, "pending task transition snapshot exceeds its bounded size")
	}
	return out, nil
}

func (h *pluginHost) GetTaskDirective(ctx context.Context, q pluginsdk.ExactTaskDirectiveGetQuery) (pluginsdk.TaskDirective, pluginsdk.HostReadReceipt, error) {
	identity, unlock, err := h.exactReadAdmission(ctx, q.RequestID, q.WorkspaceID, exactReadGetTaskDirective, "host.v2.read:task_directives", map[string]string{"directive_id": q.DirectiveID})
	if err != nil {
		return pluginsdk.TaskDirective{}, pluginsdk.HostReadReceipt{}, err
	}
	defer unlock()
	if !isBoundedApprovalIdentifier(q.DirectiveID) {
		return pluginsdk.TaskDirective{}, pluginsdk.HostReadReceipt{}, status.Error(codes.InvalidArgument, "directive_id is required")
	}
	store := h.commandStoreForDirective()
	if store == nil {
		return pluginsdk.TaskDirective{}, pluginsdk.HostReadReceipt{}, status.Error(codes.Unavailable, "task directive store is unavailable")
	}
	record, err := store.GetTaskDirective(ctx, h.installationID, q.WorkspaceID, q.DirectiveID)
	if errors.Is(err, state.ErrTaskDirectiveNotFound) {
		return pluginsdk.TaskDirective{}, pluginsdk.HostReadReceipt{}, status.Error(codes.NotFound, "task directive was not found")
	}
	if err != nil {
		return pluginsdk.TaskDirective{}, pluginsdk.HostReadReceipt{}, status.Error(codes.Unavailable, "task directive data is unavailable")
	}
	record, err = h.refreshTaskDirective(ctx, record, identity.capabilityRevision)
	if err != nil {
		return pluginsdk.TaskDirective{}, pluginsdk.HostReadReceipt{}, err
	}
	return taskDirectiveToSDK(record), singleReadReceipt(identity, q.RequestID), nil
}

func (h *pluginHost) ListTaskDirectives(ctx context.Context, q pluginsdk.ExactTaskDirectiveQuery) (pluginsdk.ExactTaskDirectivesPage, error) {
	if q.TaskID != "" && !isBoundedApprovalIdentifier(q.TaskID) {
		return pluginsdk.ExactTaskDirectivesPage{}, status.Error(codes.InvalidArgument, "task_id is invalid")
	}
	filter := map[string]string{"task_id": q.TaskID}
	identity, unlock, err := h.exactReadAdmission(ctx, q.RequestID, q.WorkspaceID, exactReadListTaskDirectives, "host.v2.read:task_directives", filter)
	if err != nil {
		return pluginsdk.ExactTaskDirectivesPage{}, err
	}
	defer unlock()
	if q.TaskID != "" {
		task, readErr := h.taskData.GetTask(ctx, q.TaskID)
		if readErr != nil || task == nil || task.WorkspaceID != q.WorkspaceID {
			return pluginsdk.ExactTaskDirectivesPage{}, status.Error(codes.NotFound, "task was not found")
		}
	}
	store := h.commandStoreForDirective()
	if store == nil {
		return pluginsdk.ExactTaskDirectivesPage{}, status.Error(codes.Unavailable, "task directive store is unavailable")
	}
	items, pageInfo, err := exactPage(h, identity, q.RequestID, q.Page, func() ([]pluginsdk.TaskDirective, error) {
		records, err := store.ListTaskDirectives(ctx, h.installationID, q.WorkspaceID, q.TaskID)
		if err != nil {
			return nil, status.Error(codes.Unavailable, "task directive data is unavailable")
		}
		if len(records) > exactSnapshotMaxRows {
			return nil, status.Error(codes.ResourceExhausted, "task directive snapshot exceeds its bounded size")
		}
		out := make([]pluginsdk.TaskDirective, len(records))
		for index, record := range records {
			record, err = h.refreshTaskDirective(ctx, record, identity.capabilityRevision)
			if err != nil {
				return nil, err
			}
			out[index] = taskDirectiveToSDK(record)
		}
		return out, nil
	})
	if err != nil {
		return pluginsdk.ExactTaskDirectivesPage{}, err
	}
	return pluginsdk.ExactTaskDirectivesPage{Items: items, PageInfo: pageInfo}, nil
}
func (h *pluginHost) ListPendingTaskTransitions(ctx context.Context, q pluginsdk.ExactPendingTaskTransitionsQuery) (pluginsdk.ExactPendingTransitionsPage, error) {
	filter := map[string]string{"task_id": q.TaskID}
	identity, unlock, err := h.exactReadAdmission(ctx, q.RequestID, q.WorkspaceID, exactReadListTaskTransitions, "host.v2.read:task_transitions", filter)
	if err != nil {
		return pluginsdk.ExactPendingTransitionsPage{}, err
	}
	defer unlock()
	items, info, err := exactPage(h, identity, q.RequestID, q.Page, func() ([]pluginsdk.PendingTaskTransition, error) {
		tasks, loadErr := h.fetchAllTasksForWorkspace(ctx, q.WorkspaceID, true, true)
		if loadErr != nil {
			return nil, status.Error(codes.Unavailable, "workspace tasks are unavailable")
		}
		taskByID := make(map[string]*taskmodels.Task, len(tasks))
		for _, task := range tasks {
			if task != nil {
				taskByID[task.ID] = task
			}
		}
		if q.TaskID != "" && taskByID[q.TaskID] == nil {
			return nil, status.Error(codes.InvalidArgument, "pending transition filter contains a task outside the workspace")
		}
		observations, loadErr := h.loadExactPendingTaskTransitions(ctx, q.WorkspaceID, q.TaskID, taskByID)
		if loadErr != nil {
			return nil, loadErr
		}
		out := make([]pluginsdk.PendingTaskTransition, len(observations))
		for i := range observations {
			out[i] = observations[i].transition
		}
		return out, nil
	})
	return pluginsdk.ExactPendingTransitionsPage{Items: items, PageInfo: info}, err
}

func digestPublicValue(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "unavailable"
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func exactContainsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func exactElapsedMilliseconds(first, last *string) *int64 {
	if first == nil || last == nil {
		return nil
	}
	start, err := time.Parse(time.RFC3339Nano, *first)
	if err != nil {
		return nil
	}
	end, err := time.Parse(time.RFC3339Nano, *last)
	if err != nil || end.Before(start) {
		return nil
	}
	value := end.Sub(start).Milliseconds()
	return &value
}

var _ pluginsdk.ExactQueryManager = (*pluginHost)(nil)
