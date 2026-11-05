//go:build copilot_native

package copilotplugins

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/tailscale/hujson"
)

const nativeInventoryVersionEnv = "KANDEV_COPILOT_NATIVE_VERSION"

var nativeInventoryStableVersion = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

type nativeInventoryCommandResult struct {
	stdout   string
	stderr   string
	exitCode int
}

// @covers AC-AGENTS-COPILOT-PLUGINS-001.1, AC-AGENTS-COPILOT-PLUGINS-004.1, AC-AGENTS-COPILOT-PLUGINS-005.8
func TestCopilotPluginNativeInventoryCompatibility(t *testing.T) {
	version := strings.TrimSpace(os.Getenv(nativeInventoryVersionEnv))
	if !nativeInventoryStableVersion.MatchString(version) {
		t.Fatalf("%s must select one exact stable version", nativeInventoryVersionEnv)
	}
	npmPath, err := exec.LookPath("npm")
	if err != nil {
		t.Fatalf("npm is required for the isolated native inventory fixture: %v", err)
	}

	root := t.TempDir()
	home := filepath.Join(root, "home")
	configDir := filepath.Join(root, "config")
	workDir := filepath.Join(root, "work")
	runtimeRoot := filepath.Join(root, "runtime")
	npmCache := filepath.Join(root, "npm-cache")
	npmUserConfig := filepath.Join(root, "npm-user.npmrc")
	npmGlobalConfig := filepath.Join(root, "npm-global.npmrc")
	for _, directory := range []string{home, configDir, workDir, runtimeRoot, npmCache} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatalf("create isolated native fixture directory: %v", err)
		}
	}
	for _, configPath := range []string{npmUserConfig, npmGlobalConfig} {
		if err := os.WriteFile(configPath, nil, 0o600); err != nil {
			t.Fatalf("create private npm configuration: %v", err)
		}
	}

	env := nativeInventoryEnvironment(home, configDir, npmCache, npmUserConfig, npmGlobalConfig)
	installNativeCopilotPackage(t, npmPath, runtimeRoot, workDir, version, env)
	cli := filepath.Join(runtimeRoot, "node_modules", ".bin", "copilot")
	if _, err := os.Stat(cli); err != nil {
		t.Fatalf("expected exact installed Copilot CLI: %v", err)
	}
	assertNativePackageMetadata(t, runtimeRoot, version)

	identity := runNativeInventoryCommand(t, workDir, env, cli, "--version")
	if got, want := nativeInventoryFirstLine(identity.stdout), "GitHub Copilot CLI "+version+"."; got != want {
		t.Fatalf("installed CLI identity = %q, want %q", got, want)
	}

	targetRoot := filepath.Join(root, "plugin roots", "target ;[space] $")
	siblingRoot := filepath.Join(root, "plugin roots", "sibling [control]")
	writeNativeInventoryPlugin(t, targetRoot, "inventory-target")
	writeNativeInventoryPlugin(t, siblingRoot, "inventory-sibling")

	repeated := runNativeInventoryCommand(
		t, workDir, env, cli,
		"--no-auto-update",
		"--disable-client-telemetry",
		"--sync-session=off",
		"--plugin-dir", targetRoot,
		"--plugin-dir", siblingRoot,
		"plugin", "list",
		"--config-dir="+configDir,
		"--json",
	)
	rows, format, reason := parseInventory([]byte(repeated.stdout))
	if reason != "" {
		t.Fatalf("adapter rejected the native repeated-mount inventory: %s", reason)
	}
	if format != inventoryFormatFlat {
		t.Fatalf("inventory format = %q, want %q", format, inventoryFormatFlat)
	}
	externalRows := nativeInventoryRowsByName(rows)
	assertNativeExternalRow(t, externalRows, "inventory-target")
	assertNativeExternalRow(t, externalRows, "inventory-sibling")
	t.Logf("case=repeated-plugin-dirs status=PASS roots=2 special_character_path=true")

	marketplaceRoot := filepath.Join(root, "local-market")
	installedRoot := filepath.Join(marketplaceRoot, "plugins", "inventory-installed")
	writeNativeInventoryPlugin(t, installedRoot, "inventory-installed")
	writeNativeInventoryMarketplace(t, marketplaceRoot)
	runNativeInventoryCommand(
		t, workDir, env, cli,
		"--no-auto-update",
		"--disable-client-telemetry",
		"--sync-session=off",
		"plugin", "marketplace", "add", marketplaceRoot,
	)
	runNativeInventoryCommand(
		t, workDir, env, cli,
		"--no-auto-update",
		"--disable-client-telemetry",
		"--sync-session=off",
		"plugin", "install", "inventory-installed@fixture-market",
	)
	assertNativeMarketplaceInstallMetadata(t, configDir, "fixture-market", "inventory-installed")

	installed := runNativeInventoryCommand(
		t, workDir, env, cli,
		"--no-auto-update",
		"--disable-client-telemetry",
		"--sync-session=off",
		"plugin", "list",
		"--config-dir="+configDir,
		"--json",
	)
	installedRows, _, reason := parseInventory([]byte(installed.stdout))
	if reason != "" {
		t.Fatalf("adapter rejected the native installed-plugin inventory: %s", reason)
	}
	installedPlugin, ok := nativeInventoryRowsByName(installedRows)["inventory-installed"]
	if !ok || installedPlugin.Source != SourceLive || installedPlugin.Marketplace != "fixture-market" {
		t.Fatalf("installed inventory did not expose inventory-installed@fixture-market")
	}
	if installedPlugin.Version == "" {
		t.Fatal("installed inventory omitted the plugin version")
	}
	launch := LaunchContext{
		AgentType:      ProviderID,
		ExecutorType:   "local",
		Env:            map[string]string{"HOME": home, "COPILOT_HOME": configDir},
		WorkDir:        workDir,
		RuntimeVersion: version,
	}
	_ = resolveInventory(installedRows, launch)
	installedPlugin = nativeInventoryRowsByName(installedRows)["inventory-installed"]
	if installedPlugin.Availability != AvailabilityAvailable {
		t.Fatalf("installed plugin availability = %q (%s), want available", installedPlugin.Availability, installedPlugin.Reason)
	}
	canonical, err := filepath.EvalSymlinks(installedRoot)
	if err != nil || installedPlugin.root != canonical {
		t.Fatalf("installed plugin root was not the observed marketplace source")
	}
	t.Logf("case=installed-plugin-metadata status=PASS version=%s installed_from=true", installedPlugin.Version)
}

