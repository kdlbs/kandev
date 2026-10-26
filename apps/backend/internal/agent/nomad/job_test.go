package nomad

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildNativeJobUsesPinnedSingleControllerAndFailClosedPolicies(t *testing.T) {
	profile := validTestProfileConfig()
	profile.Address = "http://127.0.0.1:4646"
	state := validTestState()
	state.JobID, _ = StableJobID(JobIdentity{EnvironmentID: state.EnvironmentID, Generation: state.Generation, OperationID: state.OperationID, InstallationID: state.InstallationID})
	job, err := BuildNativeJob(NativeJobInput{Profile: profile, State: state, AgentctlPath: "/var/lib/kandev/allocations/env-1/agentctl", BootstrapPath: "/var/lib/kandev/allocations/env-1/bootstrap.sh"})
	if err != nil {
		t.Fatalf("BuildNativeJob: %v", err)
	}
	if job.ID != state.JobID || job.Type != "batch" || job.Namespace != profile.Namespace || job.Region != profile.Region {
		t.Fatalf("job identity/type = %+v", job)
	}
	if len(job.Constraints) != 1 || job.Constraints[0].LTarget != "${node.unique.id}" || job.Constraints[0].RTarget != profile.NodeID {
		t.Fatalf("job is not pinned to the approved NodeID: %+v", job.Constraints)
	}
	if len(job.TaskGroups) != 1 {
		t.Fatalf("task groups = %d, want exactly 1", len(job.TaskGroups))
	}
	group := job.TaskGroups[0]
	if group.Count != 1 || group.RestartPolicy.Attempts != 0 || group.ReschedulePolicy.Attempts != 0 || group.ReschedulePolicy.Unlimited || group.Disconnect.Replace || group.Disconnect.Reconcile != "keep_original" {
		t.Fatalf("job can restart or replace the worker process: %+v", group)
	}
	if len(group.Networks) != 1 || len(group.Networks[0].DynamicPorts) != 2 {
		t.Fatalf("reserved runtime ports = %+v, want control and one instance port", group.Networks)
	}
	if len(group.Tasks) != 1 || group.Tasks[0].Driver != "raw_exec" || group.Tasks[0].Config["command"] != "/bin/sh" {
		t.Fatalf("task is not the single native bootstrap: %+v", group.Tasks)
	}
	if got := group.Tasks[0].Env["KANDEV_NOMAD_NODE_ID"]; got != "${node.unique.id}" {
		t.Fatalf("task node identity = %q, want Nomad node interpolation", got)
	}
	if strings.Contains(string(mustJSON(t, job)), "nonce") || strings.Contains(string(mustJSON(t, job)), "secret-value") {
		t.Fatal("job specification contains bootstrap secret material")
	}
}

func TestBuildNativeJobUsesEnvironmentScopedWorkspace(t *testing.T) {
	profile := validTestProfileConfig()
	profile.Address = "http://127.0.0.1:4646"
	profile.WorkspaceRoot = "/var/lib/kandev/workspaces"
	state := validTestState()
	state.WorkspaceRoot = profile.WorkspaceRoot
	state.WorkspacePath = "/var/lib/kandev/workspaces/environments/env-1/g3"
	state.JobID, _ = StableJobID(JobIdentity{EnvironmentID: state.EnvironmentID, Generation: state.Generation, OperationID: state.OperationID, InstallationID: state.InstallationID})
	job, err := BuildNativeJob(NativeJobInput{Profile: profile, State: state, AgentctlPath: "/var/lib/kandev/allocations/env-1/agentctl", BootstrapPath: "/var/lib/kandev/allocations/env-1/bootstrap.sh"})
	if err != nil {
		t.Fatalf("BuildNativeJob rejected an environment-scoped workspace: %v", err)
	}
	if got := job.TaskGroups[0].Tasks[0].Env["KANDEV_WORKSPACE_ROOT"]; got != state.WorkspacePath {
		t.Fatalf("job workspace root = %q, want immutable environment workspace %q", got, state.WorkspacePath)
	}
}

