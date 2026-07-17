// host_data.go implements pluginHost's Tasks/Sessions/Workspaces/Workflows/
// AgentProfiles/Repositories accessors — the Host data API (ADR 0042:
// docs/decisions/0042-plugin-host-data-api.md). Each accessor is
// capability-gated at the point it is called: a plugin whose manifest lacks
// the resource's api_read:<resource> capability gets back a reader whose
// every method returns gRPC PermissionDenied, so a real reader's methods
// never need to re-check the gate themselves.
//
// Reads never touch a repository directly — each real reader is backed by a
// narrow interface (taskDataSource, workflowLister, workflowStepLister,
// agentProfileDataSource, sessionCodeStatsSource) satisfied structurally by
// the real internal/task/service.Service, internal/workflow/service.Service,
// internal/agent/settings/controller.Controller, and
// internal/analytics/service.Service that backendapp wires in via
// Service.SetDataSources — mirroring how internal/plugins/delivery declares
// its own small Transport/PluginLister interfaces instead of importing this
// package's full surface.
package plugins

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	agentsettingsdto "github.com/kandev/kandev/internal/agent/settings/dto"
	analyticsmodels "github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

// Resource names gating the Host data API's read RPCs, per ADR 0042: each
// accessor requires "api_read:<resource>" in the plugin's manifest.
const (
	resourceTasks         = "tasks"
	resourceSessions      = "sessions"
	resourceWorkspaces    = "workspaces"
	resourceWorkflows     = "workflows"
	resourceAgentProfiles = "agent_profiles"
	resourceRepositories  = "repositories"
)

// apiReadCapability formats resource as the api_read:<resource> capability
// name permissionDenied expects.
func apiReadCapability(resource string) string {
	return "api_read:" + resource
}

// Pagination: Page.Cursor is a decimal string offset into the server-side
// result set. It is an implementation detail plugins must treat as opaque
// (per ADR 0042's "opaque cursor" convention) — nothing here promises it
// stays a plain offset. defaultPageLimit/maxPageLimit bound Page.Limit.
const (
	defaultPageLimit = 50
	maxPageLimit     = 200
)

// normalizePageLimit clamps limit to [1, maxPageLimit], defaulting to
// defaultPageLimit when unset or invalid.
func normalizePageLimit(limit int32) int {
	l := int(limit)
	if l <= 0 {
		return defaultPageLimit
	}
	if l > maxPageLimit {
		return maxPageLimit
	}
	return l
}

