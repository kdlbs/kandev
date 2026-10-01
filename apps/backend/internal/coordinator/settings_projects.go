package coordinator

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/coordinator/watch"
)

// maxWatchedProjects bounds a selected project list.
const maxWatchedProjects = 50

// Error codes of the settings PUT's projects member.
const (
	memberIncludeNoRepository  = "include_no_repository"
	memberEntries              = "entries"
	memberScope                = "scope"
	errProjectsNotWired        = "project reads are not wired"
	projectsScopeMessage       = "projects.scope must be all or selected"
	projectsEntryShapeMessage  = "projects.entries must be an array of {kind, id}"
	projectsToggleShapeMessage = "projects.include_no_repository must be a boolean"
	codeInvalidProjects        = "invalid_projects"
	codeProjectsEmpty          = "projects_empty"
	codeProjectsTooMany        = "projects_too_many"
	codeProjectsDuplicate      = "projects_duplicate"
	codeProjectsForeignEntry   = "projects_foreign_entry"
	fieldProjects              = "projects"
)

func projectsErr(code, message string) *SettingsError {
	return &SettingsError{Code: code, Field: fieldProjects, Message: message}
}

// projectsRequest is a projects member that passed the stateless checks.
type projectsRequest struct {
	scope         string
	entries       []ProjectEntry
	includeNoRepo bool
}

// projectState is a coordinator's stored project scope. Entries and the
// toggle outlive a switch to every project.
type projectState struct {
	scope         string
	entries       []ProjectEntry
	includeNoRepo bool
}

// equal is the server's equality rule: under every project, entries and the
// toggle are not compared.
func (p projectState) equal(o projectState) bool {
	if p.scope != o.scope {
		return false
	}
	return p.scope == watchScopeAll || (p.includeNoRepo == o.includeNoRepo && slices.Equal(p.entries, o.entries))
}

func sortProjectEntries(entries []ProjectEntry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Kind != entries[j].Kind {
			return entries[i].Kind == projectKindSet
		}
		return entries[i].ID < entries[j].ID
	})
}

// parseProjectsMember checks the projects member without any read. Entries
// and the toggle are not read under every project.
func parseProjectsMember(raw json.RawMessage) (*projectsRequest, error) {
	var member map[string]json.RawMessage
	if err := json.Unmarshal(raw, &member); err != nil || member == nil {
		return nil, projectsErr(codeInvalidProjects, "projects must be an object")
	}
	scopeRaw, ok := presentMember(member, memberScope)
	if !ok {
		return nil, projectsErr(codeInvalidProjects, projectsScopeMessage)
	}
	var scope string
	if err := json.Unmarshal(scopeRaw, &scope); err != nil || (scope != watchScopeAll && scope != watchScopeSelected) {
		return nil, projectsErr(codeInvalidProjects, projectsScopeMessage)
	}
	req := &projectsRequest{scope: scope}
	if scope == watchScopeAll {
		return req, nil
	}
	if raw, ok := presentMember(member, memberIncludeNoRepository); ok {
		if err := json.Unmarshal(raw, &req.includeNoRepo); err != nil {
			return nil, projectsErr(codeInvalidProjects, projectsToggleShapeMessage)
		}
	}
	if raw, ok := presentMember(member, memberEntries); ok {
		if err := json.Unmarshal(raw, &req.entries); err != nil {
			return nil, projectsErr(codeInvalidProjects, projectsEntryShapeMessage)
		}
	}
	return req, checkProjectEntryShape(req)
}

func checkProjectEntryShape(req *projectsRequest) error {
	for _, e := range req.entries {
		if (e.Kind != projectKindSet && e.Kind != projectKindRepository) || strings.TrimSpace(e.ID) == "" {
			return projectsErr(codeInvalidProjects, projectsEntryShapeMessage)
		}
	}
	if len(req.entries) > maxWatchedProjects {
		return projectsErr(codeProjectsTooMany, fmt.Sprintf("at most %d projects may be watched", maxWatchedProjects))
	}
	seen := make(map[ProjectEntry]struct{}, len(req.entries))
	for _, e := range req.entries {
		if _, dup := seen[e]; dup {
			return projectsErr(codeProjectsDuplicate, "duplicate project "+e.ID)
		}
		seen[e] = struct{}{}
	}
	sortProjectEntries(req.entries)
	return nil
}

