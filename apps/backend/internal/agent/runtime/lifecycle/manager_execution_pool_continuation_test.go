package lifecycle

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/nomad"
)

func TestValidateRetainedNomadLaunchInputs_PoolProfileMatchesLaunchDigest(t *testing.T) {
	metadata := map[string]interface{}{
		"address": "https://nomad.example.test:4646", "namespace": "default", "region": "global",
		"datacenter": "dc1", "platform": "darwin/arm64", "nomad_token": "$kandev-secret:secret-nomad-token",
		"node_pool": "kandev", "ssh_user": "worker",
	}
	profile, err := nomadProfileFromMetadata(metadata)
	if err != nil || profile.Mode != nomad.ModePool {
		t.Fatalf("pool profile: %+v %v", profile, err)
	}
	req := poolTestRequest()
	digest, err := nomadLaunchDigest(profile, req, req.WorkspacePath, nomadDigestWorkspacePath(profile, "env-1", 1, ""))
	if err != nil {
		t.Fatal(err)
	}
	jobID, err := nomad.StableJobID(nomad.JobIdentity{EnvironmentID: "env-1", Generation: 1, OperationID: "exec-1", InstallationID: "inst-1"})
	if err != nil {
		t.Fatal(err)
	}
	id := nomadLaunchIdentity{profile: profile, profileID: "inst-1", generation: 1, inputDigest: digest, jobID: jobID}
	state := newNomadPoolIntentState(req, id)
	state.Phase = nomadPhaseRegistered
	req.Metadata = metadata
	if err := nomad.PutEnvironmentState(req.Metadata, state); err != nil {
		t.Fatal(err)
	}
	info := &WorkspaceInfo{TaskEnvironmentID: "env-1", ValidatedTaskEnvironmentGeneration: 1}
	if err := validateRetainedNomadLaunchInputs(info, req, "exec-1"); err != nil {
		t.Fatalf("pool continuation rejected: %v", err)
	}
}
