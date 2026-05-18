package service_test

import (
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestBuildPrompt_AppendsHandoffSection_OnActiveBuilder is the
// regression guard for the missed-wiring bug from the post-implementation
// review. The Office run prompt path (office/service.BuildPrompt) MUST
// now include the Related-tasks / Documents-available / Workspace
// section when PromptContext.HandoffContext is populated. Without this
// hook, agents can't see parent/sibling document keys at run time even
// though the data exists server-side.
func TestBuildPrompt_AppendsHandoffSection_OnActiveBuilder(t *testing.T) {
	parent := v1.TaskRef{
		ID: "p", Identifier: "KAN-1", Title: "Plan",
		State: "completed", WorkspaceID: "ws-1",
	}
	pc := &service.PromptContext{
		Reason:    service.RunReasonTaskAssigned,
		TaskTitle: "Implement endpoint",
		HandoffContext: &v1.TaskContext{
			Task: v1.TaskRef{
				ID: "t", Identifier: "KAN-2", Title: "Implement",
				State: "in_progress", WorkspaceID: "ws-1",
				ParentID: "p",
			},
			Parent: &parent,
			AvailableDocs: []v1.DocumentRef{
				{TaskRef: parent, Key: "spec", Title: "Architecture spec", UpdatedAt: time.Now()},
				{TaskRef: parent, Key: "plan", Title: "Implementation plan", UpdatedAt: time.Now()},
			},
		},
	}
	out := service.BuildPrompt(pc)

	for _, needle := range []string{
		"Related tasks:",
		"Parent: KAN-1 Plan",
		"Documents available (fetch with get_task_document_kandev):",
		"KAN-1 Plan spec",
		"Architecture spec",
		"KAN-1 Plan plan",
	} {
		if !strings.Contains(out, needle) {
			t.Errorf("active BuildPrompt should contain %q\nfull prompt:\n%s", needle, out)
		}
	}
}

func TestBuildPrompt_NilHandoffContextIsNoOp(t *testing.T) {
	pc := &service.PromptContext{
		Reason:    service.RunReasonTaskAssigned,
		TaskTitle: "Implement",
	}
	out := service.BuildPrompt(pc)
	if strings.Contains(out, "Related tasks:") || strings.Contains(out, "Documents available") {
		t.Errorf("nil HandoffContext should not add any section; got:\n%s", out)
	}
}

func TestBuildPrompt_RendersWorkspaceSection(t *testing.T) {
	pc := &service.PromptContext{
		Reason:    service.RunReasonTaskAssigned,
		TaskTitle: "Implement",
		HandoffContext: &v1.TaskContext{
			Task:          v1.TaskRef{ID: "t", Title: "Implement", WorkspaceID: "ws-1"},
			WorkspaceMode: "inherit_parent",
			WorkspaceGroup: &v1.WorkspaceGroupRef{
				ID:               "g1",
				MaterializedPath: "/wt/abc",
				MaterializedKind: "single_repo",
				OwnedByKandev:    true,
				Members: []v1.TaskRef{
					{ID: "p", Identifier: "KAN-1", Title: "Plan"},
					{ID: "t", Identifier: "KAN-2", Title: "Implement"},
				},
			},
		},
	}
	out := service.BuildPrompt(pc)
	for _, needle := range []string{
		"Workspace:",
		"mode: inherit_parent",
		"shared with: KAN-1 Plan, KAN-2 Implement",
		"path: /wt/abc",
		"kind: single_repo",
	} {
		if !strings.Contains(out, needle) {
			t.Errorf("missing %q\nfull prompt:\n%s", needle, out)
		}
	}
}

// REGRESSION: document BODIES never appear in the prompt. The agent
// must call get_task_document_kandev to fetch content. Verify the
// content field of a document is never rendered.
func TestBuildPrompt_DocumentBodiesNeverInlined(t *testing.T) {
	pc := &service.PromptContext{
		Reason:    service.RunReasonTaskAssigned,
		TaskTitle: "Implement",
		HandoffContext: &v1.TaskContext{
			Task: v1.TaskRef{ID: "t", Title: "Implement"},
			AvailableDocs: []v1.DocumentRef{
				{
					TaskRef: v1.TaskRef{ID: "p", Identifier: "KAN-1", Title: "Plan"},
					Key:     "spec",
					Title:   "Architecture spec",
				},
			},
		},
	}
	out := service.BuildPrompt(pc)
	if !strings.Contains(out, "get_task_document_kandev") {
		t.Errorf("documents section MUST name the fetch tool so agents don't inline bodies\nprompt:\n%s", out)
	}
}
