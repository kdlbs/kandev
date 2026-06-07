package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
)

// statefulHostStub is a goroutine-safe stub that models the real probe
// cache more faithfully than the basic stubHostUtility used elsewhere:
// Get/Refresh share the same map, Refresh atomically swaps state into
// the cache, and concurrent calls are serialized through a mutex (the
// real Manager.Refresh uses singleflight, which is even stricter).
type statefulHostStub struct {
	mu           sync.Mutex
	caps         map[string]hostutility.AgentCapabilities
	nextRefresh  map[string]hostutility.AgentCapabilities
	refreshCalls int
}

func newStatefulHostStub() *statefulHostStub {
	return &statefulHostStub{
		caps:        map[string]hostutility.AgentCapabilities{},
		nextRefresh: map[string]hostutility.AgentCapabilities{},
	}
}

func (h *statefulHostStub) ExecutePrompt(_ context.Context, _, _, _, _ string) (*hostutility.PromptResult, error) {
	return nil, nil
}

func (h *statefulHostStub) Get(agentType string) (hostutility.AgentCapabilities, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	c, ok := h.caps[agentType]
	return c, ok
}

func (h *statefulHostStub) Refresh(_ context.Context, agentType string) (hostutility.AgentCapabilities, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.refreshCalls++
	next, ok := h.nextRefresh[agentType]
	if !ok {
		next = hostutility.AgentCapabilities{AgentType: agentType, Status: hostutility.StatusOK}
	}
	h.caps[agentType] = next
	return next, nil
}

func newIntegrationServer(t *testing.T, agents []lifecycle.InferenceAgentInfo, host *statefulHostStub) *httptest.Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})

	// Register only the two routes under test directly. RegisterRoutes
	// requires a real Controller (full service stack); for integration
	// against the inference-agents surface we only need these two.
	h := &Handlers{
		executor:     &stubInferenceExecutor{agents: agents},
		hostExecutor: host,
		logger:       log,
	}
	r.GET("/api/v1/utility/inference-agents", h.httpListInferenceAgents)
	r.POST("/api/v1/utility/inference-agents/:id/refresh", h.httpRefreshInferenceAgent)

	return httptest.NewServer(r)
}

func getJSON(t *testing.T, url string) (int, http.Header, map[string]any) {
	t.Helper()
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	var out map[string]any
	if len(body) > 0 {
		if err := json.Unmarshal(body, &out); err != nil {
			t.Fatalf("GET %s: invalid JSON: %v; body=%s", url, err, body)
		}
	}
	return resp.StatusCode, resp.Header, out
}

