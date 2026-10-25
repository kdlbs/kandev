package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	internaldb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/testutil"
	ws "github.com/kandev/kandev/pkg/websocket"
)

var workflowHandlerTemplate = testutil.NewSQLiteTemplate(func(db *sqlx.DB) error {
	_, err := sqliterepo.NewWithDB(db, db, nil)
	return err
})

type registeredWorkflowBarrier struct {
	repository.WorkflowRepository
	remaining atomic.Int32
	read      chan struct{}
	resume    chan struct{}
}

func (r *registeredWorkflowBarrier) GetWorkflow(ctx context.Context, id string) (*models.Workflow, error) {
	workflow, err := r.WorkflowRepository.GetWorkflow(ctx, id)
	if err != nil || r.remaining.Add(-1) != 0 {
		return workflow, err
	}
	close(r.read)
	select {
	case <-r.resume:
		return workflow, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type workflowTransportFixture struct {
	repo *sqliterepo.Repository
	gate *registeredWorkflowBarrier
	http *gin.Engine
	ws   *ws.Dispatcher
	mu   sync.Mutex
	seen []*bus.Event
}

func registeredWorkflowFixtures(t *testing.T) [2]*workflowTransportFixture {
	t.Helper()
	first, path := workflowHandlerTemplate.Open(t)
	raw, err := internaldb.OpenSQLite(path)
	require.NoError(t, err)
	second := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { require.NoError(t, second.Close()) })
	var fixtures [2]*workflowTransportFixture
	for i, db := range []*sqlx.DB{first, second} {
		log := newTestLogger(t)
		repo := sqliterepo.NewWithInitializedDB(db, db, nil)
		f := &workflowTransportFixture{repo: repo, http: gin.New(), ws: ws.NewDispatcher()}
		f.gate = &registeredWorkflowBarrier{WorkflowRepository: repo, read: make(chan struct{}), resume: make(chan struct{})}
		eventBus := bus.NewMemoryEventBus(log)
		t.Cleanup(eventBus.Close)
		_, err := eventBus.Subscribe(events.WorkflowUpdated, func(_ context.Context, event *bus.Event) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.seen = append(f.seen, event)
			return nil
		})
		require.NoError(t, err)
		svc := service.NewService(service.Repos{Workflows: f.gate, Workspaces: repo}, eventBus, log, service.RepositoryDiscoveryConfig{})
		RegisterWorkflowRoutes(f.http, f.ws, svc, nil, log)
		fixtures[i] = f
	}
	require.NoError(t, fixtures[0].repo.CreateWorkspace(context.Background(), &models.Workspace{ID: "transport-ws", Name: "Transport"}))
	require.NoError(t, fixtures[0].repo.CreateWorkflow(context.Background(), &models.Workflow{
		ID: "transport-fields", WorkspaceID: "transport-ws", Name: "before name", Prompt: "before prompt", Description: "keep description",
	}))
	return fixtures
}

func (f *workflowTransportFixture) update(ctx context.Context, t *testing.T, transport, payload string) (dto.WorkflowDTO, bool) {
	t.Helper()
	var result dto.WorkflowDTO
	if transport == "HTTP" {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/workflows/transport-fields", strings.NewReader(payload)).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		f.http.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			return result, false
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
		return result, true
	}
	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(payload), &body))
	body["id"] = "transport-fields"
	response, err := f.ws.Dispatch(ctx, wsWorkflowRequest(t, ws.ActionWorkflowUpdate, body))
	require.NoError(t, err)
	if response.Type != ws.MessageTypeResponse {
		return result, false
	}
	wsWorkflowResponse(t, response, &result)
	return result, true
}

func workflowDTOString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func assertWorkflowTransportEvent(t *testing.T, f *workflowTransportFixture, result dto.WorkflowDTO) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	require.Len(t, f.seen, 1)
	data := f.seen[0].Data.(map[string]interface{})
	require.Equal(t, result.Name, data["name"])
	require.Equal(t, workflowDTOString(result.Prompt), data["prompt"])
	require.Equal(t, workflowDTOString(result.Description), data["description"])
}

