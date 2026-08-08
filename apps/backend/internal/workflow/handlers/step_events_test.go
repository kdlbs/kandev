package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/workflow/controller"
	"github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/workflow/repository"
	"github.com/kandev/kandev/internal/workflow/service"
)

type stepEvent struct {
	subject string
	source  string
	step    map[string]interface{}
}

type stepEventRecorder struct {
	events []stepEvent
}

// recordStepEvents subscribes to every workflow_step.* subject so a test can
// assert not only that the expected event fired but that no extra one did — a
// duplicate publish added elsewhere in the stack shows up here.
func recordStepEvents(t *testing.T, eventBus *bus.MemoryEventBus) *stepEventRecorder {
	t.Helper()
	rec := &stepEventRecorder{}
	for _, subject := range []string{events.WorkflowStepCreated, events.WorkflowStepUpdated, events.WorkflowStepDeleted} {
		if _, err := eventBus.Subscribe(subject, func(_ context.Context, ev *bus.Event) error {
			data, ok := ev.Data.(map[string]interface{})
			if !ok {
				t.Errorf("event data type = %T, want map", ev.Data)
				return nil
			}
			step, ok := data["step"].(map[string]interface{})
			if !ok {
				t.Errorf("step payload type = %T, want map", data["step"])
				return nil
			}
			rec.events = append(rec.events, stepEvent{subject: ev.Subject, source: ev.Source, step: step})
			return nil
		}); err != nil {
			t.Fatalf("subscribe %s: %v", subject, err)
		}
	}
	return rec
}

func (r *stepEventRecorder) subjects() []string {
	subjects := make([]string, 0, len(r.events))
	for _, ev := range r.events {
		subjects = append(subjects, ev.subject)
	}
	return subjects
}

func (r *stepEventRecorder) only(t *testing.T, subject string) stepEvent {
	t.Helper()
	if len(r.events) != 1 {
		t.Fatalf("published %v, want exactly one %s", r.subjects(), subject)
	}
	if r.events[0].subject != subject {
		t.Fatalf("subject = %q, want %q", r.events[0].subject, subject)
	}
	if r.events[0].source != stepEventSource {
		t.Fatalf("source = %q, want %q", r.events[0].source, stepEventSource)
	}
	return r.events[0]
}

func setupStepRouter(t *testing.T) (*gin.Engine, *bus.MemoryEventBus, *repository.Repository) {
	t.Helper()
	rawDB, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// Pin the pool to one connection: every connection to an in-memory SQLite
	// DB gets its own database, so a second one would not see the schema.
	rawDB.SetMaxOpenConns(1)
	db := sqlx.NewDb(rawDB, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })

	// The workflows table is owned by the task repo; the workflow repo's schema
	// init reads it while seeding, so create it for the test.
	if _, err = db.Exec(`CREATE TABLE IF NOT EXISTS workflows (
		id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL DEFAULT '',
		workflow_template_id TEXT DEFAULT '', name TEXT NOT NULL,
		description TEXT DEFAULT '', created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL
	)`); err != nil {
		t.Fatalf("create workflows table: %v", err)
	}

	repo, err := repository.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repository: %v", err)
	}
	log := logger.Default()
	eventBus := bus.NewMemoryEventBus(log)
	h := NewHandlers(controller.NewController(service.NewService(repo, log)), eventBus, log)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	h.registerHTTP(router)
	return router, eventBus, repo
}

