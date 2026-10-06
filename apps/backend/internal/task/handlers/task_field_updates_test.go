package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/logger"
	internaldb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type protocolFieldReadGate struct {
	*taskrepo.Repository
	armed            atomic.Bool
	arrived, release chan struct{}
}

func (r *protocolFieldReadGate) GetTask(ctx context.Context, id string) (*models.Task, error) {
	task, err := r.Repository.GetTask(ctx, id)
	if err != nil || id != "protocol-fields" || !r.armed.Swap(false) {
		return task, err
	}
	close(r.arrived)
	select {
	case <-r.release:
		return task, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type fieldProtocolFixture struct {
	router     *gin.Engine
	dispatcher *ws.Dispatcher
	gate       *protocolFieldReadGate
	published  chan *bus.Event
	service    *service.Service
}

func fieldProtocolPair(t *testing.T, owners ...string) [2]fieldProtocolFixture {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "field-protocol.db")
	var pair [2]fieldProtocolFixture
	for i := range pair {
		raw, err := internaldb.OpenSQLite(path)
		require.NoError(t, err)
		db := sqlx.NewDb(raw, "sqlite3")
		t.Cleanup(func() { _ = db.Close() })
		var repo *taskrepo.Repository
		if i == 0 {
			var cleanup func() error
			repo, cleanup, err = repository.Provide(db, db, nil)
			require.NoError(t, err)
			t.Cleanup(func() { _ = cleanup() })
		} else {
			repo = taskrepo.NewWithInitializedDB(db, db, nil)
		}
		gate := &protocolFieldReadGate{Repository: repo, arrived: make(chan struct{}), release: make(chan struct{})}
		eventBus := bus.NewMemoryEventBus(log)
		t.Cleanup(func() { eventBus.Close() })
		published := make(chan *bus.Event, 30)
		_, err = eventBus.Subscribe(events.TaskUpdated, func(_ context.Context, e *bus.Event) error { published <- e; return nil })
		require.NoError(t, err)
		svc := service.NewService(service.Repos{Workspaces: repo, Tasks: gate, TaskRepos: repo, Workflows: repo, Messages: repo, Turns: repo, Sessions: repo, GitSnapshots: repo, RepoEntities: repo, Executors: repo, Environments: repo, TaskEnvironments: repo, Reviews: repo, ResourceCleanups: repo}, eventBus, log, service.RepositoryDiscoveryConfig{})
		require.NoError(t, svc.StartTaskResourceCleanupWorker(context.Background()))
		t.Cleanup(svc.StopTaskResourceCleanupWorker)
		router, dispatcher := gin.New(), ws.NewDispatcher()
		RegisterTaskRoutes(router, dispatcher, svc, nil, repo, nil, log)
		pair[i] = fieldProtocolFixture{router, dispatcher, gate, published, svc}
	}
	repo := pair[0].gate.Repository
	var owner string
	if len(owners) > 0 {
		owner = owners[0]
	}
	require.NoError(t, repo.CreateWorkspace(context.Background(), &models.Workspace{ID: "protocol-ws", Name: "Protocol fields", OwnerID: owner}))
	for _, id := range []string{"protocol-fields", "protocol-parent"} {
		require.NoError(t, repo.CreateTask(context.Background(), &models.Task{ID: id, WorkspaceID: "protocol-ws", Title: "Original", Description: "Original description", Priority: "medium", Metadata: map[string]interface{}{"keep": "current"}}))
	}
	return pair
}

func (f fieldProtocolFixture) request(ctx context.Context, transport string, payload map[string]interface{}) (map[string]interface{}, bool, error) {
	if transport == "REST" {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, false, err
		}
		request := httptest.NewRequest(http.MethodPatch, "/api/v1/tasks/protocol-fields", strings.NewReader(string(encoded))).WithContext(ctx)
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		f.router.ServeHTTP(response, request)
		var body map[string]interface{}
		err = json.Unmarshal(response.Body.Bytes(), &body)
		return body, response.Code == http.StatusOK, err
	}
	payload["id"] = "protocol-fields"
	request, err := ws.NewRequest("field-update", ws.ActionTaskUpdate, payload)
	if err != nil {
		return nil, false, err
	}
	response, err := f.dispatcher.Dispatch(ctx, request)
	if err != nil {
		return nil, false, err
	}
	var body map[string]interface{}
	err = response.ParsePayload(&body)
	return body, response.Type == ws.MessageTypeResponse, err
}

