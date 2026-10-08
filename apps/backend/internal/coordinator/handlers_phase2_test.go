package coordinator

import (
	"net/http"
	"testing"
)

func TestHTTPGetAndListCoordinator_Phase2Fields(t *testing.T) {
	for _, on := range []bool{true, false} {
		_, c, svc := phase2Fixture(t, on)
		h := &Handlers{service: svc, logger: newTestLogger(t)}
		params := workspaceParams(c.ID)

		rec := runHandler(h.httpGetCoordinator, http.MethodGet, "/x", "", params)
		if rec.Code != http.StatusOK {
			t.Fatalf("phase2=%v get status = %d body=%s", on, rec.Code, rec.Body.String())
		}
		var got map[string]any
		decodeBody(t, rec, &got)
		for _, k := range []string{"policy", "policy_revision", "watches"} {
			if _, ok := got[k]; ok != on {
				t.Fatalf("phase2=%v get: key %q present=%v", on, k, ok)
			}
		}

		rec = runHandler(h.httpListCoordinators, http.MethodGet, "/x", "", workspaceParams(""))
		if rec.Code != http.StatusOK {
			t.Fatalf("phase2=%v list status = %d body=%s", on, rec.Code, rec.Body.String())
		}
		var list struct {
			Coordinators []map[string]any `json:"coordinators"`
		}
		decodeBody(t, rec, &list)
		if len(list.Coordinators) == 0 {
			t.Fatalf("phase2=%v list empty: %s", on, rec.Body.String())
		}
		for _, k := range []string{"policy", "policy_revision", "watches"} {
			if _, ok := list.Coordinators[0][k]; ok != on {
				t.Fatalf("phase2=%v list: key %q present=%v", on, k, ok)
			}
		}
	}
}

func TestHTTPGetAndListCoordinator_FailedPolicyReadIs500(t *testing.T) {
	store, c, svc := phase2Fixture(t, true)
	h := &Handlers{service: svc, logger: newTestLogger(t)}
	if _, err := store.db.Exec(`UPDATE coordinators SET watch_scope = 'selected' WHERE id = ?`, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`DROP TABLE coordinator_watches`); err != nil {
		t.Fatal(err)
	}
	rec := runHandler(h.httpGetCoordinator, http.MethodGet, "/x", "", workspaceParams(c.ID))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("get status = %d body=%s", rec.Code, rec.Body.String())
	}
	rec = runHandler(h.httpListCoordinators, http.MethodGet, "/x", "", workspaceParams(""))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("list status = %d body=%s", rec.Code, rec.Body.String())
	}
}
