package config

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// PreviewImport diffs a bundle against the current workspace state.
func (s *ConfigService) PreviewImport(
	ctx context.Context, workspaceID string, bundle *ConfigBundle,
) (*ImportPreview, error) {
	preview := &ImportPreview{}
	if err := s.previewAgents(ctx, workspaceID, bundle.Agents, &preview.Agents); err != nil {
		return nil, err
	}
	if err := s.previewSkills(ctx, workspaceID, bundle.Skills, &preview.Skills); err != nil {
		return nil, err
	}
	if err := s.previewRoutines(ctx, workspaceID, bundle.Routines, &preview.Routines); err != nil {
		return nil, err
	}
	if err := s.previewProjects(ctx, workspaceID, bundle.Projects, &preview.Projects); err != nil {
		return nil, err
	}
	return preview, nil
}

func (s *ConfigService) previewAgents(
	ctx context.Context, wsID string, incoming []AgentConfig, diff *ImportDiff,
) error {
	existing, err := s.repo.ListAgentInstances(ctx, wsID)
	if err != nil {
		return err
	}
	byName := make(map[string]bool, len(existing))
	for _, a := range existing {
		byName[a.Name] = true
	}
	for _, a := range incoming {
		if byName[a.Name] {
			diff.Updated = append(diff.Updated, a.Name)
		} else {
			diff.Created = append(diff.Created, a.Name)
		}
	}
	return nil
}

func (s *ConfigService) previewSkills(
	ctx context.Context, wsID string, incoming []SkillConfig, diff *ImportDiff,
) error {
	existing, err := s.repo.ListSkills(ctx, wsID)
	if err != nil {
		return err
	}
	bySlug := make(map[string]bool, len(existing))
	for _, sk := range existing {
		bySlug[sk.Slug] = true
	}
	for _, sk := range incoming {
		if bySlug[sk.Slug] {
			diff.Updated = append(diff.Updated, sk.Slug)
		} else {
			diff.Created = append(diff.Created, sk.Slug)
		}
	}
	return nil
}

func (s *ConfigService) previewRoutines(
	ctx context.Context, wsID string, incoming []RoutineConfig, diff *ImportDiff,
) error {
	existing, err := s.repo.ListRoutines(ctx, wsID)
	if err != nil {
		return err
	}
	byName := make(map[string]bool, len(existing))
	for _, r := range existing {
		byName[r.Name] = true
	}
	for _, r := range incoming {
		if byName[r.Name] {
			diff.Updated = append(diff.Updated, r.Name)
		} else {
			diff.Created = append(diff.Created, r.Name)
		}
	}
	return nil
}

func (s *ConfigService) previewProjects(
	ctx context.Context, wsID string, incoming []ProjectConfig, diff *ImportDiff,
) error {
	existing, err := s.repo.ListProjects(ctx, wsID)
	if err != nil {
		return err
	}
	byName := make(map[string]bool, len(existing))
	for _, p := range existing {
		byName[p.Name] = true
	}
	for _, p := range incoming {
		if byName[p.Name] {
			diff.Updated = append(diff.Updated, p.Name)
		} else {
			diff.Created = append(diff.Created, p.Name)
		}
	}
	return nil
}

// ApplyImport applies a config bundle to the workspace, deduplicating by name.
func (s *ConfigService) ApplyImport(
	ctx context.Context, workspaceID string, bundle *ConfigBundle,
) (*ImportResult, error) {
	s.importMu.Lock()
	defer s.importMu.Unlock()

	return s.applyImport(ctx, workspaceID, bundle, true)
}

