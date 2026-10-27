package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/nomad"
)

const poolTestHostKey = "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

func poolTestProfile() nomad.ProfileConfig {
	return nomad.ProfileConfig{
		Mode: nomad.ModePool, Namespace: "default", Region: "global", Datacenter: "dc1", Platform: "darwin/arm64",
		NodePool: "kandev", Placement: nomad.PlacementSpread, CPUMHz: 100, MemoryMB: 256, MemoryMaxMB: 2048,
		SSHUser: "worker",
	}
}

func poolTestRequest() *ExecutorCreateRequest {
	return &ExecutorCreateRequest{InstanceID: "exec-1", TaskID: "task-1", SessionID: "sess-1", TaskEnvironmentID: "env-1", WorkspaceID: "ws-1", WorkspacePath: "/src"}
}

func poolTestIdentity(t *testing.T) nomadLaunchIdentity {
	t.Helper()
	profile := poolTestProfile()
	req := poolTestRequest()
	digest, err := nomadLaunchDigest(profile, req, req.WorkspacePath, nomadDigestWorkspacePath(profile, "env-1", 1, ""))
	if err != nil {
		t.Fatal(err)
	}
	jobID, err := nomad.StableJobID(nomad.JobIdentity{EnvironmentID: "env-1", Generation: 1, OperationID: "exec-1", InstallationID: "inst-1"})
	if err != nil {
		t.Fatal(err)
	}
	return nomadLaunchIdentity{profile: profile, profileID: "inst-1", generation: 1, inputDigest: digest, jobID: jobID}
}

func registeredPoolTestState(t *testing.T) nomad.EnvironmentState {
	t.Helper()
	state := newNomadPoolIntentState(poolTestRequest(), poolTestIdentity(t))
	state.ArtifactSHA256 = strings.Repeat("c", 64)
	job, err := nomad.NativeJobFromState(state)
	if err != nil {
		t.Fatal(err)
	}
	if state.NativeJobSpecSHA256, err = nomad.NativeJobSpecDigest(job); err != nil {
		t.Fatal(err)
	}
	state.NativeJobPolicyVersion = nomad.PoolJobPolicyVersion
	state.Phase = nomadPhaseRegistered
	return state
}

func tokenDigest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func localNode(stateRoot, identity string) *nomad.NodeRecord {
	node := &nomad.NodeRecord{
		ID: "node-local", NodePool: "kandev", SchedulingEligibility: "eligible", Status: "ready",
		Attributes: map[string]string{"kernel.name": "darwin", "cpu.arch": "arm64"},
		Meta: map[string]string{
			nomad.MetaAttach: nomad.AttachLocal, nomad.MetaStateRoot: stateRoot, nomad.MetaHostIdentity: identity,
		},
	}
	node.ReservedResources.CPU.CPUShares = 500
	node.ReservedResources.Memory.MemoryMB = 1024
	return node
}

func poolTestAllocation(jobID, nodeID string) nomad.AllocationRecord {
	return nomad.AllocationRecord{
		ID: "alloc-1", NodeID: nodeID, JobID: jobID, Namespace: "default", DesiredStatus: "run",
		ClientStatus: "running", CreateIndex: 12,
		TaskStates: map[string]nomad.AllocationTaskState{"agentctl": {State: "running"}},
		AllocatedResources: &nomad.AllocationResources{Shared: nomad.AllocationSharedResources{Ports: []nomad.AllocationPort{
			{Label: "agentctl_control", Value: 22341}, {Label: "agentctl_instance", Value: 22342},
		}}},
	}
}

func placeOn(t *testing.T, node *nomad.NodeRecord) nomad.EnvironmentState {
	t.Helper()
	state := registeredPoolTestState(t)
	placed, err := nomad.BindPoolAllocation(state, []nomad.AllocationRecord{poolTestAllocation(state.JobID, node.ID)}, node)
	if err != nil {
		t.Fatal(err)
	}
	return placed
}

func TestPoolIntentStateIsValidAndUnplaced(t *testing.T) {
	state := newNomadPoolIntentState(poolTestRequest(), poolTestIdentity(t))
	if err := state.Validate(); err != nil {
		t.Fatal(err)
	}
	if state.NodeID != "" || state.WorkspacePath != "" || state.PlacementMode != nomad.ModePool {
		t.Fatalf("unexpected intent state %+v", state)
	}
	if !nomadPoolSnapshotMatches(state, poolTestProfile()) {
		t.Fatal("snapshot must match its profile")
	}
	changed := poolTestProfile()
	changed.MemoryMaxMB = 4096
	if nomadPoolSnapshotMatches(state, changed) {
		t.Fatal("a changed profile must not match the frozen snapshot")
	}
}

