package store

import (
	"fmt"
	"time"

	utilcfg "github.com/kandev/kandev/config/utilityagents"
	"github.com/kandev/kandev/internal/utility/models"
)

// seedBuiltinAgents inserts the default built-in utility agents on first run.
// Existing agents are not overwritten, so user customizations are preserved.
func (r *sqliteRepository) seedBuiltinAgents() error {
	for _, agent := range getBuiltinAgents() {
		_, err := r.db.Exec(r.db.Rebind(`
			INSERT INTO utility_agents (id, name, description, prompt, agent_id, model, builtin, enabled, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, 1, 0, ?, ?)
			ON CONFLICT(id) DO NOTHING
		`), agent.ID, agent.Name, agent.Description, agent.Prompt, agent.AgentID, agent.Model,
			agent.CreatedAt, agent.UpdatedAt)
		if err != nil {
			return fmt.Errorf("failed to upsert built-in agent %s: %w", agent.ID, err)
		}
	}
	return nil
}

// builtinDef is a compact definition for a built-in utility agent.
type builtinDef struct {
	id, name, description, promptFile string
}

var builtinDefs = []builtinDef{
	{"builtin-commit-message", "commit-message", "Generate a commit message based on staged changes", "commit-message"},
	{"builtin-commit-description", "commit-description", "Generate a detailed commit description explaining the changes", "commit-description"},
	{"builtin-branch-name", "branch-name", "Generate a branch name based on task description", "branch-name"},
	{"builtin-pr-title", "pr-title", "Generate a PR title based on commits and changes", "pr-title"},
	{"builtin-pr-description", "pr-description", "Generate a PR description based on commits and changes", "pr-description"},
	{"builtin-enhance-prompt", "enhance-prompt", "Enhance and expand a user prompt with context and clarity", "enhance-prompt"},
	{"builtin-summarize-session", "summarize-session", "Summarize a session conversation for context handover", "summarize-session"},
}

// getBuiltinAgents returns the predefined built-in utility agents with prompts from embedded files.
func getBuiltinAgents() []*models.UtilityAgent {
	now := time.Now().UTC()
	agents := make([]*models.UtilityAgent, 0, len(builtinDefs))
	for _, d := range builtinDefs {
		agents = append(agents, &models.UtilityAgent{
			ID:          d.id,
			Name:        d.name,
			Description: d.description,
			Prompt:      utilcfg.Get(d.promptFile),
			Builtin:     true,
			Enabled:     false,
			CreatedAt:   now,
			UpdatedAt:   now,
		})
	}
	return agents
}
