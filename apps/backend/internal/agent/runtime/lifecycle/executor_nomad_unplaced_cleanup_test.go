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

// unplacedNomadServer scripts the Nomad calls cleanup makes for a pool job
// that was never placed on a node.
type unplacedNomadServer struct {
	mu          sync.Mutex
	registered  bool
	stopped     bool
	allocations string
	deletes     int
}

func (s *unplacedNomadServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/v1/job/"):
		s.deletes++
		s.stopped = true
		_, _ = w.Write([]byte(`{"EvalID":"e","JobModifyIndex":8}`))
	case strings.HasSuffix(r.URL.Path, "/allocations"):
		alloc := s.allocations
		if alloc == "" {
			alloc = "[]"
		}
		_, _ = w.Write([]byte(alloc))
	case strings.HasPrefix(r.URL.Path, "/v1/job/") && !s.registered:
		http.NotFound(w, r)
	case strings.HasPrefix(r.URL.Path, "/v1/job/"):
		stop := "false"
		if s.stopped {
			stop = "true"
		}
		_, _ = w.Write([]byte(`{"ID":"job-1","Namespace":"default","Region":"global","Stop":` + stop + `,"CreateIndex":5,"ModifyIndex":6,"JobModifyIndex":6}`))
	default:
		http.NotFound(w, r)
	}
}

func unplacedPoolCleanup(t *testing.T, server *unplacedNomadServer, phase string) (*nomadCleanup, *[]nomad.EnvironmentState, *int) {
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
		Platform: nomad.WorkerPlatform, CPUMHz: 100, MemoryMB: 256, Phase: phase, PlacementMode: nomad.ModePool,
		NodePool: "kandev", Placement: nomad.PlacementSpread, MemoryMaxMB: 2048,
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
		req:  req, profile: nomad.ProfileConfig{Mode: nomad.ModePool, Namespace: "default"}, state: state, claimID: "claim-1", client: client,
	}
	return c, &checkpoints, &releases
}

func assertSettledAbsent(t *testing.T, err error, checkpoints []nomad.EnvironmentState, releases int) {
	t.Helper()
	if err != nil {
		t.Fatalf("cleanup = %v, want settled", err)
	}
	if len(checkpoints) == 0 || checkpoints[len(checkpoints)-1].Phase != nomadPhaseAbsent {
		t.Fatalf("final checkpoint is not absent: %+v", checkpoints)
	}
	if releases != 1 {
		t.Fatalf("inventory releases = %d, want 1", releases)
	}
}

func TestNomadCleanupSettlesNeverRegisteredPoolRowAsAbsent(t *testing.T) {
	server := &unplacedNomadServer{}
	c, checkpoints, releases := unplacedPoolCleanup(t, server, nomadPhaseIntent)
	err := c.drainAndSettle(context.Background())
	assertSettledAbsent(t, err, *checkpoints, *releases)
	if server.deletes != 0 {
		t.Fatalf("job stop calls = %d, want 0 for an unregistered job", server.deletes)
	}
}

func TestNomadCleanupStopsRegisteredUnplacedPoolJobBeforeAbsent(t *testing.T) {
	server := &unplacedNomadServer{registered: true}
	c, checkpoints, releases := unplacedPoolCleanup(t, server, nomadPhaseRegistered)
	err := c.drainAndSettle(context.Background())
	assertSettledAbsent(t, err, *checkpoints, *releases)
	if server.deletes != 1 {
		t.Fatalf("job stop calls = %d, want 1", server.deletes)
	}
}

func TestNomadCleanupRetainsUnplacedPoolJobWithLiveAllocation(t *testing.T) {
	server := &unplacedNomadServer{registered: true, allocations: `[{"ID":"a1","JobID":"job-1","Namespace":"default","ClientStatus":"running","DesiredStatus":"stop"}]`}
	c, checkpoints, releases := unplacedPoolCleanup(t, server, nomadPhaseRegistered)
	c.exec.stopConfirmTimeout = 50 * time.Millisecond
	if err := c.drainAndSettle(context.Background()); err == nil {
		t.Fatal("cleanup settled with a live allocation")
	}
	for _, s := range *checkpoints {
		if s.Phase == nomadPhaseAbsent {
			t.Fatal("absence recorded with a live allocation")
		}
	}
	if *releases != 0 {
		t.Fatal("inventory released with a live allocation")
	}
}

func TestNomadCleanupFinishSkipsStagedRemovalForRefusedLaunch(t *testing.T) {
	w, releases := &recordingWorker{}, 0
	c := poolCleanupForFinish(t, w, &releases)
	c.state.RefusedBeforeStage = true
	c.worker = nil
	c.client, _ = nomad.NewClient("http://127.0.0.1:1", "token")
	if err := c.finish(context.Background()); err != nil {
		t.Fatalf("finish = %v, want released without contacting the refused node", err)
	}
	if releases != 1 {
		t.Fatalf("inventory releases = %d, want 1", releases)
	}
}

func TestNomadResumeSettlesUnplacedPoolLaunchRowAfterRestart(t *testing.T) {
	for _, phase := range []string{nomadPhaseIntent, nomadPhaseRegistered} {
		server := &unplacedNomadServer{registered: phase == nomadPhaseRegistered}
		c, checkpoints, releases := unplacedPoolCleanup(t, server, phase)
		if !nomadCleanupResumable(c.state) {
			t.Fatalf("%s: unplaced pool row is not recovery-eligible", phase)
		}
		assertSettledAbsent(t, c.resume(context.Background()), *checkpoints, *releases)
	}
}
