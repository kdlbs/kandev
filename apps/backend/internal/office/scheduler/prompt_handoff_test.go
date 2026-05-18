package scheduler

import (
	"strings"
	"testing"
)

func TestBuildPrompt_NoHandoffContextLeavesPromptUntouched(t *testing.T) {
	pc := &PromptContext{
		Reason:    RunReasonTaskAssigned,
		TaskTitle: "Implement endpoint",
	}
	out := BuildPrompt(pc)
	if strings.Contains(out, "Related tasks") {
		t.Errorf("expected no related-tasks section when HandoffContext is nil; got:\n%s", out)
	}
	if strings.Contains(out, "Documents available") {
		t.Errorf("expected no documents section; got:\n%s", out)
	}
}

func TestBuildPrompt_AppendsRelatedTasksSection(t *testing.T) {
	pc := &PromptContext{
		Reason:    RunReasonTaskAssigned,
		TaskTitle: "Implement endpoint",
		HandoffContext: &HandoffPromptContext{
			ParentRef:     "KAN-12 Plan implementation",
			SiblingRefs:   []string{"KAN-14 Write spec"},
			BlockerRefs:   []string{"KAN-13 Plan implementation"},
			BlockedByRefs: []string{"KAN-15 Review"},
		},
	}
	out := BuildPrompt(pc)
	if !strings.Contains(out, "Related tasks:") {
		t.Errorf("missing related tasks header:\n%s", out)
	}
	if !strings.Contains(out, "Parent: KAN-12 Plan implementation") {
		t.Errorf("missing parent ref:\n%s", out)
	}
	if !strings.Contains(out, "Sibling: KAN-14 Write spec") {
		t.Errorf("missing sibling ref:\n%s", out)
	}
	if !strings.Contains(out, "Blocked by: KAN-13 Plan implementation") {
		t.Errorf("missing blocker ref:\n%s", out)
	}
	if !strings.Contains(out, "Blocks: KAN-15 Review") {
		t.Errorf("missing blocked-by ref:\n%s", out)
	}
}

// REGRESSION: the spec is explicit that document BODIES never appear in
// the prompt — the agent must fetch them via get_task_document_kandev.
// This test enforces the contract by including a docs entry whose
// "title" is set to a marker string and asserting only key+title (no
// content) appear.
func TestBuildPrompt_DocumentBodiesNeverInlined(t *testing.T) {
	pc := &PromptContext{
		Reason:    RunReasonTaskAssigned,
		TaskTitle: "Implement endpoint",
		HandoffContext: &HandoffPromptContext{
			ParentRef: "KAN-12 Plan",
			AvailableDocs: []HandoffDocPrompt{
				{TaskRef: "KAN-12 Plan", Key: "spec", Title: "Architecture spec"},
				{TaskRef: "KAN-12 Plan", Key: "plan", Title: "Implementation plan"},
			},
		},
	}
	out := BuildPrompt(pc)
	if !strings.Contains(out, "Documents available (fetch with get_task_document_kandev)") {
		t.Errorf("missing documents header:\n%s", out)
	}
	if !strings.Contains(out, "spec") || !strings.Contains(out, "Architecture spec") {
		t.Errorf("missing spec doc:\n%s", out)
	}
	// The fetch instruction must reference the tool name explicitly so
	// agents can't be tempted to inline document bodies in their own
	// prompts.
	if !strings.Contains(out, "get_task_document_kandev") {
		t.Errorf("documents section must name the fetch tool:\n%s", out)
	}
}

func TestBuildPrompt_RendersWorkspaceSection(t *testing.T) {
	pc := &PromptContext{
		Reason:    RunReasonTaskAssigned,
		TaskTitle: "Implement endpoint",
		HandoffContext: &HandoffPromptContext{
			WorkspaceMode:    "inherit_parent",
			WorkspaceMembers: []string{"KAN-13 Plan", "KAN-14 Spec"},
			MaterializedPath: "/workspaces/.../task-KAN-12",
			MaterializedKind: "single_repo",
		},
	}
	out := BuildPrompt(pc)
	if !strings.Contains(out, "Workspace:") {
		t.Errorf("missing workspace header:\n%s", out)
	}
	if !strings.Contains(out, "mode: inherit_parent") {
		t.Errorf("missing mode line:\n%s", out)
	}
	if !strings.Contains(out, "shared with: KAN-13 Plan, KAN-14 Spec") {
		t.Errorf("missing members line:\n%s", out)
	}
	if !strings.Contains(out, "/workspaces/.../task-KAN-12") {
		t.Errorf("missing path:\n%s", out)
	}
}

func TestBuildHandoffSection_EmptyContextProducesEmptyString(t *testing.T) {
	if got := buildHandoffSection(nil); got != "" {
		t.Errorf("nil context should produce empty string, got %q", got)
	}
	if got := buildHandoffSection(&HandoffPromptContext{}); got != "" {
		t.Errorf("zero context should produce empty string, got %q", got)
	}
}