func doJSON(t *testing.T, router *gin.Engine, method, path string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func createStepViaHTTP(t *testing.T, router *gin.Engine, body map[string]interface{}) *models.WorkflowStep {
	t.Helper()
	rec := doJSON(t, router, http.MethodPost, "/api/v1/workflow/steps", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var step models.WorkflowStep
	if err := json.Unmarshal(rec.Body.Bytes(), &step); err != nil {
		t.Fatalf("decode created step: %v", err)
	}
	return &step
}

func TestHTTPCreateStepPublishesCreatedEvent(t *testing.T) {
	router, eventBus, _ := setupStepRouter(t)
	rec := recordStepEvents(t, eventBus)

	step := createStepViaHTTP(t, router, map[string]interface{}{
		"workflow_id": "workflow-1",
		"name":        "Backlog",
		"position":    0,
		"color":       "#123456",
	})

	got := rec.only(t, events.WorkflowStepCreated)
	if got.step["id"] != step.ID {
		t.Errorf("event step id = %v, want %q", got.step["id"], step.ID)
	}
	if got.step["workflow_id"] != "workflow-1" || got.step["name"] != "Backlog" {
		t.Errorf("unexpected step payload: %#v", got.step)
	}
}

func TestHTTPUpdateStepPublishesUpdatedEvent(t *testing.T) {
	router, eventBus, _ := setupStepRouter(t)
	step := createStepViaHTTP(t, router, map[string]interface{}{
		"workflow_id": "workflow-1",
		"name":        "Backlog",
		"position":    0,
	})
	rec := recordStepEvents(t, eventBus)

	resp := doJSON(t, router, http.MethodPut, "/api/v1/workflow/steps/"+step.ID, map[string]interface{}{
		"name":      "Triage",
		"wip_limit": 3,
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", resp.Code, resp.Body.String())
	}

	got := rec.only(t, events.WorkflowStepUpdated)
	if got.step["id"] != step.ID || got.step["name"] != "Triage" {
		t.Errorf("unexpected step payload: %#v", got.step)
	}
	if wip, ok := got.step["wip_limit"].(int); !ok || wip != 3 {
		t.Errorf("wip_limit = %#v, want 3", got.step["wip_limit"])
	}
}

func TestHTTPDeleteStepPublishesDeletedEvent(t *testing.T) {
	router, eventBus, _ := setupStepRouter(t)
	step := createStepViaHTTP(t, router, map[string]interface{}{
		"workflow_id": "workflow-1",
		"name":        "Backlog",
		"position":    0,
	})
	rec := recordStepEvents(t, eventBus)

	resp := doJSON(t, router, http.MethodDelete, "/api/v1/workflow/steps/"+step.ID, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", resp.Code, resp.Body.String())
	}

	got := rec.only(t, events.WorkflowStepDeleted)
	if got.step["id"] != step.ID {
		t.Errorf("event step id = %v, want %q", got.step["id"], step.ID)
	}
}

func TestHTTPCreateStepPublishesDemotedStartSteps(t *testing.T) {
	router, eventBus, _ := setupStepRouter(t)
	oldStart := createStepViaHTTP(t, router, map[string]interface{}{
		"workflow_id":   "workflow-1",
		"name":          "Old Start",
		"position":      0,
		"is_start_step": true,
	})
	rec := recordStepEvents(t, eventBus)

	newStart := createStepViaHTTP(t, router, map[string]interface{}{
		"workflow_id":   "workflow-1",
		"name":          "New Start",
		"position":      1,
		"is_start_step": true,
	})

	if len(rec.events) != 2 {
		t.Fatalf("published %v, want one updated (demoted) plus one created", rec.subjects())
	}
	demoted, created := rec.events[0], rec.events[1]
	if demoted.subject != events.WorkflowStepUpdated || demoted.step["id"] != oldStart.ID {
		t.Errorf("first event = %s for step %v, want %s for %q",
			demoted.subject, demoted.step["id"], events.WorkflowStepUpdated, oldStart.ID)
	}
	if isStart, ok := demoted.step["is_start_step"].(bool); !ok || isStart {
		t.Errorf("demoted step is_start_step = %#v, want false", demoted.step["is_start_step"])
	}
	if created.subject != events.WorkflowStepCreated || created.step["id"] != newStart.ID {
		t.Errorf("second event = %s for step %v, want %s for %q",
			created.subject, created.step["id"], events.WorkflowStepCreated, newStart.ID)
	}
}

func TestHTTPUpdateStepPublishesDemotedStartSteps(t *testing.T) {
	router, eventBus, _ := setupStepRouter(t)
	oldStart := createStepViaHTTP(t, router, map[string]interface{}{
		"workflow_id":   "workflow-1",
		"name":          "Old Start",
		"position":      0,
		"is_start_step": true,
	})
	promoted := createStepViaHTTP(t, router, map[string]interface{}{
		"workflow_id": "workflow-1",
		"name":        "Doing",
		"position":    1,
	})
	rec := recordStepEvents(t, eventBus)

	resp := doJSON(t, router, http.MethodPut, "/api/v1/workflow/steps/"+promoted.ID, map[string]interface{}{
		"is_start_step": true,
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", resp.Code, resp.Body.String())
	}

	if len(rec.events) != 2 {
		t.Fatalf("published %v, want one updated per demoted step plus one for the updated step", rec.subjects())
	}
	if rec.events[0].step["id"] != oldStart.ID || rec.events[1].step["id"] != promoted.ID {
		t.Errorf("event step ids = %v, %v; want %q then %q",
			rec.events[0].step["id"], rec.events[1].step["id"], oldStart.ID, promoted.ID)
	}
	for _, ev := range rec.events {
		if ev.subject != events.WorkflowStepUpdated {
			t.Errorf("subject = %q, want %q", ev.subject, events.WorkflowStepUpdated)
		}
	}
}

func TestHTTPStepMutationFailurePublishesNothing(t *testing.T) {
	router, eventBus, _ := setupStepRouter(t)
	step := createStepViaHTTP(t, router, map[string]interface{}{
		"workflow_id": "workflow-1",
		"name":        "Backlog",
		"position":    0,
	})
	rec := recordStepEvents(t, eventBus)

	resp := doJSON(t, router, http.MethodPut, "/api/v1/workflow/steps/"+step.ID, map[string]interface{}{
		"wip_limit": -1,
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("update status = %d, want 400; body = %s", resp.Code, resp.Body.String())
	}
	if len(rec.events) != 0 {
		t.Fatalf("published %v for a rejected update, want none", rec.subjects())
	}
}