// pageOffset decodes cursor as the decimal offset it was encoded as; an
// empty, invalid, or negative cursor starts back at offset 0.
func pageOffset(cursor string) int {
	if cursor == "" {
		return 0
	}
	n, err := strconv.Atoi(cursor)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// paginate slices an already-fetched, already-ordered items slice per page's
// offset/limit and builds the PageInfo the RPC hands back.
func paginate[T any](items []T, page pluginsdk.Page) ([]T, *pluginsdk.PageInfo) {
	limit := normalizePageLimit(page.Limit)
	offset := pageOffset(page.Cursor)
	if offset >= len(items) {
		return []T{}, &pluginsdk.PageInfo{}
	}
	end := offset + limit
	hasMore := end < len(items)
	if end > len(items) {
		end = len(items)
	}
	info := &pluginsdk.PageInfo{HasMore: hasMore}
	if hasMore {
		info.NextCursor = strconv.Itoa(end)
	}
	return items[offset:end], info
}

// ── Narrow data-source interfaces ───────────────────────────────────────
//
// Each interface names exactly the methods this file calls, satisfied
// structurally (no adapter type needed) by the real service kandev already
// constructs: internal/task/service.Service already has every taskDataSource
// and workflowLister method; internal/workflow/service.Service already has
// ListStepsByWorkflow; internal/agent/settings/controller.Controller already
// has ListAgents; internal/analytics/service.Service already has
// ListSessionCodeStats.

// taskDataSource is the narrow slice of internal/task/service.Service the
// Tasks/Workspaces/Repositories/Sessions readers need.
type taskDataSource interface {
	ListWorkspaces(ctx context.Context) ([]*taskmodels.Workspace, error)
	ListTasksByWorkspace(ctx context.Context, workspaceID, workflowID, repositoryID, query string, page, pageSize int, sort string, includeArchived, includeEphemeral, onlyEphemeral, excludeConfig bool) ([]*taskmodels.Task, int, error)
	GetTask(ctx context.Context, id string) (*taskmodels.Task, error)
	ListRepositories(ctx context.Context, workspaceID string) ([]*taskmodels.Repository, error)
	ListTaskSessions(ctx context.Context, taskID string) ([]*taskmodels.TaskSession, error)
	GetExecutorRunningBySessionID(ctx context.Context, sessionID string) (*taskmodels.ExecutorRunning, error)
}

// workflowLister is the narrow slice of internal/task/service.Service the
// Workflows().List RPC needs (workflows themselves are owned by the task
// service, not internal/workflow/service — only steps are).
type workflowLister interface {
	ListWorkflows(ctx context.Context, workspaceID string, includeHidden bool) ([]*taskmodels.Workflow, error)
}

// workflowStepLister is the narrow slice of internal/workflow/service.Service
// the Workflows().ListSteps RPC needs.
type workflowStepLister interface {
	ListStepsByWorkflow(ctx context.Context, workflowID string) ([]*wfmodels.WorkflowStep, error)
}

// agentProfileDataSource is the narrow slice of
// internal/agent/settings/controller.Controller the AgentProfiles().List RPC
// needs. ListAgents already filters out workspace-scoped (office) profiles
// (see filterGlobalProfiles), matching the resource's global-instance scope.
type agentProfileDataSource interface {
	ListAgents(ctx context.Context) (*agentsettingsdto.ListAgentsResponse, error)
}

// sessionCodeStatsSource is the narrow slice of
// internal/analytics/service.Service the Sessions().CodeStats RPC needs.
type sessionCodeStatsSource interface {
	ListSessionCodeStats(ctx context.Context, filter analyticsmodels.SessionCodeStatsFilter) ([]*analyticsmodels.SessionCodeStats, error)
}

// ── pluginHost accessors ────────────────────────────────────────────────
//
// These shadow the Unimplemented* defaults embedded via
// pluginsdk.UnimplementedHostData: each checks the resource's api_read
// capability once, then hands back either the real, service-backed reader or
// a denied stub whose methods all return PermissionDenied. If the capability
// is granted but the corresponding data source was never wired (e.g.
// Service.SetDataSources not called — some tests build a bare pluginHost),
// this falls back to the embedded Unimplemented reader rather than a nil
// pointer dereference.

func (h *pluginHost) Tasks() pluginsdk.TaskReader {
	if !h.capabilities.CanRead(resourceTasks) {
		return deniedTaskReader{}
	}
	if h.taskData == nil {
		return h.UnimplementedHostData.Tasks()
	}
	return taskReader{host: h}
}

func (h *pluginHost) Sessions() pluginsdk.SessionReader {
	if !h.capabilities.CanRead(resourceSessions) {
		return deniedSessionReader{}
	}
	if h.taskData == nil || h.sessionCodeStats == nil {
		return h.UnimplementedHostData.Sessions()
	}
	return sessionReader{host: h}
}

func (h *pluginHost) Workspaces() pluginsdk.WorkspaceReader {
	if !h.capabilities.CanRead(resourceWorkspaces) {
		return deniedWorkspaceReader{}
	}
	if h.taskData == nil {
		return h.UnimplementedHostData.Workspaces()
	}
	return workspaceReader{host: h}
}

func (h *pluginHost) Workflows() pluginsdk.WorkflowReader {
	if !h.capabilities.CanRead(resourceWorkflows) {
		return deniedWorkflowReader{}
	}
	if h.workflows == nil || h.workflowSteps == nil {
		return h.UnimplementedHostData.Workflows()
	}
	return workflowReader{host: h}
}

func (h *pluginHost) AgentProfiles() pluginsdk.AgentProfileReader {
	if !h.capabilities.CanRead(resourceAgentProfiles) {
		return deniedAgentProfileReader{}
	}
	if h.agentProfiles == nil {
		return h.UnimplementedHostData.AgentProfiles()
	}
	return agentProfileReader{host: h}
}

func (h *pluginHost) Repositories() pluginsdk.RepositoryReader {
	if !h.capabilities.CanRead(resourceRepositories) {
		return deniedRepositoryReader{}
	}
	if h.taskData == nil {
		return h.UnimplementedHostData.Repositories()
	}
	return repositoryReader{host: h}
}

// ── Denied readers ──────────────────────────────────────────────────────

type deniedTaskReader struct{}

func (deniedTaskReader) List(context.Context, pluginsdk.TaskFilter, pluginsdk.Page) ([]pluginsdk.Task, *pluginsdk.PageInfo, error) {
	return nil, nil, permissionDenied(apiReadCapability(resourceTasks))
}

func (deniedTaskReader) Get(context.Context, string) (*pluginsdk.Task, error) {
	return nil, permissionDenied(apiReadCapability(resourceTasks))
}

type deniedSessionReader struct{}

func (deniedSessionReader) List(context.Context, pluginsdk.SessionFilter, pluginsdk.Page) ([]pluginsdk.Session, *pluginsdk.PageInfo, error) {
	return nil, nil, permissionDenied(apiReadCapability(resourceSessions))
}

func (deniedSessionReader) CodeStats(context.Context, pluginsdk.SessionFilter, pluginsdk.Page) ([]pluginsdk.SessionCodeStats, *pluginsdk.PageInfo, error) {
	return nil, nil, permissionDenied(apiReadCapability(resourceSessions))
}

type deniedWorkspaceReader struct{}

func (deniedWorkspaceReader) List(context.Context, pluginsdk.Page) ([]pluginsdk.Workspace, *pluginsdk.PageInfo, error) {
	return nil, nil, permissionDenied(apiReadCapability(resourceWorkspaces))
}

type deniedWorkflowReader struct{}

func (deniedWorkflowReader) List(context.Context, string, pluginsdk.Page) ([]pluginsdk.Workflow, *pluginsdk.PageInfo, error) {
	return nil, nil, permissionDenied(apiReadCapability(resourceWorkflows))
}

func (deniedWorkflowReader) ListSteps(context.Context, string) ([]pluginsdk.WorkflowStep, error) {
	return nil, permissionDenied(apiReadCapability(resourceWorkflows))
}

type deniedAgentProfileReader struct{}

func (deniedAgentProfileReader) List(context.Context, pluginsdk.Page) ([]pluginsdk.AgentProfile, *pluginsdk.PageInfo, error) {
	return nil, nil, permissionDenied(apiReadCapability(resourceAgentProfiles))
}

type deniedRepositoryReader struct{}

func (deniedRepositoryReader) List(context.Context, string, pluginsdk.Page) ([]pluginsdk.Repository, *pluginsdk.PageInfo, error) {
	return nil, nil, permissionDenied(apiReadCapability(resourceRepositories))
}

// ── Real readers ────────────────────────────────────────────────────────
//
// Only ever returned once the resource's capability gate has passed (see the
// accessors above), so none of these re-check it.

// taskFetchPageSize bounds a single ListTasksByWorkspace call made while
// assembling a workspace's tasks for in-memory filter/sort/paginate. Large
// enough for realistic instance sizes; a workspace with more tasks than this
// would silently lose the excess from Host data API reads — a known v1
// limitation of fetch-then-paginate-in-memory, acceptable per ADR 0042(a)'s
// "global-with-hook" v1 scoping recommendation.
const taskFetchPageSize = 1000

type taskReader struct{ host *pluginHost }

func (r taskReader) List(ctx context.Context, filter pluginsdk.TaskFilter, page pluginsdk.Page) ([]pluginsdk.Task, *pluginsdk.PageInfo, error) {
	workspaceIDs, err := r.host.resolveWorkspaceIDs(ctx, filter.WorkspaceIDs)
	if err != nil {
		return nil, nil, err
	}
	tasks, err := r.host.fetchTasksForWorkspaces(ctx, workspaceIDs, filter.IncludeEphemeral)
	if err != nil {
		return nil, nil, err
	}
	tasks = filterTasks(tasks, filter)
	sortTasksNewestFirst(tasks)
	items, info := paginate(tasksToDTOs(tasks), page)
	return items, info, nil
}

func (r taskReader) Get(ctx context.Context, id string) (*pluginsdk.Task, error) {
	task, err := r.host.taskData.GetTask(ctx, id)
	if err != nil {
		if errors.Is(err, repoerrors.ErrTaskNotFound) {
			return nil, nil
		}
		return nil, err
	}
	dto := taskModelToDTO(task)
	return &dto, nil
}

type sessionReader struct{ host *pluginHost }

func (r sessionReader) List(ctx context.Context, filter pluginsdk.SessionFilter, page pluginsdk.Page) ([]pluginsdk.Session, *pluginsdk.PageInfo, error) {
	sessions, err := r.host.fetchSessionsForFilter(ctx, filter)
	if err != nil {
		return nil, nil, err
	}
	sessions = filterSessionsByState(sessions, filter.States)
	sortSessionsNewestFirst(sessions)

	dtos := make([]pluginsdk.Session, len(sessions))
	for i, s := range sessions {
		dtos[i] = r.host.sessionToDTO(ctx, s)
	}
	items, info := paginate(dtos, page)
	return items, info, nil
}

// CodeStats delegates straight to the analytics service, which already
// paginates via SQL Limit/Offset (per ADR 0042(b), computed on demand — no
// in-memory fetch-everything like the other readers). It asks for one extra
// row past the requested limit to derive HasMore without a second count
// query; NextCursor is offset+limit exactly like the in-memory paginate
// helper, keeping cursor semantics uniform across every Host data reader.
func (r sessionReader) CodeStats(ctx context.Context, filter pluginsdk.SessionFilter, page pluginsdk.Page) ([]pluginsdk.SessionCodeStats, *pluginsdk.PageInfo, error) {
	limit := normalizePageLimit(page.Limit)
	offset := pageOffset(page.Cursor)

	stats, err := r.host.sessionCodeStats.ListSessionCodeStats(ctx, analyticsmodels.SessionCodeStatsFilter{
		TaskIDs:      filter.TaskIDs,
		WorkspaceIDs: filter.WorkspaceIDs,
		States:       filter.States,
		Limit:        limit + 1,
		Offset:       offset,
	})
	if err != nil {
		return nil, nil, err
	}

	hasMore := len(stats) > limit
	if hasMore {
		stats = stats[:limit]
	}
	dtos := make([]pluginsdk.SessionCodeStats, len(stats))
	for i, s := range stats {
		dtos[i] = sessionCodeStatsModelToDTO(s)
	}
	info := &pluginsdk.PageInfo{HasMore: hasMore}
	if hasMore {
		info.NextCursor = strconv.Itoa(offset + limit)
	}
	return dtos, info, nil
}

type workspaceReader struct{ host *pluginHost }

func (r workspaceReader) List(ctx context.Context, page pluginsdk.Page) ([]pluginsdk.Workspace, *pluginsdk.PageInfo, error) {
	workspaces, err := r.host.taskData.ListWorkspaces(ctx)
	if err != nil {
		return nil, nil, err
	}
	dtos := make([]pluginsdk.Workspace, len(workspaces))
	for i, w := range workspaces {
		dtos[i] = workspaceModelToDTO(w)
	}
	items, info := paginate(dtos, page)
	return items, info, nil
}

type workflowReader struct{ host *pluginHost }

// List does not surface hidden workflows (e.g. system-only flows like
// Improve Kandev) — the Host data API's WorkflowReader has no includeHidden
// filter yet, so this reader defaults to the same "hidden by default"
// behavior most kandev UI listings use.
func (r workflowReader) List(ctx context.Context, workspaceID string, page pluginsdk.Page) ([]pluginsdk.Workflow, *pluginsdk.PageInfo, error) {
	workflows, err := r.host.workflows.ListWorkflows(ctx, workspaceID, false)
	if err != nil {
		return nil, nil, err
	}
	dtos := make([]pluginsdk.Workflow, len(workflows))
	for i, w := range workflows {
		dtos[i] = workflowModelToDTO(w)
	}
	items, info := paginate(dtos, page)
	return items, info, nil
}

func (r workflowReader) ListSteps(ctx context.Context, workflowID string) ([]pluginsdk.WorkflowStep, error) {
	steps, err := r.host.workflowSteps.ListStepsByWorkflow(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	dtos := make([]pluginsdk.WorkflowStep, len(steps))
	for i, s := range steps {
		dtos[i] = workflowStepModelToDTO(s)
	}
	return dtos, nil
}

type agentProfileReader struct{ host *pluginHost }

func (r agentProfileReader) List(ctx context.Context, page pluginsdk.Page) ([]pluginsdk.AgentProfile, *pluginsdk.PageInfo, error) {
	resp, err := r.host.agentProfiles.ListAgents(ctx)
	if err != nil {
		return nil, nil, err
	}
	var dtos []pluginsdk.AgentProfile
	for _, agent := range resp.Agents {
		for _, profile := range agent.Profiles {
			dtos = append(dtos, agentProfileDTOToSDK(profile))
		}
	}
	items, info := paginate(dtos, page)
	return items, info, nil
}

type repositoryReader struct{ host *pluginHost }

func (r repositoryReader) List(ctx context.Context, workspaceID string, page pluginsdk.Page) ([]pluginsdk.Repository, *pluginsdk.PageInfo, error) {
	repos, err := r.host.taskData.ListRepositories(ctx, workspaceID)
	if err != nil {
		return nil, nil, err
	}
	dtos := make([]pluginsdk.Repository, len(repos))
	for i, repository := range repos {
		dtos[i] = repositoryModelToDTO(repository)
	}
	items, info := paginate(dtos, page)
	return items, info, nil
}

// ── Fetch/filter/sort helpers (v1 scoping: ADR 0042(a) "global-with-hook") ─

// resolveWorkspaceIDs returns requested unchanged when non-empty (an
// explicit filter always narrows), otherwise every workspace the instance
// holds — this is the "global reads, filters narrow results" v1 scoping
// rule, and the single hook a future per-plugin/per-user workspace
// restriction would replace.
func (h *pluginHost) resolveWorkspaceIDs(ctx context.Context, requested []string) ([]string, error) {
	if len(requested) > 0 {
		return requested, nil
	}
	workspaces, err := h.taskData.ListWorkspaces(ctx)
	if err != nil {
		return nil, fmt.Errorf("plugins: list workspaces: %w", err)
	}
	ids := make([]string, len(workspaces))
	for i, w := range workspaces {
		ids[i] = w.ID
	}
	return ids, nil
}

// fetchTasksForWorkspaces concatenates up to taskFetchPageSize tasks per
// workspace in workspaceIDs. excludeConfig is always true: office config-mode
// tasks (json_extract(metadata, '$.config_mode')) are internal bookkeeping,
// not plugin-visible work items.
func (h *pluginHost) fetchTasksForWorkspaces(ctx context.Context, workspaceIDs []string, includeEphemeral bool) ([]*taskmodels.Task, error) {
	var all []*taskmodels.Task
	for _, workspaceID := range workspaceIDs {
		tasks, _, err := h.taskData.ListTasksByWorkspace(
			ctx, workspaceID, "", "", "", 1, taskFetchPageSize, "",
			false, includeEphemeral, false, true,
		)
		if err != nil {
			return nil, fmt.Errorf("plugins: list tasks for workspace %q: %w", workspaceID, err)
		}
		all = append(all, tasks...)
	}
	return all, nil
}

// filterTasks applies TaskFilter's WorkflowIDs/States/ParentID narrowing
// on top of the already-workspace-scoped tasks fetchTasksForWorkspaces
// returned (WorkspaceIDs and IncludeEphemeral are applied earlier, at fetch
// time).
func filterTasks(tasks []*taskmodels.Task, filter pluginsdk.TaskFilter) []*taskmodels.Task {
	if len(filter.WorkflowIDs) == 0 && len(filter.States) == 0 && filter.ParentID == nil {
		return tasks
	}
	workflowSet := toSet(filter.WorkflowIDs)
	stateSet := toSet(filter.States)
	out := make([]*taskmodels.Task, 0, len(tasks))
	for _, t := range tasks {
		if len(workflowSet) > 0 && !workflowSet[t.WorkflowID] {
			continue
		}
		if len(stateSet) > 0 && !stateSet[string(t.State)] {
			continue
		}
		if filter.ParentID != nil && t.ParentID != *filter.ParentID {
			continue
		}
		out = append(out, t)
	}
	return out
}

func toSet(values []string) map[string]bool {
	if len(values) == 0 {
		return nil
	}
	set := make(map[string]bool, len(values))
	for _, v := range values {
		set[v] = true
	}
	return set
}

func sortTasksNewestFirst(tasks []*taskmodels.Task) {
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].CreatedAt.After(tasks[j].CreatedAt) })
}

