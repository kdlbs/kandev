//go:build unix

package acp

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

// openCodeLockProvider is a local OpenAI-compatible provider that holds the
// first request at a barrier. The test acquires the isolated OpenCode database
// write lock while the prompt is parked there, then releases the response so
// OpenCode hits "database is locked" while publishing its own event.
type openCodeLockProvider struct {
	server  *httptest.Server
	mu      sync.Mutex
	hold    bool
	reached chan struct{}
	release chan struct{}
	once    sync.Once
	hits    atomic.Int32
}

func newOpenCodeLockProvider(t *testing.T) *openCodeLockProvider {
	t.Helper()
	provider := &openCodeLockProvider{
		hold:    true,
		reached: make(chan struct{}),
		release: make(chan struct{}),
	}
	provider.server = httptest.NewServer(http.HandlerFunc(provider.handle))
	t.Cleanup(provider.server.Close)
	return provider
}

func (p *openCodeLockProvider) handle(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/v1/chat/completions" {
		http.NotFound(w, r)
		return
	}
	p.hits.Add(1)
	p.mu.Lock()
	hold := p.hold
	p.mu.Unlock()
	if hold {
		p.once.Do(func() { close(p.reached) })
		select {
		case <-p.release:
		case <-r.Context().Done():
			return
		}
	}
	chunk, _ := json.Marshal(map[string]any{
		"id": "chatcmpl-lock", "object": "chat.completion.chunk", "created": 1, "model": "local-model",
		"choices": []map[string]any{{"index": 0, "delta": map[string]any{"role": "assistant", "content": "continued after the lock error"}, "finish_reason": nil}},
	})
	done, _ := json.Marshal(map[string]any{
		"id": "chatcmpl-lock", "object": "chat.completion.chunk", "created": 1, "model": "local-model",
		"choices": []map[string]any{{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}},
	})
	w.Header().Set("Content-Type", "text/event-stream")
	_, _ = fmt.Fprintf(w, "data: %s\n\ndata: %s\n\ndata: [DONE]\n\n", chunk, done)
}

func (p *openCodeLockProvider) releaseHeldRequest() {
	p.mu.Lock()
	p.hold = false
	release := p.release
	p.mu.Unlock()
	close(release)
}

