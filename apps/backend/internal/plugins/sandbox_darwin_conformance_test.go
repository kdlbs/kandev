package plugins

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/processidentity"
	"github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/common/sandboxmcp"
	"github.com/kandev/kandev/internal/common/sandboxproxy"
	pluginruntime "github.com/kandev/kandev/internal/plugins/runtime"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"github.com/stretchr/testify/require"
)

// The darwin conformance suite launches the reviewed Nono package through the
// same core steps a sandboxed attempt uses (reserve the per-attempt root,
// compile through the installed provider, materialize, start the wrapper in its
// own session) and runs each canary against a control. Output names canaries
// and results only: no real file name, content, or secret value.
const (
	darwinCanaryBound    = 10 * time.Second
	darwinMarkerPrefix   = ".kandev-conformance-"
	darwinKeychainLabel  = "kandev-conformance-service"
	darwinPluginVariable = "KANDEV_SANDBOX_CONFORMANCE_PLUGIN"
)

type darwinOutcome string

const (
	outcomeAllowed       darwinOutcome = "allowed"
	outcomeDenied        darwinOutcome = "denied"
	outcomeControlFailed darwinOutcome = "control_failed"
	outcomeError         darwinOutcome = "error"
	outcomeSkipped       darwinOutcome = "skipped"
)

type darwinRun struct {
	exit  int
	fault bool
}

func (r darwinRun) outcome(control bool) darwinOutcome {
	switch {
	case r.fault:
		return outcomeError
	case r.exit == 0:
		return outcomeAllowed
	case r.exit == 10 && !control:
		return outcomeDenied
	}
	return outcomeError
}

type darwinCanary struct {
	name       string
	args       []string
	expect     darwinOutcome
	skipUnless string
}

// @covers AC-AGENTS-SANDBOX-011.11
func TestSandboxDarwinConformance(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the darwin conformance suite runs on macOS only")
	}
	suite := newDarwinSuite(t)
	canaries := suite.canaries()
	lines := make([]string, 0, len(canaries)+3)
	failed := 0
	for _, c := range canaries {
		line, ok := suite.exercise(c)
		lines = append(lines, line)
		if !ok {
			failed++
		}
	}
	for _, c := range []struct {
		name string
		args []string
	}{{"acp_stdio_round_trip", []string{"stdio"}}, {"acp_stdio_during_denial", []string{"stdio", suite.fixtures["other_home"]}}} {
		line, ok := suite.stdio(c.name, c.args)
		lines = append(lines, line)
		if !ok {
			failed++
		}
	}
	line, ok := suite.ownership()
	lines = append(lines, line)
	if !ok {
		failed++
	}
	t.Logf("macOS %s darwin conformance, core launch\n%s", darwinVersion(), strings.Join(lines, "\n"))
	require.Zero(t, failed, "a canary did not report the expected result")
	require.Zero(t, suite.unverified, "an attempt left process group members behind")
}

type darwinSuite struct {
	t           *testing.T
	ctx         context.Context
	root        string
	svc         *Service
	binding     SandboxProviderBinding
	runtimeBin  *SandboxRuntimeBinary
	helper      string
	work        string
	fixtures    map[string]string
	keychain    string
	direct      string
	other       string
	relay       *sandboxmcp.Relay
	relayPort   int
	outsideDir  string
	tmpMarker   string
	mu          sync.Mutex
	unverified  int
	description *pluginsdk.SandboxProviderDescription
}

