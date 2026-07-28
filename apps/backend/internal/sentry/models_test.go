package sentry

import (
	"encoding/json"
	"testing"
)

// TestSearchFilter_MarshalJSON_EmitsPluralProjectSlugs pins the current wire
// shape: a filter with multiple projects serializes to a `projectSlugs` array,
// never the legacy singular `projectSlug` key.
func TestSearchFilter_MarshalJSON_EmitsPluralProjectSlugs(t *testing.T) {
	f := SearchFilter{OrgSlug: "acme", ProjectSlugs: []string{"frontend", "backend"}}
	raw, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode raw: %v", err)
	}
	if _, ok := decoded["projectSlug"]; ok {
		t.Errorf("expected no legacy projectSlug key in output, got %s", raw)
	}
	slugs, ok := decoded["projectSlugs"].([]interface{})
	if !ok || len(slugs) != 2 || slugs[0] != "frontend" || slugs[1] != "backend" {
		t.Errorf("expected projectSlugs=[frontend backend], got %s", raw)
	}
}

// TestSearchFilter_UnmarshalJSON_LegacySingularProjectSlug pins backward
// compatibility: a watch persisted before multi-project support stored
// `projectSlug` (singular string) in filter_json. Existing rows must keep
// their configured project after this upgrade instead of silently losing it.
func TestSearchFilter_UnmarshalJSON_LegacySingularProjectSlug(t *testing.T) {
	var f SearchFilter
	raw := `{"orgSlug":"acme","projectSlug":"frontend","environment":"prod"}`
	if err := json.Unmarshal([]byte(raw), &f); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if f.OrgSlug != "acme" || f.Environment != "prod" {
		t.Errorf("unrelated fields lost: %+v", f)
	}
	if len(f.ProjectSlugs) != 1 || f.ProjectSlugs[0] != "frontend" {
		t.Errorf("expected legacy projectSlug promoted to ProjectSlugs=[frontend], got %+v", f.ProjectSlugs)
	}
}

// TestSearchFilter_UnmarshalJSON_PluralTakesPrecedence pins that a payload
// carrying both keys (should never happen in practice, but decoding must be
// deterministic) prefers the current plural field.
func TestSearchFilter_UnmarshalJSON_PluralTakesPrecedence(t *testing.T) {
	var f SearchFilter
	raw := `{"orgSlug":"acme","projectSlug":"legacy","projectSlugs":["new-a","new-b"]}`
	if err := json.Unmarshal([]byte(raw), &f); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(f.ProjectSlugs) != 2 || f.ProjectSlugs[0] != "new-a" || f.ProjectSlugs[1] != "new-b" {
		t.Errorf("expected plural projectSlugs to win, got %+v", f.ProjectSlugs)
	}
}

// TestSearchFilter_UnmarshalJSON_NoProjectFields pins that a filter with
// neither key decodes to an empty (nil) ProjectSlugs rather than erroring —
// validation of "at least one project" happens in the service layer, not here.
func TestSearchFilter_UnmarshalJSON_NoProjectFields(t *testing.T) {
	var f SearchFilter
	if err := json.Unmarshal([]byte(`{"orgSlug":"acme"}`), &f); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(f.ProjectSlugs) != 0 {
		t.Errorf("expected no project slugs, got %+v", f.ProjectSlugs)
	}
}
