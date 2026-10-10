package service

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// TestSSHLiveStatusApplies pins which executor types have an SSH connection
// worth surfacing. Remote Docker reaches its daemon over SSH and records the
// same metadata keys as the SSH executor, so the environment popover can show
// the host and build a shell command that actually reaches the container.
// Without it the popover offers `docker exec` against the viewer's own
// machine, where the container does not exist.
func TestSSHLiveStatusApplies(t *testing.T) {
	cases := []struct {
		executorType models.ExecutorType
		want         bool
	}{
		{models.ExecutorTypeSSH, true},
		{models.ExecutorTypeRemoteDocker, true},
		{models.ExecutorTypeLocalDocker, false},
		{models.ExecutorTypeLocal, false},
		{models.ExecutorTypeWorktree, false},
		{models.ExecutorTypeKubernetes, false},
		{models.ExecutorTypeSprites, false},
	}

	for _, tc := range cases {
		t.Run(string(tc.executorType), func(t *testing.T) {
			if got := sshLiveStatusApplies(string(tc.executorType)); got != tc.want {
				t.Errorf("sshLiveStatusApplies(%q) = %v, want %v", tc.executorType, got, tc.want)
			}
		})
	}

	if sshLiveStatusApplies("") {
		t.Error("an empty executor type must not be treated as SSH-backed")
	}
}
