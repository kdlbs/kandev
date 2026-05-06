package config

import (
	"github.com/kandev/kandev/internal/office/configloader"
	"github.com/kandev/kandev/internal/office/models"
)

// missingNames returns names present in `existing` but not in `incoming`.
func missingNames(existing, incoming []string) []string {
	in := make(map[string]bool, len(incoming))
	for _, name := range incoming {
		in[name] = true
	}
	var out []string
	for _, name := range existing {
		if !in[name] {
			out = append(out, name)
		}
	}
	return out
}

func agentNames(in []*models.AgentInstance) []string {
	out := make([]string, len(in))
	for i, a := range in {
		out[i] = a.Name
	}
	return out
}

func skillSlugs(in []*models.Skill) []string {
	out := make([]string, len(in))
	for i, sk := range in {
		out[i] = sk.Slug
	}
	return out
}

func routineNames(in []*models.Routine) []string {
	out := make([]string, len(in))
	for i, r := range in {
		out[i] = r.Name
	}
	return out
}

func projectNames(in []*models.Project) []string {
	out := make([]string, len(in))
	for i, p := range in {
		out[i] = p.Name
	}
	return out
}

func agentBundleNames(in []AgentConfig) []string {
	out := make([]string, len(in))
	for i, a := range in {
		out[i] = a.Name
	}
	return out
}

func skillBundleSlugs(in []SkillConfig) []string {
	out := make([]string, len(in))
	for i, sk := range in {
		out[i] = sk.Slug
	}
	return out
}

func routineBundleNames(in []RoutineConfig) []string {
	out := make([]string, len(in))
	for i, r := range in {
		out[i] = r.Name
	}
	return out
}

func projectBundleNames(in []ProjectConfig) []string {
	out := make([]string, len(in))
	for i, p := range in {
		out[i] = p.Name
	}
	return out
}

func bundleAgentSet(in []AgentConfig) map[string]bool {
	out := make(map[string]bool, len(in))
	for _, a := range in {
		out[a.Name] = true
	}
	return out
}

func bundleSkillSet(in []SkillConfig) map[string]bool {
	out := make(map[string]bool, len(in))
	for _, sk := range in {
		out[sk.Slug] = true
	}
	return out
}

func bundleRoutineSet(in []RoutineConfig) map[string]bool {
	out := make(map[string]bool, len(in))
	for _, r := range in {
		out[r.Name] = true
	}
	return out
}

func bundleProjectSet(in []ProjectConfig) map[string]bool {
	out := make(map[string]bool, len(in))
	for _, p := range in {
		out[p.Name] = true
	}
	return out
}

// writeBundleToFS writes every entity in the bundle through the FileWriter.
func writeBundleToFS(w *configloader.FileWriter, bundle *ConfigBundle) error {
	return writeBundleEntities(w, bundle)
}

// writeBundleEntities writes every entity in the bundle through the FileWriter.
func writeBundleEntities(w *configloader.FileWriter, bundle *ConfigBundle) error {
	for _, a := range bundle.Agents {
		agent := &models.AgentInstance{
			Name: a.Name, Role: models.AgentRole(a.Role), Icon: a.Icon,
			BudgetMonthlyCents: a.BudgetMonthlyCents, MaxConcurrentSessions: a.MaxConcurrentSessions,
			DesiredSkills: a.DesiredSkills, ExecutorPreference: a.ExecutorPreference,
		}
		if err := w.WriteAgent(defaultWorkspaceName, agent); err != nil {
			return err
		}
	}
	for _, sk := range bundle.Skills {
		content := sk.Content
		if content == "" {
			content = "# " + sk.Name + "\n"
		}
		if err := w.WriteSkill(defaultWorkspaceName, sk.Slug, content); err != nil {
			return err
		}
	}
	for _, r := range bundle.Routines {
		routine := &models.Routine{
			Name: r.Name, Description: r.Description, TaskTemplate: r.TaskTemplate,
			ConcurrencyPolicy: r.ConcurrencyPolicy,
		}
		if err := w.WriteRoutine(defaultWorkspaceName, routine); err != nil {
			return err
		}
	}
	for _, p := range bundle.Projects {
		project := &models.Project{
			Name: p.Name, Description: p.Description, Status: models.ProjectStatus(p.Status),
			Color: p.Color, BudgetCents: p.BudgetCents,
			Repositories: p.Repositories, ExecutorConfig: p.ExecutorConfig,
		}
		if err := w.WriteProject(defaultWorkspaceName, project); err != nil {
			return err
		}
	}
	return nil
}
