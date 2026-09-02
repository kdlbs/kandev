package config

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestConfigSourceLoadsStableYAMLFields(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	contents := `server:
  trustedProxies:
    - 10.0.0.0/8
tasks:
  preparationTimeout: 17m
credentials:
  file: /run/secrets/credentials
limits:
  ghMaxConcurrent: 11
  gitMaxConcurrent: 13
  lspMaxConnections: 21
messageQueue:
  maxPerSession: 14
agentctl:
  idleTimeout: 2h
  idleReaperInterval: 3m
  notificationQueueCapacity: 2048
planning:
  coalesceWindowMs: 2400
office:
  schedulerTickMs: 7000
observability:
  otlpEndpoint: https://otel.example.test/v1/traces
launcher:
  webPort: 37430
  healthTimeoutMs: 12345
  noBrowser: true
`
	if err := os.WriteFile(configFile, []byte(contents), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadWithPath(dir)
	if err != nil {
		t.Fatalf("LoadWithPath: %v", err)
	}

	if got := nestedField(t, cfg, "Server", "TrustedProxies"); !reflect.DeepEqual(got.Interface(), []string{"10.0.0.0/8"}) {
		t.Fatalf("server.trustedProxies = %#v", got.Interface())
	}
	if got := nestedField(t, cfg, "Tasks", "PreparationTimeout"); got.Interface() != 17*time.Minute {
		t.Fatalf("tasks.preparationTimeout = %#v", got.Interface())
	}
	if got := nestedField(t, cfg, "Credentials", "File"); got.String() != "/run/secrets/credentials" {
		t.Fatalf("credentials.file = %q", got.String())
	}
	if got := nestedField(t, cfg, "Limits", "GHMaxConcurrent"); got.Int() != 11 {
		t.Fatalf("limits.ghMaxConcurrent = %d", got.Int())
	}
	if got := nestedField(t, cfg, "Limits", "GitMaxConcurrent"); got.Int() != 13 {
		t.Fatalf("limits.gitMaxConcurrent = %d", got.Int())
	}
	if got := nestedField(t, cfg, "Limits", "LSPMaxConnections"); got.Int() != 21 {
		t.Fatalf("limits.lspMaxConnections = %d", got.Int())
	}
	if got := nestedField(t, cfg, "Limits", "LSPMaxServers"); got.Int() != 21 {
		t.Fatalf("limits.lspMaxServers legacy fallback = %d", got.Int())
	}
	if got := nestedField(t, cfg, "MessageQueue", "MaxPerSession"); got.Int() != 14 {
		t.Fatalf("messageQueue.maxPerSession = %d", got.Int())
	}
	if got := nestedField(t, cfg, "Agentctl", "IdleTimeout"); got.Interface() != 2*time.Hour {
		t.Fatalf("agentctl.idleTimeout = %#v", got.Interface())
	}
	if got := nestedField(t, cfg, "Agentctl", "IdleReaperInterval"); got.Interface() != 3*time.Minute {
		t.Fatalf("agentctl.idleReaperInterval = %#v", got.Interface())
	}
	if got := nestedField(t, cfg, "Agentctl", "NotificationQueueCapacity"); got.Int() != 2048 {
		t.Fatalf("agentctl.notificationQueueCapacity = %d", got.Int())
	}
	if got := nestedField(t, cfg, "Planning", "CoalesceWindowMs"); got.Int() != 2400 {
		t.Fatalf("planning.coalesceWindowMs = %d", got.Int())
	}
	if got := nestedField(t, cfg, "Office", "SchedulerTickMs"); got.Int() != 7000 {
		t.Fatalf("office.schedulerTickMs = %d", got.Int())
	}
	if got := nestedField(t, cfg, "Observability", "OTLPEndpoint"); got.String() != "https://otel.example.test/v1/traces" {
		t.Fatalf("observability.otlpEndpoint = %q", got.String())
	}
	if got := nestedField(t, cfg, "Launcher", "WebPort"); got.Int() != 37430 {
		t.Fatalf("launcher.webPort = %d", got.Int())
	}
	if got := nestedField(t, cfg, "Launcher", "HealthTimeoutMs"); got.Int() != 12345 {
		t.Fatalf("launcher.healthTimeoutMs = %d", got.Int())
	}
	if got := nestedField(t, cfg, "Launcher", "NoBrowser"); !got.Bool() {
		t.Fatal("launcher.noBrowser = false, want true")
	}
}

func TestHomeConfigDiscovery(t *testing.T) {
	t.Setenv("KANDEV_SERVER_PORT", "")
	homeDir := t.TempDir()
	t.Chdir(t.TempDir())
	if err := os.WriteFile(filepath.Join(homeDir, "config.yaml"), []byte("server:\n  port: 40123\n"), 0o600); err != nil {
		t.Fatalf("write home config: %v", err)
	}
	t.Setenv("KANDEV_HOME_DIR", homeDir)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Port != 40123 {
		t.Fatalf("server.port = %d, want 40123", cfg.Server.Port)
	}
}

func TestHomeConfigCannotRelocateItself(t *testing.T) {
	t.Setenv("KANDEV_SERVER_PORT", "")
	homeDir := t.TempDir()
	t.Chdir(t.TempDir())
	if err := os.WriteFile(filepath.Join(homeDir, "config.yaml"), []byte("homeDir: /different\n"), 0o600); err != nil {
		t.Fatalf("write home config: %v", err)
	}
	t.Setenv("KANDEV_HOME_DIR", homeDir)

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "homeDir") || !strings.Contains(err.Error(), homeDir) {
		t.Fatalf("Load error = %v, want a homeDir relocation error naming %s", err, homeDir)
	}
}

