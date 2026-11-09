//go:build copilot_native

package copilotplugins_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/copilotplugins"
	"github.com/tailscale/hujson"
)

const nativeAdapterVersion = "1.0.105"

// nativeAdapterRunner executes production inventory commands with the same
// disposable environment as the direct native evidence commands.
type nativeAdapterRunner struct {
	baseEnv    []string
	captureDir string
	calls      int
}

func (r *nativeAdapterRunner) Run(
	ctx context.Context,
	argv []string,
	env map[string]string,
	workDir string,
) (copilotplugins.CommandResult, error) {
	r.calls++
	name := fmt.Sprintf("production-%02d", r.calls)
	merged := nativeAdapterEnvironmentOverrides(r.baseEnv, env)
	result := runNativeCommand(
		ctx,
		argv,
		workDir,
		merged,
		filepath.Join(r.captureDir, name+".stdout"),
		filepath.Join(r.captureDir, name+".stderr"),
	)
	if result.startErr != nil {
		return copilotplugins.CommandResult{ExitCode: -1}, result.startErr
	}
	return copilotplugins.CommandResult{
		Stdout:   result.stdout,
		Stderr:   result.stderr,
		ExitCode: result.exitCode,
	}, nil
}

// TestCopilotPluginNativeAdapterAcceptance qualifies the exact managed
// 1.0.105 package with disposable native state. It exercises the production
// inventory, installation-evidence, root-resolution, and admission boundary
// against native-written cached-install and live fixture state.
func TestCopilotPluginNativeAdapterAcceptance(t *testing.T) {
	version := strings.TrimSpace(os.Getenv(nativeVersionEnv))
	if version == "" {
		version = nativeAdapterVersion
	}
	if version != nativeAdapterVersion {
		t.Fatalf("%s must select exactly %s for this adapter acceptance test", nativeVersionEnv, nativeAdapterVersion)
	}
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("1.0.105 adapter qualification is scoped to linux/amd64")
	}
	artifactRoot := strings.TrimSpace(os.Getenv(nativeArtifactsEnv))
	if artifactRoot == "" {
		root, err := os.MkdirTemp("", "kandev-copilot-native-adapter-")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(root) })
		artifactRoot = filepath.Join(root, "artifacts")
		if err := os.Mkdir(artifactRoot, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	paths, err := prepareNativeRunPaths(version, artifactRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(paths.runtimeRoot); err != nil {
			t.Errorf("remove private runtime fixtures: %v", err)
		}
	})
	configDir := filepath.Join(paths.runtimeRoot, "copilot-home")
	homeDir := filepath.Join(paths.runtimeRoot, "home")
	for _, path := range []string{configDir, homeDir} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	env, err := nativeCommandEnvironment(paths)
	if err != nil {
		t.Fatal(err)
	}
	env = nativeAdapterEnvironmentOverrides(env, map[string]string{
		"HOME":            homeDir,
		"USERPROFILE":     homeDir,
		"COPILOT_HOME":    configDir,
		"COPILOT_GH_HOST": "",
		"GH_HOST":         "",
	})

	spec := agents.NewCopilotACP().ManagedNPMRuntime()
	if spec.PackageSpec(version) != "@github/copilot@"+version {
		t.Fatalf("managed runtime selected unexpected package for %s", version)
	}
	baseArgs, err := exactCopilotArgs(spec, version, paths.npmPrefix)
	if err != nil {
		t.Fatal(err)
	}
	globalArgs := nativeGlobalArgs(version, paths)
	runtimeCommand := append(append([]string(nil), baseArgs...), globalArgs...)
	run := func(extra []string, capture string, timeout time.Duration) nativeCommandResult {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		args := append(append([]string(nil), runtimeCommand...), extra...)
		return runNativeCommand(
			ctx,
			args,
			paths.workspace,
			env,
			filepath.Join(paths.captureRoot, capture+".stdout"),
			filepath.Join(paths.captureRoot, capture+".stderr"),
		)
	}

	identity := run([]string{"--version"}, "version", 2*time.Minute)
	metadataVersion, metadataOK := nativePackageMetadataVersion(paths, version)
	binaryOK := identity.startErr == nil && identity.exitCode == 0 &&
		containsExactVersion(identity.stdout, version)
	if !metadataOK || !binaryOK {
		t.Fatalf("package identity failed: metadata=%q ok=%t binary_exit=%d timed_out=%t",
			metadataVersion, metadataOK, identity.exitCode, identity.timedOut)
	}
	t.Logf("case=package-identity status=PASS version=%s", version)

	productionRunner := &nativeAdapterRunner{baseEnv: env, captureDir: paths.captureRoot}
	launch := copilotplugins.LaunchContext{
		AgentType:      copilotplugins.ProviderID,
		ExecutorType:   "local",
		Env:            map[string]string{"HOME": homeDir, "USERPROFILE": homeDir, "COPILOT_HOME": configDir, "PATH": os.Getenv("PATH")},
		WorkDir:        paths.workspace,
		RuntimeVersion: version,
		RuntimeCommand: runtimeCommand,
	}
	discover := func(target copilotplugins.LaunchContext) copilotplugins.Snapshot {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		return copilotplugins.Discover(ctx, productionRunner, target)
	}
	fresh := discover(launch)
	if fresh.Status != copilotplugins.StatusReady {
		t.Fatalf("fresh production discovery = status %s reason %s", fresh.Status, fresh.Reason)
	}
	for _, row := range fresh.Rows {
		if row.Source != copilotplugins.SourceBuiltin {
			t.Fatalf("fresh native state returned non-builtin row %#v", row)
		}
	}
	t.Logf("case=fresh-production-inventory status=PASS rows=%d", len(fresh.Rows))

	marketplaceRoot, livePluginRoot, err := createNativeAdapterMarketplace(paths.runtimeRoot)
	if err != nil {
		t.Fatal(err)
	}
	if name, version, err := nativeAdapterManifest(livePluginRoot); err != nil ||
		name != "harness" || version != "1.2.0" {
		t.Fatalf("live fixture manifest = %s %s err=%v", name, version, err)
	}
	addLiveMarketplace := run(
		[]string{"plugin", "marketplace", "add", marketplaceRoot},
		"live-marketplace-add",
		2*time.Minute,
	)
	if addLiveMarketplace.startErr != nil || addLiveMarketplace.exitCode != 0 {
		t.Fatalf("live marketplace add failed: exit=%d timed_out=%t stderr=%s",
			addLiveMarketplace.exitCode, addLiveMarketplace.timedOut, trimmedNativeOutput(addLiveMarketplace.stderr))
	}
	installLivePlugin := run(
		[]string{"plugin", "install", "harness@fixture-market"},
		"live-plugin-install",
		2*time.Minute,
	)
	if installLivePlugin.startErr != nil || installLivePlugin.exitCode != 0 {
		t.Fatalf("live plugin install failed: exit=%d timed_out=%t stderr=%s",
			installLivePlugin.exitCode, installLivePlugin.timedOut, trimmedNativeOutput(installLivePlugin.stderr))
	}

	remoteMarketplace := run(
		[]string{"plugin", "marketplace", "add", "github/copilot-plugins"},
		"installed-marketplace-add",
		2*time.Minute,
	)
	if remoteMarketplace.startErr != nil || remoteMarketplace.exitCode != 0 {
		t.Fatalf("installed marketplace add failed: exit=%d timed_out=%t stderr=%s",
			remoteMarketplace.exitCode, remoteMarketplace.timedOut, trimmedNativeOutput(remoteMarketplace.stderr))
	}
	installRemotePlugin := run(
		[]string{"plugin", "install", "build-perf-cpp@copilot-plugins"},
		"installed-plugin-install",
		2*time.Minute,
	)
	if installRemotePlugin.startErr != nil || installRemotePlugin.exitCode != 0 {
		t.Fatalf("installed plugin install failed: exit=%d timed_out=%t stderr=%s",
			installRemotePlugin.exitCode, installRemotePlugin.timedOut, trimmedNativeOutput(installRemotePlugin.stderr))
	}
	configData, err := os.ReadFile(filepath.Join(configDir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(configData), `"installedPlugins"`) ||
		!strings.Contains(string(configData), `"build-perf-cpp"`) ||
		!strings.Contains(string(configData), `"cache_path"`) {
		t.Fatalf("native config.json did not record cached installation metadata: %s", configDir)
	}

	snapshot := discover(launch)
	if snapshot.Status != copilotplugins.StatusReady {
		t.Fatalf("installed production discovery = status %s reason %s", snapshot.Status, snapshot.Reason)
	}
	installedRow, ok := nativeAdapterInventoryRow(snapshot.Rows, copilotplugins.SourceInstalled, "copilot-plugins", "build-perf-cpp")
	if !ok || installedRow.Availability != copilotplugins.AvailabilityAvailable {
		t.Fatalf("native installed row = %#v, want an available source=installed row", installedRow)
	}
	liveRow, ok := nativeAdapterInventoryRow(snapshot.Rows, copilotplugins.SourceLive, "fixture-market", "harness")
	if !ok || liveRow.Availability != copilotplugins.AvailabilityAvailable {
		t.Fatalf("native live row = %#v, want an available source=live control row", liveRow)
	}
	t.Logf("case=native-installed-metadata status=PASS source=installed row=build-perf-cpp live_control=harness")

	installedPrepared, err := copilotplugins.ResolveLaunchSelection(
		context.Background(), productionRunner, launch, []string{"installed:copilot-plugins/build-perf-cpp"},
	)
	if err != nil {
		t.Fatalf("production installed admission: %v", err)
	}
	if len(installedPrepared.Plugins) != 1 {
		t.Fatalf("installed admission = %#v", installedPrepared.Plugins)
	}
	installedRoot := installedPrepared.Plugins[0].Root
	if !filepath.IsAbs(installedRoot) {
		t.Fatalf("installed root = %q, want an absolute native cache path", installedRoot)
	}
	installedVersion, err := nativeAdapterInstalledVersion(configDir, "copilot-plugins", "build-perf-cpp")
	if err != nil {
		t.Fatal(err)
	}
	if name, version, err := nativeAdapterManifest(installedRoot); err != nil ||
		name != "build-perf-cpp" || version != installedVersion {
		t.Fatalf("installed manifest = %s %s err=%v root=%s, want version %s",
			name, version, err, installedRoot, installedVersion)
	}
	livePrepared, err := copilotplugins.ResolveLaunchSelection(
		context.Background(), productionRunner, launch, []string{"live:fixture-market/harness"},
	)
	if err != nil {
		t.Fatalf("production live admission control: %v", err)
	}
	if len(livePrepared.Plugins) != 1 {
		t.Fatalf("live admission = %#v", livePrepared.Plugins)
	}
	liveRoot, err := filepath.EvalSymlinks(livePluginRoot)
	if err != nil {
		t.Fatal(err)
	}
	if livePrepared.Plugins[0].Root != liveRoot {
		t.Fatalf("live root = %q, want %q", livePrepared.Plugins[0].Root, liveRoot)
	}
	t.Logf("case=production-installed-admission status=PASS installed=build-perf-cpp live=harness")

	fixtures, err := createNativePluginFixtures(paths.runtimeRoot)
	if err != nil {
		t.Fatal(err)
	}
	mountLaunch := launch
	mountLaunch.RuntimeCommand = append(append([]string(nil), runtimeCommand...),
		"--plugin-dir", livePluginRoot,
		"--plugin-dir", fixtures.targetRoot,
	)
	mounted := discover(mountLaunch)
	if mounted.Status != copilotplugins.StatusReady {
		t.Fatalf("repeated mount discovery = status %s reason %s", mounted.Status, mounted.Reason)
	}
	if !nativeAdapterExternalRow(mounted.Rows, "harness") ||
		!nativeAdapterExternalRow(mounted.Rows, nativeTargetPlugin) {
		t.Fatalf("repeated special-character mounts missing from production inventory: %#v", mounted.Rows)
	}
	if _, ok := nativeAdapterInventoryRow(mounted.Rows, copilotplugins.SourceInstalled, "copilot-plugins", "build-perf-cpp"); !ok {
		t.Fatalf("installed row disappeared from repeated-mount inventory")
	}
	t.Logf("case=native-live-and-repeated-mounts status=PASS live=harness external=2 installed=build-perf-cpp special_paths=2")
}

