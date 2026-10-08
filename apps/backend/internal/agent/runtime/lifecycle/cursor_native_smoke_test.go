package lifecycle

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/stretchr/testify/require"
)

const (
	cursorNativeSmokeOptIn  = "KANDEV_CURSOR_NATIVE_MCP_SMOKE"
	cursorNativeSmokeBin    = "KANDEV_CURSOR_AGENT_BIN"
	cursorNativeSmokeID     = "plugin-kandevfixture-fixture"
	cursorNativeSmokeKey    = "synthetic-fixture-access"
	cursorNativeSmokeMarker = "isolated-fixture-success"
)

func TestCursorNativeLifecyclePreparationAgainstAuthenticatedFixture(t *testing.T) {
	binary, accountToken := requireCursorNativeSmokeGate(t)
	smoke := newCursorNativeSmokeContext(t, binary, accountToken)
	writeCursorNativeSmokeCredentials(t, smoke.home)
	smoke.fixture.setAuthorized(true)

	recorder := newPrepareProgressRecorder(nil)
	require.NoError(t, smoke.prepare(recorder))
	requireCursorNativeSmokePreparation(t, recorder.Steps())
	requireCursorNativeSmokeConfig(t, smoke.workspace, smoke.home)

	toolCallsBeforeTerminal := cursorNativeSmokeToolCallCount(smoke.fixture.snapshot())
	terminalOutput := runCursorNativeSmokeTerminal(t, smoke.binary, smoke.workspace, smoke.env)
	require.Contains(t, terminalOutput, cursorNativeSmokeMarker)
	toolCallsAfterTerminal := cursorNativeSmokeToolCallCount(smoke.fixture.snapshot())
	require.Greater(t, toolCallsAfterTerminal, toolCallsBeforeTerminal,
		"the Cursor TUI prompt should call the local fixture")
	removeCursorNativeSmokeFixturePermissionGrant(t, smoke.workspace)
	acpOutput, permissionSeen := runCursorNativeSmokeACP(t, smoke.binary, smoke.workspace, smoke.env)
	require.Contains(t, acpOutput, cursorNativeSmokeMarker)
	require.True(t, permissionSeen, "ACP should request and receive allow_once for the fixture tool")
	require.Greater(t, cursorNativeSmokeToolCallCount(smoke.fixture.snapshot()), toolCallsAfterTerminal,
		"the Cursor ACP prompt should call the local fixture")
	requireCursorNativeSmokeRequests(t, smoke.fixture.snapshot(), false)
}

func TestCursorNativeLifecycleAuthenticationRecoveryLoadsSavedACPConversationInReplacementProcess(t *testing.T) {
	binary, accountToken := requireCursorNativeSmokeGate(t)
	smoke := newCursorNativeSmokeContext(t, binary, accountToken)

	firstRecorder := newPrepareProgressRecorder(nil)
	require.NoError(t, smoke.prepare(firstRecorder))
	smoke.runner.logListToolsDiagnostics(t)
	t.Logf("synthetic OAuth fixture request paths: %v", smoke.fixture.snapshotPaths())
	requireCursorNativeSmokeAuthFailure(t, firstRecorder.Steps())

	acp := startCursorNativeSmokeACP(t, smoke.binary, smoke.workspace, smoke.env)
	acp.primeSession(t)
	writeCursorNativeSmokeTaskCredentials(t, smoke.home, smoke.workspace)
	removeCursorNativeSmokeFixturePermissionGrant(t, smoke.workspace)
	smoke.fixture.setAuthorized(true)
	savedSessionID := acp.sessionID
	acp.stop()

	retryRecorder := newPrepareProgressRecorder(nil)
	require.NoError(t, smoke.prepare(retryRecorder))
	requireCursorNativeSmokePreparation(t, retryRecorder.Steps())

	toolCallsBeforeReload := cursorNativeSmokeToolCallCount(smoke.fixture.snapshot())
	reloadedACP := startCursorNativeSmokeACPWithoutSession(t, smoke.binary, smoke.workspace, smoke.env)
	loadedHistory, acpOutput, permissionSeen := reloadedACP.loadExistingSessionAndPrompt(t, savedSessionID)
	require.Equal(t, savedSessionID, reloadedACP.loadedSessionID, "session/load must reuse the saved ACP session ID")
	require.Contains(t, strings.ToLower(loadedHistory), "ready", "session/load should preserve the initial conversation")
	require.Contains(t, acpOutput, cursorNativeSmokeMarker)
	require.True(t, permissionSeen, "reloaded ACP session should request allow_once for the fixture tool")
	require.Greater(t, cursorNativeSmokeToolCallCount(smoke.fixture.snapshot()), toolCallsBeforeReload,
		"the replacement ACP process should call the local fixture after session/load")
	requireCursorNativeSmokeRequests(t, smoke.fixture.snapshot(), true)
}