func tasksToDTOs(tasks []*taskmodels.Task) []pluginsdk.Task {
	out := make([]pluginsdk.Task, len(tasks))
	for i, t := range tasks {
		out[i] = taskModelToDTO(t)
	}
	return out
}

// fetchSessionsForFilter resolves filter.TaskIDs directly when given, or
// otherwise enumerates every task across filter.WorkspaceIDs (or every
// workspace) and lists each task's sessions — a Host data API session read
// with no TaskIDs filter is, unavoidably, an N+1 fan-out over the instance's
// tasks in v1 (no session listing endpoint spans multiple tasks directly at
// the service layer today). includeEphemeral is always true here: an
// ephemeral (quick-chat) task's sessions are still real sessions from the
// Sessions resource's point of view.
func (h *pluginHost) fetchSessionsForFilter(ctx context.Context, filter pluginsdk.SessionFilter) ([]*taskmodels.TaskSession, error) {
	taskIDs := filter.TaskIDs
	if len(taskIDs) == 0 {
		workspaceIDs, err := h.resolveWorkspaceIDs(ctx, filter.WorkspaceIDs)
		if err != nil {
			return nil, err
		}
		tasks, err := h.fetchTasksForWorkspaces(ctx, workspaceIDs, true)
		if err != nil {
			return nil, err
		}
		taskIDs = make([]string, len(tasks))
		for i, t := range tasks {
			taskIDs[i] = t.ID
		}
	}

	var sessions []*taskmodels.TaskSession
	for _, taskID := range taskIDs {
		s, err := h.taskData.ListTaskSessions(ctx, taskID)
		if err != nil {
			return nil, fmt.Errorf("plugins: list sessions for task %q: %w", taskID, err)
		}
		sessions = append(sessions, s...)
	}
	return sessions, nil
}

