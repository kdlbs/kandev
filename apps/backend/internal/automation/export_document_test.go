package automation

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func fullyPopulatedExportAutomation(t *testing.T) exportAutomation {
	t.Helper()
	configNode, err := jsonToYAMLNode(map[string]any{"cron_expression": "0 9 * * *"})
	if err != nil {
		t.Fatalf("jsonToYAMLNode: %v", err)
	}
	promptNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "Do the thing"}
	return exportAutomation{
		Name:               "Daily Review",
		Description:        "Runs every morning",
		Enabled:            true,
		MaxConcurrentRuns:  1,
		ContinuationPolicy: ContinuationPolicyReuseThread,
		TaskMode:           TaskModeNormalTask,
		RepositoryMode:     RepositoryModeSelected,
		TaskTitleTemplate:  "Daily Review ({{trigger.timestamp}})",
		Prompt:             promptNode,
		AgentProfile:       &exportAgentProfile{AgentName: "Claude Code", Model: "opus[1m]", Mode: "auto"},
		ExecutorProfile:    &exportExecutorProfile{Executor: "exec-worktree", Name: "Worktree"},
		Workflow:           &exportWorkflow{Name: "Kanban", Step: "In Progress"},
		Repositories:       []string{"kegmil-offline-first"},
		Triggers: []exportTrigger{
			{Type: "scheduled", Enabled: true, Config: configNode},
		},
	}
}

// AC-40: top-level keys in fixed order version, type, automations, warnings (warnings
// omitted here since empty); automation keys in fixed order name, description,
// enabled, max_concurrent_runs, continuation_policy, task_mode, repository_mode,
// task_title_template, prompt, agent_profile,
// executor_profile, workflow, repositories, triggers; trigger keys in fixed order
// type, enabled, config.
func TestMarshalExportDocument_AC40KeyOrder(t *testing.T) {
	doc := newExportDocument([]exportAutomation{fullyPopulatedExportAutomation(t)}, nil)
	out, err := marshalExportDocument(doc)
	if err != nil {
		t.Fatalf("marshalExportDocument: %v", err)
	}

	wantOrder := []string{
		"version:",
		"type:",
		"automations:",
		"name:",
		"description:",
		"enabled:",
		"max_concurrent_runs:",
		"continuation_policy:",
		"task_mode:",
		"repository_mode:",
		"task_title_template:",
		"prompt:",
		"agent_profile:",
		"executor_profile:",
		"workflow:",
		"repositories:",
		"triggers:",
		"type:",
		"enabled:",
		"config:",
	}
	assertKeysAppearInOrder(t, string(out), wantOrder)

	if strings.Contains(string(out), "warnings:") {
		t.Errorf("expected no warnings key when Warnings is nil, got:\n%s", out)
	}
}

// AC-40: warnings key present, in the fixed top-level position after automations,
// when non-empty.
func TestMarshalExportDocument_WarningsKeyPresentWhenNonEmpty(t *testing.T) {
	doc := newExportDocument([]exportAutomation{fullyPopulatedExportAutomation(t)}, []string{"unresolved workflow"})
	out, err := marshalExportDocument(doc)
	if err != nil {
		t.Fatalf("marshalExportDocument: %v", err)
	}
	assertKeysAppearInOrder(t, string(out), []string{"automations:", "warnings:"})
}

// AC-4: keys omitted when empty are skipped without disturbing the order of the rest.
func TestMarshalExportDocument_OmitsEmptyOptionalFields(t *testing.T) {
	doc := newExportDocument([]exportAutomation{{
		Name:               "Minimal",
		Enabled:            false,
		MaxConcurrentRuns:  1,
		ContinuationPolicy: ContinuationPolicyNewTask,
		Triggers:           []exportTrigger{},
	}}, nil)
	out, err := marshalExportDocument(doc)
	if err != nil {
		t.Fatalf("marshalExportDocument: %v", err)
	}
	s := string(out)
	for _, absent := range []string{"description:", "task_title_template:", "prompt:", "agent_profile:", "executor_profile:", "workflow:", "repositories:"} {
		if strings.Contains(s, absent) {
			t.Errorf("expected %q omitted for empty field, got:\n%s", absent, s)
		}
	}
	assertKeysAppearInOrder(t, s, []string{"name:", "enabled:", "max_concurrent_runs:", "continuation_policy:", "task_mode:", "repository_mode:", "triggers:"})
}

// AC-12: indentation is 2 spaces per nesting level, not yaml.v3's package-level
// Marshal default of 4. Nested levels legitimately accumulate (a mapping key one
// level below a sequence item sits at 4 spaces, one level below that at 6, and so
// on), so this asserts the per-level increment directly rather than banning any
// particular total indent width.
func TestMarshalExportDocument_TwoSpaceIndent(t *testing.T) {
	doc := newExportDocument([]exportAutomation{fullyPopulatedExportAutomation(t)}, nil)
	out, err := marshalExportDocument(doc)
	if err != nil {
		t.Fatalf("marshalExportDocument: %v", err)
	}
	s := string(out)

	// automations: (0 indent) -> "  - name:" (2-space sequence indent).
	if !strings.Contains(s, "automations:\n  - name:") {
		t.Errorf("expected 2-space indent between automations: and its first item, got:\n%s", s)
	}
	// A mapping one level below the sequence item's own keys: agent_profile:
	// (4-space, aligned with the other keys inside "- name: ...") -> its own
	// agent_name key one level deeper still, at 6 spaces (4 + 2).
	if !strings.Contains(s, "\n    agent_profile:\n      agent_name:") {
		t.Errorf("expected 2-space increment from agent_profile: to agent_name:, got:\n%s", s)
	}

	// A default (unpinned) yaml.v3 Marshal call would use 4-space indent
	// end-to-end; confirm that shape is NOT what we produced, so this test would
	// fail if SetIndent(2) were ever dropped.
	defaultOut, err := yaml.Marshal(doc)
	if err != nil {
		t.Fatalf("yaml.Marshal (default indent): %v", err)
	}
	if s == string(defaultOut) {
		t.Errorf("pinned 2-space output is byte-identical to the default-indent Marshal output; SetIndent(2) had no effect")
	}
}

// AC-40's own commentary: order must not rest on struct field declaration order being
// an implementation detail — pin it with a version/type/automations/warnings
// constructor test independent of newExportDocument's shape.
func TestNewExportDocument_FixedVersionAndType(t *testing.T) {
	doc := newExportDocument(nil, nil)
	if doc.Version != 1 {
		t.Errorf("Version = %d, want 1", doc.Version)
	}
	if doc.Type != "kandev_automations" {
		t.Errorf("Type = %q, want kandev_automations", doc.Type)
	}
}

// assertKeysAppearInOrder checks that each key substring appears in out, in the given
// relative order (not necessarily contiguous), by scanning forward through out.
func assertKeysAppearInOrder(t *testing.T, out string, keysInOrder []string) {
	t.Helper()
	cursor := 0
	for _, key := range keysInOrder {
		idx := strings.Index(out[cursor:], key)
		if idx == -1 {
			t.Fatalf("expected key %q after position %d, not found in:\n%s", key, cursor, out)
		}
		cursor += idx + len(key)
	}
}
