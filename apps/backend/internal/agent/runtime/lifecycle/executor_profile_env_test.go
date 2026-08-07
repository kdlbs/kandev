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
	session     *models.TaskSession
	sessionErr  error
	env         *models.TaskEnvironment
	envErr      error
	profiles    map[string]*models.ExecutorProfile
	profileErr  error
	profileArgs []string
}

func (f *fakeExecutorProfileReader) GetTaskSession(_ context.Context, _ string) (*models.TaskSession, error) {
	return f.session, f.sessionErr
}

func (*fakeExecutorProfileReader) HasActiveTaskResourceCleanupJob(context.Context, string) (bool, error) {
	return false, nil
}

func (f *fakeExecutorProfileReader) GetTaskEnvironment(_ context.Context, _ string) (*models.TaskEnvironment, error) {
	return f.env, f.envErr
}

func (f *fakeExecutorProfileReader) GetExecutorProfile(_ context.Context, id string) (*models.ExecutorProfile, error) {
	f.profileArgs = append(f.profileArgs, id)
	if f.profileErr != nil {
		return nil, f.profileErr
	}
	return f.profiles[id], nil
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
func TestExecutorProfileEnvForSession_ResolvesValuesAndSecrets(t *testing.T) {
	reader := &fakeExecutorProfileReader{
		session: &models.TaskSession{ID: "session-1", ExecutorProfileID: "prof-1"},
		env:     &models.TaskEnvironment{ID: "env-1", ExecutorProfileID: "prof-1"},
		profiles: map[string]*models.ExecutorProfile{
			"prof-1": {
				ID: "prof-1",
				EnvVars: []models.ProfileEnvVar{
					{Key: "PLAIN", Value: "plain-value"},
					{Key: "FONTAWESOME_NPM_AUTH_TOKEN", SecretID: "sec-npm"},
				},
			},
		},
	}
	m := newExecutorProfileEnvManager(t, reader)

	got := m.ExecutorProfileEnvForSession(context.Background(), "session-1", "env-1")

	if got["PLAIN"] != "plain-value" {
		t.Fatalf("PLAIN = %q, want literal profile value", got["PLAIN"])
	}
	if got["FONTAWESOME_NPM_AUTH_TOKEN"] != "fa-secret-value" {
		t.Fatalf("FONTAWESOME_NPM_AUTH_TOKEN = %q, want revealed secret", got["FONTAWESOME_NPM_AUTH_TOKEN"])
	}
}

// persistTaskEnvironment's reuse branch never refreshes executor_profile_id, so
// the environment row keeps the first session's profile. A later session that
// picked a different profile must still get *its* env in the terminal, matching
// what buildLaunchAgentRequest gave the agent.
func TestExecutorProfileEnvForSession_PrefersSessionProfileOverStaleEnvironmentRow(t *testing.T) {
	reader := &fakeExecutorProfileReader{
		session: &models.TaskSession{ID: "session-2", ExecutorProfileID: "prof-current"},
		env:     &models.TaskEnvironment{ID: "env-1", ExecutorProfileID: "prof-stale"},
		profiles: map[string]*models.ExecutorProfile{
			"prof-current": {ID: "prof-current", EnvVars: []models.ProfileEnvVar{{Key: "TOKEN", Value: "current"}}},
			"prof-stale":   {ID: "prof-stale", EnvVars: []models.ProfileEnvVar{{Key: "TOKEN", Value: "stale"}}},
		},
	}
	m := newExecutorProfileEnvManager(t, reader)

	got := m.ExecutorProfileEnvForSession(context.Background(), "session-2", "env-1")

	if got["TOKEN"] != "current" {
		t.Fatalf("TOKEN = %q, want the session's profile value, not the stale environment row", got["TOKEN"])
	}
	if len(reader.profileArgs) != 1 || reader.profileArgs[0] != "prof-current" {
		t.Fatalf("profile lookups = %v, want [prof-current]", reader.profileArgs)
	}
}

// Sessions predating the executor_profile_id column (or launched without one)
// still resolve through the environment row.
func TestExecutorProfileEnvForSession_FallsBackToEnvironmentRow(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		reader    *fakeExecutorProfileReader
	}{
		{
			name:      "session has no executor profile",
			sessionID: "session-no-profile",
			reader: &fakeExecutorProfileReader{
				session: &models.TaskSession{ID: "session-no-profile"},
				env:     &models.TaskEnvironment{ID: "env-1", ExecutorProfileID: "prof-env"},
			},
		},
		{
			name:      "session lookup fails",
			sessionID: "session-error",
			reader: &fakeExecutorProfileReader{
				sessionErr: errors.New("boom"),
				env:        &models.TaskEnvironment{ID: "env-1", ExecutorProfileID: "prof-env"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.reader.profiles = map[string]*models.ExecutorProfile{
				"prof-env": {ID: "prof-env", EnvVars: []models.ProfileEnvVar{{Key: "TOKEN", Value: "from-env-row"}}},
			}
			m := newExecutorProfileEnvManager(t, tt.reader)

			got := m.ExecutorProfileEnvForSession(context.Background(), tt.sessionID, "env-1")

			if got["TOKEN"] != "from-env-row" {
				t.Fatalf("TOKEN = %q, want environment-row fallback", got["TOKEN"])
			}
		})
	}
}

func TestExecutorProfileEnvForSession_EmptyCases(t *testing.T) {
	tests := []struct {
		name   string
		envID  string
		reader ExecutorProfileReader
	}{
		{name: "no session or environment id", envID: "", reader: &fakeExecutorProfileReader{}},
		{name: "no reader wired", envID: "env-1", reader: nil},
		{
			name:   "environment lookup fails",
			envID:  "env-1",
			reader: &fakeExecutorProfileReader{envErr: errors.New("boom")},
		},
		{
			name:   "neither session nor environment has a profile",
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
				env:      &models.TaskEnvironment{ID: "env-1", ExecutorProfileID: "prof-1"},
				profiles: map[string]*models.ExecutorProfile{"prof-1": {ID: "prof-1"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newExecutorProfileEnvManager(t, tt.reader)
			if got := m.ExecutorProfileEnvForSession(context.Background(), "", tt.envID); len(got) != 0 {
				t.Fatalf("got %#v, want empty", got)
			}
		})
	}
}
