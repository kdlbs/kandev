package copilotplugins

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/common/managednpm"
)

func TestCopilotPluginCompatibleUnknownRuntime(t *testing.T) {
	root := t.TempDir()
	writePlugin(t, root, "plugin.json", "spark", "1.0.0")
	configDir := t.TempDir()
	writeFile(t, filepath.Join(configDir, "config.json"),
		`{"installedPlugins":[{"name":"spark","marketplace":"m","version":"1.0.0","cache_path":`+quote(root)+`}]}`)
	launch := launchFor(t, configDir)
	launch.RuntimeVersion = "9.9.9"
	launch.RuntimeCommand = []string{"npx", "--yes", "@github/copilot@9.9.9"}
	runner := &fakeRunner{run: func(_ context.Context, call []string) (CommandResult, error) {
		switch {
		case containsSequence(call, "--help"):
			return CommandResult{Stdout: []byte("Options:\n  --plugin-dir <path>  Load plugin directories\n"), ExitCode: 0}, nil
		case containsSequence(call, "plugin", "list"):
			return CommandResult{Stdout: []byte(`[{"name":"spark","marketplace":"m","version":"1.0.0","enabled":true,"source":"installed"}]`), ExitCode: 0}, nil
		default:
			return CommandResult{ExitCode: 1}, nil
		}
	}}

	snapshot := Discover(context.Background(), runner, launch)
	if snapshot.Status != StatusReady || !snapshot.PluginLoadingSupported {
		t.Fatalf("unknown compatible runtime discovery status = %q reason = %q, want ready", snapshot.Status, snapshot.Reason)
	}
	prepared, err := ResolveLaunchSelection(context.Background(), runner, launch, []string{"installed:m/spark"})
	if err != nil {
		t.Fatalf("unknown compatible runtime admission: %v", err)
	}
	wantRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.Plugins) != 1 || prepared.Plugins[0].Root != wantRoot {
		t.Fatalf("prepared plugins = %#v, want verified root %q", prepared.Plugins, wantRoot)
	}
	if len(runner.helpCalls) != 2 || len(runner.calls) != 2 {
		t.Fatalf("capability probes = %d inventory calls = %d, want two each", len(runner.helpCalls), len(runner.calls))
	}
	for _, call := range runner.helpCalls {
		if !containsSequence(call, "@github/copilot@9.9.9", "--help") {
			t.Fatalf("capability argv = %#v, want the selected runtime", call)
		}
	}
}