func TestBuildNativeJobRejectsChangedIdentityAndUnsafePaths(t *testing.T) {
	profile := validTestProfileConfig()
	profile.Address = "http://127.0.0.1:4646"
	state := validTestState()
	state.JobID = "wrong-job"
	_, err := BuildNativeJob(NativeJobInput{Profile: profile, State: state, AgentctlPath: "/opt/kandev/bin/agentctl", BootstrapPath: "/opt/kandev/private/bootstrap.sh"})
	if err == nil {
		t.Fatal("BuildNativeJob accepted a job ID inconsistent with immutable identity")
	}
	state = validTestState()
	state.JobID, _ = StableJobID(JobIdentity{EnvironmentID: state.EnvironmentID, Generation: state.Generation, OperationID: state.OperationID, InstallationID: state.InstallationID})
	_, err = BuildNativeJob(NativeJobInput{Profile: profile, State: state, AgentctlPath: "/var/lib/kandev/allocations/env-1/../escape", BootstrapPath: "/var/lib/kandev/allocations/env-1/bootstrap.sh"})
	if err == nil {
		t.Fatal("BuildNativeJob accepted an unnormalized agentctl path")
	}
}

func TestNomadRegisterRequestUsesCreateOnlyEnforcedIndex(t *testing.T) {
	request := JobRegisterRequest{Job: Job{ID: "job-1"}, EnforceIndex: true, JobModifyIndex: 0}
	encoded := mustJSON(t, request)
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["EnforceIndex"] != true || decoded["JobModifyIndex"] != float64(0) {
		t.Fatalf("register request is not create-only: %s", encoded)
	}
}