func filterSessionsByState(sessions []*taskmodels.TaskSession, states []string) []*taskmodels.TaskSession {
	if len(states) == 0 {
		return sessions
	}
	set := toSet(states)
	out := make([]*taskmodels.TaskSession, 0, len(sessions))
	for _, s := range sessions {
		if set[string(s.State)] {
			out = append(out, s)
		}
	}
	return out
}

func sortSessionsNewestFirst(sessions []*taskmodels.TaskSession) {
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].StartedAt.After(sessions[j].StartedAt) })
}

// sessionToDTO maps a TaskSession to the Go-native Session DTO, resolving
// ACPSessionID via resolveACPSessionID's metadata-then-executors_running
// fallback.
func (h *pluginHost) sessionToDTO(ctx context.Context, s *taskmodels.TaskSession) pluginsdk.Session {
	return pluginsdk.Session{
		ID:               s.ID,
		TaskID:           s.TaskID,
		AgentProfileID:   s.AgentProfileID,
		AgentDisplayName: sessionSnapshotString(s.AgentProfileSnapshot, "agent_display_name"),
		Model:            sessionSnapshotModel(s.AgentProfileSnapshot),
		ACPSessionID:     h.resolveACPSessionID(ctx, s),
		State:            string(s.State),
		StartedAt:        s.StartedAt.UTC().Format(time.RFC3339),
		EndedAt:          timePtrToRFC3339(s.CompletedAt),
	}
}

