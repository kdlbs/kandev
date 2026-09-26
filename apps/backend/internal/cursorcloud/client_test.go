package cursorcloud

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCreateStableAgentID(t *testing.T) {
	var gotRequest struct {
		AgentID             string         `json:"agentId"`
		Prompt              Prompt         `json:"prompt"`
		Model               ModelSelection `json:"model"`
		Repos               []Repository   `json:"repos"`
		WorkOnCurrentBranch *bool          `json:"workOnCurrentBranch"`
		AutoCreatePR        *bool          `json:"autoCreatePR"`
		MCPServers          []MCPServer    `json:"mcpServers"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/agents" {
			t.Errorf("request = %s %s, want POST /v1/agents", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer api-secret" {
			t.Errorf("Authorization = %q, want bearer credential", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotRequest); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"agent":{"id":"bc-123e4567-e89b-12d3-a456-426614174000","status":"ACTIVE","url":"https://cursor.com/agents/bc-123e4567-e89b-12d3-a456-426614174000","latestRunId":"run-1"},"run":{"id":"run-1","agentId":"bc-123e4567-e89b-12d3-a456-426614174000","status":"CREATING"}}`))
	}))
	t.Cleanup(server.Close)

	request := CreateAgentRequest{
		AgentID: "bc-123e4567-e89b-12d3-a456-426614174000",
		Prompt:  Prompt{Text: "Inspect this repository"},
		Model:   &ModelSelection{ID: "composer-2"},
		Repos:   []Repository{{URL: "https://github.com/acme/project", StartingRef: "main"}},
		MCPServers: []MCPServer{{
			Name: "kandev",
			Type: "http",
			URL:  "https://kandev.example/api/v1/managed-agent-mcp/grant-1",
			Headers: map[string]string{
				"Authorization": "Bearer grant-one",
			},
		}},
	}
	client, err := newClient("api-secret", server.Client(), server.URL)
	if err != nil {
		t.Fatalf("newClient() error = %v", err)
	}
	got, err := client.CreateAgent(context.Background(), request)
	if err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}
	if got.Agent.ID != request.AgentID || got.Run.AgentID != request.AgentID {
		t.Fatalf("response = %#v, want stable agent id %q", got, request.AgentID)
	}
	if gotRequest.AgentID != request.AgentID {
		t.Errorf("submitted agentId = %q, want stable id %q", gotRequest.AgentID, request.AgentID)
	}
	if gotRequest.MCPServers[0].Headers["Authorization"] != "Bearer grant-one" {
		t.Errorf("create MCP authorization = %q, want scoped grant", gotRequest.MCPServers[0].Headers["Authorization"])
	}
	if gotRequest.Prompt.Text != "Inspect this repository" || gotRequest.Model.ID != "composer-2" || len(gotRequest.Repos) != 1 || gotRequest.Repos[0].URL != "https://github.com/acme/project" || gotRequest.Repos[0].StartingRef != "main" {
		t.Errorf("create payload = %#v, want prompt, selected model, and published repository ref", gotRequest)
	}
	if gotRequest.WorkOnCurrentBranch == nil || *gotRequest.WorkOnCurrentBranch {
		t.Error("workOnCurrentBranch must be explicitly false for a Cursor-owned output branch")
	}
	if gotRequest.AutoCreatePR == nil || *gotRequest.AutoCreatePR {
		t.Error("autoCreatePR must be explicitly false by default")
	}
}

func TestFollowupReplacesMCPHeaders(t *testing.T) {
	var got struct {
		Prompt     Prompt      `json:"prompt"`
		MCPServers []MCPServer `json:"mcpServers"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/agents/bc-123e4567-e89b-12d3-a456-426614174000/runs" {
			t.Errorf("request = %s %s, want follow-up run", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"run":{"id":"run-2","agentId":"bc-123e4567-e89b-12d3-a456-426614174000","status":"CREATING"}}`))
	}))
	t.Cleanup(server.Close)
	client, err := newClient("api-secret", server.Client(), server.URL)
	if err != nil {
		t.Fatalf("newClient() error = %v", err)
	}
	newGrant := MCPServer{
		Name: "kandev",
		Type: "http",
		URL:  "https://kandev.example/api/v1/managed-agent-mcp/grant-2",
		Headers: map[string]string{
			"Authorization": "Bearer grant-two",
		},
	}
	gotRun, err := client.CreateRun(context.Background(), "bc-123e4567-e89b-12d3-a456-426614174000", CreateRunRequest{
		Prompt:     Prompt{Text: "Continue the task"},
		MCPServers: []MCPServer{newGrant},
	})
	if err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	if gotRun.ID != "run-2" {
		t.Fatalf("run id = %q, want run-2", gotRun.ID)
	}
	if len(got.MCPServers) != 1 || got.MCPServers[0].Headers["Authorization"] != "Bearer grant-two" {
		t.Fatalf("follow-up MCP servers = %#v, want only the replacement grant", got.MCPServers)
	}
	if got.Prompt.Text != "Continue the task" {
		t.Errorf("follow-up prompt = %q, want the submitted text", got.Prompt.Text)
	}
}

func TestGetRunPreservesUnknownStatusAndResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/agents/bc-123e4567-e89b-12d3-a456-426614174000/runs/run-1" {
			t.Errorf("request = %s %s, want GET run detail", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"run-1","agentId":"bc-123e4567-e89b-12d3-a456-426614174000","status":"PAUSED_BY_PROVIDER","result":"saved reply","durationMs":1200,"git":{"branches":[{"repoUrl":"github.com/acme/project","branch":"cursor/fix","prUrl":"https://github.com/acme/project/pull/9"}]}}`))
	}))
	t.Cleanup(server.Close)
	client, err := newClient("api-secret", server.Client(), server.URL)
	if err != nil {
		t.Fatalf("newClient() error = %v", err)
	}
	run, err := client.GetRun(context.Background(), "bc-123e4567-e89b-12d3-a456-426614174000", "run-1")
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if run.Status != "PAUSED_BY_PROVIDER" || run.Result != "saved reply" || run.DurationMs != 1200 {
		t.Fatalf("run = %#v, want the unknown status and terminal result preserved", run)
	}
	if run.Git == nil || len(run.Git.Branches) != 1 || run.Git.Branches[0].PRURL != "https://github.com/acme/project/pull/9" {
		t.Fatalf("git result = %#v, want the provider branch and PR", run.Git)
	}
}

func TestGetAgentReconcilesStableCreateIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/agents/bc-123e4567-e89b-12d3-a456-426614174000" {
			t.Errorf("request = %s %s, want GET agent by stable create ID", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"bc-123e4567-e89b-12d3-a456-426614174000","status":"ACTIVE","latestRunId":"run-1","repos":[{"url":"https://github.com/acme/project","startingRef":"main"}],"workOnCurrentBranch":false,"autoCreatePR":false}`))
	}))
	t.Cleanup(server.Close)
	client, err := newClient("api-secret", server.Client(), server.URL)
	if err != nil {
		t.Fatalf("newClient() error = %v", err)
	}
	agent, err := client.GetAgent(context.Background(), "bc-123e4567-e89b-12d3-a456-426614174000")
	if err != nil {
		t.Fatalf("GetAgent() error = %v", err)
	}
	if agent.ID != "bc-123e4567-e89b-12d3-a456-426614174000" || agent.LatestRunID != "run-1" || len(agent.Repos) != 1 || agent.Repos[0].StartingRef != "main" {
		t.Fatalf("agent = %#v, want reconciled stable agent and frozen repo selection", agent)
	}
}

func TestCancelRunUsesOneTerminalCancelRequest(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/v1/agents/bc-123e4567-e89b-12d3-a456-426614174000/runs/run-1/cancel" {
			t.Errorf("request = %s %s, want POST run cancel", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"run-1"}`))
	}))
	t.Cleanup(server.Close)
	client, err := newClient("api-secret", server.Client(), server.URL)
	if err != nil {
		t.Fatalf("newClient() error = %v", err)
	}
	err = client.CancelRun(context.Background(), "bc-123e4567-e89b-12d3-a456-426614174000", "run-1")
	if err != nil {
		t.Fatalf("CancelRun() error = %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("cancel requests = %d, want exactly one", got)
	}

	conflictServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"code":"run_not_cancellable","message":"do not leak this detail"}`))
	}))
	t.Cleanup(conflictServer.Close)
	conflictClient, err := newClient("api-secret", conflictServer.Client(), conflictServer.URL)
	if err != nil {
		t.Fatalf("new conflict client: %v", err)
	}
	err = conflictClient.CancelRun(context.Background(), "bc-123e4567-e89b-12d3-a456-426614174000", "run-1")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "run_not_cancellable" {
		t.Fatalf("CancelRun conflict = %#v, want run_not_cancellable", err)
	}
	if errors.Is(err, ErrOutcomeUnknown) {
		t.Fatal("terminal cancel conflict must be reconciled by reading run status, not treated as unknown")
	}
}

