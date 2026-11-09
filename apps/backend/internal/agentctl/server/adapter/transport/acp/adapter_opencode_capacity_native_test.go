//go:build unix

package acp

import (
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
	"syscall"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/stretchr/testify/require"
)

const openCodeNativeOptInEnv = "KANDEV_TEST_OPENCODE_NATIVE"

// openCodeCapacityProvider is a local, controlled OpenAI-compatible provider.
// It refuses every request with the exact captured OpenCode admission refusal
// until the test observes the terminal ACP prompt failure, then accepts the
// continuation. No external model service or live credential is involved.
type openCodeCapacityProvider struct {
	server *httptest.Server
	mu     sync.Mutex
	mode   string
	hits   int
}

const (
	openCodeProviderCapacity = "capacity"
	openCodeProviderAccept   = "accept"
)

func newOpenCodeCapacityProvider(t *testing.T) *openCodeCapacityProvider {
	t.Helper()
	provider := &openCodeCapacityProvider{mode: openCodeProviderCapacity}
	provider.server = httptest.NewServer(http.HandlerFunc(provider.handle))
	t.Cleanup(provider.server.Close)
	return provider
}

func (p *openCodeCapacityProvider) handle(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/v1/chat/completions" {
		http.NotFound(w, r)
		return
	}
	p.mu.Lock()
	p.hits++
	mode := p.mode
	p.mu.Unlock()
	if mode == openCodeProviderCapacity {
		message := "Upstream request failed: [ttft_admission_refused] the cell is at capacity; retry shortly (after 1 s)"
		body, err := json.Marshal(map[string]any{"error": map[string]any{"type": "server_error", "message": message}})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write(body)
		return
	}
	chunk, _ := json.Marshal(map[string]any{
		"id": "chatcmpl-local", "object": "chat.completion.chunk", "created": 1, "model": "local-model",
		"choices": []map[string]any{{"index": 0, "delta": map[string]any{"role": "assistant", "content": "continued after capacity"}, "finish_reason": nil}},
	})
	done, _ := json.Marshal(map[string]any{
		"id": "chatcmpl-local", "object": "chat.completion.chunk", "created": 1, "model": "local-model",
		"choices": []map[string]any{{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}},
	})
	w.Header().Set("Content-Type", "text/event-stream")
	_, _ = fmt.Fprintf(w, "data: %s\n\ndata: %s\n\ndata: [DONE]\n\n", chunk, done)
}

func (p *openCodeCapacityProvider) accept() {
	p.mu.Lock()
	p.mode = openCodeProviderAccept
	p.mu.Unlock()
}

func (p *openCodeCapacityProvider) hitCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.hits
}

func writeOpenCodeNativeConfig(t *testing.T, configHome, baseURL string) {
	t.Helper()
	dir := filepath.Join(configHome, "opencode")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	config := map[string]any{
		"$schema": "https://opencode.ai/config.json",
		"model":   "local/local-model",
		"provider": map[string]any{
			"local": map[string]any{
				"npm":  "@ai-sdk/openai-compatible",
				"name": "Local",
				"options": map[string]any{
					"baseURL": baseURL + "/v1",
					"apiKey":  "kandev-native-test",
				},
				"models": map[string]any{"local-model": map[string]any{"name": "Local Model"}},
			},
		},
	}
	raw, err := json.Marshal(config)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "opencode.json"), raw, 0o600))
}

