//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/pkg/agent"
)

const (
	opencodeRealE2EOptIn  = "KANDEV_OPENCODE_REAL_E2E"
	opencodeHistoryPhrase = "amber-otter-731"
)

var (
	opencodeInstallMu   sync.Mutex
	opencodeInstallRoot string
	opencodeBinaries    = make(map[string]string)
	opencodeInstallErr  error
)

func TestMain(m *testing.M) {
	code := m.Run()
	opencodeInstallMu.Lock()
	root := opencodeInstallRoot
	opencodeInstallMu.Unlock()
	if root != "" {
		_ = os.RemoveAll(root)
	}
	os.Exit(code)
}

func TestOpenCodeACP_V1ToV2Resume(t *testing.T) {
	requireOpenCodeRealE2EOptIn(t)
	v1 := installOpenCodeE2EPackage(t, managedruntime.OpenCodeFamilyV1)
	v2 := installOpenCodeE2EPackage(t, managedruntime.OpenCodeFamilyV2)
	provider := newOpenCodeHistoryProvider(t)
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatalf("create isolated workspace: %v", err)
	}
	home := filepath.Join(root, "home")
	configPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	databasePath := filepath.Join(root, "shared-opencode.db")
	writeOpenCodeE2EConfig(t, configPath, provider.server.URL, managedruntime.OpenCodeFamilyV1)
	env := isolatedOpenCodeE2EEnv(home, configPath, databasePath)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	firstManager := startOpenCodeE2EManager(t, ctx, v1, managedruntime.OpenCodeFamilyV1, workspace, env)
	firstAdapter := firstManager.GetAdapter()
	if err := firstAdapter.Initialize(ctx); err != nil {
		stopOpenCodeE2EManager(t, firstManager)
		t.Fatalf("OpenCode v1 initialize failed: %v", err)
	}
	sessionID, err := firstAdapter.NewSession(ctx, nil)
	if err != nil {
		stopOpenCodeE2EManager(t, firstManager)
		t.Fatalf("OpenCode v1 session/new failed: %v", err)
	}
	selectOpenCodeE2EModel(t, ctx, firstAdapter, "kandev-test/history-fixture")
	assertOpenCodeE2EModel(t, firstManager, firstAdapter, "kandev-test/history-fixture")
	firstEvents := collectEventsUntilPromptDone(ctx, t, firstManager, firstAdapter,
		"Remember this private phrase for the next message: "+opencodeHistoryPhrase+". Confirm only with 'saved'.")
	AssertHasEventType(t, firstEvents, adapter.EventTypeComplete)
	if !strings.Contains(openCodeEventText(firstEvents), "saved") {
		t.Fatalf("v1 response did not confirm the first turn: %q", openCodeEventText(firstEvents))
	}
	if sessionID == "" || firstAdapter.GetSessionID() != sessionID {
		t.Fatalf("v1 session ID changed after prompt: new=%q current=%q", sessionID, firstAdapter.GetSessionID())
	}
	if err := firstManager.Stop(ctx); err != nil {
		t.Fatalf("stop OpenCode v1 process: %v", err)
	}
	if firstManager.Status() != process.StatusStopped {
		t.Fatalf("OpenCode v1 process status after stop = %s", firstManager.Status())
	}

	writeOpenCodeE2EConfig(t, configPath, provider.server.URL, managedruntime.OpenCodeFamilyV2)
	secondManager := startOpenCodeE2EManager(t, ctx, v2, managedruntime.OpenCodeFamilyV2, workspace, env)
	secondAdapter := secondManager.GetAdapter()
	if err := secondAdapter.Initialize(ctx); err != nil {
		stopOpenCodeE2EManager(t, secondManager)
		t.Fatalf("OpenCode v2 initialize failed: %v", err)
	}
	if err := secondAdapter.LoadSession(ctx, sessionID, nil); err != nil {
		stopOpenCodeE2EManager(t, secondManager)
		t.Fatalf("OpenCode v2 could not load saved v1 session %q: %v", sessionID, err)
	}
	if got := secondAdapter.GetSessionID(); got != sessionID {
		stopOpenCodeE2EManager(t, secondManager)
		t.Fatalf("OpenCode v2 restored session ID %q, want saved ID %q", got, sessionID)
	}
	if modelState, ok := secondAdapter.(adapter.SessionModelStateProvider); ok {
		state := modelState.GetSessionModelState()
		if state == nil || !strings.Contains(state.CurrentModelID, "history-fixture") {
			stopOpenCodeE2EManager(t, secondManager)
			t.Fatalf("OpenCode v2 restored model state = %+v, want history fixture model", state)
		}
	}
	secondEvents := collectEventsUntilPromptDone(ctx, t, secondManager, secondAdapter,
		"What private phrase did I ask you to remember? Reply with the exact phrase only.")
	AssertHasEventType(t, secondEvents, adapter.EventTypeComplete)
	if !strings.Contains(openCodeEventText(secondEvents), opencodeHistoryPhrase) {
		stopOpenCodeE2EManager(t, secondManager)
		t.Fatalf("OpenCode v2 did not recall saved conversation history; response: %q", openCodeEventText(secondEvents))
	}
	if !provider.sawHistory() {
		stopOpenCodeE2EManager(t, secondManager)
		t.Fatal("test provider did not receive the v1 conversation history on the v2 turn")
	}
	if provider.requestCount() < 2 {
		t.Fatalf("local test provider received %d chat requests, want both v1 and v2 turns", provider.requestCount())
	}
	if err := secondManager.Stop(ctx); err != nil {
		t.Fatalf("stop OpenCode v2 process: %v", err)
	}
	if secondManager.Status() != process.StatusStopped {
		t.Fatalf("OpenCode v2 process status after stop = %s", secondManager.Status())
	}
}