func TestListRunsSupportsSubmissionResolutionPagination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/agents/bc-123e4567-e89b-12d3-a456-426614174000/runs" {
			t.Errorf("request = %s %s, want GET agent runs", r.Method, r.URL.Path)
		}
		query, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil || query.Get("limit") != "25" || query.Get("cursor") != "page-token" {
			t.Errorf("query = %q, want limit=25 and cursor=page-token", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"id":"run-2","agentId":"bc-123e4567-e89b-12d3-a456-426614174000","status":"RUNNING"}],"nextCursor":"next-page"}`))
	}))
	t.Cleanup(server.Close)
	client, err := newClient("api-secret", server.Client(), server.URL)
	if err != nil {
		t.Fatalf("newClient() error = %v", err)
	}
	page, err := client.ListRuns(context.Background(), "bc-123e4567-e89b-12d3-a456-426614174000", 25, "page-token")
	if err != nil {
		t.Fatalf("ListRuns() error = %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != "run-2" || page.NextCursor != "next-page" {
		t.Fatalf("page = %#v, want the matching run and next cursor", page)
	}
}

func TestListModelsPreservesSupportedParametersAndVariants(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/models" {
			t.Errorf("request = %s %s, want GET /v1/models", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"id":"composer-2","displayName":"Composer 2","aliases":["composer-latest"],"parameters":[{"id":"fast","values":[{"value":"false"},{"value":"true","displayName":"Fast"}]}],"variants":[{"params":[{"id":"fast","value":"true"}],"displayName":"Composer 2 Fast","isDefault":true}]}]}`))
	}))
	t.Cleanup(server.Close)
	client, err := newClient("api-secret", server.Client(), server.URL)
	if err != nil {
		t.Fatalf("newClient() error = %v", err)
	}
	catalog, err := client.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	if len(catalog.Items) != 1 || catalog.Items[0].ID != "composer-2" || catalog.Items[0].Parameters[0].Values[1].Value != "true" || !catalog.Items[0].Variants[0].IsDefault {
		t.Fatalf("catalog = %#v, want model parameters and variants", catalog)
	}
}

func TestListRepositoriesUsesCursorGitHubCatalog(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/repositories" {
			t.Errorf("request = %s %s, want GET /v1/repositories", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"url":"https://github.com/acme/project"}]}`))
	}))
	t.Cleanup(server.Close)
	client, err := newClient("api-secret", server.Client(), server.URL)
	if err != nil {
		t.Fatalf("newClient() error = %v", err)
	}
	catalog, err := client.ListRepositories(context.Background())
	if err != nil {
		t.Fatalf("ListRepositories() error = %v", err)
	}
	if len(catalog.Items) != 1 || catalog.Items[0].URL != "https://github.com/acme/project" {
		t.Fatalf("catalog = %#v, want accessible GitHub repository", catalog)
	}
}

func TestFollowupTimeoutNoRetry(t *testing.T) {
	var calls atomic.Int32
	received := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		close(received)
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)
	client, err := newClient("api-secret", server.Client(), server.URL)
	if err != nil {
		t.Fatalf("newClient() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err = client.CreateRun(ctx, "bc-123e4567-e89b-12d3-a456-426614174000", CreateRunRequest{
		Prompt: Prompt{Text: "Continue"}, MCPServers: []MCPServer{{Name: "kandev", URL: "https://kandev.example/mcp"}},
	})
	if !errors.Is(err, ErrOutcomeUnknown) {
		t.Fatalf("CreateRun() error = %v, want unknown submission", err)
	}
	<-received
	if got := calls.Load(); got != 1 {
		t.Fatalf("follow-up requests = %d, want exactly one after timeout", got)
	}
}

func TestClientAdmissionErrorsAreTypedAndSanitized(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		status     int
		body       string
		wantCode   string
		wantSecret string
	}{
		{name: "authentication", status: http.StatusUnauthorized, body: `{"code":"invalid_api_key","message":"api-secret was rejected"}`, wantSecret: "api-secret"},
		{name: "busy agent", status: http.StatusConflict, body: `{"code":"agent_busy","message":"api-secret was rejected"}`, wantCode: "agent_busy", wantSecret: "api-secret"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(testCase.status)
				_, _ = io.WriteString(w, testCase.body)
			}))
			t.Cleanup(server.Close)
			client, err := newClient("api-secret", server.Client(), server.URL)
			if err != nil {
				t.Fatalf("newClient() error = %v", err)
			}
			_, err = client.CreateRun(context.Background(), "bc-123e4567-e89b-12d3-a456-426614174000", CreateRunRequest{
				Prompt: Prompt{Text: "Continue"}, MCPServers: []MCPServer{{Name: "kandev", URL: "https://kandev.example/mcp"}},
			})
			var apiErr *APIError
			if !errors.As(err, &apiErr) || apiErr.StatusCode != testCase.status || apiErr.Code != testCase.wantCode {
				t.Fatalf("request error = %#v, want HTTP %d and code %q", err, testCase.status, testCase.wantCode)
			}
			if strings.Contains(err.Error(), testCase.wantSecret) || strings.Contains(err.Error(), "was rejected") {
				t.Fatalf("error leaked provider detail: %v", err)
			}
			if errors.Is(err, ErrOutcomeUnknown) {
				t.Fatalf("HTTP %d admission response must be definitive", testCase.status)
			}
		})
	}
}

func TestGetRunRetriesAtProviderRetryAfter(t *testing.T) {
	var calls atomic.Int32
	now := time.Date(2026, time.September, 25, 10, 0, 0, 0, time.UTC)
	wantDelay := 5 * time.Second
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", now.Add(wantDelay).Format(http.TimeFormat))
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"code":"rate_limited"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"id":"run-1","agentId":"%s","status":"FINISHED"}`, "bc-123e4567-e89b-12d3-a456-426614174000")
	}))
	t.Cleanup(server.Close)
	var slept time.Duration
	client, err := NewClientWithConfig(ClientConfig{
		APIKey: "api-secret", APIOrigin: server.URL, HTTPClient: server.Client(), Now: func() time.Time { return now },
		Sleep: func(_ context.Context, delay time.Duration) error { slept = delay; return nil },
	})
	if err != nil {
		t.Fatalf("NewClientWithConfig() error = %v", err)
	}
	run, err := client.GetRun(context.Background(), "bc-123e4567-e89b-12d3-a456-426614174000", "run-1")
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if run.Status != "FINISHED" || calls.Load() != 2 || slept != wantDelay {
		t.Fatalf("run=%#v requests=%d slept=%v, want finished after exactly the provider delay", run, calls.Load(), slept)
	}
}

