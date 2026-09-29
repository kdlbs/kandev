package coordinator

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func setupService(t *testing.T) (*Service, *Store) {
	t.Helper()
	agents := map[string]*settingsmodels.AgentProfile{"ap-1": {ID: "ap-1", WorkspaceID: testWorkspaceID}}
	executors := map[string]*taskmodels.ExecutorProfile{"ep-1": {ID: "ep-1"}}
	svc := newServiceForTest(t, agents, executors, nil)
	svc.phase2 = true
	createWorkflowsTable(t, svc.store)
	if _, err := svc.store.db.Exec(`CREATE TABLE tasks (id TEXT PRIMARY KEY, workspace_id TEXT, workflow_id TEXT, state TEXT,
		archived_at DATETIME, is_ephemeral INTEGER NOT NULL DEFAULT 0, origin TEXT)`); err != nil {
		t.Fatal(err)
	}
	addWorkflow(t, svc.store, "wf-a", testWorkspaceID)
	addWorkflow(t, svc.store, "wf-b", testWorkspaceID)
	addWorkflow(t, svc.store, "wf-other", "ws-2")
	return svc, svc.store
}

const setupPolicy = `{"actions":{"create_task":"requires_approval","start_agent":"denied","message":"requires_approval","move":"requires_approval","resume":"requires_approval","stop":"denied"}}`

func setupBody(overrides map[string]string) string {
	parts := map[string]string{
		"name":                "Planner",
		"agent_profile_id":    `"ap-1"`,
		"executor_profile_id": `"ep-1"`,
		"watches":             `{"scope":"all"}`,
		"policy":              setupPolicy,
	}
	parts["name"] = `"Planner"`
	for k, v := range overrides {
		if v == "" {
			delete(parts, k)
			continue
		}
		parts[k] = v
	}
	var b strings.Builder
	b.WriteString("{")
	first := true
	for k, v := range parts {
		if !first {
			b.WriteString(",")
		}
		first = false
		b.WriteString(`"` + k + `":` + v)
	}
	b.WriteString("}")
	return b.String()
}