func TestOpenCodeACP_V2Compatibility(t *testing.T) {
	requireOpenCodeRealE2EOptIn(t)
	binary := installOpenCodeE2EPackage(t, managedruntime.OpenCodeFamilyV2)
	provider := newOpenCodeHistoryProvider(t)
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatalf("create isolated workspace: %v", err)
	}
	home := filepath.Join(root, "home")
	configPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	writeOpenCodeV2BuiltinE2EConfig(t, configPath, provider.server.URL)
	env := isolatedOpenCodeE2EEnv(
		home, configPath, filepath.Join(root, "opencode.db"),
	)
	assertOpenCodeE2EResolvedConfig(t, binary, workspace, env, "openai", "gpt-4o-mini")
	assertOpenCodeE2EModelAvailable(t, binary, workspace, env, "openai/gpt-4o-mini")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	mgr := startOpenCodeE2EManager(t, ctx, binary, managedruntime.OpenCodeFamilyV2, workspace, env)
	defer stopOpenCodeE2EManager(t, mgr)
	adpt := mgr.GetAdapter()
	if err := adpt.Initialize(ctx); err != nil {
		t.Fatalf("OpenCode v2 initialize failed: %v", err)
	}
	if _, err := adpt.NewSession(ctx, nil); err != nil {
		t.Fatalf("OpenCode v2 session/new failed: %v", err)
	}
	selectOpenCodeE2EModel(t, ctx, adpt, "openai/gpt-4o-mini")
	assertOpenCodeE2EModel(t, mgr, adpt, "openai/gpt-4o-mini")
	events := collectEventsUntilPromptDone(ctx, t, mgr, adpt, "Reply with the exact word ready.")
	AssertHasEventType(t, events, adapter.EventTypeComplete)
	if !strings.Contains(openCodeEventText(events), "ready") {
		t.Fatalf("OpenCode v2 response = %q, want ready", openCodeEventText(events))
	}
	if provider.requestCount() < 1 {
		t.Fatalf("local test provider received %d chat requests, want at least the selected model's prompt", provider.requestCount())
	}
}

func requireOpenCodeRealE2EOptIn(t *testing.T) {
	t.Helper()
	if os.Getenv(opencodeRealE2EOptIn) != "1" {
		t.Skipf("set %s=1 to install exact OpenCode npm packages and run the isolated real ACP compatibility suite", opencodeRealE2EOptIn)
	}
}