func TestPoolLaunchDigestIsIndependentOfNode(t *testing.T) {
	a, b := poolTestIdentity(t), poolTestIdentity(t)
	if a.inputDigest != b.inputDigest || a.workspacePath != "" {
		t.Fatal("pool launch digest must be stable and carry no node workspace")
	}
}

func TestVerifyLocalNode(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "token")
	if err := os.WriteFile(tokenPath, []byte("host-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := &NomadExecutor{hostIdentityPath: tokenPath, kandevHomeDir: filepath.Join(dir, "home")}
	stateRoot := filepath.Join(dir, "state")
	good := localNode(stateRoot, tokenDigest("host-secret"))
	contract, err := nomad.ParseNodeContract(good)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.verifyLocalNode(contract, good); err != nil {
		t.Fatalf("matching local node refused: %v", err)
	}

	cases := map[string]func() (nomad.NodeContract, *nomad.NodeRecord){
		"fake local": func() (nomad.NodeContract, *nomad.NodeRecord) {
			n := localNode(stateRoot, tokenDigest("someone-else"))
			c, _ := nomad.ParseNodeContract(n)
			return c, n
		},
		"no headroom": func() (nomad.NodeContract, *nomad.NodeRecord) {
			n := localNode(stateRoot, tokenDigest("host-secret"))
			n.ReservedResources.Memory.MemoryMB = 0
			c, _ := nomad.ParseNodeContract(n)
			return c, n
		},
		"state root inside home": func() (nomad.NodeContract, *nomad.NodeRecord) {
			n := localNode(filepath.Join(dir, "home", "data", "work"), tokenDigest("host-secret"))
			c, _ := nomad.ParseNodeContract(n)
			return c, n
		},
	}
	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			c, n := build()
			if err := r.verifyLocalNode(c, n); !errors.Is(err, ErrNomadLocalNodeRefused) {
				t.Fatalf("error = %v, want local node refusal", err)
			}
		})
	}
	missing := &NomadExecutor{hostIdentityPath: filepath.Join(dir, "absent"), kandevHomeDir: filepath.Join(dir, "home")}
	if err := missing.verifyLocalNode(contract, good); !errors.Is(err, ErrNomadLocalNodeRefused) {
		t.Fatalf("missing token error = %v", err)
	}
}

func nomadNodeServer(t *testing.T, node *nomad.NodeRecord) *nomad.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/node/"+node.ID {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(node)
	}))
	t.Cleanup(srv.Close)
	client, err := nomad.NewClient(srv.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestVerifyPlacedPoolNodeFailsClosedOnDriftAndFakeLocal(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "token")
	_ = os.WriteFile(tokenPath, []byte("host-secret"), 0o600)
	r := &NomadExecutor{hostIdentityPath: tokenPath, kandevHomeDir: filepath.Join(dir, "home")}
	node := localNode(filepath.Join(dir, "state"), tokenDigest("host-secret"))
	state := placeOn(t, node)

	if _, _, err := r.verifyPlacedPoolNode(context.Background(), nomadNodeServer(t, node), state); err != nil {
		t.Fatalf("unchanged node refused: %v", err)
	}
	drifted := localNode(filepath.Join(dir, "other-state"), tokenDigest("host-secret"))
	if _, _, err := r.verifyPlacedPoolNode(context.Background(), nomadNodeServer(t, drifted), state); err == nil {
		t.Fatal("changed node metadata must fail closed")
	}
	fake := localNode(filepath.Join(dir, "state"), tokenDigest("not-this-host"))
	fakeState := placeOn(t, fake)
	if _, _, err := r.verifyPlacedPoolNode(context.Background(), nomadNodeServer(t, fake), fakeState); !errors.Is(err, ErrNomadLocalNodeRefused) {
		t.Fatalf("a node claiming to be local must be refused, got %v", err)
	}
}

