package coordinator

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/authz"
)

type fakeUndoTasks struct {
	tasks     map[string]*UndoTask
	steps     map[string]*UndoStep
	taskErr   error
	stepErr   error
	archived  []string
	moves     []fakeMove
	archErr   error
	moveErr   error
	admitted  bool
	sessions  bool
	sessErr   error
	onMove    func()
	onMoveCtx func(ctx context.Context) error
	onArchive func()
	getTasks  int
	getSteps  int
	nodes     []StepNode
	nodesErr  error
	onGetStep func()
}

type fakeMove struct {
	ID, WorkflowID, StepID string
	Opts                   UndoMoveOptions
}

func (f *fakeUndoTasks) ArchiveTask(_ context.Context, id string) error {
	f.archived = append(f.archived, id)
	if f.onArchive != nil {
		f.onArchive()
	}
	return f.archErr
}

func (f *fakeUndoTasks) GetTask(_ context.Context, id string) (*UndoTask, error) {
	f.getTasks++
	if f.taskErr != nil {
		return nil, f.taskErr
	}
	if t, ok := f.tasks[id]; ok {
		c := *t
		return &c, nil
	}
	return nil, ErrTaskNotFound
}

func (f *fakeUndoTasks) MoveTaskWithOptions(ctx context.Context, id, wf, step string, _ int, opts UndoMoveOptions) (bool, error) {
	f.moves = append(f.moves, fakeMove{id, wf, step, opts})
	if f.onMove != nil {
		f.onMove()
	}
	if f.onMoveCtx != nil {
		if err := f.onMoveCtx(ctx); err != nil {
			return false, err
		}
	}
	if f.moveErr != nil {
		return false, f.moveErr
	}
	if t, ok := f.tasks[id]; ok {
		t.WorkflowStepID = step
	}
	return f.admitted, nil
}

func (f *fakeUndoTasks) GetStep(_ context.Context, id string) (*UndoStep, error) {
	f.getSteps++
	if f.onGetStep != nil {
		f.onGetStep()
	}
	if f.stepErr != nil {
		return nil, f.stepErr
	}
	if s, ok := f.steps[id]; ok {
		c := *s
		return &c, nil
	}
	return nil, ErrStepNotFound
}

func (f *fakeUndoTasks) ListSteps(context.Context, string) ([]StepNode, error) {
	return f.nodes, f.nodesErr
}

func (f *fakeUndoTasks) HasActiveSession(context.Context, string) (bool, error) {
	return f.sessions, f.sessErr
}

func newActivityService(t *testing.T, phase2 bool) (*Service, *Store, *Coordinator, *fakeWorkspaceAuthorizer, *fakeUndoTasks) {
	t.Helper()
	store := newTestStore(t)
	az := &fakeWorkspaceAuthorizer{}
	svc := NewService(store, newValidatorForTest(nil, nil), az, newTestLogger(t), WithPhase2(phase2))
	fake := &fakeUndoTasks{tasks: map[string]*UndoTask{}, steps: map[string]*UndoStep{}, admitted: true}
	svc.SetUndoDeps(fake)
	c := newTestCoordinator(t, store, "ws-1")
	return svc, store, c, az, fake
}

func seedMoveProposal(t *testing.T, store *Store, c *Coordinator, id, outcome string) {
	t.Helper()
	_, err := store.db.ExecContext(context.Background(), store.db.Rebind(`INSERT INTO coordinator_proposals
		(id, coordinator_id, workspace_id, status, spec_json, created_at, updated_at, kind, outcome_json)
		VALUES (?, ?, ?, 'approved', '{}', ?, ?, 'move', ?)`), id, c.ID, c.WorkspaceID, time.Now().UTC(), time.Now().UTC(), outcome)
	if err != nil {
		t.Fatal(err)
	}
}

func seedApproved(t *testing.T, store *Store, c *Coordinator, id string, class Action, at time.Time, mutate func(*ActivityRow)) {
	t.Helper()
	row := validRow(c.ID)
	row.ID = id
	row.ActionClass = class
	row.Outcome = ActivityApproved
	row.CreatedAt = at
	row.UpdatedAt = at
	if mutate != nil {
		mutate(&row)
	}
	if err := store.InsertActivity(context.Background(), store.db, row); err != nil {
		t.Fatal(err)
	}
}

