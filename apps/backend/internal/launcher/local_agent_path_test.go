package launcher

import (
	"context"
	"encoding/xml"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
)

// @covers AC-PLATFORM-LOCAL-AGENT-PATH-001.1 AC-PLATFORM-LOCAL-AGENT-PATH-001.2 AC-PLATFORM-LOCAL-AGENT-PATH-001.3
func TestLocalAgentPathPostInstall(t *testing.T) {
	home, env := localAgentEnvironment(t, "/usr/bin:/bin")
	installLocalAgentFixture(t, home)
	runLocalAgentChild(t, env)
}

func TestLocalAgentPathLaunchd(t *testing.T) {
	plist := renderLaunchdPlist(nativeServiceUnitInput{Executable: "/opt/kandev/bin/kandev", HomeDir: t.TempDir()})
	// EnvironmentVariables is a nested dictionary; decode its serialized producer output.
	decoder := xml.NewDecoder(strings.NewReader(plist))
	var path string
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "key" {
			continue
		}
		var key string
		if err := decoder.DecodeElement(&key, &start); err != nil {
			t.Fatal(err)
		}
		if key != "PATH" {
			continue
		}
		if err := decoder.Decode(&path); err != nil {
			t.Fatal(err)
		}
		break
	}
	if path == "" {
		t.Fatal("launchd plist has no PATH")
	}
	home, env := localAgentEnvironment(t, path)
	installLocalAgentFixture(t, home)
	runLocalAgentChild(t, env)
}

func localAgentEnvironment(t *testing.T, path string) (string, []string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Unix executable fixtures")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", path)
	t.Setenv("KANDEV_HOME_DIR", t.TempDir())
	env := backendEnv(portConfig{}, "", "", false, "test", nil)
	return home, upsertEnv(env, "KANDEV_TEST_LOCAL_AGENT_CHILD", "1")
}

func installLocalAgentFixture(t *testing.T, home string) {
	t.Helper()
	bin := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"agent", "cursor-agent"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nprintf cursor-ok\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func runLocalAgentChild(t *testing.T, env []string) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestLocalAgentPathChild$")
	cmd.Env = env
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("child discovery/execution: %v\n%s", err, output)
	}
}

func TestLocalAgentPathChild(t *testing.T) {
	if os.Getenv("KANDEV_TEST_LOCAL_AGENT_CHILD") != "1" {
		return
	}
	result, err := agents.NewCursorACP().IsInstalled(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Available {
		t.Fatal("installed Cursor is invisible under launcher-produced PATH")
	}
	for _, name := range []string{"agent", "cursor-agent"} {
		output, err := exec.CommandContext(t.Context(), name).Output()
		if err != nil || string(output) != "cursor-ok" {
			t.Fatalf("execute %s: %q, %v", name, output, err)
		}
	}
}

// @covers AC-PLATFORM-LOCAL-AGENT-PATH-001.4 AC-PLATFORM-LOCAL-AGENT-PATH-001.5
func TestLocalAgentPathEnvironment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Setenv("PATH", `C:\tools`)
		env := backendEnv(portConfig{}, "", "", false, "test", nil)
		if got := processEnvValue(env, "PATH"); got != `C:\tools` {
			t.Fatalf("Windows PATH changed: %q", got)
		}
		return
	}
	home := t.TempDir()
	local := filepath.Join(home, ".local", "bin")
	cases := []struct{ name, home, path, want string }{
		{"append", home, "/usr/bin:/bin", "/usr/bin:/bin:" + local},
		{"already present", home, local + ":/bin", local + ":/bin"},
		{"equivalent directory", home, local + "/:/bin", local + "/:/bin"},
		{"empty path", home, "", local},
		{"missing home", "", "/bin", "/bin"},
		{"relative home", "relative", "/bin", "/bin"},
		{"separator in home", home + ":other", "/bin", "/bin"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", tc.home)
			t.Setenv("PATH", tc.path)
			env := backendEnv(portConfig{}, "", "", false, "test", nil)
			if got := processEnvValue(env, "PATH"); got != tc.want {
				t.Fatalf("PATH = %q, want %q", got, tc.want)
			}
		})
	}
	t.Run("final overrides", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		env := backendEnv(portConfig{}, "", "", false, "test", []string{"HOME=" + home, "PATH=/override"})
		want := "/override:" + local
		if got := processEnvValue(env, "PATH"); got != want {
			t.Fatalf("PATH = %q, want %q", got, want)
		}
	})
}

func TestLocalAgentPathUnavailable(t *testing.T) {
	home, env := localAgentEnvironment(t, t.TempDir())
	t.Setenv("PATH", processEnvValue(env, "PATH"))
	assertUnavailable := func() {
		t.Helper()
		result, err := agents.NewCursorACP().IsInstalled(t.Context())
		if err != nil || result.Available {
			t.Fatalf("missing/non-executable Cursor: %+v, %v", result, err)
		}
	}
	assertUnavailable()
	installLocalAgentFixture(t, home)
	if err := os.Chmod(filepath.Join(home, ".local", "bin", "cursor-agent"), 0o644); err != nil {
		t.Fatal(err)
	}
	assertUnavailable()
}

func TestLocalAgentPathPreservesExecutablePrecedence(t *testing.T) {
	preferred := t.TempDir()
	home, env := localAgentEnvironment(t, preferred)
	installLocalAgentFixture(t, home)
	want := filepath.Join(preferred, "cursor-agent")
	if err := os.WriteFile(want, []byte("#!/bin/sh\nprintf preferred\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", processEnvValue(env, "PATH"))
	result, err := agents.NewCursorACP().IsInstalled(t.Context())
	if err != nil || result.MatchedPath != want {
		t.Fatalf("discovery = %+v, %v; want %s", result, err, want)
	}
	output, err := exec.CommandContext(t.Context(), "cursor-agent").Output()
	if err != nil || string(output) != "preferred" {
		t.Fatalf("preferred executable: %q, %v", output, err)
	}
	again := backendEnv(portConfig{}, "", "", false, "test", nil)
	if got := processEnvValue(again, "PATH"); got != processEnvValue(env, "PATH") {
		t.Fatalf("repeated normalization changed PATH: %q", got)
	}
}

func TestLocalAgentPathRestart(t *testing.T) {
	home, env := localAgentEnvironment(t, "/usr/bin:/bin")
	installLocalAgentFixture(t, home)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	supervisor := newSupervisor()
	t.Cleanup(func() { supervisor.shutdown("test cleanup") })
	backend := &restartableBackend{
		command:    executable,
		args:       []string{"-test.run=^TestLocalAgentPathChild$"},
		env:        env,
		quiet:      true,
		supervisor: supervisor,
		exitCh:     make(chan int, 1),
	}
	if err := backend.start(); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		select {
		case code := <-backend.exitCh:
			if code != 0 {
				t.Fatalf("backend launch %d failed with exit %d", attempt, code)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("backend subprocess did not finish")
		}
		if attempt == 0 {
			backend.restart()
		}
	}
}
