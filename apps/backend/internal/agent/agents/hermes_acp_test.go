package agents

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/pkg/agent"
)

const (
	hermesACPCheckHelperEnv = "KANDEV_TEST_HERMES_ACP_CHECK"
	// hermesCheckCounterEnv points at the file the helper appends one marker
	// byte to per invocation, so a test can assert how many times the check
	// actually ran.
	hermesCheckCounterEnv = "KANDEV_TEST_HERMES_COUNTER"
	// hermesCheckSlowEnv is how long a slow helper mode stalls before exiting 0.
	hermesCheckSlowEnv = "KANDEV_TEST_HERMES_SLOW_MS"
)

func TestMain(m *testing.M) {
	switch os.Getenv(hermesACPCheckHelperEnv) {
	case "available":
		if slices.Equal(os.Args[1:], []string{"acp", "--check"}) {
			os.Exit(0)
		}
		os.Exit(1)
	case "unavailable":
		os.Exit(1)
	case "counted-unavailable":
		recordHermesCheckInvocation()
		os.Exit(1)
	case "timeout-then-available":
		// Stalls only on the first invocation, so a caller that measures a
		// second time sees the check succeed.
		if recordHermesCheckInvocation() == 1 {
			time.Sleep(hermesCheckSlowDuration())
		}
		os.Exit(0)
	case "always-slow":
		recordHermesCheckInvocation()
		time.Sleep(hermesCheckSlowDuration())
		os.Exit(0)
	default:
		os.Exit(m.Run())
	}
}

// recordHermesCheckInvocation appends a marker byte and returns this
// invocation's ordinal (1 for the first one). The write happens before any
// stall so a helper killed by the caller's bound is still counted.
func recordHermesCheckInvocation() int {
	path := os.Getenv(hermesCheckCounterEnv)
	ordinal := 1
	// Count the existing markers rather than trusting the append-mode file
	// offset, which is not positioned at the end on every platform.
	if existing, err := os.ReadFile(path); err == nil {
		ordinal = len(existing) + 1
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		os.Exit(3)
	}
	if _, err := f.Write([]byte{'x'}); err != nil {
		_ = f.Close()
		os.Exit(3)
	}
	// Closed before any os.Exit so the marker is on disk for the next run.
	if err := f.Close(); err != nil {
		os.Exit(3)
	}
	return ordinal
}

func hermesCheckSlowDuration() time.Duration {
	ms, err := strconv.Atoi(os.Getenv(hermesCheckSlowEnv))
	if err != nil || ms <= 0 {
		ms = 2000
	}
	return time.Duration(ms) * time.Millisecond
}

// hermesCheckInvocationCount reads how many times the helper ran.
func hermesCheckInvocationCount(t *testing.T) int {
	t.Helper()
	info, err := os.Stat(os.Getenv(hermesCheckCounterEnv))
	if err != nil {
		t.Fatalf("stat helper counter: %v", err)
	}
	return int(info.Size())
}

func TestHermesACP_IDAndDisplay(t *testing.T) {
	a := NewHermesACP()
	if got := a.ID(); got != "hermes-acp" {
		t.Errorf("ID() = %q, want hermes-acp", got)
	}
	if got := a.DisplayName(); got != "Hermes" {
		t.Errorf("DisplayName() = %q, want Hermes", got)
	}
	if !a.Enabled() {
		t.Error("Enabled() = false, want true")
	}
	if got := a.DisplayOrder(); got != 21 {
		t.Errorf("DisplayOrder() = %d, want 21", got)
	}
}

func TestHermesACP_AllCommandSurfaces(t *testing.T) {
	a := NewHermesACP()
	want := []string{"hermes", "acp"}

	assertArgvEqual(t, "BuildCommand", a.BuildCommand(CommandOptions{}).Args(), want)

	rt := a.Runtime()
	if rt == nil {
		t.Fatal("Runtime() returned nil")
	}
	if rt.Protocol != agent.ProtocolACP {
		t.Errorf("Runtime.Protocol = %q, want ACP", rt.Protocol)
	}
	assertArgvEqual(t, "Runtime.Cmd", rt.Cmd.Args(), want)

	ic := a.InferenceConfig()
	if ic == nil || !ic.Supported {
		t.Fatalf("InferenceConfig() = %+v, want Supported=true", ic)
	}
	assertArgvEqual(t, "InferenceConfig.Command", ic.Command.Args(), want)

	pa, ok := any(a).(PassthroughAgent)
	if !ok {
		t.Fatal("HermesACP must implement PassthroughAgent")
	}
	assertArgvEqual(t, "PassthroughCmd", pa.PassthroughConfig().PassthroughCmd.Args(), []string{"hermes", "chat"})
}