func TestSSHNodeMismatchedHostKeyChangesContract(t *testing.T) {
	node := &nomad.NodeRecord{
		ID: "node-ssh", NodePool: "kandev",
		Attributes: map[string]string{"kernel.name": "darwin", "cpu.arch": "arm64"},
		Meta: map[string]string{
			nomad.MetaAttach: nomad.AttachSSH, nomad.MetaStateRoot: "/var/kandev", nomad.MetaSSHHost: "worker.example.test",
			nomad.MetaSSHPort: "22", nomad.MetaSSHHostKeySHA256: poolTestHostKey,
		},
	}
	state := placeOn(t, node)
	r := &NomadExecutor{}
	if _, _, err := r.verifyPlacedPoolNode(context.Background(), nomadNodeServer(t, node), state); err != nil {
		t.Fatal(err)
	}
	swapped := *node
	swapped.Meta = map[string]string{}
	for k, v := range node.Meta {
		swapped.Meta[k] = v
	}
	swapped.Meta[nomad.MetaSSHHostKeySHA256] = "SHA256:BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"
	if _, _, err := r.verifyPlacedPoolNode(context.Background(), nomadNodeServer(t, &swapped), state); err == nil {
		t.Fatal("a changed host key must fail closed")
	}
}

func TestPoolJobMatchesRegisteredState(t *testing.T) {
	state := registeredPoolTestState(t)
	job, err := nomad.NativeJobFromState(state)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(job)
	if err != nil {
		t.Fatal(err)
	}
	record := &nomad.JobRecord{ID: job.ID, Namespace: job.Namespace, Region: job.Region, Meta: job.Meta, Raw: raw, ModifyIndex: 5}
	if !nomadJobNodeMetaMatches(record, state) || !jobMetaMatchesState(record, state) {
		t.Fatal("pool job metadata must match its state")
	}
	record.Meta = map[string]string{"kandev.node_id": "node-1"}
	if nomadJobNodeMetaMatches(record, state) {
		t.Fatal("a pool job that names a node must not match")
	}
}

func TestStagePoolLaunchFilesWritesReadyLast(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "token")
	_ = os.WriteFile(tokenPath, []byte("host-secret"), 0o600)
	node := localNode(filepath.Join(dir, "state"), tokenDigest("host-secret"))
	artifact := []byte("agentctl-binary")
	var state nomad.EnvironmentState
	// Re-derive the placed state around the real artifact so staging verifies its digest.
	registered := registeredPoolTestState(t)
	sum := sha256.Sum256(artifact)
	registered.ArtifactSHA256 = hex.EncodeToString(sum[:])
	registered.NativeJobSpecSHA256, registered.NativeJobPolicyVersion = "", 0
	job, err := nomad.NativeJobFromState(registered)
	if err != nil {
		t.Fatal(err)
	}
	registered.NativeJobSpecSHA256, _ = nomad.NativeJobSpecDigest(job)
	registered.NativeJobPolicyVersion = nomad.PoolJobPolicyVersion
	alloc := poolTestAllocation(registered.JobID, node.ID)
	state, err = nomad.BindPoolAllocation(registered, []nomad.AllocationRecord{alloc}, node)
	if err != nil {
		t.Fatal(err)
	}
	effective := poolTestProfile().EffectiveProfile(state)
	if err := stagePoolLaunchFiles(context.Background(), nomadLocalTransport{}, poolTestRequest(), effective, state, artifact, "nonce-1"); err != nil {
		t.Fatal(err)
	}
	paths, err := nomad.PoolPathsFor(state.StateRoot, state.JobID, state.EnvironmentID, state.Generation, state.ArtifactSHA256)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{paths.ArtifactPath, paths.BootstrapPath, paths.EnvironmentEnv, paths.ReadyPath} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("missing staged file %s: %v", p, err)
		}
	}
	ready, _ := os.Stat(paths.ReadyPath)
	boot, _ := os.Stat(paths.BootstrapPath)
	if ready.ModTime().Before(boot.ModTime()) {
		t.Fatal("the ready marker must be written after the bootstrap")
	}
}

func TestLocalTransportUploadRunAndForward(t *testing.T) {
	dir := t.TempDir()
	tr := nomadLocalTransport{}
	path := filepath.Join(dir, "f")
	if err := tr.Upload(context.Background(), path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("upload mode: %v %v", info, err)
	}
	out, _, err := tr.Run(context.Background(), "cat "+shellQuote(path))
	if err != nil || out != "x" {
		t.Fatalf("run = %q %v", out, err)
	}
	if fwd, err := tr.Forward(2222, nil); err != nil || fwd.LocalPort() != 2222 {
		t.Fatalf("forward = %v %v", fwd, err)
	}
	if _, err := tr.Forward(0, nil); err == nil {
		t.Fatal("an invalid port must be rejected")
	}
}

