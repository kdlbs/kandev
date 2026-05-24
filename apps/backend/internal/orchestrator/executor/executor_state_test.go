package executor

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
)

// TestResolveExecutorConfig_PropagatesSSHShell pins the contract that the
// shell selection saved on an SSH executor profile lands in launch metadata
// under MetadataKeySSHShell. Without this, sshShellFromMetadata returns
// empty and WrapLoginShell silently falls back to bash — so a user who
// installed npx under zsh+nvm sees a misleading "npx not found" preflight
// error while the readiness probe (which takes shell straight from the
// request body) reports the agent as available.
func TestResolveExecutorConfig_PropagatesSSHShell(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	repo.executors["exec-ssh-1"] = &models.Executor{
		ID:   "exec-ssh-1",
		Type: models.ExecutorTypeSSH,
		Config: map[string]string{
			"ssh_host": "example.com",
		},
	}
	repo.executorProfiles["prof-1"] = &models.ExecutorProfile{
		ID:         "prof-1",
		ExecutorID: "exec-ssh-1",
		Config: map[string]string{
			"ssh_shell": "zsh",
		},
	}

	cfg := exec.resolveExecutorConfig(context.Background(), "exec-ssh-1", "ws-1", map[string]interface{}{
		"executor_profile_id": "prof-1",
	})

	got, _ := cfg.Metadata[lifecycle.MetadataKeySSHShell].(string)
	if got != "zsh" {
		t.Fatalf("metadata[%q] = %q, want %q", lifecycle.MetadataKeySSHShell, got, "zsh")
	}
}

// TestResolveExecutorConfig_OmitsEmptySSHShell asserts that an absent /
// blank ssh_shell on the profile leaves the metadata key unset, so
// sshShellFromMetadata returns empty and WrapLoginShell's defaultLoginShell
// kicks in instead of pinning the user to a literal empty string.
func TestResolveExecutorConfig_OmitsEmptySSHShell(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	repo.executors["exec-ssh-1"] = &models.Executor{
		ID:   "exec-ssh-1",
		Type: models.ExecutorTypeSSH,
	}
	repo.executorProfiles["prof-1"] = &models.ExecutorProfile{
		ID:         "prof-1",
		ExecutorID: "exec-ssh-1",
		Config:     map[string]string{},
	}

	cfg := exec.resolveExecutorConfig(context.Background(), "exec-ssh-1", "ws-1", map[string]interface{}{
		"executor_profile_id": "prof-1",
	})

	if _, present := cfg.Metadata[lifecycle.MetadataKeySSHShell]; present {
		t.Fatalf("metadata[%q] should not be set when profile.Config has no ssh_shell", lifecycle.MetadataKeySSHShell)
	}
}
