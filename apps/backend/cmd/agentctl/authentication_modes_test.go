package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
)

const (
	agentctlAuthTestHelperEnv = "KANDEV_AGENTCTL_AUTH_TEST_HELPER"
	agentctlAuthTestHelperArg = "agentctl-authentication-modes-helper"
	agentctlAuthTestNonce     = "authentication-modes-test-bootstrap-nonce"
	agentctlAuthFailureEnv    = "KANDEV_AGENTCTL_AUTH_TEST_FAILURE_HELPER"
	agentctlAuthFailureReady  = "agentctl-authentication-modes-failure-helper-ready"
)

func TestAgentctlAuthenticationModes(t *testing.T) {
	mockAgent := buildAuthenticationModesMockAgent(t)

	t.Run("tokenless", func(t *testing.T) {
		verifyAgentctlAuthenticationMode(t, mockAgent, false)
	})
	t.Run("authenticated", func(t *testing.T) {
		verifyAgentctlAuthenticationMode(t, mockAgent, true)
	})
}

// TestAgentctlAuthenticationModesHelperProcess enters the production server
// startup path in a separate process so each mode owns its listeners and
// subprocesses until the test sends an interrupt.
func TestAgentctlAuthenticationModesHelperProcess(t *testing.T) {
	if os.Getenv(agentctlAuthTestHelperEnv) != "1" || !hasAgentctlAuthTestHelperArg(os.Args) {
		return
	}

	cfg := config.Load()
	log, err := logger.NewLogger(resolveRunLoggingConfig(cfg))
	if err != nil {
		t.Fatalf("create agentctl test logger: %v", err)
	}
	defer func() { _ = log.Close() }()
	run(cfg, log)
}

func TestAgentctlAuthenticationModesUnexpectedExitHelperProcess(t *testing.T) {
	if os.Getenv(agentctlAuthFailureEnv) != "1" {
		return
	}
	_, _ = fmt.Fprintln(os.Stdout, agentctlAuthFailureReady)
	command, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil || strings.TrimSpace(command) != "exit" {
		os.Exit(22)
	}
	os.Exit(23)
}

func TestAgentctlAuthenticationModesDetectsHelperFailureAfterReadiness(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	command := exec.Command(executable, "-test.run=^TestAgentctlAuthenticationModesUnexpectedExitHelperProcess$", "-test.v")
	command.Env = append(os.Environ(), agentctlAuthFailureEnv+"=1")
	server := &authenticationModesServer{
		command:  command,
		waitDone: make(chan struct{}),
		stdout:   &synchronizedBuffer{},
		stderr:   &synchronizedBuffer{},
	}
	command.Stdout = server.stdout
	command.Stderr = server.stderr
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatalf("open helper stdin: %v", err)
	}
	if err := command.Start(); err != nil {
		t.Fatalf("start failing helper subprocess: %v", err)
	}
	go func() {
		server.waitErr = command.Wait()
		close(server.waitDone)
	}()

	exitObserved := false
	t.Cleanup(func() {
		if exitObserved {
			return
		}
		if err := server.close(t); err != nil {
			t.Error(err)
		}
	})

	if err := waitForAuthenticationModesOutput(server, agentctlAuthFailureReady, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(stdin, "exit\n"); err != nil {
		t.Fatalf("release failing helper: %v", err)
	}
	if err := stdin.Close(); err != nil {
		t.Fatalf("close helper stdin: %v", err)
	}
	select {
	case <-server.waitDone:
	case <-time.After(5 * time.Second):
		_ = command.Process.Kill()
		<-server.waitDone
		t.Fatalf("failing helper did not exit after release\n%s", server.output())
	}

	exitObserved = true
	if err := server.close(t); err == nil {
		t.Fatal("parent cleanup did not report helper's nonzero exit")
	} else if !strings.Contains(err.Error(), "exit status 23") || !strings.Contains(err.Error(), agentctlAuthFailureReady) {
		t.Fatalf("parent cleanup error = %v, want exit status and captured readiness output", err)
	}
}

func hasAgentctlAuthTestHelperArg(args []string) bool {
	for _, arg := range args {
		if arg == agentctlAuthTestHelperArg {
			return true
		}
	}
	return false
}

type authenticationModesServer struct {
	command   *exec.Cmd
	waitDone  chan struct{}
	waitErr   error
	stdout    *synchronizedBuffer
	stderr    *synchronizedBuffer
	control   *agentctlclient.ControlClient
	clients   []*agentctlclient.Client
	instances []string
	logger    *logger.Logger
	port      int
	root      string
}

type synchronizedBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *synchronizedBuffer) Write(value []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(value)
}