func TestHermesACP_PassthroughPreservesModelAndSessions(t *testing.T) {
	a := NewHermesACP()

	for _, tc := range []struct {
		name string
		opts PassthroughOptions
		want []string
	}{
		{
			name: "model",
			opts: PassthroughOptions{Model: "nous/hermes-4"},
			want: []string{"hermes", "chat", "--model", "nous/hermes-4"},
		},
		{
			name: "last session",
			opts: PassthroughOptions{Resume: true},
			want: []string{"hermes", "chat", "--continue"},
		},
		{
			name: "specific session",
			opts: PassthroughOptions{SessionID: "session-123"},
			want: []string{"hermes", "chat", "--resume", "session-123"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertArgvEqual(t, "BuildPassthroughCommand", a.BuildPassthroughCommand(tc.opts).Args(), tc.want)
		})
	}
}

// TestHermesACP_SkipsGlobalMCPStartup pins the host-owned MCP marker. Kandev
// passes session MCP servers through ACP session/new; without the marker
// Hermes would ALSO start every globally configured server from
// ~/.hermes/config.yaml before initialize, duplicating work and slowing the
// handshake. The marker must stay exactly "1" — any other value keeps the
// default behavior.
func TestHermesACP_SkipsGlobalMCPStartup(t *testing.T) {
	rt := NewHermesACP().Runtime()
	if got := rt.Env["HERMES_ACP_SKIP_CONFIGURED_MCP"]; got != "1" {
		t.Errorf("HERMES_ACP_SKIP_CONFIGURED_MCP = %q, want exactly \"1\"", got)
	}
}

func TestHermesACP_InstallScript(t *testing.T) {
	got := NewHermesACP().InstallScript()
	for _, needle := range []string{
		"curl -fsSL https://hermes-agent.nousresearch.com/install.sh",
		"hermes",
	} {
		if !strings.Contains(got, needle) {
			t.Errorf("InstallScript missing %q: %q", needle, got)
		}
	}
	if strings.Contains(got, "Install Hermes") {
		t.Errorf("InstallScript must be executable shell, got prose: %q", got)
	}
	if strings.HasPrefix(got, "npm install -g ") {
		t.Errorf("InstallScript should use the native Hermes installer, got npm script: %q", got)
	}
}

func TestHermesACP_DetectionRequiresGlobalBinary(t *testing.T) {
	if _, err := exec.LookPath("hermes"); err == nil {
		t.Skip("detection binary \"hermes\" is on PATH; can't verify availability requirement")
	}
	result, err := NewHermesACP().IsInstalled(context.Background())
	if err != nil {
		t.Fatalf("IsInstalled error: %v", err)
	}
	if result.Available {
		t.Error("Available=true without hermes on PATH; discovery must not imply install")
	}
}

func TestHermesACP_DetectionRequiresACPCheck(t *testing.T) {
	installHermesACPCheckHelper(t)

	t.Run("available when acp check succeeds", func(t *testing.T) {
		t.Setenv(hermesACPCheckHelperEnv, "available")

		result, err := NewHermesACP().IsInstalled(context.Background())
		if err != nil {
			t.Fatalf("IsInstalled error: %v", err)
		}
		if !result.Available {
			t.Fatal("Available=false when hermes acp --check succeeds")
		}
	})

	t.Run("unavailable when acp check fails", func(t *testing.T) {
		t.Setenv(hermesACPCheckHelperEnv, "unavailable")

		result, err := NewHermesACP().IsInstalled(context.Background())
		if err != nil {
			t.Fatalf("IsInstalled error: %v", err)
		}
		if result.Available {
			t.Fatal("Available=true when hermes acp --check fails")
		}
	})

	t.Run("returns cancellation from caller context", func(t *testing.T) {
		t.Setenv(hermesACPCheckHelperEnv, "available")
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		result, err := NewHermesACP().IsInstalled(ctx)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("IsInstalled error = %v, want context.Canceled", err)
		}
		if result.Available {
			t.Fatal("Available=true with a cancelled context")
		}
	})
}

