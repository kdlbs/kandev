// data_types.go defines the Go-native mirrors of the Host data API messages
// (ADR 0043: docs/decisions/0043-plugin-host-data-api.md) plus the
// proto<->Go conversion helpers used by host.go's data accessors. As with
// the rest of types.go, authors and kandev's runtime manager only ever see
// these Go-native structs; the pluginv1 message types never leak past the
// package boundary.
//
// Conventions (mirroring the wire contract):
//   - Timestamps are RFC3339 strings, matching the JSON API and Event
//     envelope.
//   - Optional/nullable fields use *string, matching proto3 `optional`, so
//     "absent" is distinguishable from "empty".
//   - Free-form metadata uses map[string]any via mapToStruct/structToMap,
//     exactly like StateEntry.Value and Event.Payload.
package pluginsdk

import (
	"fmt"

	pluginv1 "github.com/kandev/kandev/proto/kandev/plugin/v1"
)

// Page is the Go-native mirror of kandev.plugin.v1.Page: an opaque-cursor
// pagination request. A zero Page requests the server's default page.
type Page struct {
	Limit  int32
	Cursor string
}

func (p Page) toProto() *pluginv1.Page {
	return &pluginv1.Page{Limit: p.Limit, Cursor: p.Cursor}
}

func pageFromProto(p *pluginv1.Page) Page {
	if p == nil {
		return Page{}
	}
	return Page{Limit: p.GetLimit(), Cursor: p.GetCursor()}
}

// PageInfo is the Go-native mirror of kandev.plugin.v1.PageInfo, returned
// alongside every paginated Host data read.
type PageInfo struct {
	NextCursor string
	HasMore    bool
}

func (p *PageInfo) toProto() *pluginv1.PageInfo {
	if p == nil {
		return nil
	}
	return &pluginv1.PageInfo{NextCursor: p.NextCursor, HasMore: p.HasMore}
}

func pageInfoFromProto(p *pluginv1.PageInfo) *PageInfo {
	if p == nil {
		return nil
	}
	return &PageInfo{NextCursor: p.GetNextCursor(), HasMore: p.GetHasMore()}
}

// TaskRepository is the Go-native mirror of kandev.plugin.v1.TaskRepository.
type TaskRepository struct {
	ID             string
	RepositoryID   string
	BaseBranch     string
	Position       int32
	CheckoutBranch string
}

func (r TaskRepository) toProto() *pluginv1.TaskRepository {
	return &pluginv1.TaskRepository{
		Id:             r.ID,
		RepositoryId:   r.RepositoryID,
		BaseBranch:     r.BaseBranch,
		Position:       r.Position,
		CheckoutBranch: r.CheckoutBranch,
	}
}

func taskRepositoryFromProto(p *pluginv1.TaskRepository) TaskRepository {
	if p == nil {
		return TaskRepository{}
	}
	return TaskRepository{
		ID:             p.GetId(),
		RepositoryID:   p.GetRepositoryId(),
		BaseBranch:     p.GetBaseBranch(),
		Position:       p.GetPosition(),
		CheckoutBranch: p.GetCheckoutBranch(),
	}
}

// Task is the Go-native mirror of kandev.plugin.v1.Task.
type Task struct {
	ID           string
	WorkspaceID  string
	WorkflowID   string
	Title        string
	Description  string
	State        string
	Priority     string
	CreatedBy    string
	CreatedAt    string
	UpdatedAt    string
	StartedAt    *string
	CompletedAt  *string
	ParentID     *string
	Identifier   string
	IsEphemeral  bool
	Repositories []TaskRepository
	Metadata     map[string]any
	// ArchivedAt is nil for a live task. Only ever populated when the read asked
	// for archived tasks (TaskFilter.IncludeArchived).
	ArchivedAt *string
	// PullRequests are the changes opened for this task, newest first. The link
	// is not derivable from any other field on Task: CheckoutBranch is the BASE
	// branch, and a PR title is rewritten to conventional-commit form.
	PullRequests []TaskPullRequest
	// WorkflowStepID is where on the board the task stands. State says how it is
	// going; this says where it is.
	WorkflowStepID         string
	Position               int32
	AssigneeAgentProfileID string
	Labels                 []string
	Autopilot              bool
	WIPAdmitted            bool
	QueuedForStepID        string
	QueuedAt               *string
	ProjectID              string
	ExternalID             string
}

// TaskPullRequest is one change opened for a task. Provider-neutral by design:
// review metadata specific to one forge stays out of the plugin contract.
type TaskPullRequest struct {
	Number     int64
	URL        string
	Title      string
	State      string // "open" | "closed" | "merged"
	HeadBranch string
	BaseBranch string
	IsDraft    bool
	Provider   string // "github" | "gitlab"
	MergedAt   *string
	ClosedAt   *string
	// Readiness, already synced by kandev's PR watcher. Carried so a plugin
	// asking "what can merge" does not re-fetch all of it from the forge.
	ReviewState             string
	ChecksState             string
	MergeableState          string
	UnresolvedReviewThreads int32
	ChecksTotal             int32
	ChecksPassing           int32
	Additions               int32
	Deletions               int32
	AuthorLogin             string
}

func (r TaskPullRequest) toProto() *pluginv1.TaskPullRequest {
	return &pluginv1.TaskPullRequest{
		Number: r.Number, Url: r.URL, Title: r.Title, State: r.State,
		HeadBranch: r.HeadBranch, BaseBranch: r.BaseBranch, IsDraft: r.IsDraft,
		Provider: r.Provider, MergedAt: r.MergedAt, ClosedAt: r.ClosedAt,
		ReviewState: r.ReviewState, ChecksState: r.ChecksState,
		MergeableState: r.MergeableState, UnresolvedReviewThreads: r.UnresolvedReviewThreads,
		ChecksTotal: r.ChecksTotal, ChecksPassing: r.ChecksPassing,
		Additions: r.Additions, Deletions: r.Deletions, AuthorLogin: r.AuthorLogin,
	}
}

