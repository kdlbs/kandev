package service

import (
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestCutoverSSHProfilesCannotOverrideExecutorTransport(t *testing.T) {
	svc, _, _ := createTestService(t)
	ctx := kubernetesAdminContext()
	executor := &models.Executor{Name: "Builders", Type: models.ExecutorTypeSSH, Status: models.ExecutorStatusActive}
	if err := svc.executors.CreateExecutor(ctx, executor); err != nil {
		t.Fatal(err)
	}
	profile, err := svc.CreateExecutorProfile(ctx, &CreateExecutorProfileRequest{ExecutorID: executor.ID, Name: "Shell", Config: map[string]string{"ssh_shell": "bash"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"ssh_connection_id", "ssh_host", "ssh_connection_stamp", "ssh_identity_file", "ssh_proxy_jump", "ssh_host_fingerprint"} {
		config := map[string]string{key: "override"}
		if _, err := svc.CreateExecutorProfile(ctx, &CreateExecutorProfileRequest{ExecutorID: executor.ID, Name: key, Config: config}); !errors.Is(err, ErrInvalidExecutorConfig) {
			t.Errorf("create accepted %s, err = %v", key, err)
		}
		if _, err := svc.UpdateExecutorProfile(ctx, profile.ID, &UpdateExecutorProfileRequest{Config: config}); !errors.Is(err, ErrInvalidExecutorConfig) {
			t.Errorf("update accepted %s, err = %v", key, err)
		}
	}
}