func registeredWorkflowOverlap(t *testing.T, transport string) {
	t.Helper()
	for held := range 2 {
		t.Run(fmt.Sprintf("held_field_%d", held), func(t *testing.T) {
			fixtures := registeredWorkflowFixtures(t)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			gate := fixtures[held].gate
			// The registered read-only guard reads first; the service's own SQL
			// read is held second, immediately before the mutation seam.
			gate.remaining.Store(2)
			var workers sync.WaitGroup
			var release sync.Once
			defer func() { release.Do(func() { close(gate.resume) }); cancel(); workers.Wait() }()
			payloads := [2]string{`{"name":"saved name"}`, `{"prompt":"saved prompt"}`}
			var results [2]dto.WorkflowDTO
			done := make(chan bool, 1)
			workers.Go(func() {
				var ok bool
				results[held], ok = fixtures[held].update(ctx, t, transport, payloads[held])
				done <- ok
			})
			select {
			case <-gate.read:
			case <-ctx.Done():
				t.Fatal("registered service read did not reach barrier", ctx.Err())
			}
			var ok bool
			results[1-held], ok = fixtures[1-held].update(ctx, t, transport, payloads[1-held])
			require.True(t, ok)
			release.Do(func() { close(gate.resume) })
			select {
			case ok := <-done:
				require.True(t, ok)
			case <-ctx.Done():
				t.Fatal("registered held update did not settle", ctx.Err())
			}
			stored, err := fixtures[0].repo.GetWorkflow(ctx, "transport-fields")
			require.NoError(t, err)
			require.Equal(t, "saved name", stored.Name)
			require.Equal(t, "saved prompt", stored.Prompt)
			require.Equal(t, stored.Name, results[held].Name)
			require.Equal(t, stored.Prompt, workflowDTOString(results[held].Prompt))
			for i, f := range fixtures {
				assertWorkflowTransportEvent(t, f, results[i])
			}
		})
	}
	t.Run("empty_and_read_only", func(t *testing.T) {
		fixtures := registeredWorkflowFixtures(t)
		f := fixtures[0]
		ctx := context.Background()
		result, ok := f.update(ctx, t, transport, `{"prompt":"","agent_profile_id":""}`)
		require.True(t, ok)
		require.Empty(t, result.Prompt)
		require.Equal(t, "before name", result.Name)
		assertWorkflowTransportEvent(t, f, result)
		before, err := f.repo.GetWorkflow(ctx, "transport-fields")
		require.NoError(t, err)
		source := models.WorkflowSourceGitHub
		_, err = f.repo.UpdateWorkflowFields(ctx, before.ID, models.WorkflowFieldUpdate{Source: &source})
		require.NoError(t, err)
		_, ok = f.update(ctx, t, transport, `{"name":"forbidden"}`)
		require.False(t, ok)
		stored, err := f.repo.GetWorkflow(ctx, before.ID)
		require.NoError(t, err)
		require.Equal(t, before.Name, stored.Name)
		assertWorkflowTransportEvent(t, f, result)
	})
	t.Run("null_and_empty_request", func(t *testing.T) {
		for _, payload := range []string{`{"name":null,"description":null,"prompt":null,"agent_profile_id":null}`, `{}`} {
			t.Run(payload, func(t *testing.T) {
				fixtures := registeredWorkflowFixtures(t)
				ctx := context.Background()
				before, err := fixtures[0].repo.GetWorkflow(ctx, "transport-fields")
				require.NoError(t, err)
				result, ok := fixtures[0].update(ctx, t, transport, payload)
				require.True(t, ok)
				stored, err := fixtures[1].repo.GetWorkflow(ctx, before.ID)
				require.NoError(t, err)
				require.True(t, stored.UpdatedAt.After(before.UpdatedAt))
				stored.UpdatedAt = before.UpdatedAt
				require.Equal(t, before, stored, "null and omission preserve every stored field")
				assertWorkflowTransportEvent(t, fixtures[0], result)
			})
		}
	})
	t.Run("same_field_last_value", func(t *testing.T) {
		fixtures := registeredWorkflowFixtures(t)
		ctx := context.Background()
		first, ok := fixtures[0].update(ctx, t, transport, `{"prompt":"first saved prompt"}`)
		require.True(t, ok)
		second, ok := fixtures[1].update(ctx, t, transport, `{"prompt":"last saved prompt"}`)
		require.True(t, ok)
		stored, err := fixtures[0].repo.GetWorkflow(ctx, "transport-fields")
		require.NoError(t, err)
		require.Equal(t, "last saved prompt", stored.Prompt)
		require.Equal(t, "before name", stored.Name)
		assertWorkflowTransportEvent(t, fixtures[0], first)
		assertWorkflowTransportEvent(t, fixtures[1], second)
	})
}

// @covers AC-TASKS-FIELD-UPDATES-002.4, AC-TASKS-FIELD-UPDATES-002.5
func TestRegisteredWorkflowFieldUpdatesHTTP(t *testing.T) { registeredWorkflowOverlap(t, "HTTP") }

// @covers AC-TASKS-FIELD-UPDATES-002.4, AC-TASKS-FIELD-UPDATES-002.5
func TestRegisteredWorkflowFieldUpdatesWS(t *testing.T) { registeredWorkflowOverlap(t, "WS") }
