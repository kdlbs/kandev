package dto

import (
	"testing"

	"github.com/kandev/kandev/internal/prompts/models"
	"github.com/stretchr/testify/assert"
)

func TestPromptDTOOmitsStaleRepositoryPathForDetachedOverride(t *testing.T) {
	detached := &models.Prompt{
		Name: "hook", Content: "Mine", Source: models.PromptSourceOverride,
		SourcePath: "p/old-hook.md",
	}
	assert.Empty(t, FromPrompt(detached).SourcePath, "legacy detached rows do not expose a stale path")

	linked := &models.Prompt{
		Name: "hook", Content: "Mine", AlternateContent: "Repo",
		Source: models.PromptSourceOverride, SourcePath: "p/current/hook.md",
	}
	assert.Equal(t, "p/current/hook.md", FromPrompt(linked).SourcePath)
}
