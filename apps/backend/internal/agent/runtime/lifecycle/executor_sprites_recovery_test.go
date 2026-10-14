package lifecycle

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gorilla/websocket"
	sprites "github.com/superfly/sprites-go"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

type spritesClientFactorySetter interface {
	setSpritesClientFactory(func(string) *sprites.Client)
}

func TestSpritesExecutorClosesHostProxyWithoutDestroyingSprites(t *testing.T) {
	exec := newTestSpritesExecutor(nil)
	cancelled := false
	exec.proxies["execution-sprites"] = &SpritesProxySession{cancel: func() { cancelled = true }}

	closer, ok := interface{}(exec).(Closeable)
	if !ok {
		t.Fatal("SpritesExecutor must close its host-side proxy sessions during backend shutdown")
	}
	if err := closer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !cancelled {
		t.Fatal("Close did not cancel the host-side Sprite proxy")
	}
	if len(exec.proxies) != 0 {
		t.Fatalf("tracked proxies after Close = %d, want none", len(exec.proxies))
	}
}

func TestSpritesRecoverySupportsDetailedUnknownClassification(t *testing.T) {
	exec := newTestSpritesExecutor(nil)
	recovery, ok := interface{}(exec).(DetailedRecoveryBackend)
	if !ok {
		t.Fatal("SpritesExecutor must classify unknown versus absent remote recovery candidates")
	}
	instances, outcomes, err := recovery.RecoverInstancesDetailed(context.Background(), []*models.ExecutorRunning{{
		ID: "session-sprites", SessionID: "session-sprites", TaskID: "task-sprites",
		AgentExecutionID: "execution-sprites", Runtime: agentruntime.RuntimeSprites,
		Metadata: map[string]interface{}{MetadataKeySpriteName: "kandev-abc", "env_secret_id_SPRITES_API_TOKEN": "missing"},
	}})
	if err != nil {
		t.Fatalf("RecoverInstancesDetailed: %v", err)
	}
	if len(instances) != 0 || outcomes["session-sprites"] != RecoveryOutcomeUnknown {
		t.Fatalf("missing control-plane credential result = instances %d, outcome %q; want retryable unknown", len(instances), outcomes["session-sprites"])
	}
}

