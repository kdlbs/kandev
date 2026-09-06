package persistence

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/persistence/requiredstores"
)

func TestHandlerReturnsStableRowsWithSanitizedErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tracker, err := requiredstores.NewTracker([]requiredstores.Descriptor{
		{ID: "first", OwnerPackage: "owner/first", RequiredTables: []string{"first"}},
		{ID: "second", OwnerPackage: "owner/second", RequiredTables: []string{"second"}},
	})
	if err != nil {
		t.Fatalf("NewTracker: %v", err)
	}
	if err := tracker.RecordSuccess("first"); err != nil {
		t.Fatalf("RecordSuccess(first): %v", err)
	}
	if err := tracker.RecordSuccess("second"); err != nil {
		t.Fatalf("RecordSuccess(second): %v", err)
	}
	if err := tracker.RecordProbe("first", persistenceDatabaseError("password=secret host=db.example")); err != nil {
		t.Fatalf("RecordProbe(first): %v", err)
	}
	if err := tracker.RecordProbe("second", nil); err != nil {
		t.Fatalf("RecordProbe(second): %v", err)
	}

	handler := NewHandler(tracker, requiredstores.NewHealth(tracker, nil, nil), "pgx")
	recorder := httptest.NewRecorder()
	router := gin.New()
	router.GET("/diagnostics/persistence", handler.Handle)
	router.ServeHTTP(recorder, httptest.NewRequest("GET", "/diagnostics/persistence", nil))
	if recorder.Code != 200 {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	var body Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Driver != "pgx" || body.State != string(requiredstores.StateUnhealthy) {
		t.Fatalf("header = %#v, want pgx/unhealthy", body)
	}
	if len(body.Stores) != 2 || body.Stores[0].ID != "first" || body.Stores[1].ID != "second" {
		t.Fatalf("stores = %#v, want catalog order", body.Stores)
	}
	if body.Stores[0].Error != "database probe failed" {
		t.Fatalf("error = %q, want sanitized error", body.Stores[0].Error)
	}
}

type persistenceDatabaseError string

func (e persistenceDatabaseError) Error() string { return string(e) }
