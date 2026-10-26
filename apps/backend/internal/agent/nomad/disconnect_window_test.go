package nomad

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNomadProfileDisconnectLostAfter(t *testing.T) {
	cases := map[string]struct {
		raw  string
		want time.Duration
		ok   bool
	}{
		"default when absent": {"", DefaultDisconnectLostAfter, true},
		"lower bound":         {"1m", time.Minute, true},
		"upper bound":         {"168h", 7 * 24 * time.Hour, true},
		"explicit":            {"90m", 90 * time.Minute, true},
		"below lower bound":   {"59s", 0, false},
		"above upper bound":   {"169h", 0, false},
		"zero":                {"0s", 0, false},
		"negative":            {"-1h", 0, false},
		"not a duration":      {"soon", 0, false},
		"bare number":         {"60", 0, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			config := validTestProfile()
			if tc.raw != "" {
				config["disconnect_lost_after"] = tc.raw
			}
			parsed, err := ParseProfileConfig(config)
			if tc.ok != (err == nil) {
				t.Fatalf("ParseProfileConfig(%q) error = %v, want ok=%v", tc.raw, err, tc.ok)
			}
			if tc.ok && parsed.DisconnectLostAfter != tc.want {
				t.Fatalf("DisconnectLostAfter = %s, want %s", parsed.DisconnectLostAfter, tc.want)
			}
		})
	}
}

func disconnectWindowJob(t *testing.T, window time.Duration) (Job, ProfileConfig, EnvironmentState) {
	t.Helper()
	profile := validTestProfileConfig()
	profile.DisconnectLostAfter = window
	state := validTestState()
	state.DisconnectLostAfterSeconds = int64(window / time.Second)
	return retainedJob(t, profile, state)
}

// retainedJob builds the job, records its digest and paths in the state as the
// launch path does, and rebuilds it against that retained evidence.
func retainedJob(t *testing.T, profile ProfileConfig, state EnvironmentState) (Job, ProfileConfig, EnvironmentState) {
	t.Helper()
	state.JobID, _ = StableJobID(JobIdentity{EnvironmentID: state.EnvironmentID, Generation: state.Generation, OperationID: state.OperationID, InstallationID: state.InstallationID})
	agentctl := "/var/lib/kandev/allocations/" + state.JobID + "/agentctl-" + strings.Repeat("a", 64)
	bootstrap := "/var/lib/kandev/allocations/" + state.JobID + "/bootstrap-" + strings.Repeat("a", 64) + ".sh"
	job, err := BuildNativeJob(NativeJobInput{Profile: profile, State: state, AgentctlPath: agentctl, BootstrapPath: bootstrap})
	if err != nil {
		t.Fatalf("BuildNativeJob: %v", err)
	}
	digest, err := NativeJobSpecDigest(job)
	if err != nil {
		t.Fatal(err)
	}
	state.AgentctlPath, state.BootstrapPath = agentctl, bootstrap
	state.NativeJobPolicyVersion, state.NativeJobSpecSHA256 = NativeJobPolicyVersion, digest
	job, err = BuildNativeJob(NativeJobInput{Profile: profile, State: state, AgentctlPath: agentctl, BootstrapPath: bootstrap})
	if err != nil {
		t.Fatalf("BuildNativeJob with retained evidence: %v", err)
	}
	return job, profile, state
}

func TestNativeJobCarriesConfiguredLostAfter(t *testing.T) {
	job, _, state := disconnectWindowJob(t, 90*time.Minute)
	disconnect := job.TaskGroups[0].Disconnect
	if disconnect.LostAfter != int64(90*time.Minute) || disconnect.Replace || disconnect.Reconcile != "keep_original" {
		t.Fatalf("disconnect = %+v, want 90m lost_after, no replacement, keep_original", disconnect)
	}
	rebuilt, err := NativeJobFromState(state)
	if err != nil {
		t.Fatalf("NativeJobFromState: %v", err)
	}
	if rebuilt.TaskGroups[0].Disconnect.LostAfter != int64(90*time.Minute) {
		t.Fatalf("rebuilt lost_after = %d, want the retained window", rebuilt.TaskGroups[0].Disconnect.LostAfter)
	}
}

func TestNativeJobRejectsProfileAndStateWindowMismatch(t *testing.T) {
	_, profile, state := disconnectWindowJob(t, time.Hour)
	profile.DisconnectLostAfter = 2 * time.Hour
	_, err := BuildNativeJob(NativeJobInput{Profile: profile, State: state, AgentctlPath: state.AgentctlPath, BootstrapPath: state.BootstrapPath})
	if err == nil {
		t.Fatal("a profile window that differs from the retained state was accepted")
	}
}

func TestLegacyStateWithoutWindowRebuildsWithZeroLostAfter(t *testing.T) {
	profile := validTestProfileConfig()
	profile.DisconnectLostAfter = DefaultDisconnectLostAfter
	job, _, state := retainedJob(t, profile, validTestState())
	if state.DisconnectLostAfterSeconds != 0 || job.TaskGroups[0].Disconnect.LostAfter != 0 {
		t.Fatalf("legacy job lost_after = %d", job.TaskGroups[0].Disconnect.LostAfter)
	}
	rebuilt, err := NativeJobFromState(state)
	if err != nil {
		t.Fatalf("NativeJobFromState: %v", err)
	}
	if rebuilt.TaskGroups[0].Disconnect.LostAfter != 0 {
		t.Fatalf("legacy state rebuilt with lost_after %d, want 0", rebuilt.TaskGroups[0].Disconnect.LostAfter)
	}
}

func TestEnvironmentStateValidatesDisconnectWindow(t *testing.T) {
	for name, tc := range map[string]struct {
		seconds int64
		ok      bool
	}{"legacy zero": {0, true}, "min": {60, true}, "max": {7 * 24 * 3600, true}, "below": {59, false}, "above": {7*24*3600 + 1, false}, "negative": {-1, false}} {
		t.Run(name, func(t *testing.T) {
			state := validTestState()
			state.DisconnectLostAfterSeconds = tc.seconds
			if err := state.Validate(); tc.ok != (err == nil) {
				t.Fatalf("Validate() error = %v, want ok=%v", err, tc.ok)
			}
		})
	}
}

func TestValidateNativeJobResponseComparesConfiguredLostAfter(t *testing.T) {
	job, _, _ := disconnectWindowJob(t, 2*time.Hour)
	respond := func(lostAfter float64) []byte {
		var response map[string]any
		if err := json.Unmarshal(mustJSON(t, job), &response); err != nil {
			t.Fatal(err)
		}
		response["DispatchIdempotencyToken"] = ""
		group := response["TaskGroups"].([]any)[0].(map[string]any)
		group["Disconnect"].(map[string]any)["LostAfter"] = lostAfter
		group["Disconnect"].(map[string]any)["StopOnClientAfter"] = nil
		for _, item := range group["Networks"].([]any)[0].(map[string]any)["DynamicPorts"].([]any) {
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
		return raw
	}
	if err := ValidateNativeJobResponse(respond(float64(2*time.Hour)), job); err != nil {
		t.Fatalf("configured lost_after round trip rejected: %v", err)
	}
	for name, value := range map[string]float64{"zero is no longer a reviewed default": 0, "different window": float64(time.Hour)} {
		if err := ValidateNativeJobResponse(respond(value), job); err == nil {
			t.Fatalf("%s: response with lost_after %v was accepted", name, value)
		}
	}
}