func newDarwinSuite(t *testing.T) *darwinSuite {
	t.Helper()
	s := &darwinSuite{t: t, ctx: context.Background(), fixtures: map[string]string{}}
	s.root = darwinFixtureRoot(t)
	s.installProvider()
	s.work = filepath.Join(s.root, "work")
	s.outsideDir = filepath.Join(s.root, "outside")
	require.NoError(t, os.MkdirAll(s.work, 0o700))
	require.NoError(t, os.MkdirAll(s.outsideDir, 0o700))
	s.buildHelper()
	s.fixtures["other_home"] = darwinFixture(t, filepath.Join(s.root, "other-home", "secret.txt"))
	s.fixtures["other_work"] = darwinFixture(t, filepath.Join(s.root, "other-worktree", "file.txt"))
	s.fixtures["ssh"] = darwinFixture(t, filepath.Join(s.root, "fixture-home", ".ssh", "id_fixture"))
	s.fixtures["browser"] = darwinFixture(t, filepath.Join(s.root, "fixture-home", "Library", "browser", "Cookies"))
	s.keychain = filepath.Join(s.root, "throwaway.keychain-db")
	darwinKeychain(t, s.keychain)
	s.direct, s.other = darwinListen(t), darwinListen(t)
	s.openRelay()
	s.tmpMarker = "/private/tmp/" + filepath.Base(s.root) + "-canary"
	t.Cleanup(func() { _ = os.Remove(s.tmpMarker) })
	return s
}

func (s *darwinSuite) installProvider() {
	t := s.t
	archive := darwinPackagePath(t)
	svc, dir, _, _ := newTestServiceWithDir(t)
	svc.SetSandboxProvidersEnabled(true)
	svc.SetRuntime(pluginruntime.NewManager(dir, svc.handleStatusChange, testLogger(t)))
	file, err := os.Open(archive)
	require.NoError(t, err)
	t.Cleanup(func() { _ = file.Close() })
	record, err := svc.Install(s.ctx, file)
	require.NoError(t, err)
	require.Len(t, record.SandboxProviders, 1)
	s.svc = svc
	s.binding = sandboxFixtureBinding(record, record.SandboxProviders[0].Key)
	facts := darwinFacts()
	described, err := svc.DescribeSandboxProvider(s.ctx, s.binding, &pluginsdk.DescribeSandboxProviderRequest{Context: sandboxFixtureCompileRequest(s.binding).Context, Runtime: facts})
	require.NoError(t, err)
	s.description = described.Description
	require.Nil(t, s.description.ApprovalWire, "a darwin description declares no approval wire")
	s.runtimeBin, err = svc.ReadSandboxRuntime(s.ctx, s.binding, s.description.RuntimeArtifact)
	require.NoError(t, err)
}

func darwinFacts() *pluginsdk.SandboxRuntimeFacts {
	return &pluginsdk.SandboxRuntimeFacts{Os: "darwin", Architecture: runtime.GOARCH, Transport: "local"}
}

// darwinPackagePath resolves the reviewed package. The variable names either
// the package file or the directory holding it; a missing variable or package
// fails the suite.
func darwinPackagePath(t *testing.T) string {
	t.Helper()
	location := os.Getenv(darwinPluginVariable)
	require.NotEmpty(t, location, "set %s to the reviewed Nono package directory", darwinPluginVariable)
	info, err := os.Stat(location)
	require.NoError(t, err, "the reviewed Nono package is missing")
	if !info.IsDir() {
		return location
	}
	matches, err := filepath.Glob(filepath.Join(location, "kandev-plugin-nono-*.tar.gz"))
	require.NoError(t, err)
	require.Len(t, matches, 1, "the reviewed Nono package is missing from the package directory")
	return matches[0]
}

func (s *darwinSuite) buildHelper() {
	s.helper = filepath.Join(s.root, "canary")
	build := exec.Command("go", "build", "-trimpath", "-o", s.helper, "./testdata/sandboxcanary")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	output, err := build.CombinedOutput()
	require.NoError(s.t, err, string(output))
}

func (s *darwinSuite) openRelay() {
	relay, err := sandboxmcp.Open(s.ctx, func(context.Context, string, http.Header, []byte) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: http.NoBody}, nil
	})
	require.NoError(s.t, err)
	s.t.Cleanup(func() { _ = relay.Close() })
	_, port, err := net.SplitHostPort(relay.Address())
	require.NoError(s.t, err)
	s.relayPort, err = strconv.Atoi(port)
	require.NoError(s.t, err)
	s.relay = relay
}