func nativeAdapterEnvironmentOverrides(env []string, overrides map[string]string) []string {
	filtered := make([]string, 0, len(env)+len(overrides))
	for _, item := range env {
		key, _, ok := strings.Cut(item, "=")
		if ok {
			if _, replaced := overrides[key]; replaced {
				continue
			}
		}
		filtered = append(filtered, item)
	}
	for key, value := range overrides {
		if value == "" {
			continue
		}
		filtered = append(filtered, key+"="+value)
	}
	sort.Strings(filtered)
	return filtered
}

func createNativeAdapterMarketplace(runtimeRoot string) (string, string, error) {
	marketplaceRoot := filepath.Join(runtimeRoot, "marketplace ;[space] $")
	pluginRoot := filepath.Join(marketplaceRoot, "plugins", "harness")
	if err := os.MkdirAll(filepath.Join(pluginRoot, ".github", "plugin"), 0o700); err != nil {
		return "", "", fmt.Errorf("create private marketplace fixture: %w", err)
	}
	marketplaceManifest, err := json.Marshal(map[string]any{
		"name":  "fixture-market",
		"owner": map[string]string{"name": "Kandev"},
		"plugins": []map[string]any{{
			"name":    "harness",
			"source":  "./plugins/harness",
			"version": "1.2.0",
		}},
	})
	if err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(filepath.Join(marketplaceRoot, ".github", "plugin"), 0o700); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(
		filepath.Join(marketplaceRoot, ".github", "plugin", "marketplace.json"),
		marketplaceManifest,
		0o600,
	); err != nil {
		return "", "", err
	}
	pluginManifest, err := json.Marshal(map[string]any{
		"name":    "harness",
		"version": "1.2.0",
	})
	if err != nil {
		return "", "", err
	}
	if err := os.WriteFile(
		filepath.Join(pluginRoot, ".github", "plugin", "plugin.json"),
		pluginManifest,
		0o600,
	); err != nil {
		return "", "", err
	}
	return marketplaceRoot, pluginRoot, nil
}

