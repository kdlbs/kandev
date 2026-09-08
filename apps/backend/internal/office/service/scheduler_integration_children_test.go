package service_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/service"
)

// countingPRLister records every ListTaskPRsByTaskIDs call so a test can assert
// the lookup is one batch whose count does not grow with the number of children.
type countingPRLister struct {
	mu     sync.Mutex
	calls  int
	batch  [][]string
	result map[string][]service.TaskPRLink
	err    error
}

func (c *countingPRLister) ListTaskPRsByTaskIDs(
	_ context.Context, taskIDs []string,
) (map[string][]service.TaskPRLink, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	c.batch = append(c.batch, append([]string{}, taskIDs...))
	if c.err != nil {
		return nil, c.err
	}
	return c.result, nil
}

// childFixture seeds a parent and its children on the service's repository.
type childFixture struct {
	svc *service.Service
	si  *service.SchedulerIntegration
}

func newChildFixture(t *testing.T, prs service.TaskPRLister) *childFixture {
	t.Helper()
	svc := newTestService(t, service.ServiceOptions{TaskPRs: prs})
	return &childFixture{svc: svc, si: service.NewSchedulerIntegration(svc, time.Minute)}
}

func (f *childFixture) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	f.svc.ExecSQL(t, query, args...)
}

func (f *childFixture) seedParent(t *testing.T, id, title string) {
	t.Helper()
	f.exec(t, `INSERT INTO tasks (id, workspace_id, title, identifier, state, created_at, updated_at)
		VALUES (?, 'ws-1', ?, 'KAN-1', 'IN_PROGRESS', '2026-01-01 00:00:00', '2026-01-01 00:00:00')`, id, title)
}

func (f *childFixture) seedChild(t *testing.T, id, parentID, title, state, identifier, createdAt string) {
	t.Helper()
	f.exec(t, `INSERT INTO tasks (id, workspace_id, title, identifier, state, parent_id, created_at, updated_at)
		VALUES (?, 'ws-1', ?, ?, ?, ?, ?, ?)`, id, title, identifier, state, parentID, createdAt, createdAt)
}

func (f *childFixture) seedComment(t *testing.T, id, taskID, body, createdAt string) {
	t.Helper()
	f.exec(t, `INSERT INTO task_comments (id, task_id, author_type, author_id, body, source, created_at)
		VALUES (?, ?, 'agent', 'a1', ?, 'agent', ?)`, id, taskID, body, createdAt)
}

// section returns the child list section of the assembled prompt: everything
// between the lead-in and the closing instruction.
func (f *childFixture) section(t *testing.T, reason, payload string) string {
	t.Helper()
	pc := f.si.BuildPromptContextForTest(context.Background(), reason, payload)
	prompt := service.BuildPrompt(pc)
	body, ok := strings.CutSuffix(prompt, "\nReview their output and determine next steps.")
	if !ok {
		t.Fatalf("prompt missing closing instruction:\n%s", prompt)
	}
	_, sec, ok := strings.Cut(body, "\n")
	if !ok {
		t.Fatalf("prompt missing lead-in:\n%s", prompt)
	}
	return sec
}

func TestEnrichChildrenContext_ListsLiveChildren(t *testing.T) {
	prs := &countingPRLister{result: map[string][]service.TaskPRLink{
		"c1": {{URL: "https://x/pull/2"}, {URL: "https://x/pull/1"}},
	}}
	f := newChildFixture(t, prs)
	f.seedParent(t, "p1", "Parent")
	f.seedChild(t, "c1", "p1", "Auth service", "COMPLETED", "KAN-2", "2026-01-01 00:00:01")
	f.seedChild(t, "c2", "p1", "API gateway", "CANCELLED", "KAN-3", "2026-01-01 00:00:02")
	f.seedComment(t, "cm1", "c1", "Implemented JWT generation", "2026-01-01 00:00:05")

	got := f.section(t, service.RunReasonTaskChildrenCompleted, `{"task_id":"p1"}`)
	want := "\nChild tasks:\n" +
		"- KAN-2 (Auth service) [COMPLETED] — \"Implemented JWT generation\" — https://x/pull/1, https://x/pull/2\n" +
		"- KAN-3 (API gateway) [CANCELLED]\n"
	if got != want {
		t.Errorf("section =\n%q\nwant\n%q", got, want)
	}
}