func TestValidateNativeJobResponseRejectsChangedAndUnknownNestedBehavior(t *testing.T) {
	profile := validTestProfileConfig()
	state := validTestState()
	state.JobID, _ = StableJobID(JobIdentity{EnvironmentID: state.EnvironmentID, Generation: state.Generation, OperationID: state.OperationID, InstallationID: state.InstallationID})
	paths := NativeJobInput{Profile: profile, State: state, AgentctlPath: "/var/lib/kandev/allocations/" + state.JobID + "/agentctl-" + strings.Repeat("a", 64), BootstrapPath: "/var/lib/kandev/allocations/" + state.JobID + "/bootstrap-" + strings.Repeat("a", 64) + ".sh"}
	job, err := BuildNativeJob(paths)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := NativeJobSpecDigest(job)
	if err != nil {
		t.Fatal(err)
	}
	state.AgentctlPath, state.BootstrapPath = paths.AgentctlPath, paths.BootstrapPath
	state.NativeJobPolicyVersion, state.NativeJobSpecSHA256 = NativeJobPolicyVersion, digest
	job, err = BuildNativeJob(NativeJobInput{Profile: profile, State: state, AgentctlPath: state.AgentctlPath, BootstrapPath: state.BootstrapPath})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(job)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateNativeJobResponse(raw, job); err != nil {
		t.Fatalf("unchanged complete job response rejected: %v", err)
	}
	var canonical map[string]any
	if err := json.Unmarshal(raw, &canonical); err != nil {
		t.Fatal(err)
	}
	canonical["CreateIndex"], canonical["ModifyIndex"], canonical["JobModifyIndex"] = float64(11), float64(13), float64(12)
	canonical["Status"], canonical["StatusDescription"], canonical["Version"] = "running", "fixture status", float64(2)
	canonical["Stop"], canonical["AllAtOnce"], canonical["Dispatched"] = false, false, false
	group := canonical["TaskGroups"].([]any)[0].(map[string]any)
	task := group["Tasks"].([]any)[0].(map[string]any)
	task["RestartPolicy"] = cloneJobTestMap(group["RestartPolicy"].(map[string]any)) // task inherits the group policy
	defaultsRaw, _ := json.Marshal(canonical)
	if err := ValidateNativeJobResponse(defaultsRaw, job); err != nil {
		t.Fatalf("reviewed Nomad observations/defaults rejected: %v", err)
	}

	var changed map[string]any
	if err := json.Unmarshal(raw, &changed); err != nil {
		t.Fatal(err)
	}
	task = changed["TaskGroups"].([]any)[0].(map[string]any)["Tasks"].([]any)[0].(map[string]any)
	task["Lifecycle"] = map[string]any{"Hook": "prestart", "Sidecar": true}
	changedRaw, _ := json.Marshal(changed)
	if err := ValidateNativeJobResponse(changedRaw, job); err == nil {
		t.Fatal("job with an added lifecycle hook was accepted despite matching ownership metadata")
	}

	unknown := strings.Replace(string(raw), `"Type":"batch"`, `"Type":"batch","UnrecognizedBehavior":false`, 1)
	if err := ValidateNativeJobResponse([]byte(unknown), job); err == nil {
		t.Fatal("unknown behavior field was accepted even with a zero value")
	}

	duplicate := strings.Replace(string(raw), `"ID":"`+job.ID+`"`, `"ID":"`+job.ID+`","ID":"`+job.ID+`"`, 1)
	if err := ValidateNativeJobResponse([]byte(duplicate), job); err == nil {
		t.Fatal("duplicate JSON key was accepted")
	}

	var changedCommand map[string]any
	if err := json.Unmarshal(raw, &changedCommand); err != nil {
		t.Fatal(err)
	}
	task = changedCommand["TaskGroups"].([]any)[0].(map[string]any)["Tasks"].([]any)[0].(map[string]any)
	task["Config"].(map[string]any)["args"] = []any{"/tmp/other-bootstrap.sh"}
	changedCommandRaw, _ := json.Marshal(changedCommand)
	if err := ValidateNativeJobResponse(changedCommandRaw, job); err == nil {
		t.Fatal("changed raw_exec bootstrap command was accepted")
	}
}

