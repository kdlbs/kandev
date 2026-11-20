package dashboard_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kandev/kandev/internal/office/dashboard"
	"github.com/kandev/kandev/internal/office/routing"
)

type fakeSeatWriter struct {
	gotWorkspace string
	gotChanges   []dashboard.SeatAuthorExclusionChange
	err          error
}

func (f *fakeSeatWriter) SetSeatAuthorExclusion(
	_ context.Context, workspaceID string, changes []dashboard.SeatAuthorExclusionChange,
) error {
	f.gotWorkspace = workspaceID
	f.gotChanges = changes
	return f.err
}

var officeReviewSeat = routing.SeatStep{
	WorkflowID: "wf-1", WorkflowName: "Office Default", StepID: "step-review", StepName: "Review", Role: "reviewer",
}

func TestRouting_GetIncludesSeatSteps(t *testing.T) {
	deps, fake := newRoutingTestDeps(t)
	fake.seatSteps = []routing.SeatStep{officeReviewSeat}
	rec := httptest.NewRecorder()
	deps.router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/office/workspaces/ws-1/routing", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var resp dashboard.RoutingConfigResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := dashboard.AuthorExclusionStepDTO{
		WorkflowID: "wf-1", WorkflowName: "Office Default", StepID: "step-review", StepName: "Review", Role: "reviewer",
	}
	if len(resp.AuthorExclusionSteps) != 1 || resp.AuthorExclusionSteps[0] != want {
		t.Fatalf("steps = %+v, want [%+v]", resp.AuthorExclusionSteps, want)
	}
}

func TestRouting_GetWithoutSeatStepsReturnsEmptyList(t *testing.T) {
	deps, _ := newRoutingTestDeps(t)
	rec := httptest.NewRecorder()
	deps.router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/office/workspaces/ws-1/routing", nil))
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(raw["author_exclusion_steps"]) != "[]" {
		t.Fatalf("author_exclusion_steps = %s, want []", raw["author_exclusion_steps"])
	}
}

func putSeatExclusion(t *testing.T, deps *testDeps, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/office/workspaces/ws-1/routing/author-exclusion",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	deps.router.ServeHTTP(rec, req)
	return rec
}

func TestRouting_PutSeatExclusionAppliesChangesAndReturnsState(t *testing.T) {
	deps, fake := newRoutingTestDeps(t)
	writer := &fakeSeatWriter{}
	deps.svc.SetSeatAuthorExclusionWriter(writer)
	enabled := officeReviewSeat
	enabled.ExcludeAuthorProvider = true
	fake.seatSteps = []routing.SeatStep{enabled}
	fake.setupWarnings = testSetupWarnings

	rec := putSeatExclusion(t, deps, `{"changes":[{"step_id":"step-review","role":"reviewer","enabled":true}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	wantChanges := []dashboard.SeatAuthorExclusionChange{{StepID: "step-review", Role: "reviewer", Enabled: true}}
	if writer.gotWorkspace != "ws-1" || fmt.Sprint(writer.gotChanges) != fmt.Sprint(wantChanges) {
		t.Fatalf("writer got %q %+v", writer.gotWorkspace, writer.gotChanges)
	}
	var resp dashboard.SeatAuthorExclusionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.AuthorExclusionSteps) != 1 || !resp.AuthorExclusionSteps[0].Enabled {
		t.Fatalf("steps = %+v", resp.AuthorExclusionSteps)
	}
	if len(resp.AuthorExclusionWarnings) != 1 || resp.AuthorExclusionWarnings[0].Code != "agent_single_provider" {
		t.Fatalf("warnings = %+v", resp.AuthorExclusionWarnings)
	}
}

func TestRouting_PutSeatExclusionMapsErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"invalid change", fmt.Errorf("wrap: %w", dashboard.ErrSeatChangeInvalid), http.StatusBadRequest},
		{"step not visible", fmt.Errorf("wrap: %w", dashboard.ErrSeatChangeNotFound), http.StatusNotFound},
		{"read-only workflow", fmt.Errorf("wrap: %w", dashboard.ErrSeatChangeReadOnly), http.StatusConflict},
		{"step changed concurrently", fmt.Errorf("wrap: %w", dashboard.ErrSeatChangeConflict), http.StatusConflict},
		{"storage failure", errors.New("db down"), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps, _ := newRoutingTestDeps(t)
			deps.svc.SetSeatAuthorExclusionWriter(&fakeSeatWriter{err: tc.err})
			rec := putSeatExclusion(t, deps, `{"changes":[{"step_id":"s","role":"reviewer","enabled":true}]}`)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func TestRouting_PutSeatExclusionRejectsEmptyChanges(t *testing.T) {
	deps, _ := newRoutingTestDeps(t)
	writer := &fakeSeatWriter{}
	deps.svc.SetSeatAuthorExclusionWriter(writer)
	for _, body := range []string{`{"changes":[]}`, `{"changes":[{"step_id":"","role":"reviewer"}]}`} {
		if rec := putSeatExclusion(t, deps, body); rec.Code != http.StatusBadRequest {
			t.Fatalf("body %s: status = %d, want 400", body, rec.Code)
		}
	}
	if writer.gotChanges != nil {
		t.Fatalf("writer called for an invalid request: %+v", writer.gotChanges)
	}
}

func TestRouting_PutSeatExclusionWithoutWriterIsUnavailable(t *testing.T) {
	deps, _ := newRoutingTestDeps(t)
	rec := putSeatExclusion(t, deps, `{"changes":[{"step_id":"s","role":"reviewer","enabled":true}]}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}