func setupCountRows(t *testing.T, store *Store, table string) int {
	t.Helper()
	var n int
	if err := store.db.Get(&n, `SELECT COUNT(*) FROM `+table); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func assertSetupError(t *testing.T, err error, step, field, code string) {
	t.Helper()
	var se *SettingsError
	if !errors.As(err, &se) {
		t.Fatalf("error = %T %v, want *SettingsError", err, err)
	}
	if se.Step != step || se.Field != field || se.Code != code {
		t.Fatalf("got step=%q field=%q code=%q, want step=%q field=%q code=%q (%s)", se.Step, se.Field, se.Code, step, field, code, se.Message)
	}
}

func TestSetupCreatesEverythingInOneTransaction(t *testing.T) {
	svc, store := setupService(t)
	body := setupBody(map[string]string{
		"context": `"  Focus on release  "`,
		"watches": `{"scope":"selected","workflow_ids":["wf-a","wf-b"]}`,
		"goal":    `{"goal_id":"ignored","name":"Ship 2.0","due_on":"2026-12-01","criteria":[{"text":"Docs done","done":true},{"id":"","text":"Tests green"}]}`,
	})
	c, err := svc.CreateSetup(context.Background(), testWorkspaceID, []byte(body))
	if err != nil {
		t.Fatalf("CreateSetup: %v", err)
	}
	if c.Name != "Planner" || c.Context != "Focus on release" {
		t.Fatalf("coordinator = %+v", c)
	}
	set, err := store.EffectiveWatchSet(context.Background(), store.ro, c.ID, testWorkspaceID)
	if err != nil || set.All || len(set.WorkflowIDs) != 2 {
		t.Fatalf("watch set = %+v err=%v", set, err)
	}
	goal, err := store.ActiveGoal(context.Background(), c.ID)
	if err != nil || goal == nil || goal.Name != "Ship 2.0" || len(goal.Criteria) != 2 {
		t.Fatalf("goal = %+v err=%v", goal, err)
	}
	for _, cr := range goal.Criteria {
		if cr.Done {
			t.Fatalf("criterion stored done: %+v", cr)
		}
	}
	var revision int
	var policy *string
	if err := store.db.QueryRow(`SELECT policy_revision, policy_json FROM coordinators WHERE id = ?`, c.ID).Scan(&revision, &policy); err != nil {
		t.Fatal(err)
	}
	if revision != 1 || policy == nil || !strings.Contains(*policy, `"message":"requires_approval"`) {
		t.Fatalf("revision=%d policy=%v", revision, policy)
	}
}

func TestSetupAllScopeStoresNoWatchRowsAndSkippedGoalStoresNone(t *testing.T) {
	svc, store := setupService(t)
	body := setupBody(map[string]string{"watches": `{"scope":"all","workflow_ids":["wf-a"]}`, "goal": "null", "context": "null"})
	c, err := svc.CreateSetup(context.Background(), testWorkspaceID, []byte(body))
	if err != nil {
		t.Fatalf("CreateSetup: %v", err)
	}
	if c.Context != "" || setupCountRows(t, store, "coordinator_watches") != 0 || setupCountRows(t, store, "coordinator_goals") != 0 {
		t.Fatalf("context=%q watches=%d goals=%d", c.Context, setupCountRows(t, store, "coordinator_watches"), setupCountRows(t, store, "coordinator_goals"))
	}
}

func TestSetupRepeatedNamesCreateTwoCoordinators(t *testing.T) {
	svc, store := setupService(t)
	for i := 0; i < 2; i++ {
		if _, err := svc.CreateSetup(context.Background(), testWorkspaceID, []byte(setupBody(nil))); err != nil {
			t.Fatalf("CreateSetup %d: %v", i, err)
		}
	}
	if n := setupCountRows(t, store, "coordinators"); n != 2 {
		t.Fatalf("coordinators = %d, want 2", n)
	}
}

func TestSetupValidationOrderAndFields(t *testing.T) {
	longCtx := `"` + strings.Repeat("x", 4001) + `"`
	badGoal := `{"name":""}`
	cases := []struct {
		name      string
		overrides map[string]string
		step      string
		field     string
		code      string
	}{
		{"blank name", map[string]string{"name": `"   "`}, "identity", "name", ""},
		{"missing name", map[string]string{"name": ""}, "identity", "name", ""},
		{"unknown agent profile", map[string]string{"agent_profile_id": `"nope"`}, "identity", "agent_profile_id", ""},
		{"missing executor profile", map[string]string{"executor_profile_id": ""}, "identity", "executor_profile_id", ""},
		{"missing watches", map[string]string{"watches": ""}, "watches", "watches", codeInvalidScope},
		{"bad scope", map[string]string{"watches": `{"scope":"some"}`}, "watches", "watches", codeInvalidScope},
		{"selected empty", map[string]string{"watches": `{"scope":"selected","workflow_ids":[]}`}, "watches", "watches", codeWatchesEmpty},
		{"selected absent ids", map[string]string{"watches": `{"scope":"selected"}`}, "watches", "watches", codeWatchesEmpty},
		{"duplicate", map[string]string{"watches": `{"scope":"selected","workflow_ids":["wf-a","wf-a"]}`}, "watches", "watches", codeWatchesDuplicate},
		{"foreign", map[string]string{"watches": `{"scope":"selected","workflow_ids":["wf-other"]}`}, "watches", "watches", codeWatchesForeignWorkflow},
		{"unknown workflow", map[string]string{"watches": `{"scope":"selected","workflow_ids":["ghost"]}`}, "watches", "watches", codeWatchesForeignWorkflow},
		{"empty goal object", map[string]string{"goal": `{}`}, "goal", "goal.name", ""},
		{"goal due date", map[string]string{"goal": `{"name":"g","due_on":"2026-13-40","criteria":[]}`}, "goal", "goal.due_on", ""},
		{"goal criterion text", map[string]string{"goal": `{"name":"g","criteria":[{"text":""}]}`}, "goal", "goal.criteria[0].text", ""},
		{"goal criterion id", map[string]string{"goal": `{"name":"g","criteria":[{"id":"c1","text":"t"}]}`}, "goal", "goal.criteria[0].id", ""},
		{"context too long", map[string]string{"context": longCtx}, "context", "context", ""},
		{"missing policy", map[string]string{"policy": ""}, "may-do", "policy.actions.create_task", codeActionMissing},
		{"unknown action", map[string]string{"policy": `{"actions":{"create_task":"denied","start_agent":"denied","message":"denied","move":"denied","resume":"denied","stop":"denied","fly":"denied"}}`}, "may-do", "policy.actions.fly", codeUnknownAction},
		{"stop not denied", map[string]string{"policy": `{"actions":{"create_task":"denied","start_agent":"denied","message":"denied","move":"denied","resume":"denied","stop":"requires_approval"}}`}, "may-do", "policy.actions.stop", codeStopDeniedOnly},
		{"automatic", map[string]string{"policy": `{"actions":{"create_task":"automatic","start_agent":"denied","message":"denied","move":"denied","resume":"denied","stop":"denied"}}`}, "may-do", "policy.actions.create_task", codeAutomaticNotAvailable},
		{"first failure wins: identity before watches", map[string]string{"name": `""`, "watches": `{"scope":"bad"}`}, "identity", "name", ""},
		{"first failure wins: watches before goal", map[string]string{"watches": `{"scope":"bad"}`, "goal": badGoal}, "watches", "watches", codeInvalidScope},
		{"first failure wins: goal before context", map[string]string{"goal": badGoal, "context": longCtx}, "goal", "goal.name", ""},
		{"first failure wins: context before may-do", map[string]string{"context": longCtx, "policy": `{"actions":{}}`}, "context", "context", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, store := setupService(t)
			_, err := svc.CreateSetup(context.Background(), testWorkspaceID, []byte(setupBody(tc.overrides)))
			assertSetupError(t, err, tc.step, tc.field, tc.code)
			for _, table := range []string{"coordinators", "coordinator_watches", "coordinator_goals"} {
				if n := setupCountRows(t, store, table); n != 0 {
					t.Fatalf("%s has %d rows after a rejected setup", table, n)
				}
			}
		})
	}
}

