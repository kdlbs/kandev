package coordinator

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	svcpkg "github.com/kandev/kandev/internal/task/service"
)

var readNow = time.Date(2026, 9, 29, 9, 20, 0, 0, time.UTC)

type readFixture struct {
	*admitFixture
	h *Handlers
}

func newReadFixture(t *testing.T) *readFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	f := &readFixture{admitFixture: newAdmitFixture(t)}
	f.env.store.now = func() time.Time { return readNow }
	f.h = NewHandlers(f.env.svc, newTestLogger(t))
	return f
}

func (f *readFixture) params(cid string) gin.Params {
	p := gin.Params{{Key: "id", Value: phase3Workspace}, {Key: "cid", Value: cid}}
	return p
}

func (f *readFixture) autonomy(t *testing.T) (int, map[string]any) {
	t.Helper()
	rec := runHandler(f.h.httpGetAutonomy, http.MethodGet, "/x", "", f.params(f.env.c.ID))
	return rec.Code, decodeRec(t, rec)
}

func decodeRec(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return m
}

func (f *readFixture) insertSettled(t *testing.T, id string, started time.Time, sessionTurn, outcome string) {
	t.Helper()
	var st any
	if sessionTurn != "" {
		st = sessionTurn
	}
	mustExec(t, f.env.store, `INSERT INTO coordinator_unattended_turns
		(id, coordinator_id, conversation_task_id, session_id, session_turn_id, wake_count, start_ceiling_subcents, started_at, outcome, finished_at)
		VALUES (?, ?, ?, ?, ?, 1, 500, ?, ?, ?)`, id, f.env.c.ID, ceilingConvTask, ceilingSession, st, started, outcome, started.Add(time.Minute))
}

func TestAutonomyRead_AdmittedShape(t *testing.T) {
	f := newReadFixture(t)
	code, m := f.autonomy(t)
	if code != http.StatusOK {
		t.Fatalf("status = %d body=%v", code, m)
	}
	adm := m["admission"].(map[string]any)
	if adm["ok"] != true || adm["reason"] != nil {
		t.Fatalf("admission = %v", adm)
	}
	if m["server_time"] != "2026-09-29T09:20:00Z" || m["autonomy_enabled"] != true {
		t.Fatalf("body = %v", m)
	}
	if got := len(m["containment"].(map[string]any)["conditions"].([]any)); got != 4 {
		t.Fatalf("conditions = %d, want 4", got)
	}
	if m["last_turn"] != nil || m["last_woke_at"] != nil || m["oldest_pending_at"] != nil || m["pending_wakes"].(float64) != 0 {
		t.Fatalf("empty coordinator = %v", m)
	}
}

func TestAutonomyRead_AutonomyOffHasNoAdmission(t *testing.T) {
	f := newReadFixture(t)
	mustExec(t, f.env.store, `UPDATE coordinators SET autonomy_enabled = ? WHERE id = ?`, false, f.env.c.ID)
	_, m := f.autonomy(t)
	if _, has := m["admission"]; has || m["autonomy_enabled"] != false {
		t.Fatalf("off body = %v", m)
	}
}

func TestAutonomyRead_AutonomyOffRaceAnswersOffWithoutAdmission(t *testing.T) {
	f := newReadFixture(t)
	mustExec(t, f.env.store, `UPDATE coordinators SET autonomy_enabled = ? WHERE id = ?`, false, f.env.c.ID)
	out := &AutonomyReadDTO{AutonomyEnabled: true}
	stale := &Coordinator{ID: f.env.c.ID, AutonomyEnabled: true}
	if err := f.env.svc.fillAdmission(context.Background(), stale, out); err != nil {
		t.Fatal(err)
	}
	if out.AutonomyEnabled || out.Admission != nil {
		t.Fatalf("race result = %+v", out)
	}
}