// loadProjectState reads the stored project scope through exec, entries and
// toggle included whatever the scope.
func (s *Store) loadProjectState(ctx context.Context, exec coordinatorExec, coordinatorID string) (projectState, error) {
	var (
		scope         string
		includeNoRepo bool
	)
	if err := exec.QueryRowContext(ctx, s.db.Rebind(`SELECT project_scope, include_no_repository FROM coordinators WHERE id = ?`), coordinatorID).
		Scan(&scope, &includeNoRepo); err != nil {
		return projectState{}, fmt.Errorf("read project scope: %w", err)
	}
	entries, err := s.storedProjectEntries(ctx, exec, coordinatorID)
	if err != nil {
		return projectState{}, err
	}
	return projectState{scope: normalizeWatchScope(scope), entries: entries, includeNoRepo: includeNoRepo}, nil
}

// existingProjectEntries returns the listed entries that name a live set or
// repository of the workspace. A failed read is the error.
func (s *Service) existingProjectEntries(ctx context.Context, workspaceID string, entries []ProjectEntry) (map[ProjectEntry]struct{}, error) {
	out := make(map[ProjectEntry]struct{}, len(entries))
	if len(entries) == 0 {
		return out, nil
	}
	r := s.projectReader()
	if r == nil {
		return nil, fmt.Errorf("coordinator: %s", errProjectsNotWired)
	}
	var wantSets, wantRepos bool
	for _, e := range entries {
		wantSets = wantSets || e.Kind == projectKindSet
		wantRepos = wantRepos || e.Kind == projectKindRepository
	}
	if wantSets {
		sets, err := r.ListRepositorySets(ctx, workspaceID)
		if err != nil {
			return nil, fmt.Errorf("read repository sets: %w", err)
		}
		for _, set := range sets {
			if set != nil {
				out[ProjectEntry{Kind: projectKindSet, ID: set.ID}] = struct{}{}
			}
		}
	}
	if wantRepos {
		repos, err := r.ListRepositories(ctx, workspaceID)
		if err != nil {
			return nil, fmt.Errorf("read repositories: %w", err)
		}
		for _, repo := range repos {
			if repo != nil {
				out[ProjectEntry{Kind: projectKindRepository, ID: repo.ID}] = struct{}{}
			}
		}
	}
	return out, nil
}

// keepProjectEntries drops the entries naming something that no longer
// exists but is stored; any other entry that does not exist is foreign.
// existing is read before the coordinator lock is taken.
func keepProjectEntries(existing map[ProjectEntry]struct{}, body, stored []ProjectEntry) ([]ProjectEntry, error) {
	storedSet := make(map[ProjectEntry]struct{}, len(stored))
	for _, e := range stored {
		storedSet[e] = struct{}{}
	}
	kept := make([]ProjectEntry, 0, len(body))
	for _, e := range body {
		if _, ok := existing[e]; ok {
			kept = append(kept, e)
			continue
		}
		if _, ok := storedSet[e]; !ok {
			return nil, projectsErr(codeProjectsForeignEntry, e.Kind+" "+e.ID+" is not in this workspace")
		}
	}
	return kept, nil
}

// liveProjectEntries reads, outside any lock, which of the body's entries name
// a live set or repository. It reads nothing when the member lists none.
func (s *Service) liveProjectEntries(ctx context.Context, workspaceID string, req *projectsRequest) (map[ProjectEntry]struct{}, error) {
	if req == nil || req.scope != watchScopeSelected {
		return nil, nil
	}
	return s.existingProjectEntries(ctx, workspaceID, req.entries)
}

func errProjectsEmpty() error {
	return projectsErr(codeProjectsEmpty, "keep at least one project in scope or include tasks with no repository")
}

