package scriptengine

import "testing"

func TestTicketContextProvider(t *testing.T) {
	t.Run("jira metadata resolves ticket placeholders", func(t *testing.T) {
		provider := TicketContextProvider("Refactor billing", map[string]interface{}{
			"jira_issue_key": "PROJ-42",
			"jira_issue_url": "https://x.atlassian.net/browse/PROJ-42",
		})
		vars := provider()
		if got := vars["TICKET_ID"]; got != "PROJ-42" {
			t.Errorf("TICKET_ID = %q, want PROJ-42", got)
		}
		if got := vars["TICKET_URL"]; got != "https://x.atlassian.net/browse/PROJ-42" {
			t.Errorf("TICKET_URL = %q, want jira URL", got)
		}
		if got := vars["TICKET_PROVIDER"]; got != "jira" {
			t.Errorf("TICKET_PROVIDER = %q, want jira", got)
		}
		if got := vars["TASK_TITLE"]; got != "Refactor billing" {
			t.Errorf("TASK_TITLE = %q, want Refactor billing", got)
		}
	})

	t.Run("linear metadata resolves ticket placeholders", func(t *testing.T) {
		provider := TicketContextProvider("Ship X", map[string]interface{}{
			"linear_issue_identifier": "ENG-7",
			"linear_issue_url":        "https://linear.app/team/issue/ENG-7",
		})
		vars := provider()
		if got := vars["TICKET_ID"]; got != "ENG-7" {
			t.Errorf("TICKET_ID = %q, want ENG-7", got)
		}
		if got := vars["TICKET_URL"]; got != "https://linear.app/team/issue/ENG-7" {
			t.Errorf("TICKET_URL = %q, want linear URL", got)
		}
		if got := vars["TICKET_PROVIDER"]; got != "linear" {
			t.Errorf("TICKET_PROVIDER = %q, want linear", got)
		}
	})

	t.Run("jira metadata wins when both present", func(t *testing.T) {
		provider := TicketContextProvider("", map[string]interface{}{
			"jira_issue_key":          "JIRA-1",
			"linear_issue_identifier": "LIN-1",
		})
		vars := provider()
		if got := vars["TICKET_ID"]; got != "JIRA-1" {
			t.Errorf("TICKET_ID = %q, want JIRA-1 (jira wins over linear)", got)
		}
		if got := vars["TICKET_PROVIDER"]; got != "jira" {
			t.Errorf("TICKET_PROVIDER = %q, want jira", got)
		}
	})

	t.Run("missing ticket metadata omits ticket keys but always sets TASK_TITLE", func(t *testing.T) {
		provider := TicketContextProvider("Some title", nil)
		vars := provider()
		if _, ok := vars["TICKET_ID"]; ok {
			t.Error("TICKET_ID should not be present when no ticket metadata")
		}
		if _, ok := vars["TICKET_URL"]; ok {
			t.Error("TICKET_URL should not be present when no ticket metadata")
		}
		if _, ok := vars["TICKET_PROVIDER"]; ok {
			t.Error("TICKET_PROVIDER should not be present when no ticket metadata")
		}
		if got, ok := vars["TASK_TITLE"]; !ok || got != "Some title" {
			t.Errorf("TASK_TITLE = %q (ok=%v), want Some title", got, ok)
		}
	})

	t.Run("empty task title still sets TASK_TITLE to empty string", func(t *testing.T) {
		provider := TicketContextProvider("", nil)
		vars := provider()
		if got, ok := vars["TASK_TITLE"]; !ok || got != "" {
			t.Errorf("TASK_TITLE ok=%v got=%q, want present and empty", ok, got)
		}
	})
}