// Four producers can queue this run and exactly one wins a race none of them can
// observe. The section is derived after that race, so it cannot depend on it.
func TestChildSectionIsByteIdenticalAcrossProducers(t *testing.T) {
	payloads := map[string]string{
		"office cascade":   `{"task_id":"p1","child_task_id":"c1","workspace_id":"ws-1"}`,
		"event subscriber": `{"task_id":"p1"}`,
		"wake reconciler":  `{"task_id":"p1","operation_id":"wake:p1:abc"}`,
		"orchestrator":     `{"task_id":"p1","reason":"task_children_completed"}`,
		"stale child snapshot": `{"task_id":"p1","children":[{"identifier":"STALE","title":"Stale",` +
			`"state":"COMPLETED","last_comment":"from the payload"}],"truncated":true}`,
	}

	var first, firstName string
	for name, payload := range payloads {
		f := newChildFixture(t, &countingPRLister{})
		f.seedParent(t, "p1", "Parent")
		f.seedChild(t, "c1", "p1", "Auth", "COMPLETED", "KAN-2", "2026-01-01 00:00:01")
		f.seedComment(t, "cm1", "c1", "done", "2026-01-01 00:00:05")

		got := f.section(t, service.RunReasonTaskChildrenCompleted, payload)
		if first == "" {
			first, firstName = got, name
			continue
		}
		if got != first {
			t.Errorf("producer %q rendered\n%q\nbut producer %q rendered\n%q",
				name, got, firstName, first)
		}
	}
	if !strings.Contains(first, "KAN-2") {
		t.Errorf("section did not come from the database:\n%q", first)
	}
}

// A payload's own child snapshot must not contribute to, suppress, or alter the
// section, however stale or contradictory it is.
func TestPayloadChildrenKeyIsIgnored(t *testing.T) {
	f := newChildFixture(t, &countingPRLister{})
	f.seedParent(t, "p1", "Parent")
	f.seedChild(t, "c1", "p1", "Real child", "COMPLETED", "KAN-2", "2026-01-01 00:00:01")

	payload := `{"task_id":"p1","children":[{"identifier":"GHOST","title":"Ghost",` +
		`"state":"COMPLETED","last_comment":"never happened"}],"truncated":true}`
	got := f.section(t, service.RunReasonTaskChildrenCompleted, payload)

	if strings.Contains(got, "GHOST") || strings.Contains(got, "never happened") {
		t.Errorf("payload child leaked into the section:\n%q", got)
	}
	if strings.Contains(got, "showing first 20") {
		t.Errorf("payload truncation flag leaked into the section:\n%q", got)
	}
	if !strings.Contains(got, "KAN-2 (Real child)") {
		t.Errorf("section missing the real child:\n%q", got)
	}
}

func TestChildSectionRendersForLegacyReason(t *testing.T) {
	f := newChildFixture(t, &countingPRLister{})
	f.seedParent(t, "p1", "Parent")
	f.seedChild(t, "c1", "p1", "Auth", "COMPLETED", "KAN-2", "2026-01-01 00:00:01")

	got := f.section(t, "children_completed", `{"task_id":"p1"}`)
	if !strings.Contains(got, "KAN-2 (Auth) [COMPLETED]") {
		t.Errorf("legacy reason lost the child list:\n%q", got)
	}
}

// No other wake gains content or a database read.
func TestNonChildrenReasonPerformsNoChildRead(t *testing.T) {
	prs := &countingPRLister{}
	f := newChildFixture(t, prs)
	f.seedParent(t, "p1", "Parent")
	f.seedChild(t, "c1", "p1", "Auth", "COMPLETED", "KAN-2", "2026-01-01 00:00:01")

	pc := f.si.BuildPromptContextForTest(
		context.Background(), service.RunReasonTaskAssigned, `{"task_id":"p1"}`)
	if len(pc.ChildSummaries) != 0 {
		t.Errorf("task_assigned gained %d child summaries", len(pc.ChildSummaries))
	}
	if prs.calls != 0 {
		t.Errorf("task_assigned made %d PR lookups, want 0", prs.calls)
	}
}