// applyImport applies a bundle with the reports_to lookup policy for its
// caller. Direct bundle imports may reference an existing manager that is not
// in the bundle. Filesystem syncs are authoritative snapshots, so they must
// resolve only against agents present in the snapshot before stale rows are
// pruned.
func (s *ConfigService) applyImport(
	ctx context.Context, workspaceID string, bundle *ConfigBundle,
	allowExternalManagers bool,
) (*ImportResult, error) {
	result := &ImportResult{}

	if err := s.applyAgents(ctx, workspaceID, bundle.Agents, result, allowExternalManagers); err != nil {
		return nil, fmt.Errorf("apply agents: %w", err)
	}
	if err := s.applySkills(ctx, workspaceID, bundle.Skills, result); err != nil {
		return nil, fmt.Errorf("apply skills: %w", err)
	}
	if err := s.applyRoutines(ctx, workspaceID, bundle.Routines, result); err != nil {
		return nil, fmt.Errorf("apply routines: %w", err)
	}
	if err := s.applyProjects(ctx, workspaceID, bundle.Projects, result); err != nil {
		return nil, fmt.Errorf("apply projects: %w", err)
	}

	s.activity.LogActivity(ctx, workspaceID, "user", "",
		"config_imported", "workspace", workspaceID,
		fmt.Sprintf("created=%d updated=%d", result.CreatedCount, result.UpdatedCount))

	return result, nil
}

func (s *ConfigService) applyAgents(
	ctx context.Context, wsID string, incoming []AgentConfig, result *ImportResult,
	allowExternalManagers bool,
) error {
	if duplicateName, ok := duplicateAgentName(incoming); ok {
		return fmt.Errorf("duplicate agent name %q in import bundle", duplicateName)
	}

	existing, err := s.repo.ListAgentInstances(ctx, wsID)
	if err != nil {
		return err
	}
	byName := make(map[string]*models.AgentInstance, len(existing))
	for _, a := range existing {
		byName[a.Name] = a
	}
	for _, cfg := range incoming {
		if agent, ok := byName[cfg.Name]; ok {
			fields := sqlite.AgentInstanceConfigFields{
				Role:                  cfg.Role,
				Icon:                  cfg.Icon,
				BudgetMonthlyCents:    cfg.BudgetMonthlyCents,
				MaxConcurrentSessions: cfg.MaxConcurrentSessions,
				DesiredSkills:         cfg.DesiredSkills,
				ExecutorPreference:    cfg.ExecutorPreference,
			}
			if err := s.repo.UpdateAgentInstanceConfigFields(ctx, agent.ID, fields); err != nil {
				return err
			}
			result.UpdatedCount++
		} else {
			agent := &models.AgentInstance{
				WorkspaceID:           wsID,
				Name:                  cfg.Name,
				Role:                  models.AgentRole(cfg.Role),
				Icon:                  cfg.Icon,
				Status:                models.AgentStatusIdle,
				BudgetMonthlyCents:    cfg.BudgetMonthlyCents,
				MaxConcurrentSessions: cfg.MaxConcurrentSessions,
				DesiredSkills:         cfg.DesiredSkills,
				ExecutorPreference:    cfg.ExecutorPreference,
			}
			if err := s.repo.CreateAgentInstance(ctx, agent); err != nil {
				return err
			}
			result.CreatedCount++
		}
	}
	return s.applyAgentReportsTo(ctx, wsID, incoming, result, allowExternalManagers)
}

func duplicateAgentName(incoming []AgentConfig) (string, bool) {
	seen := make(map[string]struct{}, len(incoming))
	for _, cfg := range incoming {
		if _, ok := seen[cfg.Name]; ok {
			return cfg.Name, true
		}
		seen[cfg.Name] = struct{}{}
	}
	return "", false
}