func installOpenCodeE2EPackage(t *testing.T, family managedruntime.OpenCodeFamily) string {
	t.Helper()
	spec, err := agents.NewOpenCodeACP().ManagedNPMRuntimeForFamily(family)
	if err != nil {
		t.Fatalf("resolve OpenCode %s test package: %v", family, err)
	}
	packageSpec := spec.PackageSpec("")
	opencodeInstallMu.Lock()
	defer opencodeInstallMu.Unlock()
	if opencodeInstallErr != nil {
		t.Fatalf("real OpenCode ACP prerequisite failed: %v", opencodeInstallErr)
	}
	if binary := opencodeBinaries[packageSpec]; binary != "" {
		return binary
	}
	if opencodeInstallRoot == "" {
		opencodeInstallRoot, err = os.MkdirTemp("", "kandev-opencode-real-e2e-")
		if err != nil {
			opencodeInstallErr = fmt.Errorf("create test-owned npm runtime storage: %w", err)
			t.Fatalf("real OpenCode ACP prerequisite failed: %v", opencodeInstallErr)
		}
	}
	if _, err := exec.LookPath("npm"); err != nil {
		opencodeInstallErr = fmt.Errorf("npm is required to acquire %s: %w", packageSpec, err)
		t.Fatalf("real OpenCode ACP prerequisite failed: %v", opencodeInstallErr)
	}
	name := strings.NewReplacer("@", "", "/", "-", ".", "-").Replace(packageSpec)
	installRoot := filepath.Join(opencodeInstallRoot, name)
	if err := os.MkdirAll(installRoot, 0o700); err != nil {
		opencodeInstallErr = fmt.Errorf("create isolated npm project for %s: %w", packageSpec, err)
		t.Fatalf("real OpenCode ACP prerequisite failed: %v", opencodeInstallErr)
	}
	cmd := exec.Command("npm", "install", "--prefix", installRoot, "--no-audit", "--no-fund", packageSpec)
	cmd.Env = isolatedOpenCodeE2EEnv(filepath.Join(opencodeInstallRoot, "home"), "", "")
	if output, runErr := cmd.CombinedOutput(); runErr != nil {
		_ = output // Package-manager output can include registry details; keep it out of logs.
		opencodeInstallErr = fmt.Errorf("npm could not install exact package %s: %w", packageSpec, runErr)
		t.Fatalf("real OpenCode ACP prerequisite failed: %v", opencodeInstallErr)
	}
	binary := filepath.Join(installRoot, "node_modules", ".bin", "opencode")
	if _, err := os.Stat(binary); err != nil {
		opencodeInstallErr = fmt.Errorf("exact package %s did not provide an opencode executable: %w", packageSpec, err)
		t.Fatalf("real OpenCode ACP prerequisite failed: %v", opencodeInstallErr)
	}
	opencodeBinaries[packageSpec] = binary
	return binary
}

func newOpenCodeHistoryProvider(t *testing.T) *openCodeHistoryProvider {
	t.Helper()
	provider := &openCodeHistoryProvider{}
	provider.server = httptest.NewServer(http.HandlerFunc(provider.handle))
	t.Cleanup(provider.server.Close)
	return provider
}

type openCodeHistoryProvider struct {
	server      *httptest.Server
	mu          sync.Mutex
	historySeen bool
	requests    int
}

