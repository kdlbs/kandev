package executor

import (
	"context"
	"errors"
	"testing"

	runtimeenv "github.com/kandev/kandev/internal/agent/runtime/environment"
	"github.com/kandev/kandev/internal/task/models"
)

// TestTaskEnvironmentSources_SkipsBlankProfileEnvVar pins AC-38b at the
// orchestrator assembly site: a ProfileEnvVar blank on both Value and
// SecretID must not produce an environmentSource, and its key must not
// reach req.EnvironmentDefinitions. This reverses the earlier (false) spec
// assumption that validateEnvVarValue already prevents such an entry —
// that check is agent-profile-only, so an executor profile can hold one.
func TestTaskEnvironmentSources_SkipsBlankProfileEnvVar(t *testing.T) {
	exec := newTestExecutor(t, &mockAgentManager{}, newMockRepository())

	sources, err := exec.taskEnvironmentSources("workspace-1", nil, []models.ProfileEnvVar{
		{Key: "POPULATED", Value: "value"},
		{Key: "BLANK"},
	}, nil)
	if err != nil {
		t.Fatalf("taskEnvironmentSources() error = %v", err)
	}

	if len(sources) != 1 || sources[0].key != "POPULATED" {
		t.Fatalf("sources = %#v, want only the populated entry", sources)
	}
	for _, source := range sources {
		if source.key == "BLANK" {
			t.Fatalf("sources = %#v, blank entry must not be assembled", sources)
		}
	}
}

// TestTaskEnvironmentSources_KeepsSecretBackedBlankValueEntry pins the exact
// boundary AC-38b draws: an entry with a blank Value but a non-empty
// SecretID is secret-backed, not blank, and SHALL still be emitted.
func TestTaskEnvironmentSources_KeepsSecretBackedBlankValueEntry(t *testing.T) {
	exec := newTestExecutor(t, &mockAgentManager{}, newMockRepository())

	sources, err := exec.taskEnvironmentSources("workspace-1", nil, []models.ProfileEnvVar{
		{Key: "TOKEN", SecretID: "secret-1"},
	}, nil)
	if err != nil {
		t.Fatalf("taskEnvironmentSources() error = %v", err)
	}
	if len(sources) != 1 || sources[0].key != "TOKEN" || sources[0].secretID != "secret-1" {
		t.Fatalf("sources = %#v, want the secret-backed entry kept", sources)
	}
}

// TestResolveLaunchEnvironment_DuplicateExecutorProfileKeyConflicts pins
// AC-6a through Executor.taskEnvironmentSources, per spec: the duplicate-key
// save-time guarantee is agent-profile-only (profile_crud.go:908), so one
// executor profile may legitimately hold two entries for the same key with
// different values, and the assembled path must still reject that pair —
// unchanged from today, since executor profile is Tier 1 and nothing here
// outranks it.
func TestResolveLaunchEnvironment_DuplicateExecutorProfileKeyConflicts(t *testing.T) {
	exec := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	req := &LaunchAgentRequest{
		WorkspaceID:  "workspace-1",
		ExecutorType: string(models.ExecutorTypeLocal),
		Env:          map[string]string{},
	}

	err := exec.resolveLaunchEnvironment(context.Background(), req, []models.ProfileEnvVar{
		{Key: "TOKEN", Value: "value-a"},
		{Key: "TOKEN", Value: "value-b"},
	}, nil)

	if err == nil {
		t.Fatal("resolveLaunchEnvironment() error = nil, want conflict")
	}
	var conflictErr *EnvironmentConflictError
	if !errors.As(err, &conflictErr) {
		t.Fatalf("error = %T %v, want EnvironmentConflictError", err, err)
	}
	if len(conflictErr.Origins) != 1 || conflictErr.Origins[0] != runtimeenv.OriginExecutorProfile {
		t.Fatalf("conflict origins = %#v, want only %q", conflictErr.Origins, runtimeenv.OriginExecutorProfile)
	}
}

// TestResolveLaunchEnvironment_ExecutorProfileVsManagedRuntimeStillConflicts
// pins AC-40's second clause: precedence creates no new path to resolve a
// key two Tier 1 origins both claim. An executor-profile definition for a
// key managed runtime also sets must still fail the preflight, exactly as
// it does today, because both origins tie at the top tier.
func TestResolveLaunchEnvironment_ExecutorProfileVsManagedRuntimeStillConflicts(t *testing.T) {
	exec := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	req := &LaunchAgentRequest{
		WorkspaceID:  "workspace-1",
		ExecutorType: string(models.ExecutorTypeLocal),
		Env:          map[string]string{"KANDEV_FOO": "managed-value"},
	}

	err := exec.resolveLaunchEnvironment(context.Background(), req, []models.ProfileEnvVar{
		{Key: "KANDEV_FOO", Value: "profile-value"},
	}, nil)

	if err == nil {
		t.Fatal("resolveLaunchEnvironment() error = nil, want conflict")
	}
	var conflictErr *EnvironmentConflictError
	if !errors.As(err, &conflictErr) {
		t.Fatalf("error = %T %v, want EnvironmentConflictError", err, err)
	}
}
