//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/pkg/agent"
)

// TestOpenCodePluginPackagedShellOutcome proves the complete agentctl path with
// the real OpenCode 2.2.0 CLI: automatic plugin supply, native subscription,
// ACP launch claim, private callback, commit-before-ack, and one journaled
// workload observation. It is opt-in because it installs the exact package.
func TestOpenCodePluginPackagedShellOutcome(t *testing.T) {
	requireOpenCodeRealE2EOptIn(t)
	binary := installPluginConformanceCLI(t)
	provider := newOpenCodeShellOutcomeProvider(t)
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	home := filepath.Join(root, "home")
	configPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	writeOpenCodeV2BuiltinE2EConfig(t, configPath, provider.server.URL)
	env := isolatedOpenCodeE2EEnv(home, configPath, filepath.Join(root, "opencode.db"))

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	manager := startOpenCodePluginManager(t, ctx, binary, workspace, home, root, env)
	adpt := manager.GetAdapter()
	if err := adpt.Initialize(ctx); err != nil {
		t.Fatalf("OpenCode v2 initialize failed: %v", err)
	}
	if _, err := adpt.NewSession(ctx, nil); err != nil {
		t.Fatalf("OpenCode v2 session/new failed: %v", err)
	}
	selectOpenCodeE2EModel(t, ctx, adpt, "openai/gpt-4o-mini")
	if err := adpt.Prompt(ctx, "Run the packaged background command.", nil, 1); err != nil {
		t.Fatalf("OpenCode v2 prompt failed: %v", err)
	}

	deliveryJournal, err := manager.DeliveryJournal()
	if err != nil {
		t.Fatalf("delivery journal unavailable: %v", err)
	}
	event, found := waitForPluginJournalEvent(t, deliveryJournal, manager.DeliveryStreamID())
	if !found {
		t.Fatalf("no plugin background-work observation was committed; stderr: %v", manager.GetRecentStderr())
	}
	if event.Type != streams.EventTypeBackgroundWorkUpdated || event.BackgroundWork == nil {
		t.Fatalf("journal event = %#v, want background work", event)
	}
	work := event.BackgroundWork
	if work.State != streams.RunStateFailed || work.ExitCode == nil || *work.ExitCode != 3 {
		t.Fatalf("workload = %#v, want failed exit 3", work)
	}
	if work.SourceCallID == "" || work.Title == "" {
		t.Fatalf("workload lost its launch association: %#v", work)
	}
	if work.Capabilities.Output != "none" || len(work.Capabilities.Actions) != 0 {
		t.Fatalf("workload invented capabilities: %#v", work.Capabilities)
	}
}

type openCodeShellOutcomeProvider struct {
	server   *httptest.Server
	mu       sync.Mutex
	requests int
}

func newOpenCodeShellOutcomeProvider(t *testing.T) *openCodeShellOutcomeProvider {
	t.Helper()
	provider := &openCodeShellOutcomeProvider{}
	provider.server = httptest.NewServer(http.HandlerFunc(provider.handle))
	t.Cleanup(provider.server.Close)
	return provider
}