func taskPullRequestFromProto(p *pluginv1.TaskPullRequest) TaskPullRequest {
	if p == nil {
		return TaskPullRequest{}
	}
	return TaskPullRequest{
		Number: p.GetNumber(), URL: p.GetUrl(), Title: p.GetTitle(), State: p.GetState(),
		HeadBranch: p.GetHeadBranch(), BaseBranch: p.GetBaseBranch(), IsDraft: p.GetIsDraft(),
		Provider: p.GetProvider(), MergedAt: p.MergedAt, ClosedAt: p.ClosedAt,
		ReviewState: p.GetReviewState(), ChecksState: p.GetChecksState(),
		MergeableState: p.GetMergeableState(), UnresolvedReviewThreads: p.GetUnresolvedReviewThreads(),
		ChecksTotal: p.GetChecksTotal(), ChecksPassing: p.GetChecksPassing(),
		Additions: p.GetAdditions(), Deletions: p.GetDeletions(), AuthorLogin: p.GetAuthorLogin(),
	}
}

func (t Task) toProto() (*pluginv1.Task, error) {
	metadata, err := mapToStruct(t.Metadata)
	if err != nil {
		return nil, fmt.Errorf("pluginsdk: task metadata: %w", err)
	}
	var repos []*pluginv1.TaskRepository
	if len(t.Repositories) > 0 {
		repos = make([]*pluginv1.TaskRepository, len(t.Repositories))
		for i := range t.Repositories {
			repos[i] = t.Repositories[i].toProto()
		}
	}
	return &pluginv1.Task{
		Id:           t.ID,
		WorkspaceId:  t.WorkspaceID,
		WorkflowId:   t.WorkflowID,
		Title:        t.Title,
		Description:  t.Description,
		State:        t.State,
		Priority:     t.Priority,
		CreatedBy:    t.CreatedBy,
		CreatedAt:    t.CreatedAt,
		UpdatedAt:    t.UpdatedAt,
		StartedAt:    t.StartedAt,
		CompletedAt:  t.CompletedAt,
		ParentId:     t.ParentID,
		Identifier:   t.Identifier,
		IsEphemeral:  t.IsEphemeral,
		Repositories: repos,
		Metadata:     metadata,

		ArchivedAt:             t.ArchivedAt,
		PullRequests:           taskPullRequestsToProto(t.PullRequests),
		WorkflowStepId:         t.WorkflowStepID,
		Position:               t.Position,
		AssigneeAgentProfileId: t.AssigneeAgentProfileID,
		Labels:                 t.Labels,
		Autopilot:              t.Autopilot,
		WipAdmitted:            t.WIPAdmitted,
		QueuedForStepId:        t.QueuedForStepID,
		QueuedAt:               t.QueuedAt,
		ProjectId:              t.ProjectID,
		ExternalId:             t.ExternalID,
	}, nil
}

func taskPullRequestsToProto(in []TaskPullRequest) []*pluginv1.TaskPullRequest {
	if len(in) == 0 {
		return nil
	}
	out := make([]*pluginv1.TaskPullRequest, len(in))
	for i := range in {
		out[i] = in[i].toProto()
	}
	return out
}

func taskPullRequestsFromProto(in []*pluginv1.TaskPullRequest) []TaskPullRequest {
	if len(in) == 0 {
		return nil
	}
	out := make([]TaskPullRequest, len(in))
	for i, r := range in {
		out[i] = taskPullRequestFromProto(r)
	}
	return out
}

func taskFromProto(p *pluginv1.Task) (Task, error) {
	if p == nil {
		return Task{}, nil
	}
	metadata, err := structToMap(p.GetMetadata())
	if err != nil {
		return Task{}, fmt.Errorf("pluginsdk: task metadata: %w", err)
	}
	var repos []TaskRepository
	if len(p.GetRepositories()) > 0 {
		repos = make([]TaskRepository, len(p.GetRepositories()))
		for i, r := range p.GetRepositories() {
			repos[i] = taskRepositoryFromProto(r)
		}
	}
	return Task{
		ID:           p.GetId(),
		WorkspaceID:  p.GetWorkspaceId(),
		WorkflowID:   p.GetWorkflowId(),
		Title:        p.GetTitle(),
		Description:  p.GetDescription(),
		State:        p.GetState(),
		Priority:     p.GetPriority(),
		CreatedBy:    p.GetCreatedBy(),
		CreatedAt:    p.GetCreatedAt(),
		UpdatedAt:    p.GetUpdatedAt(),
		StartedAt:    p.StartedAt,
		CompletedAt:  p.CompletedAt,
		ParentID:     p.ParentId,
		Identifier:   p.GetIdentifier(),
		IsEphemeral:  p.GetIsEphemeral(),
		Repositories: repos,
		Metadata:     metadata,

		ArchivedAt:             p.ArchivedAt,
		PullRequests:           taskPullRequestsFromProto(p.GetPullRequests()),
		WorkflowStepID:         p.GetWorkflowStepId(),
		Position:               p.GetPosition(),
		AssigneeAgentProfileID: p.GetAssigneeAgentProfileId(),
		Labels:                 p.GetLabels(),
		Autopilot:              p.GetAutopilot(),
		WIPAdmitted:            p.GetWipAdmitted(),
		QueuedForStepID:        p.GetQueuedForStepId(),
		QueuedAt:               p.QueuedAt,
		ProjectID:              p.GetProjectId(),
		ExternalID:             p.GetExternalId(),
	}, nil
}

func tasksFromProto(items []*pluginv1.Task) ([]Task, error) {
	if len(items) == 0 {
		return nil, nil
	}
	out := make([]Task, len(items))
	for i, item := range items {
		converted, err := taskFromProto(item)
		if err != nil {
			return nil, err
		}
		out[i] = converted
	}
	return out, nil
}