// resolveACPSessionID replicates the source agent-stats plugin's join key
// (docs/decisions/0042-plugin-host-data-api.md, "A real plugin exposed the
// gap"): the agent CLI's own session UUID at
// TaskSession.Metadata["acp"]["session_id"], populated once the agent emits
// a session_info frame. executors_running.resume_token carries the same id
// and survives on sessions that never got that far, so it is a best-effort
// fallback — a lookup failure (including "no such row") is silently treated
// as "no id available" rather than failing the whole read.
func (h *pluginHost) resolveACPSessionID(ctx context.Context, s *taskmodels.TaskSession) string {
	if id := acpSessionIDFromMetadata(s.Metadata); id != "" {
		return id
	}
	running, err := h.taskData.GetExecutorRunningBySessionID(ctx, s.ID)
	if err != nil || running == nil {
		return ""
	}
	return running.ResumeToken
}

func acpSessionIDFromMetadata(metadata map[string]any) string {
	acp, ok := metadata["acp"].(map[string]any)
	if !ok {
		return ""
	}
	id, _ := acp["session_id"].(string)
	return id
}

func sessionSnapshotString(snapshot map[string]any, key string) string {
	if snapshot == nil {
		return ""
	}
	v, _ := snapshot[key].(string)
	return v
}

// sessionSnapshotModel mirrors the source plugin's fallback chain for the
// agent-profile snapshot's model field, which has varied key names across
// agent types over time.
func sessionSnapshotModel(snapshot map[string]any) string {
	for _, key := range []string{"model", "model_name", "llm"} {
		if v := sessionSnapshotString(snapshot, key); v != "" {
			return v
		}
	}
	return ""
}