func TestListActivity_PagesWithCursorAndScope(t *testing.T) {
	svc, store, c, az, _ := newActivityService(t, true)
	ctx := context.Background()
	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	for i, id := range []string{"a", "b", "c"} {
		seedActivity(t, store, c.ID, id, ActionCreateTask, ActivityProposed, base.Add(time.Duration(i)*time.Second))
	}
	page, err := svc.ListActivity(ctx, "ws-1", c.ID, ListActivityParams{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Rows) != 2 || page.Rows[0].ID != "c" || page.Rows[1].ID != "b" || page.NextCursor == nil {
		t.Fatalf("page 1 = %+v", page)
	}
	next, err := svc.ListActivity(ctx, "ws-1", c.ID, ListActivityParams{Limit: 2, Before: *page.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Rows) != 1 || next.Rows[0].ID != "a" || next.NextCursor != nil {
		t.Fatalf("page 2 = %+v", next)
	}
	if len(az.scopes) != 2 || az.scopes[0] != authz.ScopeWorkspaceRead {
		t.Fatalf("scopes = %v", az.scopes)
	}
}

func TestListActivity_DefaultsAndValidation(t *testing.T) {
	svc, _, c, _, _ := newActivityService(t, true)
	ctx := context.Background()
	if _, err := svc.ListActivity(ctx, "ws-1", c.ID, ListActivityParams{}); err != nil {
		t.Fatalf("zero limit defaults: %v", err)
	}
	for _, p := range []ListActivityParams{{Limit: 51}, {Limit: -1}} {
		_, err := svc.ListActivity(ctx, "ws-1", c.ID, p)
		assertFieldError(t, err, "limit")
	}
	_, err := svc.ListActivity(ctx, "ws-1", c.ID, ListActivityParams{Class: "bogus"})
	assertFieldError(t, err, "class")
	_, err = svc.ListActivity(ctx, "ws-1", c.ID, ListActivityParams{Class: "unknown"})
	if err != nil {
		t.Fatalf("unknown class allowed: %v", err)
	}
	for _, bad := range []string{"!!!", base64.RawURLEncoding.EncodeToString([]byte("nope")),
		base64.RawURLEncoding.EncodeToString([]byte(`{"t":"yesterday","i":"x"}`))} {
		_, err = svc.ListActivity(ctx, "ws-1", c.ID, ListActivityParams{Before: bad})
		assertFieldError(t, err, "before")
	}
}

func TestListActivity_ForeignCoordinatorAndAuthz(t *testing.T) {
	svc, store, c, az, _ := newActivityService(t, true)
	other := newTestCoordinator(t, store, "ws-2")
	if _, err := svc.ListActivity(context.Background(), "ws-1", other.ID, ListActivityParams{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign coordinator err = %v", err)
	}
	az.err = errors.New("forbidden")
	if _, err := svc.ListActivity(context.Background(), "ws-1", c.ID, ListActivityParams{}); err == nil {
		t.Fatal("authz error swallowed")
	}
}

func TestListActivity_ClassFilter(t *testing.T) {
	svc, store, c, _, _ := newActivityService(t, true)
	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	seedActivity(t, store, c.ID, "a", ActionCreateTask, ActivityProposed, base)
	seedActivity(t, store, c.ID, "b", ActionMove, ActivityProposed, base.Add(time.Second))
	page, err := svc.ListActivity(context.Background(), "ws-1", c.ID, ListActivityParams{Class: "move"})
	if err != nil || len(page.Rows) != 1 || page.Rows[0].ID != "b" {
		t.Fatalf("page = %+v err = %v", page, err)
	}
}

func TestListActivity_UndoableAndEnrichment(t *testing.T) {
	svc, store, c, _, fake := newActivityService(t, true)
	ctx := context.Background()
	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	fake.tasks["t1"] = &UndoTask{Identifier: "KAN-1"}
	tid := "t1"
	seedApproved(t, store, c, "create-ok", ActionCreateTask, base, func(r *ActivityRow) { r.TargetTaskID = &tid })
	seedApproved(t, store, c, "create-notask", ActionCreateTask, base.Add(1*time.Second), nil)
	undoneAt := base
	seedApproved(t, store, c, "create-undone", ActionCreateTask, base.Add(2*time.Second), func(r *ActivityRow) {
		r.TargetTaskID = &tid
		r.UndoneAt = &undoneAt
	})
	seedApproved(t, store, c, "message", ActionMessage, base.Add(3*time.Second), nil)
	p1, p2, p3, p4 := "p-ok", "p-noop", "p-bad", "p-missing"
	seedMoveProposal(t, store, c, p1, `{"from_step_id":"s1","to_step_id":"s2"}`)
	seedMoveProposal(t, store, c, p2, `{"from_step_id":"s2","to_step_id":"s2","noop":true}`)
	seedMoveProposal(t, store, c, p3, `{"to_step_id":"s2"}`)
	seedApproved(t, store, c, "move-ok", ActionMove, base.Add(4*time.Second), func(r *ActivityRow) { r.ProposalID = &p1; r.TargetTaskID = &tid })
	seedApproved(t, store, c, "move-noop", ActionMove, base.Add(5*time.Second), func(r *ActivityRow) { r.ProposalID = &p2 })
	seedApproved(t, store, c, "move-bad", ActionMove, base.Add(6*time.Second), func(r *ActivityRow) { r.ProposalID = &p3 })
	seedApproved(t, store, c, "move-noprop", ActionMove, base.Add(7*time.Second), func(r *ActivityRow) { r.ProposalID = &p4 })
	page, err := svc.ListActivity(ctx, "ws-1", c.ID, ListActivityParams{})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]ActivityItem{}
	for _, r := range page.Rows {
		byID[r.ID] = r
	}
	want := map[string]bool{"create-ok": true, "create-notask": false, "create-undone": false, "message": false,
		"move-ok": true, "move-noop": false, "move-bad": false, "move-noprop": false}
	for id, u := range want {
		if byID[id].Undoable != u {
			t.Errorf("%s undoable = %v, want %v", id, byID[id].Undoable, u)
		}
	}
	if got := byID["create-ok"].TargetTaskIdentifier; got == nil || *got != "KAN-1" {
		t.Errorf("identifier = %v", got)
	}
	if byID["create-notask"].TargetTaskIdentifier != nil {
		t.Error("identifier set without a task")
	}
	if got := byID["move-ok"].FromStepID; got == nil || *got != "s1" {
		t.Errorf("from_step_id = %v", got)
	}
	if byID["create-ok"].FromStepID != nil {
		t.Error("from_step_id on a create row")
	}
}

func TestListActivity_TaskReadFailureNeverFailsList(t *testing.T) {
	svc, store, c, _, fake := newActivityService(t, true)
	tid := "t1"
	seedApproved(t, store, c, "a", ActionCreateTask, time.Now().UTC(), func(r *ActivityRow) { r.TargetTaskID = &tid })
	fake.taskErr = errors.New("boom")
	page, err := svc.ListActivity(context.Background(), "ws-1", c.ID, ListActivityParams{})
	if err != nil || len(page.Rows) != 1 || page.Rows[0].TargetTaskIdentifier != nil {
		t.Fatalf("page = %+v err = %v", page, err)
	}
}

func TestListActivity_JSONCarriesIDsNotNames(t *testing.T) {
	svc, store, c, _, _ := newActivityService(t, true)
	u := "user-1"
	seedApproved(t, store, c, "a", ActionMessage, time.Now().UTC(), func(r *ActivityRow) { r.ActorUserID = &u; r.UndoneBy = &u })
	page, err := svc.ListActivity(context.Background(), "ws-1", c.ID, ListActivityParams{})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(page.Rows[0])
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	if m["actor_user_id"] != "user-1" || m["undone_by"] != "user-1" {
		t.Fatalf("ids missing: %v", m)
	}
	for k := range m {
		if strings.HasSuffix(k, "_name") || strings.HasSuffix(k, "_missing") {
			t.Errorf("display field %q in API", k)
		}
	}
}

func TestListActivity_PhaseOffRefuses(t *testing.T) {
	svc, _, c, _, _ := newActivityService(t, false)
	if _, err := svc.ListActivity(context.Background(), "ws-1", c.ID, ListActivityParams{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestActivitySummary_ZerosGroupsAndEarliest(t *testing.T) {
	svc, store, c, _, _ := newActivityService(t, true)
	ctx := context.Background()
	now := time.Now().UTC()
	seedActivity(t, store, c.ID, "old", ActionMove, ActivityProposed, now.AddDate(0, 0, -100))
	seedApproved(t, store, c, "ok", ActionCreateTask, now.Add(-time.Hour), nil)
	seedApproved(t, store, c, "edited", ActionCreateTask, now.Add(-time.Hour), func(r *ActivityRow) { r.Edited = true })
	seedActivity(t, store, c.ID, "rej", ActionCreateTask, ActivityRejected, now.Add(-time.Hour))
	seedActivity(t, store, c.ID, "und", ActionCreateTask, ActivityUndone, now.Add(-time.Hour))
	ref := validRow(c.ID)
	ref.ID, ref.ActionClass, ref.Outcome, ref.Authorization, ref.RefusalCount = "ref", ActionUnknown, ActivityRefused, AuthDenied, 3
	ref.CreatedAt, ref.UpdatedAt = now.Add(-time.Hour), now.Add(-time.Hour)
	if err := store.InsertActivity(ctx, store.db, ref); err != nil {
		t.Fatal(err)
	}
	sum, err := svc.ActivitySummary(ctx, c.ID, 30)
	if err != nil {
		t.Fatal(err)
	}
	ct := sum.Classes[ActionCreateTask]
	if ct.Approved != 2 || ct.ApprovedWithEdits != 1 || ct.Rejected != 1 || ct.Undone != 1 || ct.Proposed != 0 {
		t.Fatalf("create_task = %+v", ct)
	}
	for _, a := range []Action{ActionCreateTask, ActionStartAgent, ActionMessage, ActionMove, ActionResume, ActionStop} {
		if _, ok := sum.Classes[a]; !ok {
			t.Errorf("class %s missing", a)
		}
	}
	if sum.Classes[ActionMove].Proposed != 0 {
		t.Error("row outside the window counted")
	}
	if got := sum.Classes[ActionUnknown]; got.Refused != 3 {
		t.Fatalf("unknown = %+v", got)
	}
	if sum.EarliestRowAt == nil || sum.EarliestRowAt.After(now.AddDate(0, 0, -99)) {
		t.Fatalf("earliest = %v", sum.EarliestRowAt)
	}
	if sum.Days != 30 {
		t.Fatalf("days = %d", sum.Days)
	}
}

func TestActivitySummary_UnknownAbsentWithoutRowsAndMissingCoordinator(t *testing.T) {
	svc, _, c, _, _ := newActivityService(t, true)
	sum, err := svc.ActivitySummary(context.Background(), c.ID, 30)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := sum.Classes[ActionUnknown]; ok || sum.EarliestRowAt != nil {
		t.Fatalf("sum = %+v", sum)
	}
	if _, err := svc.ActivitySummary(context.Background(), "nope", 30); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestGetActivitySummary_ValidatesDaysAndScope(t *testing.T) {
	svc, _, c, az, _ := newActivityService(t, true)
	for _, d := range []int{0, 91, -1} {
		_, err := svc.GetActivitySummary(context.Background(), "ws-1", c.ID, d)
		assertFieldError(t, err, "days")
	}
	if _, err := svc.GetActivitySummary(context.Background(), "ws-1", c.ID, DefaultSummaryDays); err != nil {
		t.Fatal(err)
	}
	if az.scopes[len(az.scopes)-1] != authz.ScopeWorkspaceRead {
		t.Fatalf("scopes = %v", az.scopes)
	}
}

func TestActivityPage_ForToolDropsIdentitiesKeepsEverythingElse(t *testing.T) {
	svc, store, c, _, _ := newActivityService(t, true)
	u := "user-1"
	seedApproved(t, store, c, "a", ActionMessage, time.Now().UTC(), func(r *ActivityRow) { r.ActorUserID = &u; r.UndoneBy = &u })
	page, err := svc.ListActivity(context.Background(), "ws-1", c.ID, ListActivityParams{})
	if err != nil {
		t.Fatal(err)
	}
	var listKeys, toolKeys map[string]any
	raw, _ := json.Marshal(page.Rows[0])
	_ = json.Unmarshal(raw, &listKeys)
	raw, _ = json.Marshal(page.ForTool().Rows[0])
	_ = json.Unmarshal(raw, &toolKeys)
	for k := range listKeys {
		_, inTool := toolKeys[k]
		wantMissing := k == "actor_user_id" || k == "undone_by" || k == "coordinator_id" || k == "workspace_id"
		if inTool == wantMissing {
			t.Errorf("key %q: in tool = %v", k, inTool)
		}
	}
	for _, k := range []string{"created_at", "updated_at"} {
		if _, ok := toolKeys[k]; !ok {
			t.Errorf("tool row lacks %s", k)
		}
	}
}
