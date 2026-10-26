package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/agent/nomad"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
)

type readmitterStub struct {
	err   error
	calls []nomad.NomadReadmitRequest
}

func (s *readmitterStub) ReadmitNomadEnvironment(_ context.Context, req nomad.NomadReadmitRequest) error {
	s.calls = append(s.calls, req)
	return s.err
}

const readmitBody = `{"node_id":"node-1","environment_id":"env-1","generation":2,"job_id":"job-1",` +
	`"attestation":{"process_tree_absent":true,"job_stopped_purge_false":true,"fence_removed":true,"operator":"op"}}`

func postReadmit(t *testing.T, stub *readmitterStub, body string) *httptest.ResponseRecorder {
	t.Helper()
	return postReadmitAs(t, stub, body, &authn.Identity{UserID: "admin", Role: authn.RoleAdmin})
}

func postReadmitAs(t *testing.T, stub *readmitterStub, body string, id *authn.Identity) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	router := gin.New()
	if id != nil {
		router.Use(func(c *gin.Context) { authn.SetOnGin(c, *id); c.Next() })
	}
	RegisterNomadReadmitRoutes(router, stub, log)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/executors/nomad/readmit", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestNomadWorkerQuarantineReadmitHandlerStatusCodes(t *testing.T) {
	cases := []struct {
		name string
		err  error
		body string
		want int
	}{
		{"released", nil, readmitBody, http.StatusOK},
		{"malformed json", nil, `{`, http.StatusBadRequest},
		{"unknown field", nil, `{"node_id":"n","extra":1}`, http.StatusBadRequest},
		{"invalid attestation", nomad.ErrNomadReadmitInvalid, readmitBody, http.StatusBadRequest},
		{"not found", nomad.ErrNomadReadmitNotFound, readmitBody, http.StatusNotFound},
		{"phase", nomad.ErrNomadReadmitPhase, readmitBody, http.StatusConflict},
		{"busy", fmt.Errorf("%w: environment claimed", nomad.ErrNomadReadmitBusy), readmitBody, http.StatusConflict},
		{"internal", errors.New("db password=secret"), readmitBody, http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := &readmitterStub{err: tc.err}
			rec := postReadmit(t, stub, tc.body)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tc.want, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "secret") {
				t.Fatalf("response leaks internal error detail: %s", rec.Body.String())
			}
			if tc.want == http.StatusOK && !strings.Contains(rec.Body.String(), `"status":"operator_released"`) {
				t.Fatalf("body = %s", rec.Body.String())
			}
		})
	}
}

func TestNomadWorkerQuarantineReadmitHandlerPassesExactIdentity(t *testing.T) {
	stub := &readmitterStub{}
	postReadmit(t, stub, readmitBody)
	if len(stub.calls) != 1 {
		t.Fatalf("calls = %d", len(stub.calls))
	}
	got := stub.calls[0]
	if got.NodeID != "node-1" || got.EnvironmentID != "env-1" || got.Generation != 2 || got.JobID != "job-1" ||
		!got.Attestation.ProcessTreeAbsent || !got.Attestation.JobStoppedPurgeFalse || !got.Attestation.FenceRemoved || got.Attestation.Operator != "op" {
		t.Fatalf("request = %+v", got)
	}
}

func TestNomadReadmitHandlerRequiresAdmin(t *testing.T) {
	cases := []struct {
		name string
		id   *authn.Identity
		want int
	}{
		{"anonymous", nil, http.StatusUnauthorized},
		{"member", &authn.Identity{UserID: "u", Role: authn.RoleMember}, http.StatusForbidden},
		{"guest", &authn.Identity{UserID: "g", Role: authn.RoleGuest}, http.StatusForbidden},
		{"admin", &authn.Identity{UserID: "a", Role: authn.RoleAdmin}, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := &readmitterStub{}
			rec := postReadmitAs(t, stub, readmitBody, tc.id)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
			if tc.want != http.StatusOK && len(stub.calls) != 0 {
				t.Fatal("readmitter invoked for an unauthorized caller")
			}
		})
	}
}

func TestNomadReadmitHandlerRejectsTrailingJSON(t *testing.T) {
	for _, body := range []string{readmitBody + readmitBody, readmitBody + ` {}`, readmitBody + ` x`} {
		stub := &readmitterStub{}
		if rec := postReadmit(t, stub, body); rec.Code != http.StatusBadRequest || len(stub.calls) != 0 {
			t.Fatalf("status = %d calls = %d for %q", rec.Code, len(stub.calls), body)
		}
	}
	stub := &readmitterStub{}
	if rec := postReadmit(t, stub, readmitBody+"\n  \n"); rec.Code != http.StatusOK {
		t.Fatalf("trailing whitespace status = %d", rec.Code)
	}
}

func TestNomadReadmitHandlerBoundsBodySize(t *testing.T) {
	stub := &readmitterStub{}
	big := `{"node_id":"` + strings.Repeat("a", 32<<10) + `"}`
	if rec := postReadmit(t, stub, big); rec.Code != http.StatusBadRequest || len(stub.calls) != 0 {
		t.Fatalf("status = %d calls = %d", rec.Code, len(stub.calls))
	}
}
