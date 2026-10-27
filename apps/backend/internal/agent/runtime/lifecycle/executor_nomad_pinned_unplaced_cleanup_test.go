package lifecycle

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/nomad"
)

// pinnedUnplacedServer scripts the Nomad calls cleanup makes for a pinned job
// and counts every quarantine-related call (node read, eligibility, variable).
type pinnedUnplacedServer struct {
	mu              sync.Mutex
	registered      bool
	allocations     string
	failAllocations bool
	deletes         int
	quarantineCalls int
}

func (s *pinnedUnplacedServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case strings.HasPrefix(r.URL.Path, "/v1/node/") || strings.HasPrefix(r.URL.Path, "/v1/var"):
		s.quarantineCalls++
		http.Error(w, "unavailable", http.StatusInternalServerError)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/v1/job/"):
		s.deletes++
		_, _ = w.Write([]byte(`{"EvalID":"e","JobModifyIndex":8}`))
	case strings.HasSuffix(r.URL.Path, "/allocations"):
		if s.failAllocations {
			http.Error(w, "unavailable", http.StatusInternalServerError)
			return
		}
		alloc := s.allocations
		if alloc == "" {
			alloc = "[]"
		}
		_, _ = w.Write([]byte(alloc))
	case strings.HasPrefix(r.URL.Path, "/v1/job/") && !s.registered:
		http.NotFound(w, r)
	case strings.HasPrefix(r.URL.Path, "/v1/job/"):
		_, _ = w.Write([]byte(`{"ID":"job-1","Namespace":"default","Region":"global","Stop":true,"CreateIndex":5,"ModifyIndex":6,"JobModifyIndex":6}`))
	default:
		http.NotFound(w, r)
	}
}

func pinnedUnplacedCleanup(t *testing.T, server *pinnedUnplacedServer) (*nomadCleanup, *[]nomad.EnvironmentState, *int) {
	t.Helper()
	ts := httptest.NewServer(server)
	t.Cleanup(ts.Close)
	client, err := nomad.NewClient(ts.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	state := nomad.EnvironmentState{
		Version: 1, EnvironmentID: "env-1", Generation: 1, OperationID: "op-1", InstallationID: "i-1", ExecutionID: "x-1",
		InputDigest: strings.Repeat("a", 64), JobID: "job-1", Namespace: "default", Region: "global", Datacenter: "dc1",
		NodeID: "node-1", Platform: nomad.WorkerPlatform, CPUMHz: 100, MemoryMB: 256, Phase: nomadPhaseRegistered,
		PinnedNodePool: "kandev", SSHHost: "worker.example", SSHUser: "kandev", SSHPort: 22,
		SSHHostFingerprint: "SHA256:abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJK",
		WorkspaceRoot:      "/var/lib/kandev/workspaces", WorkspacePath: "/var/lib/kandev/workspaces/environments/env-1/g1",
		AllocationRoot: "/var/lib/kandev/allocations",
	}
	var checkpoints []nomad.EnvironmentState
	releases := 0
	req := &ExecutorCreateRequest{
		Metadata: map[string]interface{}{},
		CheckpointRuntimeInventory: func(_ context.Context, metadata map[string]interface{}) error {
			s, err := nomad.EnvironmentStateFromMetadata(metadata)
			if err != nil {
				return err
			}
			checkpoints = append(checkpoints, s)
			return nil
		},
		ReleaseRuntimeInventory: func(context.Context) error { releases++; return nil },
	}
	c := &nomadCleanup{
		exec: &NomadExecutor{logger: newTestLogger(), sessions: map[string]*nomadAgentctlConnection{}, stopConfirmTimeout: 2 * time.Second},
		req:  req, profile: nomad.ProfileConfig{Mode: nomad.ModeLegacyPinned, Namespace: "default", NodeID: "node-1"},
		state: state, claimID: "claim-1", client: client, worker: &recordingWorker{},
	}
	return c, &checkpoints, &releases
}

func TestNomadCleanupSettlesPinnedNeverPlacedPurgedJobAsAbsentWithoutQuarantine(t *testing.T) {
	server := &pinnedUnplacedServer{}
	c, checkpoints, releases := pinnedUnplacedCleanup(t, server)
	assertSettledAbsent(t, c.drainAndSettle(context.Background()), *checkpoints, *releases)
	if server.quarantineCalls != 0 || server.deletes != 0 {
		t.Fatalf("quarantine calls=%d stops=%d, want none for a purged never-placed job", server.quarantineCalls, server.deletes)
	}
	for _, s := range *checkpoints {
		if s.QuarantineState != "" {
			t.Fatalf("checkpoint recorded quarantine state %q", s.QuarantineState)
		}
	}
}

func TestNomadCleanupStopsPinnedNeverPlacedPendingJobBeforeAbsent(t *testing.T) {
	server := &pinnedUnplacedServer{registered: true}
	c, checkpoints, releases := pinnedUnplacedCleanup(t, server)
	assertSettledAbsent(t, c.drainAndSettle(context.Background()), *checkpoints, *releases)
	if server.deletes != 1 || server.quarantineCalls != 0 {
		t.Fatalf("stops=%d quarantine calls=%d, want 1 and 0", server.deletes, server.quarantineCalls)
	}
}

func TestNomadCleanupQuarantinesPinnedRowThatEverHadAnAllocation(t *testing.T) {
	server := &pinnedUnplacedServer{allocations: `[{"ID":"a1","JobID":"job-1","Namespace":"default","ClientStatus":"complete","DesiredStatus":"stop"}]`}
	c, checkpoints, releases := pinnedUnplacedCleanup(t, server)
	if err := c.drainAndSettle(context.Background()); err == nil {
		t.Fatal("cleanup settled a pinned row that had an allocation")
	}
	assertPinnedRowRetained(t, server, *checkpoints, *releases)
}

func TestNomadCleanupFailsClosedWhenPinnedAllocationHistoryUnreadable(t *testing.T) {
	server := &pinnedUnplacedServer{failAllocations: true}
	c, checkpoints, releases := pinnedUnplacedCleanup(t, server)
	if err := c.drainAndSettle(context.Background()); err == nil {
		t.Fatal("cleanup settled a pinned row with unreadable allocation history")
	}
	assertPinnedRowRetained(t, server, *checkpoints, *releases)
}

func assertPinnedRowRetained(t *testing.T, server *pinnedUnplacedServer, checkpoints []nomad.EnvironmentState, releases int) {
	t.Helper()
	if server.quarantineCalls == 0 {
		t.Fatal("no quarantine attempt for a pinned row whose placement history is not provably empty")
	}
	for _, s := range checkpoints {
		if s.Phase == nomadPhaseAbsent {
			t.Fatal("absence recorded without proof that no runtime existed")
		}
	}
	if releases != 0 {
		t.Fatal("inventory released for a retained pinned row")
	}
}

func TestNomadResumeSettlesPinnedNeverPlacedRowWithoutQuarantine(t *testing.T) {
	for name, prepare := range map[string]func(*nomadCleanup){
		"cleanup pending without receipt": func(c *nomadCleanup) { c.state.Phase = nomadPhaseCleanupPending },
		"cleanup pending with empty receipt": func(c *nomadCleanup) {
			c.state.Phase = nomadPhaseCleanupPending
			if err := c.recordEmptyDrainReceipt(context.Background()); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			server := &pinnedUnplacedServer{}
			c, checkpoints, releases := pinnedUnplacedCleanup(t, server)
			prepare(c)
			*checkpoints = nil
			assertSettledAbsent(t, c.resume(context.Background()), *checkpoints, *releases)
			if server.quarantineCalls != 0 {
				t.Fatalf("quarantine calls = %d, want 0", server.quarantineCalls)
			}
		})
	}
}