func timePtrToRFC3339(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}

func stringPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ── Internal model → pluginsdk DTO mapping ──────────────────────────────

func taskModelToDTO(t *taskmodels.Task) pluginsdk.Task {
	repos := make([]pluginsdk.TaskRepository, len(t.Repositories))
	for i, r := range t.Repositories {
		repos[i] = pluginsdk.TaskRepository{
			ID:           r.ID,
			RepositoryID: r.RepositoryID,
			BaseBranch:   r.BaseBranch,
			Position:     int32(r.Position),
		}
	}
	return pluginsdk.Task{
		ID:          t.ID,
		WorkspaceID: t.WorkspaceID,
		WorkflowID:  t.WorkflowID,
		Title:       t.Title,
		Description: t.Description,
		State:       string(t.State),
		Priority:    t.Priority,
		// CreatedBy: kandev's Task model has no creating-user column — Origin
		// ("manual"/"agent_created"/"routine"/"automation_run") is the
		// closest analogue and is what this surfaces.
		CreatedBy: t.Origin,
		CreatedAt: t.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: t.UpdatedAt.UTC().Format(time.RFC3339),
		// StartedAt/CompletedAt: the Task model has no started_at/completed_at
		// columns (ArchivedAt is a different concept); left nil in v1.
		ParentID:     stringPtrOrNil(t.ParentID),
		Identifier:   t.Identifier,
		IsEphemeral:  t.IsEphemeral,
		Repositories: repos,
		Metadata:     t.Metadata,
	}
}

