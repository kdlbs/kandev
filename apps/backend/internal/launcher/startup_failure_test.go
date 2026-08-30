package launcher

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/config"
)

func TestStartupFailureFormattingIncludesSafeEvidenceAndRecovery(t *testing.T) {
	const healthToken = "do-not-print-this-token"
	endpoints := backendEndpointSet{
		bindHosts: []string{"192.0.2.10"},
		healthTargets: []string{
			"http://192.0.2.10:38429/health",
		},
		accessURL: "http://192.0.2.10:38429",
	}
	err := &backendHealthError{
		Class:       healthFailureForeign,
		EndpointSet: endpoints,
		Observations: []healthObservation{{
			URL:        endpoints.healthTargets[0],
			Outcome:    healthOutcomeForeignProcess,
			SafeDetail: "missing or mismatched launcher token",
		}},
	}
	cfg := &config.Config{
		Server: config.ServerConfig{Host: "192.0.2.10"},
		Source: config.ConfigSource{FilePath: "/etc/kandev/config.yaml"},
	}

	got := formatStartupFailure(err, endpoints, cfg, "/srv/kandev/logs/backend-logs.log", true)
	for _, want := range []string{
		"startup failed",
		"foreign process",
		"effective bind addresses: 192.0.2.10",
		"http://192.0.2.10:38429/health: foreign_process",
		"configuration file: /etc/kandev/config.yaml",
		"backend log: /srv/kandev/logs/backend-logs.log",
		"launcher stopped the backend after readiness failed",
		"free the selected port or choose another backend port",
		startupTroubleshootingURL,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("formatted failure missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, healthToken) || strings.Contains(got, "X-Kandev-Desktop-Health-Token") {
		t.Fatalf("formatted failure exposed health-token material:\n%s", got)
	}
}

func TestStartupFailureReportsEffectiveServerHostSource(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("server:\n  host: 192.0.2.10\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("KANDEV_SERVER_HOST", "127.0.0.1")
	cfg, err := config.LoadWithPath(dir)
	if err != nil {
		t.Fatalf("LoadWithPath() = %v", err)
	}
	if cfg.Server.Host != "127.0.0.1" {
		t.Fatalf("effective server.host = %q, want environment override", cfg.Server.Host)
	}
	if got := cfg.SourceFor("server.host"); got != config.SourceEnvironment {
		t.Fatalf("server.host source = %q, want environment", got)
	}

	endpoints := backendEndpointSet{
		bindHosts:     []string{"127.0.0.1"},
		healthTargets: []string{"http://127.0.0.1:38429/health"},
		accessURL:     "http://localhost:38429",
	}
	err = &backendHealthError{
		Class:       healthFailureUnreachable,
		EndpointSet: endpoints,
	}

	got := formatStartupFailure(err, endpoints, cfg, "/srv/kandev/logs/backend-logs.log", true)
	for _, want := range []string{
		"configuration file: " + filepath.Join(dir, "config.yaml"),
		"server.host source: environment (KANDEV_SERVER_HOST)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("formatted failure missing %q:\n%s", want, got)
		}
	}
}

func TestBackendLogPathForDevConfigUsesEffectiveBackendHome(t *testing.T) {
	repo := t.TempDir()
	t.Setenv("KANDEV_TASK_ID", "")
	t.Setenv("KANDEV_HOME_DIR", "")
	t.Setenv("KANDEV_DATABASE_PATH", "")
	defaultExtra := devExtraForTest(t, repo)
	got := backendLogPathForDevConfig(devLaunchConfig{startup: nil, extra: defaultExtra})
	want := filepath.Join(repo, ".kandev-dev", "logs", "backend-logs.log")
	if got != want {
		t.Fatalf("default dev backend log = %q, want %q", got, want)
	}

	t.Setenv("HOME", t.TempDir())
	databasePath := filepath.Join(t.TempDir(), "kandev.db")
	t.Setenv("KANDEV_DATABASE_PATH", databasePath)
	overrideExtra := devExtraForTest(t, repo)
	got = backendLogPathForDevConfig(devLaunchConfig{startup: nil, extra: overrideExtra})
	want = filepath.Join(repo, ".kandev-dev", "logs", "backend-logs.log")
	if got != want {
		t.Fatalf("explicit database dev backend log = %q, want repo-local %q", got, want)
	}
}

func devExtraForTest(t *testing.T, repo string) []string {
	t.Helper()
	_, extra := resolveDevBackendEnv(repo)
	return extra
}