func (p *openCodeShellOutcomeProvider) handle(writer http.ResponseWriter, request *http.Request) {
	if request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/models") {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"object":"list","data":[{"id":"gpt-4o-mini","object":"model","created":1,"owned_by":"kandev-e2e"}]}`))
		return
	}
	if request.Method != http.MethodPost || !strings.HasSuffix(request.URL.Path, "/chat/completions") {
		http.NotFound(writer, request)
		return
	}
	body, err := ioReadAll(request)
	if err != nil {
		http.Error(writer, "request body unavailable", http.StatusBadRequest)
		return
	}
	p.mu.Lock()
	p.requests++
	p.mu.Unlock()
	var payload struct {
		Stream bool   `json:"stream"`
		Model  string `json:"model"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(writer, "invalid chat request", http.StatusBadRequest)
		return
	}
	hasToolResult := bytes.Contains(body, []byte(`"role":"tool"`))
	if !hasToolResult {
		writeOpenCodeToolCall(writer, payload.Stream, payload.Model)
		return
	}
	writeOpenAIStream(writer, "ok", payload.Model)
}

func writeOpenCodeToolCall(writer http.ResponseWriter, stream bool, model string) {
	arguments := `{"command":"sleep 1; exit 3","background":true}`
	if stream {
		writer.Header().Set("Content-Type", "text/event-stream")
		chunk := map[string]any{
			"id": "chatcmpl-kandev-plugin", "object": "chat.completion.chunk", "created": 1, "model": model,
			"choices": []any{map[string]any{
				"index": 0,
				"delta": map[string]any{
					"role": "assistant",
					"tool_calls": []any{map[string]any{
						"index": 0, "id": "call-plugin-shell", "type": "function",
						"function": map[string]any{"name": "shell", "arguments": arguments},
					}},
				},
				"finish_reason": nil,
			}},
		}
		raw, _ := json.Marshal(chunk)
		_, _ = fmt.Fprintf(writer, "data: %s\n\n", raw)
		_, _ = fmt.Fprint(writer, `data: {"id":"chatcmpl-kandev-plugin","object":"chat.completion.chunk","created":1,"model":"`+model+`","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`+"\n\n")
		_, _ = fmt.Fprint(writer, "data: [DONE]\n\n")
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(map[string]any{
		"id": "chatcmpl-kandev-plugin", "object": "chat.completion", "created": 1, "model": model,
		"choices": []any{map[string]any{
			"index": 0,
			"message": map[string]any{
				"role": "assistant", "content": nil,
				"tool_calls": []any{map[string]any{
					"id": "call-plugin-shell", "type": "function",
					"function": map[string]any{"name": "shell", "arguments": arguments},
				}},
			},
			"finish_reason": "tool_calls",
		}},
		"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
	})
}

// installPluginConformanceCLI installs the exact version proved by the
// packaged conformance harness, independent of the repository's default
// managed v2 version.
func installPluginConformanceCLI(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	cmd := exec.Command("npm", "install", "--prefix", root, "--no-audit", "--no-fund", "@opencode/cli@2.2.0")
	cmd.Env = isolatedOpenCodeE2EEnv(filepath.Join(root, "home"), "", "")
	if output, err := cmd.CombinedOutput(); err != nil {
		_ = output
		t.Fatalf("npm could not install @opencode/cli@2.2.0: %v", err)
	}
	binary := filepath.Join(root, "node_modules", ".bin", "opencode")
	if _, err := os.Stat(binary); err != nil {
		t.Fatalf("installed package did not provide opencode: %v", err)
	}
	return binary
}

func startOpenCodePluginManager(
	t *testing.T,
	ctx context.Context,
	binary, workspace, home, root string,
	env []string,
) *process.Manager {
	t.Helper()
	provider := agents.NewOpenCodeACP()
	spec, err := provider.ManagedNPMRuntimeForFamily(managedruntime.OpenCodeFamilyV2)
	if err != nil {
		t.Fatalf("resolve OpenCode v2 command: %v", err)
	}
	command := binary + " " + strings.Join(spec.ACPArgs, " ")
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stderr"})
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	journalPath := filepath.Join(root, "delivery.bbolt")
	cfg := &config.InstanceConfig{
		WorkDir:                    workspace,
		AgentCommand:               command,
		AgentArgs:                  config.ParseCommand(command),
		AgentEnv:                   env,
		AgentType:                  provider.ID(),
		Protocol:                   agent.ProtocolACP,
		RequiresProcessKill:        true,
		HomeDir:                    home,
		TaskID:                     "plugin-task",
		SessionID:                  "plugin-session",
		OpenCodePluginEnabled:      true,
		AgentBackgroundWorkEnabled: true,
		OpenCodePluginRuntime: &agentctltypes.OpenCodePluginRuntime{
			Family: "v2", Source: "managed", Version: "2.2.0",
		},
		DurableJournalPath:        journalPath,
		DeliveryStreamID:          "plugin-stream",
		DeliveryIncarnationID:     "plugin-incarnation",
		DeliveryHarnessGeneration: 1,
	}
	manager := process.NewManager(cfg, log)
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer stopCancel()
		_ = manager.Stop(stopCtx)
	})
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("start OpenCode plugin manager: %v", err)
	}
	return manager
}

func waitForPluginJournalEvent(t *testing.T, deliveryJournal *journal.Journal, streamID string) (adapter.AgentEvent, bool) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		events, _, err := deliveryJournal.Replay(context.Background(), streamID, 0, 1000)
		if err != nil {
			t.Fatalf("replay journal: %v", err)
		}
		for _, event := range events {
			var payload adapter.AgentEvent
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				continue
			}
			if payload.Type == streams.EventTypeBackgroundWorkUpdated && payload.BackgroundWork != nil &&
				payload.BackgroundWork.State == streams.RunStateFailed {
				return payload, true
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return adapter.AgentEvent{}, false
}
