package mentions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/logger"

	apiv1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type fakeSearcher struct {
	search func(context.Context, SearchRequest) (*apiv1.MentionSearchResponse, error)
}

func TestHandlerRegisterRoutesUsesExistingWorkspaceWildcardName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/workspaces/:id", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	handler := NewHandler(fakeSearcher{search: func(_ context.Context, request SearchRequest) (*apiv1.MentionSearchResponse, error) {
		return &apiv1.MentionSearchResponse{Query: request.Query}, nil
	}}, nil)

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		handler.RegisterRoutes(router)
	}()
	if recovered != nil {
		t.Fatalf("register mention route alongside workspace routes: %v", recovered)
	}
}

func (s fakeSearcher) Search(ctx context.Context, request SearchRequest) (*apiv1.MentionSearchResponse, error) {
	return s.search(ctx, request)
}

func TestHandlerSearch_ReturnsNormalizedResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewHandler(fakeSearcher{search: func(_ context.Context, request SearchRequest) (*apiv1.MentionSearchResponse, error) {
		if request.WorkspaceID != "workspace-1" || request.Query != "auth" || request.Limit != MaxLimit {
			t.Fatalf("request = %+v", request)
		}
		return &apiv1.MentionSearchResponse{
			Query: "auth",
			Groups: []apiv1.MentionGroup{{
				Source: "tasks", Provider: "kandev", Kind: "task",
				DisplayName: "Kandev tasks", KindLabel: "Task", Status: StatusOK,
				Results: []apiv1.EntityReference{},
			}},
		}, nil
	}}, nil).RegisterRoutes(router)

	request := httptest.NewRequest(http.MethodGet,
		"/api/v1/workspaces/workspace-1/mentions/search?q=auth&limit=99", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body apiv1.MentionSearchResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Query != "auth" || len(body.Groups) != 1 || body.Groups[0].Source != "tasks" {
		t.Fatalf("body = %#v", body)
	}
}

func TestHandlerSearch_RejectsInvalidLimitWithoutSearching(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewHandler(fakeSearcher{search: func(_ context.Context, _ SearchRequest) (*apiv1.MentionSearchResponse, error) {
		t.Fatal("search called for invalid limit")
		return nil, nil
	}}, nil).RegisterRoutes(router)

	request := httptest.NewRequest(http.MethodGet,
		"/api/v1/workspaces/workspace-1/mentions/search?q=auth&limit=not-a-number", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusBadRequest, response.Body.String())
	}
}

func TestHandlerSearch_MapsSafeErrorsWithoutLeaking(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{name: "invalid request", err: fmt.Errorf("query detail: %w", ErrInvalidRequest), wantStatus: http.StatusBadRequest},
		{name: "workspace missing", err: fmt.Errorf("lookup detail: %w", ErrWorkspaceNotFound), wantStatus: http.StatusNotFound},
		{name: "internal", err: errors.New("secret database address"), wantStatus: http.StatusInternalServerError},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			router := gin.New()
			NewHandler(fakeSearcher{search: func(_ context.Context, _ SearchRequest) (*apiv1.MentionSearchResponse, error) {
				return nil, test.err
			}}, nil).RegisterRoutes(router)

			request := httptest.NewRequest(http.MethodGet,
				"/api/v1/workspaces/workspace-1/mentions/search?q=auth", nil)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, test.wantStatus, response.Body.String())
			}
			if strings.Contains(response.Body.String(), test.err.Error()) {
				t.Fatalf("response leaked raw error: %s", response.Body.String())
			}
		})
	}
}

func TestHandlerSearch_MapsClientCancellationTo499WithoutLogging(t *testing.T) {
	log, observed := newMentionObserverLogger(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewHandler(fakeSearcher{search: func(context.Context, SearchRequest) (*apiv1.MentionSearchResponse, error) {
		return nil, context.Canceled
	}}, log).RegisterRoutes(router)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(
		http.MethodGet,
		"/api/v1/workspaces/workspace-1/mentions/search?q=secret-query",
		nil,
	))

	if response.Code != 499 {
		t.Fatalf("status = %d, want 499; body = %s", response.Code, response.Body.String())
	}
	if observed.Len() != 0 {
		t.Fatalf("cancellation emitted %d log records, want none", observed.Len())
	}
}

func TestHandlerSearchLogsUnexpectedFailureOnceWithSafeFields(t *testing.T) {
	log, observed := newMentionObserverLogger(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewHandler(fakeSearcher{search: func(context.Context, SearchRequest) (*apiv1.MentionSearchResponse, error) {
		return nil, errors.New("secret database address")
	}}, log).RegisterRoutes(router)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(
		http.MethodGet,
		"/api/v1/workspaces/workspace-1/mentions/search?q=secret-query",
		nil,
	))

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	entries := observed.All()
	if len(entries) != 1 || entries[0].Level != zap.ErrorLevel {
		t.Fatalf("unexpected error records = %#v", entries)
	}
	fields := entries[0].ContextMap()
	if fields["workspace_id"] != "workspace-1" || fields["failure_stage"] != "search" ||
		fields["failure_class"] != "internal" {
		t.Fatalf("safe failure fields = %#v", fields)
	}
	serialized := entries[0].Message + response.Body.String() + fmt.Sprint(entries[0].ContextMap())
	if strings.Contains(serialized, "secret-query") || strings.Contains(serialized, "secret database address") {
		t.Fatalf("sensitive mention-search detail leaked: %s", serialized)
	}
}

func newMentionObserverLogger(t *testing.T) (*logger.Logger, *observer.ObservedLogs) {
	t.Helper()
	core, observed := observer.New(zap.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("create observer logger: %v", err)
	}
	return log, observed
}
