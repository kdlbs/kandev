package process

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.uber.org/goleak"
)

// kandevTestFixtureEnv activates fixture-binary mode in this test binary.
// Tests use fixtureExec / fixtureShellExec (below) to spawn known commands
// (sleep / echo / cat) without depending on Unix-only shell tooling — the
// test binary re-invokes itself with this env var set, and TestMain dispatches
// to runFixture before the normal test runner starts. This replaces the
// previous fixtures that hardcoded `sleep`, `cat`, `echo`, `printf` which
// don't exist as standalone executables on Windows.
const kandevTestFixtureEnv = "KANDEV_TEST_FIXTURE"

// TestMain branches into fixture-binary mode when the activation env var
// is set; otherwise it runs the test suite normally — wrapped in goleak so
// the per-process subprocess managers (workspace tracker, PTY pumps, poll
// loops) surface goroutine leaks here.
func TestMain(m *testing.M) {
	if spec := os.Getenv(kandevTestFixtureEnv); spec != "" {
		runFixture(spec)
		return
	}
	goleak.VerifyTestMain(m)
}

// runFixture executes a whitespace-separated command spec and exits.
// Supported commands:
//
//	sleep <secs>                   — sleep for <secs> seconds
//	echo <args...>                 — print args joined by spaces, plus newline
//	cat                            — copy stdin to stdout until EOF
//	write-both <stdout> <stderr>   — write exact strings to both output streams
//	echo-then-sleep <msg> <secs>   — print msg, then sleep <secs>
//	delay-then-child <pidfile> <delay-ms> <secs>
//	                               — wait, spawn a sleeping child, write its PID, then sleep
//
// New commands can be added here as tests need them; the goal is to keep the
// surface tiny so the helper stays inspectable.
func runFixture(spec string) {
	parts := strings.Fields(spec)
	if len(parts) == 0 {
		fmt.Fprintln(os.Stderr, "fixture: empty spec")
		os.Exit(2)
	}
	switch parts[0] {
	case "sleep":
		if len(parts) != 2 {
			fmt.Fprintln(os.Stderr, "fixture: sleep takes 1 arg")
			os.Exit(2)
		}
		secs, err := strconv.Atoi(parts[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "fixture: sleep: bad seconds %q\n", parts[1])
			os.Exit(2)
		}
		time.Sleep(time.Duration(secs) * time.Second)
	case "echo":
		fmt.Println(strings.Join(parts[1:], " "))
	case "cat":
		_, _ = io.Copy(os.Stdout, os.Stdin)
	case "write-both":
		if len(parts) != 3 {
			fmt.Fprintln(os.Stderr, "fixture: write-both takes 2 args")
			os.Exit(2)
		}
		_, _ = fmt.Fprint(os.Stdout, parts[1])
		_, _ = fmt.Fprint(os.Stderr, parts[2])
	case "echo-then-sleep":
		if len(parts) != 3 {
			fmt.Fprintln(os.Stderr, "fixture: echo-then-sleep takes 2 args")
			os.Exit(2)
		}
		fmt.Println(parts[1])
		// Force the kernel to flush this write to the parent's pipe before
		// we sleep. On Windows in particular, an anonymous pipe's first
		// small write can sit in the per-process buffer until the writer
		// makes a second write or exits — that would defeat the point of a
		// "print, then wait" fixture used by output-capture tests.
		_ = os.Stdout.Sync()
		secs, err := strconv.Atoi(parts[2])
		if err != nil {
			fmt.Fprintf(os.Stderr, "fixture: echo-then-sleep: bad seconds %q\n", parts[2])
			os.Exit(2)
		}
		time.Sleep(time.Duration(secs) * time.Second)
	case "sleep-with-child":
		// Forks a child copy of this fixture binary (also sleeping <secs>),
		// writes the child PID to <pidfile>, then sleeps itself. Used by the
		// process group kill regression test: when the parent's process
		// group is reaped, the child must die too. Both processes ignore
		// stdin so close-stdin-then-wait can't reach them on its own.
		if len(parts) != 3 {
			fmt.Fprintln(os.Stderr, "fixture: sleep-with-child takes 2 args")
			os.Exit(2)
		}
		pidFile := parts[1]
		secs, err := strconv.Atoi(parts[2])
		if err != nil {
			fmt.Fprintf(os.Stderr, "fixture: sleep-with-child: bad seconds %q\n", parts[2])
			os.Exit(2)
		}
		childCmd := exec.Command(os.Args[0])
		childCmd.Env = append(os.Environ(), kandevTestFixtureEnv+"=sleep "+strconv.Itoa(secs))
		if err := childCmd.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "fixture: sleep-with-child: spawn child: %v\n", err)
			os.Exit(2)
		}
		if err := os.WriteFile(pidFile, []byte(strconv.Itoa(childCmd.Process.Pid)), 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "fixture: sleep-with-child: write pidfile: %v\n", err)
			os.Exit(2)
		}
		time.Sleep(time.Duration(secs) * time.Second)
	case "delay-then-child":
		// Waits briefly before spawning the child so platform lifecycle hooks
		// installed immediately after parent Start() can be applied before the
		// child inherits them. Used by Windows Job Object tests.
		if len(parts) != 4 {
			fmt.Fprintln(os.Stderr, "fixture: delay-then-child takes 3 args")
			os.Exit(2)
		}
		pidFile := parts[1]
		delayMS, err := strconv.Atoi(parts[2])
		if err != nil {
			fmt.Fprintf(os.Stderr, "fixture: delay-then-child: bad delay %q\n", parts[2])
			os.Exit(2)
		}
		secs, err := strconv.Atoi(parts[3])
		if err != nil {
			fmt.Fprintf(os.Stderr, "fixture: delay-then-child: bad seconds %q\n", parts[3])
			os.Exit(2)
		}
		time.Sleep(time.Duration(delayMS) * time.Millisecond)
		childCmd := exec.Command(os.Args[0])
		childCmd.Env = append(os.Environ(), kandevTestFixtureEnv+"=sleep "+strconv.Itoa(secs))
		if err := childCmd.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "fixture: delay-then-child: spawn child: %v\n", err)
			os.Exit(2)
		}
		if err := os.WriteFile(pidFile, []byte(strconv.Itoa(childCmd.Process.Pid)), 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "fixture: delay-then-child: write pidfile: %v\n", err)
			os.Exit(2)
		}
		time.Sleep(time.Duration(secs) * time.Second)
	case "exit-with-child":
		// Forks a sleeping child, writes its PID, then exits immediately.
		// This models wrappers such as npx/node that can exit while a native
		// descendant remains alive in the inherited process group.
		if len(parts) != 3 {
			fmt.Fprintln(os.Stderr, "fixture: exit-with-child takes 2 args")
			os.Exit(2)
		}
		pidFile := parts[1]
		secs, err := strconv.Atoi(parts[2])
		if err != nil {
			fmt.Fprintf(os.Stderr, "fixture: exit-with-child: bad seconds %q\n", parts[2])
			os.Exit(2)
		}
		childCmd := exec.Command(os.Args[0])
		childCmd.Env = append(os.Environ(), kandevTestFixtureEnv+"=sleep "+strconv.Itoa(secs))
		if err := childCmd.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "fixture: exit-with-child: spawn child: %v\n", err)
			os.Exit(2)
		}
		if err := os.WriteFile(pidFile, []byte(strconv.Itoa(childCmd.Process.Pid)), 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "fixture: exit-with-child: write pidfile: %v\n", err)
			os.Exit(2)
		}
	default:
		fmt.Fprintf(os.Stderr, "fixture: unknown command %q\n", parts[0])
		os.Exit(2)
	}
	os.Exit(0)
}