func (s *darwinSuite) canaries() []darwinCanary {
	home, _ := os.UserHomeDir()
	chrome := filepath.Join(home, "Library", "Application Support", "Google", "Chrome")
	return []darwinCanary{
		{name: "other_home_directory", args: []string{"read", s.fixtures["other_home"]}, expect: outcomeDenied},
		{name: "other_task_worktree", args: []string{"read", s.fixtures["other_work"]}, expect: outcomeDenied},
		{name: "tmp_write", args: []string{"write", s.tmpMarker}, expect: outcomeDenied},
		{name: "ssh_listing", args: []string{"list", filepath.Join(home, ".ssh")}, expect: outcomeDenied, skipUnless: filepath.Join(home, ".ssh")},
		{name: "ssh_fixture_listing", args: []string{"list", filepath.Dir(s.fixtures["ssh"])}, expect: outcomeDenied},
		{name: "browser_data_listing", args: []string{"list", chrome}, expect: outcomeDenied, skipUnless: chrome},
		{name: "browser_data_fixture_listing", args: []string{"list", filepath.Dir(s.fixtures["browser"])}, expect: outcomeDenied},
		{name: "keychain_item", args: []string{"keychain", s.keychain, darwinKeychainLabel}, expect: outcomeDenied},
		{name: "write_outside_granted", args: []string{"write", filepath.Join(s.outsideDir, "file.txt")}, expect: outcomeDenied},
		{name: "write_inside_work", args: []string{"write", filepath.Join(s.work, "allowed.txt")}, expect: outcomeAllowed},
		{name: "direct_loopback_egress", args: []string{"dial", s.direct}, expect: outcomeDenied},
		{name: "arbitrary_loopback_port", args: []string{"dial", s.other}, expect: outcomeDenied},
		{name: "mcp_relay_route", args: []string{"proxy-post", s.relay.URL()}, expect: outcomeAllowed},
	}
}

// attempt is one core launch: a distinct reserved root, a compiled profile, and
// the materialized runtime. Closing it removes the root after the group is
// verified empty.
type attempt struct {
	reserved *sandbox.ReservedRoot
	files    *sandbox.RuntimeFiles
	compiled *pluginsdk.SandboxCompiledPolicy
	proxy    *sandboxproxy.Proxy
}

func (s *darwinSuite) prepare() *attempt {
	t := s.t
	base := filepath.Join(s.root, "state")
	require.NoError(t, os.MkdirAll(base, 0o700))
	reserved, err := sandbox.ReserveRuntimeRoot(base)
	require.NoError(t, err)
	req := sandboxFixtureCompileRequest(s.binding)
	req.Runtime = darwinFacts()
	req.Runtime.RuntimeVersion = s.description.RuntimeVersion
	req.Runtime.RuntimeDigest = s.runtimeBin.Digest
	req.References = []*pluginsdk.SandboxRuntimeReference{{Key: "sandbox.child.tmp", Value: reserved.TempDir()}}
	req.Policy.Grants = &pluginsdk.SandboxGrants{KandevMcp: true, Filesystem: []*pluginsdk.SandboxFilesystemGrant{{Path: s.work, Writable: true}, {Path: s.helper}}}
	req.Endpoints = []*pluginsdk.SandboxEndpoint{{Key: "kandev.mcp", Host: "127.0.0.1", Port: uint32(s.relayPort), Scheme: "http", Path: "/mcp"}}
	compiled, err := s.svc.CompileSandboxPolicy(s.ctx, s.binding, req)
	require.NoError(t, err)
	proxy, err := sandboxproxy.Open(s.ctx, []sandbox.NetworkGrant{{Host: "127.0.0.1", Port: uint16(s.relayPort), Protocol: "tcp"}})
	require.NoError(t, err)
	artifacts := make([]sandbox.RuntimeArtifact, 0, len(compiled.Compiled.Artifacts))
	for _, artifact := range compiled.Compiled.Artifacts {
		value := sandbox.RuntimeArtifact{Name: artifact.Name, Content: artifact.ContentUtf8}
		for _, binding := range artifact.Bindings {
			value.Bindings = append(value.Bindings, sandbox.ArtifactBinding{Path: binding.Path, Reference: binding.RuntimeReference})
		}
		artifacts = append(artifacts, value)
	}
	files, err := reserved.Materialize(s.ctx, s.runtimeBin.Content, s.runtimeBin.Digest, artifacts, map[string]string{"sandbox.child.tmp": reserved.TempDir(), "sandbox.egress.endpoint": proxy.Address()})
	require.NoError(t, err)
	return &attempt{reserved: reserved, files: files, compiled: compiled.Compiled, proxy: proxy}
}

