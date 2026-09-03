package capabilities

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestSupportsEmbeddedVscode(t *testing.T) {
	tests := []struct {
		name         string
		executorType models.ExecutorType
		hostOS       string
		want         bool
	}{
		{name: "local linux", executorType: models.ExecutorTypeLocal, hostOS: "linux", want: true},
		{name: "local macOS", executorType: models.ExecutorTypeLocal, hostOS: "darwin", want: true},
		{name: "local windows", executorType: models.ExecutorTypeLocal, hostOS: "windows", want: false},
		{name: "local unknown host", executorType: models.ExecutorTypeLocal, hostOS: "freebsd", want: false},
		{name: "worktree linux", executorType: models.ExecutorTypeWorktree, hostOS: "linux", want: true},
		{name: "worktree macOS", executorType: models.ExecutorTypeWorktree, hostOS: "darwin", want: true},
		{name: "worktree windows", executorType: models.ExecutorTypeWorktree, hostOS: "windows", want: false},
		{name: "worktree unknown host", executorType: models.ExecutorTypeWorktree, hostOS: "freebsd", want: false},
		{name: "local Docker on Windows host", executorType: models.ExecutorTypeLocalDocker, hostOS: "windows", want: true},
		{name: "remote Docker", executorType: models.ExecutorTypeRemoteDocker, hostOS: "windows", want: true},
		{name: "Sprites", executorType: models.ExecutorTypeSprites, hostOS: "windows", want: true},
		{name: "SSH", executorType: models.ExecutorTypeSSH, hostOS: "windows", want: true},
		{name: "Kubernetes", executorType: models.ExecutorTypeKubernetes, hostOS: "windows", want: true},
		{name: "mock remote fails closed", executorType: models.ExecutorTypeMockRemote, hostOS: "linux", want: false},
		{name: "unknown executor fails closed", executorType: models.ExecutorType("future"), hostOS: "linux", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SupportsEmbeddedVscode(tt.executorType, tt.hostOS); got != tt.want {
				t.Errorf("SupportsEmbeddedVscode(%q, %q) = %v, want %v", tt.executorType, tt.hostOS, got, tt.want)
			}
		})
	}
}
