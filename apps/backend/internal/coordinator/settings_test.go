package coordinator

import (
	"context"
	"errors"
	"testing"

	taskservice "github.com/kandev/kandev/internal/task/service"
)

func TestSaveSettings_ValidationCodes(t *testing.T) {
	store, c, _, svc := phase2ApproveFixture(t)
	createWorkflowsTable(t, store)
	addWorkflow(t, store, "wf-a", "ws-1")
	addWorkflow(t, store, "wf-other", "ws-2")
	all := func(extra string) string {
		return `{"policy":{"actions":{"create_task":"requires_approval","start_agent":"denied","message":"denied","move":"denied","resume":"denied","stop":"denied"` + extra + `}}}`
	}
	cases := []struct {
		name, body, code, field string
	}{
		{"not an object", `[]`, codeInvalidBody, ""},
		{"policy wrong type", `{"policy":3}`, codeInvalidBody, "policy"},
		{"policy without actions", `{"policy":{}}`, codeActionMissing, "policy.actions.create_task"},
		{"unknown action", all(`,"merge":"denied"`), codeUnknownAction, "policy.actions.merge"},
		{"missing action", `{"policy":{"actions":{"create_task":"denied"}}}`, codeActionMissing, "policy.actions.start_agent"},
		{"invalid setting", `{"policy":{"actions":{"create_task":"maybe","start_agent":"denied","message":"denied","move":"denied","resume":"denied","stop":"denied"}}}`, codeInvalidSetting, "policy.actions.create_task"},
		{"null setting", `{"policy":{"actions":{"create_task":null,"start_agent":"denied","message":"denied","move":"denied","resume":"denied","stop":"denied"}}}`, codeInvalidSetting, "policy.actions.create_task"},
		{"automatic", `{"policy":{"actions":{"create_task":"automatic","start_agent":"denied","message":"denied","move":"denied","resume":"denied","stop":"denied"}}}`, codeAutomaticNotAvailable, "policy.actions.create_task"},
		{"stop not denied", `{"policy":{"actions":{"create_task":"denied","start_agent":"denied","message":"denied","move":"denied","resume":"denied","stop":"requires_approval"}}}`, codeStopDeniedOnly, "policy.actions.stop"},
		{"stop automatic wins", `{"policy":{"actions":{"create_task":"denied","start_agent":"denied","message":"denied","move":"denied","resume":"denied","stop":"automatic"}}}`, codeAutomaticNotAvailable, "policy.actions.stop"},
		{"bad scope", `{"watches":{"scope":"some"}}`, codeInvalidScope, "watches"},
		{"missing scope", `{"watches":{}}`, codeInvalidScope, "watches"},
		{"selected empty", `{"watches":{"scope":"selected","workflow_ids":[]}}`, codeWatchesEmpty, "watches"},
		{"selected absent ids", `{"watches":{"scope":"selected"}}`, codeWatchesEmpty, "watches"},
		{"duplicate", `{"watches":{"scope":"selected","workflow_ids":["wf-a","wf-a"]}}`, codeWatchesDuplicate, "watches"},
		{"foreign workflow", `{"watches":{"scope":"selected","workflow_ids":["wf-other"]}}`, codeWatchesForeignWorkflow, "watches"},
		{"policy error wins over watches", `{"policy":{"actions":{}},"watches":{"scope":"bad"}}`, codeActionMissing, "policy.actions.create_task"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.SaveSettings(context.Background(), c.WorkspaceID, c.ID, []byte(tc.body))
			var se *SettingsError
			if !errors.As(err, &se) || se.Code != tc.code || se.Field != tc.field {
				t.Fatalf("err = %v, want code %q field %q", err, tc.code, tc.field)
			}
			got, _ := store.GetCoordinatorByID(context.Background(), c.ID)
			if got.PolicyRevision != 0 {
				t.Fatalf("a refused save changed the revision to %d", got.PolicyRevision)
			}
		})
	}
}

func TestSaveSettings_TooManyWorkflows(t *testing.T) {
	_, c, _, svc := phase2ApproveFixture(t)
	ids := `"w0"`
	for i := 1; i <= maxWatchedWorkflows; i++ {
		ids += `,"w` + string(rune('a'+i%26)) + string(rune('a'+i/26)) + `"`
	}
	_, err := svc.SaveSettings(context.Background(), c.WorkspaceID, c.ID, []byte(`{"watches":{"scope":"selected","workflow_ids":[`+ids+`]}}`))
	if got := settingsCode(t, err); got != codeWatchesTooMany {
		t.Fatalf("code = %q", got)
	}
}

func TestSaveSettings_EmptyAndNullBodiesChangeNothing(t *testing.T) {
	_, c, _, svc := phase2ApproveFixture(t)
	for _, body := range []string{`{}`, `{"policy":null,"watches":null}`, `{"unrelated":1}`} {
		got := mustSave(t, svc, c.WorkspaceID, c.ID, body)
		if got.PolicyRevision != 0 {
			t.Fatalf("%s: revision = %d, want 0", body, got.PolicyRevision)
		}
	}
}