func TestPathWithin(t *testing.T) {
	if !pathWithin("/a/b", "/a/b/c") || !pathWithin("/a/b", "/a/b") || pathWithin("/a/b", "/a/bc") || pathWithin("/a/b", "/a") {
		t.Fatal("pathWithin boundary cases")
	}
}

// poolBindServer serves the Nomad reads of an unplaced pool bind: the job, its
// allocation (with the embedded job) and the node the allocation landed on.
func poolBindServer(t *testing.T, state nomad.EnvironmentState, node *nomad.NodeRecord, embeddedJobMutate func(map[string]any)) *nomad.Client {
	t.Helper()
	job, err := nomad.NativeJobFromState(state)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(job)
	var jobMap map[string]any
	_ = json.Unmarshal(encoded, &jobMap)
	jobMap["CreateIndex"], jobMap["ModifyIndex"], jobMap["JobModifyIndex"] = 3, 4, 4
	embedded := map[string]any{}
	for k, v := range jobMap {
		embedded[k] = v
	}
	if embeddedJobMutate != nil {
		embeddedJobMutate(embedded)
	}
	embeddedRaw, _ := json.Marshal(embedded)
	alloc := poolTestAllocation(state.JobID, node.ID)
	alloc.Job = embeddedRaw
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/job/" + state.JobID:
			_ = json.NewEncoder(w).Encode(jobMap)
		case "/v1/allocation/alloc-1":
			_ = json.NewEncoder(w).Encode(alloc)
		case "/v1/node/" + node.ID:
			_ = json.NewEncoder(w).Encode(node)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	client, err := nomad.NewClient(srv.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestBindUnplacedPoolAllocationRecordsPlacedNode(t *testing.T) {
	state := registeredPoolTestState(t)
	state.JobCreateIndex, state.JobDefinitionIndex = 3, 4
	node := localNode(t.TempDir(), tokenDigest("host-secret"))
	client := poolBindServer(t, state, node, nil)
	stub := poolTestAllocation(state.JobID, node.ID)

	placed, err := bindUnplacedPoolAllocation(context.Background(), client, "default", state, stub)
	if err != nil {
		t.Fatal(err)
	}
	if placed.NodeID != node.ID || placed.AllocationID != "alloc-1" || placed.Phase != nomadPhaseAllocated ||
		placed.NodeAttach != nomad.AttachLocal || placed.WorkspacePath == "" || placed.ControlPort != 22341 {
		t.Fatalf("unexpected placed state: %+v", placed)
	}
}

func TestBindUnplacedPoolAllocationRefusals(t *testing.T) {
	state := registeredPoolTestState(t)
	state.JobCreateIndex, state.JobDefinitionIndex = 3, 4
	node := localNode(t.TempDir(), tokenDigest("host-secret"))

	wrongPool := *node
	wrongPool.NodePool = "other"
	if _, err := bindUnplacedPoolAllocation(context.Background(), poolBindServer(t, state, &wrongPool, nil), "default", state, poolTestAllocation(state.JobID, node.ID)); err == nil {
		t.Fatal("a node outside the profile node pool must be refused")
	}
	unprepared := *node
	unprepared.Meta = map[string]string{}
	if _, err := bindUnplacedPoolAllocation(context.Background(), poolBindServer(t, state, &unprepared, nil), "default", state, poolTestAllocation(state.JobID, node.ID)); err == nil {
		t.Fatal("a node without the kandev contract must be refused")
	}
	tampered := poolBindServer(t, state, node, func(job map[string]any) { job["Priority"] = 99 })
	if _, err := bindUnplacedPoolAllocation(context.Background(), tampered, "default", state, poolTestAllocation(state.JobID, node.ID)); err == nil {
		t.Fatal("an allocation whose embedded job differs from the immutable job must be refused")
	}
	foreign := poolTestAllocation("another-job", node.ID)
	if _, err := bindUnplacedPoolAllocation(context.Background(), poolBindServer(t, state, node, nil), "default", state, foreign); err == nil {
		t.Fatal("an allocation of another job must be refused")
	}
}
