package lsp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func TestLSPHTTPRoutesAreTaskLanguageScopedAndStampUserOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeLSPHTTPService{}
	router := gin.New()
	RegisterRoutes(router, service)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/task-1/lsp/kotlin/restart", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if service.action != ActionRestart || service.taskID != "task-1" || service.language != "kotlin" {
		t.Fatalf("control = action=%q task=%q language=%q", service.action, service.taskID, service.language)
	}
	if service.origin.Initiator != InitiatorUser || service.origin.Reason != "user_restart" {
		t.Fatalf("origin = %#v", service.origin)
	}
}

func TestLSPHTTPRoutesShareTheExistingTaskWildcard(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/tasks/:id", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	RegisterRoutes(router, &fakeLSPHTTPService{})

	response := httptest.NewRecorder()
	router.ServeHTTP(
		response,
		httptest.NewRequest(http.MethodGet, "/api/v1/tasks/task-1/lsp", nil),
	)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestLSPHTTPMutationRejectsOwnershipAndOriginInjection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeLSPHTTPService{}
	router := gin.New()
	RegisterRoutes(router, service)

	for _, body := range []string{
		`{"task_id":"other"}`,
		`{"session_id":"session-1"}`,
		`{"generation":99}`,
		`{"initiator":"agent"}`,
		`{"reason":"forged"}`,
	} {
		request := httptest.NewRequest(
			http.MethodPost,
			"/api/v1/tasks/task-1/lsp/go/start",
			bytes.NewBufferString(body),
		)
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("body=%s status=%d response=%s", body, response.Code, response.Body.String())
		}
	}
	if service.action != ActionNone {
		t.Fatalf("forged request reached controller: %q", service.action)
	}
}

func TestLSPHTTPPolicyAcceptsOnlyPolicyAndMapsDisabledRestartConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeLSPHTTPService{controlErr: ErrServerDisabled}
	router := gin.New()
	RegisterRoutes(router, service)

	policyResponse := httptest.NewRecorder()
	policyRequest := httptest.NewRequest(
		http.MethodPut,
		"/api/v1/tasks/task-1/lsp/python/policy",
		bytes.NewBufferString(`{"policy":"keep_warm"}`),
	)
	router.ServeHTTP(policyResponse, policyRequest)
	if policyResponse.Code != http.StatusConflict {
		t.Fatalf("policy status=%d body=%s", policyResponse.Code, policyResponse.Body.String())
	}
	if service.policy != PolicyKeepWarm || service.origin.Reason != "user_set_policy" {
		t.Fatalf("policy=%q origin=%#v", service.policy, service.origin)
	}

	service.controlErr = nil
	snapshotResponse := httptest.NewRecorder()
	snapshotRequest := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/task-1/lsp", nil)
	router.ServeHTTP(snapshotResponse, snapshotRequest)
	if snapshotResponse.Code != http.StatusOK {
		t.Fatalf("snapshot status=%d", snapshotResponse.Code)
	}
	var snapshot TaskSnapshot
	if err := json.Unmarshal(snapshotResponse.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.TaskID != "task-1" {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestLSPHTTPNotFoundMappingRequiresTypedTaskError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name string
		err  error
		want int
	}{
		{name: "typed task absence", err: repoerrors.ErrTaskNotFound, want: http.StatusNotFound},
		{name: "unrelated dependency text", err: errors.New("language-server binary not found"), want: http.StatusInternalServerError},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(response)
			writeHTTPResult(ctx, nil, test.err)
			if response.Code != test.want {
				t.Fatalf("status=%d body=%s, want %d", response.Code, response.Body.String(), test.want)
			}
		})
	}
}

type fakeLSPHTTPService struct {
	taskID     string
	language   string
	action     Action
	policy     Policy
	origin     Origin
	controlErr error
}

func (f *fakeLSPHTTPService) Snapshot(_ context.Context, taskID string) (*TaskSnapshot, error) {
	return &TaskSnapshot{TaskID: taskID}, f.controlErr
}

func (f *fakeLSPHTTPService) SetPolicy(
	_ context.Context,
	taskID, language string,
	policy Policy,
	origin Origin,
) (*LanguageSnapshot, error) {
	f.taskID, f.language, f.policy, f.origin = taskID, language, policy, origin
	return &LanguageSnapshot{}, f.controlErr
}

func (f *fakeLSPHTTPService) Start(
	_ context.Context,
	taskID, language string,
	origin Origin,
) (*LanguageSnapshot, error) {
	f.taskID, f.language, f.action, f.origin = taskID, language, ActionStart, origin
	return &LanguageSnapshot{}, f.controlErr
}

func (f *fakeLSPHTTPService) Stop(
	_ context.Context,
	taskID, language string,
	origin Origin,
) (*LanguageSnapshot, error) {
	f.taskID, f.language, f.action, f.origin = taskID, language, ActionStop, origin
	return &LanguageSnapshot{}, f.controlErr
}

func (f *fakeLSPHTTPService) Restart(
	_ context.Context,
	taskID, language string,
	origin Origin,
) (*LanguageSnapshot, error) {
	f.taskID, f.language, f.action, f.origin = taskID, language, ActionRestart, origin
	return &LanguageSnapshot{}, f.controlErr
}
