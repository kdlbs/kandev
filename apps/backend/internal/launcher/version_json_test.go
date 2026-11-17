package launcher

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// @covers AC-CLI-VERSION-JSON-001.1
func TestRunVersionJSON(t *testing.T) {
	output := captureLauncherStdout(t, func() {
		if code := Run([]string{"--version", "--json"}, BuildInfo{Version: "1.2.3", Commit: "abc", BuildTime: "now"}); code != 0 {
			t.Fatalf("Run(--version --json) = %d, want 0", code)
		}
	})
	if output != "{\"version\":\"1.2.3\",\"commit\":\"abc\",\"build_time\":\"now\"}\n" {
		t.Fatalf("version JSON = %q", output)
	}
}

// @covers AC-CLI-VERSION-JSON-001.1 AC-CLI-VERSION-JSON-001.2
func TestVersionJSONArgumentsAndStamps(t *testing.T) {
	build := BuildInfo{Version: "v1.2.3", Commit: "a\"b\\c\n", BuildTime: "time\n\"x"}
	want := "{\"version\":\"v1.2.3\",\"commit\":\"a\\\"b\\\\c\\n\",\"build_time\":\"time\\n\\\"x\"}\n"
	for _, args := range [][]string{{"--version", "--json"}, {"--json", "--version"}, {"-V", "--json"}, {"--json", "-V"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var code int
			output := captureLauncherStdout(t, func() { code = Run(args, build) })
			if code != 0 || output != want {
				t.Fatalf("Run(%v) = %d, %q; want 0, %q", args, code, output, want)
			}
			var decoded map[string]string
			if err := json.Unmarshal([]byte(output), &decoded); err != nil {
				t.Fatal(err)
			}
			if len(decoded) != 3 || decoded["version"] != build.Version || decoded["commit"] != build.Commit || decoded["build_time"] != build.BuildTime {
				t.Fatalf("decoded output = %#v", decoded)
			}
		})
	}
	var code int
	output := captureLauncherStdout(t, func() { code = Run([]string{"--version", "--json"}, BuildInfo{}) })
	if code != 0 || output != "{\"version\":\"dev\",\"commit\":\"unknown\",\"build_time\":\"unknown\"}\n" {
		t.Fatalf("empty build = %d, %q", code, output)
	}
}

// @covers AC-CLI-VERSION-JSON-001.3 AC-CLI-VERSION-JSON-001.4
func TestVersionJSONAvoidsStartupAndPreservesPlainVersion(t *testing.T) {
	clearLauncherConfigurationEnvironment(t)
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("KANDEV_HOME_DIR", filepath.Join(dir, "home"))
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("database: [\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var code int
	controlErr := captureStderr(t, func() { code = Run([]string{"run"}, BuildInfo{}) })
	if code != 1 || !strings.Contains(controlErr, "load startup configuration") {
		t.Fatalf("startup control = %d, %q", code, controlErr)
	}
	for _, args := range [][]string{{"--version", "--json"}, {"--json"}, {"--version"}, {"-V"}} {
		var stdout string
		stderr := captureStderr(t, func() {
			stdout = captureLauncherStdout(t, func() { code = Run(args, BuildInfo{Version: "v1.2.3"}) })
		})
		if len(args) == 1 && args[0] == "--json" {
			if code != 2 || stdout != "" || !strings.Contains(stderr, "--json requires --version") {
				t.Fatalf("Run(%v) = %d, %q, %q", args, code, stdout, stderr)
			}
		} else if code != 0 || stderr != "" || (len(args) == 1 && stdout != "v1.2.3\n") || (len(args) == 2 && !strings.HasPrefix(stdout, "{\"version\":")) {
			t.Fatalf("Run(%v) = %d, %q, %q", args, code, stdout, stderr)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "home")); !os.IsNotExist(err) {
		t.Fatalf("home directory unexpectedly exists or stat failed: %v", err)
	}
}

// @covers AC-CLI-VERSION-JSON-001.5
func TestVersionJSONWriteFailures(t *testing.T) {
	for _, writer := range []struct {
		name string
		out  versionTestWriter
	}{
		{name: "error", out: versionTestWriter{err: errors.New("unavailable")}},
		{name: "short", out: versionTestWriter{short: true}},
	} {
		t.Run(writer.name, func(t *testing.T) {
			var stderr bytes.Buffer
			if code := printVersionJSON(writer.out, &stderr, BuildInfo{}); code != 1 || !strings.Contains(stderr.String(), "failed to write version output") {
				t.Fatalf("write failure = %d, %q", code, stderr.String())
			}
		})
	}
}

type versionTestWriter struct {
	err   error
	short bool
}

func (w versionTestWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	if w.short {
		return len(p) - 1, nil
	}
	return len(p), nil
}