func (p *openCodeHistoryProvider) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/models") {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"history-fixture","object":"model","created":1,"owned_by":"kandev-e2e"},{"id":"gpt-4o-mini","object":"model","created":1,"owned_by":"kandev-e2e"}]}`))
		return
	}
	if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/chat/completions") {
		http.NotFound(w, r)
		return
	}
	p.mu.Lock()
	p.requests++
	p.mu.Unlock()
	body, err := ioReadAll(r)
	if err != nil {
		http.Error(w, "request body unavailable", http.StatusBadRequest)
		return
	}
	var request struct {
		Stream bool   `json:"stream"`
		Model  string `json:"model"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		http.Error(w, "invalid chat completion request", http.StatusBadRequest)
		return
	}
	hasHistory := bytes.Contains(body, []byte(opencodeHistoryPhrase))
	if bytes.Contains(body, []byte("What private phrase did I ask you to remember?")) && hasHistory {
		p.mu.Lock()
		p.historySeen = true
		p.mu.Unlock()
	}
	answer := "ready"
	if bytes.Contains(body, []byte("What private phrase did I ask you to remember?")) {
		answer = "not in conversation"
		if hasHistory {
			answer = opencodeHistoryPhrase
		}
	} else if bytes.Contains(body, []byte("Remember this private phrase for the next message:")) {
		answer = "saved"
	}
	if request.Stream {
		writeOpenAIStream(w, answer, request.Model)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id": "chatcmpl-kandev-test", "object": "chat.completion", "created": 1, "model": request.Model,
		"choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": answer}, "finish_reason": "stop"}},
		"usage":   map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
	})
}

func (p *openCodeHistoryProvider) sawHistory() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.historySeen
}

func (p *openCodeHistoryProvider) requestCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.requests
}

func assertOpenCodeE2EModel(t *testing.T, mgr *process.Manager, adpt adapter.AgentAdapter, expected string) {
	t.Helper()
	modelState, ok := adpt.(adapter.SessionModelStateProvider)
	if !ok {
		t.Fatal("OpenCode ACP adapter does not expose session model state")
	}
	state := modelState.GetSessionModelState()
	if state == nil || state.CurrentModelID != expected {
		t.Fatalf("OpenCode selected model = %+v, want %s from isolated provider config; stderr: %v", state, expected, mgr.GetRecentStderr())
	}
}

func selectOpenCodeE2EModel(t *testing.T, ctx context.Context, adpt adapter.AgentAdapter, modelID string) {
	t.Helper()
	modelSetter, ok := adpt.(adapter.ModelSettableAdapter)
	if !ok {
		t.Fatal("OpenCode ACP adapter does not support session model selection")
	}
	if err := modelSetter.SetModel(ctx, modelID); err != nil {
		t.Fatalf("select isolated test model: %v", err)
	}
}

func assertOpenCodeE2EResolvedConfig(t *testing.T, binary, workspace string, env []string, providerID, modelID string) {
	t.Helper()
	cmd := exec.Command(binary, "debug", "config")
	cmd.Dir = workspace
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("read resolved OpenCode v2 config: %v: %s", err, output)
	}
	if !bytes.Contains(output, []byte(`"providerID": "`+providerID+`"`)) || !bytes.Contains(output, []byte(`"model": "`+modelID+`"`)) {
		t.Fatalf("OpenCode v2 did not load the isolated local provider config: %s", output)
	}
}

func assertOpenCodeE2EModelAvailable(t *testing.T, binary, workspace string, env []string, modelID string) {
	t.Helper()
	cmd := exec.Command(binary, "models")
	cmd.Dir = workspace
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("list OpenCode v2 models: %v: %s", err, output)
	}
	if !bytes.Contains(output, []byte(modelID)) {
		t.Fatalf("OpenCode v2 model catalog omitted the configured test model: %s", output)
	}
}

func writeOpenAIStream(w http.ResponseWriter, answer, modelID string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = fmt.Fprintf(w, "data: %s\n\n", `{"id":"chatcmpl-kandev-test","object":"chat.completion.chunk","created":1,"model":"`+modelID+`","choices":[{"index":0,"delta":{"role":"assistant","content":"`+answer+`"},"finish_reason":null}]}`)
	_, _ = fmt.Fprint(w, `data: {"id":"chatcmpl-kandev-test","object":"chat.completion.chunk","created":1,"model":"`+modelID+`","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`+"\n\n")
	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
}

func writeOpenCodeE2EConfig(t *testing.T, path, endpoint string, family managedruntime.OpenCodeFamily) {
	t.Helper()
	var value any
	if family == managedruntime.OpenCodeFamilyV1 {
		value = map[string]any{
			"$schema":    "https://opencode.ai/config.json",
			"model":      "kandev-test/history-fixture",
			"autoupdate": false,
			"provider": map[string]any{
				"kandev-test": map[string]any{
					"npm": "@ai-sdk/openai-compatible", "name": "Kandev local test provider",
					"options": map[string]any{"baseURL": endpoint + "/v1", "apiKey": "local-test-key"},
					"models": map[string]any{"history-fixture": map[string]any{
						"name": "History fixture", "limit": map[string]any{"context": 200000, "output": 32000},
					}},
				},
			},
		}
	} else {
		value = map[string]any{
			"$schema":    "https://opencode.ai/config.json",
			"model":      "kandev-test/history-fixture",
			"autoupdate": false,
			"providers": map[string]any{
				"kandev-test": map[string]any{
					"package":  "@opencode/ai/providers/openai-compatible",
					"name":     "Kandev local test provider",
					"env":      []string{"OPENAI_API_KEY"},
					"settings": map[string]any{"baseURL": endpoint + "/v1"},
					"models": map[string]any{"history-fixture": map[string]any{
						"name": "History fixture", "limit": map[string]any{"context": 200000, "output": 32000},
					}},
				},
			},
		}
	}
	writeOpenCodeConfigValue(t, path, value)
}

func writeOpenCodeV2BuiltinE2EConfig(t *testing.T, path, endpoint string) {
	t.Helper()
	value := map[string]any{
		"$schema":    "https://opencode.ai/config.json",
		"model":      "openai/gpt-4o-mini",
		"autoupdate": false,
		"providers": map[string]any{
			"openai": map[string]any{
				"package":  "@opencode/ai/providers/openai/chat",
				"settings": map[string]any{"baseURL": endpoint + "/v1"},
				"models": map[string]any{"gpt-4o-mini": map[string]any{
					"name": "History fixture", "limit": map[string]any{"context": 200000, "output": 32000},
				}},
			},
		},
	}
	writeOpenCodeConfigValue(t, path, value)
}

func writeOpenCodeConfigValue(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("encode isolated OpenCode config: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create isolated OpenCode config directory: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write isolated OpenCode config: %v", err)
	}
}

