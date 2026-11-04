package environment

import (
	"context"
	"strings"

	"github.com/kandev/kandev/internal/task/models"
)

// ResolveProfile applies profile declarations before managed agent defaults.
// Secret values use the caller's scoped resolver and are never silently omitted.
func ResolveProfile(ctx context.Context, values []models.ProfileEnvVar, defaults map[string]string, reveal RevealFunc) (map[string]string, error) {
	definitions := make([]Definition, 0, len(values)+len(defaults))
	declared := make(map[string]bool, len(values))
	for _, value := range values {
		if strings.TrimSpace(value.Key) == "" || (value.Value == "" && value.SecretID == "") {
			continue
		}
		definitions = append(definitions, Definition{Key: value.Key, Literal: value.Value, SecretID: value.SecretID, Origin: OriginAgentProfile})
		declared[value.Key] = true
	}
	for key, value := range defaults {
		if !declared[key] {
			definitions = append(definitions, Definition{Key: key, Literal: value, Origin: OriginManagedAgentDefaults})
		}
	}
	resolved, _, err := Resolve(ctx, definitions, reveal)
	return resolved, err
}