func requireCursorNativeSmokeGate(t *testing.T) (string, string) {
	t.Helper()
	if runtime.GOOS != "darwin" {
		t.Skip("native Cursor MCP smoke runs only on macOS")
	}
	if os.Getenv(cursorNativeSmokeOptIn) != "1" {
		t.Skip("set " + cursorNativeSmokeOptIn + "=1 to run the native Cursor MCP smoke")
	}
	return cursorNativeSmokeExecutable(t), readCursorNativeSmokeAccountToken(t)
}

type cursorNativeSmokeContext struct {
	binary    string
	home      string
	workspace string
	env       map[string]string
	fixture   *cursorNativeSmokeFixture
	runner    *cursorNativeSmokeCapturingRunner
	manager   *Manager
	execution *AgentExecution
	agent     agents.Agent
	profile   *AgentProfileInfo
}

func newCursorNativeSmokeContext(t *testing.T, binary, accountToken string) *cursorNativeSmokeContext {
	t.Helper()

	home := t.TempDir()
	t.Cleanup(func() { requireNoCursorNativeSmokeAccountTokenOnDisk(t, home, accountToken) })
	t.Setenv("HOME", home)
	workspace := t.TempDir()
	fixture := newCursorNativeSmokeFixture(t)
	writeCursorNativeSmokeInputs(t, home, workspace, fixture.server.URL)
	env := cursorNativeSmokeEnvironment(home, binary, accountToken)

	manager := newTestManager(t)
	runner := &cursorNativeSmokeCapturingRunner{
		delegate: mcpconfig.ExecNativeMCPCommandRunner{},
		secrets:  []string{accountToken, cursorNativeSmokeKey},
	}
	manager.SetCursorNativeMCPCommandRunner(runner)
	manager.cursorInventoryLoader = func(context.Context) (mcpconfig.CursorNativeInventory, error) {
		return mcpconfig.CursorNativeInventory{}, nil
	}
	execution := &AgentExecution{
		ID: "native-smoke-execution", TaskID: "native-smoke-task", SessionID: "native-smoke-session",
		AgentProfileID: "native-smoke-profile", ExecutorType: string(executor.NameLocal),
		WorkspacePath: workspace, standalonePort: 1234,
		metadata: map[string]interface{}{MetadataKeyRepositoryConfigured: false},
	}
	execution.setMetadataValue("executor_mcp_policy", mcpconfig.Policy{AllowHTTP: true})
	execution.setRuntimeEnvironment(env)
	profile := &AgentProfileInfo{
		CursorMCPAuthEnabled: true, CursorPluginsMCPEnabled: true,
		MCPSelectionMode: "selected", MCPSelectedServers: []string{cursorNativeSmokeID},
	}
	cursor := agents.NewCursorACP()
	runtimeConfig := cursor.Runtime()
	runtimeConfig.Cmd = agents.NewCommand(binary, "acp")
	agent := cursorMCPRuntimeOverrideAgent{Agent: cursor, runtime: runtimeConfig}
	return &cursorNativeSmokeContext{
		binary: binary, home: home, workspace: workspace, env: env,
		fixture: fixture, runner: runner, manager: manager, execution: execution,
		agent: agent, profile: profile,
	}
}

func (s *cursorNativeSmokeContext) prepare(recorder *prepareProgressRecorder) error {
	return s.manager.materializeRuntimeProjectMCPWithPreparation(
		context.Background(), s.execution, s.agent, s.profile, string(executor.NameLocal), recorder,
	)
}

type cursorNativeSmokeCapturingRunner struct {
	delegate mcpconfig.NativeMCPCommandRunner
	secrets  []string
	results  []mcpconfig.NativeMCPCommandResult
}