func TestSetupInvalidBodyHasNoStep(t *testing.T) {
	cases := map[string]struct{ body, field string }{
		"not json":        {`nope`, ""},
		"array":           {`[]`, ""},
		"null":            {`null`, ""},
		"name number":     {`{"name":5}`, "name"},
		"context number":  {`{"context":5}`, "context"},
		"watches string":  {`{"watches":"all"}`, "watches"},
		"policy array":    {`{"policy":[]}`, "policy"},
		"goal string":     {`{"goal":"x"}`, "goal"},
		"scope number":    {`{"watches":{"scope":3}}`, "watches"},
		"type over order": {`{"name":"","context":5}`, "context"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			svc, _ := setupService(t)
			_, err := svc.CreateSetup(context.Background(), testWorkspaceID, []byte(tc.body))
			assertSetupError(t, err, "", tc.field, codeInvalidBody)
		})
	}
}

func TestSetupFailedReadIsNotAValidationError(t *testing.T) {
	svc, store := setupService(t)
	if _, err := store.db.Exec(`DROP TABLE workflows`); err != nil {
		t.Fatal(err)
	}
	body := setupBody(map[string]string{"watches": `{"scope":"selected","workflow_ids":["wf-a"]}`})
	_, err := svc.CreateSetup(context.Background(), testWorkspaceID, []byte(body))
	var se *SettingsError
	if err == nil || errors.As(err, &se) {
		t.Fatalf("err = %v, want a plain read failure", err)
	}
	if n := setupCountRows(t, store, "coordinators"); n != 0 {
		t.Fatalf("coordinators = %d", n)
	}
}

func TestSetupInvalidNameWinsOverFailingReads(t *testing.T) {
	svc, store := setupService(t)
	if _, err := store.db.Exec(`DROP TABLE workflows`); err != nil {
		t.Fatal(err)
	}
	body := setupBody(map[string]string{"name": `""`, "watches": `{"scope":"selected","workflow_ids":["wf-a"]}`})
	_, err := svc.CreateSetup(context.Background(), testWorkspaceID, []byte(body))
	assertSetupError(t, err, "identity", "name", "")
}

func TestSetupGoalInsertFaultRollsBackCoordinatorAndWatches(t *testing.T) {
	svc, store := setupService(t)
	if _, err := store.db.Exec(`DROP TABLE coordinator_goals`); err != nil {
		t.Fatal(err)
	}
	body := setupBody(map[string]string{
		"watches": `{"scope":"selected","workflow_ids":["wf-a"]}`,
		"goal":    `{"name":"g","criteria":[]}`,
	})
	_, err := svc.CreateSetup(context.Background(), testWorkspaceID, []byte(body))
	if err == nil {
		t.Fatal("want an error")
	}
	if n := setupCountRows(t, store, "coordinators"); n != 0 {
		t.Fatalf("coordinators = %d after a failed goal insert", n)
	}
	if n := setupCountRows(t, store, "coordinator_watches"); n != 0 {
		t.Fatalf("watch rows = %d after a failed goal insert", n)
	}
}

