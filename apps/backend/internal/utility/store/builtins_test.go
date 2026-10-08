package store

import (
	"context"
	"strings"
	"testing"
)

// TestSeedBuiltinAgents_IncludesSuggestNextPrompt verifies the prompt-suggestion
// fallback built-in is seeded with a template that encodes the prediction rules.
func TestSeedBuiltinAgents_IncludesSuggestNextPrompt(t *testing.T) {
	sqlxDB := openTestDB(t)
	if _, err := newSQLiteRepositoryWithDB(sqlxDB, sqlxDB); err != nil {
		t.Fatalf("schema init: %v", err)
	}

	var name, prompt string
	err := sqlxDB.QueryRowxContext(context.Background(),
		`SELECT name, prompt FROM utility_agents WHERE id = ? AND builtin = 1`, SuggestNextPromptAgentID,
	).Scan(&name, &prompt)
	if err != nil {
		t.Fatalf("query suggest-next-prompt built-in: %v", err)
	}
	if name != "suggest-next-prompt" {
		t.Fatalf("name = %q, want suggest-next-prompt", name)
	}
	for _, want := range []string{
		"{{TaskTitle}}",
		"{{ConversationHistory}}",
		"2 to 12 words",
		"language",
		"silent",
		"follow-up question",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
	// A follow-up question is often the natural next message outside coding.
	if strings.Contains(prompt, "- A question.") {
		t.Error("prompt must not forbid follow-up questions")
	}
}