// applyAgentReportsTo resolves each incoming agent's reports_to name to the
// target agent's ID and persists it. It runs after every agent in the bundle
// has been created or updated above, so it re-lists the workspace to pick up
// IDs assigned to newly-created rows — this makes resolution independent of
// bundle order. When allowExternalManagers is true, a reports_to name may
// also reference an agent that is only in the workspace. Filesystem syncs pass
// false because the snapshot is authoritative and rows absent from it are
// pruned after this import. A name that resolves to nothing (dangling
// reference), to the agent itself, or that would create a cycle in the
// reporting hierarchy is recorded as a warning and left empty rather than
// failing the import.
func (s *ConfigService) applyAgentReportsTo(
	ctx context.Context, wsID string, incoming []AgentConfig, result *ImportResult,
	allowExternalManagers bool,
) error {
	current, err := s.repo.ListAgentInstances(ctx, wsID)
	if err != nil {
		return fmt.Errorf("resolve reports_to: list agents: %w", err)
	}
	byName := make(map[string]*models.AgentInstance, len(current))
	byID := make(map[string]*models.AgentInstance, len(current))
	incomingNames := bundleAgentSet(incoming)
	for _, a := range current {
		if !allowExternalManagers && !incomingNames[a.Name] {
			continue
		}
		byName[a.Name] = a
		byID[a.ID] = a
	}
	incomingByName := make(map[string]AgentConfig, len(incoming))
	for _, cfg := range incoming {
		incomingByName[cfg.Name] = cfg
	}
	for _, cfg := range incoming {
		agent, ok := byName[cfg.Name]
		if !ok {
			continue
		}
		reportsTo, warning := resolveReportsTo(cfg, byName, byID, incomingByName)
		if warning != "" {
			result.Warnings = append(result.Warnings, warning)
		}
		if agent.ReportsTo == reportsTo {
			continue
		}
		if err := s.repo.UpdateAgentReportsTo(ctx, agent.ID, reportsTo); err != nil {
			return fmt.Errorf("resolve reports_to: update %q: %w", agent.Name, err)
		}
	}
	return nil
}

// resolveReportsTo resolves cfg.ReportsTo (a name) against byName (a name ->
// agent index built from the target workspace after apply, already filtered
// to the callers' allowExternalManagers policy). It returns the resolved
// manager ID, or an empty string plus a warning when the name is a
// self-reference, does not match any known agent, or would create a cycle in
// the reporting hierarchy.
func resolveReportsTo(
	cfg AgentConfig,
	byName map[string]*models.AgentInstance,
	byID map[string]*models.AgentInstance,
	incomingByName map[string]AgentConfig,
) (string, string) {
	if cfg.ReportsTo == "" {
		return "", ""
	}
	if cfg.ReportsTo == cfg.Name {
		return "", fmt.Sprintf("agent %q cannot report to itself", cfg.Name)
	}
	manager, ok := byName[cfg.ReportsTo]
	if !ok {
		return "", fmt.Sprintf("agent %q reports_to %q, which was not found", cfg.Name, cfg.ReportsTo)
	}
	if reportsToCycleExists(cfg.Name, cfg.ReportsTo, byName, byID, incomingByName) {
		return "", fmt.Sprintf("agent %q reports_to %q, which would create a cycle", cfg.Name, cfg.ReportsTo)
	}
	return manager.ID, ""
}

// reportsToCycleExists walks the proposed manager chain upward from start,
// looking for target. Each hop is resolved against the proposed graph: when
// the current node is itself part of the incoming bundle, its parent is the
// bundle's declared reports_to name; otherwise its parent comes from
// byName/byID, the same allowExternalManagers-filtered view used to resolve
// the direct manager lookup above — an agent a sync would already reject as
// a manager (because it fell outside the filter) is equally invisible as an
// intermediate hop, since such an edge can never end up persisted anyway.
// The walk is iterative and bounded by a visited set (plus a hard hop cap as
// a second bound) so a pre-existing cyclic row already in the database
// cannot hang resolution of an unrelated agent.
func reportsToCycleExists(
	target, start string,
	byName map[string]*models.AgentInstance,
	byID map[string]*models.AgentInstance,
	incomingByName map[string]AgentConfig,
) bool {
	visited := make(map[string]bool, len(byName)+len(incomingByName))
	node := start
	// The visited set is the primary bound. Keep a small secondary cap in case
	// a future graph representation makes the set incomplete.
	maxHops := len(byName) + 1
	for i := 0; i < maxHops; i++ {
		if node == target {
			return true
		}
		if visited[node] {
			return false
		}
		visited[node] = true

		var parent string
		if incCfg, ok := incomingByName[node]; ok {
			parent = incCfg.ReportsTo
		} else if agent, ok := byName[node]; ok && agent.ReportsTo != "" {
			if parentAgent, ok := byID[agent.ReportsTo]; ok {
				parent = parentAgent.Name
			}
		}
		if parent == "" {
			return false
		}
		node = parent
	}
	return false
}

