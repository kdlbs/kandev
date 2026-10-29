package service

import (
	"context"
	"errors"
	"testing"
)

func TestParseRemoteRepositoryURLRefusesLoopbackOutsideE2E(t *testing.T) {
	t.Setenv(e2eMockEnv, "")
	for _, raw := range []string{"file:///tmp/repos/widgets.git", "http://127.0.0.1:9418/widgets.git", "https://localhost/widgets"} {
		if _, _, _, _, err := parseRemoteRepositoryURL(raw, ""); err == nil {
			t.Errorf("parseRemoteRepositoryURL(%q) accepted a local remote outside the e2e profile", raw)
		}
	}
}

func TestParseRemoteRepositoryURLAcceptsLoopbackUnderE2E(t *testing.T) {
	t.Setenv(e2eMockEnv, "true")
	for raw, wantName := range map[string]string{
		"file:///tmp/repos/widgets.git":        "widgets",
		"http://127.0.0.1:9418/srv/gadgets":    "gadgets",
		"https://localhost/team/sprockets.git": "sprockets",
	} {
		provider, owner, name, canonical, err := parseRemoteRepositoryURL(raw, "")
		if err != nil {
			t.Fatalf("parseRemoteRepositoryURL(%q): %v", raw, err)
		}
		if provider != providerE2ELocal || owner != e2eLocalOwner || name != wantName || canonical != raw {
			t.Errorf("parseRemoteRepositoryURL(%q) = %q %q %q %q", raw, provider, owner, name, canonical)
		}
	}
}

func TestParseRemoteRepositoryURLKeepsAllowlistUnderE2E(t *testing.T) {
	t.Setenv(e2eMockEnv, "true")
	if _, _, _, _, err := parseRemoteRepositoryURL("https://git.example.invalid/acme/widgets.git", ""); err == nil {
		t.Fatal("a non-loopback unknown host must still be refused under the e2e profile")
	}
	if provider, _, _, _, err := parseRemoteRepositoryURL("https://github.com/acme/widgets.git", ""); err != nil || provider != providerGitHub {
		t.Fatalf("github remote under e2e = %q, %v", provider, err)
	}
}

func TestPluginExecutorRepositoryCompatibilityLoopbackRemote(t *testing.T) {
	svc := &Service{}
	input := TaskRepositoryInput{RemoteURL: "file:///tmp/repos/widgets.git"}

	t.Setenv(e2eMockEnv, "")
	if err := svc.validatePluginExecutorRepositoryCompatibility(context.Background(), input); !errors.Is(err, ErrInvalidExecutorConfig) {
		t.Fatalf("production error = %v, want ErrInvalidExecutorConfig", err)
	}

	t.Setenv(e2eMockEnv, "true")
	if err := svc.validatePluginExecutorRepositoryCompatibility(context.Background(), input); err != nil {
		t.Fatalf("e2e error = %v, want nil", err)
	}
	if err := svc.validatePluginExecutorRepositoryCompatibility(context.Background(), TaskRepositoryInput{LocalPath: "/tmp/checkout"}); !errors.Is(err, ErrInvalidExecutorConfig) {
		t.Fatalf("a host checkout path must stay refused under e2e, got %v", err)
	}
	if err := svc.validatePluginExecutorRepositoryCompatibility(context.Background(), TaskRepositoryInput{RemoteURL: "/tmp/checkout"}); !errors.Is(err, ErrInvalidExecutorConfig) {
		t.Fatalf("a bare filesystem path must stay refused under e2e, got %v", err)
	}
}