func (b *synchronizedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

func verifyAgentctlAuthenticationMode(t *testing.T, mockAgent string, authenticated bool) {
	t.Helper()

	root := t.TempDir()
	controlPort := freeTCPPort(t)
	instancePortBase := freeTCPPortRange(t, 3)
	server := startAuthenticationModesServer(t, root, mockAgent, controlPort, instancePortBase, authenticated)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	if err := server.control.Health(ctx); err != nil {
		t.Fatalf("agentctl health: %v", err)
	}

	if authenticated {
		for _, authorization := range []string{"", "Basic malformed", "Bearer incorrect-token"} {
			assertAuthenticationModesUnauthorized(t, controlPort, "/api/v1/instances", authorization, "")
		}

		token, err := server.control.Handshake(ctx, agentctlAuthTestNonce)
		if err != nil {
			t.Fatalf("bootstrap handshake: %v", err)
		}
		if token == "" {
			t.Fatal("bootstrap handshake returned an empty credential")
		}
	}

	firstInstance := createAuthenticationModesInstance(t, ctx, server, mockAgent)
	firstClient := newAuthenticationModesInstanceClient(t, server, firstInstance.ID, firstInstance.Port)
	if authenticated {
		for _, authorization := range []string{"", "Basic malformed", "Bearer incorrect-token"} {
			assertAuthenticationModesUnauthorized(t, firstInstance.Port, "/api/v1/status", authorization, firstInstance.ID)
		}
	}

	if err := firstClient.ConfigureAgent(ctx, mockAgent, nil, nil, "", nil); err != nil {
		t.Fatalf("configure agent: %v", err)
	}
	if _, err := firstClient.GetStatus(ctx); err != nil {
		t.Fatalf("read instance status after configuration: %v", err)
	}
	runAuthenticationModesTurn(t, ctx, firstClient, firstInstance.WorkspacePath)

	if !authenticated {
		return
	}

	oldCredential := server.control.AuthToken()
	rotation, err := server.control.RotateCredential(ctx)
	if err != nil {
		t.Fatalf("rotate control credential: %v", err)
	}
	if rotation.Credential == "" || rotation.Credential == oldCredential {
		t.Fatal("credential rotation did not issue a new credential")
	}
	server.control.SetAuthToken(rotation.Credential)
	if err := server.control.ConfirmCredentialRotation(ctx, rotation.RotationID); err != nil {
		t.Fatalf("confirm control credential rotation: %v", err)
	}

	oldInstanceClient := newAuthenticationModesInstanceClientWithToken(t, server, firstInstance.ID, firstInstance.Port, oldCredential)
	if _, err := oldInstanceClient.GetStatus(ctx); err == nil {
		t.Fatal("existing instance accepted a superseded credential")
	}
	newInstanceClient := newAuthenticationModesInstanceClient(t, server, firstInstance.ID, firstInstance.Port)
	if _, err := newInstanceClient.GetStatus(ctx); err != nil {
		t.Fatalf("existing instance rejected the rotated credential: %v", err)
	}

	secondInstance := createAuthenticationModesInstance(t, ctx, server, mockAgent)
	secondClient := newAuthenticationModesInstanceClient(t, server, secondInstance.ID, secondInstance.Port)
	if err := secondClient.ConfigureAgent(ctx, mockAgent, nil, nil, "", nil); err != nil {
		t.Fatalf("configure instance created after credential rotation: %v", err)
	}
	if _, err := secondClient.GetStatus(ctx); err != nil {
		t.Fatalf("read status for instance created after credential rotation: %v", err)
	}
}

type authenticationModesInstance struct {
	ID            string
	Port          int
	WorkspacePath string
}

func createAuthenticationModesInstance(t *testing.T, ctx context.Context, server *authenticationModesServer, mockAgent string) authenticationModesInstance {
	t.Helper()
	workspace := t.TempDir()
	created, err := server.control.CreateInstance(ctx, &agentctlclient.CreateInstanceRequest{
		WorkspacePath: workspace,
		AgentCommand:  mockAgent,
		AgentType:     "mock-agent",
		Protocol:      "acp",
	})
	if err != nil {
		t.Fatalf("create instance: %v", err)
	}
	server.instances = append(server.instances, created.ID)
	return authenticationModesInstance{ID: created.ID, Port: created.Port, WorkspacePath: workspace}
}

func newAuthenticationModesInstanceClient(t *testing.T, server *authenticationModesServer, instanceID string, port int) *agentctlclient.Client {
	t.Helper()
	return newAuthenticationModesInstanceClientWithToken(t, server, instanceID, port, server.control.AuthToken())
}

func newAuthenticationModesInstanceClientWithToken(t *testing.T, server *authenticationModesServer, instanceID string, port int, token string) *agentctlclient.Client {
	t.Helper()
	options := []agentctlclient.ClientOption{agentctlclient.WithExecutionID(instanceID)}
	if token != "" {
		options = append(options, agentctlclient.WithAuthToken(token))
	}
	client := agentctlclient.NewClient("127.0.0.1", port, server.logger, options...)
	server.clients = append(server.clients, client)
	return client
}

func runAuthenticationModesTurn(t *testing.T, ctx context.Context, client *agentctlclient.Client, workspace string) {
	t.Helper()
	if _, err := client.Start(ctx); err != nil {
		t.Fatalf("start mock agent: %v", err)
	}

	events := make(chan agentctlclient.AgentEvent, 256)
	streamCtx, stopStream := context.WithCancel(ctx)
	defer stopStream()
	if err := client.StreamUpdates(streamCtx, func(event agentctlclient.AgentEvent) {
		events <- event
	}, nil, nil); err != nil {
		t.Fatalf("connect agent stream: %v", err)
	}
	if _, err := client.Initialize(ctx, "kandev-authentication-test", "1"); err != nil {
		t.Fatalf("initialize mock agent: %v", err)
	}
	if _, err := client.NewSession(ctx, workspace, nil); err != nil {
		t.Fatalf("create mock-agent session: %v", err)
	}
	if err := client.Prompt(ctx, `e2e:message("agentctl-authentication-mode-ok")`, nil, 1); err != nil {
		t.Fatalf("send deterministic mock-agent prompt: %v", err)
	}

	var output strings.Builder
	completed := false
	for !completed {
		select {
		case event := <-events:
			if event.Type == streams.EventTypeMessageChunk {
				output.WriteString(event.Text)
			}
			if event.Type == streams.EventTypeComplete {
				completed = true
			}
		case <-ctx.Done():
			t.Fatalf("mock-agent turn did not complete: %v", ctx.Err())
		}
	}
	if !strings.Contains(output.String(), "agentctl-authentication-mode-ok") {
		t.Fatalf("streamed output = %q, missing deterministic response", output.String())
	}
	client.CloseUpdatesStream()
}

func assertAuthenticationModesUnauthorized(t *testing.T, port int, path, authorization, instanceID string) {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d%s", port, path), nil)
	if err != nil {
		t.Fatalf("create unauthorized request: %v", err)
	}
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	if instanceID != "" {
		request.Header.Set("X-Instance-ID", instanceID)
	}
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		t.Fatalf("send unauthorized request to %s: %v", path, err)
	}
	defer func() { _ = response.Body.Close() }()
	_, _ = io.Copy(io.Discard, response.Body)
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("request to %s with Authorization %q returned %d, want 401", path, authorization, response.StatusCode)
	}
}