// postEmptyE issues a POST with an empty body. Returns an error rather
// than calling t.Fatalf so it is safe to invoke from goroutines spawned
// by TestIntegration_ConcurrentRefresh — t.Fatalf is only legal from the
// test's main goroutine and panics elsewhere on Go 1.21+ (cubic review).
func postEmptyE(url string) (int, map[string]any, error) {
	req, _ := http.NewRequest(http.MethodPost, url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("POST %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(body, &out)
	return resp.StatusCode, out, nil
}

// postEmpty is the main-goroutine convenience wrapper. Concurrent callers
// must use postEmptyE.
func postEmpty(t *testing.T, url string) (int, map[string]any) {
	t.Helper()
	code, body, err := postEmptyE(url)
	if err != nil {
		t.Fatalf("%v", err)
	}
	return code, body
}

// TestIntegration_ProbeStateMachine drives GET → POST refresh → GET
// against the real gin router with one ACP-capable agent. Verifies the
// status field transitions, the cache reflects the refresh result, and
// the wire format (Content-Type, JSON shape) is correct.
func TestIntegration_ProbeStateMachine(t *testing.T) {
	host := newStatefulHostStub()
	host.caps["claude-acp"] = hostutility.AgentCapabilities{
		AgentType: "claude-acp",
		Status:    hostutility.StatusAuthRequired,
		Error:     "please run `claude login`",
	}
	host.nextRefresh["claude-acp"] = hostutility.AgentCapabilities{
		AgentType:      "claude-acp",
		Status:         hostutility.StatusOK,
		CurrentModelID: "sonnet",
		Models: []hostutility.Model{
			{ID: "sonnet", Name: "Sonnet"},
			{ID: "opus", Name: "Opus"},
		},
	}
	ts := newIntegrationServer(t,
		[]lifecycle.InferenceAgentInfo{{ID: "claude-acp", Name: "Claude ACP Agent", DisplayName: "Claude"}},
		host)
	defer ts.Close()

	code, hdr, body := getJSON(t, ts.URL+"/api/v1/utility/inference-agents")
	if code != http.StatusOK {
		t.Fatalf("initial GET: code=%d body=%v", code, body)
	}
	if ct := hdr.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q", ct)
	}
	agents := body["agents"].([]any)
	if len(agents) != 1 {
		t.Fatalf("want 1 agent, got %d", len(agents))
	}
	a := agents[0].(map[string]any)
	if a["status"] != "auth_required" {
		t.Fatalf("initial status: %v", a["status"])
	}
	if len(a["models"].([]any)) != 0 {
		t.Fatalf("initial models should be empty, got %v", a["models"])
	}
	if !strings.Contains(a["status_message"].(string), "claude login") {
		t.Fatalf("status_message: %v", a["status_message"])
	}

	code, body = postEmpty(t, ts.URL+"/api/v1/utility/inference-agents/claude-acp/refresh")
	if code != http.StatusOK || body["status"] != "ok" {
		t.Fatalf("refresh: code=%d body=%v", code, body)
	}

	_, _, body = getJSON(t, ts.URL+"/api/v1/utility/inference-agents")
	a = body["agents"].([]any)[0].(map[string]any)
	if a["status"] != "ok" {
		t.Fatalf("post-refresh status: %v", a["status"])
	}
	models := a["models"].([]any)
	if len(models) != 2 {
		t.Fatalf("want 2 models, got %d", len(models))
	}
	first := models[0].(map[string]any)
	if first["id"] != "sonnet" || first["is_default"] != true {
		t.Fatalf("first model: %+v", first)
	}
}

// TestIntegration_RefreshUnknownAgent verifies the handler 404s for an
// id not in the registry and does NOT call Refresh (no probe storm via
// arbitrary path params).
func TestIntegration_RefreshUnknownAgent(t *testing.T) {
	host := newStatefulHostStub()
	ts := newIntegrationServer(t,
		[]lifecycle.InferenceAgentInfo{{ID: "claude-acp", Name: "Claude ACP Agent", DisplayName: "Claude"}},
		host)
	defer ts.Close()

	code, _ := postEmpty(t, ts.URL+"/api/v1/utility/inference-agents/bogus-id/refresh")
	if code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", code)
	}
	if host.refreshCalls != 0 {
		t.Fatalf("refresh should not be invoked for unknown agent; got %d calls", host.refreshCalls)
	}
}

// TestIntegration_RefreshPathFuzz hits the refresh route with
// path-traversal-ish ids. None should cause a server error, and none
// should be treated as a valid agent.
func TestIntegration_RefreshPathFuzz(t *testing.T) {
	host := newStatefulHostStub()
	ts := newIntegrationServer(t,
		[]lifecycle.InferenceAgentInfo{{ID: "claude-acp", Name: "Claude ACP Agent", DisplayName: "Claude"}},
		host)
	defer ts.Close()

	cases := []string{
		"/api/v1/utility/inference-agents/..%2Fclaude-acp/refresh",
		"/api/v1/utility/inference-agents/claude-acp%00/refresh",
		"/api/v1/utility/inference-agents/" + strings.Repeat("a", 512) + "/refresh",
	}
	for _, p := range cases {
		t.Run(p, func(t *testing.T) {
			code, _ := postEmpty(t, ts.URL+p)
			if code >= 500 {
				t.Fatalf("path %q: server error %d", p, code)
			}
		})
	}
	if host.refreshCalls != 0 {
		t.Fatalf("none of the fuzzed ids should have reached Refresh; got %d", host.refreshCalls)
	}
}

