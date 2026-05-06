package service

import (
	"context"

	"github.com/kandev/kandev/internal/office/configloader"
)

// appendDeletions adds names of DB rows missing from the bundle to preview.*.Deleted.
func (s *Service) appendDeletions(
	ctx context.Context, workspaceID string, bundle *ConfigBundle, preview *ImportPreview,
) error {
	agents, err := s.ListAgentsFromConfig(ctx, workspaceID)
	if err != nil {
		return err
	}
	skills, err := s.ListSkillsFromConfig(ctx, workspaceID)
	if err != nil {
		return err
	}
	routines, err := s.ListRoutinesFromConfig(ctx, workspaceID)
	if err != nil {
		return err
	}
	projects, err := s.ListProjectsFromConfig(ctx, workspaceID)
	if err != nil {
		return err
	}
	preview.Agents.Deleted = missingNames(
		agentNames(agents), agentBundleNames(bundle.Agents))
	preview.Skills.Deleted = missingNames(
		skillSlugs(skills), skillBundleSlugs(bundle.Skills))
	preview.Routines.Deleted = missingNames(
		routineNames(routines), routineBundleNames(bundle.Routines))
	preview.Projects.Deleted = missingNames(
		projectNames(projects), projectBundleNames(bundle.Projects))
	return nil
}

// deleteRowsMissingFromBundle removes DB rows that are not present in the bundle.
func (s *Service) deleteRowsMissingFromBundle(
	ctx context.Context, workspaceID string, bundle *ConfigBundle,
) error {
	if err := s.deleteMissingAgents(ctx, workspaceID, bundle); err != nil {
		return err
	}
	if err := s.deleteMissingSkills(ctx, workspaceID, bundle); err != nil {
		return err
	}
	if err := s.deleteMissingRoutines(ctx, workspaceID, bundle); err != nil {
		return err
	}
	return s.deleteMissingProjects(ctx, workspaceID, bundle)
}

func (s *Service) deleteMissingAgents(
	ctx context.Context, workspaceID string, bundle *ConfigBundle,
) error {
	rows, err := s.ListAgentsFromConfig(ctx, workspaceID)
	if err != nil {
		return err
	}
	keep := bundleAgentSet(bundle.Agents)
	for _, a := range rows {
		if keep[a.Name] {
			continue
		}
		if err := s.DeleteAgentInstance(ctx, a.ID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) deleteMissingSkills(
	ctx context.Context, workspaceID string, bundle *ConfigBundle,
) error {
	rows, err := s.ListSkillsFromConfig(ctx, workspaceID)
	if err != nil {
		return err
	}
	keep := bundleSkillSet(bundle.Skills)
	for _, sk := range rows {
		if keep[sk.Slug] {
			continue
		}
		if err := s.DeleteSkill(ctx, sk.ID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) deleteMissingRoutines(
	ctx context.Context, workspaceID string, bundle *ConfigBundle,
) error {
	rows, err := s.ListRoutinesFromConfig(ctx, workspaceID)
	if err != nil {
		return err
	}
	keep := bundleRoutineSet(bundle.Routines)
	for _, r := range rows {
		if keep[r.Name] {
			continue
		}
		if err := s.DeleteRoutine(ctx, r.ID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) deleteMissingProjects(
	ctx context.Context, workspaceID string, bundle *ConfigBundle,
) error {
	rows, err := s.ListProjectsFromConfig(ctx, workspaceID)
	if err != nil {
		return err
	}
	keep := bundleProjectSet(bundle.Projects)
	for _, p := range rows {
		if keep[p.Name] {
			continue
		}
		if err := s.DeleteProject(ctx, p.ID); err != nil {
			return err
		}
	}
	return nil
}

// diffBundles returns an ImportPreview describing how `incoming` differs from `existing`.
func diffBundles(incoming, existing *ConfigBundle) *ImportPreview {
	preview := &ImportPreview{}
	existAgents := bundleAgentSet(existing.Agents)
	for _, a := range incoming.Agents {
		if existAgents[a.Name] {
			preview.Agents.Updated = append(preview.Agents.Updated, a.Name)
		} else {
			preview.Agents.Created = append(preview.Agents.Created, a.Name)
		}
	}
	preview.Agents.Deleted = missingNames(
		agentBundleNames(existing.Agents), agentBundleNames(incoming.Agents))

	existSkills := bundleSkillSet(existing.Skills)
	for _, sk := range incoming.Skills {
		if existSkills[sk.Slug] {
			preview.Skills.Updated = append(preview.Skills.Updated, sk.Slug)
		} else {
			preview.Skills.Created = append(preview.Skills.Created, sk.Slug)
		}
	}
	preview.Skills.Deleted = missingNames(
		skillBundleSlugs(existing.Skills), skillBundleSlugs(incoming.Skills))

	existRoutines := bundleRoutineSet(existing.Routines)
	for _, r := range incoming.Routines {
		if existRoutines[r.Name] {
			preview.Routines.Updated = append(preview.Routines.Updated, r.Name)
		} else {
			preview.Routines.Created = append(preview.Routines.Created, r.Name)
		}
	}
	preview.Routines.Deleted = missingNames(
		routineBundleNames(existing.Routines), routineBundleNames(incoming.Routines))

	existProjects := bundleProjectSet(existing.Projects)
	for _, p := range incoming.Projects {
		if existProjects[p.Name] {
			preview.Projects.Updated = append(preview.Projects.Updated, p.Name)
		} else {
			preview.Projects.Created = append(preview.Projects.Created, p.Name)
		}
	}
	preview.Projects.Deleted = missingNames(
		projectBundleNames(existing.Projects), projectBundleNames(incoming.Projects))
	return preview
}

// writeBundleToFS writes every entity in the bundle through the FileWriter.
func writeBundleToFS(w *configloader.FileWriter, bundle *ConfigBundle) error {
	return writeBundleEntities(w, bundle)
}
