package service_test

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/office/service"
)

// childSection returns just the child list section of a children-completed
// prompt: everything between the lead-in and the closing instruction. The
// section is under a byte-identity contract, so tests compare it whole rather
// than probing for substrings.
func childSection(t *testing.T, pc *service.PromptContext) string {
	t.Helper()
	pc.Reason = service.RunReasonTaskChildrenCompleted
	if pc.TaskIdentifier == "" {
		pc.TaskIdentifier = "KAN-1"
	}
	if pc.TaskTitle == "" {
		pc.TaskTitle = "Parent"
	}
	prompt := service.BuildPrompt(pc)
	const closing = "\nReview their output and determine next steps."
	body, ok := strings.CutSuffix(prompt, closing)
	if !ok {
		t.Fatalf("prompt missing closing instruction:\n%s", prompt)
	}
	_, section, ok := strings.Cut(body, "\n")
	if !ok {
		t.Fatalf("prompt missing lead-in:\n%s", prompt)
	}
	return section
}

// line returns the single rendered line for one child.
func line(t *testing.T, c service.ChildSummaryPrompt) string {
	t.Helper()
	section := childSection(t, &service.PromptContext{
		ChildSummaries: []service.ChildSummaryPrompt{c},
	})
	lines := strings.Split(strings.TrimSuffix(section, "\n"), "\n")
	// section is "", "Child tasks:", "- ...".
	if len(lines) != 3 {
		t.Fatalf("section rendered %d lines, want 3 (blank, heading, one child):\n%q",
			len(lines), section)
	}
	return lines[2]
}

func TestChildSummaryLine_Shapes(t *testing.T) {
	base := service.ChildSummaryPrompt{Identifier: "KAN-2", Title: "Auth", State: "COMPLETED"}

	withComment := base
	withComment.LastComment = "shipped it"
	withLinks := base
	withLinks.PRLinks = []string{"https://x/1"}
	withBoth := withComment
	withBoth.PRLinks = []string{"https://x/1"}

	cases := []struct {
		name string
		in   service.ChildSummaryPrompt
		want string
	}{
		{"bare", base, `- KAN-2 (Auth) [COMPLETED]`},
		{"comment only", withComment, `- KAN-2 (Auth) [COMPLETED] — "shipped it"`},
		{"links only", withLinks, `- KAN-2 (Auth) [COMPLETED] — https://x/1`},
		{"both", withBoth, `- KAN-2 (Auth) [COMPLETED] — "shipped it" — https://x/1`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := line(t, tc.in); got != tc.want {
				t.Errorf("line = %q\nwant  %q", got, tc.want)
			}
		})
	}
}

// A child with no comments and one whose most recent comment is an empty body
// carry the same information: none. They must render identically.
func TestChildSummaryLine_EmptyCommentRendersAsNoComment(t *testing.T) {
	none := service.ChildSummaryPrompt{Identifier: "KAN-2", Title: "Auth", State: "DONE"}
	empty := none
	empty.LastComment = ""

	if a, b := line(t, none), line(t, empty); a != b {
		t.Errorf("no-comment child rendered %q, empty-comment child rendered %q", a, b)
	}
}

func TestChildSummaryLine_MissingIdentifier(t *testing.T) {
	got := line(t, service.ChildSummaryPrompt{Title: "Auth", State: "DONE"})
	if want := `- ? (Auth) [DONE]`; got != want {
		t.Errorf("line = %q, want %q", got, want)
	}
}

