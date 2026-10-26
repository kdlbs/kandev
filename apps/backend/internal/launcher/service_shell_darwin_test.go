//go:build darwin

package launcher

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestServiceShellDarwinLoginEnvironment(t *testing.T) {
	accountHome := filepath.Join(t.TempDir(), `Kandev & '<$>' home`)
	shimDir := filepath.Join(accountHome, `tools & '<$>' shims`)
	if err := os.MkdirAll(shimDir, 0o700); err != nil {
		t.Fatal(err)
	}
	node := filepath.Join(shimDir, "node")
	writeServiceShellFixture(t, node, "#!/bin/sh\nprintf node-from-login\n", 0o755)
	interactiveMarker := filepath.Join(accountHome, "interactive-started")
	writeDarwinLoginFiles(t, accountHome, shimDir)
	probe := filepath.Join(accountHome, "kandev-probe")
	result := filepath.Join(accountHome, "probe-result")
	writeServiceShellFixture(t, probe, darwinShellProbeScript, 0o755)

	for _, shell := range []string{"/bin/bash", "/bin/zsh"} {
		t.Run(filepath.Base(shell), func(t *testing.T) {
			invocation, err := buildServiceShellInvocation(shell, accountHome, probe, darwinShellTestEnvironment(result, interactiveMarker))
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(invocation.Path, invocation.Args[1:]...)
			cmd.Env = invocation.Env
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("login shell: %v\n%s", err, output)
			}
			values := readDarwinShellProbe(t, result)
			assertDarwinShellValues(t, shell, accountHome, shimDir, values)
			assertDarwinShellToolDiscovery(t, node, values[9])
			if _, err := os.Stat(interactiveMarker); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("interactive startup marker exists: stat err=%v", err)
			}
			if err := os.Remove(result); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestServiceShellDarwinExecChild(t *testing.T) {
	if os.Getenv("KANDEV_TEST_SERVICE_SHELL_CHILD") != "1" {
		return
	}
	home := os.Getenv("KANDEV_TEST_SERVICE_SHELL_HOME")
	lookupServiceShellUser = func(uid string) (*user.User, error) {
		return &user.User{Uid: uid, HomeDir: home}, nil
	}
	os.Args = []string{os.Getenv("KANDEV_TEST_SERVICE_SHELL_EXECUTABLE")}
	args, err := parseServiceArgs([]string{"run", "--shell", os.Getenv("KANDEV_TEST_SERVICE_SHELL_PATH")})
	if err == nil {
		err = prepareServiceShellArgs(&args, goosDarwin)
	}
	if err == nil {
		err = runNativeServiceShell(args)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(77)
	}
	t.Fatal("process replacement returned without an error")
}

func TestServiceShellDarwinLaunchdLifecycle(t *testing.T) {
	if os.Getenv("KANDEV_TEST_LAUNCHD_SHELL") != "1" {
		t.Skip("set KANDEV_TEST_LAUNCHD_SHELL=1 in a disposable macOS login account")
	}
	fixture := newDarwinLaunchdShellFixture(t)
	fixture.start(fixture.executable)
	firstPID := fixture.assertRunning()
	fixture.signalAndRestart(firstPID)
	fixture.fullRestart()
	fixture.startupExitFailure()
	fixture.missingExecutableFailure()
	fixture.hangingStartupFailure()
}

type darwinLaunchdShellFixture struct {
	t          *testing.T
	domain     string
	target     string
	label      string
	plistPath  string
	outputPath string
	errorPath  string
	home       string
	shimDir    string
	executable string
	resultPath string
	stopPath   string
	testBinary string
}

func newDarwinLaunchdShellFixture(t *testing.T) *darwinLaunchdShellFixture {
	t.Helper()
	domain := "gui/" + strconv.Itoa(os.Getuid())
	if _, err := runDarwinLaunchctl(t, "print", domain); err != nil {
		t.Fatalf("launchd GUI domain %s is unavailable: %v", domain, err)
	}
	temp := t.TempDir()
	fixture := &darwinLaunchdShellFixture{
		t: t, domain: domain,
		label:      fmt.Sprintf("com.kdlbs.kandev.shell-test.%d.%d", os.Getpid(), time.Now().UnixNano()),
		plistPath:  filepath.Join(temp, "service shell test.plist"),
		outputPath: filepath.Join(temp, "service shell stdout.log"), errorPath: filepath.Join(temp, "service shell stderr.log"),
		home:       filepath.Join(temp, `disposable home & '<$>'`),
		resultPath: filepath.Join(temp, "selected environment.txt"), stopPath: filepath.Join(temp, "signal received.txt"),
		executable: filepath.Join(temp, "kandev process fixture"),
	}
	fixture.target = domain + "/" + fixture.label
	fixture.shimDir = filepath.Join(fixture.home, `tool shims & '<$>'`)
	if err := os.MkdirAll(fixture.shimDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeDarwinLoginFiles(t, fixture.home, fixture.shimDir)
	writeServiceShellFixture(t, fixture.executable, darwinLaunchdFixtureScript, 0o755)
	testBinary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	fixture.testBinary = testBinary
	if !filepath.IsAbs(fixture.testBinary) {
		t.Fatalf("test binary path %q is not absolute", fixture.testBinary)
	}
	t.Cleanup(func() { _, _ = runDarwinLaunchctl(t, "bootout", fixture.target) })
	return fixture
}

func (f *darwinLaunchdShellFixture) start(childExecutable string) {
	f.t.Helper()
	_, _ = runDarwinLaunchctl(f.t, "bootout", f.target)
	waitForDarwinLaunchdStopped(f.t, f.target, 10*time.Second)
	removeDarwinLaunchdOutput(f.t, f.resultPath)
	removeDarwinLaunchdOutput(f.t, f.stopPath)
	env := darwinLaunchdShellEnvironment(f.home, f.resultPath, f.stopPath, childExecutable)
	arguments := []string{f.testBinary, "-test.run=^TestServiceShellDarwinExecChild$"}
	plist := darwinShellLaunchdPlist(f.label, arguments, env, f.outputPath, f.errorPath)
	writeServiceShellFixture(f.t, f.plistPath, plist, 0o600)
	if output, err := runDarwinLaunchctl(f.t, "bootstrap", f.domain, f.plistPath); err != nil {
		f.t.Fatalf("bootstrap test job: %v\n%s", err, output)
	}
}

func (f *darwinLaunchdShellFixture) assertRunning() int {
	f.t.Helper()
	waitForDarwinFile(f.t, f.resultPath, 15*time.Second)
	values := readDarwinShellProbe(f.t, f.resultPath)
	if len(values) != 13 {
		f.t.Fatalf("launchd process recorded %d values: %#v", len(values), values)
	}
	childPID, err := strconv.Atoi(values[0])
	if err != nil {
		f.t.Fatalf("fixture PID %q: %v", values[0], err)
	}
	launchdPID := waitForDarwinLaunchdPID(f.t, f.target, 10*time.Second)
	if childPID != launchdPID {
		f.t.Fatalf("process identity changed across shell exec: fixture PID=%d launchd PID=%d", childPID, launchdPID)
	}
	assertDarwinShellValues(f.t, "/bin/zsh", f.home, f.shimDir, values[1:])
	return childPID
}

func (f *darwinLaunchdShellFixture) signalAndRestart(previousPID int) {
	f.t.Helper()
	if output, err := runDarwinLaunchctl(f.t, "kill", "SIGTERM", f.target); err != nil {
		f.t.Fatalf("signal service: %v\n%s", err, output)
	}
	waitForDarwinFile(f.t, f.stopPath, 10*time.Second)
	waitForDarwinLaunchdStopped(f.t, f.target, 10*time.Second)
	removeDarwinLaunchdOutput(f.t, f.resultPath)
	removeDarwinLaunchdOutput(f.t, f.stopPath)
	if output, err := runDarwinLaunchctl(f.t, "kickstart", f.target); err != nil {
		f.t.Fatalf("restart service: %v\n%s", err, output)
	}
	if restartedPID := f.assertRunning(); restartedPID == previousPID {
		f.t.Fatalf("kickstart reused stopped process PID %d", previousPID)
	}
}

func (f *darwinLaunchdShellFixture) fullRestart() {
	f.t.Helper()
	_, _ = runDarwinLaunchctl(f.t, "bootout", f.target)
	waitForDarwinLaunchdStopped(f.t, f.target, 10*time.Second)
	f.start(f.executable)
	f.assertRunning()
}

func (f *darwinLaunchdShellFixture) startupExitFailure() {
	f.t.Helper()
	writeStartupFile(f.t, filepath.Join(f.home, ".zshenv"), "exit 23\n")
	f.start(f.executable)
	waitForDarwinLaunchdStopped(f.t, f.target, 10*time.Second)
	assertNoDarwinFile(f.t, f.resultPath)
}

func (f *darwinLaunchdShellFixture) missingExecutableFailure() {
	f.t.Helper()
	writeStartupFile(f.t, filepath.Join(f.home, ".zshenv"), "export KANDEV_ZSHENV_MARKER='loaded'\n")
	f.start(filepath.Join(filepath.Dir(f.executable), "missing-kandev-executable"))
	waitForDarwinLaunchdStopped(f.t, f.target, 10*time.Second)
	assertNoDarwinFile(f.t, f.resultPath)
}

func (f *darwinLaunchdShellFixture) hangingStartupFailure() {
	f.t.Helper()
	writeStartupFile(f.t, filepath.Join(f.home, ".zshenv"), "while :; do :; done\n")
	f.start(f.executable)
	waitForDarwinLaunchdPID(f.t, f.target, 10*time.Second)
	time.Sleep(time.Second)
	assertNoDarwinFile(f.t, f.resultPath)
	if output, err := runDarwinLaunchctl(f.t, "kill", "SIGTERM", f.target); err != nil {
		f.t.Fatalf("stop hanging startup: %v\n%s", err, output)
	}
	waitForDarwinLaunchdStopped(f.t, f.target, 10*time.Second)
}

const darwinShellProbeScript = `#!/bin/sh
printf '%s\n' "$HOME" "$SHELL" "$KANDEV_SERVICE_MANAGER" "$KANDEV_HOME_DIR" "$KANDEV_SERVER_PORT" "$KANDEV_LOGIN_MARKER" "$KANDEV_ZSHENV_MARKER" "$KANDEV_ZPROFILE_MARKER" "$KANDEV_ZLOGIN_MARKER" "$PATH" "$#" "$1" > "$KANDEV_TEST_SERVICE_SHELL_RESULT"
`

const darwinLaunchdFixtureScript = `#!/bin/sh
result_tmp="$KANDEV_TEST_SERVICE_SHELL_RESULT.tmp.$$"
printf '%s\n' "$$" "$HOME" "$SHELL" "$KANDEV_SERVICE_MANAGER" "$KANDEV_HOME_DIR" "$KANDEV_SERVER_PORT" "$KANDEV_LOGIN_MARKER" "$KANDEV_ZSHENV_MARKER" "$KANDEV_ZPROFILE_MARKER" "$KANDEV_ZLOGIN_MARKER" "$PATH" "$#" "$1" > "$result_tmp"
/bin/mv "$result_tmp" "$KANDEV_TEST_SERVICE_SHELL_RESULT"
trap 'printf stopped > "$KANDEV_TEST_SERVICE_SHELL_STOP"; exit 0' TERM INT
while :; do /bin/sleep 1; done
`

func writeDarwinLoginFiles(t *testing.T, home, shimDir string) {
	t.Helper()
	bashProfile := "export PATH=" + shellSingleQuote(shimDir) + ":$PATH\n" +
		"export KANDEV_LOGIN_MARKER='bash-profile'\n" +
		"export KANDEV_SERVICE_MANAGER='login-override'\n"
	writeStartupFile(t, filepath.Join(home, ".bash_profile"), bashProfile)
	writeStartupFile(t, filepath.Join(home, ".bashrc"), "touch \"$KANDEV_TEST_SERVICE_SHELL_INTERACTIVE\"\n")
	writeStartupFile(t, filepath.Join(home, ".zshenv"), "export KANDEV_ZSHENV_MARKER='zshenv'\n")
	writeStartupFile(t, filepath.Join(home, ".zprofile"), "export PATH="+shellSingleQuote(shimDir)+":$PATH\nexport KANDEV_ZPROFILE_MARKER='zprofile'\n")
	writeStartupFile(t, filepath.Join(home, ".zlogin"), "export KANDEV_LOGIN_MARKER='zlogin'\nexport KANDEV_ZLOGIN_MARKER='zlogin'\nexport KANDEV_SERVICE_MANAGER='login-override'\n")
	writeStartupFile(t, filepath.Join(home, ".zshrc"), "touch \"$KANDEV_TEST_SERVICE_SHELL_INTERACTIVE\"\n")
}

func writeStartupFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func darwinShellTestEnvironment(result, interactiveMarker string) []string {
	return []string{
		"PATH=/usr/bin:/bin", "HOME=/wrong/home", "SHELL=/wrong/shell",
		"KANDEV_SERVICE_MANAGER=launchd", "KANDEV_HOME_DIR=/service home/&data", "KANDEV_SERVER_PORT=38429",
		"KANDEV_TEST_SERVICE_SHELL_RESULT=" + result,
		"KANDEV_TEST_SERVICE_SHELL_INTERACTIVE=" + interactiveMarker,
	}
}

func darwinLaunchdShellEnvironment(home, result, stop, executable string) map[string]string {
	return map[string]string{
		"PATH": "/usr/bin:/bin", "HOME": "/wrong/home", "SHELL": "/wrong/shell",
		"KANDEV_RUNNING_AS_SERVICE": "true", "KANDEV_SERVICE_MANAGER": "launchd", "KANDEV_SERVICE_MODE": "user",
		"KANDEV_HOME_DIR": "/service home/&data", "KANDEV_SERVER_PORT": "38429",
		"KANDEV_TEST_SERVICE_SHELL_CHILD": "1", "KANDEV_TEST_SERVICE_SHELL_HOME": home,
		"KANDEV_TEST_SERVICE_SHELL_EXECUTABLE": executable, "KANDEV_TEST_SERVICE_SHELL_PATH": "/bin/zsh",
		"KANDEV_TEST_SERVICE_SHELL_RESULT": result, "KANDEV_TEST_SERVICE_SHELL_STOP": stop,
		"KANDEV_TEST_SERVICE_SHELL_INTERACTIVE": filepath.Join(home, "interactive-started"),
	}
}

func darwinShellLaunchdPlist(label string, arguments []string, env map[string]string, stdout, stderr string) string {
	var program, environment strings.Builder
	for _, argument := range arguments {
		program.WriteString("    <string>" + escapeXML(argument) + "</string>\n")
	}
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		environment.WriteString("    <key>" + escapeXML(key) + "</key><string>" + escapeXML(env[key]) + "</string>\n")
	}
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>Label</key><string>` + escapeXML(label) + `</string>
  <key>ProgramArguments</key><array>
` + program.String() + `  </array>
  <key>EnvironmentVariables</key><dict>
` + environment.String() + `  </dict>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><false/>
  <key>StandardOutPath</key><string>` + escapeXML(stdout) + `</string>
  <key>StandardErrorPath</key><string>` + escapeXML(stderr) + `</string>
</dict></plist>
`
}

func runDarwinLaunchctl(t *testing.T, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "/bin/launchctl", args...).CombinedOutput()
	return string(output), err
}

func waitForDarwinFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}

func readDarwinShellProbe(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

func assertDarwinShellValues(t *testing.T, shell, home, shimDir string, values []string) {
	t.Helper()
	if len(values) != 12 || values[0] != home || values[1] != shell || values[2] != "launchd" ||
		values[3] != "/service home/&data" || values[4] != "38429" || !strings.Contains(values[9], shimDir) ||
		values[10] != "1" || values[11] != "--headless" {
		t.Fatalf("unexpected login-shell result: %#v", values)
	}
	if shell == "/bin/bash" && values[5] != "bash-profile" {
		t.Fatalf("Bash login marker = %q", values[5])
	}
	if shell == "/bin/zsh" && (values[5] != "zlogin" || values[6] != "zshenv" || values[7] != "zprofile" || values[8] != "zlogin") {
		t.Fatalf("Zsh login markers = %#v", values[5:9])
	}
}

func assertDarwinShellToolDiscovery(t *testing.T, node, path string) {
	t.Helper()
	t.Setenv("PATH", path)
	resolved, err := exec.LookPath("node")
	if err != nil || resolved != node {
		t.Fatalf("exec.LookPath(node) = (%q, %v), want %q", resolved, err, node)
	}
	output, err := exec.Command("node").Output()
	if err != nil || string(output) != "node-from-login" {
		t.Fatalf("execute login-shell node = %q, %v", output, err)
	}
}

func waitForDarwinLaunchdPID(t *testing.T, target string, timeout time.Duration) int {
	t.Helper()
	deadline := time.Now().Add(timeout)
	pidPattern := regexp.MustCompile(`(?m)^\s*pid = ([0-9]+)$`)
	for time.Now().Before(deadline) {
		output, err := runDarwinLaunchctl(t, "print", target)
		if err == nil {
			if match := pidPattern.FindStringSubmatch(output); len(match) == 2 {
				pid, parseErr := strconv.Atoi(match[1])
				if parseErr == nil {
					return pid
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for launchd process %s", target)
	return 0
}

func waitForDarwinLaunchdStopped(t *testing.T, target string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	pidPattern := regexp.MustCompile(`(?m)^\s*pid = [0-9]+$`)
	for time.Now().Before(deadline) {
		output, err := runDarwinLaunchctl(t, "print", target)
		if err != nil || !pidPattern.MatchString(output) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("launchd process %s did not stop", target)
}

func assertNoDarwinFile(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected startup result file %s: stat err=%v", path, err)
	}
}

func removeDarwinLaunchdOutput(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("remove stale launchd output %s: %v", path, err)
	}
}