func TestConfigSourceMetadataReportsSelectedFileAndYAMLValues(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("server:\n  port: 40124\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("KANDEV_SERVER_PORT", "40125")

	cfg, err := LoadWithPath(dir)
	if err != nil {
		t.Fatalf("LoadWithPath: %v", err)
	}
	source := reflect.ValueOf(cfg).Elem().FieldByName("Source")
	if !source.IsValid() {
		t.Fatal("Config.Source is missing")
	}
	file := source.FieldByName("FilePath")
	if !file.IsValid() || file.String() != filepath.Join(dir, "config.yaml") {
		t.Fatalf("selected config file = %#v, want %s", file.Interface(), filepath.Join(dir, "config.yaml"))
	}
	values := source.FieldByName("Values")
	if !values.IsValid() {
		t.Fatal("Config.Source.Values is missing")
	}
	got := values.MapIndex(reflect.ValueOf("server.port"))
	if !got.IsValid() || got.String() != "environment" {
		t.Fatalf("server.port source = %#v, want environment", got.Interface())
	}
}

func TestConfigPrecedenceEnvironmentOverYAML(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("messageQueue:\n  maxPerSession: 4\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("KANDEV_QUEUE_MAX_PER_SESSION", "9")
	t.Setenv("KANDEV_SERVER_PORT", "")

	cfg, err := LoadWithPath(dir)
	if err != nil {
		t.Fatalf("LoadWithPath: %v", err)
	}
	if cfg.MessageQueue.MaxPerSession != 9 {
		t.Fatalf("messageQueue.maxPerSession = %d, want 9", cfg.MessageQueue.MaxPerSession)
	}
	if got := cfg.SourceFor("messageQueue.maxPerSession"); got != SourceEnvironment {
		t.Fatalf("messageQueue.maxPerSession source = %q, want %q", got, SourceEnvironment)
	}
}

func TestTaskLSPServerCapacityUsesCanonicalStartupConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(dir, "config.yaml"),
		[]byte("limits:\n  lspMaxServers: 3\n"),
		0o600,
	); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("KANDEV_LSP_MAX_SERVERS", "")
	t.Setenv("KANDEV_LSP_MAX_CONNECTIONS", "")

	cfg, err := LoadWithPath(dir)
	if err != nil {
		t.Fatalf("LoadWithPath: %v", err)
	}
	if got := nestedField(t, cfg, "Limits", "LSPMaxServers"); got.Int() != 3 {
		t.Fatalf("limits.lspMaxServers = %d, want 3", got.Int())
	}
	if got := cfg.SourceFor("limits.lspMaxServers"); got != SourceConfiguration {
		t.Fatalf("limits.lspMaxServers source = %q, want %q", got, SourceConfiguration)
	}
}