func TestAutonomyRead_HeldReasonsCarryDetailAndUntilOnlyForCooldown(t *testing.T) {
	f := newReadFixture(t)
	f.fc.mode = "disabled"
	_, m := f.autonomy(t)
	adm := m["admission"].(map[string]any)
	if adm["ok"] != false || adm["reason"] != "containment" || adm["detail"] != "auth_enabled" {
		t.Fatalf("containment admission = %v", adm)
	}
	if _, has := adm["until"]; has {
		t.Fatalf("until on a non-cooldown reason: %v", adm)
	}
	f.fc.mode = ""
	*f.fc = *contained()
	finished := readNow.Add(-4 * time.Minute)
	f.insertSettled(t, "done", finished.Add(-time.Minute), "st-x", "completed")
	_, m = f.autonomy(t)
	adm = m["admission"].(map[string]any)
	if adm["reason"] != "cooldown" || adm["until"] != "2026-09-29T09:21:00Z" {
		t.Fatalf("cooldown admission = %v", adm)
	}
}

func TestAutonomyRead_AdmitReadErrorIs500NotAHold(t *testing.T) {
	f := newReadFixture(t)
	f.reader.pendingErr = errors.New("boom")
	code, m := f.autonomy(t)
	if code != http.StatusInternalServerError || m["error"] != "read_error" {
		t.Fatalf("status = %d body = %v", code, m)
	}
	if _, has := m["admission"]; has {
		t.Fatalf("partial body: %v", m)
	}
}

func TestAutonomyRead_SpendUnmeasuredIsAHoldNot500(t *testing.T) {
	f := newReadFixture(t)
	f.ledger.sumFn = func(spendCall) (taskmodels.UsageSum, error) { return taskmodels.UsageSum{}, errors.New("boom") }
	code, m := f.autonomy(t)
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	adm := m["admission"].(map[string]any)
	if adm["reason"] != "spend_unmeasured" || adm["detail"] != "" {
		t.Fatalf("admission = %v", adm)
	}
	sp := m["spend"].(map[string]any)
	if sp["measurable"] != false || sp["degraded"] != false || sp["window_subcents"] != nil ||
		sp["mean_daily_subcents_7d"] != nil || sp["mean_known"] != false {
		t.Fatalf("failed spend row = %v", sp)
	}
}

func TestAutonomyRead_SpendThreeRows(t *testing.T) {
	f := newReadFixture(t)
	mustExec(t, f.env.store, `UPDATE coordinators SET cost_ceiling_subcents = ? WHERE id = ?`, 100000, f.env.c.ID)
	windowOnly := func(c spendCall) bool { return c.To.Sub(c.From) <= spendWindow }
	f.ledger.sumFn = func(c spendCall) (taskmodels.UsageSum, error) {
		if windowOnly(c) {
			return taskmodels.UsageSum{CostSubcents: 64000}, nil
		}
		return taskmodels.UsageSum{CostSubcents: 7 * 58000}, nil
	}
	_, m := f.autonomy(t)
	sp := m["spend"].(map[string]any)
	if sp["measurable"] != true || sp["degraded"] != false || sp["window_subcents"].(float64) != 64000 ||
		sp["mean_daily_subcents_7d"].(float64) != 58000 || sp["mean_known"] != true || sp["ceiling_subcents"].(float64) != 100000 {
		t.Fatalf("measurable row = %v", sp)
	}

	f.ledger.sumFn = func(c spendCall) (taskmodels.UsageSum, error) {
		if windowOnly(c) {
			return taskmodels.UsageSum{CostSubcents: 12, HasUnpriced: true}, nil
		}
		return taskmodels.UsageSum{CostSubcents: 7 * 100}, nil
	}
	_, m = f.autonomy(t)
	sp = m["spend"].(map[string]any)
	if sp["measurable"] != false || sp["degraded"] != true || sp["window_subcents"] != nil ||
		sp["mean_daily_subcents_7d"].(float64) != 100 || sp["mean_known"] != true {
		t.Fatalf("unpriced row = %v", sp)
	}
}

func TestAutonomyRead_FailedMeanReadIsNeverZero(t *testing.T) {
	f := newReadFixture(t)
	f.ledger.sumFn = func(c spendCall) (taskmodels.UsageSum, error) {
		if c.To.Sub(c.From) > spendWindow {
			return taskmodels.UsageSum{}, errors.New("boom")
		}
		return taskmodels.UsageSum{CostSubcents: 10}, nil
	}
	_, m := f.autonomy(t)
	sp := m["spend"].(map[string]any)
	if sp["measurable"] != true || sp["mean_known"] != false || sp["mean_daily_subcents_7d"] != nil {
		t.Fatalf("spend = %v", sp)
	}
}

