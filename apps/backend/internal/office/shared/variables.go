package shared

import (
	"regexp"
	"strings"
	"time"
)

var templateVarRe = regexp.MustCompile(`\{\{(\w+)\}\}`)

// BuiltinVars returns the built-in template variable values for the given time.
func BuiltinVars(now time.Time) map[string]string {
	return map[string]string{
		"date":     now.Format("2006-01-02"),
		"datetime": now.Format(time.RFC3339),
	}
}

// ResolveVariables merges builtins, declared defaults, and provided values.
// Later sources override earlier ones: builtins < defaults < provided.
func ResolveVariables(
	now time.Time,
	declaredDefaults map[string]string,
	provided map[string]string,
) map[string]string {
	merged := BuiltinVars(now)
	for k, v := range declaredDefaults {
		merged[k] = v
	}
	for k, v := range provided {
		merged[k] = v
	}
	return merged
}

// InterpolateTemplate replaces {{name}} placeholders with resolved values.
// Unknown variables are left as-is.
func InterpolateTemplate(tmpl string, vars map[string]string) string {
	return templateVarRe.ReplaceAllStringFunc(tmpl, func(match string) string {
		name := strings.TrimPrefix(strings.TrimSuffix(match, "}}"), "{{")
		if val, ok := vars[name]; ok {
			return val
		}
		return match
	})
}