func installHermesACPCheckHelper(t *testing.T) {
	t.Helper()

	filename := hermesBin
	if runtime.GOOS == "windows" {
		filename += ".exe"
	}

	path := filepath.Join(t.TempDir(), filename)
	copyHermesCheckHelper(t, os.Args[0], path)
	t.Setenv("PATH", filepath.Dir(path)+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// copyHermesCheckHelper copies the test binary to stand in for the agent
// binary. A hard link would be cheaper, but on Windows the running image stays
// locked for the lifetime of the test, so the framework's temp-dir cleanup
// fails to unlink it.
func copyHermesCheckHelper(t *testing.T, src, dst string) {
	t.Helper()
	in, err := os.Open(src)
	if err != nil {
		t.Fatalf("open test binary: %v", err)
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o700)
	if err != nil {
		_ = in.Close()
		t.Fatalf("create helper copy: %v", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = in.Close()
		_ = out.Close()
		t.Fatalf("copy test binary: %v", err)
	}
	if err := out.Close(); err != nil {
		_ = in.Close()
		t.Fatalf("close helper copy: %v", err)
	}
	if err := in.Close(); err != nil {
		t.Fatalf("close test binary: %v", err)
	}
}

// useHermesCheckCounter gives a staged-measurement case a private counter file
// and a stall long enough to blow the sub-second bounds used below.
func useHermesCheckCounter(t *testing.T) {
	t.Helper()
	t.Setenv(hermesCheckCounterEnv, filepath.Join(t.TempDir(), "hermes-check-calls"))
	t.Setenv(hermesCheckSlowEnv, "2000")
}

// hermesStagedCheckBounds both sit clear of the helper's 2s stall, so a bound
// that elapses is a real timeout rather than a slow success. The first bound
// also has to clear the cost of spawning the helper at all, which reaches a
// few hundred milliseconds on a loaded Windows host.
const (
	hermesStagedFirst  = 800 * time.Millisecond
	hermesStagedSecond = 1500 * time.Millisecond
)

// stubHermesCommandCheck replaces hermesCommandCheckRunner with scripted
// answers, counting attempts synchronously in the caller, and restores the
// real runner when the subtest ends. Counting via the helper subprocess is
// load-sensitive — a bound can elapse before the child records its start, so
// the counter file can lag behind (or miss) the attempt the parent already
// timed out — which flakes timing-critical retry assertions under a full
// package run. Real-subprocess behavior stays covered by
// TestRunCommandCheck_SeparatesTimeoutFromFailure.
func stubHermesCommandCheck(t *testing.T, attempts *int, script ...func(ctx context.Context) (found, timedOut bool, err error)) {
	t.Helper()
	old := hermesCommandCheckRunner
	hermesCommandCheckRunner = func(ctx context.Context, _ string, _ time.Duration, _ ...string) (bool, bool, error) {
		*attempts++
		if *attempts <= len(script) {
			return script[*attempts-1](ctx)
		}
		return false, true, nil
	}
	t.Cleanup(func() { hermesCommandCheckRunner = old })
}

// runHermesCheck drives withHermesACPCheck through Detect, as IsInstalled does.
func runHermesCheck(t *testing.T, ctx context.Context) (*DiscoveryResult, error) {
	t.Helper()
	return Detect(ctx, withHermesACPCheck(hermesStagedFirst, hermesStagedSecond))
}

// TestHermesACP_RetriesOnlyATimedOutCheck covers the distinction that
// detection turned on: a bound elapsing is inconclusive and worth measuring
// again, while the check itself answering is a final answer. Without this,
// a healthy Hermes was reported as not installed every time a concurrent
// discovery sweep stretched its check past the shared bound.
func TestHermesACP_RetriesOnlyATimedOutCheck(t *testing.T) {
	installHermesACPCheckHelper(t)

	t.Run("second measurement succeeds after a timeout", func(t *testing.T) {
		// Deterministic: the runner stub counts attempts in the caller, so
		// this no longer depends on the helper subprocess recording its
		// start before the first bound elapses.
		installHermesACPCheckHelper(t)
		var attempts int
		stubHermesCommandCheck(t, &attempts,
			func(context.Context) (bool, bool, error) { return false, true, nil },
			func(context.Context) (bool, bool, error) { return true, false, nil },
		)

		result, err := runHermesCheck(t, context.Background())
		if err != nil {
			t.Fatalf("Detect error: %v", err)
		}
		if !result.Available {
			t.Fatal("Available=false; a timed-out check that then succeeds must report installed")
		}
		if result.MatchedPath == "" {
			t.Error("MatchedPath is empty for an available agent")
		}
		if attempts != 2 {
			t.Errorf("check attempts = %d, want 2 (one timed-out, one retry)", attempts)
		}
	})

	t.Run("both measurements time out", func(t *testing.T) {
		// Deterministic: same stub approach — both scripted measurements
		// report an elapsed bound, so the retry branch must run exactly twice
		// regardless of subprocess scheduling under a full package run.
		installHermesACPCheckHelper(t)
		var attempts int
		stubHermesCommandCheck(t, &attempts,
			func(context.Context) (bool, bool, error) { return false, true, nil },
			func(context.Context) (bool, bool, error) { return false, true, nil },
		)

		result, err := runHermesCheck(t, context.Background())
		if err != nil {
			t.Fatalf("Detect error: %v", err)
		}
		if result.Available {
			t.Fatal("Available=true although neither measurement answered")
		}
		if attempts != 2 {
			t.Errorf("check attempts = %d, want 2 (both bounds elapsed)", attempts)
		}
	})

	t.Run("a nonzero exit is not retried", func(t *testing.T) {
		useHermesCheckCounter(t)
		t.Setenv(hermesACPCheckHelperEnv, "counted-unavailable")

		result, err := runHermesCheck(t, context.Background())
		if err != nil {
			t.Fatalf("Detect error: %v", err)
		}
		if result.Available {
			t.Fatal("Available=true although hermes acp --check failed")
		}
		if got := hermesCheckInvocationCount(t); got != 1 {
			t.Errorf("helper invocations = %d, want 1; a conclusive failure must not be retried", got)
		}
	})

	t.Run("a slow check within the first bound is available", func(t *testing.T) {
		useHermesCheckCounter(t)
		t.Setenv(hermesACPCheckHelperEnv, "always-slow")

		result, err := Detect(context.Background(),
			withHermesACPCheck(10*time.Second, 10*time.Second))
		if err != nil {
			t.Fatalf("Detect error: %v", err)
		}
		if !result.Available {
			t.Fatal("Available=false although the check answered within the bound")
		}
		if got := hermesCheckInvocationCount(t); got != 1 {
			t.Errorf("helper invocations = %d, want 1 (the first bound was sufficient)", got)
		}
	})

	t.Run("returns cancellation from caller context", func(t *testing.T) {
		t.Setenv(hermesACPCheckHelperEnv, "always-slow")
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		result, err := runHermesCheck(t, ctx)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Detect error = %v, want context.Canceled", err)
		}
		if result.Available {
			t.Fatal("Available=true with a cancelled context")
		}
	})

	t.Run("a missing binary stays unavailable", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())

		result, err := runHermesCheck(t, context.Background())
		if err != nil {
			t.Fatalf("Detect error: %v", err)
		}
		if result.Available {
			t.Fatal("Available=true without hermes on PATH; discovery must not imply install")
		}
	})
}