func tasksToProto(items []Task) ([]*pluginv1.Task, error) {
	if len(items) == 0 {
		return nil, nil
	}
	out := make([]*pluginv1.Task, len(items))
	for i := range items {
		converted, err := items[i].toProto()
		if err != nil {
			return nil, err
		}
		out[i] = converted
	}
	return out, nil
}

// TaskFilter is the Go-native mirror of kandev.plugin.v1.TaskFilter.
type TaskFilter struct {
	WorkspaceIDs     []string
	WorkflowIDs      []string
	States           []string
	ParentID         *string
	IncludeEphemeral bool
	// IncludeArchived defaults false, matching the behaviour before this field
	// existed: a plugin that does not ask still sees only live tasks. A plugin
	// reporting on delivery needs true — an archived task is usually a
	// delivered one, and omitting it makes finished work look like it never
	// happened.
	IncludeArchived bool
}

func (f TaskFilter) toProto() *pluginv1.TaskFilter {
	return &pluginv1.TaskFilter{
		WorkspaceIds:     f.WorkspaceIDs,
		WorkflowIds:      f.WorkflowIDs,
		States:           f.States,
		ParentId:         f.ParentID,
		IncludeEphemeral: f.IncludeEphemeral,
		IncludeArchived:  f.IncludeArchived,
	}
}

func taskFilterFromProto(p *pluginv1.TaskFilter) TaskFilter {
	if p == nil {
		return TaskFilter{}
	}
	return TaskFilter{
		WorkspaceIDs:     p.GetWorkspaceIds(),
		WorkflowIDs:      p.GetWorkflowIds(),
		States:           p.GetStates(),
		ParentID:         p.ParentId,
		IncludeEphemeral: p.GetIncludeEphemeral(),
		IncludeArchived:  p.GetIncludeArchived(),
	}
}

// Workspace is the Go-native mirror of kandev.plugin.v1.Workspace.
type Workspace struct {
	ID                    string
	Name                  string
	Description           *string
	OwnerID               string
	DefaultExecutorID     *string
	DefaultAgentProfileID *string
	CreatedAt             string
	UpdatedAt             string
}

func (w Workspace) toProto() *pluginv1.Workspace {
	return &pluginv1.Workspace{
		Id:                    w.ID,
		Name:                  w.Name,
		Description:           w.Description,
		OwnerId:               w.OwnerID,
		DefaultExecutorId:     w.DefaultExecutorID,
		DefaultAgentProfileId: w.DefaultAgentProfileID,
		CreatedAt:             w.CreatedAt,
		UpdatedAt:             w.UpdatedAt,
	}
}

func workspaceFromProto(p *pluginv1.Workspace) Workspace {
	if p == nil {
		return Workspace{}
	}
	return Workspace{
		ID:                    p.GetId(),
		Name:                  p.GetName(),
		Description:           p.Description,
		OwnerID:               p.GetOwnerId(),
		DefaultExecutorID:     p.DefaultExecutorId,
		DefaultAgentProfileID: p.DefaultAgentProfileId,
		CreatedAt:             p.GetCreatedAt(),
		UpdatedAt:             p.GetUpdatedAt(),
	}
}

func workspacesFromProto(items []*pluginv1.Workspace) []Workspace {
	if len(items) == 0 {
		return nil
	}
	out := make([]Workspace, len(items))
	for i, item := range items {
		out[i] = workspaceFromProto(item)
	}
	return out
}

func workspacesToProto(items []Workspace) []*pluginv1.Workspace {
	if len(items) == 0 {
		return nil
	}
	out := make([]*pluginv1.Workspace, len(items))
	for i := range items {
		out[i] = items[i].toProto()
	}
	return out
}

// Workflow is the Go-native mirror of kandev.plugin.v1.Workflow.
type Workflow struct {
	ID          string
	WorkspaceID string
	Name        string
	Description *string
	SortOrder   int32
	CreatedAt   string
	UpdatedAt   string
}

func (w Workflow) toProto() *pluginv1.Workflow {
	return &pluginv1.Workflow{
		Id:          w.ID,
		WorkspaceId: w.WorkspaceID,
		Name:        w.Name,
		Description: w.Description,
		SortOrder:   w.SortOrder,
		CreatedAt:   w.CreatedAt,
		UpdatedAt:   w.UpdatedAt,
	}
}

func workflowFromProto(p *pluginv1.Workflow) Workflow {
	if p == nil {
		return Workflow{}
	}
	return Workflow{
		ID:          p.GetId(),
		WorkspaceID: p.GetWorkspaceId(),
		Name:        p.GetName(),
		Description: p.Description,
		SortOrder:   p.GetSortOrder(),
		CreatedAt:   p.GetCreatedAt(),
		UpdatedAt:   p.GetUpdatedAt(),
	}
}

func workflowsFromProto(items []*pluginv1.Workflow) []Workflow {
	if len(items) == 0 {
		return nil
	}
	out := make([]Workflow, len(items))
	for i, item := range items {
		out[i] = workflowFromProto(item)
	}
	return out
}

func workflowsToProto(items []Workflow) []*pluginv1.Workflow {
	if len(items) == 0 {
		return nil
	}
	out := make([]*pluginv1.Workflow, len(items))
	for i := range items {
		out[i] = items[i].toProto()
	}
	return out
}

