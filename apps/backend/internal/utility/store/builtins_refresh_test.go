package store

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	utilcfg "github.com/kandev/kandev/config/utilityagents"
)

const enhancePromptAgentID = "builtin-enhance-prompt"

// enhancePromptV1 is the enhance-prompt template shipped before the template
// listed changed files instead of embedding the session diff.
const enhancePromptV1 = `Enhance the following user prompt to be clearer and more actionable for a coding assistant.

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
Do not include explanations or meta-commentary around the prompt.`

func TestEnhancePromptV1HashMatchesShippedTemplate(t *testing.T) {
	sum := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.TrimSpace(enhancePromptV1))))
	if sum != enhancePromptV1SHA256 {
		t.Fatalf("legacy enhance-prompt hash = %s, want %s", sum, enhancePromptV1SHA256)
	}
}

// seedStoredEnhancePrompt replaces the stored prompt without touching
// updated_at, which is how a row seeded by an older release looks.
func seedStoredEnhancePrompt(t *testing.T, prompt string) *sqliteRepository {
	t.Helper()
	sqlxDB := openTestDB(t)
	if _, err := newSQLiteRepositoryWithDB(sqlxDB, sqlxDB); err != nil {
		t.Fatalf("initial schema init: %v", err)
	}
	if _, err := sqlxDB.Exec(`UPDATE utility_agents SET prompt = ? WHERE id = ?`, prompt, enhancePromptAgentID); err != nil {
		t.Fatalf("seed stored prompt: %v", err)
	}
	repo, err := newSQLiteRepositoryWithDB(sqlxDB, sqlxDB)
	if err != nil {
		t.Fatalf("replay schema: %v", err)
	}
	return repo
}

func storedEnhancePrompt(t *testing.T, repo *sqliteRepository) string {
	t.Helper()
	agent, err := repo.GetAgentByID(context.Background(), enhancePromptAgentID)
	if err != nil {
		t.Fatalf("read builtin: %v", err)
	}
	return agent.Prompt
}

func TestRefreshUneditedBuiltinPromptUpgradesShippedRevision(t *testing.T) {
	repo := seedStoredEnhancePrompt(t, enhancePromptV1)

	got := storedEnhancePrompt(t, repo)
	if got != utilcfg.Get("enhance-prompt") {
		t.Fatalf("prompt was not refreshed to the embedded template: %q", got)
	}
	if !strings.Contains(got, "{{ChangedFiles}}") || strings.Contains(got, "{{GitDiff}}") {
		t.Fatalf("refreshed prompt should list changed files, not the diff: %q", got)
	}
}

// A row from before the placeholder migration stores {{.Key}} placeholders. The
// migration normalizes it first, so it is still recognized as shipped text.
func TestRefreshUneditedBuiltinPromptUpgradesDottedPlaceholderRevision(t *testing.T) {
	dotted := strings.NewReplacer(
		"{{UserPrompt}}", "{{.UserPrompt}}",
		"{{TaskTitle}}", "{{.TaskTitle}}",
		"{{TaskDescription}}", "{{.TaskDescription}}",
		"{{GitDiff}}", "{{.GitDiff}}",
	).Replace(enhancePromptV1)
	repo := seedStoredEnhancePrompt(t, dotted)

	if got := storedEnhancePrompt(t, repo); got != utilcfg.Get("enhance-prompt") {
		t.Fatalf("dotted legacy prompt was not refreshed: %q", got)
	}
}

func TestRefreshUneditedBuiltinPromptKeepsUnknownText(t *testing.T) {
	const unknown = "A prompt no release shipped {{GitDiff}}"
	repo := seedStoredEnhancePrompt(t, unknown)

	if got := storedEnhancePrompt(t, repo); got != unknown {
		t.Fatalf("unrecognized prompt = %q, want it preserved", got)
	}
}

func TestRefreshUneditedBuiltinPromptPreservesUserEdit(t *testing.T) {
	sqlxDB := openTestDB(t)
	repo, err := newSQLiteRepositoryWithDB(sqlxDB, sqlxDB)
	if err != nil {
		t.Fatalf("initial schema init: %v", err)
	}
	ctx := context.Background()
	agent, err := repo.GetAgentByID(ctx, enhancePromptAgentID)
	if err != nil {
		t.Fatalf("read builtin: %v", err)
	}
	const customizedPrompt = enhancePromptV1 + "\n\nKeep the response under three sentences."
	agent.Prompt = customizedPrompt
	if err := repo.UpdateAgent(ctx, agent); err != nil {
		t.Fatalf("edit builtin prompt: %v", err)
	}

	replayed, err := newSQLiteRepositoryWithDB(sqlxDB, sqlxDB)
	if err != nil {
		t.Fatalf("replay schema: %v", err)
	}
	if got := storedEnhancePrompt(t, replayed); got != customizedPrompt {
		t.Fatalf("user-edited prompt = %q, want it preserved", got)
	}
}

func TestRefreshBuiltinPromptAfterProfileOnlyUpdate(t *testing.T) {
	sqlxDB := openTestDB(t)
	repo, err := newSQLiteRepositoryWithDB(sqlxDB, sqlxDB)
	if err != nil {
		t.Fatalf("initial schema init: %v", err)
	}
	if _, err := sqlxDB.Exec(`UPDATE utility_agents SET prompt = ? WHERE id = ?`, enhancePromptV1, enhancePromptAgentID); err != nil {
		t.Fatalf("seed stored prompt: %v", err)
	}
	ctx := context.Background()
	agent, err := repo.GetAgentByID(ctx, enhancePromptAgentID)
	if err != nil {
		t.Fatalf("read builtin: %v", err)
	}
	agent.AgentProfileID = "profile-1"
	agent.ProfileBindingState = "explicit"
	if err := repo.UpdateAgent(ctx, agent); err != nil {
		t.Fatalf("update builtin profile binding: %v", err)
	}
	updated, err := repo.GetAgentByID(ctx, enhancePromptAgentID)
	if err != nil {
		t.Fatalf("read updated builtin: %v", err)
	}
	if updated.CreatedAt.Equal(updated.UpdatedAt) {
		t.Fatal("profile-only update did not advance updated_at")
	}
	if updated.Prompt != enhancePromptV1 {
		t.Fatalf("profile-only update changed the legacy prompt before replay: %q", updated.Prompt)
	}

	replayed, err := newSQLiteRepositoryWithDB(repo.db, repo.db)
	if err != nil {
		t.Fatalf("replay schema: %v", err)
	}
	refreshed, err := replayed.GetAgentByID(ctx, enhancePromptAgentID)
	if err != nil {
		t.Fatalf("read refreshed builtin: %v", err)
	}
	if refreshed.Prompt != utilcfg.Get("enhance-prompt") {
		t.Fatalf("prompt was not refreshed after profile-only update: %q", refreshed.Prompt)
	}
	if refreshed.AgentProfileID != "profile-1" || refreshed.ProfileBindingState != "explicit" {
		t.Fatalf("profile binding changed during prompt refresh: (%q, %q)", refreshed.AgentProfileID, refreshed.ProfileBindingState)
	}
}