// One producer substitutes a completed state for a child sitting in a terminal
// workflow step. That reinterpretation is not available to the other three, so
// it must not be reproduced here.
func TestChildStateIsReportedAsStored(t *testing.T) {
	f := newChildFixture(t, &countingPRLister{})
	f.seedParent(t, "p1", "Parent")
	f.seedChild(t, "c1", "p1", "In a done step", "IN_PROGRESS", "KAN-2", "2026-01-01 00:00:01")
	f.exec(t, `UPDATE tasks SET workflow_step_id = 'step-done' WHERE id = 'c1'`)

	got := f.section(t, service.RunReasonTaskChildrenCompleted, `{"task_id":"p1"}`)
	if !strings.Contains(got, "[IN_PROGRESS]") {
		t.Errorf("state was reinterpreted rather than reported as stored:\n%q", got)
	}
}

// The child list is context, not the point: the parent must still be woken.
func TestChildReadFailureStillLaunches(t *testing.T) {
	f := newChildFixture(t, &countingPRLister{})
	f.seedParent(t, "p1", "Parent")
	f.seedChild(t, "c1", "p1", "Auth", "COMPLETED", "KAN-2", "2026-01-01 00:00:01")
	// The last-comment subquery cannot run without this table.
	f.exec(t, `DROP TABLE task_comments`)

	pc := f.si.BuildPromptContextForTest(
		context.Background(), service.RunReasonTaskChildrenCompleted, `{"task_id":"p1"}`)
	prompt := service.BuildPrompt(pc)

	if len(pc.ChildSummaries) != 0 {
		t.Errorf("failed read produced %d summaries", len(pc.ChildSummaries))
	}
	if strings.Contains(prompt, "Child tasks:") {
		t.Errorf("failed read still rendered a section:\n%s", prompt)
	}
	if !strings.HasPrefix(prompt, "All child tasks") ||
		!strings.HasSuffix(prompt, "\nReview their output and determine next steps.") {
		t.Errorf("failed read changed the lead-in or closing instruction:\n%s", prompt)
	}
}

func TestPRLookupFailureStillRendersLine(t *testing.T) {
	cases := map[string]service.TaskPRLister{
		"lister unwired": nil,
		"lister errors":  &countingPRLister{err: fmt.Errorf("pr backend down")},
	}
	for name, prs := range cases {
		t.Run(name, func(t *testing.T) {
			f := newChildFixture(t, prs)
			f.seedParent(t, "p1", "Parent")
			f.seedChild(t, "c1", "p1", "Auth", "COMPLETED", "KAN-2", "2026-01-01 00:00:01")
			f.seedComment(t, "cm1", "c1", "done", "2026-01-01 00:00:05")

			got := f.section(t, service.RunReasonTaskChildrenCompleted, `{"task_id":"p1"}`)
			want := "\nChild tasks:\n- KAN-2 (Auth) [COMPLETED] — \"done\"\n"
			if got != want {
				t.Errorf("section =\n%q\nwant\n%q", got, want)
			}
		})
	}
}

func TestRunWithoutTaskIDSkipsChildRead(t *testing.T) {
	prs := &countingPRLister{}
	f := newChildFixture(t, prs)

	pc := f.si.BuildPromptContextForTest(
		context.Background(), service.RunReasonTaskChildrenCompleted, `{}`)
	prompt := service.BuildPrompt(pc)

	if len(pc.ChildSummaries) != 0 {
		t.Errorf("run without a task id produced %d summaries", len(pc.ChildSummaries))
	}
	if prs.calls != 0 {
		t.Errorf("run without a task id made %d PR lookups, want 0", prs.calls)
	}
	if strings.Contains(prompt, "Child tasks:") {
		t.Errorf("run without a task id rendered a section:\n%s", prompt)
	}
}

func TestParentGoneSkipsSection(t *testing.T) {
	f := newChildFixture(t, &countingPRLister{})
	f.seedParent(t, "p1", "Parent")
	f.seedChild(t, "c1", "p1", "Orphan", "COMPLETED", "KAN-2", "2026-01-01 00:00:01")
	f.exec(t, `DELETE FROM tasks WHERE id = 'p1'`)

	got := f.section(t, service.RunReasonTaskChildrenCompleted, `{"task_id":"p1"}`)
	if got != "" {
		t.Errorf("deleted parent rendered a section:\n%q", got)
	}
}