// WorkflowStep is the Go-native mirror of kandev.plugin.v1.WorkflowStep.
type WorkflowStep struct {
	ID         string
	WorkflowID string
	Name       string
	Position   int32
	StageType  string
	// Color is the step's own presentation colour. A plugin rendering a board
	// reads it rather than inventing a palette, so its columns match the ones
	// the operator already sees in kandev.
	Color          string
	IsStartStep    bool
	WIPLimit       int32 // 0 means unlimited
	AgentProfileID string
	// OnEnterActionTypes lists the on_enter action types configured on this
	// step, as authored. A type appearing here means it is configured, not a
	// guarantee of when or whether it fires. Config maps are deliberately not
	// exposed. Nil when the step has no on_enter actions, never an empty
	// non-nil slice. Callers must ignore action types they do not recognize.
	OnEnterActionTypes []string
}

func (s WorkflowStep) toProto() *pluginv1.WorkflowStep {
	return &pluginv1.WorkflowStep{
		Id:                 s.ID,
		WorkflowId:         s.WorkflowID,
		Name:               s.Name,
		Position:           s.Position,
		StageType:          s.StageType,
		Color:              s.Color,
		IsStartStep:        s.IsStartStep,
		WipLimit:           s.WIPLimit,
		AgentProfileId:     s.AgentProfileID,
		OnEnterActionTypes: s.OnEnterActionTypes,
	}
}

func workflowStepFromProto(p *pluginv1.WorkflowStep) WorkflowStep {
	if p == nil {
		return WorkflowStep{}
	}
	return WorkflowStep{
		ID:                 p.GetId(),
		WorkflowID:         p.GetWorkflowId(),
		Name:               p.GetName(),
		Position:           p.GetPosition(),
		StageType:          p.GetStageType(),
		OnEnterActionTypes: nonEmptyStrings(p.GetOnEnterActionTypes()),
		Color:              p.GetColor(),
		IsStartStep:        p.GetIsStartStep(),
		WIPLimit:           p.GetWipLimit(),
		AgentProfileID:     p.GetAgentProfileId(),
	}
}

// nonEmptyStrings enforces the documented nil-not-empty invariant at the SDK
// decode boundary, independent of what a caller happened to serialize onto
// the wire.
func nonEmptyStrings(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	return s
}

func workflowStepsFromProto(items []*pluginv1.WorkflowStep) []WorkflowStep {
	if len(items) == 0 {
		return nil
	}
	out := make([]WorkflowStep, len(items))
	for i, item := range items {
		out[i] = workflowStepFromProto(item)
	}
	return out
}

func workflowStepsToProto(items []WorkflowStep) []*pluginv1.WorkflowStep {
	if len(items) == 0 {
		return nil
	}
	out := make([]*pluginv1.WorkflowStep, len(items))
	for i := range items {
		out[i] = items[i].toProto()
	}
	return out
}

// AgentProfile is the Go-native mirror of kandev.plugin.v1.AgentProfile.
type AgentProfile struct {
	ID          string
	AgentID     string
	DisplayName string
	Name        string
	Model       string
	Mode        string
}

// ExecutorProfile is the Go-native mirror of kandev.plugin.v1.ExecutorProfile.
type ExecutorProfile struct {
	ID           string
	DisplayName  string
	ExecutorType string
}

func (e ExecutorProfile) toProto() *pluginv1.ExecutorProfile {
	return &pluginv1.ExecutorProfile{Id: e.ID, DisplayName: e.DisplayName, ExecutorType: e.ExecutorType}
}

func executorProfileFromProto(p *pluginv1.ExecutorProfile) ExecutorProfile {
	if p == nil {
		return ExecutorProfile{}
	}
	return ExecutorProfile{ID: p.GetId(), DisplayName: p.GetDisplayName(), ExecutorType: p.GetExecutorType()}
}

func executorProfilesFromProto(profiles []*pluginv1.ExecutorProfile) []ExecutorProfile {
	if profiles == nil {
		return nil
	}
	out := make([]ExecutorProfile, len(profiles))
	for i, profile := range profiles {
		out[i] = executorProfileFromProto(profile)
	}
	return out
}

func executorProfilesToProto(profiles []ExecutorProfile) []*pluginv1.ExecutorProfile {
	if profiles == nil {
		return nil
	}
	out := make([]*pluginv1.ExecutorProfile, len(profiles))
	for i, profile := range profiles {
		out[i] = profile.toProto()
	}
	return out
}

func (a AgentProfile) toProto() *pluginv1.AgentProfile {
	return &pluginv1.AgentProfile{
		Id:          a.ID,
		AgentId:     a.AgentID,
		DisplayName: a.DisplayName,
		Name:        a.Name,
		Model:       a.Model,
		Mode:        a.Mode,
	}
}

func agentProfileFromProto(p *pluginv1.AgentProfile) AgentProfile {
	if p == nil {
		return AgentProfile{}
	}
	return AgentProfile{
		ID:          p.GetId(),
		AgentID:     p.GetAgentId(),
		DisplayName: p.GetDisplayName(),
		Name:        p.GetName(),
		Model:       p.GetModel(),
		Mode:        p.GetMode(),
	}
}

func agentProfilesFromProto(items []*pluginv1.AgentProfile) []AgentProfile {
	if len(items) == 0 {
		return nil
	}
	out := make([]AgentProfile, len(items))
	for i, item := range items {
		out[i] = agentProfileFromProto(item)
	}
	return out
}

func agentProfilesToProto(items []AgentProfile) []*pluginv1.AgentProfile {
	if len(items) == 0 {
		return nil
	}
	out := make([]*pluginv1.AgentProfile, len(items))
	for i := range items {
		out[i] = items[i].toProto()
	}
	return out
}

// Repository is the Go-native mirror of kandev.plugin.v1.Repository.
type Repository struct {
	ID                   string
	WorkspaceID          string
	Name                 string
	DefaultBranch        *string
	SourceType           string
	ProviderID           string
	ProviderRepositoryID string
	ProviderHost         string
	ProviderScope        string
	OwnerOrProject       string
	ProviderName         string
	RemoteURL            string
}

