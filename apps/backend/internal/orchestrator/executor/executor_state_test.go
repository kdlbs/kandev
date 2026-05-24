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

// TestResolveExecutorConfig_AuthoritativeSSHKeys_ClobberTaskMetadata pins
// the security contract from B1 of the thermo-nuclear review: a task that
// supplies ssh_workdir_root or ssh_shell in its metadata MUST be
// overridden by the profile's value — including when the profile has no
// value set (empty wins). Without this, a task can redirect every remote
// command on launch to a host path or login shell of its choosing.
func TestResolveExecutorConfig_AuthoritativeSSHKeys_ClobberTaskMetadata(t *testing.T) {
	cases := []struct {
		name         string
		key          string
		profileValue string
		taskValue    string
		wantMetadata string // empty means the key should be present with ""
	}{
		{
			name:         "profile_wins_when_set",
			key:          lifecycle.MetadataKeySSHWorkdirRoot,
			profileValue: "/srv/kandev",
			taskValue:    "/etc",
			wantMetadata: "/srv/kandev",
		},
		{
			name:         "empty_profile_clobbers_task",
			key:          lifecycle.MetadataKeySSHWorkdirRoot,
			profileValue: "",
			taskValue:    "/etc",
			wantMetadata: "",
		},
		{
			name:         "shell_profile_wins_when_set",
			key:          lifecycle.MetadataKeySSHShell,
			profileValue: "zsh",
			taskValue:    "bash --norc",
			wantMetadata: "zsh",
		},
		{
			name:         "empty_shell_profile_clobbers_task",
			key:          lifecycle.MetadataKeySSHShell,
			profileValue: "",
			taskValue:    "bash --norc",
			wantMetadata: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMockRepository()
			exec := newTestExecutor(t, &mockAgentManager{}, repo)
			repo.executors["exec-ssh-1"] = &models.Executor{ID: "exec-ssh-1", Type: models.ExecutorTypeSSH}
			repo.executorProfiles["prof-1"] = &models.ExecutorProfile{
				ID:         "prof-1",
				ExecutorID: "exec-ssh-1",
				Config:     map[string]string{tc.key: tc.profileValue},
			}

			cfg := exec.resolveExecutorConfig(context.Background(), "exec-ssh-1", "ws-1", map[string]interface{}{
				"executor_profile_id": "prof-1",
				tc.key:                tc.taskValue,
			})

			got, _ := cfg.Metadata[tc.key].(string)
			if got != tc.wantMetadata {
				t.Fatalf("metadata[%q] = %q, want %q (profile=%q task=%q)",
					tc.key, got, tc.wantMetadata, tc.profileValue, tc.taskValue)
			}
		})
	}
}

// TestResolveExecutorConfig_PassthroughKeys_DontClobberTaskMetadata pins
// the inverse: ordinary passthrough keys (git_user_name etc.) keep their
// historical "only set if non-empty" semantics. A task-supplied value
// survives when the profile leaves the key blank. This is intentional
// for non-security-sensitive keys; flag explicitly if it ever needs to
// change.
func TestResolveExecutorConfig_PassthroughKeys_DontClobberTaskMetadata(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	repo.executors["exec-ssh-1"] = &models.Executor{ID: "exec-ssh-1", Type: models.ExecutorTypeSSH}
	repo.executorProfiles["prof-1"] = &models.ExecutorProfile{
		ID:         "prof-1",
		ExecutorID: "exec-ssh-1",
		Config:     map[string]string{}, // profile has no git_user_name
	}

	cfg := exec.resolveExecutorConfig(context.Background(), "exec-ssh-1", "ws-1", map[string]interface{}{
		"executor_profile_id": "prof-1",
		"git_user_name":       "Task Author",
	})

	if got, _ := cfg.Metadata["git_user_name"].(string); got != "Task Author" {
		t.Fatalf("metadata[git_user_name] = %q, want task value to survive empty profile", got)
	}
}
