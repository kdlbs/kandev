package utility

import (
	"github.com/kandev/kandev/internal/agent/settings/cliflags"
	"github.com/kandev/kandev/internal/agent/settings/models"
)

// ProfileInferenceEnvironment is shared by actual dispatch and usage preview.
// Secret references are not ambient fallbacks; this preserves the existing
// one-shot launch contract (only literal profile vars are forwarded).
func ProfileInferenceEnvironment(flags []models.CLIFlag, prefix string, variables []models.ProfileEnvVar) ([]string, []string, map[string]string, error) {
	cli, err := cliflags.Resolve(flags)
	if err != nil {
		return nil, nil, nil, err
	}
	var tokens []string
	if prefix != "" {
		if err := cliflags.ValidateCommandPrefix(prefix); err != nil {
			return nil, nil, nil, err
		}
		tokens, err = cliflags.Tokenise(prefix)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	env := make(map[string]string, len(variables))
	for _, value := range variables {
		if value.Key != "" && value.SecretID == "" {
			env[value.Key] = value.Value
		}
	}
	return cli, tokens, env, nil
}