// @covers AC-TASKS-FIELD-UPDATES-001.1, AC-TASKS-FIELD-UPDATES-001.2, AC-TASKS-FIELD-UPDATES-001.3, AC-TASKS-FIELD-UPDATES-001.7
func TestTaskFieldUpdatesRegisteredProtocols(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, titleTransport := range []string{"REST", "WS"} {
		for _, first := range []int{0, 1} {
			t.Run(fmt.Sprintf("title_%s_first_%d", titleTransport, first), func(t *testing.T) {
				pair := fieldProtocolPair(t)
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				var workers sync.WaitGroup
				defer func() { cancel(); workers.Wait() }()
				transports := []string{titleTransport, "REST"}
				if titleTransport == "REST" {
					transports[1] = "WS"
				}
				payloads := []map[string]interface{}{{"title": "Concurrent title"}, {"description": "Concurrent description"}}
				type outcome struct {
					body map[string]interface{}
					ok   bool
					err  error
				}
				results := [2]chan outcome{make(chan outcome, 1), make(chan outcome, 1)}
				for i := range pair {
					pair[i].gate.armed.Store(true)
					workers.Add(1)
					go func(i int) {
						defer workers.Done()
						body, ok, err := pair[i].request(ctx, transports[i], payloads[i])
						results[i] <- outcome{body, ok, err}
					}(i)
				}
				for _, f := range pair {
					select {
					case <-f.gate.arrived:
					case <-ctx.Done():
						t.Fatal("registered request did not reach real read", ctx.Err())
					}
				}
				for n, i := range []int{first, 1 - first} {
					close(pair[i].gate.release)
					var result outcome
					select {
					case result = <-results[i]:
					case <-ctx.Done():
						t.Fatal("registered request did not settle", ctx.Err())
					}
					require.NoError(t, result.err)
					require.True(t, result.ok, result.body)
					if n == 1 {
						require.Equal(t, "Concurrent title", result.body["title"])
						require.Equal(t, "Concurrent description", result.body["description"])
					}
					require.Len(t, pair[i].published, 1)
					e := <-pair[i].published
					data := e.Data.(map[string]interface{})
					require.Equal(t, result.body["title"], data["title"])
					require.Equal(t, result.body["description"], data["description"])
				}
				workers.Wait()
				current, err := pair[0].gate.Repository.GetTask(ctx, "protocol-fields")
				require.NoError(t, err)
				require.Equal(t, "Concurrent title", current.Title)
				require.Equal(t, "Concurrent description", current.Description)
				for _, transport := range []string{"REST", "WS"} {
					body, ok, err := pair[0].request(ctx, transport, map[string]interface{}{"title": nil, "description": nil, "metadata": nil, "repositories": nil, "parent_id": nil, "assignee_user_id": nil})
					require.NoError(t, err)
					require.True(t, ok, body)
					require.Equal(t, current.Title, body["title"])
					require.Equal(t, current.Description, body["description"])
					<-pair[0].published
					body, ok, err = pair[0].request(ctx, transport, map[string]interface{}{"title": "", "description": "", "metadata": map[string]interface{}{}, "repositories": []interface{}{}, "parent_id": "", "assignee_user_id": ""})
					require.NoError(t, err)
					require.True(t, ok, body)
					require.Equal(t, "", body["title"])
					require.Equal(t, "", body["description"])
					event := <-pair[0].published
					data := event.Data.(map[string]interface{})
					require.Equal(t, "", data["title"])
					persisted, err := pair[0].gate.Repository.GetTask(ctx, "protocol-fields")
					require.NoError(t, err)
					require.Empty(t, persisted.Metadata)
					require.Empty(t, persisted.Repositories)
					require.Empty(t, persisted.AssigneeUserID)
					current = persisted
					body, ok, err = pair[0].request(ctx, transport, map[string]interface{}{"title": "Must reject", "priority": "invalid"})
					require.NoError(t, err)
					require.False(t, ok, body)
					require.Empty(t, pair[0].published)
					persisted, err = pair[0].gate.Repository.GetTask(ctx, "protocol-fields")
					require.NoError(t, err)
					require.Equal(t, current.Title, persisted.Title)
					require.True(t, current.UpdatedAt.Equal(persisted.UpdatedAt))
				}
			})
		}
	}
}
