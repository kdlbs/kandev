package launcher

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/config"
)

func TestLauncherBootstrapUsesYAMLValues(t *testing.T) {
	clearLauncherConfigurationEnvironment(t)
	dir := t.TempDir()
	t.Chdir(dir)
	dbPath := filepath.Join(dir, "state", "custom.db")
	configPath := filepath.Join(dir, "config.yaml")
	writeLauncherConfig(t, configPath, "homeDir: /srv/kandev\nserver:\n  host: 127.0.0.1\n  port: 4321\ndatabase:\n  path: "+dbPath+"\nlogging:\n  level: error\nlauncher:\n  webPort: 4322\n  healthTimeoutMs: 12345\n  noBrowser: true\n")

	cfg, err := loadBootstrapConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Source.FilePath != configPath {
		t.Fatalf("selected config = %q, want %q", cfg.Source.FilePath, configPath)
	}
	if got := resolveHomeDirForConfig(cfg); got != "/srv/kandev" {
		t.Fatalf("home directory = %q, want /srv/kandev", got)
	}
	if got := resolveDatabasePathForConfig(cfg); got != dbPath {
		t.Fatalf("database path = %q, want %q", got, dbPath)
	}
	if got, err := resolvePortsForConfig(Options{}, cfg); err != nil || got != 4321 {
		t.Fatalf("backend port = %d, %v, want 4321", got, err)
	}
	if got, err := resolveWebPortForConfig(Options{}, cfg); err != nil || got != 4322 {
		t.Fatalf("web port = %d, %v, want 4322", got, err)
	}
	if got := resolveLogLevelForConfig(Options{}, cfg); got != "error" {
		t.Fatalf("log level = %q, want error", got)
	}
	if got := healthTimeoutForConfig(45000, cfg); got != 12345*time.Millisecond {
		t.Fatalf("health timeout = %s, want 12.345s", got)
	}
	if shouldOpenBrowser(Options{}, cfg) {
		t.Fatal("YAML launcher.noBrowser did not suppress the browser")
	}
}

func TestLauncherBootstrapEnvironmentOverridesYAMLAndFlagsOverrideEnvironment(t *testing.T) {
	clearLauncherConfigurationEnvironment(t)
	dir := t.TempDir()
	t.Chdir(dir)
	writeLauncherConfig(t, filepath.Join(dir, "config.yaml"), "server:\n  port: 4321\nlogging:\n  level: error\nlauncher:\n  webPort: 4322\n  healthTimeoutMs: 12345\n  noBrowser: false\n")
	t.Setenv("KANDEV_BACKEND_PORT", "4323")
	t.Setenv("KANDEV_PORT", "4324")
	t.Setenv("KANDEV_WEB_PORT", "4325")
	t.Setenv("KANDEV_LOG_LEVEL", "warn")
	t.Setenv("KANDEV_HEALTH_TIMEOUT_MS", "23456")
	t.Setenv("KANDEV_NO_BROWSER", "true")

	cfg, err := loadBootstrapConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got, err := resolvePortsForConfig(Options{}, cfg); err != nil || got != 4323 {
		t.Fatalf("environment backend port = %d, %v, want 4323", got, err)
	}
	if got, err := resolvePortsForConfig(Options{BackendPort: 4326}, cfg); err != nil || got != 4326 {
		t.Fatalf("flag backend port = %d, %v, want 4326", got, err)
	}
	if got, err := resolveWebPortForConfig(Options{}, cfg); err != nil || got != 4325 {
		t.Fatalf("environment web port = %d, %v, want 4325", got, err)
	}
	if got, err := resolveWebPortForConfig(Options{WebPort: 4327}, cfg); err != nil || got != 4327 {
		t.Fatalf("flag web port = %d, %v, want 4327", got, err)
	}
	if got := resolveLogLevelForConfig(Options{}, cfg); got != "warn" {
		t.Fatalf("environment log level = %q, want warn", got)
	}
	if got := healthTimeoutForConfig(45000, cfg); got != 23456*time.Millisecond {
		t.Fatalf("environment health timeout = %s, want 23.456s", got)
	}
	if shouldOpenBrowser(Options{}, cfg) {
		t.Fatal("environment no-browser override did not suppress the browser")
	}
}

func TestLauncherBackendEnvironmentHandsOffSelectedConfigFile(t *testing.T) {
	clearLauncherConfigurationEnvironment(t)
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	writeLauncherConfig(t, configPath, "database:\n  path: /srv/custom.db\n")
	cfg, err := config.LoadWithPath(configPath)
	if err != nil {
		t.Fatal(err)
	}

	env := backendEnvForConfig(
		portConfig{BackendPort: 4321, AgentctlPort: 4322},
		"info",
		"warn",
		false,
		"health-token",
		nil,
		cfg,
	)
	if got := envValue(env, config.InternalConfigFileEnv); got != configPath {
		t.Fatalf("selected config handoff = %q, want %q", got, configPath)
	}
	if got := envValue(env, "KANDEV_DATABASE_PATH"); got != "" {
		t.Fatalf("YAML database path was copied into public environment as %q", got)
	}
}

