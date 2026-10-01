package dream

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/coordinator/replay"
	"github.com/kandev/kandev/internal/db"
)

type fakeEpisode struct {
	mu        sync.Mutex
	answer    string
	promptErr error
	block     chan struct{}
	archived  []string
	message   string
}

func (e *fakeEpisode) CreateTask(context.Context, *coordinator.Coordinator) (string, error) {
	return "task-1", nil
}
func (e *fakeEpisode) CreateSession(context.Context, string) (string, error) { return "sess-1", nil }
func (e *fakeEpisode) Prompt(ctx context.Context, _, _, msg string) (string, error) {
	e.mu.Lock()
	e.message = msg
	e.mu.Unlock()
	if e.block != nil {
		select {
		case <-e.block:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return e.answer, e.promptErr
}
func (e *fakeEpisode) Archive(_ context.Context, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.archived = append(e.archived, id)
	return nil
}

type fakeReplay struct {
	mu    sync.Mutex
	calls []replay.Request
}

func (r *fakeReplay) Run(_ context.Context, req replay.Request) (replay.Result, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, req)
	return replay.Result{RowID: "rr-" + req.ItemID, Verdict: replay.VerdictImprovement, Guard: replay.GuardPass}, nil
}

type fakeConditions struct {
	mu          sync.Mutex
	containment bool
	measurable  bool
	atCeiling   bool
}

func (c *fakeConditions) ContainmentOK(context.Context, *coordinator.Coordinator) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.containment
}
func (c *fakeConditions) Spend(context.Context, *coordinator.Coordinator, time.Time) (bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.measurable, c.atCeiling
}
func (c *fakeConditions) Model(context.Context, *coordinator.Coordinator) string { return "m" }

type fixture struct {
	t     *testing.T
	db    *sqlx.DB
	store *coordinator.Store
	coord *coordinator.Coordinator
	now   time.Time
	ep    *fakeEpisode
	rp    *fakeReplay
	cond  *fakeConditions
	sch   *Scheduler
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dream.db")
	wc, err := db.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = wc.Close() })
	rc, err := db.OpenSQLiteReader(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rc.Close() })
	w := sqlx.NewDb(wc, "sqlite3")
	store, err := coordinator.NewStore(w, sqlx.NewDb(rc, "sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	c := &coordinator.Coordinator{
		WorkspaceID: "ws-1", Name: "C", AgentProfileID: "a", ExecutorProfileID: "e",
		TaskAgentProfileID: "ta", TaskExecutorProfileID: "te",
	}
	if err := store.CreateCoordinator(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, db: w, store: store, coord: c, now: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC),
		ep: &fakeEpisode{}, rp: &fakeReplay{}, cond: &fakeConditions{containment: true, measurable: true}}
	f.exec(`UPDATE coordinators SET autonomy_enabled = 1 WHERE id = ?`, c.ID)
	if err := store.SetShadowDreamEnabled(context.Background(), c.WorkspaceID, c.ID, true); err != nil {
		t.Fatal(err)
	}
	f.sch = New(Deps{Store: store, Episode: f.ep, Replay: f.rp, Conditions: f.cond,
		Clock: func() time.Time { return f.now }, Bound: 5 * time.Second, Refresh: time.Hour})
	t.Cleanup(f.sch.Stop)
	return f
}

func (f *fixture) exec(q string, args ...any) {
	f.t.Helper()
	if _, err := f.db.Exec(f.db.Rebind(q), args...); err != nil {
		f.t.Fatalf("exec %q: %v", q, err)
	}
}

func (f *fixture) seedEvidence(turns int) []string {
	f.t.Helper()
	var ids []string
	for i := 0; i < turns; i++ {
		id := "t" + string(rune('a'+i))
		at := f.now.Add(-time.Duration(turns-i) * time.Hour)
		f.exec(`INSERT INTO coordinator_turns (id, coordinator_id, session_id, session_turn_id, "trigger", started_at, finished_at, verdict)
			VALUES (?, ?, 's', ?, 'wake', ?, ?, 'ok')`, id, f.coord.ID, id, at, at.Add(time.Minute))
		ids = append(ids, id)
	}
	f.exec(`INSERT INTO coordinator_outcomes (proposal_id, coordinator_id, kind, decision, decided_at, graded_at)
		VALUES ('p1', ?, 'create_task', 'approved', ?, ?)`, f.coord.ID, f.now.Add(-time.Hour), f.now)
	return ids
}

