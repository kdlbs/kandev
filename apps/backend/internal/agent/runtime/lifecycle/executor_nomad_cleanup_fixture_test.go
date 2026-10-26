package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/nomad"
)

// nomadCleanupFixture scripts the drain, job-stop and worker commands of a
// cleanup run on top of nomadCreateFixture.
type nomadCleanupFixture struct {
	mu               sync.Mutex
	drainFail        bool
	drainIdentity    string
	stopFail         bool
	stopped          bool
	allocationStatus string
	failFenceRelease bool
	failBootstrapRm  bool
	receiptAtStop    string
	drainCalls       int
	stopCalls        int
	releaseCalls     int
	ssh              []string
	failReleaseHook  bool
}

func (c *nomadCleanupFixture) serveDrain(w http.ResponseWriter, r *http.Request, f *nomadCreateFixture) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var req struct {
		RequestID              string `json:"request_id"`
		ExpectedServerIdentity string `json:"expected_server_identity"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	c.drainCalls++
	f.fake.record("drain")
	if c.drainFail {
		http.Error(w, "drain failed", http.StatusInternalServerError)
		return
	}
	identity := req.ExpectedServerIdentity
	if c.drainIdentity != "" {
		identity = c.drainIdentity
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"request_id": req.RequestID, "server_identity": identity, "sealed": true, "drained": true,
		"scope": "managed_process_groups", "version": "runtime-seal-drain.v1",
		"completed_at": time.Date(2026, 10, 24, 1, 2, 3, 0, time.UTC),
	})
}

func (c *nomadCleanupFixture) serveNomad(w http.ResponseWriter, r *http.Request, f *nomadCreateFixture) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch {
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/v1/job/"):
		c.stopCalls++
		f.fake.record("job:stop")
		c.receiptAtStop = ""
		if n := len(f.checkpoints); n > 0 {
			c.receiptAtStop = f.checkpoints[n-1].DrainReceiptDigest
		}
		if c.stopFail {
			http.Error(w, "stop failed", http.StatusInternalServerError)
			return true
		}
		c.stopped = true
		_, _ = io.WriteString(w, `{"EvalID":"eval-2","JobModifyIndex":8}`)
		return true
	case c.stopped && r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/allocations"):
		status := c.allocationStatus
		if status == "" {
			status = "complete"
		}
		_, _ = io.WriteString(w, `[{"ID":"alloc-1","NodeID":"node-1","JobID":"`+f.job.Job.ID+`","Namespace":"default","DesiredStatus":"stop","ClientStatus":"`+status+`","CreateIndex":9}]`)
		return true
	case c.stopped && r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/job/"):
		response := f.job.Job
		response.Stop = true
		response.CreateIndex, response.ModifyIndex, response.JobModifyIndex = 5, 10, 8
		_ = json.NewEncoder(w).Encode(response)
		return true
	}
	return false
}

func (c *nomadCleanupFixture) serveSSH(command string, f *nomadCreateFixture) sshExecResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ssh = append(c.ssh, command)
	if strings.HasPrefix(command, "rm -f -- ") {
		f.fake.record("ssh:bootstrap-rm")
		if c.failBootstrapRm {
			return sshFail("rm failed")
		}
		return sshOK
	}
	f.fake.record("ssh:fence-release")
	if c.failFenceRelease {
		return sshFail("fence release failed")
	}
	return sshOK
}

// prepareNomadCleanupFixture leaves the fixture with a ready, connected
// environment that records the runtime identity cleanup needs.
func prepareNomadCleanupFixture(t *testing.T) *nomadCreateFixture {
	t.Helper()
	fixture := newNomadCreateFixture(t, false)
	prepareRetainedNomadFixture(t, fixture, "ready")
	state, err := nomad.EnvironmentStateFromMetadata(fixture.request.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	state.ServerIdentity, state.AllocationID, state.AllocationCreateIndex = "fixture-agentctl", "alloc-1", 9
	state.ControlPort, state.InstancePort = 41230, 41231
	if err := nomad.PutEnvironmentState(fixture.request.Metadata, state); err != nil {
		t.Fatal(err)
	}
	fixture.request.ReleaseRuntimeInventory = func(context.Context) error {
		fixture.cleanup.mu.Lock()
		defer fixture.cleanup.mu.Unlock()
		fixture.cleanup.releaseCalls++
		fixture.fake.record("inventory:release")
		if fixture.cleanup.failReleaseHook {
			return errors.New("release failed")
		}
		return nil
	}
	return fixture
}

func (f *nomadCreateFixture) lastCheckpoint(t *testing.T) nomad.EnvironmentState {
	t.Helper()
	if len(f.checkpoints) == 0 {
		t.Fatal("no checkpoint was recorded")
	}
	return f.checkpoints[len(f.checkpoints)-1]
}

func indexOfCall(calls []string, want string) int {
	for i, call := range calls {
		if call == want {
			return i
		}
	}
	return -1
}
