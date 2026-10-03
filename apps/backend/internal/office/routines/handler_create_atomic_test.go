package routines_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/office/routines"
)

// listRoutinesViaHTTP fetches the workspace routine list over HTTP and
// returns the decoded routines.
func listRoutinesViaHTTP(t *testing.T, router *gin.Engine) []*routines.RoutineWithSchedule {
	t.Helper()
	rec := doRequest(t, router, http.MethodGet, "/api/v1/workspaces/ws-1/routines", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list routines status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp routines.RoutineListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode routine list: %v", err)
	}
	return resp.Routines
}

// A create body carrying a valid cron trigger creates both rows in one call
// and arms the trigger (next_run_at non-null).
func TestCreateRoutineWithTrigger_ArmsInSingleRequest(t *testing.T) {
	router := newTestRouter(t)

	rec := doRequest(t, router, http.MethodPost, "/api/v1/workspaces/ws-1/routines", `{
		"name": "atomic",
		"trigger": {"kind": "cron", "cron_expression": "0 9 * * *", "timezone": "Asia/Seoul"}
	}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201; body = %s", rec.Code, rec.Body.String())
	}
	var resp routines.RoutineResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode routine response: %v", err)
	}
	routineID := resp.Routine.ID

	list := listRoutinesViaHTTP(t, router)
	if len(list) != 1 {
		t.Fatalf("routine count = %d, want 1", len(list))
	}

	triggersRec := doRequest(t, router, http.MethodGet, "/api/v1/routines/"+routineID+"/triggers", "")
	if triggersRec.Code != http.StatusOK {
		t.Fatalf("list triggers status = %d", triggersRec.Code)
	}
	var triggers routines.TriggerListResponse
	if err := json.Unmarshal(triggersRec.Body.Bytes(), &triggers); err != nil {
		t.Fatalf("decode trigger list: %v", err)
	}
	if len(triggers.Triggers) != 1 {
		t.Fatalf("trigger count = %d, want 1", len(triggers.Triggers))
	}
	if triggers.Triggers[0].NextRunAt == nil {
		t.Fatal("trigger next_run_at is nil, want an armed value")
	}
}

// A rejected trigger in the create body is a 400 and writes no routine at all
// (REQ-OFFICE-ROUTINE-WIRE-002.10, atomic create).
func TestCreateRoutineWithTrigger_RejectedTriggerWritesNoRoutine(t *testing.T) {
	router := newTestRouter(t)

	rec := doRequest(t, router, http.MethodPost, "/api/v1/workspaces/ws-1/routines", `{
		"name": "atomic",
		"trigger": {"kind": "cron", "cron_expression": "0 0 30 2 *"}
	}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("create status = %d, want 400; body = %s", rec.Code, rec.Body.String())
	}
	if list := listRoutinesViaHTTP(t, router); len(list) != 0 {
		t.Fatalf("routine count = %d after rejected trigger, want 0", len(list))
	}
}