func TestValidateNativeJobResponseAcceptsExactNomad207RoundTripDefaults(t *testing.T) {
	profile := validTestProfileConfig()
	state := validTestState()
	state.JobID, _ = StableJobID(JobIdentity{EnvironmentID: state.EnvironmentID, Generation: state.Generation, OperationID: state.OperationID, InstallationID: state.InstallationID})
	paths := NativeJobInput{Profile: profile, State: state, AgentctlPath: "/var/lib/kandev/allocations/" + state.JobID + "/agentctl-" + strings.Repeat("a", 64), BootstrapPath: "/var/lib/kandev/allocations/" + state.JobID + "/bootstrap-" + strings.Repeat("a", 64) + ".sh"}
	job, err := BuildNativeJob(paths)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := NativeJobSpecDigest(job)
	if err != nil {
		t.Fatal(err)
	}
	state.AgentctlPath, state.BootstrapPath = paths.AgentctlPath, paths.BootstrapPath
	state.NativeJobPolicyVersion, state.NativeJobSpecSHA256 = NativeJobPolicyVersion, digest
	job, err = BuildNativeJob(NativeJobInput{Profile: profile, State: state, AgentctlPath: state.AgentctlPath, BootstrapPath: state.BootstrapPath})
	if err != nil {
		t.Fatal(err)
	}

	// This is the sanitized shape captured from Nomad 2.0.7's full job GET:
	// empty/default response fields are explicit, while dynamic port Value/To
	// fields are omitted. Keep the actual capture separately under testdata.
	var response map[string]any
	if err := json.Unmarshal(mustJSON(t, job), &response); err != nil {
		t.Fatal(err)
	}
	response["DispatchIdempotencyToken"] = ""
	group := response["TaskGroups"].([]any)[0].(map[string]any)
	disconnect := group["Disconnect"].(map[string]any)
	disconnect["LostAfter"], disconnect["StopOnClientAfter"] = float64(0), nil
	ports := group["Networks"].([]any)[0].(map[string]any)["DynamicPorts"].([]any)
	for _, item := range ports {
		delete(item.(map[string]any), "Value")
		delete(item.(map[string]any), "To")
	}
	task := group["Tasks"].([]any)[0].(map[string]any)
	task["Identity"] = map[string]any{
		"Name": "default", "Audience": []any{"nomadproject.io"}, "ChangeMode": "", "ChangeSignal": "",
		"Env": false, "File": false, "Filepath": "", "ServiceName": "", "TTL": float64(0), "ExtraClaims": nil,
	}
	task["Kind"], task["Meta"], task["ShutdownDelay"] = "", nil, float64(0)
	raw, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateNativeJobResponse(raw, job); err != nil {
		t.Fatalf("exact Nomad 2.0.7 round-trip defaults were rejected: %v", err)
	}

	for name, mutate := range map[string]func(map[string]any){
		"nonzero lost_after": func(value map[string]any) {
			value["TaskGroups"].([]any)[0].(map[string]any)["Disconnect"].(map[string]any)["LostAfter"] = float64(1)
		},
		"stop_on_client_after": func(value map[string]any) {
			value["TaskGroups"].([]any)[0].(map[string]any)["Disconnect"].(map[string]any)["StopOnClientAfter"] = float64(1)
		},
		"numeric zero stop_on_client_after is not null": func(value map[string]any) {
			value["TaskGroups"].([]any)[0].(map[string]any)["Disconnect"].(map[string]any)["StopOnClientAfter"] = float64(0)
		},
		"assigned dynamic port": func(value map[string]any) {
			value["TaskGroups"].([]any)[0].(map[string]any)["Networks"].([]any)[0].(map[string]any)["DynamicPorts"].([]any)[0].(map[string]any)["Value"] = float64(41000)
		},
		"non-default identity": func(value map[string]any) {
			value["TaskGroups"].([]any)[0].(map[string]any)["Tasks"].([]any)[0].(map[string]any)["Identity"].(map[string]any)["Env"] = true
		},
		"task kind": func(value map[string]any) {
			value["TaskGroups"].([]any)[0].(map[string]any)["Tasks"].([]any)[0].(map[string]any)["Kind"] = "service"
		},
		"dispatch idempotency token": func(value map[string]any) { value["DispatchIdempotencyToken"] = "unexpected" },
	} {
		t.Run(name, func(t *testing.T) {
			var changed map[string]any
			if err := json.Unmarshal(raw, &changed); err != nil {
				t.Fatal(err)
			}
			mutate(changed)
			changedRaw, err := json.Marshal(changed)
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidateNativeJobResponse(changedRaw, job); err == nil {
				t.Fatal("changed behavior was accepted as a reviewed server default")
			}
		})
	}

	var changed map[string]any
	if err := json.Unmarshal(raw, &changed); err != nil {
		t.Fatal(err)
	}
	changed["DispatchIdempotencyToken"] = "unexpected"
	changedRaw, err := json.Marshal(changed)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateNativeJobResponse(changedRaw, job); err == nil || !strings.Contains(err.Error(), "$.DispatchIdempotencyToken") {
		t.Fatalf("mismatch should report a safe field path, got %v", err)
	}
}