func (r *cursorNativeSmokeCapturingRunner) Run(
	ctx context.Context,
	executable string,
	args []string,
	workspace string,
	env map[string]string,
) (mcpconfig.NativeMCPCommandResult, error) {
	result, err := r.delegate.Run(ctx, executable, args, workspace, env)
	if len(args) >= 2 && args[0] == "mcp" && args[1] == "list-tools" {
		result.Stdout = append([]byte(nil), result.Stdout...)
		result.Stderr = append([]byte(nil), result.Stderr...)
		r.results = append(r.results, result)
	}
	return result, err
}

func (r *cursorNativeSmokeCapturingRunner) logListToolsDiagnostics(t *testing.T) {
	t.Helper()
	for i, result := range r.results {
		stdout := cursorNativeSmokeRedactDiagnostic(result.Stdout, r.secrets)
		stderr := cursorNativeSmokeRedactDiagnostic(result.Stderr, r.secrets)
		require.NotContains(t, stdout, cursorNativeSmokeKey)
		require.NotContains(t, stderr, cursorNativeSmokeKey)
		t.Logf("native list-tools diagnostic %d (redacted, bounded): exit=%d stdout=%q stderr=%q", i+1, result.ExitCode, stdout, stderr)
	}
}

func cursorNativeSmokeRedactDiagnostic(data []byte, secrets []string) string {
	const diagnosticLimit = 4096
	value := string(data)
	for _, secret := range secrets {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, "[REDACTED]")
		}
	}
	if len(value) > diagnosticLimit {
		value = value[:diagnosticLimit]
	}
	return value
}

func readCursorNativeSmokeAccountToken(t *testing.T) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "/usr/bin/security", "find-generic-password", "-s", "cursor-access-token", "-a", "cursor-user", "-w")
	data, err := command.Output()
	require.NoError(t, err, "existing Cursor account access token must be available in the login keychain")
	token := strings.TrimSpace(string(data))
	require.NotEmpty(t, token, "existing Cursor account access token must not be empty")
	return token
}

func cursorNativeSmokeExecutable(t *testing.T) string {
	t.Helper()
	binary := strings.TrimSpace(os.Getenv(cursorNativeSmokeBin))
	if binary == "" {
		var err error
		binary, err = exec.LookPath("cursor-agent")
		require.NoError(t, err, "cursor-agent must be on PATH or provided with %s", cursorNativeSmokeBin)
	}
	absolute, err := filepath.Abs(binary)
	require.NoError(t, err)
	info, err := os.Stat(absolute)
	require.NoError(t, err)
	require.True(t, info.Mode().IsRegular(), "Cursor binary must be a regular file")
	require.NotZero(t, info.Mode().Perm()&0o111, "Cursor binary must be executable")
	return absolute
}

func cursorNativeSmokeEnvironment(home, binary, accountToken string) map[string]string {
	path := filepath.Dir(binary)
	if current := os.Getenv("PATH"); current != "" {
		path += string(os.PathListSeparator) + current
	}
	return map[string]string{
		"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, ".config"),
		"CURSOR_CONFIG_DIR": filepath.Join(home, ".cursor"), "PATH": path,
		"AGENT_CLI_CREDENTIAL_STORE": "memory", "CURSOR_AUTH_TOKEN": accountToken, "CURSOR_API_KEY": "",
		"CURSOR_ACCESS_TOKEN": "", "CURSOR_SESSION_TOKEN": "",
		"HTTP_PROXY": "", "HTTPS_PROXY": "", "ALL_PROXY": "", "http_proxy": "", "https_proxy": "", "all_proxy": "",
		"NO_PROXY": "127.0.0.1,localhost", "no_proxy": "127.0.0.1,localhost",
	}
}

func writeCursorNativeSmokeInputs(t *testing.T, home, workspace, fixtureURL string) {
	t.Helper()
	cursorHome := filepath.Join(home, ".cursor")
	pluginRoot := filepath.Join(cursorHome, "plugins", "local", "kandevfixture")
	writeCursorNativeSmokeJSON(t, filepath.Join(pluginRoot, "mcp.json"), map[string]any{
		"mcpServers": map[string]any{"fixture": map[string]string{"url": fixtureURL + "/mcp"}},
	})
	writeCursorNativeSmokeJSON(t, filepath.Join(workspace, ".cursor", "cli.json"), map[string]any{
		"permissions": map[string]any{"allow": []string{"Mcp(" + cursorNativeSmokeID + ":fixture_ping)"}, "deny": []string{}},
	})
}

