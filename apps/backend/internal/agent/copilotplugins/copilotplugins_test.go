package copilotplugins

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeRunner struct {
	result CommandResult
	err    error
	calls  [][]string
	envs   []map[string]string
	dirs   []string
	run    func(context.Context) (CommandResult, error)
}

func (f *fakeRunner) Run(ctx context.Context, argv []string, env map[string]string, workDir string) (CommandResult, error) {
	f.calls = append(f.calls, append([]string{}, argv...))
	f.envs = append(f.envs, env)
	f.dirs = append(f.dirs, workDir)
	if f.run != nil {
		return f.run(ctx)
	}
	return f.result, f.err
}

func launchFor(t *testing.T, configDir string) LaunchContext {
	t.Helper()
	return LaunchContext{
		AgentType:      ProviderID,
		ExecutorType:   "local",
		Env:            map[string]string{"HOME": t.TempDir(), "COPILOT_HOME": configDir, "PATH": os.Getenv("PATH")},
		WorkDir:        t.TempDir(),
		RuntimeVersion: "1.0.88",
		RuntimeCommand: []string{"npx", "--yes", "--prefer-offline", "@github/copilot@1.0.88"},
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writePlugin(t *testing.T, root, manifestRelative, name, version string) {
	t.Helper()
	writeFile(t, filepath.Join(root, manifestRelative), `{"name":`+quote(name)+`,"version":`+quote(version)+`}`)
}

func quote(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func TestInventoryFormats(t *testing.T) {
	rows := []map[string]any{
		{"name": "harness", "marketplace": "fixture-market", "version": "1.2.0", "enabled": false, "source": "installed"},
	}
	encoded, _ := json.Marshal(rows)
	snapshot := Discover(context.Background(), &fakeRunner{result: CommandResult{Stdout: encoded}}, launchFor(t, t.TempDir()))
	if snapshot.Status != StatusReady || len(snapshot.Rows) != 1 {
		t.Fatalf("snapshot = %#v, want one ready row", snapshot)
	}
	if snapshot.Format != inventoryFormatFlat {
		t.Fatalf("format = %q", snapshot.Format)
	}
	if snapshot.Rows[0].NativeEnabled {
		t.Fatal("native disabled row lost its status")
	}

	envelope := []byte(`{"plugins":[{"name":"harness","enabled":true,"source":"installed"}],"errors":[]}`)
	snapshot = Discover(context.Background(), &fakeRunner{result: CommandResult{Stdout: envelope}}, launchFor(t, t.TempDir()))
	if snapshot.Status != StatusReady || len(snapshot.Rows) != 1 || snapshot.Format != inventoryFormatEnvel {
		t.Fatalf("envelope snapshot = %#v", snapshot)
	}

	failedEnvelope := []byte(`{"plugins":[],"errors":[{"message":"boom"}]}`)
	snapshot = Discover(context.Background(), &fakeRunner{result: CommandResult{Stdout: failedEnvelope}}, launchFor(t, t.TempDir()))
	if snapshot.Status != StatusUnavailable || snapshot.Reason != ReasonLegacyErrors {
		t.Fatalf("legacy errors snapshot = %#v", snapshot)
	}
}

func TestInventoryIdentities(t *testing.T) {
	root := t.TempDir()
	writePlugin(t, root, ".github/plugin/plugin.json", "harness", "1.2.0")
	configDir := t.TempDir()
	rows := []map[string]any{
		{"name": "harness", "marketplace": "market-a", "version": "1.2.0", "enabled": true, "source": "external"},
		{"name": "browser-use", "version": "0.0.4", "enabled": false, "source": "builtin"},
		{"name": "harness", "marketplace": "", "version": "1.2.0", "enabled": true, "source": "external"},
		{"name": "harness", "marketplace": "market-a", "version": "1.2.0", "enabled": true, "source": "external"},
	}
	encoded, _ := json.Marshal(rows)
	snapshot := Discover(context.Background(), &fakeRunner{result: CommandResult{Stdout: encoded}}, launchFor(t, configDir))
	if snapshot.Status != StatusReady {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	if len(snapshot.Rows) != 3 {
		t.Fatalf("rows = %d, want deduped 3: %#v", len(snapshot.Rows), snapshot.Rows)
	}
	byID := map[string]InventoryRow{}
	for _, row := range snapshot.Rows {
		byID[row.ID.String()] = row
	}
	if byID["external:market-a/harness"].Reason != ReasonAmbiguousIdentity {
		t.Fatalf("duplicate identity = %#v", byID["external:market-a/harness"])
	}
	if byID["builtin:/browser-use"].Reason != ReasonBuiltinNotMountable {
		t.Fatalf("builtin = %#v", byID["builtin:/browser-use"])
	}
	if byID["external:/harness"].Reason != ReasonExternalNotMountable {
		t.Fatalf("external = %#v", byID["external:/harness"])
	}
}

func TestInventoryRejectsInvalidOutput(t *testing.T) {
	tests := []struct {
		name   string
		stdout string
		reason ReasonCode
	}{
		{name: "duplicate key", stdout: `[{"name":"a","name":"b","enabled":true,"source":"installed"}]`, reason: ReasonInvalidInventory},
		{name: "missing required", stdout: `[{"name":"a","source":"installed"}]`, reason: ReasonInvalidInventory},
		{name: "wrong enabled type", stdout: `[{"name":"a","enabled":"yes","source":"installed"}]`, reason: ReasonInvalidInventory},
		{name: "unknown container", stdout: `"plugins"`, reason: ReasonInvalidInventory},
		{name: "unclosed", stdout: `[{"name":"a","enabled":true,"source":"installed"}`, reason: ReasonInvalidInventory},
		{name: "control name", stdout: "[{\"name\":\"a\\u0000b\",\"enabled\":true,\"source\":\"installed\"}]", reason: ReasonInvalidInventory},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := Discover(context.Background(), &fakeRunner{result: CommandResult{Stdout: []byte(test.stdout)}}, launchFor(t, t.TempDir()))
			if snapshot.Status == StatusReady {
				t.Fatalf("snapshot accepted invalid output: %#v", snapshot)
			}
			if snapshot.Reason != test.reason {
				t.Fatalf("reason = %q, want %q", snapshot.Reason, test.reason)
			}
		})
	}
}

func TestInstallationMetadata(t *testing.T) {
	root := t.TempDir()
	writePlugin(t, root, "plugin.json", "spark", "1.0.0")
	wantRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	configDir := t.TempDir()
	writeFile(t, filepath.Join(configDir, "config.json"), `// User settings belong in settings.json.
{
  "installedPlugins": [
    {"name":"spark","marketplace":"copilot-plugins","version":"1.0.0","cache_path":`+quote(root)+`,"enabled":true}
  ]
}`)
	rows := []map[string]any{{"name": "spark", "marketplace": "copilot-plugins", "version": "1.0.0", "enabled": true, "source": "installed"}}
	encoded, _ := json.Marshal(rows)
	snapshot := Discover(context.Background(), &fakeRunner{result: CommandResult{Stdout: encoded}}, launchFor(t, configDir))
	if len(snapshot.Rows) != 1 || snapshot.Rows[0].Availability != AvailabilityAvailable {
		t.Fatalf("installed snapshot = %#v", snapshot)
	}
	if snapshot.Rows[0].root != wantRoot {
		t.Fatalf("root = %q, want %q", snapshot.Rows[0].root, wantRoot)
	}

	launch104 := launchFor(t, configDir)
	launch104.RuntimeVersion = "1.0.104"
	snapshot = Discover(context.Background(), &fakeRunner{result: CommandResult{Stdout: encoded}}, launch104)
	if len(snapshot.Rows) != 1 || snapshot.Rows[0].Availability != AvailabilityAvailable {
		t.Fatalf("1.0.104 installed snapshot = %#v", snapshot)
	}

	rows[0]["version"] = "2.0.0"
	encoded, _ = json.Marshal(rows)
	snapshot = Discover(context.Background(), &fakeRunner{result: CommandResult{Stdout: encoded}}, launchFor(t, configDir))
	if snapshot.Rows[0].Reason != ReasonMetadataUnavailable {
		t.Fatalf("stale version reason = %q", snapshot.Rows[0].Reason)
	}
}

func TestPluginRootValidation(t *testing.T) {
	configDir := t.TempDir()
	root := t.TempDir()
	writePlugin(t, root, "plugin.json", "spark", "1.0.0")
	wantRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}

	writeConfig := func(cachePath string) {
		t.Helper()
		writeFile(t, filepath.Join(configDir, "config.json"), `{"installedPlugins":[{"name":"spark","marketplace":"copilot-plugins","version":"1.0.0","cache_path":`+quote(cachePath)+`}]}`)
	}
	inventory := func() Snapshot {
		encoded := []byte(`[{"name":"spark","marketplace":"copilot-plugins","version":"1.0.0","enabled":true,"source":"installed"}]`)
		return Discover(context.Background(), &fakeRunner{result: CommandResult{Stdout: encoded}}, launchFor(t, configDir))
	}

	writeConfig(filepath.Join(root, "missing"))
	if reason := inventory().Rows[0].Reason; reason != ReasonMissingRoot {
		t.Fatalf("missing root reason = %q", reason)
	}
	writeConfig("/")
	if reason := inventory().Rows[0].Reason; reason != ReasonUnsafeRoot {
		t.Fatalf("volume root reason = %q", reason)
	}
	writeConfig(root + string(filepath.Separator) + "..")
	if reason := inventory().Rows[0].Reason; reason != ReasonUnsafeRoot {
		t.Fatalf("traversal reason = %q", reason)
	}

	linkDir := t.TempDir()
	link := filepath.Join(linkDir, "plugin-link")
	if err := os.Symlink(root, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	writeConfig(link)
	snapshot := inventory()
	if snapshot.Rows[0].Availability != AvailabilityAvailable {
		t.Fatalf("symlink root = %#v", snapshot.Rows[0])
	}
	if snapshot.Rows[0].root != wantRoot {
		t.Fatalf("symlink root canonical = %q, want %q", snapshot.Rows[0].root, wantRoot)
	}

	writePlugin(t, root, "plugin.json", "other", "1.0.0")
	writeConfig(root)
	// The first existing manifest is now `other`, which no longer matches.
	if reason := inventory().Rows[0].Reason; reason != ReasonManifestMismatch {
		t.Fatalf("manifest mismatch reason = %q", reason)
	}

	realManifest := filepath.Join(root, "real-plugin.json")
	if err := os.Rename(filepath.Join(root, "plugin.json"), realManifest); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realManifest, filepath.Join(root, "plugin.json")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	writeConfig(root)
	if reason := inventory().Rows[0].Reason; reason != ReasonManifestMismatch {
		t.Fatalf("symlinked manifest reason = %q", reason)
	}
}

func TestLivePluginRoot(t *testing.T) {
	marketplaceRoot := t.TempDir()
	writeFile(t, filepath.Join(marketplaceRoot, ".github", "plugin", "marketplace.json"), `{
  "name":"fixture-market",
  "owner":{"name":"Kandev"},
  "plugins":[{"name":"harness","source":"./plugins/harness","version":"1.2.0"}]
}`)
	root := filepath.Join(marketplaceRoot, "plugins", "harness")
	writePlugin(t, root, ".github/plugin/plugin.json", "harness", "1.2.0")
	wantRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	encoded := []byte(`[{"name":"harness","marketplace":"fixture-market","version":"1.2.0","enabled":true,"source":"live","installedFrom":` + quote(marketplaceRoot) + `}]`)
	snapshot := Discover(context.Background(), &fakeRunner{result: CommandResult{Stdout: encoded}}, launchFor(t, t.TempDir()))
	if len(snapshot.Rows) != 1 || snapshot.Rows[0].Availability != AvailabilityAvailable {
		t.Fatalf("live snapshot = %#v", snapshot)
	}
	if snapshot.Rows[0].root != wantRoot {
		t.Fatalf("live root = %q, want %q", snapshot.Rows[0].root, wantRoot)
	}

	// A relative marketplace source that escapes the marketplace root is not
	// a verified installation directory.
	writeFile(t, filepath.Join(marketplaceRoot, ".github", "plugin", "marketplace.json"), `{
  "name":"fixture-market",
  "owner":{"name":"Kandev"},
  "plugins":[{"name":"harness","source":"../outside","version":"1.2.0"}]
}`)
	outside := filepath.Join(filepath.Dir(marketplaceRoot), "outside")
	writePlugin(t, outside, "plugin.json", "harness", "1.2.0")
	snapshot = Discover(context.Background(), &fakeRunner{result: CommandResult{Stdout: encoded}}, launchFor(t, t.TempDir()))
	if snapshot.Rows[0].Reason != ReasonUnsupportedSource {
		t.Fatalf("traversal source reason = %q", snapshot.Rows[0].Reason)
	}
}

func TestPluginArguments(t *testing.T) {
	pluginA := filepath.Join(t.TempDir(), "plugin a")
	pluginB := filepath.Join(t.TempDir(), "plugin-b")
	if err := os.MkdirAll(pluginA, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(pluginB, 0o755); err != nil {
		t.Fatal(err)
	}
	plugins := []ResolvedPlugin{
		{ID: PluginID{Source: SourceInstalled, Marketplace: "m", Name: "b"}, Root: pluginB},
		{ID: PluginID{Source: SourceInstalled, Marketplace: "m", Name: "a"}, Root: pluginA},
	}
	args, err := ApplyToArgs([]string{"npx", "@github/copilot@1.0.88", "--acp", "--", "prompt"}, plugins)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"npx", "@github/copilot@1.0.88", "--acp", "--plugin-dir", pluginB, "--plugin-dir", pluginA, "--", "prompt"}
	if strings.Join(args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("args = %#v, want %#v", args, want)
	}

	// Reapplying is idempotent because the typed mounts are equal manual roots.
	args, err = ApplyToArgs(args, plugins)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("idempotent args = %#v", args)
	}

	// A manual mount for the same root suppresses a typed pair.
	manual := []string{"npx", "copilot", "--plugin-dir", pluginA}
	args, err = ApplyToArgs(manual, plugins[1:])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(args, "\x00") != strings.Join(manual, "\x00") {
		t.Fatalf("manual dedupe args = %#v", args)
	}

	// Relative manual roots resolve against the session working directory.
	workDir := t.TempDir()
	relativeRoot := filepath.Join(workDir, "relative-plugin")
	writePlugin(t, relativeRoot, "plugin.json", "relative", "1.0.0")
	relativeManual := []string{"npx", "copilot", "--plugin-dir", "relative-plugin"}
	args, err = ApplyToArgsInDir(relativeManual, []ResolvedPlugin{{
		ID: PluginID{Source: SourceInstalled, Marketplace: "", Name: "relative"}, Root: relativeRoot,
	}}, workDir)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(strings.Join(args, " "), "--plugin-dir") != 1 {
		t.Fatalf("relative manual root was duplicated: %#v", args)
	}
}

func TestPluginArgumentConflict(t *testing.T) {
	manualRoot := t.TempDir()
	writePlugin(t, manualRoot, "plugin.json", "harness", "1.2.0")
	otherRoot := t.TempDir()
	writePlugin(t, otherRoot, "plugin.json", "harness", "1.2.0")
	_, err := ApplyToArgs(
		[]string{"copilot", "--plugin-dir", manualRoot},
		[]ResolvedPlugin{{ID: PluginID{Source: SourceInstalled, Marketplace: "", Name: "harness"}, Root: otherRoot}},
	)
	var pluginErr *Error
	if !errors.As(err, &pluginErr) || pluginErr.Reason != ReasonPluginMountConflict {
		t.Fatalf("conflict error = %#v", err)
	}
}

func TestPluginLaunchContext(t *testing.T) {
	launch := launchFor(t, t.TempDir())
	launch.CLIFlags = []string{"--model", "auto"}
	if _, err := ResolveSelection(context.Background(), &fakeRunner{}, launch, nil); err != nil {
		t.Fatalf("empty selection should be a no-op: %v", err)
	}
	launch.ExecutorType = "local_docker"
	_, err := ResolveSelection(context.Background(), &fakeRunner{}, launch, []string{"installed:m/p"})
	var pluginErr *Error
	if !errors.As(err, &pluginErr) || pluginErr.Reason != ReasonUnsupportedLaunch {
		t.Fatalf("executor error = %#v", err)
	}
	launch = launchFor(t, t.TempDir())
	launch.RuntimeVersion = "9.9.9"
	_, err = ResolveSelection(context.Background(), &fakeRunner{}, launch, []string{"installed:m/p"})
	if !errors.As(err, &pluginErr) || pluginErr.Reason != ReasonRuntimeUnsupported {
		t.Fatalf("runtime error = %#v", err)
	}
}

func TestInventoryCommandFailures(t *testing.T) {
	tests := []struct {
		name   string
		result CommandResult
		err    error
		reason ReasonCode
	}{
		{name: "nonzero exit", result: CommandResult{ExitCode: 1}, reason: ReasonCLIUnavailable},
		{name: "truncated", result: CommandResult{Truncated: true}, reason: ReasonOutputLimit},
		{name: "unavailable", err: ErrExecutableUnavailable, reason: ReasonCLIUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := Discover(context.Background(), &fakeRunner{result: test.result, err: test.err}, launchFor(t, t.TempDir()))
			if snapshot.Reason != test.reason {
				t.Fatalf("reason = %q, want %q", snapshot.Reason, test.reason)
			}
		})
	}

	timeoutRunner := &fakeRunner{run: func(ctx context.Context) (CommandResult, error) {
		<-ctx.Done()
		return CommandResult{ExitCode: -1}, ctx.Err()
	}}
	launch := launchFor(t, t.TempDir())
	launch.InventoryTimeout = 20 * time.Millisecond
	snapshot := Discover(context.Background(), timeoutRunner, launch)
	if snapshot.Reason != ReasonTimeout {
		t.Fatalf("timeout reason = %q", snapshot.Reason)
	}

	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	snapshot = Discover(cancelledCtx, &fakeRunner{run: func(ctx context.Context) (CommandResult, error) {
		return CommandResult{ExitCode: -1}, ctx.Err()
	}}, launchFor(t, t.TempDir()))
	if snapshot.Reason != ReasonCancelled {
		t.Fatalf("cancelled reason = %q", snapshot.Reason)
	}
}