func workspaceModelToDTO(w *taskmodels.Workspace) pluginsdk.Workspace {
	return pluginsdk.Workspace{
		ID:                    w.ID,
		Name:                  w.Name,
		Description:           stringPtrOrNil(w.Description),
		OwnerID:               w.OwnerID,
		DefaultExecutorID:     w.DefaultExecutorID,
		DefaultAgentProfileID: w.DefaultAgentProfileID,
		CreatedAt:             w.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:             w.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func workflowModelToDTO(w *taskmodels.Workflow) pluginsdk.Workflow {
	return pluginsdk.Workflow{
		ID:          w.ID,
		WorkspaceID: w.WorkspaceID,
		Name:        w.Name,
		Description: stringPtrOrNil(w.Description),
		SortOrder:   int32(w.SortOrder),
		CreatedAt:   w.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:   w.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func workflowStepModelToDTO(s *wfmodels.WorkflowStep) pluginsdk.WorkflowStep {
	return pluginsdk.WorkflowStep{
		ID:         s.ID,
		WorkflowID: s.WorkflowID,
		Name:       s.Name,
		Position:   int32(s.Position),
		StageType:  string(s.StageType),
	}
}

func repositoryModelToDTO(r *taskmodels.Repository) pluginsdk.Repository {
	return pluginsdk.Repository{
		ID:            r.ID,
		WorkspaceID:   r.WorkspaceID,
		Name:          r.Name,
		DefaultBranch: stringPtrOrNil(r.DefaultBranch),
	}
}

func agentProfileDTOToSDK(p agentsettingsdto.AgentProfileDTO) pluginsdk.AgentProfile {
	return pluginsdk.AgentProfile{
		ID:          p.ID,
		AgentID:     p.AgentID,
		DisplayName: p.AgentDisplayName,
		Name:        p.Name,
		Model:       p.Model,
		Mode:        p.Mode,
	}
}

func sessionCodeStatsModelToDTO(s *analyticsmodels.SessionCodeStats) pluginsdk.SessionCodeStats {
	return pluginsdk.SessionCodeStats{
		SessionID:               s.SessionID,
		LinesAddedCommitted:     s.LinesAddedCommitted,
		LinesDeletedCommitted:   s.LinesDeletedCommitted,
		LinesAddedPeakPending:   s.LinesAddedPeakPending,
		LinesDeletedPeakPending: s.LinesDeletedPeakPending,
	}
}
