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
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func portMergeRequest(ctx context.Context, f fieldProtocolFixture) (map[string]interface{}, error) {
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/tasks/protocol-fields/port-forwarding", strings.NewReader(`{"enabled":true}`)).WithContext(ctx)
	response := httptest.NewRecorder()
	f.router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		return nil, fmt.Errorf("status %d: %s", response.Code, response.Body.String())
	}
	var body map[string]interface{}
	err := json.Unmarshal(response.Body.Bytes(), &body)
	return body, err
}

// @covers AC-TASKS-FIELD-UPDATES-001.8, AC-TASKS-FIELD-UPDATES-001.9, AC-TASKS-FIELD-UPDATES-001.12
func TestTaskMetadataMergeRegisteredPortForwarding(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, other := range []string{"title", "merge"} {
		for first := range 2 {
			t.Run(fmt.Sprintf("%s_first_%d", other, first), func(t *testing.T) {
				pair := fieldProtocolPair(t)
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				var workers sync.WaitGroup
				defer func() { cancel(); workers.Wait() }()
				type result struct {
					body map[string]interface{}
					err  error
				}
				outcomes := [2]chan result{make(chan result, 1), make(chan result, 1)}
				for i := range pair {
					pair[i].gate.armed.Store(true)
					workers.Add(1)
					go func(i int) {
						defer workers.Done()
						if i == 1 {
							body, err := portMergeRequest(ctx, pair[i])
							outcomes[i] <- result{body, err}
							return
						}
						if other == "title" {
							body, ok, err := pair[i].request(ctx, "REST", map[string]interface{}{"title": "Accepted title"})
							if !ok && err == nil {
								err = fmt.Errorf("ordinary PATCH rejected: %v", body)
							}
							outcomes[i] <- result{body, err}
							return
						}
						svcTask, err := pair[i].service.UpdateTaskMetadata(ctx, "protocol-fields", map[string]interface{}{"alpha": "accepted"})
						var body map[string]interface{}
						if err == nil {
							var encoded []byte
							encoded, err = json.Marshal(dto.FromTask(svcTask))
							if err == nil {
								err = json.Unmarshal(encoded, &body)
							}
						}
						outcomes[i] <- result{body, err}
					}(i)
				}
				for _, f := range pair {
					select {
					case <-f.gate.arrived:
					case <-ctx.Done():
						t.Fatal(ctx.Err())
					}
				}
				for _, i := range []int{first, 1 - first} {
					close(pair[i].gate.release)
					select {
					case got := <-outcomes[i]:
						require.NoError(t, got.err)
						if got.body != nil {
							select {
							case e := <-pair[i].published:
								data := e.Data.(map[string]interface{})
								require.Equal(t, got.body["title"], data["title"])
								require.Equal(t, got.body["metadata"], data["metadata"])
								require.Equal(t, got.body["updated_at"], data["updated_at"])
							case <-ctx.Done():
								t.Fatal(ctx.Err())
							}
						}
					case <-ctx.Done():
						t.Fatal(ctx.Err())
					}
				}
				current, err := pair[0].gate.Repository.GetTask(ctx, "protocol-fields")
				require.NoError(t, err)
				require.Equal(t, true, current.Metadata[models.MetaKeyPortForwardingEnabled])
				require.Equal(t, "current", current.Metadata["keep"])
				if other == "title" {
					require.Equal(t, "Accepted title", current.Title)
				} else {
					require.Equal(t, "accepted", current.Metadata["alpha"])
				}
			})
		}
	}
}

// @covers AC-TASKS-FIELD-UPDATES-001.11, AC-TASKS-FIELD-UPDATES-001.12
func TestTaskMetadataMergeRegisteredPortForwardingErrors(t *testing.T) {
	pair := fieldProtocolPair(t, "owner")
	f := pair[0]
	ctx := context.Background()
	before, err := f.gate.Repository.GetTask(ctx, "protocol-fields")
	require.NoError(t, err)
	for _, body := range []string{`{}`, `{"enabled":null}`, `{"enabled":"true"}`, `{"enabled":true,"x":1}`, `{"enabled":`, `{"enabled":true} {}`} {
		request := httptest.NewRequest(http.MethodPatch, "/api/v1/tasks/protocol-fields/port-forwarding", strings.NewReader(body))
		response := httptest.NewRecorder()
		f.router.ServeHTTP(response, request)
		require.Equal(t, http.StatusBadRequest, response.Code)
	}
	require.NoError(t, f.gate.UpsertWorkspaceMember(ctx, &models.WorkspaceMember{WorkspaceID: "protocol-ws", UserID: "viewer", Role: "viewer"}))
	for _, tt := range []struct {
		user, id string
		status   int
	}{{"viewer", "protocol-fields", http.StatusForbidden}, {"foreign", "protocol-fields", http.StatusNotFound}, {"foreign", "missing", http.StatusNotFound}} {
		request := httptest.NewRequest(http.MethodPatch, "/api/v1/tasks/"+tt.id+"/port-forwarding", strings.NewReader(`{"enabled":true}`))
		request = request.WithContext(authn.WithIdentity(ctx, authn.Identity{UserID: tt.user, Role: authn.RoleMember}))
		response := httptest.NewRecorder()
		f.router.ServeHTTP(response, request)
		require.Equal(t, tt.status, response.Code, response.Body.String())
	}
	after, err := f.gate.Repository.GetTask(ctx, "protocol-fields")
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.Empty(t, f.published)
}