func isolatedOpenCodeE2EEnv(home, configPath, databasePath string) []string {
	values := make(map[string]string)
	for _, key := range []string{
		"PATH", "TMPDIR", "TMP", "TEMP", "SYSTEMROOT", "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY",
		"SSL_CERT_FILE", "NODE_EXTRA_CA_CERTS",
	} {
		if value := os.Getenv(key); value != "" {
			values[key] = value
		}
	}
	values["HOME"] = home
	values["XDG_CONFIG_HOME"] = filepath.Join(home, ".config")
	values["XDG_DATA_HOME"] = filepath.Join(home, ".local", "share")
	values["XDG_CACHE_HOME"] = filepath.Join(home, ".cache")
	values["XDG_STATE_HOME"] = filepath.Join(home, ".local", "state")
	values["npm_config_cache"] = filepath.Join(home, ".npm-cache")
	values["OPENAI_API_KEY"] = "local-test-key"
	values["NO_COLOR"] = "1"
	values["NO_PROXY"] = appendLoopbackNoProxy(values["NO_PROXY"])
	values["no_proxy"] = values["NO_PROXY"]
	values["CI"] = "1"
	if configPath != "" {
		values["OPENCODE_CONFIG"] = configPath
	}
	if databasePath != "" {
		values["OPENCODE_DB"] = databasePath
	}
	env := make([]string, 0, len(values))
	for key, value := range values {
		env = append(env, key+"="+value)
	}
	return env
}

func appendLoopbackNoProxy(existing string) string {
	parts := []string{"127.0.0.1", "localhost", "::1"}
	if existing != "" {
		parts = append(parts, existing)
	}
	return strings.Join(parts, ",")
}

func startOpenCodeE2EManager(
	t *testing.T,
	ctx context.Context,
	binary string,
	family managedruntime.OpenCodeFamily,
	workspace string,
	env []string,
) *process.Manager {
	t.Helper()
	provider := agents.NewOpenCodeACP()
	spec, err := provider.ManagedNPMRuntimeForFamily(family)
	if err != nil {
		t.Fatalf("resolve OpenCode %s command: %v", family, err)
	}
	command := binary + " " + strings.Join(spec.ACPArgs, " ")
	cfg := buildInstanceConfig(command, agent.ProtocolACP, workspace, true, "")
	cfg.AgentCommand = command
	cfg.AgentArgs = config.ParseCommand(command)
	cfg.AgentEnv = env
	cfg.AgentType = provider.ID()
	cfg.RequiresProcessKill = true
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stderr"})
	if err != nil {
		t.Fatalf("create OpenCode e2e logger: %v", err)
	}
	mgr := process.NewManager(cfg, log)
	if err := mgr.Start(ctx); err != nil {
		_ = mgr.Stop(context.Background())
		t.Fatalf("start OpenCode %s process: %v", binary, err)
	}
	t.Cleanup(func() { stopOpenCodeE2EManager(t, mgr) })
	if mgr.GetAdapter() == nil {
		stopOpenCodeE2EManager(t, mgr)
		t.Fatal("OpenCode process manager has no ACP adapter")
	}
	return mgr
}

func stopOpenCodeE2EManager(t *testing.T, mgr *process.Manager) {
	t.Helper()
	if mgr == nil || mgr.Status() == process.StatusStopped {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := mgr.Stop(ctx); err != nil {
		t.Errorf("stop OpenCode test process: %v", err)
	}
	if mgr.Status() != process.StatusStopped {
		t.Errorf("OpenCode test process remained in status %s", mgr.Status())
	}
}

func openCodeEventText(events []adapter.AgentEvent) string {
	var text strings.Builder
	for _, event := range events {
		if event.Type == adapter.EventTypeMessageChunk {
			text.WriteString(event.Text)
		}
	}
	return text.String()
}

func ioReadAll(r *http.Request) ([]byte, error) {
	body, readErr := io.ReadAll(r.Body)
	closeErr := r.Body.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return body, nil
}