func (a *attempt) command(t *testing.T, ctx context.Context, work, helper string, args []string) *exec.Cmd {
	t.Helper()
	argv := []string{a.files.Executable}
	for _, argument := range a.compiled.Prefix[1:] {
		switch value := argument.Value.(type) {
		case *pluginsdk.SandboxArgument_Literal:
			argv = append(argv, value.Literal)
		case *pluginsdk.SandboxArgument_ArtifactReference:
			argv = append(argv, a.files.Artifacts[value.ArtifactReference])
		default:
			t.Fatal("unexpected runtime reference")
		}
	}
	argv = append(append(argv, helper), args...)
	command := exec.CommandContext(ctx, argv[0], argv[1:]...)
	command.Dir = work
	env, err := sandbox.IsolatedEnvironment(map[string]string{"PATH": "/usr/bin:/bin"}, nil, a.files.Directories)
	require.NoError(t, err)
	command.Env = env
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	command.Cancel = func() error { return syscall.Kill(-command.Process.Pid, syscall.SIGKILL) }
	command.WaitDelay = time.Second
	return command
}

// close verifies the wrapper's group is empty, then removes the attempt's files.
// A group that does not empty is an unverified termination: the root stays for
// the next owner-liveness purge and the suite fails.
func (a *attempt) close(s *darwinSuite, wrapper int) {
	if a.proxy != nil {
		_ = a.proxy.Close()
	}
	if wrapper > 0 && !groupEnded(wrapper) {
		s.mu.Lock()
		s.unverified++
		s.mu.Unlock()
		return
	}
	_ = a.files.Close()
}