func writeCursorNativeSmokeCredentials(t *testing.T, home string) {
	t.Helper()
	project := filepath.Join(home, ".cursor", "projects", "fixture-credentials")
	writeCursorNativeSmokeJSON(t, filepath.Join(project, "mcp-auth.json"), cursorNativeSmokeCredential())
}

func writeCursorNativeSmokeTaskCredentials(t *testing.T, home, workspace string) {
	t.Helper()
	destination := cursorMCPAuthDestinationForTest(t, filepath.Join(home, ".cursor", "projects"), workspace)
	if err := os.Remove(destination); err != nil && !errors.Is(err, os.ErrNotExist) {
		require.NoError(t, err)
	}
	writeCursorNativeSmokeJSON(t, destination, cursorNativeSmokeCredential())
}

func removeCursorNativeSmokeFixturePermissionGrant(t *testing.T, workspace string) {
	t.Helper()
	writeCursorNativeSmokeJSON(t, filepath.Join(workspace, ".cursor", "cli.json"), map[string]any{
		"permissions": map[string]any{"allow": []string{}, "deny": []string{}},
	})
}

func cursorNativeSmokeCredential() map[string]any {
	return map[string]any{
		cursorNativeSmokeID: map[string]any{
			"tokens":     map[string]string{"access_token": cursorNativeSmokeKey, "token_type": "Bearer"},
			"clientInfo": map[string]string{"client_id": "synthetic-fixture-client"},
		},
	}
}

func writeCursorNativeSmokeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, data, 0o600))
}

func requireCursorNativeSmokePreparation(t *testing.T, steps []PrepareStep) {
	t.Helper()
	for _, expected := range []struct {
		kind, serverID string
	}{
		{PrepareStepKindAgentMCPDiscovery, cursorNativeSmokeID},
		{PrepareStepKindAgentMCPSelection, cursorNativeSmokeID},
		{PrepareStepKindAgentMCPCredentials, ""},
		{PrepareStepKindAgentMCPApproval, cursorNativeSmokeID},
		{PrepareStepKindAgentMCPVerification, cursorNativeSmokeID},
	} {
		step, ok := findCursorNativeSmokeStep(steps, expected.kind, expected.serverID)
		require.Truef(t, ok, "preparation step %s for %q was not recorded", expected.kind, expected.serverID)
		require.Equal(t, PrepareStepCompleted, step.Status, "%s for %q: %s", step.Kind, step.MCPServerID, step.FailureCode)
	}
}

func requireCursorNativeSmokeAuthFailure(t *testing.T, steps []PrepareStep) {
	t.Helper()
	step, ok := findCursorNativeSmokeStep(steps, PrepareStepKindAgentMCPVerification, cursorNativeSmokeID)
	require.True(t, ok, "verification step was not recorded")
	require.Equal(t, PrepareStepFailed, step.Status)
	require.Equal(t, "authentication_required", step.FailureCode)
	require.Empty(t, step.Command)
	require.Empty(t, step.Output)
	require.Empty(t, step.Error)
}

func findCursorNativeSmokeStep(steps []PrepareStep, kind, serverID string) (PrepareStep, bool) {
	for _, step := range steps {
		if step.Kind == kind && step.MCPServerID == serverID {
			return step, true
		}
	}
	return PrepareStep{}, false
}

func requireCursorNativeSmokeConfig(t *testing.T, workspace, home string) {
	t.Helper()
	projectConfig := readCursorProjectMCPForTest(t, filepath.Join(workspace, ".cursor", "mcp.json"))
	require.Contains(t, projectConfig, cursorNativeSmokeID)
	require.Contains(t, string(projectConfig[cursorNativeSmokeID]), "127.0.0.1")
	credentialLink := cursorMCPAuthDestinationForTest(t, filepath.Join(home, ".cursor", "projects"), workspace)
	info, err := os.Lstat(credentialLink)
	require.NoError(t, err)
	require.True(t, info.Mode()&os.ModeSymlink != 0, "task credentials must use the lifecycle bridge link")
	credentialData, err := os.ReadFile(credentialLink)
	require.NoError(t, err)
	require.Contains(t, string(credentialData), cursorNativeSmokeKey)
	permissionData, err := os.ReadFile(filepath.Join(workspace, ".cursor", "cli.json"))
	require.NoError(t, err)
	var permissions struct {
		Permissions struct {
			Allow []string `json:"allow"`
			Deny  []string `json:"deny"`
		} `json:"permissions"`
	}
	require.NoError(t, json.Unmarshal(permissionData, &permissions))
	require.Equal(t, []string{"Mcp(" + cursorNativeSmokeID + ":fixture_ping)"}, permissions.Permissions.Allow)
	require.Empty(t, permissions.Permissions.Deny)
}