// The run carries no snapshot, so a change between queue time and assembly is
// reflected, and reassembling the same run re-derives rather than reuses.
func TestSectionReflectsAssemblyTimeValues(t *testing.T) {
	f := newChildFixture(t, &countingPRLister{})
	f.seedParent(t, "p1", "Parent")
	f.seedChild(t, "c1", "p1", "Auth", "COMPLETED", "KAN-2", "2026-01-01 00:00:01")
	f.seedComment(t, "cm1", "c1", "first pass", "2026-01-01 00:00:05")

	before := f.section(t, service.RunReasonTaskChildrenCompleted, `{"task_id":"p1"}`)
	if !strings.Contains(before, `"first pass"`) || !strings.Contains(before, "[COMPLETED]") {
		t.Fatalf("unexpected initial section:\n%q", before)
	}

	// Two assemblies with nothing changed in between are byte-identical.
	if again := f.section(t, service.RunReasonTaskChildrenCompleted, `{"task_id":"p1"}`); again != before {
		t.Errorf("reassembly differed:\n%q\nvs\n%q", again, before)
	}

	f.exec(t, `UPDATE tasks SET state = 'IN_PROGRESS', title = 'Auth v2' WHERE id = 'c1'`)
	f.seedComment(t, "cm2", "c1", "reopened", "2026-01-01 00:00:09")
	f.seedChild(t, "c2", "p1", "New sibling", "COMPLETED", "KAN-3", "2026-01-01 00:00:02")

	after := f.section(t, service.RunReasonTaskChildrenCompleted, `{"task_id":"p1"}`)
	for _, want := range []string{"Auth v2", "[IN_PROGRESS]", `"reopened"`, "KAN-3 (New sibling)"} {
		if !strings.Contains(after, want) {
			t.Errorf("section missing %q after the change:\n%q", want, after)
		}
	}
}

// The section's cost must not grow with the number of children.
func TestChildReadCountDoesNotGrowWithChildren(t *testing.T) {
	measure := func(t *testing.T, children int) *countingPRLister {
		t.Helper()
		prs := &countingPRLister{}
		f := newChildFixture(t, prs)
		f.seedParent(t, "p1", "Parent")
		for i := range children {
			id := fmt.Sprintf("c%02d", i)
			f.seedChild(t, id, "p1", id, "COMPLETED", "KAN-"+id,
				fmt.Sprintf("2026-01-01 00:00:%02d", i))
			f.seedComment(t, "cm"+id, id, "done", "2026-01-01 00:01:00")
		}
		f.section(t, service.RunReasonTaskChildrenCompleted, `{"task_id":"p1"}`)
		return prs
	}

	few := measure(t, 3)
	many := measure(t, 30)

	if few.calls != 1 || many.calls != 1 {
		t.Errorf("PR lookups: 3 children = %d calls, 30 children = %d calls, want 1 each",
			few.calls, many.calls)
	}
	if len(few.batch) != 1 || len(few.batch[0]) != 3 {
		t.Errorf("3 children batched as %v, want one batch of 3", few.batch)
	}
	// 30 live children exceed the 20-row cap, so the batch is the capped set.
	if len(many.batch) != 1 || len(many.batch[0]) != 20 {
		t.Errorf("30 children batched as %d batches of %d, want one batch of 20",
			len(many.batch), len(many.batch[0]))
	}
}

func TestChildSectionTruncationNotice(t *testing.T) {
	f := newChildFixture(t, &countingPRLister{})
	f.seedParent(t, "p1", "Parent")
	for i := range 21 {
		id := fmt.Sprintf("c%02d", i)
		f.seedChild(t, id, "p1", id, "COMPLETED", "KAN-"+id,
			fmt.Sprintf("2026-01-01 00:00:%02d", i))
	}

	got := f.section(t, service.RunReasonTaskChildrenCompleted, `{"task_id":"p1"}`)
	if n := strings.Count(got, "\n- "); n != 20 {
		t.Errorf("section rendered %d child lines, want 20", n)
	}
	if !strings.Contains(got, "showing first 20 children") {
		t.Errorf("section missing the truncation notice:\n%q", got)
	}
}