func groupEnded(pgid int) bool {
	deadline := time.Now().Add(5 * time.Second)
	for {
		members, err := processidentity.GroupMembers(pgid)
		if err == nil && len(members) == 0 {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// launch runs the helper under a sandbox attempt (or directly as the control)
// and returns its exit, stdout, and the wrapper's process ID.
func (s *darwinSuite) launch(sandboxed bool, stdin string, args ...string) (darwinRun, []byte, int) {
	ctx, cancel := context.WithTimeout(s.ctx, darwinCanaryBound)
	defer cancel()
	var command *exec.Cmd
	var attempt *attempt
	if sandboxed {
		attempt = s.prepare()
		command = attempt.command(s.t, ctx, s.work, s.helper, args)
	} else {
		command = exec.CommandContext(ctx, s.helper, args...)
		command.Dir = s.work
		command.Env = []string{"PATH=/usr/bin:/bin"}
		command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		command.Cancel = func() error { return syscall.Kill(-command.Process.Pid, syscall.SIGKILL) }
		command.WaitDelay = time.Second
	}
	command.Stdin = strings.NewReader(stdin)
	var stdout bytes.Buffer
	command.Stdout = &stdout
	if err := command.Start(); err != nil {
		if attempt != nil {
			attempt.close(s, 0)
		}
		return darwinRun{fault: true}, nil, 0
	}
	wrapper := command.Process.Pid
	err := command.Wait()
	if attempt != nil {
		attempt.close(s, wrapper)
	}
	var exit *exec.ExitError
	switch {
	case err == nil:
		return darwinRun{}, stdout.Bytes(), wrapper
	case errors.As(err, &exit) && exit.ExitCode() >= 0:
		return darwinRun{exit: exit.ExitCode()}, stdout.Bytes(), wrapper
	}
	return darwinRun{fault: true}, stdout.Bytes(), wrapper
}

func (s *darwinSuite) exercise(c darwinCanary) (string, bool) {
	if c.skipUnless != "" {
		if _, err := os.Stat(c.skipUnless); err != nil {
			return fmt.Sprintf("%-32s %s", c.name, outcomeSkipped), true
		}
	}
	controlArgs := c.args
	if c.args[0] == "proxy-post" {
		controlArgs = []string{"post-direct", c.args[1]}
	}
	control, _, _ := s.launch(false, "", controlArgs...)
	sandboxed, _, _ := s.launch(true, "", c.args...)
	line := fmt.Sprintf("%-32s control=%s sandbox=%s exit=%d", c.name, control.outcome(true), sandboxed.outcome(false), sandboxed.exit)
	if control.outcome(true) != outcomeAllowed {
		return line + " " + string(outcomeControlFailed), false
	}
	return line, sandboxed.outcome(false) == c.expect
}

// stdio proves ACP stdio passes unchanged: zero non-JSON stdout lines, with and
// without a denial while the stream is open.
func (s *darwinSuite) stdio(name string, args []string) (string, bool) {
	request := "{\"jsonrpc\":\"2.0\",\"id\":7,\"method\":\"ping\"}\n"
	control, controlOut, _ := s.launch(false, request, args...)
	sandboxed, out, _ := s.launch(true, request, args...)
	valid := control.outcome(true) == outcomeAllowed && sandboxed.outcome(false) == outcomeAllowed && darwinJSONLines(controlOut, true) == 1 && darwinJSONLines(out, true) == 1 && darwinJSONLines(out, false) == 0
	return fmt.Sprintf("%-32s control=%s sandbox=%s non_json_lines=%d", name, control.outcome(true), sandboxed.outcome(false), darwinJSONLines(out, false)), valid
}

func darwinJSONLines(data []byte, valid bool) int {
	count := 0
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		if json.Valid(scanner.Bytes()) == valid {
			count++
		}
	}
	return count
}

// ownership proves the sandboxed child stays in the wrapper's process group.
func (s *darwinSuite) ownership() (string, bool) {
	run, out, wrapper := s.launch(true, "", "group")
	var reply struct{ Pgid int }
	ok := run.outcome(false) == outcomeAllowed && json.Unmarshal(out, &reply) == nil && reply.Pgid == wrapper && wrapper > 0
	return fmt.Sprintf("%-32s sandbox=%s child_group_equals_wrapper=%t", "ownership", run.outcome(false), ok), ok
}

// darwinFixtureRoot creates the marker-named root under the user's home,
// outside every denied temporary root. The marker records the owning process; a
// purge at start removes roots whose owner is gone. The root is removed on
// success, failure, panic, SIGINT and SIGTERM.
func darwinFixtureRoot(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	purgeDarwinRoots(home)
	root, err := os.MkdirTemp(home, darwinMarkerPrefix+"p"+strconv.Itoa(os.Getpid())+"-")
	require.NoError(t, err)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		if _, ok := <-signals; ok {
			_ = os.RemoveAll(root)
			os.Exit(130)
		}
	}()
	t.Cleanup(func() {
		signal.Stop(signals)
		close(signals)
		_ = os.RemoveAll(root)
	})
	return root
}

func purgeDarwinRoots(home string) {
	entries, _ := os.ReadDir(home)
	pattern := regexp.MustCompile(`^` + regexp.QuoteMeta(darwinMarkerPrefix) + `p(\d+)-`)
	for _, entry := range entries {
		if match := pattern.FindStringSubmatch(entry.Name()); match != nil {
			pid, _ := strconv.Atoi(match[1])
			if errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
				_ = os.RemoveAll(filepath.Join(home, entry.Name()))
			}
		}
	}
}

func darwinFixture(t *testing.T, path string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte("synthetic fixture content"), 0o600))
	return path
}

func darwinListen(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()
	return listener.Addr().String()
}

// darwinKeychain makes a throwaway keychain file that is never added to the
// search list. The login keychain is never read, modified or unlocked.
func darwinKeychain(t *testing.T, path string) {
	t.Helper()
	for _, args := range [][]string{
		{"create-keychain", "-p", "synthetic", path},
		{"add-generic-password", "-a", "account", "-s", darwinKeychainLabel, "-w", "synthetic-secret", path},
	} {
		output, err := exec.Command("/usr/bin/security", args...).CombinedOutput()
		require.NoError(t, err, "keychain setup failed: %d bytes of output withheld", len(output))
	}
}

func darwinVersion() string {
	output, err := exec.Command("sw_vers", "-productVersion").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(output))
}