func (f *fixture) dreams() []coordinator.Dream {
	rows, err := f.store.ListDreams(context.Background(), f.coord.ID, nil, 20)
	if err != nil {
		f.t.Fatal(err)
	}
	return rows
}

func (f *fixture) waitFinished() coordinator.Dream {
	f.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if rows := f.dreams(); len(rows) == 1 && rows[0].Status != coordinator.DreamRunning {
			return rows[0]
		}
		time.Sleep(5 * time.Millisecond)
	}
	f.t.Fatal("dream did not finish")
	return coordinator.Dream{}
}

func answerJSON(a, b string) string {
	return `{"items":[{"kind":"note_add","text":"Ask first","cited_turn_ids":["` + a + `","` + b + `"]},` +
		`{"kind":"standing_order_retire","text":"old","target_id":"missing","cited_turn_ids":["` + a + `","` + b + `"]}],` +
		`"considered":["raise the ceiling"]}`
}

func TestTick_RunsOneDreamToPartialReport(t *testing.T) {
	f := newFixture(t)
	ids := f.seedEvidence(6)
	f.ep.answer = "```json\n" + answerJSON(ids[0], ids[1]) + "\n```"

	f.sch.Tick(context.Background(), f.coord.ID)
	got := f.waitFinished()

	if got.Status != coordinator.DreamPartial {
		t.Fatalf("status = %s (%s), want partial: one item passed, one is bad_target", got.Status, got.Reason)
	}
	items, _ := f.store.ListDreamItems(context.Background(), got.ID)
	if len(items) != 2 || items[0].Gate != GatePass || items[1].Gate != GateBadTarget {
		t.Fatalf("items = %+v", items)
	}
	if items[0].Verdict != replay.VerdictImprovement || items[0].ReplayID == "" || items[1].ReplayID != "" {
		t.Fatalf("only the passing item is replayed: %+v", items)
	}
	if len(f.rp.calls) != 1 {
		t.Fatalf("replays = %d, want 1", len(f.rp.calls))
	}
	if got.EpisodeTaskID != "task-1" || got.EpisodeSessionID != "sess-1" || len(got.Considered) != 1 {
		t.Fatalf("row = %+v", got)
	}

	// The next tick archives the episode task once and starts no second dream (24h spacing).
	f.sch.Tick(context.Background(), f.coord.ID)
	f.sch.Tick(context.Background(), f.coord.ID)
	if len(f.ep.archived) != 1 || f.ep.archived[0] != "task-1" {
		t.Fatalf("archived = %v", f.ep.archived)
	}
	if rows := f.dreams(); len(rows) != 1 {
		t.Fatalf("dreams = %d, want 1", len(rows))
	}
}

func TestTick_AdmissionConditionsStoreNothing(t *testing.T) {
	cases := map[string]func(f *fixture){
		"shadow off":      func(f *fixture) { _ = f.store.SetShadowDreamEnabled(context.Background(), "ws-1", f.coord.ID, false) },
		"autonomy off":    func(f *fixture) { f.exec(`UPDATE coordinators SET autonomy_enabled = 0 WHERE id = ?`, f.coord.ID) },
		"paused":          func(f *fixture) { f.exec(`UPDATE coordinators SET paused_at = ? WHERE id = ?`, f.now, f.coord.ID) },
		"containment":     func(f *fixture) { f.cond.containment = false },
		"spend unknown":   func(f *fixture) { f.cond.measurable = false },
		"at ceiling":      func(f *fixture) { f.cond.atCeiling = true },
		"too few turns":   func(f *fixture) { f.exec(`DELETE FROM coordinator_turns WHERE id = 'ta'`) },
		"no decision yet": func(f *fixture) { f.exec(`DELETE FROM coordinator_outcomes`) },
	}
	for name, breakIt := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.seedEvidence(5)
			breakIt(f)
			f.sch.Tick(context.Background(), f.coord.ID)
			f.sch.Stop()
			if rows := f.dreams(); len(rows) != 0 {
				t.Fatalf("rows = %+v, want none", rows)
			}
			if running, _ := f.store.RunningDream(context.Background(), f.coord.ID); running != nil {
				t.Fatal("a lease row was stored")
			}
		})
	}
}

