package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func hierarchyRegisteredRoutes(t *testing.T) (*gin.Engine, *ws.Dispatcher, *taskrepo.Repository, chan *bus.Event) {
	t.Helper()
	_, h, repo := newQueuedTaskDTOBuilder(t)
	eventBus := bus.NewMemoryEventBus(h.logger)
	t.Cleanup(func() { eventBus.Close() })
	published := make(chan *bus.Event, 20)
	_, err := eventBus.Subscribe(events.TaskUpdated, func(_ context.Context, e *bus.Event) error { published <- e; return nil })
	require.NoError(t, err)
	svc := service.NewService(service.Repos{Workspaces: repo, Tasks: repo, TaskRepos: repo, Workflows: repo, Messages: repo, Turns: repo, Sessions: repo, GitSnapshots: repo, RepoEntities: repo, Executors: repo, Environments: repo, TaskEnvironments: repo, Reviews: repo}, eventBus, h.logger, service.RepositoryDiscoveryConfig{})
	router, dispatcher := gin.New(), ws.NewDispatcher()
	RegisterTaskRoutes(router, dispatcher, svc, nil, repo, nil, h.logger)
	for _, id := range []string{"a", "b"} {
		require.NoError(t, repo.CreateTask(context.Background(), &models.Task{ID: id, WorkspaceID: "ws-1", Title: id, Metadata: map[string]interface{}{"keep": id}}))
	}
	return router, dispatcher, repo, published
}

func TestTaskHierarchyAdmissionRegisteredREST(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, position := range []bool{false, true} {
		t.Run(fmt.Sprintf("position_%v", position), func(t *testing.T) {
			router, _, repo, published := hierarchyRegisteredRoutes(t)
			results := make(chan int, 2)
			start := make(chan struct{})
			var workers sync.WaitGroup
			for i, id := range []string{"a", "b"} {
				workers.Add(1)
				go func(i int, id string) {
					defer workers.Done()
					<-start
					payload := map[string]interface{}{"parent_id": []string{"b", "a"}[i], "title": "accepted-" + id}
					if position {
						payload["position"] = 20 + i
					}
					encoded, _ := json.Marshal(payload)
					response := httptest.NewRecorder()
					req := httptest.NewRequest(http.MethodPatch, "/api/v1/tasks/"+id, strings.NewReader(string(encoded)))
					req.Header.Set("Content-Type", "application/json")
					router.ServeHTTP(response, req)
					results <- response.Code
				}(i, id)
			}
			close(start)
			workers.Wait()
			close(results)
			successes, rejections := 0, 0
			for code := range results {
				switch code {
				case http.StatusOK:
					successes++
				case http.StatusBadRequest:
					rejections++
				default:
					t.Errorf("unexpected status %d", code)
				}
			}
			require.Equal(t, 1, successes)
			require.Equal(t, 1, rejections)
			require.Len(t, published, 1)
			a, err := repo.GetTask(context.Background(), "a")
			require.NoError(t, err)
			b, err := repo.GetTask(context.Background(), "b")
			require.NoError(t, err)
			require.False(t, a.ParentID == "b" && b.ParentID == "a")
			for _, task := range []*models.Task{a, b} {
				if task.ParentID == "" {
					require.Equal(t, task.ID, task.Title)
					require.Zero(t, task.Position)
					require.Equal(t, task.ID, task.Metadata["keep"])
				}
			}
			// Omission must preserve the accepted parent and ordinary metadata; an
			// explicit empty parent must publish the existing clear-parent marker.
			winner := a
			if winner.ParentID == "" {
				winner = b
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodPatch, "/api/v1/tasks/"+winner.ID, strings.NewReader(`{"description":"Requested"}`)))
			require.Equal(t, http.StatusOK, response.Code)
			current, err := repo.GetTask(context.Background(), winner.ID)
			require.NoError(t, err)
			require.Equal(t, winner.ParentID, current.ParentID)
			response = httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodPatch, "/api/v1/tasks/"+winner.ID, strings.NewReader(`{"parent_id":""}`)))
			require.Equal(t, http.StatusOK, response.Code)
			var last *bus.Event
			for len(published) > 0 {
				last = <-published
			}
			data := last.Data.(map[string]interface{})
			parent, present := data["parent_id"]
			require.True(t, present)
			require.Nil(t, parent)
		})
	}
}

func TestTaskHierarchyAdmissionRegisteredWS(t *testing.T) {
	for _, position := range []bool{false, true} {
		t.Run(fmt.Sprintf("position_%v", position), func(t *testing.T) {
			_, dispatcher, repo, published := hierarchyRegisteredRoutes(t)
			results := make(chan *ws.Message, 2)
			start := make(chan struct{})
			var workers sync.WaitGroup
			for i, id := range []string{"a", "b"} {
				workers.Add(1)
				go func(i int, id string) {
					defer workers.Done()
					<-start
					payload := map[string]interface{}{"id": id, "parent_id": []string{"b", "a"}[i]}
					if position {
						payload["position"] = 30 + i
					}
					request, err := ws.NewRequest("move-"+id, ws.ActionTaskUpdate, payload)
					if err != nil {
						t.Error(err)
						return
					}
					response, err := dispatcher.Dispatch(context.Background(), request)
					if err != nil {
						t.Error(err)
						return
					}
					results <- response
				}(i, id)
			}
			close(start)
			workers.Wait()
			close(results)
			successes, rejections := 0, 0
			for response := range results {
				if response.Type == ws.MessageTypeResponse {
					successes++
				} else {
					var failure ws.ErrorPayload
					require.NoError(t, response.ParsePayload(&failure))
					require.Equal(t, ws.ErrorCodeValidation, failure.Code)
					rejections++
				}
			}
			require.Equal(t, 1, successes)
			require.Equal(t, 1, rejections)
			require.Len(t, published, 1)
			a, err := repo.GetTask(context.Background(), "a")
			require.NoError(t, err)
			b, err := repo.GetTask(context.Background(), "b")
			require.NoError(t, err)
			require.False(t, a.ParentID == "b" && b.ParentID == "a")

			winner := a
			if winner.ParentID == "" {
				winner = b
			}
			request, err := ws.NewRequest("rename", ws.ActionTaskUpdate, map[string]interface{}{"id": winner.ID, "title": "Renamed"})
			require.NoError(t, err)
			response, err := dispatcher.Dispatch(context.Background(), request)
			require.NoError(t, err)
			require.Equal(t, ws.MessageTypeResponse, response.Type)
			current, err := repo.GetTask(context.Background(), winner.ID)
			require.NoError(t, err)
			require.Equal(t, winner.ParentID, current.ParentID)
			request, err = ws.NewRequest("clear", ws.ActionTaskUpdate, map[string]interface{}{"id": winner.ID, "parent_id": ""})
			require.NoError(t, err)
			response, err = dispatcher.Dispatch(context.Background(), request)
			require.NoError(t, err)
			require.Equal(t, ws.MessageTypeResponse, response.Type)
			var last *bus.Event
			for len(published) > 0 {
				last = <-published
			}
			data := last.Data.(map[string]interface{})
			parent, present := data["parent_id"]
			require.True(t, present)
			require.Nil(t, parent)
		})
	}
}
