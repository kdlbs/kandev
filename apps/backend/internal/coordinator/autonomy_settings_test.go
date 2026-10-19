package coordinator

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func TestParseCostCeilingUSD(t *testing.T) {
	valid := map[string]int64{
		"0.01": 100, "0.1": 1000, "1": 10_000, "12.34": 123_400,
		"10000": 100_000_000, "10000.00": 100_000_000, "0.50": 5000, "100.5": 1_005_000,
	}
	for in, want := range valid {
		got, err := ParseCostCeilingUSD(in)
		if err != nil || got != want {
			t.Errorf("ParseCostCeilingUSD(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"0", "0.0", "0.00", "0.001", "10000.01", "10001", "99999.99", "100000", "1e3", "", "-1", "1.", ".5", " 1", "1,5", "abc"} {
		_, err := ParseCostCeilingUSD(in)
		assertFieldError(t, err, "cost_ceiling")
	}
}

func TestFormatCostCeilingUSD(t *testing.T) {
	for in, want := range map[int64]string{100: "0.01", 10_000: "1.00", 123_400: "12.34", 100_000_000: "10000.00", 1000: "0.10"} {
		if got := FormatCostCeilingUSD(in); got != want {
			t.Errorf("FormatCostCeilingUSD(%d) = %q, want %q", in, got, want)
		}
	}
}

const phase3Workspace = "ws-1"

type phase3Env struct {
	svc     *Service
	store   *Store
	c       *Coordinator
	kicks   []string
	kickErr error
	kickPan bool
	events  captureCoordinatorUpdated
}

func newPhase3Env(t *testing.T, phase3 bool) *phase3Env {
	t.Helper()
	store := newTestStore(t)
	agents := map[string]*settingsmodels.AgentProfile{"ap-1": {ID: "ap-1", WorkspaceID: phase3Workspace}}
	executors := map[string]*taskmodels.ExecutorProfile{"ep-1": {ID: "ep-1"}}
	svc := NewService(store, newValidatorForTest(agents, executors), &fakeWorkspaceAuthorizer{}, newTestLogger(t), WithPhase2(phase3), WithPhase3(phase3))
	env := &phase3Env{svc: svc, store: store}
	memBus := bus.NewMemoryEventBus(newTestLogger(t))
	t.Cleanup(memBus.Close)
	if _, err := memBus.Subscribe(events.CoordinatorUpdated, env.events.record); err != nil {
		t.Fatal(err)
	}
	svc.SetDecisionDeps(nil, nil, memBus)
	svc.SetKick(func(_ context.Context, id string) error {
		env.kicks = append(env.kicks, id)
		if env.kickPan {
			panic("kick exploded")
		}
		return env.kickErr
	})
	c := &Coordinator{WorkspaceID: phase3Workspace, Name: "Ops", AgentProfileID: "ap-1", ExecutorProfileID: "ep-1", Context: "ctx"}
	if err := store.CreateCoordinator(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	env.c = c
	return env
}

func (e *phase3Env) patch(t *testing.T, body string) (*Coordinator, error) {
	t.Helper()
	var req PatchCoordinatorRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("bad test body: %v", err)
	}
	return e.svc.PatchCoordinator(context.Background(), phase3Workspace, e.c.ID, req)
}

func (e *phase3Env) reload(t *testing.T) *Coordinator {
	t.Helper()
	got, err := e.store.GetCoordinatorByID(context.Background(), e.c.ID)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestPatchAutonomy_SetsAndClearsCeilingAndToggles(t *testing.T) {
	env := newPhase3Env(t, true)
	got, err := env.patch(t, `{"cost_ceiling_usd":"12.34"}`)
	if err != nil || got.CostCeilingSubcents == nil || *got.CostCeilingSubcents != 123_400 {
		t.Fatalf("set ceiling = %+v, %v", got, err)
	}
	if got, err = env.patch(t, `{"autonomy_enabled":true}`); err != nil || !got.AutonomyEnabled {
		t.Fatalf("autonomy on = %+v, %v", got, err)
	}
	if got, err = env.patch(t, `{"autonomy_enabled":false,"cost_ceiling_usd":null}`); err != nil || got.AutonomyEnabled || got.CostCeilingSubcents != nil {
		t.Fatalf("autonomy off + clear = %+v, %v", got, err)
	}
	stored := env.reload(t)
	if stored.AutonomyEnabled || stored.CostCeilingSubcents != nil {
		t.Fatalf("stored = %+v", stored)
	}
}

func TestPatchAutonomy_DoesNotTouchConfigRevisionOrConversation(t *testing.T) {
	env := newPhase3Env(t, true)
	conv := "conv-1"
	if _, err := env.store.db.Exec(env.store.db.Rebind(`UPDATE coordinators SET conversation_task_id = ?, config_revision = 4 WHERE id = ?`), conv, env.c.ID); err != nil {
		t.Fatal(err)
	}
	got, err := env.patch(t, `{"cost_ceiling_usd":"5","autonomy_enabled":true}`)
	if err != nil {
		t.Fatal(err)
	}
	if got.ConfigRevision != 4 || got.ConversationTaskID == nil || *got.ConversationTaskID != conv {
		t.Fatalf("config_revision/conversation changed: %+v", got)
	}
	if len(env.kicks) != 1 {
		t.Fatalf("kicks = %v, want 1", env.kicks)
	}
}

func TestPatchAutonomy_Interlock(t *testing.T) {
	env := newPhase3Env(t, true)
	_, err := env.patch(t, `{"autonomy_enabled":true}`)
	assertFieldError(t, err, "cost_ceiling")
	if _, err := env.patch(t, `{"cost_ceiling_usd":"3","autonomy_enabled":true}`); err != nil {
		t.Fatal(err)
	}
	_, err = env.patch(t, `{"cost_ceiling_usd":null}`)
	assertFieldError(t, err, "cost_ceiling")
	stored := env.reload(t)
	if !stored.AutonomyEnabled || stored.CostCeilingSubcents == nil || *stored.CostCeilingSubcents != 30_000 {
		t.Fatalf("rejected PATCH changed the row: %+v", stored)
	}
	if _, err := env.patch(t, `{"autonomy_enabled":false,"cost_ceiling_usd":null}`); err != nil {
		t.Fatalf("off and clear in one PATCH: %v", err)
	}
}

func TestPatchAutonomy_InvalidValuesAre400AndApplyNothing(t *testing.T) {
	env := newPhase3Env(t, true)
	cases := map[string]string{
		`{"cost_ceiling_usd":10}`:                           "cost_ceiling",
		`{"cost_ceiling_usd":"0"}`:                          "cost_ceiling",
		`{"cost_ceiling_usd":"0.001"}`:                      "cost_ceiling",
		`{"cost_ceiling_usd":"10000.01"}`:                   "cost_ceiling",
		`{"cost_ceiling_usd":"1e3"}`:                        "cost_ceiling",
		`{"autonomy_enabled":null}`:                         "autonomy_enabled",
		`{"autonomy_enabled":"true"}`:                       "autonomy_enabled",
		`{"autonomy_enabled":1}`:                            "autonomy_enabled",
		`{"name":"renamed","autonomy_enabled":null}`:        "autonomy_enabled",
		`{"name":"renamed","cost_ceiling_usd":"nope"}`:      "cost_ceiling",
		`{"name":"renamed","cost_ceiling_usd":{"a":1}}`:     "cost_ceiling",
		`{"name":"renamed","autonomy_enabled":[true]}`:      "autonomy_enabled",
		`{"cost_ceiling_usd":"5","autonomy_enabled":"yes"}`: "autonomy_enabled",
	}
	for body, field := range cases {
		_, err := env.patch(t, body)
		assertFieldError(t, err, field)
	}
	stored := env.reload(t)
	if stored.Name != "Ops" || stored.AutonomyEnabled || stored.CostCeilingSubcents != nil {
		t.Fatalf("a rejected PATCH changed the row: %+v", stored)
	}
	if len(env.kicks) != 0 || len(env.events.snapshot()) != 0 {
		t.Fatalf("rejected PATCH published/kicked: %v %v", env.kicks, env.events.snapshot())
	}
}

func TestPatchAutonomy_AbsentKeysLeaveColumnsUnchanged(t *testing.T) {
	env := newPhase3Env(t, true)
	if _, err := env.patch(t, `{"cost_ceiling_usd":"2","autonomy_enabled":true}`); err != nil {
		t.Fatal(err)
	}
	if _, err := env.patch(t, `{"name":"Renamed"}`); err != nil {
		t.Fatal(err)
	}
	stored := env.reload(t)
	if !stored.AutonomyEnabled || stored.CostCeilingSubcents == nil || *stored.CostCeilingSubcents != 20_000 || stored.Name != "Renamed" {
		t.Fatalf("stored = %+v", stored)
	}
}

func TestPatchAutonomy_PublishesAndKicksOnChangeOnly(t *testing.T) {
	env := newPhase3Env(t, true)
	if _, err := env.patch(t, `{"cost_ceiling_usd":"2","autonomy_enabled":true}`); err != nil {
		t.Fatal(err)
	}
	if len(env.kicks) != 1 || env.kicks[0] != env.c.ID {
		t.Fatalf("kicks = %v", env.kicks)
	}
	evs := env.events.snapshot()
	if len(evs) != 1 || !evs[0].AutonomyChanged || evs[0].CoordinatorID != env.c.ID || evs[0].WorkspaceID != phase3Workspace {
		t.Fatalf("events = %+v", evs)
	}
	if _, err := env.patch(t, `{"cost_ceiling_usd":"2.00","autonomy_enabled":true}`); err != nil {
		t.Fatal(err)
	}
	if _, err := env.patch(t, `{"name":"x"}`); err != nil {
		t.Fatal(err)
	}
	if len(env.kicks) != 1 || len(env.events.snapshot()) != 1 {
		t.Fatalf("an unchanged value published or kicked: %v %v", env.kicks, env.events.snapshot())
	}
	if _, err := env.patch(t, `{"cost_ceiling_usd":"3"}`); err != nil {
		t.Fatal(err)
	}
	if len(env.kicks) != 2 || len(env.events.snapshot()) != 2 {
		t.Fatalf("a changed ceiling must publish and kick: %v %v", env.kicks, env.events.snapshot())
	}
}

func TestPatchAutonomy_KickFailureDoesNotChangeResult(t *testing.T) {
	for name, set := range map[string]func(*phase3Env){
		"error": func(e *phase3Env) { e.kickErr = errors.New("boom") },
		"panic": func(e *phase3Env) { e.kickPan = true },
	} {
		t.Run(name, func(t *testing.T) {
			env := newPhase3Env(t, true)
			set(env)
			got, err := env.patch(t, `{"cost_ceiling_usd":"2","autonomy_enabled":true}`)
			if err != nil || !got.AutonomyEnabled {
				t.Fatalf("PATCH = %+v, %v", got, err)
			}
			if len(env.kicks) != 1 || !env.reload(t).AutonomyEnabled {
				t.Fatalf("kicks = %v", env.kicks)
			}
		})
	}
}

func TestPatchAutonomy_NilKickIsSafe(t *testing.T) {
	env := newPhase3Env(t, true)
	env.svc.SetKick(nil)
	if _, err := env.patch(t, `{"cost_ceiling_usd":"2","autonomy_enabled":true}`); err != nil {
		t.Fatal(err)
	}
}

func TestPatchAutonomy_IgnoredWhilePhase3NotEffective(t *testing.T) {
	env := newPhase3Env(t, false)
	got, err := env.patch(t, `{"cost_ceiling_usd":"not money","autonomy_enabled":"maybe","name":"kept"}`)
	if err != nil {
		t.Fatalf("PATCH = %v, want 200 with the fields ignored", err)
	}
	stored := env.reload(t)
	if got.Name != "kept" || stored.AutonomyEnabled || stored.CostCeilingSubcents != nil {
		t.Fatalf("stored = %+v", stored)
	}
	if _, err := env.patch(t, `{"autonomy_enabled":true,"cost_ceiling_usd":"2"}`); err != nil {
		t.Fatal(err)
	}
	if stored = env.reload(t); stored.AutonomyEnabled || stored.CostCeilingSubcents != nil {
		t.Fatalf("valid fields were applied while not effective: %+v", stored)
	}
	if len(env.kicks) != 0 {
		t.Fatalf("kicks = %v", env.kicks)
	}
}

func TestPatchAutonomy_OffSupersedesPendingWakes(t *testing.T) {
	env := newPhase3Env(t, true)
	ctx := context.Background()
	if _, err := env.patch(t, `{"cost_ceiling_usd":"2","autonomy_enabled":true}`); err != nil {
		t.Fatal(err)
	}
	insertWake(t, env.store, env.c, "w-pending", "pending")
	insertWake(t, env.store, env.c, "w-delivered", "delivered")
	insertTurn(t, env.store, env.c, "t-open", nil)
	if _, err := env.patch(t, `{"autonomy_enabled":false}`); err != nil {
		t.Fatal(err)
	}
	if got := wakeStatus(t, env.store, "w-pending"); got != "superseded" {
		t.Fatalf("pending wake = %q", got)
	}
	if got := wakeStatus(t, env.store, "w-delivered"); got != "delivered" {
		t.Fatalf("delivered wake = %q", got)
	}
	if !turnExists(t, env.store, "t-open") {
		t.Fatal("open turn row must be untouched")
	}
	// A wake returned to pending after autonomy went off is superseded by a
	// later off PATCH even though the row already reads off.
	if _, err := env.store.db.ExecContext(ctx, `UPDATE coordinator_wakes SET status = 'pending' WHERE id = 'w-delivered'`); err != nil {
		t.Fatal(err)
	}
	if _, err := env.patch(t, `{"autonomy_enabled":false}`); err != nil {
		t.Fatal(err)
	}
	if got := wakeStatus(t, env.store, "w-delivered"); got != "superseded" {
		t.Fatalf("pending wake on an already-off row = %q", got)
	}
	if len(env.kicks) != 2 {
		t.Fatalf("an unchanged off PATCH must not kick: %v", env.kicks)
	}
}

func TestPatchAutonomy_OnDoesNotSupersedeWakes(t *testing.T) {
	env := newPhase3Env(t, true)
	insertWake(t, env.store, env.c, "w1", "pending")
	if _, err := env.patch(t, `{"cost_ceiling_usd":"2","autonomy_enabled":true}`); err != nil {
		t.Fatal(err)
	}
	if got := wakeStatus(t, env.store, "w1"); got != "pending" {
		t.Fatalf("wake = %q", got)
	}
}

func TestCoordinatorDTO_Phase3Fields(t *testing.T) {
	env := newPhase3Env(t, true)
	c := *env.c
	raw := func(c *Coordinator, p3 bool) map[string]json.RawMessage {
		dto := NewCoordinatorDTO(c)
		if p3 {
			dto.WithAutonomy(c)
		}
		b, err := json.Marshal(dto)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]json.RawMessage
		_ = json.Unmarshal(b, &m)
		return m
	}
	m := raw(&c, true)
	if string(m["autonomy_enabled"]) != "false" || string(m["cost_ceiling_usd"]) != "null" {
		t.Fatalf("unset fields = %s %s", m["autonomy_enabled"], m["cost_ceiling_usd"])
	}
	sub := int64(123_400)
	c.AutonomyEnabled, c.CostCeilingSubcents = true, &sub
	m = raw(&c, true)
	if string(m["autonomy_enabled"]) != "true" || string(m["cost_ceiling_usd"]) != `"12.34"` {
		t.Fatalf("set fields = %s %s", m["autonomy_enabled"], m["cost_ceiling_usd"])
	}
	m = raw(&c, false)
	if _, ok := m["autonomy_enabled"]; ok {
		t.Fatal("autonomy_enabled present while phase 3 is not effective")
	}
	if _, ok := m["cost_ceiling_usd"]; ok {
		t.Fatal("cost_ceiling_usd present while phase 3 is not effective")
	}
}

func TestActivityItem_UnattendedTurnIDOnlyWhenPhase3Effective(t *testing.T) {
	for _, on := range []bool{false, true} {
		env := newPhase3Env(t, on)
		row := validRow(env.c.ID)
		if err := env.store.InsertActivity(context.Background(), env.store.db, row); err != nil {
			t.Fatal(err)
		}
		if _, err := env.store.db.Exec(`UPDATE coordinator_activity SET unattended_turn_id = 'turn-9'`); err != nil {
			t.Fatal(err)
		}
		rows, err := env.store.ListActivityRows(context.Background(), env.c.ID, "", nil, 10)
		if err != nil || len(rows) != 1 {
			t.Fatalf("rows = %v, %v", rows, err)
		}
		items := env.svc.enrichActivity(context.Background(), env.c.ID, rows)
		b, _ := json.Marshal(items[0])
		var m map[string]json.RawMessage
		_ = json.Unmarshal(b, &m)
		got, present := m["unattended_turn_id"]
		if present != on || (on && string(got) != `"turn-9"`) {
			t.Fatalf("phase3=%v: unattended_turn_id present=%v value=%s", on, present, got)
		}
	}
}