func startAuthenticationModesServer(t *testing.T, root, mockAgent string, controlPort, instancePortBase int, authenticated bool) *authenticationModesServer {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: filepath.Join(root, "client.log")})
	if err != nil {
		t.Fatalf("create client logger: %v", err)
	}
	t.Cleanup(func() { _ = log.Close() })

	arguments := []string{"-test.run=^TestAgentctlAuthenticationModesHelperProcess$", "--", agentctlAuthTestHelperArg}
	command := exec.Command(executable, arguments...)
	command.Dir = root
	server := &authenticationModesServer{
		command:  command,
		waitDone: make(chan struct{}),
		stdout:   &synchronizedBuffer{},
		stderr:   &synchronizedBuffer{},
		logger:   log,
		port:     controlPort,
		root:     root,
	}
	command.Stdout = server.stdout
	command.Stderr = server.stderr
	command.Env = authenticationModesHelperEnvironment(root, mockAgent, controlPort, instancePortBase, authenticated)
	if err := command.Start(); err != nil {
		t.Fatalf("start agentctl server subprocess: %v", err)
	}
	go func() {
		server.waitErr = command.Wait()
		close(server.waitDone)
	}()
	server.control = agentctlclient.NewControlClient("127.0.0.1", controlPort, log)
	t.Cleanup(func() {
		if err := server.close(t); err != nil {
			t.Error(err)
		}
	})
	waitForAuthenticationModesServer(t, server)
	return server
}

