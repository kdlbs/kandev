package automation

import (
	"context"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// AC-33: an automation whose stored name is the empty string emits
// `name: ""` (not omitted — Name carries no yaml `omitempty` tag and is not
// in AC-4's omit-when-empty list) and its zip entry falls back to
// `automation.yml` via AC-26's empty-slug fallback. Written directly through
// svc.store.CreateAutomation, bypassing Service.CreateAutomation's
// empty-name rejection, exactly as the spec's own AC-33 note describes: "a
// row written by any other path is representable".
func TestExportAutomationsDocument_AC33_EmptyNameEmitsEmptyStringNameKey(t *testing.T) {
	svc, wsLookup := exportServiceTestFixture(t)
	wsLookup.exists["ws-1"] = true
	if err := svc.store.CreateAutomation(context.Background(), &Automation{
		ID:                "auto-empty-name",
		WorkspaceID:       "ws-1",
		Name:              "",
		MaxConcurrentRuns: 1,
	}); err != nil {
		t.Fatalf("CreateAutomation: %v", err)
	}

	body, err := svc.ExportAutomationsDocument(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("ExportAutomationsDocument: %v", err)
	}
	var doc exportDocument
	if err := yaml.Unmarshal(body, &doc); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if len(doc.Automations) != 1 {
		t.Fatalf("got %d automations, want 1", len(doc.Automations))
	}
	if doc.Automations[0].Name != "" {
		t.Errorf("Name = %q, want empty string", doc.Automations[0].Name)
	}
	if !strings.Contains(string(body), "name: \"\"") {
		t.Errorf("expected literal `name: \"\"` in marshalled output, got:\n%s", body)
	}
}

func TestExportAutomationsZip_AC33_EmptyNameFallsBackToAutomationSlug(t *testing.T) {
	svc, wsLookup := exportServiceTestFixture(t)
	wsLookup.exists["ws-1"] = true
	if err := svc.store.CreateAutomation(context.Background(), &Automation{
		ID:                "auto-empty-name",
		WorkspaceID:       "ws-1",
		Name:              "",
		MaxConcurrentRuns: 1,
	}); err != nil {
		t.Fatalf("CreateAutomation: %v", err)
	}

	data, err := svc.ExportAutomationsZip(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("ExportAutomationsZip: %v", err)
	}
	names := zipEntryNames(t, data)
	want := []string{".kandev/automations/automation.yml"}
	if len(names) != len(want) || names[0] != want[0] {
		t.Errorf("entries = %v, want %v", names, want)
	}
}

// AC-34: an automation whose stored max_concurrent_runs is zero or negative
// is exported unchanged, not substituted with the service's create-time
// default of 1. Written directly through svc.store.CreateAutomation, since
// Service.CreateAutomation itself defaults a non-positive request value to 1
// (service.go's own AC-34 rationale: "export reports what is in the
// database; it is not a validation pass").
func TestExportAutomationsDocument_AC34_NonPositiveMaxConcurrentRunsPassesThroughUnchanged(t *testing.T) {
	cases := []struct {
		name string
		want int
	}{
		{name: "zero", want: 0},
		{name: "negative", want: -5},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svc, wsLookup := exportServiceTestFixture(t)
			wsLookup.exists["ws-1"] = true
			if err := svc.store.CreateAutomation(context.Background(), &Automation{
				ID:                "auto-" + c.name,
				WorkspaceID:       "ws-1",
				Name:              "Automation " + c.name,
				MaxConcurrentRuns: c.want,
			}); err != nil {
				t.Fatalf("CreateAutomation: %v", err)
			}

			body, err := svc.ExportAutomationsDocument(context.Background(), "ws-1")
			if err != nil {
				t.Fatalf("ExportAutomationsDocument: %v", err)
			}
			var doc exportDocument
			if err := yaml.Unmarshal(body, &doc); err != nil {
				t.Fatalf("yaml.Unmarshal: %v", err)
			}
			if len(doc.Automations) != 1 {
				t.Fatalf("got %d automations, want 1", len(doc.Automations))
			}
			if doc.Automations[0].MaxConcurrentRuns != c.want {
				t.Errorf("MaxConcurrentRuns = %d, want %d (unchanged from stored value, not substituted with the service default of 1)",
					doc.Automations[0].MaxConcurrentRuns, c.want)
			}
		})
	}
}