func TestPluginReadOnlyOperations(t *testing.T) {
	runner := &fakeRunner{result: CommandResult{Stdout: []byte(`[]`)}}
	launch := launchFor(t, t.TempDir())
	launch.CLIFlags = []string{"plugin", "enable", "harness"}
	snapshot := Discover(context.Background(), runner, launch)
	if snapshot.Status != StatusReady {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("calls = %d", len(runner.calls))
	}
	for _, arg := range runner.calls[0] {
		switch arg {
		case "enable", "disable", "install", "update", "uninstall":
			t.Fatalf("inventory invoked a mutating native operation: %#v", runner.calls[0])
		}
	}
	if !containsSequence(runner.calls[0], "plugin", "list") {
		t.Fatalf("inventory argv = %#v", runner.calls[0])
	}
}

func containsSequence(args []string, sequence ...string) bool {
	for index := 0; index+len(sequence) <= len(args); index++ {
		matches := true
		for offset, expected := range sequence {
			if args[index+offset] != expected {
				matches = false
				break
			}
		}
		if matches {
			return true
		}
	}
	return false
}

func TestPluginMetadataPrivacy(t *testing.T) {
	root := t.TempDir()
	writePlugin(t, root, "plugin.json", "spark", "1.0.0")
	configDir := t.TempDir()
	writeFile(t, filepath.Join(configDir, "config.json"), `{"installedPlugins":[{"name":"spark","marketplace":"m","version":"1.0.0","cache_path":`+quote(root)+`}]}`)
	encoded := []byte(`[{"name":"spark","marketplace":"m","version":"1.0.0","enabled":true,"source":"installed"}]`)
	snapshot := Discover(context.Background(), &fakeRunner{result: CommandResult{Stdout: encoded}}, launchFor(t, configDir))
	if snapshot.Rows[0].root == "" {
		t.Fatal("root was not resolved")
	}
	private, err := json.Marshal(snapshot.Rows[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(private), root) {
		t.Fatalf("resolved root leaked into JSON: %s", private)
	}
	if _, err := ResolveSelection(context.Background(), &fakeRunner{result: CommandResult{Stdout: []byte(`[]`)}}, launchFor(t, configDir), []string{"installed:m/spark"}); err == nil {
		t.Fatal("missing selection unexpectedly resolved")
	} else if strings.Contains(err.Error(), root) || strings.Contains(err.Error(), configDir) {
		t.Fatalf("error leaked a filesystem path: %v", err)
	}
}

func TestConfigDirPrecedence(t *testing.T) {
	launch := LaunchContext{Env: map[string]string{"HOME": "/home/example"}}
	dir, explicit, err := configDirForLaunch(launch)
	if err != nil || explicit || dir != filepath.Join("/home/example", ".copilot") {
		t.Fatalf("home fallback = %q explicit=%v err=%v", dir, explicit, err)
	}
	launch.Env["COPILOT_HOME"] = "/copilot/home"
	dir, explicit, err = configDirForLaunch(launch)
	if err != nil || !explicit || dir != "/copilot/home" {
		t.Fatalf("COPILOT_HOME = %q explicit=%v err=%v", dir, explicit, err)
	}
	launch.CLIFlags = []string{"--config-dir=/flag/home"}
	dir, explicit, err = configDirForLaunch(launch)
	if err != nil || !explicit || dir != "/flag/home" {
		t.Fatalf("flag = %q explicit=%v err=%v", dir, explicit, err)
	}
	launch.CLIFlags = []string{"--config-dir", "/a", "--config-dir=/b"}
	if _, _, err := configDirForLaunch(launch); err == nil {
		t.Fatal("ambiguous config dir unexpectedly accepted")
	}
}

func TestRuntimeSupportedVersions(t *testing.T) {
	for _, version := range []string{"1.0.88", "1.0.104"} {
		if !RuntimeSupported(version) {
			t.Fatalf("%s should be qualified", version)
		}
	}
	if RuntimeSupported("1.0.103") || RuntimeSupported("") {
		t.Fatal("unqualified runtime was accepted")
	}
}

func TestPluginSelectionValidation(t *testing.T) {
	valid := []string{"installed:gst-agentic-platform/harness", "live:fixture-market/other"}
	if err := ValidateSelection(valid); err != nil {
		t.Fatalf("valid selection rejected: %v", err)
	}
	if _, err := NormalizeSelection([]string{"live:b/a", "installed:c/d", "installed:c/d"}); err != nil {
		t.Fatalf("duplicate normalization: %v", err)
	}
	invalid := [][]string{
		{"not-an-id"},
		{"installed:a/b/c"},
		{"installed:a/.."},
		{"installed:a/%2e%2e"},
		{"installed:a/%2f"},
		{"Installed:a/b"},
		{"builtin:/browser-use"},
		{"external:/manual"},
		{"unknown:market/plugin"},
		{""},
	}
	for _, ids := range invalid {
		if err := ValidateSelection(ids); err == nil {
			t.Fatalf("invalid selection accepted: %#v", ids)
		}
	}
	tooMany := make([]string, maxSelectedPlugins+1)
	for index := range tooMany {
		tooMany[index] = "installed:m/p" + strings.Repeat("a", index%3)
	}
	if err := ValidateSelection(tooMany); err == nil {
		t.Fatal("oversized selection accepted")
	}
}

func TestSelectionTimeoutUsesRequestDeadline(t *testing.T) {
	runner := &fakeRunner{run: func(ctx context.Context) (CommandResult, error) {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > inventoryTimeout+time.Second {
			t.Fatalf("inventory deadline = %v, ok=%v", deadline, ok)
		}
		return CommandResult{Stdout: []byte(`[]`)}, nil
	}}
	Discover(context.Background(), runner, launchFor(t, t.TempDir()))
}

func TestCopilotPluginNativeAuthorityAdmission(t *testing.T) {
	for _, version := range []string{"1.0.88", "1.0.104"} {
		t.Run(version, func(t *testing.T) {
			root := t.TempDir()
			writePlugin(t, root, "plugin.json", "spark", "1.0.0")
			wantRoot, err := filepath.EvalSymlinks(root)
			if err != nil {
				t.Fatal(err)
			}
			configDir := t.TempDir()
			writeFile(t, filepath.Join(configDir, "config.json"),
				`{"installedPlugins":[{"name":"spark","marketplace":"m","version":"1.0.0","cache_path":`+quote(root)+`}]}`)
			runner := &fakeRunner{result: CommandResult{Stdout: []byte(
				`[{"name":"spark","marketplace":"m","version":"1.0.0","enabled":true,"source":"installed"}]`,
			)}}
			launch := launchFor(t, configDir)
			launch.RuntimeVersion = version
			launch.RuntimeCommand = []string{"npx", "--yes", "@github/copilot@" + version}

			prepared, err := ResolveLaunchSelection(context.Background(), runner, launch, []string{"installed:m/spark"})
			if err != nil {
				t.Fatalf("supported native preparation: %v", err)
			}
			if !RuntimeSupported(version) || !MountsPermitted(version) {
				t.Fatalf("version %s is not consistently eligible for discovery and mounting", version)
			}
			if len(prepared.Plugins) != 1 || prepared.Plugins[0].Root != wantRoot {
				t.Fatalf("prepared plugins = %#v, want verified root %q", prepared.Plugins, wantRoot)
			}
			if len(runner.calls) != 1 || runner.dirs[0] != launch.WorkDir {
				t.Fatalf("inventory calls=%d workdir=%q, want one call in %q", len(runner.calls), runner.dirs, launch.WorkDir)
			}
			if !containsSequence(runner.calls[0], "@github/copilot@"+version, "plugin", "list") {
				t.Fatalf("inventory argv = %#v, want selected runtime %s", runner.calls[0], version)
			}
		})
	}
}

func TestWrappedLaunchContextFailsBeforeNativeReads(t *testing.T) {
	launch := launchFor(t, t.TempDir())
	launch.CommandPrefix = []string{"sandbox", "--"}
	runner := &fakeRunner{result: CommandResult{Stdout: []byte(`[]`)}}
	snapshot := Discover(context.Background(), runner, launch)
	if snapshot.Status != StatusUnsupported || snapshot.Reason != ReasonUnsupportedLaunch {
		t.Fatalf("wrapped discovery = %#v", snapshot)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("wrapped discovery ran %d native commands", len(runner.calls))
	}
	_, err := ResolveLaunchSelection(context.Background(), runner, launch, []string{"installed:m/p"})
	var pluginErr *Error
	if !errors.As(err, &pluginErr) || pluginErr.Reason != ReasonUnsupportedLaunch {
		t.Fatalf("wrapped launch error = %#v", err)
	}
}

func TestRuntimeVersionFromCommand(t *testing.T) {
	version, ok := RuntimeVersionFromCommand([]string{"npx", "@github/copilot@1.0.104", "--acp"})
	if !ok || version != "1.0.104" {
		t.Fatalf("version = %q, ok = %v", version, ok)
	}
	if _, ok := RuntimeVersionFromCommand([]string{"copilot", "--acp"}); ok {
		t.Fatal("native binary command must not claim a managed package version")
	}
}

func TestRelativeConfigDirectoryIsRejected(t *testing.T) {
	cases := map[string]func(*LaunchContext){
		"copilot_home": func(launch *LaunchContext) {
			launch.Env["COPILOT_HOME"] = "./copilot"
		},
		"config_dir": func(launch *LaunchContext) {
			launch.Env["COPILOT_HOME"] = ""
			launch.CLIFlags = []string{"--config-dir=relative/copilot"}
		},
		"home": func(launch *LaunchContext) {
			delete(launch.Env, "COPILOT_HOME")
			launch.Env["HOME"] = "relative-home"
		},
	}
	for name, configure := range cases {
		t.Run(name, func(t *testing.T) {
			launch := launchFor(t, t.TempDir())
			configure(&launch)
			runner := &fakeRunner{result: CommandResult{Stdout: []byte(`[]`)}}
			snapshot := Discover(context.Background(), runner, launch)
			if snapshot.Status != StatusUnsupported || snapshot.Reason != ReasonUnsupportedLaunch {
				t.Fatalf("relative config snapshot = %#v", snapshot)
			}
			if len(runner.calls) != 0 {
				t.Fatalf("relative config ran %d native commands", len(runner.calls))
			}
			_, err := ResolveLaunchSelection(context.Background(), runner, launch, []string{"installed:m/p"})
			var pluginErr *Error
			if !errors.As(err, &pluginErr) || pluginErr.Reason != ReasonUnsupportedLaunch {
				t.Fatalf("relative config launch error = %#v", err)
			}
		})
	}
}
