package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type transportAdmissionGate struct {
	repository.RepositoryBranchPolicyRepository
	ready, release chan struct{}
}

func (g *transportAdmissionGate) CreateRepositoryBranchPoliciesIfEmpty(ctx context.Context, id string, policies []*models.RepositoryBranchPolicy) error {
	close(g.ready)
	select {
	case <-g.release:
	case <-ctx.Done():
		return ctx.Err()
	}
	return g.RepositoryBranchPolicyRepository.CreateRepositoryBranchPoliciesIfEmpty(ctx, id, policies)
}

func admissionPeerTransport(t *testing.T, repo *tasksqlite.Repository, eventBus *bus.MemoryEventBus) (*gin.Engine, *ws.Dispatcher, *transportAdmissionGate) {
	t.Helper()
	var seq int
	var name, filename string
	require.NoError(t, repo.DB().QueryRow("PRAGMA database_list").Scan(&seq, &name, &filename))
	connection, err := db.OpenSQLite(filename)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, connection.Close()) })
	database := sqlx.NewDb(connection, "sqlite3")
	peer := tasksqlite.NewWithInitializedDB(database, database, nil)
	gate := &transportAdmissionGate{RepositoryBranchPolicyRepository: peer, ready: make(chan struct{}), release: make(chan struct{})}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	require.NoError(t, err)
	svc := service.NewService(service.Repos{Workspaces: peer, RepoEntities: peer, BranchPolicies: gate}, eventBus, log, service.RepositoryDiscoveryConfig{})
	router, dispatcher := gin.New(), ws.NewDispatcher()
	RegisterRepositoryBranchPolicyRoutes(router, dispatcher, svc, log)
	return router, dispatcher, gate
}

type admissionEventRecord struct {
	event *bus.Event
	rows  []*models.RepositoryBranchPolicy
	err   error
}

func recordAdmissionEvents(t *testing.T, eventBus *bus.MemoryEventBus, repo *tasksqlite.Repository) func() []admissionEventRecord {
	t.Helper()
	var mu sync.Mutex
	var records []admissionEventRecord
	sub, err := eventBus.Subscribe(events.RepositoryBranchPolicyCreated, func(ctx context.Context, event *bus.Event) error {
		rows, err := repo.ListRepositoryBranchPolicies(ctx, "repo-policy-handler")
		mu.Lock()
		defer mu.Unlock()
		records = append(records, admissionEventRecord{event: event, rows: rows, err: err})
		return err
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sub.Unsubscribe()) })
	return func() []admissionEventRecord {
		mu.Lock()
		defer mu.Unlock()
		return append([]admissionEventRecord(nil), records...)
	}
}

type admissionTransportResult struct {
	status int
	failed bool
	body   []byte
	err    error
}

func callAdmissionTransport(ctx context.Context, router *gin.Engine, dispatcher *ws.Dispatcher, websocket bool, production, development string) admissionTransportResult {
	payload := map[string]string{"repository_id": "repo-policy-handler", "production_branch": production, "development_branch": development}
	if websocket {
		message, err := ws.NewRequest("gitflow-admission", ws.ActionRepositoryBranchPolicyGitflow, payload)
		if err != nil {
			return admissionTransportResult{err: err}
		}
		response, err := dispatcher.Dispatch(ctx, message)
		if err != nil {
			return admissionTransportResult{err: err}
		}
		return admissionTransportResult{failed: response.Type == ws.MessageTypeError, body: response.Payload}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return admissionTransportResult{err: err}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "/api/v1/repositories/repo-policy-handler/branch-policies/gitflow", bytes.NewReader(body))
	if err != nil {
		return admissionTransportResult{err: err}
	}
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return admissionTransportResult{status: response.Code, body: response.Body.Bytes()}
}

// @covers AC-WORKSPACES-BRANCH-POLICIES-002.3, AC-WORKSPACES-BRANCH-POLICIES-002.4, AC-WORKSPACES-BRANCH-POLICIES-002.6
func TestGitflowAdmissionHTTP(t *testing.T) { checkAdmissionTransport(t, false) }