func TestGetRunRetryWaitStopsOnCancellation(t *testing.T) {
	var calls atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"code":"rate_limited"}`)
	}))
	t.Cleanup(server.Close)
	client, err := NewClientWithConfig(ClientConfig{
		APIKey: "api-secret", APIOrigin: server.URL, HTTPClient: server.Client(),
		Sleep: func(ctx context.Context, _ time.Duration) error {
			cancel()
			return ctx.Err()
		},
	})
	if err != nil {
		t.Fatalf("NewClientWithConfig() error = %v", err)
	}
	_, err = client.GetRun(ctx, "bc-123e4567-e89b-12d3-a456-426614174000", "run-1")
	if !errors.Is(err, context.Canceled) || calls.Load() != 1 {
		t.Fatalf("GetRun() error=%v requests=%d, want canceled retry wait and one request", err, calls.Load())
	}
}

func TestCreateMalformedSuccessRemainsUnknown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{malformed}`)
	}))
	t.Cleanup(server.Close)
	client, err := newClient("api-secret", server.Client(), server.URL)
	if err != nil {
		t.Fatalf("newClient() error = %v", err)
	}
	_, err = client.CreateAgent(context.Background(), CreateAgentRequest{
		AgentID: "bc-123e4567-e89b-12d3-a456-426614174000", Prompt: Prompt{Text: "Create"},
		Repos:      []Repository{{URL: "https://github.com/acme/project", StartingRef: "main"}},
		MCPServers: []MCPServer{{Name: "kandev", URL: "https://kandev.example/mcp"}},
	})
	if !errors.Is(err, ErrUnsupportedContract) || !errors.Is(err, ErrOutcomeUnknown) || strings.Contains(fmt.Sprint(err), "malformed") {
		t.Fatalf("CreateAgent() error = %v, want sanitized unsupported contract and unknown state", err)
	}
}

func TestCreateStableAgentIDConflictRequiresReconciliation(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]string{"code": agentIDConflictCode, "message": "an agent already exists"})
	}))
	t.Cleanup(server.Close)
	client, err := newClient("api-secret", server.Client(), server.URL)
	if err != nil {
		t.Fatalf("newClient() error = %v", err)
	}
	_, err = client.CreateAgent(context.Background(), CreateAgentRequest{
		AgentID: "bc-123e4567-e89b-12d3-a456-426614174000", Prompt: Prompt{Text: "Create"},
		Repos:      []Repository{{URL: "https://github.com/acme/project", StartingRef: "main"}},
		MCPServers: []MCPServer{{Name: "kandev", URL: "https://kandev.example/mcp"}},
	})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != agentIDConflictCode || !errors.Is(err, ErrOutcomeUnknown) {
		t.Fatalf("CreateAgent() error = %#v, want conflict requiring reconciliation", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("create requests = %d, want no blind retry", calls.Load())
	}
}
