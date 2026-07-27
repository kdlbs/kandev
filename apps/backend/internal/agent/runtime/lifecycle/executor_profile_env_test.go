package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
)

type fakeExecutorProfileReader struct {
	env         *models.TaskEnvironment
	envErr      error
	profile     *models.ExecutorProfile
	profileErr  error
	profileArgs []string
}

func (f *fakeExecutorProfileReader) GetTaskEnvironment(_ context.Context, _ string) (*models.TaskEnvironment, error) {
	return f.env, f.envErr
}

func (f *fakeExecutorProfileReader) GetExecutorProfile(_ context.Context, id string) (*models.ExecutorProfile, error) {
	f.profileArgs = append(f.profileArgs, id)
	return f.profile, f.profileErr
}

func newExecutorProfileEnvManager(t *testing.T, reader ExecutorProfileReader) *Manager {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	store := newInMemorySecretStore()
	if createErr := store.Create(context.Background(), &secrets.SecretWithValue{
		Secret: secrets.Secret{ID: "sec-npm", Name: "npm-token"},
		Value:  "fa-secret-value",
	}); createErr != nil {
		t.Fatalf("seed secret: %v", createErr)
	}
	m := &Manager{logger: log, secretStore: store}
	m.SetExecutorProfileReader(reader)
	return m
}

// The terminal must see the same executor-profile env vars the agent subprocess
// and the repository setup script get (PR #1971 covered the setup script only).
func TestExecutorProfileEnvForEnvironment_ResolvesValuesAndSecrets(t *testing.T) {
	reader := &fakeExecutorProfileReader{
		env: &models.TaskEnvironment{ID: "env-1", ExecutorProfileID: "prof-1"},
		profile: &models.ExecutorProfile{
			ID: "prof-1",
			EnvVars: []models.ProfileEnvVar{
				{Key: "PLAIN", Value: "plain-value"},
				{Key: "FONTAWESOME_NPM_AUTH_TOKEN", SecretID: "sec-npm"},
			},
		},
	}
	m := newExecutorProfileEnvManager(t, reader)

	got := m.ExecutorProfileEnvForEnvironment(context.Background(), "env-1")

	if got["PLAIN"] != "plain-value" {
		t.Fatalf("PLAIN = %q, want literal profile value", got["PLAIN"])
	}
	if got["FONTAWESOME_NPM_AUTH_TOKEN"] != "fa-secret-value" {
		t.Fatalf("FONTAWESOME_NPM_AUTH_TOKEN = %q, want revealed secret", got["FONTAWESOME_NPM_AUTH_TOKEN"])
	}
	if len(reader.profileArgs) != 1 || reader.profileArgs[0] != "prof-1" {
		t.Fatalf("profile lookups = %v, want [prof-1]", reader.profileArgs)
	}
}

func TestExecutorProfileEnvForEnvironment_EmptyCases(t *testing.T) {
	tests := []struct {
		name   string
		envID  string
		reader ExecutorProfileReader
	}{
		{name: "no environment id", envID: "", reader: &fakeExecutorProfileReader{}},
		{name: "no reader wired", envID: "env-1", reader: nil},
		{
			name:   "environment lookup fails",
			envID:  "env-1",
			reader: &fakeExecutorProfileReader{envErr: errors.New("boom")},
		},
		{
			name:   "environment has no executor profile",
			envID:  "env-1",
			reader: &fakeExecutorProfileReader{env: &models.TaskEnvironment{ID: "env-1"}},
		},
		{
			name:  "profile lookup fails",
			envID: "env-1",
			reader: &fakeExecutorProfileReader{
				env:        &models.TaskEnvironment{ID: "env-1", ExecutorProfileID: "prof-1"},
				profileErr: errors.New("boom"),
			},
		},
		{
			name:  "profile has no env vars",
			envID: "env-1",
			reader: &fakeExecutorProfileReader{
				env:     &models.TaskEnvironment{ID: "env-1", ExecutorProfileID: "prof-1"},
				profile: &models.ExecutorProfile{ID: "prof-1"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newExecutorProfileEnvManager(t, tt.reader)
			if got := m.ExecutorProfileEnvForEnvironment(context.Background(), tt.envID); len(got) != 0 {
				t.Fatalf("got %#v, want empty", got)
			}
		})
	}
}
