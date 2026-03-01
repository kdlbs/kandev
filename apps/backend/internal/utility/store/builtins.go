package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/utility/models"
)

// seedBuiltinAgents creates the default built-in utility agents if they don't exist.
func (r *sqliteRepository) seedBuiltinAgents() error {
	builtinAgents := r.getBuiltinAgents()

	for _, agent := range builtinAgents {
		// Check if agent already exists
		var exists bool
		err := r.db.QueryRow(r.db.Rebind("SELECT 1 FROM utility_agents WHERE id = ?"), agent.ID).Scan(&exists)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		if exists {
			continue
		}

		// Insert built-in agent (disabled by default, user must configure agent/model)
		_, err = r.db.Exec(r.db.Rebind(`
			INSERT INTO utility_agents (id, name, description, prompt, agent_id, model, builtin, enabled, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`), agent.ID, agent.Name, agent.Description, agent.Prompt, agent.AgentID, agent.Model,
			1, 0, agent.CreatedAt, agent.UpdatedAt)
		if err != nil {
			return fmt.Errorf("failed to insert built-in agent %s: %w", agent.ID, err)
		}
	}

	return nil
}

// getBuiltinAgents returns the predefined built-in utility agents.
func (r *sqliteRepository) getBuiltinAgents() []*models.UtilityAgent {
	now := time.Now().UTC()
	return []*models.UtilityAgent{
		r.builtinCommitMessage(now),
		r.builtinBranchName(now),
		r.builtinPRDescription(now),
		r.builtinEnhancePrompt(now),
	}
}

func (r *sqliteRepository) builtinCommitMessage(now time.Time) *models.UtilityAgent {
	return &models.UtilityAgent{
		ID:          "builtin-commit-message",
		Name:        "commit-message",
		Description: "Generate a commit message based on staged changes",
		AgentID:     "", // User must configure
		Model:       "", // User must configure
		Builtin:     true,
		Enabled:     false, // Disabled until user configures
		CreatedAt:   now,
		UpdatedAt:   now,
		Prompt: `Generate a concise git commit message for the following changes.

## Staged Changes (git diff --staged):
{{GitDiff}}

## Instructions:
1. Follow the Conventional Commits format: <type>(<scope>): <description>
2. Types: feat, fix, docs, style, refactor, perf, test, build, ci, chore
3. Scope is optional but recommended (e.g., api, ui, db)
4. Description should be imperative mood ("add" not "added")
5. Keep the first line under 72 characters
6. If needed, add a blank line and bullet points for details

## Output Format:
Return ONLY the commit message, no explanations or markdown.`,
	}
}

func (r *sqliteRepository) builtinBranchName(now time.Time) *models.UtilityAgent {
	return &models.UtilityAgent{
		ID:          "builtin-branch-name",
		Name:        "branch-name",
		Description: "Generate a branch name based on task description",
		AgentID:     "", // User must configure
		Model:       "", // User must configure
		Builtin:     true,
		Enabled:     false, // Disabled until user configures
		CreatedAt:   now,
		UpdatedAt:   now,
		Prompt: `Generate a git branch name for the following task.

## Task Title:
{{TaskTitle}}

## Task Description:
{{TaskDescription}}

## Instructions:
1. Use kebab-case (lowercase with hyphens)
2. Start with a type prefix: feature/, fix/, refactor/, docs/, chore/
3. Keep it concise but descriptive (max 50 characters total)
4. Include relevant keywords from the task
5. Avoid special characters except hyphens

## Output Format:
Return ONLY the branch name, no explanations.
Example: feature/add-user-authentication`,
	}
}

func (r *sqliteRepository) builtinPRDescription(now time.Time) *models.UtilityAgent {
	return &models.UtilityAgent{
		ID:          "builtin-pr-description",
		Name:        "pr-description",
		Description: "Generate a PR description based on commits and changes",
		AgentID:     "", // User must configure
		Model:       "", // User must configure
		Builtin:     true,
		Enabled:     false, // Disabled until user configures
		CreatedAt:   now,
		UpdatedAt:   now,
		Prompt: `Generate a Pull Request description based on the following information.

## Commits:
{{CommitLog}}

## Changed Files:
{{ChangedFiles}}

## Diff Summary:
{{DiffSummary}}

## Instructions:
1. Start with a clear title summarizing the PR
2. Include a "## Summary" section explaining what this PR does
3. Add a "## Changes" section with bullet points of key changes
4. Include a "## Testing" section describing how to test
5. If applicable, mention breaking changes or migration steps
6. Keep it professional and concise

## Output Format:
Return ONLY the PR description in Markdown format. And nothing else.`,
	}
}

func (r *sqliteRepository) builtinEnhancePrompt(now time.Time) *models.UtilityAgent {
	return &models.UtilityAgent{
		ID:          "builtin-enhance-prompt",
		Name:        "enhance-prompt",
		Description: "Enhance and expand a user prompt with context and clarity",
		AgentID:     "", // User must configure
		Model:       "", // User must configure
		Builtin:     true,
		Enabled:     false, // Disabled until user configures
		CreatedAt:   now,
		UpdatedAt:   now,
		Prompt: `Enhance the following user prompt to be clearer and more actionable for a coding assistant.

## User's Original Prompt:
{{UserPrompt}}

## Task Context:
Title: {{TaskTitle}}
Description: {{TaskDescription}}

## Current Git Changes (if any):
{{GitDiff}}

## Instructions:
1. Expand vague requests into specific, actionable instructions
2. Add relevant context from the task and git changes
3. Clarify ambiguous terms or requirements
4. Structure the prompt clearly with numbered steps if appropriate
5. Keep the original intent - don't change what the user wants to do
6. Be concise - don't add unnecessary verbosity

## Output Format:
Return ONLY the enhanced prompt text, ready to be sent to the coding assistant.
Do not include explanations or meta-commentary around the prompt.`,
	}
}
