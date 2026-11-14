package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestExecutorProfilesRejectSharedSecretGrants(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	executor := createTestExecutor(t, svc, "Local runner", models.ExecutorTypeLocal)
	shared := []models.ProfileEnvVar{{Key: "SHARED_TOKEN", SecretID: "synthetic-secret", SharedByUserID: "synthetic-owner"}}

	_, err := svc.CreateExecutorProfile(ctx, &CreateExecutorProfileRequest{
		ExecutorID: executor.ID, Name: "Shared", EnvVars: shared,
	})
	if !errors.Is(err, ErrExecutorProfileSecretShareUnsupported) {
		t.Fatalf("create with grant = %v, want ErrExecutorProfileSecretShareUnsupported", err)
	}

	profile, err := svc.CreateExecutorProfile(ctx, &CreateExecutorProfileRequest{
		ExecutorID: executor.ID, Name: "Plain", EnvVars: []models.ProfileEnvVar{{Key: "PLAIN", Value: "plain"}},
	})
	if err != nil {
		t.Fatalf("create plain profile: %v", err)
	}
	_, err = svc.UpdateExecutorProfile(ctx, profile.ID, &UpdateExecutorProfileRequest{EnvVars: shared})
	if !errors.Is(err, ErrExecutorProfileSecretShareUnsupported) {
		t.Fatalf("update with grant = %v, want ErrExecutorProfileSecretShareUnsupported", err)
	}
	stored, err := repo.GetExecutorProfile(ctx, profile.ID)
	if err != nil {
		t.Fatalf("get profile: %v", err)
	}
	if len(stored.EnvVars) != 1 || stored.EnvVars[0].Key != "PLAIN" || stored.EnvVars[0].SharedByUserID != "" {
		t.Fatalf("rejected update changed env vars to %#v", stored.EnvVars)
	}
}