func nativeAdapterInstalledVersion(configDir, marketplace, name string) (string, error) {
	data, err := os.ReadFile(filepath.Join(configDir, "config.json"))
	if err != nil {
		return "", err
	}
	standardized, err := hujson.Standardize(data)
	if err != nil {
		return "", err
	}
	var config struct {
		InstalledPlugins []struct {
			Name        string `json:"name"`
			Marketplace string `json:"marketplace"`
			Version     string `json:"version"`
		} `json:"installedPlugins"`
	}
	if err := json.Unmarshal(standardized, &config); err != nil {
		return "", err
	}
	for _, entry := range config.InstalledPlugins {
		if entry.Name == name && entry.Marketplace == marketplace && entry.Version != "" {
			return entry.Version, nil
		}
	}
	return "", fmt.Errorf("native installation metadata did not name %s@%s", name, marketplace)
}

func nativeAdapterManifest(pluginRoot string) (string, string, error) {
	candidates := []string{
		"plugin.json",
		filepath.Join(".plugin", "plugin.json"),
		filepath.Join(".github", "plugin", "plugin.json"),
		filepath.Join(".claude-plugin", "plugin.json"),
	}
	var data []byte
	var err error
	for _, relative := range candidates {
		data, err = os.ReadFile(filepath.Join(pluginRoot, relative))
		if err == nil {
			break
		}
	}
	if err != nil {
		return "", "", err
	}
	var manifest struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return "", "", err
	}
	return manifest.Name, manifest.Version, nil
}

func nativeAdapterInventoryRow(
	rows []copilotplugins.InventoryRow,
	source, marketplace, name string,
) (copilotplugins.InventoryRow, bool) {
	for _, row := range rows {
		if row.Source == source && row.Marketplace == marketplace && row.Name == name {
			return row, true
		}
	}
	return copilotplugins.InventoryRow{}, false
}

func nativeAdapterExternalRow(rows []copilotplugins.InventoryRow, name string) bool {
	for _, row := range rows {
		if row.Source == copilotplugins.SourceExternal && row.Name == name {
			return true
		}
	}
	return false
}

func trimmedNativeOutput(output []byte) string {
	value := strings.TrimSpace(string(output))
	if len(value) > 512 {
		return value[:512]
	}
	return value
}