func TestSetupWatchesInsertFaultRollsBackCoordinator(t *testing.T) {
	svc, store := setupService(t)
	if _, err := store.db.Exec(`DROP TABLE coordinator_watches`); err != nil {
		t.Fatal(err)
	}
	body := setupBody(map[string]string{"watches": `{"scope":"selected","workflow_ids":["wf-a"]}`})
	if _, err := svc.CreateSetup(context.Background(), testWorkspaceID, []byte(body)); err == nil {
		t.Fatal("want an error")
	}
	if n := setupCountRows(t, store, "coordinators"); n != 0 {
		t.Fatalf("coordinators = %d after a failed watch insert", n)
	}
}

func TestSetupRequiresManageAndPhase2(t *testing.T) {
	svc, _ := setupService(t)
	svc.phase2 = false
	if _, err := svc.CreateSetup(context.Background(), testWorkspaceID, []byte(setupBody(nil))); !errors.Is(err, ErrNotFound) {
		t.Fatalf("phase 2 off: err = %v, want ErrNotFound", err)
	}
	svc.phase2 = true
	if _, err := svc.CreateSetup(context.Background(), testWorkspaceID, []byte(`nope`)); err == nil {
		t.Fatal("want an error")
	}
	assertLastScope(t, svc, "workspace.manage")
}

func TestSetupForbiddenBeforeBodyDecode(t *testing.T) {
	agents := map[string]*settingsmodels.AgentProfile{}
	svc := newServiceForTest(t, agents, nil, errors.New("denied"))
	svc.phase2 = true
	_, err := svc.CreateSetup(context.Background(), testWorkspaceID, []byte(`nope`))
	var se *SettingsError
	if err == nil || errors.As(err, &se) {
		t.Fatalf("err = %v, want the authorizer error, not a body error", err)
	}
}

func TestHTTPSetupRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, _ := setupService(t)
	router := gin.New()
	RegisterRoutes(router, svc, newTestLogger(t))
	post := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/"+testWorkspaceID+"/coordinators/setup", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(rec, req)
		return rec
	}
	rec := post(setupBody(nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var dto CoordinatorDTO
	decodeBody(t, rec, &dto)
	if dto.ID == "" || dto.CoordinatorPhase2 == nil || dto.PolicyRevision != 1 || dto.Watches.Scope != "all" {
		t.Fatalf("dto = %+v", dto)
	}
	rec = post(setupBody(map[string]string{"watches": `{"scope":"selected","workflow_ids":[]}`}))
	var got map[string]string
	decodeBody(t, rec, &got)
	if rec.Code != http.StatusBadRequest || got["step"] != "watches" || got["field"] != "watches" || got["code"] != codeWatchesEmpty || got["error"] == "" {
		t.Fatalf("status=%d body=%v", rec.Code, got)
	}
	rec = post(setupBody(map[string]string{"name": `""`}))
	got = map[string]string{}
	decodeBody(t, rec, &got)
	if _, hasCode := got["code"]; hasCode || got["step"] != "identity" || got["field"] != "name" {
		t.Fatalf("identity error body = %v (no code expected)", got)
	}
	rec = post(`[]`)
	got = map[string]string{}
	decodeBody(t, rec, &got)
	if _, hasStep := got["step"]; hasStep || got["code"] != codeInvalidBody {
		t.Fatalf("invalid body = %v", got)
	}
}

func TestHTTPSetupRouteAbsentWithoutPhase2(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, _ := setupService(t)
	svc.phase2 = false
	router := gin.New()
	RegisterRoutes(router, svc, newTestLogger(t))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/"+testWorkspaceID+"/coordinators/setup", strings.NewReader(setupBody(nil))))
	// With the phase-2 routes off, "setup" is read as a coordinator id by
	// no POST route, so the answer is 404.
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestSetupResponseDegradesWhenPolicyReadFails(t *testing.T) {
	svc, _ := setupService(t)
	h := NewHandlers(svc, newTestLogger(t))
	ghost := &Coordinator{ID: "missing-coordinator", WorkspaceID: testWorkspaceID, Name: "ghost"}
	dto := h.setupResponse(context.Background(), ghost)
	if dto == nil || dto.ID != ghost.ID || dto.Name != "ghost" {
		t.Fatalf("dto = %+v, want the base shape of the committed coordinator", dto)
	}
}