func TestSpritesRecoveryAttachesSavedSpriteWithoutProvisioning(t *testing.T) {
	var mu sync.Mutex
	var requests []string
	var instanceLookups []string
	const savedInstancePort = 49271
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	instanceAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		mu.Lock()
		instanceLookups = append(instanceLookups, req.Method+" "+req.URL.Path)
		mu.Unlock()
		if req.Method != http.MethodGet || req.URL.Path != "/api/v1/instances/execution-sprites" {
			http.Error(w, "unexpected agentctl request", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"execution-sprites","port":49271,"status":"running","workspace_path":"/workspace","session_id":"session-sprites","task_id":"task-sprites","env":{"RECOVERED":"sprites-live"},"workspace_source_roots":["/workspace/src"]}`))
	}))
	defer instanceAPI.Close()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		mu.Lock()
		requests = append(requests, req.Method+" "+req.URL.Path+"?"+req.URL.RawQuery)
		mu.Unlock()
		if req.Method == http.MethodGet && req.URL.Path == "/v1/sprites/kandev-saved/proxy" {
			if req.Header.Get("Authorization") != "Bearer api-token" {
				http.Error(w, "missing Sprite API authentication", http.StatusUnauthorized)
				return
			}
			proxyAgentctlWebSocket(w, req, upgrader, instanceAPI.Listener.Addr().String(), spritesAgentctlPort, savedInstancePort)
			return
		}
		if req.Method != http.MethodGet || req.URL.Path != "/v1/sprites/kandev-saved" {
			http.Error(w, "unexpected mutation or resource", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"kandev-saved","id":"sprite-saved","status":"running"}`))
	}))
	defer api.Close()

	exec := newTestSpritesExecutor(&mockSecretStore{store: map[string]string{"sprite-token": "api-token"}})
	exec.agentctlPort = spritesAgentctlPort
	setter, ok := interface{}(exec).(spritesClientFactorySetter)
	if !ok {
		t.Fatal("SpritesExecutor does not expose the recovery client factory seam")
	}
	setter.setSpritesClientFactory(func(token string) *sprites.Client {
		if token != "api-token" {
			t.Errorf("Sprites API token = %q, want resolved secret", token)
		}
		return sprites.New(token, sprites.WithBaseURL(api.URL), sprites.WithDisableControl())
	})
	recovery, ok := interface{}(exec).(DetailedRecoveryBackend)
	if !ok {
		t.Fatal("SpritesExecutor must classify unknown versus absent remote recovery candidates")
	}
	recoveryCtx, cancelRecovery := context.WithCancel(context.Background())
	instances, outcomes, err := recovery.RecoverInstancesDetailed(recoveryCtx, []*models.ExecutorRunning{{
		ID: "session-sprites", SessionID: "session-sprites", TaskID: "task-sprites",
		AgentExecutionID: "execution-sprites", Runtime: agentruntime.RuntimeSprites, AgentctlPort: 45123,
		Metadata: map[string]interface{}{
			MetadataKeySpriteName:             "kandev-saved",
			"env_secret_id_SPRITES_API_TOKEN": "sprite-token",
		},
	}})
	if err != nil {
		t.Fatalf("RecoverInstancesDetailed: %v", err)
	}
	cancelRecovery()
	if len(instances) != 1 {
		t.Fatalf("recovered instances = %d, outcomes %#v, want one attached saved Sprite", len(instances), outcomes)
	}
	instance := instances[0]
	if instance.InstanceID != "execution-sprites" || instance.SessionID != "session-sprites" || instance.TaskID != "task-sprites" {
		t.Fatalf("recovered identity = instance=%q session=%q task=%q", instance.InstanceID, instance.SessionID, instance.TaskID)
	}
	if instance.Client == nil || instance.Client.AuthToken() != "" {
		t.Fatal("Sprites agentctl transport must remain unauthenticated behind its authenticated local proxy")
	}
	if instance.WorkspacePath != spritesWorkspacePath || getMetadataString(instance.Metadata, MetadataKeySpriteName) != "kandev-saved" {
		t.Fatalf("recovered workspace/resource = %q / %q", instance.WorkspacePath, getMetadataString(instance.Metadata, MetadataKeySpriteName))
	}
	if instance.Env["RECOVERED"] != "sprites-live" || len(instance.WorkspaceSourceRoots) != 1 || instance.WorkspaceSourceRoots[0] != "/workspace/src" {
		t.Fatalf("recovered live environment/source roots = %#v / %#v", instance.Env, instance.WorkspaceSourceRoots)
	}
	if _, classified := outcomes["session-sprites"]; classified {
		t.Fatalf("successful recovery should be represented by its returned instance, got outcome %q", outcomes["session-sprites"])
	}
	proxy := exec.proxies["execution-sprites"]
	if proxy == nil || proxy.localPort <= 0 || proxy.proxySession == nil || proxy.proxySession.RemotePort != savedInstancePort {
		t.Fatalf("recovery proxy = %#v, want the saved instance port %d", proxy, savedInstancePort)
	}
	if instance.DiscardRecovery == nil {
		t.Fatal("recovered Sprites instance lacks exact-attempt cleanup")
	}
	instance.DiscardRecovery()
	instance.Client.Close()
	exec.mu.RLock()
	removedCandidate := exec.proxies["execution-sprites"] == nil
	exec.mu.RUnlock()
	if !removedCandidate {
		t.Fatal("discarding a rejected recovery left its proxy registered")
	}
	retried, retryOutcomes, err := recovery.RecoverInstancesDetailed(context.Background(), []*models.ExecutorRunning{{
		ID: "session-sprites", SessionID: "session-sprites", TaskID: "task-sprites",
		AgentExecutionID: "execution-sprites", Runtime: agentruntime.RuntimeSprites,
		Metadata: map[string]interface{}{
			MetadataKeySpriteName:             "kandev-saved",
			"env_secret_id_SPRITES_API_TOKEN": "sprite-token",
		},
	}})
	if err != nil {
		t.Fatalf("retry RecoverInstancesDetailed: %v", err)
	}
	if len(retried) != 1 || retryOutcomes["session-sprites"] != "" || retried[0].Client == nil {
		t.Fatalf("retry after exact discard returned %d instances, outcome %q; want a new attached proxy", len(retried), retryOutcomes["session-sprites"])
	}
	if retried[0].DiscardRecovery == nil {
		t.Fatal("retried Sprites instance lacks exact-attempt cleanup")
	}
	exec.mu.RLock()
	retriedProxy := exec.proxies["execution-sprites"]
	exec.mu.RUnlock()
	if retriedProxy == nil || retriedProxy == proxy || retriedProxy.proxySession.RemotePort != savedInstancePort {
		t.Fatal("retry did not register a new proxy to the exact saved instance port")
	}
	successor := &SpritesProxySession{spriteName: "successor"}
	exec.mu.Lock()
	exec.proxies["execution-sprites"] = successor
	exec.mu.Unlock()
	retried[0].DiscardRecovery()
	exec.mu.RLock()
	stillCurrent := exec.proxies["execution-sprites"] == successor
	exec.mu.RUnlock()
	if !stillCurrent {
		t.Fatal("discarding a rejected recovery removed a successor proxy")
	}
	closer, ok := interface{}(exec).(Closeable)
	if !ok {
		t.Fatal("SpritesExecutor must close its host-side proxy sessions")
	}
	if err := closer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	mu.Lock()
	gotRequests := append([]string(nil), requests...)
	mu.Unlock()
	if len(gotRequests) != 4 {
		t.Fatalf("Sprites API requests = %q, want two saved-resource GET and read-only proxy pairs", gotRequests)
	}
	for index := 0; index < len(gotRequests); index += 2 {
		if !strings.HasPrefix(gotRequests[index], "GET /v1/sprites/kandev-saved?") ||
			!strings.HasPrefix(gotRequests[index+1], "GET /v1/sprites/kandev-saved/proxy?") {
			t.Fatalf("Sprites recovery request pair = %q, want only saved-resource GET and readonly proxy", gotRequests[index:index+2])
		}
	}
	mu.Lock()
	gotInstanceLookups := append([]string(nil), instanceLookups...)
	mu.Unlock()
	if len(gotInstanceLookups) != 2 {
		t.Fatalf("agentctl instance lookups = %q, want one readonly GET per recovery attempt", gotInstanceLookups)
	}
	for _, lookup := range gotInstanceLookups {
		if lookup != "GET /api/v1/instances/execution-sprites" {
			t.Fatalf("agentctl instance lookup = %q, want exact readonly GET", lookup)
		}
	}
}

