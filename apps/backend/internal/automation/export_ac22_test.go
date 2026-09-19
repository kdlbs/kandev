package automation

import (
	"reflect"
	"testing"
)

// fieldDisposition records what AC-22 calls a field's "disposition": either exported
// under a named YAML key, or excluded for a stated reason. See
// docs/specs/office/requirements/automations-yaml-export.md § Round-trip completeness, the field
// disposition table under "Data model".
//
// This is the anti-`reports_to` guard: Office's config export declared and emitted an
// agent field that was silently dropped on import because no test asserted every field
// had a disposition. A field added to Automation or AutomationTrigger without an entry
// here fails TestAC22_* immediately, forcing an explicit export/exclude decision.
type fieldDisposition struct {
	exported bool
	yamlKey  string // required when exported; documents the emitted key/path
}

var automationFieldDispositions = map[string]fieldDisposition{
	"Name":               {exported: true, yamlKey: "name"},
	"Description":        {exported: true, yamlKey: "description"},
	"Prompt":             {exported: true, yamlKey: "prompt"},
	"TaskTitleTemplate":  {exported: true, yamlKey: "task_title_template"},
	"Enabled":            {exported: true, yamlKey: "enabled"},
	"MaxConcurrentRuns":  {exported: true, yamlKey: "max_concurrent_runs"},
	"ContinuationPolicy": {exported: true, yamlKey: "continuation_policy"},
	"TaskMode":           {exported: true, yamlKey: "task_mode"},
	"RepositoryMode":     {exported: true, yamlKey: "repository_mode"},
	"Triggers":           {exported: true, yamlKey: "triggers"},
	"WorkflowID":         {exported: true, yamlKey: "workflow.name"},
	"WorkflowStepID":     {exported: true, yamlKey: "workflow.step"},
	"AgentProfileID":     {exported: true, yamlKey: "agent_profile"},
	"ExecutorProfileID":  {exported: true, yamlKey: "executor_profile"},
	"RepositoryIDs":      {exported: true, yamlKey: "repositories"},
	"Repositories":       {exported: false},
	"ContinuationTaskID": {exported: false}, // runtime pointer to the reused task

	"WebhookSecret":   {exported: false}, // secret
	"ID":              {exported: false}, // instance identity
	"WorkspaceID":     {exported: false}, // instance identity
	"LastTriggeredAt": {exported: false}, // runtime state / fire anchor
	"CreatedAt":       {exported: false}, // runtime state / fire anchor
	"UpdatedAt":       {exported: false}, // runtime state
	"LegacyBoardCard": {exported: false}, // derived from withdrawn execution_mode column
}

var automationTriggerFieldDispositions = map[string]fieldDisposition{
	"Type":    {exported: true, yamlKey: "type"},
	"Enabled": {exported: true, yamlKey: "enabled"},
	"Config":  {exported: true, yamlKey: "config"}, // decoded form

	"ConfigJSON":      {exported: false}, // raw storage of Config; same value
	"ID":              {exported: false}, // instance identity
	"AutomationID":    {exported: false}, // instance identity
	"LastEvaluatedAt": {exported: false}, // runtime state / fire anchor
	"CreatedAt":       {exported: false}, // runtime state / fire anchor
	"UpdatedAt":       {exported: false}, // runtime state
}

func TestAC22_AutomationFieldDisposition(t *testing.T) {
	assertFieldDispositionsCoverStruct(t, reflect.TypeOf(Automation{}), automationFieldDispositions)
}

func TestAC22_AutomationTriggerFieldDisposition(t *testing.T) {
	assertFieldDispositionsCoverStruct(t, reflect.TypeOf(AutomationTrigger{}), automationTriggerFieldDispositions)
}

// assertFieldDispositionsCoverStruct fails when a struct field has no recorded
// disposition (drift: a new field shipped without an export decision) or when a
// recorded disposition names a field that no longer exists (drift: a stale entry
// survived a rename/removal).
func assertFieldDispositionsCoverStruct(t *testing.T, typ reflect.Type, dispositions map[string]fieldDisposition) {
	t.Helper()
	seen := make(map[string]bool, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		name := typ.Field(i).Name
		if _, ok := dispositions[name]; !ok {
			t.Errorf("field %s.%s has no recorded AC-22 disposition; add it to the export field-disposition table and either export it under a yamlKey or exclude it with a reason", typ.Name(), name)
			continue
		}
		seen[name] = true
	}
	for name := range dispositions {
		if !seen[name] {
			t.Errorf("recorded AC-22 disposition for %s.%s does not match any struct field; remove the stale entry", typ.Name(), name)
		}
	}
}