func nativeInventoryEnvironment(home, configDir, npmCache, npmUserConfig, npmGlobalConfig string) []string {
	env := make([]string, 0, 16)
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		lowerKey := strings.ToLower(key)
		if lowerKey == "home" || lowerKey == "copilot_home" {
			continue
		}
		if !nativeInventoryEnvironmentKeyAllowed(lowerKey) {
			continue
		}
		env = append(env, item)
	}
	return append(env,
		"HOME="+home,
		"COPILOT_HOME="+configDir,
		"COPILOT_AUTO_UPDATE=false",
		"NO_COLOR=1",
		"NPM_CONFIG_CACHE="+npmCache,
		"NPM_CONFIG_USERCONFIG="+npmUserConfig,
		"NPM_CONFIG_GLOBALCONFIG="+npmGlobalConfig,
	)
}

func nativeInventoryEnvironmentKeyAllowed(key string) bool {
	switch key {
	case "path", "user", "logname", "shell", "lang", "term",
		"term_program", "colorterm", "gh_host", "copilot_gh_host",
		"http_proxy", "https_proxy", "all_proxy", "no_proxy",
		"ssl_cert_file", "ssl_cert_dir", "node_extra_ca_certs":
		return true
	default:
		return strings.HasPrefix(key, "lc_")
	}
}

func installNativeCopilotPackage(t *testing.T, npmPath, runtimeRoot, workDir, version string, env []string) {
	t.Helper()
	runNativeInventoryCommand(t, workDir, env, npmPath,
		"install",
		"--prefix", runtimeRoot,
		"--no-save",
		"--package-lock=false",
		"--no-audit",
		"--no-fund",
		"--prefer-online",
		"@github/copilot@"+version,
	)
}