func requireCursorNativeSmokeRequests(t *testing.T, requests []cursorNativeSmokeRequest, expectUnauthorized bool) {
	t.Helper()
	require.NotEmpty(t, requests, "native list-tools must reach the loopback MCP fixture")
	methods := make(map[string]bool)
	unauthorized := false
	for _, request := range requests {
		require.Equal(t, "/mcp", request.path)
		methods[request.method] = true
		unauthorized = unauthorized || !request.authorized
	}
	if expectUnauthorized {
		require.True(t, unauthorized, "the initial preparation should capture missing fixture credentials")
	} else {
		require.False(t, unauthorized, "the authenticated preparation should not make an unauthenticated request")
	}
	require.True(t, requests[len(requests)-1].authorized, "the credential refresh must reach the fixture")
	require.True(t, methods["tools/list"], "native verification did not list fixture tools")
	require.True(t, methods["tools/call"], "the native prompt must call the fixture tool")
}

func cursorNativeSmokeToolCallCount(requests []cursorNativeSmokeRequest) int {
	count := 0
	for _, request := range requests {
		if request.method == "tools/call" && request.authorized {
			count++
		}
	}
	return count
}

func requireNoCursorNativeSmokeAccountTokenOnDisk(t *testing.T, home, accountToken string) {
	t.Helper()
	err := filepath.WalkDir(home, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if bytes.Contains(data, []byte(accountToken)) {
			return errors.New("account token persisted in isolated Cursor HOME")
		}
		return nil
	})
	require.NoError(t, err, "Cursor account token must remain in memory only")
}

func runCursorNativeSmokeTerminal(t *testing.T, binary, workspace string, env map[string]string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, "--trust", "-p", "--output-format", "json",
		"Call only fixture_ping from plugin-kandevfixture-fixture once and report its exact success marker. Do not use other tools.")
	command.Dir = workspace
	command.Env = cursorNativeSmokeEnvList(env)
	command.WaitDelay = 2 * time.Second
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		t.Fatalf("Cursor terminal fixture prompt failed: %v", err)
	}
	return stdout.String()
}

func runCursorNativeSmokeACP(t *testing.T, binary, workspace string, env map[string]string) (string, bool) {
	t.Helper()
	acp := startCursorNativeSmokeACP(t, binary, workspace, env)
	return acp.prompt(t)
}

type cursorNativeSmokeACPProcess struct {
	command         *exec.Cmd
	cancel          context.CancelFunc
	stdin           io.WriteCloser
	client          *cursorNativeSmokeACPClient
	workspace       string
	sessionID       string
	loadedSessionID string
	stopped         bool
}

func startCursorNativeSmokeACP(t *testing.T, binary, workspace string, env map[string]string) *cursorNativeSmokeACPProcess {
	t.Helper()
	process := startCursorNativeSmokeACPWithoutSession(t, binary, workspace, env)
	newSession := process.client.request(t, 2, "session/new", map[string]any{"cwd": workspace, "mcpServers": []any{}})
	var session struct {
		SessionID string `json:"sessionId"`
	}
	require.NoError(t, json.Unmarshal(newSession, &session))
	require.NotEmpty(t, session.SessionID)
	process.sessionID = session.SessionID
	return process
}