// @covers AC-AGENTS-INSTALL-DETECTION-001.2, AC-AGENTS-INSTALL-DETECTION-001.3
func TestHermesACP_CallerCancellationDuringRetry(t *testing.T) {
	installHermesACPCheckHelper(t)
	for _, tc := range []struct {
		name   string
		cancel bool
		want   error
	}{
		{name: "caller cancels second check", cancel: true, want: context.Canceled},
		{name: "parent deadline during second check", want: context.DeadlineExceeded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				started, stopped := make(chan struct{}), make(chan struct{})
				var attempts int
				stubHermesCommandCheck(t, &attempts,
					func(context.Context) (bool, bool, error) { return false, true, nil },
					func(ctx context.Context) (bool, bool, error) {
						defer close(stopped)
						close(started)
						<-ctx.Done()
						return false, false, ctx.Err()
					},
				)
				type outcome struct {
					result *DiscoveryResult
					err    error
				}
				out, done := make(chan outcome, 1), make(chan struct{})
				go func() {
					defer close(done)
					result, err := runHermesCheck(t, ctx)
					out <- outcome{result: result, err: err}
				}()
				defer func() { cancel(); <-done }()
				<-started
				if tc.cancel {
					cancel()
				}
				<-ctx.Done()
				<-done
				got := <-out
				if attempts != 2 {
					t.Errorf("check attempts = %d, want exactly 2", attempts)
				}
				if !errors.Is(got.err, tc.want) || got.err != ctx.Err() {
					t.Errorf("Detect error = %v, want caller error %v", got.err, tc.want)
				}
				if got.result.Available || got.result.MatchedPath != "" {
					t.Errorf("cancelled retry result = %+v, want unavailable without a path", got.result)
				}
				select {
				case <-stopped:
				default:
					t.Error("second check did not finish before Detect returned")
				}
			})
		})
	}
}

