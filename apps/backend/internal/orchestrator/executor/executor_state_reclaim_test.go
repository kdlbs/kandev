package executor

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
)

// Reclaiming a remote task directory is irreversible and happens on a machine
// Kandev does not own. The executor profile is the only thing allowed to arm
// it: a task that could smuggle ssh_reclaim_task_dir through its own metadata
// would be authorizing a deletion on a host whose profile never opted in.
//
// The mechanism is placement in profileConfigAuthoritativeKeys rather than
// profileConfigPassthroughKeys — authoritative keys are written from the
// profile unconditionally, including when the profile value is empty. This is
// the same treatment ssh_workdir_root and ssh_shell get, for the same
// redirect-vector reason.
func TestApplyProfileConfigToMetadata_TaskCannotEnableReclaim(t *testing.T) {
	tests := []struct {
		name          string
		profileConfig map[string]string
		taskMetadata  map[string]interface{}
		want          string
	}{
		{
			name:          "task metadata cannot enable reclaim when the profile is silent",
			profileConfig: map[string]string{},
			taskMetadata:  map[string]interface{}{lifecycle.MetadataKeySSHReclaimTaskDir: "true"},
			want:          "",
		},
		{
			name:          "task metadata cannot enable reclaim when the profile disables it",
			profileConfig: map[string]string{lifecycle.MetadataKeySSHReclaimTaskDir: "false"},
			taskMetadata:  map[string]interface{}{lifecycle.MetadataKeySSHReclaimTaskDir: "true"},
			want:          "false",
		},
		{
			name:          "profile opt-in reaches launch metadata",
			profileConfig: map[string]string{lifecycle.MetadataKeySSHReclaimTaskDir: "true"},
			taskMetadata:  map[string]interface{}{},
			want:          "true",
		},
		{
			name:          "task metadata cannot disable a profile opt-in",
			profileConfig: map[string]string{lifecycle.MetadataKeySSHReclaimTaskDir: "true"},
			taskMetadata:  map[string]interface{}{lifecycle.MetadataKeySSHReclaimTaskDir: "false"},
			want:          "true",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			metadata := map[string]interface{}{}
			for k, v := range tc.taskMetadata {
				metadata[k] = v
			}
			applyProfileConfigToMetadata(tc.profileConfig, metadata)

			got, _ := metadata[lifecycle.MetadataKeySSHReclaimTaskDir].(string)
			if got != tc.want {
				t.Fatalf("metadata[%s] = %q, want %q",
					lifecycle.MetadataKeySSHReclaimTaskDir, got, tc.want)
			}
			if tc.want != "true" && lifecycle.SSHReclaimEnabled(metadata) {
				t.Fatal("SSHReclaimEnabled = true for metadata that must not enable reclamation")
			}
			if tc.want == "true" && !lifecycle.SSHReclaimEnabled(metadata) {
				t.Fatal("SSHReclaimEnabled = false despite a profile opt-in")
			}
		})
	}
}

// TestProfileConfigReclaimKeyIsAuthoritative pins the list membership itself,
// so moving the key to the passthrough list fails here rather than silently
// re-opening the smuggling path above.
func TestProfileConfigReclaimKeyIsAuthoritative(t *testing.T) {
	for _, key := range profileConfigPassthroughKeys {
		if key == lifecycle.MetadataKeySSHReclaimTaskDir {
			t.Fatalf("%s must not be a passthrough key", lifecycle.MetadataKeySSHReclaimTaskDir)
		}
	}
	for _, key := range profileConfigAuthoritativeKeys {
		if key == lifecycle.MetadataKeySSHReclaimTaskDir {
			return
		}
	}
	t.Fatalf("%s is missing from profileConfigAuthoritativeKeys", lifecycle.MetadataKeySSHReclaimTaskDir)
}
