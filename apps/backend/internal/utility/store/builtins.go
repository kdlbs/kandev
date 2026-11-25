package store

import (
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	utilcfg "github.com/kandev/kandev/config/utilityagents"
	"github.com/kandev/kandev/internal/utility/models"
)

// seedBuiltinAgents inserts the default built-in utility agents on first run.
// Existing agents are not overwritten, so user customizations are preserved.
func (r *sqliteRepository) seedBuiltinAgents() error {
	for _, agent := range getBuiltinAgents() {
		_, err := r.db.Exec(r.db.Rebind(`
			INSERT INTO utility_agents (id, name, description, prompt, agent_id, model, agent_profile_id, profile_binding_state, builtin, enabled, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, '', 'inherit', 1, 0, ?, ?)
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
	{"builtin-code-review", "code-review", "Review the task's changed files and return anchored findings", "code-review"},
}

// CodeReviewAgentID is the built-in utility agent that supplies the default
// reviewer identity for a native code-review pass.
const CodeReviewAgentID = "builtin-code-review"

// enhancePromptV1SHA256 identifies the enhance-prompt template shipped before
// it listed changed files instead of embedding the session diff. Hashes use the
// same TrimSpace normalization as utilcfg.Get.
const enhancePromptV1SHA256 = "2196530e1daf3ecbbf8b8d3883aa98475db5dc052b11ca5c0e02533eb351de15"

// builtinPromptRefresh replaces a stored built-in prompt with the current
// embedded template when the stored text is an exact shipped revision.
type builtinPromptRefresh struct {
	id           string
	promptFile   string
	legacyHashes []string
}

var builtinPromptRefreshes = []builtinPromptRefresh{
	{id: "builtin-enhance-prompt", promptFile: "enhance-prompt", legacyHashes: []string{enhancePromptV1SHA256}},
}

// refreshUneditedBuiltinPrompts upgrades known built-in prompt revisions.
// Custom prompt text and profile bindings remain unchanged.
func (r *sqliteRepository) refreshUneditedBuiltinPrompts() error {
	for _, refresh := range builtinPromptRefreshes {
		if err := r.refreshUneditedBuiltinPrompt(refresh); err != nil {
			return err
		}
	}
	return nil
}

func (r *sqliteRepository) refreshUneditedBuiltinPrompt(refresh builtinPromptRefresh) error {
	var storedPrompt string
	var builtinInt int
	err := r.db.QueryRow(r.db.Rebind(`
		SELECT prompt, builtin
		FROM utility_agents
		WHERE id = ?
	`), refresh.id).Scan(&storedPrompt, &builtinInt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read stored built-in prompt %s: %w", refresh.id, err)
	}
	if builtinInt != 1 || !isShippedPromptRevision(storedPrompt, refresh.legacyHashes) {
		return nil
	}
	if _, err := r.db.Exec(r.db.Rebind(`
		UPDATE utility_agents
		SET prompt = ?
		WHERE id = ? AND builtin = ? AND prompt = ?
	`), utilcfg.Get(refresh.promptFile), refresh.id, builtinInt, storedPrompt); err != nil {
		return fmt.Errorf("refresh stored built-in prompt %s: %w", refresh.id, err)
	}
	return nil
}

func isShippedPromptRevision(prompt string, hashes []string) bool {
	sum := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.TrimSpace(prompt))))
	return slices.Contains(hashes, sum)
}

// builtinSeedAgentID is the inference-agent ID embedded in seeded built-in
// rows. Kept aligned with the schema DEFAULT so a row that survives every
// migration path still maps to a registered inference agent.
const builtinSeedAgentID = "claude-acp"

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
			AgentID:     builtinSeedAgentID,
			Builtin:     true,
			Enabled:     false,
			CreatedAt:   now,
			UpdatedAt:   now,
		})
	}
	return agents
}
