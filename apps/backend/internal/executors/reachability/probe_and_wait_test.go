package reachability

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-EXECUTORS-SSH-REACHABILITY-002.2
//
// Two overlapping immediate-probe callers for the same executor must
// coalesce into exactly one dial and receive one identical record each — the
// acceptance criterion this task adds ProbeAndWait to satisfy. The blocking
// probeFunc plus a short settle delay before releasing mirrors
// golang.org/x/sync/singleflight's own TestDoDupSuppress: there is no faster
// deterministic signal for "the second caller has entered Do" without
// instrumenting production code for the test.
func TestPollerProbeAndWait_CoalescesConcurrentCallsForSameExecutor(t *testing.T) {
	repo := newFakeRepository()
	p := New(repo, 60, logger.Default())
	p.Start(context.Background())
	defer p.Stop()

	var dialCount int32
	started := make(chan struct{})
	release := make(chan struct{})
	p.probe = func(_ context.Context, executor *models.Executor) agentruntime.SSHProbeOutcome {
		if atomic.AddInt32(&dialCount, 1) == 1 {
			close(started)
		}
		<-release
		return agentruntime.SSHProbeOutcome{Success: true, Host: executor.Config["ssh_host"]}
	}

	executor := sshExecutor("exec-coalesce")

	type call struct {
		result ProbeResult
		ran    bool
	}
	results := make(chan call, 2)
	go func() {
		result, ran := p.ProbeAndWait(executor)
		results <- call{result, ran}
	}()
	<-started
	go func() {
		result, ran := p.ProbeAndWait(executor)
		results <- call{result, ran}
	}()
	time.Sleep(100 * time.Millisecond)
	close(release)

	first := <-results
	second := <-results

	if got := atomic.LoadInt32(&dialCount); got != 1 {
		t.Fatalf("dial count = %d, want exactly 1 (coalesced)", got)
	}
	if !first.ran || !second.ran {
		t.Fatalf("ran = %v, %v, want both true", first.ran, second.ran)
	}
	firstJSON, _ := json.Marshal(first.result)
	secondJSON, _ := json.Marshal(second.result)
	if string(firstJSON) != string(secondJSON) {
		t.Fatalf("responses differ:\n  first:  %s\n  second: %s", firstJSON, secondJSON)
	}
	if !first.result.Persisted {
		t.Fatalf("Persisted = false, want true — the probe outcome was a plain success")
	}
}

// @covers AC-EXECUTORS-SSH-REACHABILITY-002.2
//
// The singleflight group shares one result across coalesced callers. Each
// caller must get its own copy of the record, not a shared pointer, so one
// caller mutating its response cannot corrupt what the other already
// received (the risk this task's work order names explicitly).
func TestPollerProbeAndWait_ReturnsIndependentRecordCopies(t *testing.T) {
	repo := newFakeRepository()
	p := New(repo, 60, logger.Default())
	p.Start(context.Background())
	defer p.Stop()
	p.probe = func(_ context.Context, executor *models.Executor) agentruntime.SSHProbeOutcome {
		return agentruntime.SSHProbeOutcome{Success: true, Host: executor.Config["ssh_host"]}
	}

	executor := sshExecutor("exec-copy")
	result, ran := p.ProbeAndWait(executor)
	if !ran {
		t.Fatalf("ran = false, want true")
	}
	if result.Record == nil {
		t.Fatalf("Record = nil, want the stored record")
	}
	originalHost := result.Record.Host
	result.Record.Host = "mutated-in-caller"

	second, ran := p.ProbeAndWait(executor)
	if !ran {
		t.Fatalf("second call: ran = false, want true")
	}
	if second.Record.Host == "mutated-in-caller" {
		t.Fatalf("second caller observed the first caller's mutation")
	}
	if second.Record.Host != originalHost {
		t.Fatalf("second.Record.Host = %q, want %q", second.Record.Host, originalHost)
	}
}

// @covers AC-EXECUTORS-SSH-REACHABILITY-001.28
//
// The immediate probe must work while the scheduled poller is disabled
// (interval 0) — ProbeAndWait must not depend on the loop ever having run.
func TestPollerProbeAndWait_WorksWhilePollerDisabled(t *testing.T) {
	repo := newFakeRepository()
	p := New(repo, 0, logger.Default())
	p.Start(context.Background())
	defer p.Stop()
	p.probe = func(_ context.Context, executor *models.Executor) agentruntime.SSHProbeOutcome {
		return agentruntime.SSHProbeOutcome{Success: true, Host: executor.Config["ssh_host"]}
	}

	result, ran := p.ProbeAndWait(sshExecutor("exec-disabled"))
	if !ran {
		t.Fatalf("ran = false, want true even though the scheduled poller is disabled")
	}
	if result.Record == nil || result.Record.State != models.ExecutorReachabilityStateReachable {
		t.Fatalf("result = %+v, want a persisted reachable record", result)
	}
}

// @covers AC-EXECUTORS-SSH-REACHABILITY-002.2
//
// A probe that ran but was not stored (the repository refused the write)
// still reports ran=true with Persisted=false, never an error — the route
// this backs answers 200 either way.
func TestPollerProbeAndWait_WriteRefusedReportsPersistedFalse(t *testing.T) {
	repo := newFakeRepository()
	repo.upsertErr = errRepoRefused
	p := New(repo, 60, logger.Default())
	p.Start(context.Background())
	defer p.Stop()
	p.probe = func(_ context.Context, executor *models.Executor) agentruntime.SSHProbeOutcome {
		return agentruntime.SSHProbeOutcome{Success: true, Host: executor.Config["ssh_host"]}
	}

	result, ran := p.ProbeAndWait(sshExecutor("exec-refused"))
	if !ran {
		t.Fatalf("ran = false, want true")
	}
	if result.Persisted {
		t.Fatalf("Persisted = true, want false — the repository write failed")
	}
}

// TestPollerProbeAndWait_RefusedAfterStop mirrors
// TestPollerProbeNow_RefusedAfterStop: once Stop has run, ProbeAndWait must
// not dial at all.
func TestPollerProbeAndWait_RefusedAfterStop(t *testing.T) {
	repo := newFakeRepository()
	p := New(repo, 60, logger.Default())
	p.Start(context.Background())
	p.Stop()

	dialed := false
	p.probe = func(_ context.Context, executor *models.Executor) agentruntime.SSHProbeOutcome {
		dialed = true
		return agentruntime.SSHProbeOutcome{Success: true, Host: executor.Config["ssh_host"]}
	}

	_, ran := p.ProbeAndWait(sshExecutor("exec-stopped"))
	if ran {
		t.Fatalf("ran = true, want false — the poller was stopped")
	}
	if dialed {
		t.Fatalf("probe was dialed after Stop")
	}
}