func TestBackendEnvDoesNotSynthesizeDefaultDatabasePath(t *testing.T) {
	clearLauncherConfigurationEnvironment(t)
	unsetEnvForTest(t, "KANDEV_DATABASE_PATH")

	cfg := &config.Config{
		HomeDir: t.TempDir(),
		Source: config.ConfigSource{Values: map[string]config.SettingSource{
			"database.path": config.SourceDefault,
		}},
	}
	env := backendEnvForConfig(
		portConfig{BackendPort: 4321, AgentctlPort: 4322},
		"info",
		"warn",
		false,
		"health-token",
		nil,
		cfg,
	)
	for _, item := range env {
		if strings.HasPrefix(item, "KANDEV_DATABASE_PATH=") {
			t.Fatalf("default database path was synthesized into child environment: %q", item)
		}
	}
}

func TestBackendEnvPreservesExplicitEnvironmentDatabasePath(t *testing.T) {
	clearLauncherConfigurationEnvironment(t)
	explicitPath := filepath.Join(t.TempDir(), "custom.db")
	cfg := &config.Config{
		Database: config.DatabaseConfig{Path: explicitPath},
		Source: config.ConfigSource{Values: map[string]config.SettingSource{
			"database.path": config.SourceEnvironment,
		}},
	}

	env := backendEnvForConfig(
		portConfig{BackendPort: 4321, AgentctlPort: 4322},
		"info",
		"warn",
		false,
		"health-token",
		nil,
		cfg,
	)
	if got := envValue(env, "KANDEV_DATABASE_PATH"); got != explicitPath {
		t.Fatalf("explicit database path = %q, want %q", got, explicitPath)
	}
}

func TestLauncherSelectedConfigHandoffSurvivesWorkingDirectoryChange(t *testing.T) {
	clearLauncherConfigurationEnvironment(t)
	sourceDir := t.TempDir()
	otherDir := t.TempDir()
	configPath := filepath.Join(sourceDir, "config.yaml")
	writeLauncherConfig(t, configPath, "server:\n  port: 4321\n")
	t.Setenv(config.InternalConfigFileEnv, configPath)
	t.Chdir(otherDir)

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Source.FilePath != configPath {
		t.Fatalf("child selected config = %q, want %q", cfg.Source.FilePath, configPath)
	}
	if cfg.Server.Port != 4321 {
		t.Fatalf("child server port = %d, want 4321", cfg.Server.Port)
	}
}

func TestDevBootstrapUsesConfiguredWebPort(t *testing.T) {
	clearLauncherConfigurationEnvironment(t)
	repo := makeRepoTree(t)
	t.Chdir(repo)
	writeLauncherConfig(t, filepath.Join(repo, "config.yaml"), "launcher:\n  webPort: 4328\n")

	cfg, code := devLaunchConfigFor(Options{Command: CommandDev})
	if code != 0 {
		t.Fatalf("devLaunchConfigFor() code = %d, want 0", code)
	}
	if cfg.ports.WebPort != 4328 {
		t.Fatalf("dev web port = %d, want 4328", cfg.ports.WebPort)
	}
}

func TestLauncherInvalidBootstrapReportsSelectedFile(t *testing.T) {
	clearLauncherConfigurationEnvironment(t)
	dir := t.TempDir()
	t.Chdir(dir)
	configPath := filepath.Join(dir, "config.yaml")
	writeLauncherConfig(t, configPath, "launcher:\n  healthTimeoutMs: invalid\n")

	_, err := loadBootstrapConfig()
	if err == nil {
		t.Fatal("loadBootstrapConfig() succeeded for invalid launcher configuration")
	}
	if !strings.Contains(err.Error(), configPath) {
		t.Fatalf("error = %v, want selected file %q", err, configPath)
	}
}

func clearLauncherConfigurationEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"KANDEV_HOME_DIR",
		"KANDEV_SERVER_HOST",
		"KANDEV_DATABASE_PATH",
		"KANDEV_LOG_LEVEL",
		"KANDEV_HEALTH_TIMEOUT_MS",
		"KANDEV_NO_BROWSER",
		config.InternalConfigFileEnv,
	} {
		t.Setenv(name, "")
	}
	for _, name := range []string{"KANDEV_SERVER_PORT", "KANDEV_BACKEND_PORT", "KANDEV_PORT", "KANDEV_WEB_PORT"} {
		unsetEnvForTest(t, name)
	}
}

func writeLauncherConfig(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}
