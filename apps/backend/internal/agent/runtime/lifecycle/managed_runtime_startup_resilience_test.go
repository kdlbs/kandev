package lifecycle

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
)

// @covers AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.6
// @covers AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.7
func TestManagedStartupRecoveryMixedBurstPreservesSharedTree(t *testing.T) {
	sentinel := filepath.Join(t.TempDir(), "shared-npm-tree", "sibling-sentinel")
	writeSentinel := func() {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(sentinel), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(sentinel, []byte("healthy sibling package"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeSentinel()

	healthyManager, healthyExecution, healthyMock, healthyAgent := newManagedRuntimeRetryFixture(t, false)
	recoveredManager, recoveredExecution, recoveredMock, recoveredAgent := newManagedRuntimeRetryFixture(t, false)
	exhaustedManager, exhaustedExecution, exhaustedMock, exhaustedAgent := newManagedRuntimeRetryFixture(t, true)
	for _, mock := range []*restartMockAgentctlServer{healthyMock, recoveredMock, exhaustedMock} {
		mock.onCacheRepair = func() { _ = os.Remove(sentinel) }
	}

	// Prove this fixture's repair endpoint can delete the shared tree.
	recoveredClient, release := recoveredExecution.AcquireAgentCtlClient()
	if err := recoveredClient.RepairManagedRuntimeCache(context.Background(), "opencode-ai@1.2.3"); err != nil {
		release()
		t.Fatalf("positive control repair: %v", err)
	}
	release()
	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Fatalf("positive control sentinel stat error = %v, want deletion", err)
	}
	writeSentinel()
	recoveredMock.mu.Lock()
	recoveredMock.httpActions = nil
	recoveredMock.repairPackageSpecs = nil
	recoveredMock.mu.Unlock()

	start := make(chan struct{})
	type outcome struct {
		name string
		err  error
	}
	results := make(chan outcome, 3)
	go func() {
		<-start
		_, err := healthyManager.startManagedRuntimeRetry(
			context.Background(), healthyExecution, healthyAgent, "", nil, nil,
		)
		results <- outcome{name: "healthy", err: err}
	}()
	go func() {
		<-start
		attempted, err := recoveredManager.retryManagedRuntimeStartup(
			context.Background(), recoveredExecution, managedACPInitializeFailure("initial initialize failed"),
			recoveredAgent, "", nil, nil,
		)
		if err == nil && !attempted {
			err = errors.New("recovered launch did not attempt one replacement")
		}
		results <- outcome{name: "recovered", err: err}
	}()
	go func() {
		<-start
		attempted, err := exhaustedManager.retryManagedRuntimeStartup(
			context.Background(), exhaustedExecution, managedACPInitializeFailure("initial initialize failed"),
			exhaustedAgent, "", nil, nil,
		)
		var startupErr *routingerr.ManagedRuntimeStartupError
		if !attempted || !errors.As(err, &startupErr) || startupErr.Code != routingerr.CodeAgentRuntime {
			if err == nil {
				err = errors.New("exhausted launch did not report its final post-initialize failure")
			}
		} else {
			err = nil
		}
		results <- outcome{name: "exhausted", err: err}
	}()
	close(start)
	for range 3 {
		result := <-results
		if result.err != nil {
			t.Fatalf("mixed burst %s outcome: %v", result.name, result.err)
		}
	}

	if got := healthyMock.getHTTPActions(); !slices.Equal(got, []string{"configure", "start"}) {
		t.Fatalf("healthy sibling actions = %#v, want live initial launch only", got)
	}
	for name, mock := range map[string]*restartMockAgentctlServer{
		"recovered": recoveredMock,
		"exhausted": exhaustedMock,
	} {
		if got := mock.getHTTPActions(); !slices.Equal(got, []string{"stop", "configure", "start"}) {
			t.Errorf("%s actions = %#v, want one stop and one replacement", name, got)
		}
		if got := mock.getManagedRuntimeRepairSpecs(); len(got) != 0 {
			t.Errorf("%s recovery called cache repair: %#v", name, got)
		}
	}
	if data, err := os.ReadFile(sentinel); err != nil || string(data) != "healthy sibling package" {
		t.Fatalf("shared sibling sentinel = (%q, %v), want preserved file", data, err)
	}
}