func startCursorNativeSmokeACPWithoutSession(t *testing.T, binary, workspace string, env map[string]string) *cursorNativeSmokeACPProcess {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	command := exec.CommandContext(ctx, binary, "--trust", "acp")
	command.Dir = workspace
	command.Env = cursorNativeSmokeEnvList(env)
	command.WaitDelay = 2 * time.Second
	stdin, err := command.StdinPipe()
	require.NoError(t, err)
	stdout, err := command.StdoutPipe()
	require.NoError(t, err)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	require.NoError(t, command.Start())
	client := &cursorNativeSmokeACPClient{input: stdin, decoder: json.NewDecoder(bufio.NewReader(stdout))}
	process := &cursorNativeSmokeACPProcess{
		command: command, cancel: cancel, stdin: stdin, client: client, workspace: workspace,
	}
	t.Cleanup(process.stop)
	initializeResult := client.request(t, 1, "initialize", map[string]any{
		"protocolVersion": 1, "clientCapabilities": map[string]any{},
		"clientInfo": map[string]string{"name": "isolated-cursor-mcp-smoke", "version": "1"},
	})
	client.initializeResult = initializeResult
	return process
}

func (p *cursorNativeSmokeACPProcess) loadExistingSessionAndPrompt(t *testing.T, sessionID string) (string, string, bool) {
	t.Helper()
	p.client.request(t, 2, "session/load", map[string]any{
		"sessionId":  sessionID,
		"cwd":        p.workspace,
		"mcpServers": []any{},
	})
	loadedHistory := p.client.output.String()
	p.sessionID = sessionID
	p.loadedSessionID = sessionID
	p.client.output.Reset()
	output, permissionSeen := p.promptWithRequestID(t, 3, "Call only fixture_ping from plugin-kandevfixture-fixture once and report its exact success marker. Do not use other tools.")
	return loadedHistory, output, permissionSeen
}

func (p *cursorNativeSmokeACPProcess) prompt(t *testing.T) (string, bool) {
	t.Helper()
	return p.promptWithRequestID(t, 3, "Call only fixture_ping from plugin-kandevfixture-fixture once and report its exact success marker. Do not use other tools.")
}

func (p *cursorNativeSmokeACPProcess) primeSession(t *testing.T) {
	t.Helper()
	output, _ := p.promptWithRequestID(t, 3, "Reply only with READY. Do not use tools.")
	require.Contains(t, strings.ToLower(output), "ready", "initial prompt should persist the ACP session")
	p.client.output.Reset()
}

func (p *cursorNativeSmokeACPProcess) promptWithRequestID(t *testing.T, requestID int, prompt string) (string, bool) {
	t.Helper()
	p.client.request(t, requestID, "session/prompt", map[string]any{
		"sessionId": p.sessionID,
		"prompt":    []map[string]string{{"type": "text", "text": prompt}},
	})
	return p.client.output.String(), p.client.fixturePermissionAllowed
}

func (p *cursorNativeSmokeACPProcess) stop() {
	if p == nil {
		return
	}
	if p.stopped {
		return
	}
	p.stopped = true
	if p.stdin != nil {
		_ = p.stdin.Close()
	}
	if p.command != nil && p.command.Process != nil {
		_ = p.command.Process.Kill()
		_ = p.command.Wait()
	}
	if p.cancel != nil {
		p.cancel()
	}
}

func cursorNativeSmokeEnvList(overrides map[string]string) []string {
	values := make(map[string]string)
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			values[key] = value
		}
	}
	for key, value := range overrides {
		values[key] = value
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	env := make([]string, 0, len(keys))
	for _, key := range keys {
		env = append(env, key+"="+values[key])
	}
	return env
}

type cursorNativeSmokeRequest struct {
	method     string
	path       string
	authorized bool
}

type cursorNativeSmokeFixture struct {
	server       *httptest.Server
	mu           sync.Mutex
	authorized   bool
	requests     []cursorNativeSmokeRequest
	requestPaths []string
}

func newCursorNativeSmokeFixture(t *testing.T) *cursorNativeSmokeFixture {
	t.Helper()
	fixture := &cursorNativeSmokeFixture{}
	fixture.server = httptest.NewServer(http.HandlerFunc(fixture.serveHTTP))
	t.Cleanup(fixture.server.Close)
	return fixture
}

func (f *cursorNativeSmokeFixture) setAuthorized(authorized bool) {
	f.mu.Lock()
	f.authorized = authorized
	f.mu.Unlock()
}