func TestNomad207ObservedResponseDefaultsAreExplicitAndStopRemainsBehavior(t *testing.T) {
	readFixture := func(name string) []byte {
		t.Helper()
		data, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	jobRaw := readFixture("nomad-2.0.7-job-stopped.json")
	jobValue, err := decodeExactJSON(jobRaw)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateJobFieldTree(jobValue, "$", 0); err != nil {
		t.Fatalf("captured Nomad 2.0.7 GET job fields were not reviewed: %v", err)
	}
	jobNormalized, err := normalizeJobResponse(jobValue, "$", nil)
	if err != nil {
		t.Fatalf("normalize captured Nomad 2.0.7 GET job: %v", err)
	}
	job := jobNormalized.(map[string]any)
	currentJob := jobValue.(map[string]any)
	if currentJob["DispatchIdempotencyToken"] != "" {
		t.Fatal("captured Nomad 2.0.7 job no longer documents the empty dispatch token default")
	}
	group := currentJob["TaskGroups"].([]any)[0].(map[string]any)
	disconnect := group["Disconnect"].(map[string]any)
	if disconnect["LostAfter"] != json.Number("0") || disconnect["StopOnClientAfter"] != nil {
		t.Fatal("captured Nomad 2.0.7 disconnect defaults changed")
	}
	port := group["Networks"].([]any)[0].(map[string]any)["DynamicPorts"].([]any)[0].(map[string]any)
	if _, exists := port["Value"]; exists {
		t.Fatal("captured dynamic port should omit zero Value")
	}
	if _, exists := port["To"]; exists {
		t.Fatal("captured dynamic port should omit zero To")
	}
	task := group["Tasks"].([]any)[0].(map[string]any)
	if !isDefaultWorkloadIdentity(task["Identity"]) || task["Kind"] != "" || task["Meta"] != nil || task["ShutdownDelay"] != json.Number("0") {
		t.Fatal("captured Nomad 2.0.7 task defaults changed")
	}
	if job["Stop"] != true {
		t.Fatalf("captured stopped job lost its behavior-changing Stop=true field: %#v", job["Stop"])
	}
	if _, exists := job["Update"]; exists {
		t.Fatal("exact Nomad 2.0.7 no-op Update defaults were not normalized")
	}
	allocationRaw := readFixture("nomad-2.0.7-allocation-stopped.json")
	allocation, err := decodeExactJSON(allocationRaw)
	if err != nil {
		t.Fatal(err)
	}
	allocationObject := allocation.(map[string]any)
	embedded, ok := allocationObject["Job"].(map[string]any)
	if !ok {
		t.Fatal("captured full allocation omitted its embedded job")
	}
	if err := validateJobFieldTree(embedded, "$", 0); err != nil {
		t.Fatalf("captured allocation embedded job fields were not reviewed: %v", err)
	}
	embeddedNormalized, err := normalizeJobResponse(embedded, "$", nil)
	if err != nil {
		t.Fatalf("normalize captured allocation embedded job: %v", err)
	}
	if embedded["Stop"] != false {
		t.Fatal("allocation's original run job must retain Stop=false in the captured response")
	}
	if _, exists := embeddedNormalized.(map[string]any)["Stop"]; exists {
		t.Fatal("the exact Stop=false default should normalize away only after the raw value was reviewed")
	}
	if currentJob["JobModifyIndex"] == embedded["JobModifyIndex"] {
		t.Fatal("stopped current job fixture should retain its changed definition index relative to the historical allocation")
	}
}

func cloneJobTestMap(source map[string]any) map[string]any {
	clone := make(map[string]any, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func TestBuildBootstrapScriptUsesPermanentWorkerFenceAndPrivateFile(t *testing.T) {
	state := validTestState()
	state.JobID, _ = StableJobID(JobIdentity{EnvironmentID: state.EnvironmentID, Generation: state.Generation, OperationID: state.OperationID, InstallationID: state.InstallationID})
	script, err := BuildBootstrapScript(state, state.AllocationRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"if ! mkdir '/var/lib/worker-state/node-b/active-environment'",
		"KANDEV_BOOTSTRAP_ENV_FILE",
		"NOMAD_JOB_ID",
		"NOMAD_ALLOC_ID",
		"KANDEV_NOMAD_NODE_ID",
		"$KANDEV_NOMAD_NODE_ID",
		"mkdir -p \"$workspace_root\"",
		"export AGENTCTL_INSTANCE_PORT_BASE=\"$instance_port\"",
		"export AGENTCTL_INSTANCE_PORT_MAX=\"$instance_port\"",
		". \"$bootstrap_env_file\"",
		"agentctl_path=$KANDEV_AGENTCTL_PATH",
		"export KANDEV_AGENTCTL_PATH=\"$agentctl_path\"",
		"AGENTCTL_LISTEN_HOST=127.0.0.1",
		"exec \"$agentctl_path\"",
	} {
		if !strings.Contains(script, required) {
			t.Errorf("bootstrap script missing %q", required)
		}
	}
	for _, forbidden := range []string{`${NOMAD_NODE_ID`, "rm -rf", "sleep", "PID", "AGENTCTL_BOOTSTRAP_NONCE=", "nomad_token"} {
		if strings.Contains(script, forbidden) {
			t.Errorf("bootstrap script includes unsafe/secret behavior %q", forbidden)
		}
	}
}

func TestBuildBootstrapScriptRejectsChangedAllocationRoot(t *testing.T) {
	if _, err := BuildBootstrapScript(validTestState(), "/tmp/other"); err == nil {
		t.Fatal("bootstrap accepted allocation root that differs from immutable worker state")
	}
}

func TestEncodePrivateEnvironmentQuotesValuesAndRejectsInvalidKeys(t *testing.T) {
	encoded, err := EncodePrivateEnvironment(map[string]string{
		"AGENTCTL_BOOTSTRAP_NONCE": "one-use value'with quote",
		"KANDEV_API_KEY":           "opaque-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "opaque-secret") == false || !strings.Contains(string(encoded), "'\\''") {
		t.Fatalf("private env fragment did not preserve safely quoted values: %q", encoded)
	}
	if _, err := EncodePrivateEnvironment(map[string]string{"BAD=KEY": "x"}); err == nil {
		t.Fatal("private env serializer accepted a shell injection key")
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func validTestProfileConfig() ProfileConfig {
	return ProfileConfig{
		Namespace: "default", Region: "global", Datacenter: "dc1", NodeID: "node-b",
		Platform: "darwin/arm64", SSHUser: "kandev", SSHHost: "worker-b.example", SSHPort: 22,
		SSHHostFingerprint: "SHA256:abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJK",
		WorkspaceRoot:      "/var/lib/kandev/workspaces", AllocationRoot: "/var/lib/kandev/allocations",
		CPUMHz: 1500, MemoryMB: 4096, NomadTokenSecretID: "secret-nomad-token",
	}
}

func TestNomadNativeJobDisconnectPolicy(t *testing.T) {
	profile := validTestProfileConfig()
	profile.Address = "http://127.0.0.1:4646"
	state := validTestState()
	state.JobID, _ = StableJobID(JobIdentity{EnvironmentID: state.EnvironmentID, Generation: state.Generation, OperationID: state.OperationID, InstallationID: state.InstallationID})
	job, err := BuildNativeJob(NativeJobInput{Profile: profile, State: state, AgentctlPath: "/var/lib/kandev/allocations/env-1/agentctl", BootstrapPath: "/var/lib/kandev/allocations/env-1/bootstrap.sh"})
	if err != nil {
		t.Fatalf("BuildNativeJob: %v", err)
	}
	group := job.TaskGroups[0]
	if group.Count != 1 || group.RestartPolicy.Attempts != 0 || group.ReschedulePolicy.Attempts != 0 || group.ReschedulePolicy.Unlimited {
		t.Fatalf("group can restart, reschedule, or scale: %+v", group)
	}
	if group.Disconnect.Replace || group.Disconnect.Reconcile != "keep_original" {
		t.Fatalf("disconnect policy = %+v, want no replacement and keep_original", group.Disconnect)
	}
}