// One child is exactly one line. A line break in any field — not only the
// comment — must not split it, or the parent reads a phantom child.
func TestChildSummaryLine_IsSingleLine(t *testing.T) {
	cases := []struct {
		name string
		in   service.ChildSummaryPrompt
	}{
		{"newline in title", service.ChildSummaryPrompt{
			Identifier: "KAN-2", Title: "Auth\nservice", State: "DONE"}},
		{"CRLF in comment", service.ChildSummaryPrompt{
			Identifier: "KAN-2", Title: "Auth", State: "DONE", LastComment: "a\r\nb"}},
		{"line separator in comment", service.ChildSummaryPrompt{
			Identifier: "KAN-2", Title: "Auth", State: "DONE", LastComment: "a b"}},
		{"tab in title", service.ChildSummaryPrompt{
			Identifier: "KAN-2", Title: "a\tb", State: "DONE"}},
		{"newline in identifier", service.ChildSummaryPrompt{
			Identifier: "KAN\n2", Title: "Auth", State: "DONE"}},
		{"newline in state", service.ChildSummaryPrompt{
			Identifier: "KAN-2", Title: "Auth", State: "DO\nNE"}},
		{"newline in url", service.ChildSummaryPrompt{
			Identifier: "KAN-2", Title: "Auth", State: "DONE",
			PRLinks: []string{"https://x/\n1"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := line(t, tc.in)
			if strings.ContainsAny(got, "\n\r") || strings.Contains(got, " ") {
				t.Errorf("line contains a break: %q", got)
			}
			// A control character becomes one space, never an escape, so it
			// cannot change the field's rendered length.
			if strings.Contains(got, `\n`) || strings.Contains(got, `\t`) {
				t.Errorf("control character survived as an escape: %q", got)
			}
		})
	}
}

// Sanitization replaces one rune with one rune, so it can never turn an empty
// body into a non-empty one and gain a segment.
func TestChildSummaryLine_ControlOnlyCommentStillRenders(t *testing.T) {
	got := line(t, service.ChildSummaryPrompt{
		Identifier: "KAN-2", Title: "Auth", State: "DONE", LastComment: "\n\t"})
	if want := `- KAN-2 (Auth) [DONE] — "  "`; got != want {
		t.Errorf("line = %q, want %q", got, want)
	}
}

func TestChildSummaryLine_CommentCap(t *testing.T) {
	const marker = " [truncated]"
	cases := []struct {
		name      string
		body      string
		wantMark  bool
		wantRunes int
	}{
		{"at the limit", strings.Repeat("a", 500), false, 500},
		{"one past the limit", strings.Repeat("a", 501), true, 497},
		{"multibyte at the limit", strings.Repeat("界", 500), false, 500},
		{"multibyte past the limit", strings.Repeat("界", 501), true, 497},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := line(t, service.ChildSummaryPrompt{
				Identifier: "KAN-2", Title: "Auth", State: "DONE", LastComment: tc.body})
			_, quoted, _ := strings.Cut(got, `— `)
			body := strings.TrimSuffix(strings.TrimPrefix(quoted, `"`), `"`)
			if strings.HasSuffix(body, marker) != tc.wantMark {
				t.Errorf("marker present = %v, want %v (body %q)",
					!tc.wantMark, tc.wantMark, body)
			}
			if n := utf8.RuneCountInString(body); n != tc.wantRunes {
				t.Errorf("rendered body = %d code points, want %d", n, tc.wantRunes)
			}
			if !utf8.ValidString(got) {
				t.Error("line is not valid UTF-8")
			}
		})
	}
}

func TestChildSummaryLine_TitleCap(t *testing.T) {
	got := line(t, service.ChildSummaryPrompt{
		Identifier: "KAN-2", Title: strings.Repeat("界", 201), State: "DONE"})
	title := got[strings.Index(got, "(")+1 : strings.LastIndex(got, ")")]
	if !strings.HasSuffix(title, " [truncated]") {
		t.Errorf("over-cap title not marked: %q", title)
	}
	if n := utf8.RuneCountInString(title); n != 200 {
		t.Errorf("title = %d code points, want 200 including the marker", n)
	}

	exact := line(t, service.ChildSummaryPrompt{
		Identifier: "KAN-2", Title: strings.Repeat("界", 200), State: "DONE"})
	if strings.Contains(exact, "[truncated]") {
		t.Error("title exactly at the cap was marked truncated")
	}
}

func TestChildSummaryLine_IdentifierAndStateCaps(t *testing.T) {
	got := line(t, service.ChildSummaryPrompt{
		Identifier: strings.Repeat("界", 51), Title: "Auth", State: strings.Repeat("界", 51)})

	identifier := strings.Repeat("界", 38) + " [truncated]"
	state := strings.Repeat("界", 38) + " [truncated]"
	if !strings.Contains(got, "- "+identifier+" (Auth) [") {
		t.Errorf("identifier was not capped: %q", got)
	}
	if !strings.HasSuffix(got, "["+state+"]") {
		t.Errorf("state was not capped: %q", got)
	}
	if n := utf8.RuneCountInString(identifier); n != 50 {
		t.Errorf("identifier = %d code points, want 50 including the marker", n)
	}
	if n := utf8.RuneCountInString(state); n != 50 {
		t.Errorf("state = %d code points, want 50 including the marker", n)
	}

	exact := line(t, service.ChildSummaryPrompt{
		Identifier: strings.Repeat("界", 50), Title: "Auth", State: strings.Repeat("界", 50)})
	if strings.Contains(exact, "[truncated]") {
		t.Errorf("identifier or state exactly at the cap was marked truncated: %q", exact)
	}
}

