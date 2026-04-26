package service

import (
	"regexp"
	"strings"
	"time"
)

var templateVarRe = regexp.MustCompile(`\{\{(\w+)\}\}`)

// builtinVars returns the built-in variable values for the given time.
func builtinVars(now time.Time) map[string]string {
	return map[string]string{
		"date":     now.Format("2006-01-02"),
		"datetime": now.Format(time.RFC3339),
	}
}

// resolveVariables merges builtins, declared defaults, and provided values.
// Later sources override earlier ones.
func resolveVariables(
	now time.Time,
	declaredDefaults map[string]string,
	provided map[string]string,
) map[string]string {
	merged := builtinVars(now)
	for k, v := range declaredDefaults {
		merged[k] = v
	}
	for k, v := range provided {
		merged[k] = v
	}
	return merged
}

// interpolateTemplate replaces {{name}} placeholders with resolved values.
// Unknown variables are left as-is.
func interpolateTemplate(tmpl string, vars map[string]string) string {
	return templateVarRe.ReplaceAllStringFunc(tmpl, func(match string) string {
		name := strings.TrimPrefix(strings.TrimSuffix(match, "}}"), "{{")
		if val, ok := vars[name]; ok {
			return val
		}
		return match
	})
}
