package gitlab

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
)

// newTestLogger builds a quiet logger for tests that exercise the factory.
func newTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	return log
}

type stubSecrets struct {
	items   []*SecretListItem
	tokens  map[string]string
	listErr error
}

func (s *stubSecrets) List(context.Context) ([]*SecretListItem, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.items, nil
}

func (s *stubSecrets) Reveal(_ context.Context, id string) (string, error) {
	return s.tokens[id], nil
}

func TestNewClient_MockEnvShortcircuits(t *testing.T) {
	t.Setenv("KANDEV_MOCK_GITLAB", "true")
	log := newTestLogger(t)
	c, method, err := NewClient(context.Background(), "", nil, log)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if _, ok := c.(*MockClient); !ok {
		t.Fatalf("got %T, want *MockClient", c)
	}
	if method != "mock" {
		t.Errorf("method = %q, want mock", method)
	}
}

func TestNewClient_EnvTokenBeatsSecrets(t *testing.T) {
	t.Setenv("KANDEV_MOCK_GITLAB", "")
	t.Setenv("GITLAB_TOKEN", "from-env")
	log := newTestLogger(t)

	secrets := &stubSecrets{
		items:  []*SecretListItem{{ID: "s1", Name: "GITLAB_TOKEN", HasValue: true}},
		tokens: map[string]string{"s1": "from-secret"},
	}
	c, method, err := NewClient(context.Background(), "", secrets, log)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	pat, ok := c.(*PATClient)
	if !ok {
		// glab might be installed on the test runner — accept GLab as well
		// only when the env-var path is unreachable. The branch under test
		// here is env-var, so explicitly assert PATClient.
		if _, isGLab := c.(*GLabClient); isGLab {
			t.Skip("glab CLI present on test host; env-var branch is shadowed")
		}
		t.Fatalf("got %T, want *PATClient", c)
	}
	if pat.token != "from-env" {
		t.Errorf("token = %q, want from-env", pat.token)
	}
	if method != AuthMethodPAT {
		t.Errorf("method = %q, want pat", method)
	}
}

func TestNewClient_FallsBackToSecrets(t *testing.T) {
	t.Setenv("KANDEV_MOCK_GITLAB", "")
	t.Setenv("GITLAB_TOKEN", "")
	log := newTestLogger(t)

	secrets := &stubSecrets{
		items:  []*SecretListItem{{ID: "s1", Name: "GITLAB_TOKEN", HasValue: true}},
		tokens: map[string]string{"s1": "from-secret"},
	}
	c, _, err := NewClient(context.Background(), "https://gitlab.example.com", secrets, log)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if _, ok := c.(*PATClient); !ok {
		if _, isGLab := c.(*GLabClient); isGLab {
			t.Skip("glab CLI present on test host; secret-store branch is shadowed")
		}
		t.Fatalf("got %T, want *PATClient", c)
	}
}

func TestNewClient_NoopWhenUnconfigured(t *testing.T) {
	t.Setenv("KANDEV_MOCK_GITLAB", "")
	t.Setenv("GITLAB_TOKEN", "")
	log := newTestLogger(t)

	c, method, err := NewClient(context.Background(), "", nil, log)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if _, ok := c.(*NoopClient); !ok {
		if _, isGLab := c.(*GLabClient); isGLab {
			t.Skip("glab CLI present on test host; noop branch is shadowed")
		}
		t.Fatalf("got %T, want *NoopClient", c)
	}
	if method != AuthMethodNone {
		t.Errorf("method = %q, want none", method)
	}
}

func TestNewClient_AcceptsLowercaseSecretName(t *testing.T) {
	t.Setenv("KANDEV_MOCK_GITLAB", "")
	t.Setenv("GITLAB_TOKEN", "")
	log := newTestLogger(t)

	secrets := &stubSecrets{
		items:  []*SecretListItem{{ID: "s2", Name: "gitlab_token", HasValue: true}},
		tokens: map[string]string{"s2": "lc"},
	}
	c, _, err := NewClient(context.Background(), "", secrets, log)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if _, ok := c.(*PATClient); !ok {
		if _, isGLab := c.(*GLabClient); isGLab {
			t.Skip("glab CLI present on test host; secret-store branch is shadowed")
		}
		t.Fatalf("got %T, want *PATClient", c)
	}
}

func TestNewClient_IgnoresSecretsWithoutValue(t *testing.T) {
	t.Setenv("KANDEV_MOCK_GITLAB", "")
	t.Setenv("GITLAB_TOKEN", "")
	log := newTestLogger(t)

	secrets := &stubSecrets{
		items: []*SecretListItem{{ID: "s3", Name: "GITLAB_TOKEN", HasValue: false}},
	}
	c, _, err := NewClient(context.Background(), "", secrets, log)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if _, ok := c.(*NoopClient); !ok {
		if _, isGLab := c.(*GLabClient); isGLab {
			t.Skip("glab CLI present on test host; noop branch is shadowed")
		}
		t.Fatalf("got %T, want *NoopClient (HasValue=false should be skipped)", c)
	}
}