func TestAutonomyRead_UnknownCoordinatorAndOtherWorkspaceAre404(t *testing.T) {
	f := newReadFixture(t)
	for name, params := range map[string]gin.Params{
		"unknown id":      f.params("nope"),
		"other workspace": {{Key: "id", Value: "other-ws"}, {Key: "cid", Value: f.env.c.ID}},
	} {
		rec := runHandler(f.h.httpGetAutonomy, http.MethodGet, "/x", "", params)
		if rec.Code != http.StatusNotFound || decodeRec(t, rec)["error"] != "coordinator_not_found" {
			t.Errorf("%s: status = %d body=%s", name, rec.Code, rec.Body.String())
		}
	}
}

func TestAutonomyRead_ReaderWithoutScopeIs403(t *testing.T) {
	f := newReadFixture(t)
	f.env.svc.authz = &fakeWorkspaceAuthorizer{err: svcpkg.ErrForbidden}
	rec := runHandler(f.h.httpGetAutonomy, http.MethodGet, "/x", "", f.params(f.env.c.ID))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestAutonomyRead_StopStateBoundaryIsStrictlyMoreThanFiveMinutes(t *testing.T) {
	cases := []struct {
		name     string
		age      time.Duration
		finished bool
		want     any
		hasReq   bool
	}{
		{"four minutes", 4 * time.Minute, false, nil, true},
		{"exactly five minutes", 5 * time.Minute, false, nil, true},
		{"five minutes and a second", 5*time.Minute + time.Second, false, "stop_failing", true},
		{"settled turn never fails", 30 * time.Minute, true, nil, true},
		{"no stop requested", 0, false, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newReadFixture(t)
			f.insertTurn(t, "open", ceilingSession, "st-open", 500)
			if tc.hasReq {
				mustExec(t, f.env.store, `UPDATE coordinator_unattended_turns SET stop_requested_at = ? WHERE id = 'open'`, readNow.Add(-tc.age))
			}
			if tc.finished {
				mustExec(t, f.env.store, `UPDATE coordinator_unattended_turns SET outcome = 'stopped_at_ceiling', finished_at = ? WHERE id = 'open'`, readNow)
			}
			_, m := f.autonomy(t)
			lt := m["last_turn"].(map[string]any)
			if lt["stop_state"] != tc.want {
				t.Fatalf("stop_state = %v, want %v (turn %v)", lt["stop_state"], tc.want, lt)
			}
		})
	}
}

func TestAutonomyRead_LastTurnIsNewestByStartedAtWhateverItsOutcome(t *testing.T) {
	f := newReadFixture(t)
	f.insertSettled(t, "older", readNow.Add(-3*time.Hour), "st-1", "completed")
	f.insertSettled(t, "newer", readNow.Add(-2*time.Hour), "", "send_failed")
	_, m := f.autonomy(t)
	lt := m["last_turn"].(map[string]any)
	if lt["id"] != "newer" || lt["outcome"] != "send_failed" || lt["cost_subcents"] != nil {
		t.Fatalf("last_turn = %v", lt)
	}
	if m["last_woke_at"] != "2026-09-29T06:20:00Z" {
		t.Fatalf("last_woke_at = %v, want the accepted turn's start", m["last_woke_at"])
	}
}

func TestAutonomyRead_PendingPairAgrees(t *testing.T) {
	f := newReadFixture(t)
	insertWakeAt(t, f.env.store, f.env.c, "w1", "pending", readNow.Add(-20*time.Minute))
	insertWakeAt(t, f.env.store, f.env.c, "w2", "pending", readNow.Add(-40*time.Minute))
	insertWakeAt(t, f.env.store, f.env.c, "w3", "delivered", readNow.Add(-90*time.Minute))
	_, m := f.autonomy(t)
	if m["pending_wakes"].(float64) != 2 || m["oldest_pending_at"] != "2026-09-29T08:40:00Z" {
		t.Fatalf("pending = %v / %v", m["pending_wakes"], m["oldest_pending_at"])
	}
}

