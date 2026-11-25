package github

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

const (
	ghAccountDBusFixtureModeEnv = "KANDEV_GH_ACCOUNT_DBUS_FIXTURE_MODE"
	ghAccountDBusTokenFileEnv   = "KANDEV_GH_ACCOUNT_DBUS_TOKEN_FILE"
	ghAccountDBusMarkerFileEnv  = "KANDEV_GH_ACCOUNT_DBUS_MARKER_FILE"
)

func init() {
	switch os.Getenv(ghAccountDBusFixtureModeEnv) {
	case "gh":
		exitGHAccountDBusFixture(os.Args[1:])
	case "dbus-launch":
		exitGHAccountDBusLauncherFixture(os.Args[1:])
	}
}

func exitGHAccountDBusFixture(args []string) {
	if !reflect.DeepEqual(args, []string{"auth", "token", "--help"}) &&
		!reflect.DeepEqual(args, []string{"auth", "token", "--hostname", "github.com", "--user", "alice"}) {
		os.Exit(91)
	}

	address, ok := os.LookupEnv("DBUS_SESSION_BUS_ADDRESS")
	if !ok || address == "" || address == "autolaunch:" {
		executable, err := os.Executable()
		if err != nil {
			os.Exit(92)
		}
		cmd := exec.Command(executable, "--dbus-launch")
		cmd.Env = replaceFixtureEnvironment(os.Environ(), map[string]string{
			ghAccountDBusFixtureModeEnv: "dbus-launch",
		})
		if err := cmd.Run(); err != nil {
			os.Exit(93)
		}
	}

	if reflect.DeepEqual(args, []string{"auth", "token", "--help"}) {
		fmt.Println("  -u, --user string   The account to output the token for")
		os.Exit(0)
	}

	token, err := os.ReadFile(os.Getenv(ghAccountDBusTokenFileEnv))
	if err != nil {
		os.Exit(94)
	}
	_, _ = os.Stdout.Write(token)
	os.Exit(0)
}

func exitGHAccountDBusLauncherFixture(args []string) {
	if !reflect.DeepEqual(args, []string{"--dbus-launch"}) {
		os.Exit(95)
	}
	marker, err := os.OpenFile(os.Getenv(ghAccountDBusMarkerFileEnv), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		os.Exit(96)
	}
	_, writeErr := marker.WriteString("attempt\n")
	closeErr := marker.Close()
	if writeErr != nil || closeErr != nil {
		os.Exit(97)
	}
	os.Exit(0)
}

func replaceFixtureEnvironment(env []string, replacements map[string]string) []string {
	result := make([]string, 0, len(env)+len(replacements))
	for _, entry := range env {
		key, _, ok := strings.Cut(entry, "=")
		if !ok {
			result = append(result, entry)
			continue
		}
		if _, replace := replacements[key]; !replace {
			result = append(result, entry)
		}
	}
	for key, value := range replacements {
		result = append(result, key+"="+value)
	}
	return result
}

// @covers AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.19
// @covers AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.20
// @covers AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.21
// @covers AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.22
func TestSystemGHAccountRunnerDoesNotAutolaunchDBus(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("D-Bus session-bus autolaunch is Linux-specific")
	}

	dir := t.TempDir()
	testBinary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	fixtureBinary := filepath.Join(dir, "gh")
	binary, err := os.ReadFile(testBinary)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixtureBinary, binary, 0o755); err != nil {
		t.Fatal(err)
	}

	tokenFile := filepath.Join(dir, "hosts-token")
	const wantToken = "selected-fixture-token"
	if err := os.WriteFile(tokenFile, []byte(wantToken), 0o600); err != nil {
		t.Fatal(err)
	}
	markerFile := filepath.Join(dir, "dbus-launch-attempts")

	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GH_CONFIG_DIR", filepath.Join(dir, "gh-config"))
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "autolaunch:")
	t.Setenv(ghAccountDBusFixtureModeEnv, "gh")
	t.Setenv(ghAccountDBusTokenFileEnv, tokenFile)
	t.Setenv(ghAccountDBusMarkerFileEnv, markerFile)

	control := exec.Command(fixtureBinary, "auth", "token", "--help")
	if _, err := control.CombinedOutput(); err != nil {
		t.Fatalf("autolaunch marker positive control failed: %v", err)
	}
	controlMarker, err := os.ReadFile(markerFile)
	if err != nil || strings.Count(string(controlMarker), "attempt\n") != 1 {
		t.Fatal("fixture did not record the positive-control autolaunch attempt")
	}
	if err := os.Remove(markerFile); err != nil {
		t.Fatalf("reset autolaunch marker: %v", err)
	}

	for range 3 {
		token, err := ResolveGHAccountToken(context.Background(), "github.com", "alice")
		if err != nil {
			t.Fatalf("ResolveGHAccountToken() returned an error: %v", err)
		}
		if token != wantToken {
			t.Fatal("ResolveGHAccountToken() did not return the selected file token")
		}
	}

	marker, err := os.ReadFile(markerFile)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read autolaunch marker: %v", err)
	}
	if len(marker) != 0 {
		t.Fatalf("fixture observed %d D-Bus autolaunch attempts", strings.Count(string(marker), "attempt\n"))
	}
}