// TestOpenCodeLockTimeoutNativeCompatibility reproduces the captured SQLite
// lock timeout on the installed OpenCode runtime: the prompt fails, the process
// and initialized native session stay available, and a distinct follow-up runs
// on that same session after the lock is released.
//
// @covers AC-PLATFORM-TURN-CONTINUITY-001.3, AC-PLATFORM-TURN-CONTINUITY-004.2, AC-PLATFORM-TURN-CONTINUITY-004.3
func TestOpenCodeLockTimeoutNativeCompatibility(t *testing.T) {
	if os.Getenv(openCodeNativeOptInEnv) != "1" {
		t.Skip("set " + openCodeNativeOptInEnv + "=1 to run the installed-runtime compatibility test")
	}
	binary, err := exec.LookPath("opencode")
	require.NoError(t, err, "the opted-in native test requires an installed opencode runtime")

	tempRoot := t.TempDir()
	configHome := filepath.Join(tempRoot, "config")
	dataHome := filepath.Join(tempRoot, "data")
	workDir := filepath.Join(tempRoot, "work")
	for _, dir := range []string{workDir, dataHome, filepath.Join(tempRoot, "state"), filepath.Join(tempRoot, "cache")} {
		require.NoError(t, os.MkdirAll(dir, 0o700))
	}
	provider := newOpenCodeLockProvider(t)
	writeOpenCodeNativeConfig(t, configHome, provider.server.URL)

	version, err := exec.Command(binary, "--version").CombinedOutput()
	require.NoError(t, err, "resolving the OpenCode version failed: %s", string(version))
	t.Logf("opencode version: %s", strings.TrimSpace(string(version)))

	cmd := exec.Command(binary, "acp")
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(),
		"HOME="+tempRoot,
		"XDG_CONFIG_HOME="+configHome,
		"XDG_DATA_HOME="+dataHome,
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	require.NoError(t, a.Initialize(ctx))
	_, err = a.NewSession(ctx, nil)
	require.NoError(t, err, "native session/new failed: %s", lastLines(logPath, 40))
	nativeSessionID := a.sessionID
	require.NotEmpty(t, nativeSessionID)

	promptDone := make(chan error, 1)
	go func() { promptDone <- a.Prompt(ctx, "Prove a database lock does not end the conversation", nil, 1) }()
	select {
	case <-provider.reached:
	case <-time.After(2 * time.Minute):
		t.Fatalf("native provider request never reached the barrier: %s", lastLines(logPath, 40))
	}

	var dbFiles []string
	_ = filepath.WalkDir(dataHome, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr == nil && !entry.IsDir() && strings.HasSuffix(path, ".db") {
			dbFiles = append(dbFiles, path)
		}
		return nil
	})
	t.Logf("opencode db files before lock: %v", dbFiles)
	var dataFiles []string
	_ = filepath.WalkDir(dataHome, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr == nil && !entry.IsDir() && len(dataFiles) < 40 {
			dataFiles = append(dataFiles, strings.TrimPrefix(path, dataHome))
		}
		return nil
	})
	t.Logf("opencode data files: %v", dataFiles)
	dbPath := filepath.Join(dataHome, "opencode", "opencode.db")
	require.FileExists(t, dbPath, "the isolated OpenCode database was not created")
	db, dbErr := sql.Open("sqlite3", dbPath+"?_busy_timeout=200&_journal_mode=WAL")
	require.NoError(t, dbErr)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.Ping())
	_, err = db.Exec("BEGIN IMMEDIATE")
	require.NoError(t, err, "could not acquire the OpenCode database writer lock")
	lockHeld := true
	defer func() {
		if lockHeld {
			_, _ = db.Exec("ROLLBACK")
		}
	}()

	provider.releaseHeldRequest()
	// OpenCode's SQL writer gives up after its own bounded lock timeout. Keep
	// the lock for that window, then release it as the production environment
	// eventually did.
	time.Sleep(12 * time.Second)
	_, err = db.Exec("ROLLBACK")
	require.NoError(t, err)
	lockHeld = false
	_ = db.Close()
	failure := waitForNativeSettlement(t, a, 2*time.Minute, logPath, provider)
	t.Logf("native lock failure disposition=%q message=%q", failure.PromptFailureDisposition, failure.Error)
	require.Equal(t, uint64(1), failure.PromptGeneration)
	require.Equal(t, streams.PromptFailureDispositionRetainRuntime, failure.PromptFailureDisposition,
		"a real OpenCode SQLite lock timeout must retain the runtime: %s", lastLines(logPath, 60))
	require.NotNil(t, failure.ProviderError)
	require.NotEmpty(t, failure.ProviderError.Message)
	lowerMessage := strings.ToLower(failure.ProviderError.Message)
	require.True(t, strings.Contains(lowerMessage, "database is locked") ||
		strings.Contains(lowerMessage, "failed to execute statement"),
		"unexpected native lock diagnostic: %q", failure.ProviderError.Message)
	select {
	case err := <-promptDone:
		require.NoError(t, err, "the retained native prompt must settle through the error event")
	case <-time.After(30 * time.Second):
		t.Fatal("the native prompt did not settle after the lock failure")
	}

	require.NoError(t, a.Prompt(ctx, "Continue after releasing the database lock", nil, 2),
		"native follow-up prompt failed: %s", lastLines(logPath, 60))
	complete := waitForNativeEvent(t, a, streams.EventTypeComplete, 2*time.Minute)
	require.Equal(t, uint64(2), complete.PromptGeneration)
	require.Equal(t, nativeSessionID, a.sessionID, "the follow-up must reuse the initialized native session")
	require.Nil(t, cmd.ProcessState, "the ACP runtime must stay alive across the lock failure")
	t.Logf("native follow-up completed on pid %d, session %s, connection %p; provider requests: %d",
		cmd.Process.Pid, nativeSessionID, a.acpConn, provider.hits.Load())
}

// waitForNativeSettlement consumes adapter events until the prompt settles,
// logging observed frames so a native incompatibility is diagnosable.
func waitForNativeSettlement(
	t *testing.T,
	a *Adapter,
	timeout time.Duration,
	logPath string,
	provider *openCodeLockProvider,
) AgentEvent {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case event := <-a.updatesCh:
			t.Logf("native event: type=%q disposition=%q message=%q", event.Type, event.PromptFailureDisposition, event.Error)
			switch event.Type {
			case streams.EventTypeError:
				return event
			case streams.EventTypeComplete:
				t.Fatalf("the prompt completed despite the held database lock (provider hits=%d): %s",
					provider.hits.Load(), lastLines(logPath, 80))
			}
		case <-deadline:
			raw, _ := os.ReadFile(logPath)
			var lockLines []string
			for _, line := range strings.Split(string(raw), "\n") {
				lower := strings.ToLower(line)
				if strings.Contains(lower, "locked") || strings.Contains(lower, "sqlite") {
					lockLines = append(lockLines, line)
					if len(lockLines) == 10 {
						break
					}
				}
			}
			t.Fatalf("prompt did not settle; provider hits=%d lock log lines=%v stderr=%s",
				provider.hits.Load(), lockLines, lastLines(logPath, 80))
		}
	}
}