func TestSaveSettings_ReaderIsForbiddenAndForeignWorkspaceIs404(t *testing.T) {
	_, c, _, svc := phase2ApproveFixture(t)
	svc.authz = &fakeWorkspaceAuthorizer{err: taskservice.ErrForbidden}
	if _, err := svc.SaveSettings(context.Background(), c.WorkspaceID, c.ID, []byte(`{}`)); !errors.Is(err, taskservice.ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
	svc.authz = &fakeWorkspaceAuthorizer{}
	if _, err := svc.SaveSettings(context.Background(), "ws-other", c.ID, []byte(`{}`)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// A PUT compared with the effective set is a no-op even when a stale stored id
// remains, and a real change writes the effective result without the stale id.
func TestSaveSettings_StaleStoredIDIsDroppedByARealChange(t *testing.T) {
	store, c, _, svc := phase2ApproveFixture(t)
	createWorkflowsTable(t, store)
	addWorkflow(t, store, "wf-a", "ws-1")
	addWorkflow(t, store, "wf-b", "ws-1")
	addWorkflow(t, store, "wf-w", "ws-1")
	mustSave(t, svc, c.WorkspaceID, c.ID, `{"watches":{"scope":"selected","workflow_ids":["wf-a","wf-w"]}}`)
	dropWorkflow(t, store, "wf-w")
	got := mustSave(t, svc, c.WorkspaceID, c.ID, `{"watches":{"scope":"selected","workflow_ids":["wf-a","wf-b"]}}`)
	if got.PolicyRevision != 2 || len(got.Watches.WorkflowIDs) != 2 {
		t.Fatalf("got %+v, want revision 2 over [wf-a wf-b]", got)
	}
	var stale int
	_ = store.db.Get(&stale, `SELECT COUNT(*) FROM coordinator_watches WHERE workflow_id = 'wf-w'`)
	if stale != 0 {
		t.Fatalf("stale rows = %d, want 0", stale)
	}
}

// Stored {W} alone: the effective set is empty; a save that leaves Watches as
// stored is not refused (AC 003.2).
func TestSaveSettings_SaveLeavingEmptyEffectiveWatchesAsStoredIsAccepted(t *testing.T) {
	store, c, _, svc := phase2ApproveFixture(t)
	createWorkflowsTable(t, store)
	addWorkflow(t, store, "wf-w", "ws-1")
	mustSave(t, svc, c.WorkspaceID, c.ID, `{"watches":{"scope":"selected","workflow_ids":["wf-w"]}}`)
	dropWorkflow(t, store, "wf-w")
	for _, body := range []string{
		`{"watches":{"scope":"selected","workflow_ids":[]}}`,
		`{"watches":{"scope":"selected"}}`,
		`{"watches":{"scope":"selected","workflow_ids":["wf-w"]}}`,
	} {
		got := mustSave(t, svc, c.WorkspaceID, c.ID, body)
		if got.PolicyRevision != 1 || len(got.Watches.WorkflowIDs) != 0 || got.Watches.Scope != "selected" {
			t.Fatalf("%s: got %+v, want no-op over empty selected", body, got)
		}
	}
}

func TestGetSettings_ListsOnlyEffectiveIDs(t *testing.T) {
	store, c, _, svc := phase2ApproveFixture(t)
	createWorkflowsTable(t, store)
	addWorkflow(t, store, "wf-a", "ws-1")
	addWorkflow(t, store, "wf-w", "ws-1")
	mustSave(t, svc, c.WorkspaceID, c.ID, `{"watches":{"scope":"selected","workflow_ids":["wf-a","wf-w"]}}`)
	dropWorkflow(t, store, "wf-w")
	got, err := svc.GetSettings(context.Background(), c.WorkspaceID, c.ID)
	if err != nil || len(got.Watches.WorkflowIDs) != 1 || got.Watches.WorkflowIDs[0] != "wf-a" {
		t.Fatalf("got %+v, %v", got, err)
	}
	set, err := svc.EffectiveWatchSet(context.Background(), c.ID)
	if err != nil || set.Contains("wf-w") || !set.Contains("wf-a") {
		t.Fatalf("effective set = %+v, %v", set, err)
	}
}

// A watches-only save with a stored policy that cannot be parsed still
// succeeds; the policy member is left as stored.
func TestSaveSettings_WatchesOnlySaveIgnoresCorruptPolicy(t *testing.T) {
	store, c, _, svc := phase2ApproveFixture(t)
	if _, err := store.db.Exec(`UPDATE coordinators SET policy_json = '{bad' WHERE id = ?`, c.ID); err != nil {
		t.Fatal(err)
	}
	createWorkflowsTable(t, store)
	addWorkflow(t, store, "wf-a", "ws-1")
	got := mustSave(t, svc, c.WorkspaceID, c.ID, `{"watches":{"scope":"selected","workflow_ids":["wf-a"]}}`)
	if got.PolicyRevision != 1 || got.Watches.Scope != "selected" {
		t.Fatalf("got %+v, want revision 1 selected", got)
	}
	var raw *string
	_ = store.db.Get(&raw, `SELECT policy_json FROM coordinators WHERE id = ?`, c.ID)
	if raw == nil || *raw != "{bad" {
		t.Fatalf("policy_json = %v, want left as stored", raw)
	}
}