func (s *ConfigService) applySkills(
	ctx context.Context, wsID string, incoming []SkillConfig, result *ImportResult,
) error {
	existing, err := s.repo.ListSkills(ctx, wsID)
	if err != nil {
		return err
	}
	bySlug := make(map[string]*models.Skill, len(existing))
	for _, sk := range existing {
		bySlug[sk.Slug] = sk
	}
	for _, cfg := range incoming {
		if skill, ok := bySlug[cfg.Slug]; ok {
			fields := sqlite.SkillConfigFields{
				Name:        cfg.Name,
				Description: cfg.Description,
				SourceType:  models.SkillSourceType(cfg.SourceType),
				Content:     cfg.Content,
			}
			if err := s.repo.UpdateSkillConfigFields(ctx, skill.ID, fields); err != nil {
				return err
			}
			result.UpdatedCount++
		} else {
			skill := &models.Skill{
				WorkspaceID: wsID,
				Name:        cfg.Name,
				Slug:        cfg.Slug,
				Description: cfg.Description,
				SourceType:  models.SkillSourceType(cfg.SourceType),
				Content:     cfg.Content,
			}
			if err := s.repo.CreateSkill(ctx, skill); err != nil {
				return err
			}
			result.CreatedCount++
		}
	}
	return nil
}

func (s *ConfigService) applyRoutines(
	ctx context.Context, wsID string, incoming []RoutineConfig, result *ImportResult,
) error {
	existing, err := s.repo.ListRoutines(ctx, wsID)
	if err != nil {
		return err
	}
	byName := make(map[string]*models.Routine, len(existing))
	for _, r := range existing {
		byName[r.Name] = r
	}
	for _, cfg := range incoming {
		if routine, ok := byName[cfg.Name]; ok {
			fields := sqlite.RoutineConfigFields{
				Description:       cfg.Description,
				TaskTemplate:      cfg.TaskTemplate,
				ConcurrencyPolicy: models.RoutineConcurrencyPolicy(cfg.ConcurrencyPolicy),
			}
			if err := s.repo.UpdateRoutineConfigFields(ctx, routine.ID, fields); err != nil {
				return err
			}
			result.UpdatedCount++
		} else {
			routine := &models.Routine{
				WorkspaceID:       wsID,
				Name:              cfg.Name,
				Description:       cfg.Description,
				TaskTemplate:      cfg.TaskTemplate,
				Status:            "active",
				ConcurrencyPolicy: models.RoutineConcurrencyPolicy(cfg.ConcurrencyPolicy),
			}
			if err := s.repo.CreateRoutine(ctx, routine); err != nil {
				return err
			}
			result.CreatedCount++
		}
	}
	return nil
}

func (s *ConfigService) applyProjects(
	ctx context.Context, wsID string, incoming []ProjectConfig, result *ImportResult,
) error {
	existing, err := s.repo.ListProjects(ctx, wsID)
	if err != nil {
		return err
	}
	byName := make(map[string]*models.Project, len(existing))
	for _, p := range existing {
		byName[p.Name] = p
	}
	for _, cfg := range incoming {
		if project, ok := byName[cfg.Name]; ok {
			fields := sqlite.ProjectConfigFields{
				Description:    cfg.Description,
				Color:          cfg.Color,
				BudgetCents:    cfg.BudgetCents,
				Repositories:   cfg.Repositories,
				ExecutorConfig: cfg.ExecutorConfig,
			}
			if err := s.repo.UpdateProjectConfigFields(ctx, project.ID, fields); err != nil {
				return err
			}
			result.UpdatedCount++
		} else {
			project := &models.Project{
				WorkspaceID:    wsID,
				Name:           cfg.Name,
				Description:    cfg.Description,
				Status:         models.ProjectStatusActive,
				Color:          cfg.Color,
				BudgetCents:    cfg.BudgetCents,
				Repositories:   cfg.Repositories,
				ExecutorConfig: cfg.ExecutorConfig,
			}
			if err := s.repo.CreateProject(ctx, project); err != nil {
				return err
			}
			result.CreatedCount++
		}
	}
	return nil
}
