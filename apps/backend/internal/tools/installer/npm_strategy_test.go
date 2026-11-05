package installer

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type npmProcessRunner struct {
	env      map[string]string
	registry string
	path     string
}

func (r *npmProcessRunner) CommandEnvironment() (map[string]string, error) {
	return r.env, nil
}

func (r *npmProcessRunner) CombinedOutput(ctx context.Context, spec CommandSpec) ([]byte, error) {
	r.path = spec.Path
	env := os.Environ()
	for key, value := range spec.Env {
		env = upsertCommandEnv(env, key, value)
	}
	config := exec.CommandContext(ctx, spec.Path, "config", "get", "registry")
	config.Dir, config.Env = spec.Dir, env
	output, err := config.CombinedOutput()
	if err != nil {
		return output, err
	}
	r.registry = strings.TrimSpace(string(output))
	cmd := exec.CommandContext(ctx, spec.Path, spec.Args...)
	cmd.Dir, cmd.Env = spec.Dir, env
	return cmd.CombinedOutput()
}

// @covers AC-PLATFORM-LSP-FILE-INTELLIGENCE-001.9
func TestNpmStrategyRealNpmLocalInstall(t *testing.T) {
	if os.Getenv("KANDEV_TEST_NPM_INTEGRATION") != "1" {
		t.Skip("set KANDEV_TEST_NPM_INTEGRATION=1 to exercise real npm")
	}
	npmPath, err := exec.LookPath("npm")
	if err != nil {
		t.Fatalf("npm is required when integration test is enabled: %v", err)
	}
	if runtime.GOOS == windowsOS {
		t.Skip("real npm integration does not invoke Windows npm.cmd; platform shim lookup has separate coverage")
	}
	taskPath := os.Getenv("PATH")
	for _, scenario := range []struct {
		name, global, location string
	}{
		{name: "local", global: "false", location: "project"},
		{name: "global-flag", global: "true", location: "project"},
		{name: "global-location", global: "false", location: "global"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			root := t.TempDir()
			fixture := filepath.Join(root, "fixture")
			if err := os.MkdirAll(fixture, 0o755); err != nil {
				t.Fatal(err)
			}
			writeNpmFixture(t, fixture)
			prefix := filepath.Join(root, "managed")
			userConfig := filepath.Join(root, "user.npmrc")
			globalConfig := filepath.Join(root, "global.npmrc")
			for _, path := range []string{userConfig, globalConfig} {
				if err := os.WriteFile(path, nil, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("PATH", t.TempDir())
			t.Setenv("NPM_CONFIG_GLOBAL", scenario.global)
			t.Setenv("NPM_CONFIG_LOCATION", scenario.location)
			const registry = "http://127.0.0.1:9/"
			runner := &npmProcessRunner{env: map[string]string{
				"PATH": taskPath, "NPM_CONFIG_USERCONFIG": userConfig,
				"NPM_CONFIG_GLOBALCONFIG": globalConfig, "NPM_CONFIG_CACHE": filepath.Join(root, "cache"),
				"NPM_CONFIG_REGISTRY": registry, "NPM_CONFIG_OFFLINE": "true",
				"NPM_CONFIG_AUDIT": "false", "NPM_CONFIG_FUND": "false",
			}}
			strategy := NewNpmStrategy(prefix, "fixture-server", []string{fixture}, installStrategyTestLogger(t), runner)
			result, err := strategy.Install(context.Background())
			if err != nil {
				t.Fatalf("Install() under global=%s location=%s: %v", scenario.global, scenario.location, err)
			}
			want := filepath.Join(prefix, "node_modules", ".bin", "fixture-server")
			if result.BinaryPath != want {
				t.Errorf("BinaryPath = %q, want %q", result.BinaryPath, want)
			}
			if _, err := os.Stat(want); err != nil {
				t.Errorf("local executable missing: %v", err)
			}
			if _, err := os.Stat(filepath.Join(prefix, "bin", "fixture-server")); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("unexpected global-layout executable: %v", err)
			}
			if runner.path != npmPath || runner.registry != registry {
				t.Errorf("runner selected npm %q and registry %q, want %q and %q", runner.path, runner.registry, npmPath, registry)
			}
		})
	}
}

func writeNpmFixture(t *testing.T, dir string) {
	t.Helper()
	files := map[string]string{
		"package.json": `{"name":"fixture-server-package","version":"1.0.0","bin":{"fixture-server":"server.js"}}`,
		"server.js":    "#!/usr/bin/env node\nprocess.exit(0);\n",
	}
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}