func (r Repository) toProto() *pluginv1.Repository {
	return &pluginv1.Repository{
		Id:                   r.ID,
		WorkspaceId:          r.WorkspaceID,
		Name:                 r.Name,
		DefaultBranch:        r.DefaultBranch,
		SourceType:           r.SourceType,
		ProviderId:           r.ProviderID,
		ProviderRepositoryId: r.ProviderRepositoryID,
		ProviderHost:         r.ProviderHost,
		ProviderScope:        r.ProviderScope,
		OwnerOrProject:       r.OwnerOrProject,
		ProviderName:         r.ProviderName,
		RemoteUrl:            r.RemoteURL,
	}
}

func repositoryFromProto(p *pluginv1.Repository) Repository {
	if p == nil {
		return Repository{}
	}
	return Repository{
		ID:                   p.GetId(),
		WorkspaceID:          p.GetWorkspaceId(),
		Name:                 p.GetName(),
		DefaultBranch:        p.DefaultBranch,
		SourceType:           p.GetSourceType(),
		ProviderID:           p.GetProviderId(),
		ProviderRepositoryID: p.GetProviderRepositoryId(),
		ProviderHost:         p.GetProviderHost(),
		ProviderScope:        p.GetProviderScope(),
		OwnerOrProject:       p.GetOwnerOrProject(),
		ProviderName:         p.GetProviderName(),
		RemoteURL:            p.GetRemoteUrl(),
	}
}

func repositoriesFromProto(items []*pluginv1.Repository) []Repository {
	if len(items) == 0 {
		return nil
	}
	out := make([]Repository, len(items))
	for i, item := range items {
		out[i] = repositoryFromProto(item)
	}
	return out
}

func repositoriesToProto(items []Repository) []*pluginv1.Repository {
	if len(items) == 0 {
		return nil
	}
	out := make([]*pluginv1.Repository, len(items))
	for i := range items {
		out[i] = items[i].toProto()
	}
	return out
}

// Session is the Go-native mirror of kandev.plugin.v1.Session.
type Session struct {
	ID               string
	TaskID           string
	AgentProfileID   string
	AgentDisplayName string
	Model            string
	ACPSessionID     string
	State            string
	StartedAt        string
	EndedAt          *string
	AgentProfileName string // profile name from the snapshot at run time
}

func (s Session) toProto() *pluginv1.Session {
	return &pluginv1.Session{
		Id:               s.ID,
		TaskId:           s.TaskID,
		AgentProfileId:   s.AgentProfileID,
		AgentDisplayName: s.AgentDisplayName,
		Model:            s.Model,
		AcpSessionId:     s.ACPSessionID,
		State:            s.State,
		StartedAt:        s.StartedAt,
		EndedAt:          s.EndedAt,
		AgentProfileName: s.AgentProfileName,
	}
}

func sessionFromProto(p *pluginv1.Session) Session {
	if p == nil {
		return Session{}
	}
	return Session{
		ID:               p.GetId(),
		TaskID:           p.GetTaskId(),
		AgentProfileID:   p.GetAgentProfileId(),
		AgentDisplayName: p.GetAgentDisplayName(),
		Model:            p.GetModel(),
		ACPSessionID:     p.GetAcpSessionId(),
		State:            p.GetState(),
		StartedAt:        p.GetStartedAt(),
		EndedAt:          p.EndedAt,
		AgentProfileName: p.GetAgentProfileName(),
	}
}

func sessionsFromProto(items []*pluginv1.Session) []Session {
	if len(items) == 0 {
		return nil
	}
	out := make([]Session, len(items))
	for i, item := range items {
		out[i] = sessionFromProto(item)
	}
	return out
}

func sessionsToProto(items []Session) []*pluginv1.Session {
	if len(items) == 0 {
		return nil
	}
	out := make([]*pluginv1.Session, len(items))
	for i := range items {
		out[i] = items[i].toProto()
	}
	return out
}

// SessionFilter is the Go-native mirror of kandev.plugin.v1.SessionFilter.
type SessionFilter struct {
	TaskIDs      []string
	WorkspaceIDs []string
	States       []string
}

func (f SessionFilter) toProto() *pluginv1.SessionFilter {
	return &pluginv1.SessionFilter{
		TaskIds:      f.TaskIDs,
		WorkspaceIds: f.WorkspaceIDs,
		States:       f.States,
	}
}

func sessionFilterFromProto(p *pluginv1.SessionFilter) SessionFilter {
	if p == nil {
		return SessionFilter{}
	}
	return SessionFilter{
		TaskIDs:      p.GetTaskIds(),
		WorkspaceIDs: p.GetWorkspaceIds(),
		States:       p.GetStates(),
	}
}

// SessionCodeStats is the Go-native mirror of
// kandev.plugin.v1.SessionCodeStats — a computed per-session code-change
// summary, never the raw commit/snapshot rows.
//
// CommittedLinesAvailable is false (with LinesAddedCommitted/
// LinesDeletedCommitted reading 0) for a session that predates
// commit-capture activation and has no observed commits — capture wasn't
// running yet, so that zero cannot be told apart from a real zero-change
// session. This is a separate bool rather than making the existing fields
// pointers: they are already-shipped public SDK fields (ADR 0043's
// additive-only DTO contract), and changing their Go type from int64 to
// *int64 would be a source-compatibility break for any plugin rebuilding
// against a new SDK version. See internal/analytics/models.SessionCodeStats
// for the full contract (that internal type is not part of the public
// plugin surface and keeps its own *int64 representation).
type SessionCodeStats struct {
	SessionID               string
	LinesAddedCommitted     int64
	LinesDeletedCommitted   int64
	LinesAddedPeakPending   int64
	LinesDeletedPeakPending int64
	CommittedLinesAvailable bool
}

