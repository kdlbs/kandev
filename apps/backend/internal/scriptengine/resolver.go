// Package scriptengine provides executor-agnostic script resolution with dynamic placeholders.
// It is designed to be reusable by any executor type (Sprites, Docker, remote SSH, etc.).
package scriptengine

import (
	"maps"
	"strings"
)

// PlaceholderProvider supplies dynamic placeholder values at resolution time.
type PlaceholderProvider func() map[string]string

// Resolver resolves {{key}} placeholders in scripts.
type Resolver struct {
	staticVars map[string]string
	providers  []PlaceholderProvider
}

// NewResolver creates a new placeholder resolver.
func NewResolver() *Resolver {
	return &Resolver{
		staticVars: make(map[string]string),
	}
}

// WithStatic registers static key-value placeholders.
func (r *Resolver) WithStatic(vars map[string]string) *Resolver {
	maps.Copy(r.staticVars, vars)
	return r
}

// WithProvider registers a dynamic placeholder provider.
func (r *Resolver) WithProvider(p PlaceholderProvider) *Resolver {
	r.providers = append(r.providers, p)
	return r
}

// Resolve replaces all {{key}} placeholders in the script.
// Unknown placeholders are left as-is so the user can see what wasn't resolved.
func (r *Resolver) Resolve(script string) string {
	if script == "" {
		return script
	}
	vars := r.mergedVars()
	if len(vars) == 0 {
		return script
	}
	result := script
	for key, value := range vars {
		result = strings.ReplaceAll(result, "{{"+key+"}}", value)
	}
	return result
}

// mergedVars collects all static vars and dynamic provider vars into a single map.
// Provider values override static values if keys collide.
func (r *Resolver) mergedVars() map[string]string {
	merged := make(map[string]string, len(r.staticVars))
	maps.Copy(merged, r.staticVars)
	for _, p := range r.providers {
		maps.Copy(merged, p())
	}
	return merged
}

// ResolveScript is a convenience function that replaces {{key}} placeholders with values from vars.
// Unknown placeholders are left as-is.
func ResolveScript(script string, vars map[string]string) string {
	if script == "" || len(vars) == 0 {
		return script
	}
	result := script
	for key, value := range vars {
		result = strings.ReplaceAll(result, "{{"+key+"}}", value)
	}
	return result
}
