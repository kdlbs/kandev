package dashboard_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/dashboard"
	"github.com/kandev/kandev/internal/office/models"
)

func TestRouting_PutSeatExclusionReleasesOnlyTheStepsTurnedOff(t *testing.T) {
	cases := []struct {
		name string
		body string
		want [][]string
	}{
		{
			"switches turned off",
			`{"changes":[{"step_id":"a","role":"reviewer","enabled":false},` +
				`{"step_id":"a","role":"approver","enabled":false},` +
				`{"step_id":"b","role":"reviewer","enabled":true},` +
				`{"step_id":"c","role":"reviewer","enabled":false}]}`,
			[][]string{{"a", "c"}},
		},
		{"switch turned on", `{"changes":[{"step_id":"s","role":"reviewer","enabled":true}]}`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps, fake := newRoutingTestDeps(t)
			deps.svc.SetSeatAuthorExclusionWriter(&fakeSeatWriter{})
			rec := putSeatExclusion(t, deps, tc.body)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d (body %s)", rec.Code, rec.Body.String())
			}
			if !reflect.DeepEqual(fake.releasedSteps, tc.want) {
				t.Fatalf("released = %v, want %v", fake.releasedSteps, tc.want)
			}
		})
	}
}

func TestRouting_PutSeatExclusionKeepsParkedRunsWhenTheWriteFails(t *testing.T) {
	deps, fake := newRoutingTestDeps(t)
	deps.svc.SetSeatAuthorExclusionWriter(&fakeSeatWriter{err: errors.New("db down")})
	putSeatExclusion(t, deps, `{"changes":[{"step_id":"s","role":"reviewer","enabled":false}]}`)
	if len(fake.releasedSteps) != 0 {
		t.Fatalf("released after a failed write: %v", fake.releasedSteps)
	}
}

func TestRouting_PutSeatExclusionSucceedsWhenReleasingParkedRunsFails(t *testing.T) {
	deps, fake := newRoutingTestDeps(t)
	fake.releaseErr = errors.New("db down")
	deps.svc.SetSeatAuthorExclusionWriter(&fakeSeatWriter{})
	rec := putSeatExclusion(t, deps, `{"changes":[{"step_id":"s","role":"reviewer","enabled":false}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("the switch is saved, so the request succeeds: status = %d", rec.Code)
	}
}

func TestRouting_PutSeatExclusionReportsTheSaveWhenTheReadBackFails(t *testing.T) {
	deps, fake := newRoutingTestDeps(t)
	fake.seatStepsErr = errors.New("db down")
	writer := &fakeSeatWriter{}
	deps.svc.SetSeatAuthorExclusionWriter(writer)
	rec := putSeatExclusion(t, deps, `{"changes":[{"step_id":"s","role":"reviewer","enabled":true}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("the switch is saved, so the request succeeds: status = %d (%s)", rec.Code, rec.Body.String())
	}
	if len(writer.gotChanges) != 1 {
		t.Fatalf("changes = %v, want the one change saved", writer.gotChanges)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(body["author_exclusion_steps"]) != "null" {
		t.Fatalf("steps = %s, want null so the client reloads them", body["author_exclusion_steps"])
	}
}

func TestRouting_PutSeatExclusionRejectsAgentCallers(t *testing.T) {
	deps, fake := newRoutingTestDeps(t)
	writer := &fakeSeatWriter{}
	deps.svc.SetSeatAuthorExclusionWriter(writer)
	router := gin.New()
	router.Use(withAgentCaller(&models.AgentInstance{ID: "agent-1", WorkspaceID: "ws-1"}))
	dashboard.RegisterRoutes(router.Group("/api/v1/office"), deps.svc, deps.repo, nil, nil, nil, logger.Default())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/office/workspaces/ws-1/routing/author-exclusion",
		bytes.NewBufferString(`{"changes":[{"step_id":"s","role":"reviewer","enabled":false}]}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d (%s), want 403", rec.Code, rec.Body.String())
	}
	if writer.gotChanges != nil || len(fake.releasedSteps) != 0 {
		t.Fatalf("an agent request reached the write: changes=%v released=%v", writer.gotChanges, fake.releasedSteps)
	}
}
