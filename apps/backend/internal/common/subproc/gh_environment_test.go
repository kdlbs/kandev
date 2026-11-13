package subproc

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

const ghEnvironmentChildRunFlag = "-test.run=^TestGHEnvironmentChild$"

// @covers AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.19
// @covers AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.21
// @covers AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.22
func TestGHEnvironmentSessionBusSelection(t *testing.T) {
	const envKey = "DBUS_SESSION_BUS_ADDRESS"
	standardAddress := func(runtimeDir string) string {
		return "unix:path=" + path.Join(runtimeDir, "bus")
	}
	newRuntimeDir := func(t *testing.T) string {
		t.Helper()
		dir := filepath.Join(t.TempDir(), "run-user")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		return dir
	}

	t.Run("missing, empty, and autolaunch assignments choose an explicit standard address", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			env  []string
		}{
			{name: "missing"},
			{name: "empty", env: []string{envKey + "="}},
			{name: "autolaunch", env: []string{envKey + "=autolaunch:"}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				runtimeDir := newRuntimeDir(t)
				if got := selectGHSessionBusAddress(tc.env, runtimeDir); got != standardAddress(runtimeDir) {
					t.Fatalf("selected bus address was not the explicit standard address for %s", tc.name)
				}
			})
		}
	})

	t.Run("explicit addresses remain authoritative", func(t *testing.T) {
		for _, address := range []string{"unix:path=/tmp/live-user-bus", "unix:path=/tmp/unreachable-user-bus"} {
			runtimeDir := newRuntimeDir(t)
			if err := os.WriteFile(path.Join(runtimeDir, "bus"), nil, 0o600); err != nil {
				t.Fatal(err)
			}
			if got := selectGHSessionBusAddress([]string{envKey + "=" + address}, runtimeDir); got != address {
				t.Fatal("explicit user bus address was replaced")
			}
		}
	})

	t.Run("standard bus wins over legacy address file", func(t *testing.T) {
		runtimeDir := newRuntimeDir(t)
		if err := os.WriteFile(path.Join(runtimeDir, "bus"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
		legacyPath := path.Join(runtimeDir, "dbus-session")
		if err := os.WriteFile(legacyPath, []byte(envKey+"=unix:path=/tmp/legacy-bus\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		got := selectGHSessionBusAddress([]string{envKey + "=autolaunch:"}, runtimeDir)
		if got != standardAddress(runtimeDir) {
			t.Fatal("standard user bus did not take precedence over the legacy address")
		}
	})

	t.Run("legacy address is parsed without evaluating shell syntax", func(t *testing.T) {
		runtimeDir := newRuntimeDir(t)
		const legacyAddress = "unix:path=/tmp/legacy-bus,guid=1234"
		contents := envKey + "=" + legacyAddress + "\r\n"
		if err := os.WriteFile(path.Join(runtimeDir, "dbus-session"), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := selectGHSessionBusAddress(nil, runtimeDir); got != legacyAddress {
			t.Fatal("valid legacy user bus address was not selected")
		}
	})

	t.Run("invalid legacy files fall back to standard address", func(t *testing.T) {
		for _, tc := range []struct {
			name       string
			contents   []byte
			unreadable bool
		}{
			{name: "missing"},
			{name: "unreadable", unreadable: true},
			{name: "malformed", contents: []byte("DBUS_ADDRESS=unix:path=/tmp/ignored\n")},
			{name: "empty", contents: []byte(envKey + "=\n")},
			{name: "autolaunch", contents: []byte(envKey + "=autolaunch:\n")},
		} {
			t.Run(tc.name, func(t *testing.T) {
				runtimeDir := newRuntimeDir(t)
				legacyPath := path.Join(runtimeDir, "dbus-session")
				if tc.unreadable {
					makeGHLegacyAddressUnreadable(t, legacyPath)
				} else if tc.contents != nil {
					if err := os.WriteFile(legacyPath, tc.contents, 0o600); err != nil {
						t.Fatal(err)
					}
				}
				got := selectGHSessionBusAddress([]string{envKey + "=autolaunch:"}, runtimeDir)
				if got != standardAddress(runtimeDir) {
					t.Fatal("invalid legacy address did not fall back to the explicit standard address")
				}
			})
		}
	})

	t.Run("later bus appearance is observed and a selected address stays explicit", func(t *testing.T) {
		runtimeDir := newRuntimeDir(t)
		const legacyAddress = "unix:path=/tmp/legacy-bus"
		legacyPath := path.Join(runtimeDir, "dbus-session")
		if err := os.WriteFile(legacyPath, []byte(envKey+"="+legacyAddress+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := selectGHSessionBusAddress([]string{envKey + "=autolaunch:"}, runtimeDir); got != legacyAddress {
			t.Fatal("legacy address was not selected before the standard bus appeared")
		}
		busPath := path.Join(runtimeDir, "bus")
		if err := os.WriteFile(busPath, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		selected := selectGHSessionBusAddress([]string{envKey + "=autolaunch:"}, runtimeDir)
		if selected != standardAddress(runtimeDir) {
			t.Fatal("later standard user bus was not selected")
		}
		if err := os.Remove(busPath); err != nil {
			t.Fatal(err)
		}
		if got := selectGHSessionBusAddress([]string{envKey + "=" + selected}, runtimeDir); got != selected {
			t.Fatal("selected explicit address changed after its socket disappeared")
		}
	})

	cmd := exec.Command("unused-gh-command")
	cmd.Env = []string{envKey + "=autolaunch:"}
	before := append([]string(nil), cmd.Env...)
	prepareGHCommand(cmd)
	if runtime.GOOS == "linux" {
		if got, ok := environmentValue(cmd.Env, envKey); !ok || got == "" || got == "autolaunch:" {
			t.Fatal("Linux command did not receive an explicit session bus address")
		}
	} else if !reflect.DeepEqual(cmd.Env, before) {
		t.Fatal("non-Linux command environment was changed")
	}
}

func makeGHLegacyAddressUnreadable(t *testing.T, legacyPath string) {
	t.Helper()
	if err := os.WriteFile(legacyPath, []byte("DBUS_SESSION_BUS_ADDRESS=unix:path=/tmp/private-bus\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(legacyPath, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadFile(legacyPath); err == nil {
		t.Skip("filesystem permissions are not enforced for this test process")
	}
}

// @covers AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.19
// @covers AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.20
// @covers AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.21
// @covers AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.22
func TestRunGHHelpersApplySessionBusPolicy(t *testing.T) {
	const (
		dbusEnv = "DBUS_SESSION_BUS_ADDRESS"
		homeEnv = "HOME"
	)
	parent := map[string]string{
		homeEnv:                   filepath.Join(t.TempDir(), "parent-home"),
		"PATH":                    os.Getenv("PATH"),
		"GH_CONFIG_DIR":           filepath.Join(t.TempDir(), "parent-gh-config"),
		"GH_HOST":                 "parent-enterprise.example",
		"GH_TOKEN":                "parent-gh-token-sentinel",
		"GITHUB_TOKEN":            "parent-github-token-sentinel",
		"GH_ENTERPRISE_TOKEN":     "parent-gh-enterprise-token-sentinel",
		"GITHUB_ENTERPRISE_TOKEN": "parent-github-enterprise-token-sentinel",
		dbusEnv:                   "autolaunch:",
	}
	for key, value := range parent {
		t.Setenv(key, value)
	}

	keys := []string{
		homeEnv,
		"PATH",
		"GH_CONFIG_DIR",
		"GH_HOST",
		"GH_TOKEN",
		"GITHUB_TOKEN",
		"GH_ENTERPRISE_TOKEN",
		"GITHUB_ENTERPRISE_TOKEN",
		dbusEnv,
	}
	explicit := []string{
		homeEnv + "=" + filepath.Join(t.TempDir(), "explicit-home"),
		"PATH=" + os.Getenv("PATH"),
		"GH_CONFIG_DIR=" + filepath.Join(t.TempDir(), "explicit-gh-config"),
		"GH_HOST=explicit-enterprise.example",
		"GH_TOKEN=explicit-gh-token-sentinel",
		"GITHUB_TOKEN=explicit-github-token-sentinel",
		"GH_ENTERPRISE_TOKEN=explicit-gh-enterprise-token-sentinel",
		"GITHUB_ENTERPRISE_TOKEN=explicit-github-enterprise-token-sentinel",
		dbusEnv + "=unix:path=/tmp/explicit-user-bus",
	}

	type runner func([]string) ([]byte, error)
	command := func(env []string) *exec.Cmd {
		cmd := exec.Command(os.Args[0], ghEnvironmentChildRunFlag)
		cmd.Env = env
		return cmd
	}
	runners := []struct {
		name string
		run  runner
	}{
		{
			name: "RunGH",
			run: func(env []string) ([]byte, error) {
				var stdout bytes.Buffer
				cmd := command(env)
				cmd.Stdout = &stdout
				err := RunGH(context.Background(), cmd)
				return stdout.Bytes(), err
			},
		},
		{
			name: "RunGHOutput",
			run: func(env []string) ([]byte, error) {
				return RunGHOutput(context.Background(), command(env))
			},
		},
		{
			name: "RunGHCombinedOutput",
			run: func(env []string) ([]byte, error) {
				return RunGHCombinedOutput(context.Background(), command(env))
			},
		},
		{
			name: "RunGHAfterAcquire",
			run: func(env []string) ([]byte, error) {
				var stdout bytes.Buffer
				runErr, execCtxErr := RunGHAfterAcquire(
					context.Background(),
					10*time.Second,
					func(execCtx context.Context) *exec.Cmd {
						cmd := exec.CommandContext(execCtx, os.Args[0], ghEnvironmentChildRunFlag)
						cmd.Env = env
						cmd.Stdout = &stdout
						return cmd
					},
				)
				if execCtxErr != nil {
					return stdout.Bytes(), execCtxErr
				}
				return stdout.Bytes(), runErr
			},
		},
		{
			name: "RunGHCombinedAfterAcquire",
			run: func(env []string) ([]byte, error) {
				out, runErr, execCtxErr := RunGHCombinedAfterAcquire(
					context.Background(),
					10*time.Second,
					func(execCtx context.Context) *exec.Cmd {
						cmd := exec.CommandContext(execCtx, os.Args[0], ghEnvironmentChildRunFlag)
						cmd.Env = env
						return cmd
					},
				)
				if execCtxErr != nil {
					return out, execCtxErr
				}
				return out, runErr
			},
		},
	}

	for _, tc := range runners {
		for _, envCase := range []struct {
			name string
			env  []string
			want map[string]string
		}{
			{name: "inherited environment", env: nil, want: parent},
			{name: "explicit environment", env: explicit, want: environmentMap(keys, explicit)},
		} {
			t.Run(tc.name+"/"+envCase.name, func(t *testing.T) {
				out, err := tc.run(envCase.env)
				if err != nil {
					t.Fatalf("%s failed: %v", tc.name, err)
				}
				var got map[string]string
				if err := json.NewDecoder(bytes.NewReader(out)).Decode(&got); err != nil {
					t.Fatalf("decode child environment: %v", err)
				}
				want := cloneEnvironmentMap(envCase.want)
				if runtime.GOOS == "linux" && want[dbusEnv] == "autolaunch:" {
					want[dbusEnv] = selectGHSessionBusAddress(os.Environ(), ghRuntimeDirectory())
				}
				if !sameEnvironmentValues(got, want) {
					t.Errorf("child environment did not contain the expected managed values")
					for _, key := range keys {
						if got[key] != want[key] {
							t.Errorf("child environment did not preserve %s", key)
						}
					}
				}
				for key, value := range parent {
					if os.Getenv(key) != value {
						t.Errorf("parent environment changed for %s", key)
					}
				}
			})
		}
	}
}

func TestGHEnvironmentChild(t *testing.T) {
	for _, arg := range os.Args {
		if arg == ghEnvironmentChildRunFlag {
			values := make(map[string]string)
			for _, key := range []string{
				"HOME",
				"PATH",
				"GH_CONFIG_DIR",
				"GH_HOST",
				"GH_TOKEN",
				"GITHUB_TOKEN",
				"GH_ENTERPRISE_TOKEN",
				"GITHUB_ENTERPRISE_TOKEN",
				"DBUS_SESSION_BUS_ADDRESS",
			} {
				if value, ok := os.LookupEnv(key); ok {
					values[key] = value
				}
			}
			if err := json.NewEncoder(os.Stdout).Encode(values); err != nil {
				t.Fatal("could not encode child environment")
			}
			return
		}
	}
}

func environmentMap(keys []string, env []string) map[string]string {
	values := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := environmentValue(env, key); ok {
			values[key] = value
		}
	}
	return values
}

func cloneEnvironmentMap(source map[string]string) map[string]string {
	copy := make(map[string]string, len(source))
	for key, value := range source {
		copy[key] = value
	}
	return copy
}

func sameEnvironmentValues(got, want map[string]string) bool {
	if len(got) != len(want) {
		return false
	}
	for key, value := range want {
		if got[key] != value {
			return false
		}
	}
	return true
}

func TestGHEnvironmentAssignmentReplacementRemovesDuplicates(t *testing.T) {
	got := replaceEnvironmentAssignment([]string{
		"A=first",
		"DBUS_SESSION_BUS_ADDRESS=old",
		"B=value",
		"DBUS_SESSION_BUS_ADDRESS=last",
	}, "DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/1000/bus")
	want := []string{"A=first", "B=value", "DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/1000/bus"}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("D-Bus assignment replacement did not remove earlier values")
	}
	if strings.Contains(strings.Join(got, "\n"), "old") || strings.Contains(strings.Join(got, "\n"), "last") {
		t.Fatal("stale D-Bus assignment remained")
	}
}