func TestTaskLSPServerCapacityEnvironmentPrefersCanonicalAlias(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(dir, "config.yaml"),
		[]byte("limits:\n  lspMaxServers: 3\n"),
		0o600,
	); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("KANDEV_LSP_MAX_SERVERS", "5")
	t.Setenv("KANDEV_LSP_MAX_CONNECTIONS", "7")

	cfg, err := LoadWithPath(dir)
	if err != nil {
		t.Fatalf("LoadWithPath: %v", err)
	}
	if cfg.Limits.LSPMaxServers != 5 {
		t.Fatalf("limits.lspMaxServers = %d, want preferred environment value 5", cfg.Limits.LSPMaxServers)
	}
	if got := cfg.SourceFor("limits.lspMaxServers"); got != SourceEnvironment {
		t.Fatalf("limits.lspMaxServers source = %q, want %q", got, SourceEnvironment)
	}
}

func TestTaskLSPServerCapacityFallsBackToLegacyEnvironmentAlias(t *testing.T) {
	t.Setenv("KANDEV_LSP_MAX_SERVERS", "")
	t.Setenv("KANDEV_LSP_MAX_CONNECTIONS", "7")

	cfg, err := LoadWithPath(t.TempDir())
	if err != nil {
		t.Fatalf("LoadWithPath: %v", err)
	}
	if cfg.Limits.LSPMaxServers != 7 {
		t.Fatalf("limits.lspMaxServers = %d, want legacy environment value 7", cfg.Limits.LSPMaxServers)
	}
}