// resolveProjects turns the body's projects member into the state to write,
// or nil when it leaves the stored value as it is. The body is compared with
// the stored value first; under every project only the scope is compared.
// existing is the live subset of the body's entries, read before the lock.
func resolveProjects(existing map[ProjectEntry]struct{}, stored projectState, req *projectsRequest) (*projectState, error) {
	if req == nil {
		return nil, nil
	}
	if req.scope == watchScopeAll {
		next := projectState{scope: watchScopeAll, entries: stored.entries, includeNoRepo: stored.includeNoRepo}
		if next.equal(stored) {
			return nil, nil
		}
		return &next, nil
	}
	body := projectState{scope: watchScopeSelected, entries: req.entries, includeNoRepo: req.includeNoRepo}
	if body.equal(stored) {
		return nil, nil
	}
	if len(req.entries) == 0 && !req.includeNoRepo {
		return nil, errProjectsEmpty()
	}
	kept, err := keepProjectEntries(existing, req.entries, stored.entries)
	if err != nil {
		return nil, err
	}
	if len(kept) == 0 && !req.includeNoRepo {
		return nil, errProjectsEmpty()
	}
	return &projectState{scope: watchScopeSelected, entries: kept, includeNoRepo: req.includeNoRepo}, nil
}

// writeProjectState stores next. Under every project only the scope is
// written: the entries and the toggle are kept for the next switch back.
func (s *Store) writeProjectState(ctx context.Context, tx coordinatorExec, workspaceID, coordinatorID string, next projectState) error {
	query, args := `UPDATE coordinators SET project_scope = ? WHERE id = ?`, []any{next.scope, coordinatorID}
	if next.scope == watchScopeSelected {
		query, args = `UPDATE coordinators SET project_scope = ?, include_no_repository = ? WHERE id = ?`, []any{next.scope, next.includeNoRepo, coordinatorID}
	}
	if _, err := tx.ExecContext(ctx, s.db.Rebind(query), args...); err != nil {
		return fmt.Errorf("write project scope: %w", err)
	}
	if next.scope != watchScopeSelected {
		return nil
	}
	if _, err := tx.ExecContext(ctx, s.db.Rebind(`DELETE FROM coordinator_watch_projects WHERE coordinator_id = ?`), coordinatorID); err != nil {
		return fmt.Errorf("clear watch projects: %w", err)
	}
	return s.insertProjectEntries(ctx, tx, workspaceID, coordinatorID, next.entries)
}

func (s *Store) insertProjectEntries(ctx context.Context, tx coordinatorExec, workspaceID, coordinatorID string, entries []ProjectEntry) error {
	now := s.now().UTC()
	for _, e := range entries {
		if _, err := tx.ExecContext(ctx, s.db.Rebind(`INSERT INTO coordinator_watch_projects (coordinator_id, entry_kind, entry_id, workspace_id, created_at) VALUES (?, ?, ?, ?, ?)`),
			coordinatorID, e.Kind, e.ID, workspaceID, now); err != nil {
			return fmt.Errorf("insert watch project: %w", err)
		}
	}
	return nil
}

// ProjectsDTO is the enforcement member of the settings read: the live
// repositories a selected scope covers. RepositoryIDs is null when the sets
// or repositories could not be read.
type ProjectsDTO struct {
	Scope               string   `json:"scope"`
	RepositoryIDs       []string `json:"repository_ids"`
	IncludeNoRepository bool     `json:"include_no_repository"`
}

// ProjectsConfigDTO is the editor member of the settings read.
type ProjectsConfigDTO struct {
	Scope               string         `json:"scope"`
	Entries             []ProjectEntry `json:"entries"`
	IncludeNoRepository bool           `json:"include_no_repository"`
}

// WatchProjectsDTO is the projects member of the coordinator read's watches.
type WatchProjectsDTO struct {
	Scope               string    `json:"scope"`
	RepositoryIDs       []string  `json:"repository_ids"`
	IncludeNoRepository bool      `json:"include_no_repository"`
	Names               *[]string `json:"names,omitempty"`
}

// WatchProjectsAllDTO is the projects member under every project.
type WatchProjectsAllDTO struct {
	Scope string `json:"scope"`
}

func (s *Service) resolveStoredScope(ctx context.Context, workspaceID string, st projectState) (watch.Resolved, error) {
	return newWatchGateFromScope(ctx, s.projectReader(), workspaceID, storedScope(st))
}

func storedScope(st projectState) watch.Scope {
	scope := watch.Scope{Selected: st.scope == watchScopeSelected, IncludeNoRepo: st.includeNoRepo}
	for _, e := range st.entries {
		if e.Kind == projectKindSet {
			scope.SetIDs = append(scope.SetIDs, e.ID)
		} else {
			scope.RepoIDs = append(scope.RepoIDs, e.ID)
		}
	}
	return scope
}