// fixtureExec returns the (Command argv, Env) pair tests pass to runners that
// take a Command []string and an Env map (e.g. InteractiveStartRequest). The
// runner spawns this test binary with the env var set, TestMain dispatches
// to runFixture, and the spec is executed in-process.
func fixtureExec(spec string) (command []string, env map[string]string) {
	return []string{os.Args[0]}, map[string]string{kandevTestFixtureEnv: spec}
}

// fixtureShellExec returns the (Command string, Env) pair tests pass to
// runners that take a single shell-string Command (e.g. StartProcessRequest,
// which runs via `sh -lc` on Unix and `cmd /c` on Windows). The spec travels
// in the env var so we don't have to worry about per-shell argument quoting.
//
// Quoting is platform-specific because cmd.exe's `/c` parser strips at most
// one outer pair of quotes — pre-quoting the path on Windows turns into
// `cmd /c "\"C:\path\""` after Go's exec escaping, and cmd then fails to
// resolve the executable. We leave Windows paths bare (Go's syscall.EscapeArg
// adds quotes only if needed) and double-quote on Unix where sh splits on
// whitespace.
func fixtureShellExec(spec string) (command string, env map[string]string) {
	if runtime.GOOS == "windows" {
		return os.Args[0], map[string]string{kandevTestFixtureEnv: spec}
	}
	return `"` + os.Args[0] + `"`, map[string]string{kandevTestFixtureEnv: spec}
}

// fixtureCmd builds an *exec.Cmd ready to run a fixture spec via this test
// binary. Used by tests that bypass the runner abstractions and call
// exec.Command directly.
func fixtureCmd(spec string) *exec.Cmd {
	cmd := exec.Command(os.Args[0])
	cmd.Env = fixtureEnvSlice(spec)
	return cmd
}

// fixtureEnvSlice returns os.Environ() plus the fixture activation variable,
// formatted as a []string suitable for exec.Cmd.Env / InstanceConfig.AgentEnv.
func fixtureEnvSlice(spec string) []string {
	return append(os.Environ(), kandevTestFixtureEnv+"="+spec)
}

// fixtureArgs returns the argv to spawn this test binary in fixture mode.
// Used together with fixtureEnvSlice when populating fields like
// InstanceConfig.AgentArgs.
func fixtureArgs() []string {
	return []string{os.Args[0]}
}