func authenticationModesHelperEnvironment(root, mockAgent string, controlPort, instancePortBase int, authenticated bool) []string {
	environment := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + root,
		"TMPDIR=" + root,
		"KANDEV_HOME_DIR=" + root,
		agentctlAuthTestHelperEnv + "=1",
		"AGENTCTL_PORT=" + strconv.Itoa(controlPort),
		"AGENTCTL_INSTANCE_PORT_BASE=" + strconv.Itoa(instancePortBase),
		"AGENTCTL_INSTANCE_PORT_MAX=" + strconv.Itoa(instancePortBase+2),
		"AGENTCTL_PROTOCOL=acp",
		"AGENTCTL_AGENT_COMMAND=" + mockAgent,
		"AGENTCTL_WORKDIR=" + root,
		"AGENTCTL_LOG_LEVEL=error",
		"AGENTCTL_LOG_FORMAT=json",
		"KANDEV_DEBUG_AGENT_MESSAGES=false",
		"KANDEV_ACP_IDLE_TIMEOUT=1h",
		"KANDEV_ACP_IDLE_REAPER_INTERVAL=1m",
	}
	if authenticated {
		environment = append(environment, "AGENTCTL_BOOTSTRAP_NONCE="+agentctlAuthTestNonce)
	}
	if runtime.GOOS == "windows" {
		for _, key := range []string{"SYSTEMROOT", "WINDIR", "TEMP", "TMP"} {
			if value := os.Getenv(key); value != "" {
				environment = append(environment, key+"="+value)
			}
		}
	}
	return environment
}

func waitForAuthenticationModesServer(t *testing.T, server *authenticationModesServer) {
	t.Helper()
	deadline := time.NewTimer(20 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		err := server.control.Health(ctx)
		cancel()
		if err == nil {
			return
		}
		select {
		case <-server.waitDone:
			t.Fatalf("agentctl subprocess exited before becoming healthy: %v\n%s", server.waitErr, server.output())
		case <-deadline.C:
			t.Fatalf("agentctl subprocess did not become healthy: %v\n%s", err, server.output())
		case <-ticker.C:
		}
	}
}

func waitForAuthenticationModesOutput(server *authenticationModesServer, expected string, timeout time.Duration) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if strings.Contains(server.output(), expected) {
			return nil
		}
		select {
		case <-server.waitDone:
			return fmt.Errorf("helper exited before readiness marker %q: %v\n%s", expected, server.waitErr, server.output())
		case <-deadline.C:
			return fmt.Errorf("helper did not print readiness marker %q\n%s", expected, server.output())
		case <-ticker.C:
		}
	}
}

func (s *authenticationModesServer) output() string {
	return strings.TrimSpace(s.stdout.String() + "\n" + s.stderr.String())
}

func (s *authenticationModesServer) close(t *testing.T) error {
	t.Helper()
	for _, client := range s.clients {
		client.Close()
	}
	if s.control != nil {
		for _, instanceID := range s.instances {
			if err := s.control.DeleteInstance(context.Background(), instanceID); err != nil {
				t.Errorf("delete test instance %s: %v", instanceID, err)
			}
		}
		s.control.Close()
	}
	if s.command == nil || s.command.Process == nil {
		return nil
	}
	select {
	case <-s.waitDone:
		return s.exitFailure("before cleanup")
	default:
	}
	forcedShutdown := false
	if err := s.command.Process.Signal(os.Interrupt); err != nil {
		if killErr := s.command.Process.Kill(); runtime.GOOS == "windows" && killErr == nil {
			forcedShutdown = true
		}
	}
	select {
	case <-s.waitDone:
		if forcedShutdown {
			return nil
		}
		return s.exitFailure("during cleanup")
	case <-time.After(15 * time.Second):
		_ = s.command.Process.Kill()
		<-s.waitDone
		return fmt.Errorf("agentctl subprocess failed to stop after interrupt\n%s", s.output())
	}
}

func (s *authenticationModesServer) exitFailure(stage string) error {
	if s.waitErr == nil {
		return nil
	}
	return fmt.Errorf("agentctl subprocess exited %s: %w\n%s", stage, s.waitErr, s.output())
}

func buildAuthenticationModesMockAgent(t *testing.T) string {
	t.Helper()
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve agentctl package directory: %v", err)
	}
	backendRoot := filepath.Clean(filepath.Join(workingDirectory, "../.."))
	name := "mock-agent"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(t.TempDir(), name)
	command := exec.Command("go", "build", "-trimpath", "-o", binary, "./cmd/mock-agent")
	command.Dir = backendRoot
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build mock-agent fixture: %v\n%s", err, output)
	}
	return binary
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate free TCP port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("release allocated TCP port: %v", err)
	}
	return port
}

func freeTCPPortRange(t *testing.T, size int) int {
	t.Helper()
	for attempt := 0; attempt < 100; attempt++ {
		base := freeTCPPort(t)
		if base+size >= 65536 {
			continue
		}
		listeners := make([]net.Listener, 0, size)
		available := true
		for offset := 0; offset < size; offset++ {
			listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(base+offset)))
			if err != nil {
				available = false
				break
			}
			listeners = append(listeners, listener)
		}
		for _, listener := range listeners {
			_ = listener.Close()
		}
		if available {
			return base
		}
	}
	t.Fatal("could not allocate a free contiguous TCP port range")
	return 0
}