func TestAutonomyRead_ContainmentUnreadableIsAHoldNotAnError(t *testing.T) {
	f := newReadFixture(t)
	f.fc.execErr = errors.New("boom")
	code, m := f.autonomy(t)
	adm := m["admission"].(map[string]any)
	if code != http.StatusOK || adm["reason"] != "containment" || adm["detail"] != "executor_isolated" {
		t.Fatalf("status = %d admission = %v", code, adm)
	}
	conds := m["containment"].(map[string]any)["conditions"].([]any)
	first := conds[0].(map[string]any)
	if first["met"] != false || first["detail"] != "unreadable" {
		t.Fatalf("first condition = %v", first)
	}
}

func TestAutonomyRead_ReadDoesNotMoveTheContainmentCounter(t *testing.T) {
	f := newReadFixture(t)
	f.fc.mode = "disabled"
	before := containmentFailedCount("auth_enabled")
	f.autonomy(t)
	if got := containmentFailedCount("auth_enabled"); got != before {
		t.Fatalf("counter moved %d -> %d", before, got)
	}
}

func TestRunRead_ReturnsWholeRowAndWakesInMessageOrder(t *testing.T) {
	f := newReadFixture(t)
	f.insertTurn(t, "run-1", ceilingSession, "st-1", 500)
	f.conv.tasks["t-a"] = &taskmodels.Task{ID: "t-a", Title: "A full title that is long", Identifier: "KAN-418"}
	mustExec(t, f.env.store, `UPDATE coordinator_unattended_turns SET wake_count = 3, denied_permissions = 1 WHERE id = 'run-1'`)
	mustExec(t, f.env.store, `INSERT INTO coordinator_wakes (id, coordinator_id, workspace_id, task_id, kind, episode_key, status, turn_id, created_at, updated_at)
		VALUES ('w-b', ?, ?, 't-gone', 'stalled', 'e2', 'delivered', 'run-1', ?, ?),
		       ('w-a', ?, ?, 't-a', 'question', 'e1', 'delivered', 'run-1', ?, ?),
		       ('w-c', ?, ?, 't-a', 'review', 'e3', 'delivered', 'run-1', ?, ?)`,
		f.env.c.ID, phase3Workspace, readNow.Add(-2*time.Hour), readNow,
		f.env.c.ID, phase3Workspace, readNow.Add(-3*time.Hour), readNow,
		f.env.c.ID, phase3Workspace, readNow.Add(-2*time.Hour), readNow)
	rec := runHandler(f.h.httpGetRun, http.MethodGet, "/x", "", append(f.params(f.env.c.ID), gin.Param{Key: "runId", Value: "run-1"}))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	m := decodeRec(t, rec)
	if m["wake_count"].(float64) != 3 || m["denied_permissions"].(float64) != 1 || m["outcome"] != nil || m["finished_at"] != nil {
		t.Fatalf("run = %v", m)
	}
	wakes := m["wakes"].([]any)
	var ids []string
	for _, w := range wakes {
		ids = append(ids, w.(map[string]any)["id"].(string))
	}
	if len(ids) != 3 || ids[0] != "w-a" || ids[1] != "w-b" || ids[2] != "w-c" {
		t.Fatalf("wake order = %v", ids)
	}
	first := wakes[0].(map[string]any)
	if first["task_identifier"] != "KAN-418" || first["task_title"] != "A full title that is long" || first["kind"] != "question" {
		t.Fatalf("first wake = %v", first)
	}
	gone := wakes[1].(map[string]any)
	if gone["task_identifier"] != nil || gone["task_title"] != nil || gone["task_id"] != "t-gone" {
		t.Fatalf("unreadable task wake = %v", gone)
	}
}

func TestRunRead_NoWakesIsEmptyArray(t *testing.T) {
	f := newReadFixture(t)
	f.insertTurn(t, "run-1", ceilingSession, "st-1", 500)
	rec := runHandler(f.h.httpGetRun, http.MethodGet, "/x", "", append(f.params(f.env.c.ID), gin.Param{Key: "runId", Value: "run-1"}))
	if got := decodeRec(t, rec)["wakes"]; got == nil || len(got.([]any)) != 0 {
		t.Fatalf("wakes = %v, want []", got)
	}
}

