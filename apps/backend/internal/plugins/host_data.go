// host_data.go implements pluginHost's Tasks/Sessions/Workspaces/Workflows/
// AgentProfiles/Repositories accessors — the Host data API (ADR 0043:
// docs/decisions/0043-plugin-host-data-api.md). Each accessor is
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
	"strconv"
	"time"

	agentsettingsdto "github.com/kandev/kandev/internal/agent/settings/dto"
	analyticsmodels "github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

// Resource names gating the Host data API's read RPCs, per ADR 0043: each
// accessor requires "api_read:<resource>" in the plugin's manifest.
const (
	resourceTasks            = "tasks"
	resourceSessions         = "sessions"
	resourceWorkspaces       = "workspaces"
	resourceWorkflows        = "workflows"
	resourceAgentProfiles    = "agent_profiles"
	resourceExecutorProfiles = "executor_profiles"
	resourceRepositories     = "repositories"
	resourceMessages         = "messages"
	resourceInteractions     = "interactions"
)

// apiReadCapability formats resource as the api_read:<resource> capability
// name permissionDenied expects.
func apiReadCapability(resource string) string {
	return "api_read:" + resource
}

// apiWriteCapability formats resource as the api_write:<resource> capability
// name permissionDenied expects for the Host data API's write RPCs (ADR 0043).
func apiWriteCapability(resource string) string {
	return "api_write:" + resource
}

// Pagination: Page.Cursor is a decimal string offset into the server-side
// result set. It is an implementation detail plugins must treat as opaque
// (per ADR 0043's "opaque cursor" convention) — nothing here promises it
// stays a plain offset. defaultPageLimit/maxPageLimit bound Page.Limit.
const (
	defaultPageLimit = 50
	maxPageLimit     = 200
)

// maxMessageFilterValues bounds the combined number of session/task/type
// values a ListMessages filter may carry, keeping the resulting IN(...) bind
// parameters comfortably under SQLite's SQLITE_MAX_VARIABLE_NUMBER (999 on
// older builds) with headroom for the since/until/limit/offset params.
const maxMessageFilterValues = 400

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
	ListAllExecutorProfiles(ctx context.Context) ([]*taskmodels.ExecutorProfile, error)
	GetExecutor(ctx context.Context, id string) (*taskmodels.Executor, error)
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