func assertNativePackageMetadata(t *testing.T, runtimeRoot, version string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(
		runtimeRoot, "node_modules", "@github", "copilot", "package.json",
	))
	if err != nil {
		t.Fatalf("read exact package metadata: %v", err)
	}
	var metadata struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		t.Fatalf("decode exact package metadata: %v", err)
	}
	if metadata.Name != "@github/copilot" || metadata.Version != version {
		t.Fatalf("package metadata = %s@%s, want @github/copilot@%s", metadata.Name, metadata.Version, version)
	}
}

func writeNativeInventoryPlugin(t *testing.T, root, name string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatalf("create private plugin root: %v", err)
	}
	data, err := json.Marshal(map[string]any{
		"name":    name,
		"version": "1.0.0",
	})
	if err != nil {
		t.Fatalf("encode private plugin manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "plugin.json"), data, 0o600); err != nil {
		t.Fatalf("write private plugin manifest: %v", err)
	}
}

func runNativeInventoryCommand(t *testing.T, workDir string, env []string, executable string, args ...string) nativeInventoryCommandResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Dir = workDir
	cmd.Env = env
	cmd.WaitDelay = runnerWaitDelay
	stdout := &limitedBuffer{limit: inventoryStdoutLimit}
	stderr := &limitedBuffer{limit: inventoryStderrLimit}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	result, err := runCommand(ctx, cmd, stdout, stderr)
	if err != nil {
		t.Fatalf("native Copilot command failed: %v (stderr withheld; %d bytes)", err, len(result.Stderr))
	}
	if result.ExitCode != 0 {
		t.Fatalf("native Copilot command exited %d (stderr withheld; %d bytes)", result.ExitCode, len(result.Stderr))
	}
	return nativeInventoryCommandResult{
		stdout:   string(result.Stdout),
		stderr:   string(result.Stderr),
		exitCode: result.ExitCode,
	}
}

func nativeInventoryFirstLine(output string) string {
	return strings.TrimSpace(strings.SplitN(output, "\n", 2)[0])
}

func nativeInventoryRowsByName(rows []InventoryRow) map[string]InventoryRow {
	byName := make(map[string]InventoryRow, len(rows))
	for _, row := range rows {
		byName[row.Name] = row
	}
	return byName
}

func assertNativeExternalRow(t *testing.T, rows map[string]InventoryRow, name string) {
	t.Helper()
	row, ok := rows[name]
	if !ok {
		t.Fatalf("external plugin %q was not visible in the native inventory", name)
	}
	if row.Source != SourceExternal || row.Version != "1.0.0" {
		t.Fatalf("external plugin %q = source %q version %q", name, row.Source, row.Version)
	}
}

func writeNativeInventoryMarketplace(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatalf("create private marketplace root: %v", err)
	}
	data, err := json.Marshal(map[string]any{
		"name": "fixture-market",
		"metadata": map[string]string{
			"description": "Kandev native inventory fixture",
			"version":     "1.0.0",
		},
		"owner": map[string]string{"name": "Kandev"},
		"plugins": []map[string]any{{
			"name":        "inventory-installed",
			"description": "Kandev native inventory fixture",
			"version":     "1.0.0",
			"source":      "plugins/inventory-installed",
		}},
	})
	if err != nil {
		t.Fatalf("encode private marketplace manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "marketplace.json"), data, 0o600); err != nil {
		t.Fatalf("write private marketplace manifest: %v", err)
	}
}

func assertNativeMarketplaceInstallMetadata(t *testing.T, configDir, marketplace, plugin string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(configDir, "settings.json"))
	if err != nil {
		t.Fatalf("read native installation metadata: %v", err)
	}
	var settings struct {
		ExtraKnownMarketplaces map[string]json.RawMessage `json:"extraKnownMarketplaces"`
		EnabledPlugins         map[string]bool            `json:"enabledPlugins"`
	}
	standardized, err := hujson.Standardize(data)
	if err != nil {
		t.Fatalf("standardize native installation metadata: %v", err)
	}
	if err := json.Unmarshal(standardized, &settings); err != nil {
		t.Fatalf("decode native installation metadata: %v", err)
	}
	if _, ok := settings.ExtraKnownMarketplaces[marketplace]; !ok {
		t.Fatalf("native installation metadata omitted marketplace %q", marketplace)
	}
	if !settings.EnabledPlugins[plugin+"@"+marketplace] {
		t.Fatalf("native installation metadata did not enable %s@%s", plugin, marketplace)
	}
}
