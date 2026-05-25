package jobs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newJobsRouter(tracker *Tracker) *gin.Engine {
	r := gin.New()
	g := r.Group("/api/v1/system")
	g.GET("/jobs/:id", HandleGet(tracker))
	return r
}

func TestHandleGet_UnknownJobReturns404(t *testing.T) {
	tracker := NewTracker(nil, newTestLogger())
	r := newJobsRouter(tracker)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/jobs/missing-id", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestHandleGet_KnownJobReturns200(t *testing.T) {
	tracker := NewTracker(nil, newTestLogger())
	id := tracker.Start(context.TODO(), "vacuum", func(_ context.Context) (map[string]interface{}, error) {
		return map[string]interface{}{"ok": true}, nil
	})
	waitForState(t, tracker, id, StateSucceeded)

	r := newJobsRouter(tracker)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/jobs/"+id, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var job Job
	if err := json.Unmarshal(w.Body.Bytes(), &job); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if job.ID != id || job.State != StateSucceeded {
		t.Errorf("job = %+v", job)
	}
}
