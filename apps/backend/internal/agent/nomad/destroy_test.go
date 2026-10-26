package nomad

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
)

func destroyState() EnvironmentState {
	state := validTestState()
	state.Phase = "ready"
	state.AllocationID = "alloc-1"
	state.AllocationCreateIndex = 7
	return state
}

func destroyJob(state EnvironmentState) *JobRecord {
	return &JobRecord{ID: state.JobID, Namespace: state.Namespace, Spec: Job{Stop: true}}
}

func destroyAlloc(state EnvironmentState) AllocationRecord {
	return AllocationRecord{ID: state.AllocationID, NodeID: state.NodeID, JobID: state.JobID, Namespace: state.Namespace, ClientStatus: "complete", CreateIndex: 7}
}

func readyNode(state EnvironmentState) *NodeRecord {
	return &NodeRecord{ID: state.NodeID, Status: "ready"}
}

func TestNomadDestroyIdentityRejectsForeignAllocation(t *testing.T) {
	state := destroyState()
	if err := ConfirmStopped(state, destroyJob(state), []AllocationRecord{destroyAlloc(state)}, readyNode(state)); err != nil {
		t.Fatalf("exact stopped evidence rejected: %v", err)
	}
	mutations := map[string]func(*AllocationRecord, *JobRecord, *NodeRecord, *[]AllocationRecord){
		"foreign allocation": func(a *AllocationRecord, _ *JobRecord, _ *NodeRecord, _ *[]AllocationRecord) { a.ID = "alloc-x" },
		"different node":     func(a *AllocationRecord, _ *JobRecord, _ *NodeRecord, _ *[]AllocationRecord) { a.NodeID = "node-other" },
		"running": func(a *AllocationRecord, _ *JobRecord, _ *NodeRecord, _ *[]AllocationRecord) {
			a.ClientStatus = "running"
		},
		"unknown status": func(a *AllocationRecord, _ *JobRecord, _ *NodeRecord, _ *[]AllocationRecord) {
			a.ClientStatus = "unknown"
		},
		"job not stopped": func(_ *AllocationRecord, j *JobRecord, _ *NodeRecord, _ *[]AllocationRecord) { j.Spec.Stop = false },
		"node down":       func(_ *AllocationRecord, _ *JobRecord, n *NodeRecord, _ *[]AllocationRecord) { n.Status = "down" },
		"successor": func(a *AllocationRecord, _ *JobRecord, _ *NodeRecord, l *[]AllocationRecord) {
			successor := *a
			successor.ID = "alloc-2"
			*l = append(*l, successor)
		},
	}
	for name, mutate := range mutations {
		alloc, job, node := destroyAlloc(state), destroyJob(state), readyNode(state)
		list := []AllocationRecord{alloc}
		mutate(&alloc, job, node, &list)
		if len(list) == 1 {
			list[0] = alloc
		}
		if err := ConfirmStopped(state, job, list, node); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	if err := ConfirmStopped(state, nil, []AllocationRecord{destroyAlloc(state)}, readyNode(state)); err == nil {
		t.Fatal("missing job accepted")
	}
}

func TestNomadDestroyIdentityStopJobNeverPurges(t *testing.T) {
	var method, purge, global, namespace, token string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, purge, global = r.Method, r.URL.Query().Get("purge"), r.URL.Query().Get("global")
		namespace, token = r.URL.Query().Get("namespace"), r.Header.Get("X-Nomad-Token")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "fixture-token")
	if err != nil {
		t.Fatal(err)
	}
	if err := client.StopJob(context.Background(), "default", "job-1"); err != nil {
		t.Fatal(err)
	}
	if method != http.MethodDelete || purge != "false" || global != "false" || namespace != "default" || token != "fixture-token" {
		t.Fatalf("unexpected stop request: %s purge=%q global=%q ns=%q", method, purge, global, namespace)
	}
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("fixture-token leaked"))
	}))
	defer failing.Close()
	bad, _ := NewClient(failing.URL, "fixture-token")
	err = bad.StopJob(context.Background(), "default", "job-1")
	if err == nil || strings.Contains(err.Error(), "fixture-token") {
		t.Fatalf("stop error = %v", err)
	}
	if client.StopJob(context.Background(), "", "job-1") == nil {
		t.Fatal("empty namespace accepted")
	}
}

func TestNomadCleanupEvidenceDigestStableAcrossPhase(t *testing.T) {
	state := destroyState()
	receipt := types.RuntimeDrainReceipt{RequestID: "r1", ServerIdentity: "srv", Sealed: true, Drained: true, Scope: "managed", Version: "1", CompletedAt: time.Unix(100, 5)}
	base := ReceiptDigest(state, receipt, "claim-1")
	if !digestPattern.MatchString(base) {
		t.Fatalf("digest %q is not sha256 hex", base)
	}
	moved := state
	moved.Phase, moved.Revision = "cleanup_pending", 9
	moved.DrainReceiptDigest, moved.DrainReceiptGeneration = base, state.Generation
	if ReceiptDigest(moved, receipt, "claim-1") != base {
		t.Fatal("digest changed across phase, revision or stored receipt")
	}
	if ReceiptDigest(state, receipt, "claim-2") == base {
		t.Fatal("digest ignores claim")
	}
	other := receipt
	other.Drained = false
	if ReceiptDigest(state, other, "claim-1") == base {
		t.Fatal("digest ignores receipt content")
	}
	foreign := state
	foreign.AllocationID = "alloc-2"
	if ReceiptDigest(foreign, receipt, "claim-1") == base {
		t.Fatal("digest ignores allocation identity")
	}
}

func TestNomadCleanupEvidenceEmptyHistoryIsNotAbsence(t *testing.T) {
	state := destroyState()
	if err := ConfirmStopped(state, destroyJob(state), nil, readyNode(state)); err == nil {
		t.Fatal("empty allocation history accepted as absence")
	}
	if err := ConfirmStopped(state, destroyJob(state), []AllocationRecord{}, readyNode(state)); err == nil {
		t.Fatal("zero-length allocation history accepted as absence")
	}
}

func TestNomadCleanupEvidenceFenceCommandComparesOwner(t *testing.T) {
	base := t.TempDir()
	state := destroyState()
	state.AllocationRoot = filepath.Join(base, "root", "allocations")
	profile := ProfileConfig{NodeID: state.NodeID, AllocationRoot: state.AllocationRoot}
	command, err := FenceReleaseCommand(profile, state)
	if err != nil {
		t.Fatal(err)
	}
	fence := filepath.Join(base, "worker-state", state.NodeID, "active-environment")
	run := func() error { return exec.Command("sh", "-c", command).Run() }
	write := func(owner string) {
		_ = os.RemoveAll(fence)
		if err := os.MkdirAll(fence, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(fence, "owner"), []byte(owner+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("someone-else\nalloc-9")
	if run() == nil {
		t.Fatal("foreign owner released")
	}
	if _, err := os.Stat(filepath.Join(fence, "owner")); err != nil {
		t.Fatalf("foreign owner marker removed: %v", err)
	}
	write(fenceOwnerToken(state))
	if err := run(); err != nil {
		t.Fatalf("matching owner not released: %v", err)
	}
	if _, err := os.Stat(fence); !os.IsNotExist(err) {
		t.Fatalf("fence still present: %v", err)
	}
	if err := run(); err != nil {
		t.Fatalf("already released fence is not idempotent: %v", err)
	}
	mismatched := state
	mismatched.NodeID = "node-other"
	if _, err := FenceReleaseCommand(profile, mismatched); err == nil {
		t.Fatal("mismatched node accepted")
	}
}
