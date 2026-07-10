package worktree

import "testing"

func TestRenderTaskBranchNameUsesTicketAliases(t *testing.T) {
	got, err := RenderTaskBranchName(BranchNameTemplateInput{
		Template: "{ticket}-{title}-{suffix}",
		TaskID:   "task-123",
		Title:    "Fix GitHub imports",
		Ticket:   "KAN-42",
		Suffix:   "abc",
	})
	if err != nil {
		t.Fatalf("RenderTaskBranchName: %v", err)
	}
	if got != "kan-42-fix-github-imports-abc" {
		t.Fatalf("branch name = %q, want %q", got, "kan-42-fix-github-imports-abc")
	}

	got, err = RenderTaskBranchName(BranchNameTemplateInput{
		Template: "{issue_key}-{task_id}",
		TaskID:   "task-123",
		Title:    "Fix GitHub imports",
		Ticket:   "KAN-42",
		Suffix:   "abc",
	})
	if err != nil {
		t.Fatalf("RenderTaskBranchName alias: %v", err)
	}
	if got != "kan-42-task-123" {
		t.Fatalf("branch name alias = %q, want %q", got, "kan-42-task-123")
	}
}

func TestRenderTaskBranchNameAllowsTemplateWithoutSuffix(t *testing.T) {
	got, err := RenderTaskBranchName(BranchNameTemplateInput{
		Template: "feature/{ticket}-{title}",
		TaskID:   "task-123",
		Title:    "Add branch templates",
		Ticket:   "KAN-42",
		Suffix:   "abc",
	})
	if err != nil {
		t.Fatalf("RenderTaskBranchName: %v", err)
	}
	if got != "feature/kan-42-add-branch-templates" {
		t.Fatalf("branch name = %q, want %q", got, "feature/kan-42-add-branch-templates")
	}
}

func TestRenderTaskBranchNameRejectsPrefixPlaceholder(t *testing.T) {
	_, err := RenderTaskBranchName(BranchNameTemplateInput{
		Template: "{prefix}{title}-{suffix}",
		TaskID:   "task-123",
		Title:    "Fix branch",
		Suffix:   "abc",
	})
	if err == nil {
		t.Fatal("expected invalid branch name error for unsupported prefix placeholder")
	}
}

func TestRenderTaskBranchNameRejectsInvalidRenderedName(t *testing.T) {
	tests := []struct {
		name     string
		template string
		title    string
	}{
		{name: "parent path", template: "../{title}", title: "Fix branch"},
		{name: "hidden component", template: "feature/.{title}", title: "Fix branch"},
		{name: "trailing dot", template: "feature/{title}.", title: "Fix branch"},
		{name: "lock component", template: "feature.lock/{title}", title: "Fix branch"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := RenderTaskBranchName(BranchNameTemplateInput{
				Template: tt.template,
				TaskID:   "task-123",
				Title:    tt.title,
				Suffix:   "abc",
			})
			if err == nil {
				t.Fatal("expected invalid branch name error")
			}
		})
	}
}

func TestTicketForBranchNameResolvesKnownMetadata(t *testing.T) {
	tests := []struct {
		name       string
		identifier string
		metadata   map[string]any
		want       string
	}{
		{name: "task identifier wins", identifier: "TASK-7", metadata: map[string]any{"jira_issue_key": "KAN-42"}, want: "TASK-7"},
		{name: "jira issue key", metadata: map[string]any{"jira_issue_key": "KAN-42"}, want: "KAN-42"},
		{name: "linear identifier", metadata: map[string]any{"linear_issue_identifier": "LIN-9"}, want: "LIN-9"},
		{name: "github issue", metadata: map[string]any{"issue_repo": "kdlbs/kandev", "issue_number": float64(1610)}, want: "kdlbs-kandev-1610"},
		{name: "github pr", metadata: map[string]any{"pr_repo": "kdlbs/kandev", "pr_number": 37}, want: "kdlbs-kandev-37"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TicketForBranchName(tt.identifier, tt.metadata)
			if got != tt.want {
				t.Fatalf("TicketForBranchName() = %q, want %q", got, tt.want)
			}
		})
	}
}