func TestChildSummaryLine_PRSegment(t *testing.T) {
	t.Run("sorted ascending by url", func(t *testing.T) {
		got := line(t, service.ChildSummaryPrompt{
			Identifier: "KAN-2", Title: "Auth", State: "DONE",
			PRLinks: []string{"https://x/3", "https://x/1", "https://x/2"}})
		want := `- KAN-2 (Auth) [DONE] — https://x/1, https://x/2, https://x/3`
		if got != want {
			t.Errorf("line = %q\nwant  %q", got, want)
		}
	})

	t.Run("caps at ten and reports the unrendered count", func(t *testing.T) {
		links := make([]string, 12)
		for i := range links {
			links[i] = fmt.Sprintf("https://x/%02d", i)
		}
		got := line(t, service.ChildSummaryPrompt{
			Identifier: "KAN-2", Title: "Auth", State: "DONE", PRLinks: links})
		if n := strings.Count(got, "https://x/"); n != 10 {
			t.Errorf("rendered %d urls, want 10", n)
		}
		if !strings.HasSuffix(got, " (+2 more)") {
			t.Errorf("line = %q, want it to end with the count NOT rendered", got)
		}
		if strings.Contains(got, "https://x/10") || strings.Contains(got, "https://x/11") {
			t.Errorf("elided urls must be the last by sort order: %q", got)
		}
	})

	t.Run("clamps the numeral", func(t *testing.T) {
		links := make([]string, 1200)
		for i := range links {
			links[i] = fmt.Sprintf("https://x/%04d", i)
		}
		got := line(t, service.ChildSummaryPrompt{
			Identifier: "KAN-2", Title: "Auth", State: "DONE", PRLinks: links})
		if !strings.HasSuffix(got, " (+999+ more)") {
			t.Errorf("line = %q, want a clamped marker", got)
		}
	})

	t.Run("exactly ten gets no marker", func(t *testing.T) {
		links := make([]string, 10)
		for i := range links {
			links[i] = fmt.Sprintf("https://x/%02d", i)
		}
		got := line(t, service.ChildSummaryPrompt{
			Identifier: "KAN-2", Title: "Auth", State: "DONE", PRLinks: links})
		if strings.Contains(got, "more)") {
			t.Errorf("ten urls should not elide anything: %q", got)
		}
	})

	t.Run("caps each url", func(t *testing.T) {
		got := line(t, service.ChildSummaryPrompt{
			Identifier: "KAN-2", Title: "Auth", State: "DONE",
			PRLinks: []string{"https://x/" + strings.Repeat("a", 300)}})
		_, url, _ := strings.Cut(got, "— ")
		if !strings.HasSuffix(url, " [truncated]") {
			t.Errorf("over-cap url not marked: %q", url)
		}
		if n := utf8.RuneCountInString(url); n != 200 {
			t.Errorf("url = %d code points, want 200 including the marker", n)
		}
	})

	t.Run("empty slice renders nothing", func(t *testing.T) {
		got := line(t, service.ChildSummaryPrompt{
			Identifier: "KAN-2", Title: "Auth", State: "DONE", PRLinks: []string{}})
		if want := `- KAN-2 (Auth) [DONE]`; got != want {
			t.Errorf("line = %q, want %q", got, want)
		}
	})
}

// The heading labels a list that may contain a child no longer in a terminal
// state, so it must not assert that the children it lists have completed. The
// lead-in describes why the wake fired and is unaffected.
func TestChildrenCompletedPrompt_HeadingDoesNotAssertCompletion(t *testing.T) {
	pc := &service.PromptContext{
		Reason:         service.RunReasonTaskChildrenCompleted,
		TaskIdentifier: "KAN-1",
		TaskTitle:      "Parent",
		ChildSummaries: []service.ChildSummaryPrompt{
			{Identifier: "KAN-2", Title: "Restarted", State: "IN_PROGRESS"},
		},
	}
	prompt := service.BuildPrompt(pc)

	if !strings.Contains(prompt, "\nChild tasks:\n") {
		t.Errorf("prompt missing the neutral heading:\n%s", prompt)
	}
	if strings.Contains(prompt, "Completed children:") {
		t.Errorf("heading still asserts completion:\n%s", prompt)
	}
	if !strings.HasPrefix(prompt, "All child tasks for your task [KAN-1]: Parent have completed.\n") {
		t.Errorf("lead-in changed:\n%s", prompt)
	}
	if !strings.HasSuffix(prompt, "\nReview their output and determine next steps.") {
		t.Errorf("closing instruction changed:\n%s", prompt)
	}
}

func TestChildrenCompletedPrompt_EmptySectionKeepsLeadInAndClosing(t *testing.T) {
	prompt := service.BuildPrompt(&service.PromptContext{
		Reason:         service.RunReasonTaskChildrenCompleted,
		TaskIdentifier: "KAN-1",
		TaskTitle:      "Parent",
	})
	want := "All child tasks for your task [KAN-1]: Parent have completed.\n" +
		"\nReview their output and determine next steps."
	if prompt != want {
		t.Errorf("prompt = %q\nwant     %q", prompt, want)
	}
}