// @covers AC-WORKSPACES-BRANCH-POLICIES-002.3, AC-WORKSPACES-BRANCH-POLICIES-002.4, AC-WORKSPACES-BRANCH-POLICIES-002.6
func TestGitflowAdmissionWS(t *testing.T) { checkAdmissionTransport(t, true) }

func checkAdmissionTransport(t *testing.T, websocket bool) {
	router, dispatcher, repo, eventBus := newRepositoryBranchPolicyTestRouterWithBus(t, "Policy workspace")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	entity, err := repo.GetRepository(ctx, "repo-policy-handler")
	require.NoError(t, err)
	for _, name := range []string{"release", "next"} {
		cmd := exec.CommandContext(ctx, "git", "branch", name)
		cmd.Dir, cmd.Env = entity.LocalPath, append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
		output, err := cmd.CombinedOutput()
		require.NoErrorf(t, err, "git branch %s: %s", name, output)
	}
	peerRouter, peerDispatcher, gate := admissionPeerTransport(t, repo, eventBus)
	readEvents := recordAdmissionEvents(t, eventBus, repo)
	var resume sync.Once
	done := make(chan struct{})
	var loser admissionTransportResult
	t.Cleanup(func() { resume.Do(func() { close(gate.release) }); cancel(); <-done })
	go func() {
		defer close(done)
		loser = callAdmissionTransport(ctx, peerRouter, peerDispatcher, websocket, "release", "next")
	}()
	select {
	case <-gate.ready:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	winner := callAdmissionTransport(ctx, router, dispatcher, websocket, "main", "develop")
	resume.Do(func() { close(gate.release) })
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	require.NoError(t, winner.err)
	require.NoError(t, loser.err)
	if websocket {
		require.False(t, winner.failed)
		require.True(t, loser.failed)
		var failure ws.ErrorPayload
		require.NoError(t, json.Unmarshal(loser.body, &failure))
		require.Equal(t, ws.ErrorCodeConflict, failure.Code)
		require.Contains(t, failure.Message, "already")
	} else {
		require.Equal(t, http.StatusCreated, winner.status, string(winner.body))
		require.Equal(t, http.StatusConflict, loser.status, string(loser.body))
		require.Contains(t, string(loser.body), "already")
	}
	assertAdmissionTransportRows(t, ctx, repo, winner.body, readEvents())
}

func assertAdmissionTransportRows(t *testing.T, ctx context.Context, repo *tasksqlite.Repository, response []byte, records []admissionEventRecord) {
	t.Helper()
	var result dto.ListRepositoryBranchPoliciesResponse
	require.NoError(t, json.Unmarshal(response, &result))
	require.Equal(t, 4, result.Total)
	require.Len(t, result.Policies, 4)
	rows, err := repo.ListRepositoryBranchPolicies(ctx, "repo-policy-handler")
	require.NoError(t, err)
	require.ElementsMatch(t, result.Policies, branchPolicyListResponse(rows).Policies)
	require.Len(t, records, 4)
	byID := make(map[string]*models.RepositoryBranchPolicy)
	for _, row := range rows {
		byID[row.ID] = row
		if row.Name == "Hotfix" {
			require.Equal(t, "main", row.BaseBranch)
		} else {
			require.Equal(t, "develop", row.BaseBranch)
		}
	}
	seen := make(map[string]bool)
	for _, record := range records {
		require.NoError(t, record.err)
		require.ElementsMatch(t, rows, record.rows)
		data := record.event.Data.(map[string]interface{})
		id := data["id"].(string)
		require.False(t, seen[id])
		seen[id] = true
		policy := byID[id]
		require.NotNil(t, policy)
		for key, value := range map[string]string{"name": policy.Name, "repository_id": policy.RepositoryID,
			"workspace_id": "ws-policy-handler", "base_branch": policy.BaseBranch, "branch_template": policy.BranchTemplate,
			"pull_request_target": policy.PullRequestTarget, "created_at": policy.CreatedAt.Format(time.RFC3339),
			"updated_at": policy.UpdatedAt.Format(time.RFC3339)} {
			require.Equal(t, value, data[key], key)
		}
	}
}