func TestRunRead_StopStateMatchesTheAutonomyRead(t *testing.T) {
	f := newReadFixture(t)
	f.insertTurn(t, "run-1", ceilingSession, "st-1", 500)
	mustExec(t, f.env.store, `UPDATE coordinator_unattended_turns SET stop_requested_at = ? WHERE id = 'run-1'`, readNow.Add(-6*time.Minute))
	rec := runHandler(f.h.httpGetRun, http.MethodGet, "/x", "", append(f.params(f.env.c.ID), gin.Param{Key: "runId", Value: "run-1"}))
	if decodeRec(t, rec)["stop_state"] != "stop_failing" {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestRunRead_NotFoundCasesAreIndistinguishable(t *testing.T) {
	f := newReadFixture(t)
	other := &Coordinator{WorkspaceID: phase3Workspace, Name: "Other", AgentProfileID: "ap-1", ExecutorProfileID: "ep-1", Context: "c"}
	if err := f.env.store.CreateCoordinator(context.Background(), other); err != nil {
		t.Fatal(err)
	}
	mustExec(t, f.env.store, `INSERT INTO coordinator_unattended_turns
		(id, coordinator_id, conversation_task_id, session_id, wake_count, start_ceiling_subcents, started_at)
		VALUES ('theirs', ?, 'c', 's', 1, 1, ?)`, other.ID, readNow)
	for name, runID := range map[string]string{"missing": "nope", "another coordinator": "theirs"} {
		rec := runHandler(f.h.httpGetRun, http.MethodGet, "/x", "", append(f.params(f.env.c.ID), gin.Param{Key: "runId", Value: runID}))
		if rec.Code != http.StatusNotFound || decodeRec(t, rec)["error"] != "run_not_found" {
			t.Errorf("%s: status = %d body=%s", name, rec.Code, rec.Body.String())
		}
	}
	rec := runHandler(f.h.httpGetRun, http.MethodGet, "/x", "", append(f.params("nope"), gin.Param{Key: "runId", Value: "theirs"}))
	if rec.Code != http.StatusNotFound || decodeRec(t, rec)["error"] != "coordinator_not_found" {
		t.Errorf("unknown coordinator: status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRunRead_StoreFailureIs500ReadErrorWithNoBody(t *testing.T) {
	f := newReadFixture(t)
	mustExec(t, f.env.store, `DROP TABLE coordinator_wakes`)
	f.insertTurn(t, "run-1", ceilingSession, "st-1", 500)
	rec := runHandler(f.h.httpGetRun, http.MethodGet, "/x", "", append(f.params(f.env.c.ID), gin.Param{Key: "runId", Value: "run-1"}))
	m := decodeRec(t, rec)
	if rec.Code != http.StatusInternalServerError || m["error"] != "read_error" || m["id"] != nil {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAutonomyRead_StoreFailureIs500ReadError(t *testing.T) {
	f := newReadFixture(t)
	mustExec(t, f.env.store, `UPDATE coordinators SET autonomy_enabled = ? WHERE id = ?`, false, f.env.c.ID)
	mustExec(t, f.env.store, `DROP TABLE coordinator_wakes`)
	code, m := f.autonomy(t)
	if code != http.StatusInternalServerError || m["error"] != "read_error" {
		t.Fatalf("status = %d body=%v", code, m)
	}
}

func TestAutonomyReadRoutes_AreRegisteredOnlyWhilePhase3IsEffective(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, phase3 := range []bool{true, false} {
		env := newPhase3Env(t, phase3)
		router := gin.New()
		RegisterRoutes(router, env.svc, newTestLogger(t))
		for _, path := range []string{"/autonomy", "/runs/x"} {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/"+phase3Workspace+"/coordinators/"+env.c.ID+path, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			unregistered := rec.Code == http.StatusNotFound && rec.Body.String() == "404 page not found"
			if phase3 == unregistered {
				t.Errorf("%s: phase3=%v status=%d body=%q", path, phase3, rec.Code, rec.Body.String())
			}
		}
	}
}
