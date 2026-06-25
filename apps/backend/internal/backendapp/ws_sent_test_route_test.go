package backendapp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
	gateways "github.com/kandev/kandev/internal/gateway/websocket"
)

func TestRegisterWsSentTestRouteReturnsConnectionEvents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	source := &fakeWsSentSource{
		connectionEvents: []gateways.WsSentEvent{
			{ConnectionSeq: 2, Type: "notification", Action: "task.updated", SentAt: time.Unix(2, 0).UTC()},
		},
		connectionMax: 4,
	}
	registerWsSentTestRoute(router, source, logger.Default())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/_test/ws-sent?connection_id=conn-1&since_seq=1", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s; want 200", rec.Code, rec.Body.String())
	}
	if source.connectionID != "conn-1" || source.sinceSeq != 1 {
		t.Fatalf("source args = connection %q since %d; want conn-1/1", source.connectionID, source.sinceSeq)
	}
	want := `"max_connection_seq":4`
	if body := rec.Body.String(); !strings.Contains(body, want) || !strings.Contains(body, `"connection_seq":2`) {
		t.Fatalf("response body %s missing expected sent-log fields", body)
	}
}

func TestRegisterWsSentTestRouteReturnsSessionEvents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	source := &fakeWsSentSource{
		sessionEvents: []gateways.WsSentEvent{
			{
				ConnectionSeq: 3,
				SessionSeq:    2,
				SessionID:     "session-1",
				Type:          "notification",
				Action:        "session.message.added",
				SentAt:        time.Unix(3, 0).UTC(),
			},
		},
		sessionMax: 2,
	}
	registerWsSentTestRoute(router, source, logger.Default())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/_test/ws-sent?connection_id=conn-1&session_id=session-1", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s; want 200", rec.Code, rec.Body.String())
	}
	if source.sessionConnectionID != "conn-1" || source.sessionID != "session-1" {
		t.Fatalf("source session args = %q/%q; want conn-1/session-1", source.sessionConnectionID, source.sessionID)
	}
	if body := rec.Body.String(); !strings.Contains(body, `"max_session_seq":2`) || !strings.Contains(body, `"session_seq":2`) {
		t.Fatalf("response body %s missing expected session sent-log fields", body)
	}
}

func TestRegisterWsSentTestRouteRequiresConnectionID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerWsSentTestRoute(router, &fakeWsSentSource{}, logger.Default())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/_test/ws-sent", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400", rec.Code)
	}
}

type fakeWsSentSource struct {
	connectionID     string
	sinceSeq         int64
	connectionEvents []gateways.WsSentEvent
	connectionMax    int64

	sessionConnectionID string
	sessionID           string
	sessionEvents       []gateways.WsSentEvent
	sessionMax          int64
}

func (f *fakeWsSentSource) GetSentEventsFor(connectionID string, sinceSeq int64) ([]gateways.WsSentEvent, int64, bool) {
	f.connectionID = connectionID
	f.sinceSeq = sinceSeq
	return f.connectionEvents, f.connectionMax, true
}

func (f *fakeWsSentSource) GetSentEventsForSession(connectionID, sessionID string) ([]gateways.WsSentEvent, int64, bool) {
	f.sessionConnectionID = connectionID
	f.sessionID = sessionID
	return f.sessionEvents, f.sessionMax, true
}