func TestSpritesRecoveryClassifiesMissingSavedSprite(t *testing.T) {
	api := newSpritesAPIServer(t)
	api.mu.Lock()
	api.getStatus = http.StatusNotFound
	api.mu.Unlock()
	exec := newTestSpritesExecutor(&mockSecretStore{store: map[string]string{"sprite-token": "api-token"}})
	setter, ok := interface{}(exec).(spritesClientFactorySetter)
	if !ok {
		t.Fatal("SpritesExecutor does not expose the recovery client factory seam")
	}
	setter.setSpritesClientFactory(func(token string) *sprites.Client {
		return sprites.New(token, sprites.WithBaseURL(api.server.URL), sprites.WithDisableControl())
	})
	recovery, ok := interface{}(exec).(DetailedRecoveryBackend)
	if !ok {
		t.Fatal("SpritesExecutor must classify unknown versus absent remote recovery candidates")
	}
	instances, outcomes, err := recovery.RecoverInstancesDetailed(context.Background(), []*models.ExecutorRunning{{
		ID: "session-sprites", SessionID: "session-sprites", TaskID: "task-sprites",
		AgentExecutionID: "execution-sprites", Runtime: agentruntime.RuntimeSprites,
		Metadata: map[string]interface{}{
			MetadataKeySpriteName:             "kandev-gone",
			"env_secret_id_SPRITES_API_TOKEN": "sprite-token",
		},
	}})
	if err != nil {
		t.Fatalf("RecoverInstancesDetailed: %v", err)
	}
	if len(instances) != 0 || outcomes["session-sprites"] != RecoveryOutcomeNoMatchingInstance {
		t.Fatalf("missing saved Sprite = instances %d, outcome %q; want no matching instance", len(instances), outcomes["session-sprites"])
	}
	if len(exec.proxies) != 0 {
		t.Fatalf("missing saved Sprite opened %d host proxies", len(exec.proxies))
	}
	api.mu.Lock()
	created := append([]string(nil), api.created...)
	api.mu.Unlock()
	if len(created) != 0 {
		t.Fatalf("recovery provisioned replacement Sprites %q", created)
	}
}