func TestNomadResumeKeepsPinnedRowWithAllocationHistoryQuarantined(t *testing.T) {
	server := &pinnedUnplacedServer{allocations: `[{"ID":"a1","JobID":"job-1","Namespace":"default","ClientStatus":"complete","DesiredStatus":"stop"}]`}
	c, checkpoints, releases := pinnedUnplacedCleanup(t, server)
	c.state.Phase = nomadPhaseCleanupPending
	if err := c.resume(context.Background()); err == nil {
		t.Fatal("resume settled a pinned row that had an allocation")
	}
	assertPinnedRowRetained(t, server, *checkpoints, *releases)
}

// A purged job lists no allocations once they are garbage collected, so an
// empty history cannot disprove an allocation the state already recorded.
func TestNomadCleanupQuarantinesPinnedRowWithRecordedAllocationAndGCdHistory(t *testing.T) {
	for via, wantReason := range map[string]string{
		"drainAndSettle": nomadCleanupReasonIdentityUnrecorded,
		"resume":         nomadCleanupReasonInterrupted,
	} {
		t.Run(via, func(t *testing.T) {
			server := &pinnedUnplacedServer{}
			c, checkpoints, releases := pinnedUnplacedCleanup(t, server)
			c.state.Phase = nomadPhaseAllocated
			c.state.AllocationID, c.state.AllocationCreateIndex = "alloc-1", 7
			var err error
			if via == "resume" {
				c.state.Phase = nomadPhaseCleanupPending
				err = c.resume(context.Background())
			} else {
				err = c.drainAndSettle(context.Background())
			}
			if err == nil || !strings.Contains(err.Error(), wantReason) {
				t.Fatalf("err = %v, want retained with %s", err, wantReason)
			}
			assertPinnedRowRetained(t, server, *checkpoints, *releases)
		})
	}
}
