package lifecycle

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// newRemediateTestManager builds a Manager with just enough wiring for
// classifyAndMaybeRemediate to run: a logger wrapper around a no-op zap
// and nothing else. Other manager subsystems are nil; the method under
// test must not touch them. We avoid the package-wide newTestManager
// helper because it spins up the full lifecycle DI graph, which is more
// surface area than these failure-boundary tests need.
func newRemediateTestManager(t *testing.T) *Manager {
	t.Helper()
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("NewFromZap: %v", err)
	}
	return &Manager{logger: log}
}

func TestClassifyAndMaybeRemediate_TriggersRemediation_OnNpxCacheCorrupted(t *testing.T) {
	stderr := strings.Join([]string{
		"npm error code ENOTEMPTY",
		"npm error path /Users/test/.npm/_npx/d820eb7d96bc2600/node_modules/foo",
		"npm error ENOTEMPTY: directory not empty",
	}, "\n")

	var (
		mu         sync.Mutex
		calledWith string
		done       = make(chan struct{}, 1)
	)
	m := newRemediateTestManager(t)
	m.remediateNpxCache = func(path string, _ *zap.Logger) error {
		mu.Lock()
		calledWith = path
		mu.Unlock()
		done <- struct{}{}
		return nil
	}

	exec := &AgentExecution{ID: "exec-1", AgentID: "claude-acp", TaskID: "t-1", SessionID: "s-1"}
	m.classifyAndMaybeRemediate(exec, 190, stderr)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("remediation hook was not invoked within 1s")
	}

	mu.Lock()
	defer mu.Unlock()
	want := "/Users/test/.npm/_npx/d820eb7d96bc2600"
	if calledWith != want {
		t.Errorf("remediateNpxCache called with %q, want %q", calledWith, want)
	}
}

func TestClassifyAndMaybeRemediate_NoCallOnUnrelatedFailure(t *testing.T) {
	m := newRemediateTestManager(t)
	called := make(chan struct{}, 1)
	m.remediateNpxCache = func(string, *zap.Logger) error {
		called <- struct{}{}
		return nil
	}

	exec := &AgentExecution{ID: "exec-2", AgentID: "claude-acp"}
	m.classifyAndMaybeRemediate(exec, 1, "some unrelated stderr")

	if len(called) != 0 {
		t.Fatal("remediation hook should not have been called")
	}
}