func TestTick_BadOutputFailsAndLateFinishChangesNothing(t *testing.T) {
	f := newFixture(t)
	f.seedEvidence(5)
	f.ep.answer = "not json"
	f.sch.Tick(context.Background(), f.coord.ID)
	got := f.waitFinished()
	if got.Status != coordinator.DreamFailed || got.Reason != ReasonBadOutput {
		t.Fatalf("got %s/%s, want failed/bad_output", got.Status, got.Reason)
	}
}

func TestTick_RunErrorAndTimeout(t *testing.T) {
	f := newFixture(t)
	f.seedEvidence(5)
	f.ep.promptErr = errors.New("boom")
	f.sch.Tick(context.Background(), f.coord.ID)
	if got := f.waitFinished(); got.Reason != ReasonRunError {
		t.Fatalf("reason = %s", got.Reason)
	}

	g := newFixture(t)
	g.seedEvidence(5)
	g.ep.block = make(chan struct{})
	g.sch.d.Bound = 50 * time.Millisecond
	g.sch.Tick(context.Background(), g.coord.ID)
	if got := g.waitFinished(); got.Reason != ReasonTimeout {
		t.Fatalf("reason = %s, want timeout", got.Reason)
	}
}

func TestStopCoordinator_FailsRowAsPausedAndCancels(t *testing.T) {
	f := newFixture(t)
	f.seedEvidence(5)
	f.ep.block = make(chan struct{})
	f.sch.Tick(context.Background(), f.coord.ID)
	waitFor(t, func() bool {
		r, _ := f.store.RunningDream(context.Background(), f.coord.ID)
		return r != nil && r.EpisodeSessionID != ""
	})

	if err := f.sch.StopCoordinator(context.Background(), f.coord.ID); err != nil {
		t.Fatal(err)
	}
	got := f.waitFinished()
	if got.Status != coordinator.DreamFailed || got.Reason != ReasonPaused {
		t.Fatalf("got %s/%s, want failed/paused", got.Status, got.Reason)
	}
	// Cleanup archives the episode of the stopped row, paused or not.
	f.sch.Tick(context.Background(), f.coord.ID)
	if len(f.ep.archived) != 1 {
		t.Fatalf("archived = %v", f.ep.archived)
	}
}

func TestTick_ExpiredLeaseBecomesLeaseLostAndIsCleanedUp(t *testing.T) {
	f := newFixture(t)
	d := coordinator.Dream{ID: "old", CoordinatorID: f.coord.ID, WindowStart: f.now.Add(-48 * time.Hour), WindowEnd: f.now.Add(-24 * time.Hour),
		InputHash: "h", StartedAt: f.now.Add(-time.Hour)}
	if ok, err := f.store.InsertRunningDream(context.Background(), d); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if _, err := f.store.SetDreamEpisodeTask(context.Background(), "old", "task-old"); err != nil {
		t.Fatal(err)
	}
	f.sch.Tick(context.Background(), f.coord.ID)
	got, err := f.store.GetDream(context.Background(), f.coord.ID, "old")
	if err != nil || got.Status != coordinator.DreamFailed || got.Reason != ReasonLeaseLost {
		t.Fatalf("got %+v %v", got, err)
	}
	if len(f.ep.archived) != 1 || f.ep.archived[0] != "task-old" {
		t.Fatalf("archived = %v (a task id with no session id archives the task only)", f.ep.archived)
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not reached")
}