func (s SessionCodeStats) toProto() *pluginv1.SessionCodeStats {
	return &pluginv1.SessionCodeStats{
		SessionId:               s.SessionID,
		LinesAddedCommitted:     s.LinesAddedCommitted,
		LinesDeletedCommitted:   s.LinesDeletedCommitted,
		LinesAddedPeakPending:   s.LinesAddedPeakPending,
		LinesDeletedPeakPending: s.LinesDeletedPeakPending,
		CommittedLinesAvailable: s.CommittedLinesAvailable,
	}
}

func sessionCodeStatsFromProto(p *pluginv1.SessionCodeStats) SessionCodeStats {
	if p == nil {
		return SessionCodeStats{}
	}
	return SessionCodeStats{
		SessionID:               p.GetSessionId(),
		LinesAddedCommitted:     p.GetLinesAddedCommitted(),
		LinesDeletedCommitted:   p.GetLinesDeletedCommitted(),
		LinesAddedPeakPending:   p.GetLinesAddedPeakPending(),
		LinesDeletedPeakPending: p.GetLinesDeletedPeakPending(),
		CommittedLinesAvailable: p.GetCommittedLinesAvailable(),
	}
}

func sessionCodeStatsSliceFromProto(items []*pluginv1.SessionCodeStats) []SessionCodeStats {
	if len(items) == 0 {
		return nil
	}
	out := make([]SessionCodeStats, len(items))
	for i, item := range items {
		out[i] = sessionCodeStatsFromProto(item)
	}
	return out
}

func sessionCodeStatsSliceToProto(items []SessionCodeStats) []*pluginv1.SessionCodeStats {
	if len(items) == 0 {
		return nil
	}
	out := make([]*pluginv1.SessionCodeStats, len(items))
	for i := range items {
		out[i] = items[i].toProto()
	}
	return out
}

// Message is the Go-native mirror of kandev.plugin.v1.Message — one
// user/agent message in a session transcript. Content has kandev's injected
// <kandev-system> blocks stripped; raw system content is never exposed.
type Message struct {
	ID         string
	SessionID  string
	TaskID     string
	TurnID     string
	AuthorType string // "user" | "agent"
	Content    string
	Type       string // "message" default; see the MessageType vocabulary
	CreatedAt  string // RFC3339
}

func (m Message) toProto() *pluginv1.Message {
	return &pluginv1.Message{
		Id:         m.ID,
		SessionId:  m.SessionID,
		TaskId:     m.TaskID,
		TurnId:     m.TurnID,
		AuthorType: m.AuthorType,
		Content:    m.Content,
		Type:       m.Type,
		CreatedAt:  m.CreatedAt,
	}
}

func messageFromProto(p *pluginv1.Message) Message {
	if p == nil {
		return Message{}
	}
	return Message{
		ID:         p.GetId(),
		SessionID:  p.GetSessionId(),
		TaskID:     p.GetTaskId(),
		TurnID:     p.GetTurnId(),
		AuthorType: p.GetAuthorType(),
		Content:    p.GetContent(),
		Type:       p.GetType(),
		CreatedAt:  p.GetCreatedAt(),
	}
}

func messagesFromProto(items []*pluginv1.Message) []Message {
	if len(items) == 0 {
		return nil
	}
	out := make([]Message, len(items))
	for i, item := range items {
		out[i] = messageFromProto(item)
	}
	return out
}

func messagesToProto(items []Message) []*pluginv1.Message {
	if len(items) == 0 {
		return nil
	}
	out := make([]*pluginv1.Message, len(items))
	for i := range items {
		out[i] = items[i].toProto()
	}
	return out
}

// MessageFilter is the Go-native mirror of kandev.plugin.v1.MessageFilter.
// Since/Until are RFC3339 strings bounding created_at (Since inclusive, Until
// exclusive); nil means unbounded on that end.
type MessageFilter struct {
	SessionIDs []string
	TaskIDs    []string
	Since      *string
	Until      *string
	Types      []string
}

func (f MessageFilter) toProto() *pluginv1.MessageFilter {
	return &pluginv1.MessageFilter{
		SessionIds: f.SessionIDs,
		TaskIds:    f.TaskIDs,
		Since:      f.Since,
		Until:      f.Until,
		Types:      f.Types,
	}
}

func messageFilterFromProto(p *pluginv1.MessageFilter) MessageFilter {
	if p == nil {
		return MessageFilter{}
	}
	return MessageFilter{
		SessionIDs: p.GetSessionIds(),
		TaskIDs:    p.GetTaskIds(),
		Since:      p.Since,
		Until:      p.Until,
		Types:      p.GetTypes(),
	}
}

// ── Write inputs (ADR 0043 phase 2: api_write) ────────────────────────────
//
// These mirror kandev.plugin.v1.CreateTaskRequest/UpdateTaskRequest/
// CreateCommentRequest. Optional/nullable fields use *string so "leave
// unchanged" (nil) is distinguishable from "set to empty" ("") — matching the
// proto `optional` on the wire. The server stamps write provenance
// (source = "plugin:<id>") itself; there is no field for a plugin to set it.

// CreateTaskInput is the Go-native mirror of
// kandev.plugin.v1.CreateTaskRequest. WorkspaceID/WorkflowID may be left empty
// to accept the host's defaults (the single workspace, and that workspace's
// first workflow); the host returns a gRPC InvalidArgument when a default
// cannot be resolved unambiguously.
type CreateTaskInput struct {
	WorkspaceID    string
	WorkflowID     string
	WorkflowStepID *string
	Title          string
	Description    string
	ParentID       *string
	// StartAgent asks the host to auto-launch an agent on the created task,
	// mirroring the REST/MCP create_task start_agent flag. Best-effort: a
	// launch failure does not fail task creation.
	StartAgent   bool
	Repositories []PluginTaskRepository
	Launch       *PluginTaskLaunchOptions
	Metadata     map[string]any
}