func TestCopilotPluginContractFailures(t *testing.T) {
	launch := launchFor(t, t.TempDir())
	launch.RuntimeVersion = "8.7.6"
	launch.RuntimeCommand = []string{"npx", "--yes", "@github/copilot@8.7.6"}

	t.Run("missing plugin loading capability", func(t *testing.T) {
		runner := compatibilityTestRunner([]byte("Usage: copilot [options]\n"), []byte(`[]`))
		snapshot := Discover(context.Background(), runner, launch)
		if snapshot.Status != StatusUnsupported || snapshot.Reason != ReasonPluginLoadingUnsupported {
			t.Fatalf("snapshot = %#v, want bounded capability failure", snapshot)
		}
		if snapshot.PluginLoadingSupported || len(runner.calls) != 0 {
			t.Fatalf("missing capability ran inventory or became eligible: %#v calls=%d", snapshot, len(runner.calls))
		}
	})

	t.Run("malformed inventory keeps its format reason", func(t *testing.T) {
		runner := compatibilityTestRunner([]byte("Options:\n  --plugin-dir <path>\n"), []byte(`{"plugins":[]}`))
		snapshot := Discover(context.Background(), runner, launch)
		if snapshot.Status != StatusUnavailable || snapshot.Reason != ReasonInvalidInventory || !snapshot.PluginLoadingSupported {
			t.Fatalf("snapshot = %#v, want observed capability and invalid inventory", snapshot)
		}
	})

	t.Run("manifest mismatch rejects selected root", func(t *testing.T) {
		badRoot := t.TempDir()
		writePlugin(t, badRoot, "plugin.json", "other", "1.0.0")
		configDir := t.TempDir()
		writeFile(t, filepath.Join(configDir, "config.json"),
			`{"installedPlugins":[{"name":"spark","marketplace":"m","version":"1.0.0","cache_path":`+quote(badRoot)+`}]}`)
		badLaunch := launch
		badLaunch.Env = map[string]string{"HOME": t.TempDir(), "COPILOT_HOME": configDir, "PATH": os.Getenv("PATH")}
		badLaunch.CLIFlags = nil
		runner := compatibilityTestRunner([]byte("Options:\n  --plugin-dir <path>\n"), []byte(`[{"name":"spark","marketplace":"m","version":"1.0.0","enabled":true,"source":"installed"}]`))
		_, err := ResolveLaunchSelection(context.Background(), runner, badLaunch, []string{"installed:m/spark"})
		var pluginErr *Error
		if !errors.As(err, &pluginErr) || pluginErr.Reason != ReasonManifestMismatch {
			t.Fatalf("selected manifest error = %#v, want manifest_mismatch", err)
		}
	})

	t.Run("mixed selected roots reject atomically", func(t *testing.T) {
		validRoot := t.TempDir()
		writePlugin(t, validRoot, "plugin.json", "spark", "1.0.0")
		configDir := t.TempDir()
		writeFile(t, filepath.Join(configDir, "config.json"),
			`{"installedPlugins":[{"name":"spark","marketplace":"m","version":"1.0.0","cache_path":`+quote(validRoot)+`}]}`)
		mixedLaunch := launch
		mixedLaunch.Env = map[string]string{"HOME": t.TempDir(), "COPILOT_HOME": configDir, "PATH": os.Getenv("PATH")}
		mixedLaunch.CLIFlags = nil
		rows := []byte(`[{"name":"spark","marketplace":"m","version":"1.0.0","enabled":true,"source":"installed"}]`)
		runner := compatibilityTestRunner([]byte("Options:\n  --plugin-dir <path>\n"), rows)
		prepared, err := ResolveLaunchSelection(context.Background(), runner, mixedLaunch,
			[]string{"installed:m/spark", "installed:m/missing"})
		var pluginErr *Error
		if !errors.As(err, &pluginErr) || pluginErr.Reason != ReasonMetadataUnavailable || pluginErr.PluginID != "installed:m/missing" {
			t.Fatalf("mixed selection error = %#v, want missing selected identity", err)
		}
		if len(prepared.Plugins) != 0 {
			t.Fatalf("mixed selection returned a partial preparation: %#v", prepared)
		}
	})

	t.Run("invalid unselected row does not reject valid selection", func(t *testing.T) {
		validRoot := t.TempDir()
		writePlugin(t, validRoot, "plugin.json", "spark", "1.0.0")
		configDir := t.TempDir()
		writeFile(t, filepath.Join(configDir, "config.json"),
			`{"installedPlugins":[{"name":"spark","marketplace":"m","version":"1.0.0","cache_path":`+quote(validRoot)+`}]}`)
		selectedLaunch := launch
		selectedLaunch.Env = map[string]string{"HOME": t.TempDir(), "COPILOT_HOME": configDir, "PATH": os.Getenv("PATH")}
		selectedLaunch.CLIFlags = nil
		rows := []byte(`[
			{"name":"spark","marketplace":"m","version":"1.0.0","enabled":true,"source":"installed"},
			{"name":"broken","marketplace":"m","version":"1.0.0","enabled":true,"source":"installed"}
		]`)
		runner := compatibilityTestRunner([]byte("Options:\n  --plugin-dir <path>\n"), rows)
		prepared, err := ResolveLaunchSelection(context.Background(), runner, selectedLaunch, []string{"installed:m/spark"})
		if err != nil {
			t.Fatalf("valid selected root rejected by unrelated row: %v", err)
		}
		if len(prepared.Plugins) != 1 {
			t.Fatalf("prepared plugins = %#v, want only selected valid root", prepared.Plugins)
		}
	})
}

