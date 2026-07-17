// data_types.go defines the Go-native mirrors of the Host data API messages
// (ADR 0042: docs/decisions/0042-plugin-host-data-api.md) plus the
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
	ID           string
	RepositoryID string
	BaseBranch   string
	Position     int32
}

func (r TaskRepository) toProto() *pluginv1.TaskRepository {
	return &pluginv1.TaskRepository{
		Id:           r.ID,
		RepositoryId: r.RepositoryID,
		BaseBranch:   r.BaseBranch,
		Position:     r.Position,
	}
}

func taskRepositoryFromProto(p *pluginv1.TaskRepository) TaskRepository {
	if p == nil {
		return TaskRepository{}
	}
	return TaskRepository{
		ID:           p.GetId(),
		RepositoryID: p.GetRepositoryId(),
		BaseBranch:   p.GetBaseBranch(),
		Position:     p.GetPosition(),
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
	}, nil
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
}

func (f TaskFilter) toProto() *pluginv1.TaskFilter {
	return &pluginv1.TaskFilter{
		WorkspaceIds:     f.WorkspaceIDs,
		WorkflowIds:      f.WorkflowIDs,
		States:           f.States,
		ParentId:         f.ParentID,
		IncludeEphemeral: f.IncludeEphemeral,
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
}

func (s WorkflowStep) toProto() *pluginv1.WorkflowStep {
	return &pluginv1.WorkflowStep{
		Id:         s.ID,
		WorkflowId: s.WorkflowID,
		Name:       s.Name,
		Position:   s.Position,
		StageType:  s.StageType,
	}
}

func workflowStepFromProto(p *pluginv1.WorkflowStep) WorkflowStep {
	if p == nil {
		return WorkflowStep{}
	}
	return WorkflowStep{
		ID:         p.GetId(),
		WorkflowID: p.GetWorkflowId(),
		Name:       p.GetName(),
		Position:   p.GetPosition(),
		StageType:  p.GetStageType(),
	}
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
	ID            string
	WorkspaceID   string
	Name          string
	DefaultBranch *string
}

func (r Repository) toProto() *pluginv1.Repository {
	return &pluginv1.Repository{
		Id:            r.ID,
		WorkspaceId:   r.WorkspaceID,
		Name:          r.Name,
		DefaultBranch: r.DefaultBranch,
	}
}

func repositoryFromProto(p *pluginv1.Repository) Repository {
	if p == nil {
		return Repository{}
	}
	return Repository{
		ID:            p.GetId(),
		WorkspaceID:   p.GetWorkspaceId(),
		Name:          p.GetName(),
		DefaultBranch: p.DefaultBranch,
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
type SessionCodeStats struct {
	SessionID               string
	LinesAddedCommitted     int64
	LinesDeletedCommitted   int64
	LinesAddedPeakPending   int64
	LinesDeletedPeakPending int64
}

func (s SessionCodeStats) toProto() *pluginv1.SessionCodeStats {
	return &pluginv1.SessionCodeStats{
		SessionId:               s.SessionID,
		LinesAddedCommitted:     s.LinesAddedCommitted,
		LinesDeletedCommitted:   s.LinesDeletedCommitted,
		LinesAddedPeakPending:   s.LinesAddedPeakPending,
		LinesDeletedPeakPending: s.LinesDeletedPeakPending,
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