func TestHermesACP_LogosNonEmpty(t *testing.T) {
	a := NewHermesACP()
	if len(a.Logo(LogoLight)) == 0 {
		t.Error("Logo(LogoLight) is empty")
	}
	if len(a.Logo(LogoDark)) == 0 {
		t.Error("Logo(LogoDark) is empty")
	}
	if !strings.Contains(string(a.Logo(LogoLight)), "<svg") {
		t.Error("Logo(LogoLight) is not SVG")
	}
}

func TestHermesACP_RemoteAuth(t *testing.T) {
	auth := NewHermesACP().RemoteAuth()
	if auth == nil {
		t.Fatal("RemoteAuth() returned nil")
	}
	if len(auth.Methods) != 1 {
		t.Fatalf("Methods len = %d, want 1", len(auth.Methods))
	}

	files := auth.Methods[0]
	if files.Type != "files" {
		t.Errorf("Methods[0].Type = %q, want files", files.Type)
	}
	if files.TargetRelDir != ".hermes" {
		t.Errorf("TargetRelDir = %q, want .hermes", files.TargetRelDir)
	}
	for _, osName := range []string{"darwin", "linux"} {
		paths := files.SourceFiles[osName]
		want := []string{".hermes/.env", ".hermes/config.yaml"}
		if !slices.Equal(paths, want) {
			t.Errorf("SourceFiles[%s] = %#v, want %#v", osName, paths, want)
		}
		for _, p := range paths {
			if strings.Contains(p, "state.db") || strings.Contains(p, "skills") {
				t.Errorf("must not copy non-auth state: %q", p)
			}
		}
	}
}

func TestHermesACP_LoginCommand(t *testing.T) {
	cmd := NewHermesACP().LoginCommand()
	if cmd == nil {
		t.Fatal("LoginCommand() returned nil")
	}
	want := []string{"hermes", "model"}
	if !slices.Equal(cmd.Cmd, want) {
		t.Errorf("LoginCommand.Cmd = %#v, want %#v", cmd.Cmd, want)
	}
	if cmd.Description == "" {
		t.Error("LoginCommand.Description is empty")
	}
}

func TestHermesACP_SessionAndSkills(t *testing.T) {
	rt := NewHermesACP().Runtime()
	if rt.WorkingDir != "{workspace}" {
		t.Errorf("WorkingDir = %q, want {workspace}", rt.WorkingDir)
	}
	if rt.UserSkillDir != ".hermes/skills" {
		t.Errorf("UserSkillDir = %q, want .hermes/skills", rt.UserSkillDir)
	}
	sc := rt.SessionConfig
	if !sc.NativeSessionResume {
		t.Error("NativeSessionResume = false, want true")
	}
	if sc.NewSessionOnWorkspaceRebind {
		t.Error("NewSessionOnWorkspaceRebind = true, want false (Hermes reloads sessions with the new cwd)")
	}
	if sc.CanRecover == nil || !*sc.CanRecover {
		t.Error("CanRecover must be true")
	}
	if sc.SessionDirTemplate != "{home}/.hermes" {
		t.Errorf("SessionDirTemplate = %q, want {home}/.hermes", sc.SessionDirTemplate)
	}
	if sc.SessionDirTarget != "/root/.hermes" {
		t.Errorf("SessionDirTarget = %q, want /root/.hermes", sc.SessionDirTarget)
	}
}

func TestHermesACP_PermissionAndBillingDefaults(t *testing.T) {
	a := NewHermesACP()
	if len(a.PermissionSettings()) != 0 {
		t.Errorf("PermissionSettings() = %#v, want empty (agentctl auto-approve is authoritative)", a.PermissionSettings())
	}
	if got := a.BillingType(); got != usage.BillingTypeAPIKey {
		t.Errorf("BillingType() = %q, want %q", got, usage.BillingTypeAPIKey)
	}
	catalog := CatalogPermissionSettings(a)
	auto, ok := catalog[PermissionKeyAutoApprove]
	if !ok {
		t.Fatal("catalog missing auto_approve")
	}
	if auto.ApplyMethod != PermissionApplyMethodAgentctlAutoApprove {
		t.Errorf("auto_approve ApplyMethod = %q, want agentctl auto-approve", auto.ApplyMethod)
	}
}