func (in CreateTaskInput) toProto() (*pluginv1.CreateTaskRequest, error) {
	metadata, err := mapToStruct(in.Metadata)
	if err != nil {
		return nil, fmt.Errorf("pluginsdk: task metadata: %w", err)
	}
	return &pluginv1.CreateTaskRequest{
		WorkspaceId:    in.WorkspaceID,
		WorkflowId:     in.WorkflowID,
		WorkflowStepId: in.WorkflowStepID,
		Title:          in.Title,
		Description:    in.Description,
		ParentId:       in.ParentID,
		StartAgent:     in.StartAgent,
		Repositories:   pluginTaskRepositoriesToProto(in.Repositories),
		Launch:         in.Launch.toProto(),
		Metadata:       metadata,
	}, nil
}

func createTaskInputFromProto(p *pluginv1.CreateTaskRequest) (CreateTaskInput, error) {
	if p == nil {
		return CreateTaskInput{}, nil
	}
	metadata, err := structToMap(p.GetMetadata())
	if err != nil {
		return CreateTaskInput{}, fmt.Errorf("pluginsdk: task metadata: %w", err)
	}
	return CreateTaskInput{
		WorkspaceID:    p.GetWorkspaceId(),
		WorkflowID:     p.GetWorkflowId(),
		WorkflowStepID: p.WorkflowStepId,
		Title:          p.GetTitle(),
		Description:    p.GetDescription(),
		ParentID:       p.ParentId,
		StartAgent:     p.GetStartAgent(),
		Repositories:   pluginTaskRepositoriesFromProto(p.GetRepositories()),
		Launch:         pluginTaskLaunchOptionsFromProto(p.GetLaunch()),
		Metadata:       metadata,
	}, nil
}

// RemoteRepositoryDescriptor is a fully resolved plugin-provider repository
// identity. CloneURL is exact and credential-free; the host must not rebuild it.
type RemoteRepositoryDescriptor struct {
	ProviderID           string
	ProviderHost         string
	ProviderScope        string
	OwnerOrProject       string
	ProviderRepositoryID string
	Name                 string
	CloneURL             string
	DefaultBranch        *string
	BaseBranch           *string
	HeadBranch           *string
	PullRequestNumber    *int64
}

func (d *RemoteRepositoryDescriptor) toProto() *pluginv1.RemoteRepositoryDescriptor {
	if d == nil {
		return nil
	}
	return &pluginv1.RemoteRepositoryDescriptor{
		ProviderId: d.ProviderID, ProviderHost: d.ProviderHost, ProviderScope: d.ProviderScope, OwnerOrProject: d.OwnerOrProject,
		ProviderRepositoryId: d.ProviderRepositoryID, Name: d.Name, CloneUrl: d.CloneURL,
		DefaultBranch: d.DefaultBranch, BaseBranch: d.BaseBranch, HeadBranch: d.HeadBranch, PullRequestNumber: d.PullRequestNumber,
	}
}

func remoteRepositoryDescriptorFromProto(p *pluginv1.RemoteRepositoryDescriptor) *RemoteRepositoryDescriptor {
	if p == nil {
		return nil
	}
	return &RemoteRepositoryDescriptor{
		ProviderID: p.GetProviderId(), ProviderHost: p.GetProviderHost(), ProviderScope: p.GetProviderScope(), OwnerOrProject: p.GetOwnerOrProject(),
		ProviderRepositoryID: p.GetProviderRepositoryId(), Name: p.GetName(), CloneURL: p.GetCloneUrl(),
		DefaultBranch: p.DefaultBranch, BaseBranch: p.BaseBranch, HeadBranch: p.HeadBranch, PullRequestNumber: p.PullRequestNumber,
	}
}

// PluginTaskRepository selects an existing repository or a complete remote
// descriptor to attach when a plugin creates a task.
type PluginTaskRepository struct {
	RepositoryID      string
	Remote            *RemoteRepositoryDescriptor
	BaseBranch        *string
	CheckoutBranch    *string
	PullRequestNumber *int64
}

func (r PluginTaskRepository) toProto() *pluginv1.PluginTaskRepository {
	return &pluginv1.PluginTaskRepository{RepositoryId: r.RepositoryID, Remote: r.Remote.toProto(), BaseBranch: r.BaseBranch, CheckoutBranch: r.CheckoutBranch, PullRequestNumber: r.PullRequestNumber}
}

func pluginTaskRepositoryFromProto(p *pluginv1.PluginTaskRepository) PluginTaskRepository {
	if p == nil {
		return PluginTaskRepository{}
	}
	return PluginTaskRepository{RepositoryID: p.GetRepositoryId(), Remote: remoteRepositoryDescriptorFromProto(p.GetRemote()), BaseBranch: p.BaseBranch, CheckoutBranch: p.CheckoutBranch, PullRequestNumber: p.PullRequestNumber}
}

func pluginTaskRepositoriesToProto(repositories []PluginTaskRepository) []*pluginv1.PluginTaskRepository {
	if repositories == nil {
		return nil
	}
	out := make([]*pluginv1.PluginTaskRepository, len(repositories))
	for i, repository := range repositories {
		out[i] = repository.toProto()
	}
	return out
}

func pluginTaskRepositoriesFromProto(repositories []*pluginv1.PluginTaskRepository) []PluginTaskRepository {
	if repositories == nil {
		return nil
	}
	out := make([]PluginTaskRepository, len(repositories))
	for i, repository := range repositories {
		out[i] = pluginTaskRepositoryFromProto(repository)
	}
	return out
}

// PluginTaskLaunchOptions declares the optional launch context for a plugin-
// created task. Host implementation validates each value before use.
type PluginTaskLaunchOptions struct {
	AgentProfileID    *string
	ExecutorProfileID *string
	Prompt            *string
	PlanMode          *string
}

