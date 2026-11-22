package controller

import (
	"errors"
	"strings"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/settings/models"
)

const (
	ClaudeExactModelAliasConflictErrorCode = "claude_exact_model_alias_conflict"
	claudeModelEnvVar                      = "ANTHROPIC_MODEL"
	claudeModelAliasSonnet                 = "sonnet"
	claudeModelAliasOpus                   = "opus"
	claudeModelAliasHaiku                  = "haiku"
	claudeModelAliasFable                  = "fable"
)

var ErrClaudeExactModelAliasConflict = errors.New("claude exact model alias conflict")

type claudeModelConflict struct {
	model string
	value string
}

func validateClaudeExactModelAliasConflict(
	agentName string,
	model string,
	requireExactModel bool,
	envVars []models.ProfileEnvVar,
) error {
	if _, conflict := claudeExactModelAliasConflict(agentName, model, requireExactModel, envVars); conflict {
		return ErrClaudeExactModelAliasConflict
	}
	return nil
}

func isNewClaudeExactModelAliasConflict(
	model string,
	requireExactModel bool,
	envVars []models.ProfileEnvVar,
	savedModel string,
	savedRequireExactModel bool,
	savedEnvVars []models.ProfileEnvVar,
) bool {
	current, conflict := claudeModelAliasConflictCandidate(model, requireExactModel, envVars)
	if !conflict {
		return false
	}
	saved, hadConflict := claudeModelAliasConflictCandidate(savedModel, savedRequireExactModel, savedEnvVars)
	return !hadConflict || current != saved
}

func claudeExactModelAliasConflict(
	agentName string,
	model string,
	requireExactModel bool,
	envVars []models.ProfileEnvVar,
) (claudeModelConflict, bool) {
	if agentName != agents.NewClaudeACP().ID() {
		return claudeModelConflict{}, false
	}
	return claudeModelAliasConflictCandidate(model, requireExactModel, envVars)
}

func claudeModelAliasConflictCandidate(
	model string,
	requireExactModel bool,
	envVars []models.ProfileEnvVar,
) (claudeModelConflict, bool) {
	if !requireExactModel || !isBareClaudeModelAlias(model) {
		return claudeModelConflict{}, false
	}
	for _, envVar := range envVars {
		if envVar.Key != claudeModelEnvVar || strings.TrimSpace(envVar.SecretID) != "" {
			continue
		}
		value := strings.TrimSpace(envVar.Value)
		if value != "" && value != model {
			return claudeModelConflict{model: model, value: value}, true
		}
	}
	return claudeModelConflict{}, false
}

func isBareClaudeModelAlias(model string) bool {
	switch model {
	case claudeModelAliasSonnet, claudeModelAliasOpus, claudeModelAliasHaiku, claudeModelAliasFable:
		return true
	default:
		return false
	}
}