func compatibilityTestRunner(help, inventory []byte) *fakeRunner {
	return &fakeRunner{run: func(_ context.Context, argv []string) (CommandResult, error) {
		if containsSequence(argv, "--help") {
			return CommandResult{Stdout: help, ExitCode: 0}, nil
		}
		return CommandResult{Stdout: inventory, ExitCode: 0}, nil
	}}
}

func TestManagedRuntimeProbeUsesIsolatedNPMProject(t *testing.T) {
	tempRoot := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("TEMP", tempRoot)
		t.Setenv("TMP", tempRoot)
	} else {
		t.Setenv("TMPDIR", tempRoot)
	}
	home := t.TempDir()
	workDir := t.TempDir()
	configDir := t.TempDir()
	launch := launchFor(t, configDir)
	launch.Env = map[string]string{"HOME": home, "COPILOT_HOME": configDir, "PATH": os.Getenv("PATH")}
	launch.WorkDir = workDir
	launch.RuntimeVersion = "9.9.9"
	launch.RuntimeCommand = agents.NewCopilotACP().ManagedNPMRuntime().InteractiveCommand("9.9.9").Args()
	runner := compatibilityTestRunner(
		[]byte("Options:\n  --plugin-dir <path>  Load plugin directories\n"),
		[]byte(`[]`),
	)

	snapshot := Discover(context.Background(), runner, launch)
	if snapshot.Status != StatusReady {
		t.Fatalf("managed runtime discovery = %#v", snapshot)
	}
	if len(runner.helpCalls) != 1 || len(runner.calls) != 1 {
		t.Fatalf("managed runtime commands = help:%d inventory:%d, want one each", len(runner.helpCalls), len(runner.calls))
	}
	var resolvedPrefix string
	for _, argv := range [][]string{runner.helpCalls[0], runner.calls[0]} {
		index := -1
		for position, arg := range argv {
			if arg == "--prefix" && position+1 < len(argv) {
				index = position + 1
				break
			}
		}
		if index < 0 || !filepath.IsAbs(argv[index]) || argv[index] == managednpm.NPMProjectPrefix {
			t.Fatalf("managed runtime argv = %#v, want a resolved private npm prefix", argv)
		}
		relative, err := filepath.Rel(tempRoot, argv[index])
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			t.Fatalf("managed npm prefix %q is outside temp root %q", argv[index], tempRoot)
		}
		if resolvedPrefix == "" {
			resolvedPrefix = argv[index]
		} else if argv[index] != resolvedPrefix {
			t.Fatalf("help and inventory npm prefixes differ: %q / %q", resolvedPrefix, argv[index])
		}
	}
	for _, path := range []string{
		filepath.Join(home, ".kandev", "managed-npm-runtime"),
		filepath.Join(workDir, "node_modules"),
		filepath.Join(workDir, "package.json"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("probe created runtime project state at %q, stat error = %v", path, err)
		}
	}
}

func TestCopilotPluginProbeCancellationDoesNotPublishPartialInventory(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inventoryStarted := make(chan struct{})
	runner := &fakeRunner{run: func(runCtx context.Context, argv []string) (CommandResult, error) {
		if containsSequence(argv, "--help") {
			return CommandResult{Stdout: []byte("Options:\n  --plugin-dir <path>\n"), ExitCode: 0}, nil
		}
		close(inventoryStarted)
		<-runCtx.Done()
		return CommandResult{ExitCode: -1}, runCtx.Err()
	}}
	done := make(chan Snapshot, 1)
	launch := launchFor(t, t.TempDir())
	go func() { done <- Discover(ctx, runner, launch) }()
	<-inventoryStarted
	cancel()
	snapshot := <-done
	if snapshot.Status != StatusUnavailable || snapshot.Reason != ReasonCancelled || !snapshot.PluginLoadingSupported {
		t.Fatalf("cancelled probe = %#v, want cancelled after observing capability without inventory", snapshot)
	}
	if len(snapshot.Rows) != 0 || len(runner.calls) != 1 || len(runner.helpCalls) != 1 {
		t.Fatalf("cancelled probe published partial inventory: snapshot=%#v help=%d inventory=%d", snapshot, len(runner.helpCalls), len(runner.calls))
	}
}