func TestEmptyCredentialsEnvironmentClearsYAML(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("credentials:\n  file: /run/secrets/credentials\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("KANDEV_CREDENTIALS_FILE", "")

	cfg, err := LoadWithPath(dir)
	if err != nil {
		t.Fatalf("LoadWithPath: %v", err)
	}
	if cfg.Credentials.File != "" {
		t.Fatalf("credentials.file = %q, want empty environment override", cfg.Credentials.File)
	}
	if got := cfg.SourceFor("credentials.file"); got != SourceEnvironment {
		t.Fatalf("credentials.file source = %q, want environment", got)
	}
}

func TestRepositoryDiscoveryRootsEnvironmentUsesCommaSeparatedValues(t *testing.T) {
	t.Setenv("KANDEV_REPOSITORYDISCOVERY_ROOTS", "/one,/two")

	cfg, err := LoadWithPath(t.TempDir())
	if err != nil {
		t.Fatalf("LoadWithPath: %v", err)
	}
	if !reflect.DeepEqual(cfg.RepositoryDiscovery.Roots, []string{"/one", "/two"}) {
		t.Fatalf("repositoryDiscovery.roots = %#v, want comma-separated paths", cfg.RepositoryDiscovery.Roots)
	}
}

func TestInvalidYAMLValueNamesSelectedFileAndKey(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configFile, []byte("tasks:\n  preparationTimeout: not-a-duration\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("KANDEV_SERVER_PORT", "")

	_, err := LoadWithPath(dir)
	if err == nil || !strings.Contains(err.Error(), configFile) || !strings.Contains(err.Error(), "preparationTimeout") {
		t.Fatalf("Load error = %v, want selected file and preparationTimeout", err)
	}
}

func TestSecretConfigPermissionsWarningOmitsValue(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission warning is not applicable")
	}
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	secret := "do-not-log-this-secret"
	if err := os.WriteFile(configFile, []byte("auth:\n  jwtSecret: "+secret+"\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("KANDEV_SERVER_PORT", "")

	cfg, err := LoadWithPath(dir)
	if err != nil {
		t.Fatalf("LoadWithPath: %v", err)
	}
	if len(cfg.Source.Warnings) != 1 || !strings.Contains(cfg.Source.Warnings[0], configFile) {
		t.Fatalf("warnings = %#v, want one warning naming %s", cfg.Source.Warnings, configFile)
	}
	if strings.Contains(cfg.Source.Warnings[0], secret) {
		t.Fatalf("warning exposed secret: %q", cfg.Source.Warnings[0])
	}
}

func TestInvalidYAMLRangeNamesSettingAndFile(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configFile, []byte("limits:\n  ghMaxConcurrent: 0\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("KANDEV_SERVER_PORT", "")

	_, err := LoadWithPath(dir)
	if err == nil || !strings.Contains(err.Error(), configFile) || !strings.Contains(err.Error(), "ghMaxConcurrent") {
		t.Fatalf("Load error = %v, want selected file and ghMaxConcurrent", err)
	}
}

func TestHomeConfigDiscoveryOrderUsesOneFirstExistingFile(t *testing.T) {
	workingDir := t.TempDir()
	homeDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workingDir, "config.yaml"), []byte("server:\n  port: 40126\n"), 0o600); err != nil {
		t.Fatalf("write working-directory config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "config.yaml"), []byte("server:\n  port: 40127\n"), 0o600); err != nil {
		t.Fatalf("write home config: %v", err)
	}
	t.Setenv("KANDEV_HOME_DIR", homeDir)
	t.Setenv("KANDEV_SERVER_PORT", "")
	t.Chdir(workingDir)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Port != 40126 || cfg.Source.FilePath != filepath.Join(workingDir, "config.yaml") {
		t.Fatalf("selected config = %q, port = %d; want working-directory file and port 40126", cfg.Source.FilePath, cfg.Server.Port)
	}
}

func TestHomeConfigInvalidFirstDoesNotFallThrough(t *testing.T) {
	workingDir := t.TempDir()
	homeDir := t.TempDir()
	configFile := filepath.Join(workingDir, "config.yaml")
	if err := os.WriteFile(configFile, []byte("limits:\n  ghMaxConcurrent: 0\n"), 0o600); err != nil {
		t.Fatalf("write invalid config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "config.yaml"), []byte("limits:\n  ghMaxConcurrent: 22\n"), 0o600); err != nil {
		t.Fatalf("write home config: %v", err)
	}
	t.Setenv("KANDEV_HOME_DIR", homeDir)
	t.Setenv("KANDEV_SERVER_PORT", "")
	t.Chdir(workingDir)

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), configFile) {
		t.Fatalf("Load error = %v, want invalid first file and no fallback", err)
	}
}

func nestedField(t *testing.T, value any, names ...string) reflect.Value {
	t.Helper()
	current := reflect.ValueOf(value)
	if current.Kind() == reflect.Pointer {
		current = current.Elem()
	}
	for _, name := range names {
		if current.Kind() != reflect.Struct {
			t.Fatalf("cannot find %s in %s", name, current.Type())
		}
		current = current.FieldByName(name)
		if !current.IsValid() {
			t.Fatalf("configuration field %s is missing", strings.Join(names, "."))
		}
	}
	return current
}

func TestHomeConfigDefaultDotDirectory(t *testing.T) {
	userHome := t.TempDir()
	defaultHome := filepath.Join(userHome, ".kandev")
	if err := os.MkdirAll(defaultHome, 0o700); err != nil {
		t.Fatalf("create default home: %v", err)
	}
	configFile := filepath.Join(defaultHome, "config.yaml")
	if err := os.WriteFile(configFile, []byte("server:\n  port: 40128\n"), 0o600); err != nil {
		t.Fatalf("write default home config: %v", err)
	}
	t.Setenv("HOME", userHome)
	t.Setenv("KANDEV_HOME_DIR", "")
	t.Setenv("KANDEV_SERVER_PORT", "")
	t.Chdir(t.TempDir())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Port != 40128 || cfg.Source.FilePath != configFile {
		t.Fatalf("selected config = %q, port = %d; want %s and port 40128", cfg.Source.FilePath, cfg.Server.Port, configFile)
	}
}