func newWatchGateFromScope(ctx context.Context, r ProjectReader, workspaceID string, scope watch.Scope) (watch.Resolved, error) {
	var reader watch.Reader
	if r != nil {
		reader = projectReads{r: r}
	}
	return watch.NewResolver(reader).Resolve(ctx, workspaceID, scope)
}

// projectsView builds the settings read members for a stored state: the
// enforcement member while the stored scope is selected, whatever the flag,
// and the editor member while phase 3.1 is effective.
func (s *Service) projectsView(ctx context.Context, workspaceID string, st projectState) (*ProjectsDTO, *ProjectsConfigDTO) {
	var enforced *ProjectsDTO
	if st.scope == watchScopeSelected {
		enforced = &ProjectsDTO{Scope: st.scope, IncludeNoRepository: st.includeNoRepo}
		resolved, err := s.resolveStoredScope(ctx, workspaceID, st)
		if err != nil {
			s.logger.Warn("coordinator: project listing read failed", zap.String("workspace_id", workspaceID), zap.Error(err))
		} else {
			enforced.RepositoryIDs = nonNilIDs(resolved.RepositoryIDs())
		}
	}
	if !s.phase31 {
		return enforced, nil
	}
	return enforced, &ProjectsConfigDTO{Scope: st.scope, Entries: nonNilEntries(st.entries), IncludeNoRepository: st.includeNoRepo}
}

func nonNilEntries(entries []ProjectEntry) []ProjectEntry {
	if entries == nil {
		return []ProjectEntry{}
	}
	return entries
}

// WatchProjectsView is the projects member of the coordinator read's
// watches, or nil when there is none to show.
func (s *Service) WatchProjectsView(ctx context.Context, c *Coordinator) (any, error) {
	st, err := s.store.loadProjectState(ctx, s.store.ro, c.ID)
	if err != nil {
		return nil, err
	}
	if st.scope != watchScopeSelected {
		if s.phase31 {
			return &WatchProjectsAllDTO{Scope: watchScopeAll}, nil
		}
		return nil, nil
	}
	view, _ := s.projectsView(ctx, c.WorkspaceID, st)
	return &WatchProjectsDTO{
		Scope: st.scope, RepositoryIDs: view.RepositoryIDs, IncludeNoRepository: st.includeNoRepo,
		Names: s.projectNames(ctx, c.WorkspaceID, st.entries),
	}, nil
}

type namedProject struct{ id, name string }

// projectNames lists the display names of the listed sets, then of the listed
// repositories, each by lowercase name then id; entries naming nothing live
// are omitted. It is nil when a listing read failed.
func (s *Service) projectNames(ctx context.Context, workspaceID string, entries []ProjectEntry) *[]string {
	r := s.projectReader()
	if r == nil {
		return nil
	}
	var setIDs, repoIDs = map[string]struct{}{}, map[string]struct{}{}
	for _, e := range entries {
		if e.Kind == projectKindSet {
			setIDs[e.ID] = struct{}{}
		} else {
			repoIDs[e.ID] = struct{}{}
		}
	}
	var sets, repos []namedProject
	if len(setIDs) > 0 {
		listed, err := r.ListRepositorySets(ctx, workspaceID)
		if err != nil {
			return nil
		}
		for _, set := range listed {
			if _, ok := setIDs[set.ID]; ok {
				sets = append(sets, namedProject{set.ID, set.Name})
			}
		}
	}
	if len(repoIDs) > 0 {
		listed, err := r.ListRepositories(ctx, workspaceID)
		if err != nil {
			return nil
		}
		for _, repo := range listed {
			if _, ok := repoIDs[repo.ID]; ok {
				repos = append(repos, namedProject{repo.ID, repo.Name})
			}
		}
	}
	names := make([]string, 0, len(sets)+len(repos))
	for _, group := range [][]namedProject{sets, repos} {
		sort.Slice(group, func(i, j int) bool {
			a, b := strings.ToLower(group[i].name), strings.ToLower(group[j].name)
			if a != b {
				return a < b
			}
			return group[i].id < group[j].id
		})
		for _, p := range group {
			names = append(names, p.name)
		}
	}
	return &names
}
