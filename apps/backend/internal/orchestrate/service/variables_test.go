package service

import (
	"testing"
	"time"
)

func TestBuiltinVars(t *testing.T) {
	now := time.Date(2026, 4, 25, 14, 30, 0, 0, time.UTC)
	vars := builtinVars(now)
	if vars["date"] != "2026-04-25" {
		t.Errorf("date = %q, want 2026-04-25", vars["date"])
	}
	if vars["datetime"] != "2026-04-25T14:30:00Z" {
		t.Errorf("datetime = %q, want 2026-04-25T14:30:00Z", vars["datetime"])
	}
}

func TestResolveVariables_PriorityOrder(t *testing.T) {
	now := time.Date(2026, 4, 25, 14, 30, 0, 0, time.UTC)
	defaults := map[string]string{"env": "staging", "date": "custom-date"}
	provided := map[string]string{"env": "production", "extra": "val"}

	vars := resolveVariables(now, defaults, provided)
	// Spec: builtins -> defaults -> provided (later wins).
	// defaults["date"] overrides the builtin date.
	if vars["date"] != "custom-date" {
		t.Errorf("defaults should override builtins, got %q", vars["date"])
	}
	if vars["env"] != "production" {
		t.Errorf("provided should override defaults, got %q", vars["env"])
	}
	if vars["extra"] != "val" {
		t.Errorf("extra = %q, want val", vars["extra"])
	}
	// datetime builtin still present (not overridden).
	if vars["datetime"] != "2026-04-25T14:30:00Z" {
		t.Errorf("datetime = %q", vars["datetime"])
	}
}

func TestInterpolateTemplate_Basic(t *testing.T) {
	vars := map[string]string{"date": "2026-04-25", "name": "Security"}
	result := interpolateTemplate("{{name}} Scan - {{date}}", vars)
	if result != "Security Scan - 2026-04-25" {
		t.Errorf("got %q", result)
	}
}

func TestInterpolateTemplate_UnknownVarsLeftAsIs(t *testing.T) {
	vars := map[string]string{"date": "2026-04-25"}
	result := interpolateTemplate("{{date}} - {{unknown}}", vars)
	if result != "2026-04-25 - {{unknown}}" {
		t.Errorf("unknown vars should be left as-is, got %q", result)
	}
}

func TestInterpolateTemplate_Empty(t *testing.T) {
	result := interpolateTemplate("no vars here", nil)
	if result != "no vars here" {
		t.Errorf("got %q", result)
	}
}