// TestOpenCodeCapacityNativeCompatibility proves the captured refusal on the
// installed OpenCode CLI: the terminal prompt error classifies as capacity,
// the ACP runtime stays usable, and another prompt runs on the same initialized
// native session. It needs a real binary and a local controlled provider, so it
// is opt-in and fails rather than skips once opted in.
//
// @covers AC-PLATFORM-TURN-CONTINUITY-001.1, AC-PLATFORM-TURN-CONTINUITY-001.2
func TestOpenCodeCapacityNativeCompatibility(t *testing.T) {
	if os.Getenv(openCodeNativeOptInEnv) != "1" {
		t.Skip("set " + openCodeNativeOptInEnv + "=1 to run the installed-runtime compatibility test")
	}
	binary, err := exec.LookPath("opencode")
	require.NoError(t, err, "the opted-in native test requires an installed opencode runtime")

	tempRoot := t.TempDir()
	configHome := filepath.Join(tempRoot, "config")
	workDir := filepath.Join(tempRoot, "work")
	for _, dir := range []string{workDir, filepath.Join(tempRoot, "data"), filepath.Join(tempRoot, "state"), filepath.Join(tempRoot, "cache")} {
		require.NoError(t, os.MkdirAll(dir, 0o700))
	}
	provider := newOpenCodeCapacityProvider(t)
	writeOpenCodeNativeConfig(t, configHome, provider.server.URL)

	version, err := exec.Command(binary, "--version").CombinedOutput()
	require.NoError(t, err, "resolving the OpenCode version failed: %s", string(version))
	t.Logf("opencode version: %s", strings.TrimSpace(string(version)))

	cmd := exec.Command(binary, "acp")
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(),
		"HOME="+tempRoot,
		"XDG_CONFIG_HOME="+configHome,
		"XDG_DATA_HOME="+filepath.Join(tempRoot, "data"),
		"XDG_STATE_HOME="+filepath.Join(tempRoot, "state"),
		"XDG_CACHE_HOME="+filepath.Join(tempRoot, "cache"),
	)
	logPath := filepath.Join(tempRoot, "opencode-stderr.log")
	logFile, logErr := os.Create(logPath)
	require.NoError(t, logErr)
	defer func() { _ = logFile.Close() }()
	cmd.Stderr = logFile
	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = stdin.Close()
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		_ = cmd.Wait()
	})

	a := newTestAdapterForAgent(openCodeAgentID)
	a.cfg.WorkDir = workDir
	require.NoError(t, a.Connect(stdin, stdout))
	t.Cleanup(func() { _ = a.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	require.NoError(t, a.Initialize(ctx))
	_, err = a.NewSession(ctx, nil)
	require.NoError(t, err, "native session/new failed: %s", lastLines(logPath, 40))
	nativeSessionID := a.sessionID
	require.NotEmpty(t, nativeSessionID)

	started := time.Now()
	require.NoError(t, a.Prompt(ctx, "Prove an interrupted provider turn can continue", nil, 1),
		"native prompt failed: %s", lastLines(logPath, 40))
	failure := waitForNativeEvent(t, a, streams.EventTypeError, 2*time.Minute)
	settled := time.Since(started)
	t.Logf("terminal settlement after %s; provider requests: %d", settled, provider.hitCount())

	require.Equal(t, uint64(1), failure.PromptGeneration)
	require.NotNil(t, failure.ProviderError)
	require.Contains(t, failure.ProviderError.Message, "[ttft_admission_refused] the cell is at capacity")
	require.Equal(t, streams.PromptFailureDispositionRetainRuntime, failure.PromptFailureDisposition)
	require.NotNil(t, failure.CapacityContinuation)
	require.Equal(t, streams.CapacityContinuationOpenCodeLiveSessionV1, failure.CapacityContinuation.Support)
	t.Logf("classified capacity: completed_tools=%d safe=%v", failure.CapacityContinuation.CompletedTools,
		failure.CapacityContinuation.SafeFor(1))

	provider.accept()
	continued := time.Now()
	require.NoError(t, a.Prompt(ctx, "Continue the unfinished request", nil, 2),
		"native continuation prompt failed: %s", lastLines(logPath, 40))
	complete := waitForNativeEvent(t, a, streams.EventTypeComplete, 2*time.Minute)
	t.Logf("continuation completed after %s; provider requests: %d", time.Since(continued), provider.hitCount())

	require.Equal(t, uint64(2), complete.PromptGeneration)
	require.Equal(t, nativeSessionID, a.sessionID, "the continuation must reuse the initialized native session")
	require.Nil(t, cmd.ProcessState, "the ACP runtime must stay alive across the capacity failure")
}

func lastLines(path string, count int) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "no OpenCode stderr log: " + err.Error()
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) > count {
		lines = lines[len(lines)-count:]
	}
	return strings.Join(lines, "\n")
}

func waitForNativeEvent(t *testing.T, a *Adapter, eventType string, timeout time.Duration) AgentEvent {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case event := <-a.updatesCh:
			if event.Type == eventType {
				return event
			}
		case <-deadline:
			t.Fatalf("timed out after %s waiting for %q event", timeout, eventType)
		}
	}
}