func (f *cursorNativeSmokeFixture) serveHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requestPaths = append(f.requestPaths, r.Method+" "+r.URL.Path)
	f.mu.Unlock()
	if r.Method == http.MethodGet && r.URL.Path == "/.well-known/oauth-protected-resource/mcp" {
		f.writeJSON(w, http.StatusOK, map[string]any{
			"resource":                 f.server.URL + "/mcp",
			"authorization_servers":    []string{f.server.URL},
			"scopes_supported":         []string{"fixture"},
			"bearer_methods_supported": []string{"header"},
		})
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == "/.well-known/oauth-authorization-server" {
		f.writeJSON(w, http.StatusOK, map[string]any{
			"issuer":                                f.server.URL,
			"authorization_endpoint":                f.server.URL + "/authorize",
			"token_endpoint":                        f.server.URL + "/token",
			"registration_endpoint":                 f.server.URL + "/register",
			"response_types_supported":              []string{"code"},
			"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
			"code_challenge_methods_supported":      []string{"S256"},
			"token_endpoint_auth_methods_supported": []string{"none"},
			"scopes_supported":                      []string{"fixture"},
		})
		return
	}
	if r.Method == http.MethodPost && r.URL.Path == "/register" {
		var request struct {
			RedirectURIs []string `json:"redirect_uris"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&request); err != nil {
			http.Error(w, "invalid synthetic client registration", http.StatusBadRequest)
			return
		}
		f.writeJSON(w, http.StatusCreated, map[string]any{
			"client_id":                  "isolated-fixture-client",
			"client_name":                "isolated fixture client",
			"redirect_uris":              request.RedirectURIs,
			"grant_types":                []string{"authorization_code", "refresh_token"},
			"response_types":             []string{"code"},
			"token_endpoint_auth_method": "none",
		})
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == "/authorize" {
		f.writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "login_required", "error_description": "synthetic fixture requires a credential",
		})
		return
	}
	if r.Method == http.MethodPost && r.URL.Path == "/token" {
		f.writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "invalid_grant", "error_description": "synthetic fixture does not issue external tokens",
		})
		return
	}
	if r.Method == http.MethodDelete {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == "/mcp" {
		f.mu.Lock()
		authorized := f.authorized && r.Header.Get("Authorization") == "Bearer "+cursorNativeSmokeKey
		f.requests = append(f.requests, cursorNativeSmokeRequest{method: http.MethodGet, path: r.URL.Path, authorized: authorized})
		f.mu.Unlock()
		if !authorized {
			metadataURL := f.server.URL + "/.well-known/oauth-protected-resource/mcp"
			w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+metadataURL+`", error="invalid_token"`)
			http.Error(w, "fixture authentication required", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if r.Method != http.MethodPost || r.URL.Path != "/mcp" {
		http.NotFound(w, r)
		return
	}
	var request struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Method  string          `json:"method"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&request); err != nil {
		http.Error(w, "invalid fixture request", http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	authorized := f.authorized && r.Header.Get("Authorization") == "Bearer "+cursorNativeSmokeKey
	f.requests = append(f.requests, cursorNativeSmokeRequest{method: request.Method, path: r.URL.Path, authorized: authorized})
	f.mu.Unlock()
	if !authorized {
		http.Error(w, "fixture authentication required", http.StatusUnauthorized)
		return
	}
	if len(request.ID) == 0 || strings.HasPrefix(request.Method, "notifications/") {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	result := cursorNativeSmokeResult(request.Method)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
}

func (f *cursorNativeSmokeFixture) writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func cursorNativeSmokeResult(method string) any {
	switch method {
	case "initialize":
		return map[string]any{
			"protocolVersion": "2024-11-05", "capabilities": map[string]any{"tools": map[string]any{}},
			"serverInfo": map[string]string{"name": "isolated-cursor-mcp-fixture", "version": "1"},
		}
	case "tools/list":
		return map[string]any{"tools": []map[string]any{{
			"name": "fixture_ping", "description": "Read-only isolated smoke fixture.",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
			"annotations": map[string]any{"readOnlyHint": true, "destructiveHint": false},
		}}}
	case "tools/call":
		return map[string]any{"content": []map[string]string{{"type": "text", "text": cursorNativeSmokeMarker}}, "isError": false}
	case "resources/list":
		return map[string]any{"resources": []any{}}
	case "prompts/list":
		return map[string]any{"prompts": []any{}}
	default:
		return map[string]any{}
	}
}

func (f *cursorNativeSmokeFixture) snapshot() []cursorNativeSmokeRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]cursorNativeSmokeRequest(nil), f.requests...)
}

func (f *cursorNativeSmokeFixture) snapshotPaths() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.requestPaths...)
}