func (o *PluginTaskLaunchOptions) toProto() *pluginv1.PluginTaskLaunchOptions {
	if o == nil {
		return nil
	}
	return &pluginv1.PluginTaskLaunchOptions{AgentProfileId: o.AgentProfileID, ExecutorProfileId: o.ExecutorProfileID, Prompt: o.Prompt, PlanMode: o.PlanMode}
}

func pluginTaskLaunchOptionsFromProto(p *pluginv1.PluginTaskLaunchOptions) *PluginTaskLaunchOptions {
	if p == nil {
		return nil
	}
	return &PluginTaskLaunchOptions{AgentProfileID: p.AgentProfileId, ExecutorProfileID: p.ExecutorProfileId, Prompt: p.Prompt, PlanMode: p.PlanMode}
}

// UpdateTaskInput is the Go-native mirror of
// kandev.plugin.v1.UpdateTaskRequest. Every field except ID is optional: a nil
// pointer leaves that field untouched, a non-nil pointer overwrites it. The
// conservative field surface (title/description/state) is the documented
// plugin-writable mask. WorkflowStepID is rejected when non-nil — it exists
// on this struct only for wire-shape symmetry with UpdateTaskRequest; use
// TaskReader.Move (see host.go) to transition a task between workflow steps,
// which routes through the same validation, WIP admission, task.moved
// publication, auto-start gates, and queue reconciliation the board's own
// move uses.
type UpdateTaskInput struct {
	ID             string
	Title          *string
	Description    *string
	State          *string
	WorkflowStepID *string
}

func (in UpdateTaskInput) toProto() *pluginv1.UpdateTaskRequest {
	return &pluginv1.UpdateTaskRequest{
		Id:             in.ID,
		Title:          in.Title,
		Description:    in.Description,
		State:          in.State,
		WorkflowStepId: in.WorkflowStepID,
	}
}

func updateTaskInputFromProto(p *pluginv1.UpdateTaskRequest) UpdateTaskInput {
	if p == nil {
		return UpdateTaskInput{}
	}
	return UpdateTaskInput{
		ID:             p.GetId(),
		Title:          p.Title,
		Description:    p.Description,
		State:          p.State,
		WorkflowStepID: p.WorkflowStepId,
	}
}

// MoveTaskInput is the Go-native mirror of kandev.plugin.v1.MoveTaskRequest.
// Unlike Update, Move transitions the task through the same path the board's
// own move uses, so on_enter actions (auto-start included) actually fire.
// WorkflowID is optional: nil inherits the task's current workflow; a
// pointer to "" is rejected rather than treated as inherit. Position is not
// optional: an omitted position and a position of zero are the same
// request, both placing the task at the top of the target step.
type MoveTaskInput struct {
	TaskID         string
	WorkflowStepID string
	WorkflowID     *string
	Position       int32
}

func (in MoveTaskInput) toProto() *pluginv1.MoveTaskRequest {
	return &pluginv1.MoveTaskRequest{
		TaskId:         in.TaskID,
		WorkflowStepId: in.WorkflowStepID,
		WorkflowId:     in.WorkflowID,
		Position:       in.Position,
	}
}

func moveTaskInputFromProto(p *pluginv1.MoveTaskRequest) MoveTaskInput {
	if p == nil {
		return MoveTaskInput{}
	}
	return MoveTaskInput{
		TaskID:         p.GetTaskId(),
		WorkflowStepID: p.GetWorkflowStepId(),
		WorkflowID:     p.WorkflowId,
		Position:       p.GetPosition(),
	}
}

// MoveTaskOutcome is the Go-native mirror of kandev.plugin.v1.MoveTaskResponse.
// Transitioned reports whether a ledger row was written for this move (false
// when the task was already on the target step). QueuedForStepID is the sole
// admission discriminator, reported on both values of Transitioned: nil means
// admitted (when Transitioned) or not applicable (when not); non-nil means
// queued for that step holding no WIP slot. FromStepID is the step the task
// left; empty when Transitioned is false.
type MoveTaskOutcome struct {
	Task            *Task
	Transitioned    bool
	QueuedForStepID *string
	FromStepID      string
}

func (o MoveTaskOutcome) toProto() (*pluginv1.MoveTaskResponse, error) {
	var protoTask *pluginv1.Task
	if o.Task != nil {
		var err error
		protoTask, err = o.Task.toProto()
		if err != nil {
			return nil, err
		}
	}
	return &pluginv1.MoveTaskResponse{
		Task:            protoTask,
		Transitioned:    o.Transitioned,
		QueuedForStepId: o.QueuedForStepID,
		FromStepId:      o.FromStepID,
	}, nil
}

func moveTaskOutcomeFromProto(p *pluginv1.MoveTaskResponse) (*MoveTaskOutcome, error) {
	if p == nil {
		return nil, nil
	}
	task, err := taskFromProto(p.GetTask())
	if err != nil {
		return nil, err
	}
	return &MoveTaskOutcome{
		Task:            &task,
		Transitioned:    p.GetTransitioned(),
		QueuedForStepID: p.QueuedForStepId,
		FromStepID:      p.GetFromStepId(),
	}, nil
}

// MessageDispatch is the Go-native mirror of
// kandev.plugin.v1.SendMessageResponse — the result of delivering a prompt to a
// task session. Status is "queued" (the session was running and the message
// was queued for its next turn), "sent" (delivered to an idle/resumed session),
// or "started" (the session was launched with it as the first prompt).
type MessageDispatch struct {
	SessionID string
	Status    string
}

func (d MessageDispatch) toProto() *pluginv1.SendMessageResponse {
	return &pluginv1.SendMessageResponse{SessionId: d.SessionID, Status: d.Status}
}

func messageDispatchFromProto(p *pluginv1.SendMessageResponse) *MessageDispatch {
	if p == nil {
		return nil
	}
	return &MessageDispatch{SessionID: p.GetSessionId(), Status: p.GetStatus()}
}