func TestResolveStartupPrompt(t *testing.T) {
	t.Run("empty prompt returns empty string", func(t *testing.T) {
		got := ResolveStartupPrompt("", "title", nil)
		if got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})

	t.Run("watcher-imported task with jira metadata resolves all placeholders", func(t *testing.T) {
		prompt := "Read {{TICKET_URL}} and start work.\nAcceptance criteria are in the ticket."
		got := ResolveStartupPrompt(prompt, "Fix bug", map[string]interface{}{
			"jira_issue_key": "PROJ-42",
			"jira_issue_url": "https://x/PROJ-42",
		})
		want := "Read https://x/PROJ-42 and start work.\nAcceptance criteria are in the ticket."
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("manual task drops lines with unresolved ticket placeholders", func(t *testing.T) {
		prompt := "Read {{TICKET_URL}} carefully.\nThen begin work on {{TASK_TITLE}}."
		got := ResolveStartupPrompt(prompt, "Refactor billing", nil)
		want := "Then begin work on Refactor billing."
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("linear metadata resolves TICKET_ID and TICKET_URL", func(t *testing.T) {
		prompt := "Please pick up {{TICKET_ID}} ({{TICKET_URL}})."
		got := ResolveStartupPrompt(prompt, "", map[string]interface{}{
			"linear_issue_identifier": "ENG-7",
			"linear_issue_url":        "https://linear.app/team/issue/ENG-7",
		})
		want := "Please pick up ENG-7 (https://linear.app/team/issue/ENG-7)."
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("collapses whitespace after dropping ticket-only line", func(t *testing.T) {
		prompt := "Read {{TICKET_URL}}.\n\nStart with {{TASK_TITLE}}."
		got := ResolveStartupPrompt(prompt, "Refactor", nil)
		want := "Start with Refactor."
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("preserves blank lines between resolved lines", func(t *testing.T) {
		prompt := "First line.\n\nSecond line about {{TASK_TITLE}}."
		got := ResolveStartupPrompt(prompt, "X", nil)
		want := "First line.\n\nSecond line about X."
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("no placeholders returns literal text", func(t *testing.T) {
		prompt := "Just start work."
		got := ResolveStartupPrompt(prompt, "", nil)
		if got != "Just start work." {
			t.Errorf("got %q, want %q", got, "Just start work.")
		}
	})

	t.Run("all placeholders unresolved returns empty string", func(t *testing.T) {
		prompt := "Read {{TICKET_URL}}.\nRead {{TICKET_ID}}."
		got := ResolveStartupPrompt(prompt, "", nil)
		if got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})

	t.Run("mixed CRLF/LF preserves line boundaries", func(t *testing.T) {
		// LF only expected; document behavior for LF-only input.
		prompt := "Line one about {{TASK_TITLE}}.\nLine two about {{TICKET_URL}}."
		got := ResolveStartupPrompt(prompt, "X", nil)
		want := "Line one about X."
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("CRLF line endings normalize to LF", func(t *testing.T) {
		prompt := "Read {{TICKET_URL}}.\r\nStart with {{TASK_TITLE}}."
		got := ResolveStartupPrompt(prompt, "Refactor", nil)
		want := "Start with Refactor."
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("CRLF-only prompt does not leave stray CR in output", func(t *testing.T) {
		prompt := "Start with {{TASK_TITLE}}.\r\nSecond line."
		got := ResolveStartupPrompt(prompt, "X", nil)
		want := "Start with X.\nSecond line."
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("task title containing literal {{...}} is preserved", func(t *testing.T) {
		// Regression: previously the resolved-string regex would misidentify
		// the substituted {{BUG-123}} as an unresolved placeholder and drop
		// the line.
		prompt := "Description: {{TASK_TITLE}}"
		got := ResolveStartupPrompt(prompt, "Investigate {{BUG-123}}", nil)
		want := "Description: Investigate {{BUG-123}}"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("preserves leading whitespace on kept lines", func(t *testing.T) {
		prompt := "  - Read {{TICKET_URL}}\n  - Start on {{TASK_TITLE}}"
		got := ResolveStartupPrompt(prompt, "Refactor", nil)
		want := "  - Start on Refactor"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}