// messageDataSource is the narrow slice of internal/task/service.Service the
// Messages().List RPC needs. ListMessagesForPlugin filters by session/task
// ids and a created_at time range, returning oldest-first with SQL-level
// limit/offset (Limit is requested as page-limit+1 so the reader can derive
// HasMore without a second count query).
type messageDataSource interface {
	ListMessagesForPlugin(ctx context.Context, filter taskmodels.PluginMessageFilter) ([]*taskmodels.Message, error)
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

// Tasks returns the task accessor. Unlike the read-only accessors below, it
// mixes read (List/Get, api_read:tasks) and write (Create/Update,
// api_write:tasks) methods whose capabilities gate independently, so the gate
// cannot live at the accessor — each method checks its own capability (reads
// here, writes in host_write.go).
func (h *pluginHost) Tasks() pluginsdk.TaskReader {
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

func (h *pluginHost) ExecutorProfiles() pluginsdk.ExecutorProfileReader {
	if !h.capabilities.CanRead(resourceExecutorProfiles) {
		return deniedExecutorProfileReader{}
	}
	if h.taskData == nil {
		return h.UnimplementedHostData.ExecutorProfiles()
	}
	return executorProfileReader{host: h}
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

// Messages mixes read (List, api_read:messages) and write (Send,
// api_write:messages) with independent capabilities, so — like Tasks() — the
// gate can't live at the accessor; each method checks its own capability (List
// here, Send in host_write.go).
func (h *pluginHost) Messages() pluginsdk.MessageReader {
	return messageReader{host: h}
}

// ── Denied readers ──────────────────────────────────────────────────────

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

type deniedExecutorProfileReader struct{}

func (deniedExecutorProfileReader) List(context.Context, pluginsdk.Page) ([]pluginsdk.ExecutorProfile, *pluginsdk.PageInfo, error) {
	return nil, nil, permissionDenied(apiReadCapability(resourceExecutorProfiles))
}

type deniedRepositoryReader struct{}

func (deniedRepositoryReader) List(context.Context, string, pluginsdk.Page) ([]pluginsdk.Repository, *pluginsdk.PageInfo, error) {
	return nil, nil, permissionDenied(apiReadCapability(resourceRepositories))
}

// ── Real readers ────────────────────────────────────────────────────────
//
// Only ever returned once the resource's capability gate has passed (see the
// accessors above), so none of these re-check it.

// taskFetchPageSize bounds each individual ListTasksByWorkspace call made
// while assembling a workspace's tasks for in-memory filter/sort/paginate.
// fetchTasksForWorkspaces loops pagination to completion per workspace (see
// its doc comment), so this only bounds one round trip's page size — it does
// NOT bound how many tasks a workspace can have before Host data API reads
// start silently dropping them.
const taskFetchPageSize = 1000

type taskReader struct{ host *pluginHost }

func (r taskReader) List(ctx context.Context, filter pluginsdk.TaskFilter, page pluginsdk.Page) ([]pluginsdk.Task, *pluginsdk.PageInfo, error) {
	if !r.host.capabilities.CanRead(resourceTasks) {
		return nil, nil, permissionDenied(apiReadCapability(resourceTasks))
	}
	if r.host.taskData == nil {
		return r.host.UnimplementedHostData.Tasks().List(ctx, filter, page)
	}
	workspaceIDs, err := r.host.resolveWorkspaceIDs(ctx, filter.WorkspaceIDs)
	if err != nil {
		return nil, nil, err
	}
	tasks, err := r.host.fetchTasksForWorkspaces(ctx, workspaceIDs, filter.IncludeEphemeral, false)
	if err != nil {
		return nil, nil, err
	}
	tasks = filterTasks(tasks, filter)
	sortTasksNewestFirst(tasks)
	items, info := paginate(tasksToDTOs(tasks), page)
	return items, info, nil
}

// Get returns a gRPC NotFound error (not a (nil, nil) success) when id
// doesn't resolve to a task, so the in-process contract matches exactly what
// a real plugin observes over the wire via grpcHostServer.GetTask.
func (r taskReader) Get(ctx context.Context, id string) (*pluginsdk.Task, error) {
	if !r.host.capabilities.CanRead(resourceTasks) {
		return nil, permissionDenied(apiReadCapability(resourceTasks))
	}
	if r.host.taskData == nil {
		return r.host.UnimplementedHostData.Tasks().Get(ctx, id)
	}
	task, err := r.host.taskData.GetTask(ctx, id)
	if err != nil {
		if errors.Is(err, repoerrors.ErrTaskNotFound) {
			return nil, taskNotFound(id)
		}
		return nil, err
	}
	dto := taskModelToDTO(task)
	return &dto, nil
}

type sessionReader struct{ host *pluginHost }

// List paginates the raw, already-sorted sessions BEFORE converting to DTOs:
// sessionToDTO resolves ACPSessionID via resolveACPSessionID, which issues a
// GetExecutorRunningBySessionID query for any session lacking the id in its
// metadata. Converting every fetched session (as opposed to just the
// returned page) would turn that into an O(N) fan-out of DB queries per read
// instead of O(limit).
func (r sessionReader) List(ctx context.Context, filter pluginsdk.SessionFilter, page pluginsdk.Page) ([]pluginsdk.Session, *pluginsdk.PageInfo, error) {
	sessions, err := r.host.fetchSessionsForFilter(ctx, filter)
	if err != nil {
		return nil, nil, err
	}
	sessions = filterSessionsByState(sessions, filter.States)
	sortSessionsNewestFirst(sessions)

	pageSessions, info := paginate(sessions, page)
	dtos := make([]pluginsdk.Session, len(pageSessions))
	for i, s := range pageSessions {
		dtos[i] = r.host.sessionToDTO(ctx, s)
	}
	return dtos, info, nil
}

// CodeStats delegates straight to the analytics service, which already
// paginates via SQL Limit/Offset (per ADR 0043(b), computed on demand — no
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

type executorProfileReader struct{ host *pluginHost }

func (r executorProfileReader) List(ctx context.Context, page pluginsdk.Page) ([]pluginsdk.ExecutorProfile, *pluginsdk.PageInfo, error) {
	profiles, err := r.host.taskData.ListAllExecutorProfiles(ctx)
	if err != nil {
		return nil, nil, err
	}
	validProfiles := make([]*taskmodels.ExecutorProfile, 0, len(profiles))
	for _, profile := range profiles {
		if profile != nil {
			validProfiles = append(validProfiles, profile)
		}
	}
	pageProfiles, info := paginate(validProfiles, page)
	dtos := make([]pluginsdk.ExecutorProfile, 0, len(pageProfiles))
	for _, profile := range pageProfiles {
		executor, err := r.host.taskData.GetExecutor(ctx, profile.ExecutorID)
		if err != nil && !errors.Is(err, taskmodels.ErrExecutorNotFound) {
			return nil, nil, err
		}
		executorType := ""
		if executor != nil {
			executorType = string(executor.Type)
		}
		dtos = append(dtos, pluginsdk.ExecutorProfile{
			ID: profile.ID, DisplayName: profile.Name, ExecutorType: executorType,
		})
	}
	return dtos, info, nil
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

type messageReader struct{ host *pluginHost }

// List paginates conversation content at the SQL layer (limit/offset, like
// sessionReader.CodeStats) rather than fetching everything into memory — a
// task's transcript can be very large. Since/Until are RFC3339; an
// unparseable value is a gRPC InvalidArgument. Content is sanitized by
// messageModelToDTO (kandev-system blocks stripped) before it reaches the
// plugin.
func (r messageReader) List(ctx context.Context, filter pluginsdk.MessageFilter, page pluginsdk.Page) ([]pluginsdk.Message, *pluginsdk.PageInfo, error) {
	if !r.host.capabilities.CanRead(resourceMessages) {
		return nil, nil, permissionDenied(apiReadCapability(resourceMessages))
	}
	if r.host.messageData == nil {
		return r.host.UnimplementedHostData.Messages().List(ctx, filter, page)
	}
	// Each session/task/type value becomes its own SQL bind parameter; cap
	// their combined count well under SQLite's host-parameter limit
	// (~500-999) so a large filter fails fast with a clear InvalidArgument
	// instead of a cryptic "too many SQL variables" at query execution.
	if n := len(filter.SessionIDs) + len(filter.TaskIDs) + len(filter.Types); n > maxMessageFilterValues {
		return nil, nil, invalidArgument(fmt.Sprintf("message filter has %d session/task/type values, max %d", n, maxMessageFilterValues))
	}
	since, err := parseFilterTime(filter.Since, "since")
	if err != nil {
		return nil, nil, err
	}
	until, err := parseFilterTime(filter.Until, "until")
	if err != nil {
		return nil, nil, err
	}
	limit := normalizePageLimit(page.Limit)
	offset := pageOffset(page.Cursor)

	messages, err := r.host.messageData.ListMessagesForPlugin(ctx, taskmodels.PluginMessageFilter{
		SessionIDs: filter.SessionIDs,
		TaskIDs:    filter.TaskIDs,
		Types:      filter.Types,
		Since:      since,
		Until:      until,
		Limit:      limit + 1,
		Offset:     offset,
	})
	if err != nil {
		return nil, nil, err
	}

	hasMore := len(messages) > limit
	if hasMore {
		messages = messages[:limit]
	}
	dtos := make([]pluginsdk.Message, len(messages))
	for i, m := range messages {
		dtos[i] = messageModelToDTO(m)
	}
	info := &pluginsdk.PageInfo{HasMore: hasMore}
	if hasMore {
		info.NextCursor = strconv.Itoa(offset + limit)
	}
	return dtos, info, nil
}

// parseFilterTime parses an optional RFC3339 filter bound, returning a gRPC
// InvalidArgument error naming the field when a non-nil value doesn't parse.
func parseFilterTime(value *string, field string) (*time.Time, error) {
	if value == nil || *value == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, *value)
	if err != nil {
		return nil, invalidArgument(fmt.Sprintf("%s must be RFC3339: %v", field, err))
	}
	return &t, nil
}
