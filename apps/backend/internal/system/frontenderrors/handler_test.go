package frontenderrors

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestHandlerLogsBoundedStructuredFrontendError(t *testing.T) {
	router, observed := newHandlerTestRouter(t, "user-1")
	body := `{
		"client_timestamp":"2026-07-30T12:34:56.789Z",
		"source":"sonner",
		"task_id":"task-123",
		"title":"Failed to save",
		"description":"The backend rejected the update",
		"url":"http://localhost/settings/system/logs?token=secret#private",
		"user_agent":"test-agent",
		"language":"en-US",
		"platform":"test-platform",
		"viewport":{"width":1440,"height":900},
		"stack":"toast stack",
		"error":{"name":"TypeError","message":"Failed to fetch","stack":"error stack"}
	}`
	response := performFrontendErrorRequest(router, body)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	entries := observed.All()
	if len(entries) != 1 {
		t.Fatalf("logged entries = %d, want 1", len(entries))
	}
	entry := entries[0]
	if entry.Level != zapcore.ErrorLevel || entry.Message != "frontend error toast" {
		t.Fatalf("entry = %s %q", entry.Level, entry.Message)
	}
	fields := entry.ContextMap()
	if fields["task_id"] != "task-123" || fields["source"] != "sonner" ||
		fields["title"] != "Failed to save" || fields["error_name"] != "TypeError" {
		t.Fatalf("structured fields = %#v", fields)
	}
	if fields["url"] != "http://localhost/settings/system/logs" {
		t.Fatalf("logged url = %#v", fields["url"])
	}
}

func TestHandlerRejectsInvalidAndOversizedRequests(t *testing.T) {
	router, observed := newHandlerTestRouter(t, "user-1")
	tests := []struct {
		name string
		body string
		want int
	}{
		{name: "malformed", body: `{`, want: http.StatusBadRequest},
		{name: "unsupported source", body: `{"source":"other","title":"visible"}`, want: http.StatusBadRequest},
		{name: "empty text", body: `{"source":"sonner","title":" ","description":""}`, want: http.StatusBadRequest},
		{name: "bad timestamp", body: `{"source":"sonner","title":"visible","client_timestamp":"yesterday"}`, want: http.StatusBadRequest},
		{name: "oversized", body: `{"source":"sonner","title":"` + strings.Repeat("x", maxRequestBodyBytes) + `"}`, want: http.StatusRequestEntityTooLarge},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := performFrontendErrorRequest(router, test.body)
			if response.Code != test.want {
				t.Fatalf("status = %d, want %d, body = %s", response.Code, test.want, response.Body.String())
			}
		})
	}
	if observed.Len() != 0 {
		t.Fatalf("invalid requests produced %d log entries", observed.Len())
	}
}

func TestHandlerTruncatesUTF8FieldsAndMarksEntry(t *testing.T) {
	router, observed := newHandlerTestRouter(t, "user-1")
	body, err := json.Marshal(Request{
		Source: "toast-provider", Title: strings.Repeat("🙂", titleByteLimit/2),
		TaskID: strings.Repeat("t", taskIDByteLimit+20),
	})
	if err != nil {
		t.Fatal(err)
	}
	response := performFrontendErrorRequest(router, string(body))
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	fields := observed.All()[0].ContextMap()
	title, _ := fields["title"].(string)
	taskID, _ := fields["task_id"].(string)
	if len(title) > titleByteLimit || !strings.HasSuffix(title, "🙂") {
		t.Fatalf("title truncation split UTF-8 or exceeded limit: %d bytes", len(title))
	}
	if len(taskID) != taskIDByteLimit || fields["truncated"] != true {
		t.Fatalf("task/truncated fields = %d/%#v", len(taskID), fields["truncated"])
	}
}

func TestHandlerLimitsAcceptedRequestBytes(t *testing.T) {
	router, observed := newHandlerTestRouter(t, "user-1")
	body := `{"source":"sonner","title":"visible"}` + strings.Repeat(" ", 40*1024)

	if response := performFrontendErrorRequest(router, body); response.Code != http.StatusNoContent {
		t.Fatalf("first status = %d, body = %s", response.Code, response.Body.String())
	}
	response := performFrontendErrorRequest(router, body)
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want %d, body = %s",
			response.Code, http.StatusTooManyRequests, response.Body.String())
	}
	if response.Header().Get("Retry-After") == "" {
		t.Fatal("missing Retry-After")
	}
	if observed.Len() != 1 {
		t.Fatalf("logged entries = %d, want 1", observed.Len())
	}
}

func newHandlerTestRouter(t *testing.T, userID string) (*gin.Engine, *observer.ObservedLogs) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	core, observed := observer.New(zapcore.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		authn.SetOnGin(c, authn.Identity{UserID: userID, Role: authn.RoleMember})
		c.Next()
	})
	router.POST("/api/v1/system/logs/frontend-errors", Handle(New(log, time.Now)))
	return router, observed
}

func performFrontendErrorRequest(router *gin.Engine, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/system/logs/frontend-errors", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