// TestIntegration_ConcurrentRefresh hammers the refresh endpoint to
// verify race-freedom. The stub has no singleflight (that lives in the
// real Manager), so every one of the N requests must reach the stub —
// the assertion below pins refreshCalls == N. The primary signal is
// still that 50 concurrent goroutines drain without panic or data
// race, which is what -race exercises.
func TestIntegration_ConcurrentRefresh(t *testing.T) {
	host := newStatefulHostStub()
	host.caps["claude-acp"] = hostutility.AgentCapabilities{AgentType: "claude-acp", Status: hostutility.StatusAuthRequired}
	host.nextRefresh["claude-acp"] = hostutility.AgentCapabilities{
		AgentType: "claude-acp", Status: hostutility.StatusOK,
		Models: []hostutility.Model{{ID: "sonnet", Name: "Sonnet"}},
	}
	ts := newIntegrationServer(t,
		[]lifecycle.InferenceAgentInfo{{ID: "claude-acp", Name: "Claude ACP Agent", DisplayName: "Claude"}},
		host)
	defer ts.Close()

	const N = 50
	errs := make(chan error, N)
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			code, body, err := postEmptyE(ts.URL + "/api/v1/utility/inference-agents/claude-acp/refresh")
			if err != nil {
				errs <- err
				return
			}
			if code != http.StatusOK || body["status"] != "ok" {
				errs <- fmt.Errorf("code=%d status=%v", code, body["status"])
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	if host.refreshCalls != N {
		t.Fatalf("want %d refresh calls, got %d", N, host.refreshCalls)
	}
}

// TestIntegration_StatusMessageSanitizationOnTheWire feeds a 100KB
// probe error full of api_key= occurrences plus a unicode probe error
// through the GET endpoint. The redacted form must reach the wire
// without any raw secret material and without exploding the response.
func TestIntegration_StatusMessageSanitizationOnTheWire(t *testing.T) {
	t.Run("large payload", func(t *testing.T) {
		host := newStatefulHostStub()
		host.caps["claude-acp"] = hostutility.AgentCapabilities{
			AgentType: "claude-acp",
			Status:    hostutility.StatusFailed,
			Error:     strings.Repeat("api_key=sk-secret-xyz ", 5000),
		}
		ts := newIntegrationServer(t,
			[]lifecycle.InferenceAgentInfo{{ID: "claude-acp", Name: "Claude ACP Agent", DisplayName: "Claude"}},
			host)
		defer ts.Close()

		code, _, body := getJSON(t, ts.URL+"/api/v1/utility/inference-agents")
		if code != http.StatusOK {
			t.Fatalf("code=%d", code)
		}
		msg := body["agents"].([]any)[0].(map[string]any)["status_message"].(string)
		if strings.Contains(msg, "sk-secret-xyz") {
			t.Fatalf("secret leaked through sanitizer; len=%d head=%q", len(msg), msg[:80])
		}
	})

	t.Run("unicode probe error", func(t *testing.T) {
		host := newStatefulHostStub()
		host.caps["claude-acp"] = hostutility.AgentCapabilities{
			AgentType: "claude-acp",
			Status:    hostutility.StatusFailed,
			Error:     "认证失败: token=abc123 / Müllerverfahren",
		}
		ts := newIntegrationServer(t,
			[]lifecycle.InferenceAgentInfo{{ID: "claude-acp", Name: "Claude ACP Agent", DisplayName: "Claude"}},
			host)
		defer ts.Close()

		_, _, body := getJSON(t, ts.URL+"/api/v1/utility/inference-agents")
		msg := body["agents"].([]any)[0].(map[string]any)["status_message"].(string)
		if strings.Contains(msg, "abc123") {
			t.Fatalf("unicode secret leaked: %v", msg)
		}
		if !strings.Contains(msg, "认证失败") {
			t.Fatalf("unicode prose was lost in sanitization: %v", msg)
		}
	})
}
