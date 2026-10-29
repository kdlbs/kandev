//go:build copilot_native

package copilotplugins_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/agent/acpdbg"
)

type nativeToolCall struct {
	name      string
	kind      string
	status    string
	rawInput  any
	rawOutput any
	content   any
}

type nativeACPDriver struct {
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	stdout     io.ReadCloser
	stderr     io.ReadCloser
	framer     *acpdbg.Framer
	recorder   *acpdbg.Recorder
	frames     *nativeACPFrameQueue
	readWG     sync.WaitGroup
	stderrWG   sync.WaitGroup
	stderrMu   sync.Mutex
	captureMu  sync.Mutex
	stderrLog  []string
	captureErr error
	toolCalls  map[string]*nativeToolCall

	knownServers   []string
	allowedServers map[string]bool
	approved       map[string]int
	rejected       map[string]int
	unexpected     []string
	sentinels      *nativeTestSentinels
	closeOnce      sync.Once
}

type nativeSentinelTrafficSummary struct {
	InitializeObserved      bool
	ToolsListObserved       bool
	ToolCallObserved        bool
	ToolCount               int
	ToolCallRequestCount    int
	ToolCallResultSentCount int
}

type nativeTestSentinel interface {
	URL() string
	Close()
	Summary() acpdbg.MCPSentinelSummary
}

type nativeACPFrameQueue struct {
	mu     sync.Mutex
	frames []acpdbg.Frame
	err    error
	closed bool
	wake   chan struct{}
}

type nativeTestSentinels struct {
	servers map[string]nativeTestSentinel
}

func newNativeACPFrameQueue() *nativeACPFrameQueue {
	return &nativeACPFrameQueue{wake: make(chan struct{}, 1)}
}

func (q *nativeACPFrameQueue) push(frame acpdbg.Frame) {
	q.mu.Lock()
	if !q.closed {
		q.frames = append(q.frames, frame)
	}
	q.mu.Unlock()
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

func (q *nativeACPFrameQueue) finish(err error) {
	q.mu.Lock()
	q.closed = true
	q.err = err
	q.mu.Unlock()
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

func (q *nativeACPFrameQueue) next(ctx context.Context) (acpdbg.Frame, error) {
	for {
		q.mu.Lock()
		if len(q.frames) > 0 {
			frame := q.frames[0]
			q.frames[0] = nil
			q.frames = q.frames[1:]
			if len(q.frames) == 0 {
				q.frames = nil
			}
			q.mu.Unlock()
			return frame, nil
		}
		if q.closed {
			err := q.err
			q.mu.Unlock()
			if err == nil {
				err = io.EOF
			}
			return nil, err
		}
		q.mu.Unlock()

		select {
		case <-q.wake:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func newNativeTestSentinels(recorder *acpdbg.Recorder) *nativeTestSentinels {
	return &nativeTestSentinels{servers: map[string]nativeTestSentinel{
		nativeTargetServer:  newNativeMCPSSESentinel(recorder),
		nativeSiblingServer: newNativeMCPSSESentinel(recorder),
		nativeConfigServer:  newNativeMCPSSESentinel(recorder),
		nativeACPServer:     newNativeMCPSSESentinel(recorder),
	}}
}

func (s *nativeTestSentinels) close() {
	for _, server := range s.servers {
		server.Close()
	}
}

func (s *nativeTestSentinels) summary(server string) nativeSentinelTrafficSummary {
	sentinel := s.servers[server]
	if detailed, ok := sentinel.(interface {
		TrafficSummary() nativeSentinelTrafficSummary
	}); ok {
		return detailed.TrafficSummary()
	}
	summary := sentinel.Summary()
	return nativeSentinelTrafficSummary{
		InitializeObserved: summary.InitializeObserved,
		ToolsListObserved:  summary.ToolsListObserved,
		ToolCallObserved:   summary.ToolCallObserved,
		ToolCount:          summary.ToolCount,
	}
}

func (s *nativeTestSentinels) httpURL(server string) string {
	sentinel, ok := s.servers[server].(interface{ HTTPURL() string })
	if !ok {
		return ""
	}
	return sentinel.HTTPURL()
}

func (s *nativeTestSentinels) allowsReadOnlyCall(server, toolName string, input any) bool {
	if !nativeToolNameMatchesServer(toolName, server) {
		return false
	}
	sentinel, ok := s.servers[server].(interface {
		allowsReadOnlyCall(string, any) bool
	})
	return ok && sentinel.allowsReadOnlyCall(nativeSentinelToolName, input)
}

func captureNativeCaseEvidence(
	result *nativeACPCaseResult,
	driver *nativeACPDriver,
	sentinels *nativeTestSentinels,
	servers []string,
	deniedServer string,
	managed bool,
) {
	result.traffic = make(map[string]nativeSentinelTrafficSummary, len(servers))
	result.toolCalls = make(map[string]int, len(servers))
	result.successfulCalls = make(map[string]bool, len(servers))
	for _, server := range servers {
		traffic := sentinels.summary(server)
		result.traffic[server] = traffic
		result.toolCalls[server] = driver.callCount(server)
		result.successfulCalls[server] = traffic.ToolCallRequestCount == 1 &&
			traffic.ToolCallObserved &&
			traffic.ToolCallResultSentCount == 1 &&
			(driver.callCount(server) == 0 || driver.hasSuccessfulSentinelCall(server))
	}
	if deniedServer != "" {
		result.nativeDenialObserved = driver.hasNativeDenial(deniedServer, managed)
		result.harnessRejected = driver.wasHarnessRejected(deniedServer)
	}
	result.unexpectedToolUpdates = len(driver.unexpected)
	result.captureFailed = driver.captureFailed()
}

func nativeACPCaseEvidenceSummary(result nativeACPCaseResult, servers []string) string {
	parts := make([]string, 0, len(servers)+1)
	for _, server := range servers {
		traffic := result.traffic[server]
		parts = append(parts, fmt.Sprintf(
			"%s{initialize=%t,tools_list=%t,endpoint_tool_requests=%d,endpoint_tool_call=%t,result_sent=%d,acp_tool_calls=%d,success=%t}",
			server,
			traffic.InitializeObserved,
			traffic.ToolsListObserved,
			traffic.ToolCallRequestCount,
			traffic.ToolCallObserved,
			traffic.ToolCallResultSentCount,
			result.toolCalls[server],
			result.successfulCalls[server],
		))
	}
	parts = append(parts, fmt.Sprintf(
		"native_denial=%t,harness_rejected=%t,unexpected_tool_updates=%d,capture_failed=%t",
		result.nativeDenialObserved,
		result.harnessRejected,
		result.unexpectedToolUpdates,
		result.captureFailed,
	))
	return strings.Join(parts, ";")
}

func startNativeACPDriver(
	args []string,
	workdir string,
	env []string,
	recorder *acpdbg.Recorder,
	sentinels *nativeTestSentinels,
	knownServers []string,
	allowedServers []string,
) (*nativeACPDriver, error) {
	if len(args) == 0 {
		return nil, errors.New("native ACP command is empty")
	}
	cmd := execCommand(args, workdir, env)
	if err := configureNativeProcessGroup(cmd); err != nil {
		return nil, err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, errors.New("native ACP stdin pipe could not be created")
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, errors.New("native ACP stdout pipe could not be created")
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, errors.New("native ACP stderr pipe could not be created")
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
		return nil, errors.New("exact native package executable could not be started")
	}
	if err := recorder.Meta("start", map[string]any{
		"command": args,
		"workdir": workdir,
	}); err != nil {
		_ = stdin.Close()
		_ = terminateNativeProcessGroup(cmd, 0)
		_ = cmd.Wait()
		_ = stdout.Close()
		_ = stderr.Close()
		return nil, errors.New("private ACP start capture could not be written")
	}

	driver := &nativeACPDriver{
		cmd:            cmd,
		stdin:          stdin,
		stdout:         stdout,
		stderr:         stderr,
		framer:         acpdbg.NewFramer(stdin, stdout),
		recorder:       recorder,
		frames:         newNativeACPFrameQueue(),
		toolCalls:      make(map[string]*nativeToolCall),
		knownServers:   append([]string(nil), knownServers...),
		allowedServers: make(map[string]bool, len(allowedServers)),
		approved:       make(map[string]int),
		rejected:       make(map[string]int),
		sentinels:      sentinels,
	}
	for _, server := range allowedServers {
		driver.allowedServers[server] = true
	}
	driver.readWG.Add(1)
	go driver.readFrames()
	driver.stderrWG.Add(1)
	go driver.readStderr()
	return driver, nil
}

func (d *nativeACPDriver) readFrames() {
	defer d.readWG.Done()
	for {
		frame, err := d.framer.Read()
		if err != nil {
			d.frames.finish(err)
			return
		}
		d.recordCaptureError(d.recorder.Received(frame))
		d.frames.push(frame)
	}
}

func (d *nativeACPDriver) readStderr() {
	defer d.stderrWG.Done()
	scanner := bufio.NewScanner(d.stderr)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		d.recordCaptureError(d.recorder.Stderr(line))
		d.stderrMu.Lock()
		d.stderrLog = append(d.stderrLog, line)
		d.stderrMu.Unlock()
	}
}

func (d *nativeACPDriver) request(ctx context.Context, method string, params any) (acpdbg.Frame, error) {
	request, requestID := d.framer.NewRequest(method, params)
	if err := d.recorder.Sent(request); err != nil {
		return nil, errors.New("native ACP request could not be recorded")
	}
	if err := d.framer.Write(request); err != nil {
		return nil, errors.New("native ACP request could not be written")
	}
	for {
		frame, err := d.frames.next(ctx)
		if err != nil {
			return nil, normalizeACPError(method, err)
		}
		if nativeACPResponseID(frame, requestID) {
			if errorMessage, ok := nativeACPErrorMessage(frame); ok {
				if strings.Contains(strings.ToLower(errorMessage), "auth") ||
					strings.Contains(strings.ToLower(errorMessage), "login") ||
					strings.Contains(strings.ToLower(errorMessage), "credential") {
					return nil, errors.New("authentication_required")
				}
				return nil, errors.New("native ACP request returned an error")
			}
			return frame, nil
		}
		if err := d.handleUnsolicited(frame); err != nil {
			return nil, err
		}
	}
}

func (d *nativeACPDriver) initialize(ctx context.Context) (string, bool, bool, error) {
	frame, err := d.request(ctx, "initialize", map[string]any{
		"protocolVersion": 1,
		"clientInfo": map[string]any{
			"name":    "copilot-native-qualification",
			"version": "1",
		},
		"clientCapabilities": map[string]any{
			"fs":       map[string]any{"readTextFile": false, "writeTextFile": false},
			"terminal": false,
		},
	})
	if err != nil {
		return "", false, false, err
	}
	result, _ := frame["result"].(map[string]any)
	agentInfo, _ := result["agentInfo"].(map[string]any)
	version, _ := agentInfo["version"].(string)
	capabilities, _ := result["agentCapabilities"].(map[string]any)
	mcpCapabilities, _ := capabilities["mcpCapabilities"].(map[string]any)
	http, _ := mcpCapabilities["http"].(bool)
	sse, _ := mcpCapabilities["sse"].(bool)
	return version, http, sse, nil
}

func (d *nativeACPDriver) newSession(
	ctx context.Context,
	workspace string,
	acpServer map[string]any,
) (string, error) {
	mcpServers := []any{}
	if acpServer != nil {
		mcpServers = append(mcpServers, acpServer)
	}
	frame, err := d.request(ctx, "session/new", map[string]any{
		"cwd":        workspace,
		"mcpServers": mcpServers,
	})
	if err != nil {
		return "", err
	}
	result, _ := frame["result"].(map[string]any)
	sessionID, _ := result["sessionId"].(string)
	if sessionID == "" {
		return "", errors.New("native ACP session/new returned no session identity")
	}
	return sessionID, nil
}

func (d *nativeACPDriver) prompt(ctx context.Context, sessionID, text string) error {
	frame, err := d.request(ctx, "session/prompt", map[string]any{
		"sessionId": sessionID,
		"prompt": []any{
			map[string]any{"type": "text", "text": text},
		},
	})
	if err != nil {
		return err
	}
	result, _ := frame["result"].(map[string]any)
	if stopReason, ok := result["stopReason"].(string); ok && stopReason != "" && stopReason != "end_turn" {
		return errors.New("native ACP prompt did not finish normally")
	}
	return nil
}

func (d *nativeACPDriver) handleUnsolicited(frame acpdbg.Frame) error {
	switch frame.Method() {
	case "session/update":
		d.observeToolUpdate(frame)
	case "session/request_permission":
		return d.handlePermissionRequest(frame)
	default:
		if frame.Method() != "" && frame.ID() != nil {
			reply := acpdbg.NewMethodNotFound(frame.ID(), frame.Method())
			if err := d.recorder.Sent(reply); err != nil {
				d.recordCaptureError(err)
				return errors.New("unexpected ACP request could not be captured")
			}
			if err := d.framer.Write(reply); err != nil {
				return errors.New("unexpected ACP request could not be rejected")
			}
			d.unexpected = append(d.unexpected, frame.Method())
		}
	}
	return nil
}

func (d *nativeACPDriver) observeToolUpdate(frame acpdbg.Frame) {
	params, _ := frame["params"].(map[string]any)
	update, _ := params["update"].(map[string]any)
	if update == nil {
		return
	}
	kind, _ := update["sessionUpdate"].(string)
	if kind != "tool_call" && kind != "tool_call_update" {
		return
	}
	d.mergeToolCall(update)
}

func (d *nativeACPDriver) mergeToolCall(update map[string]any) {
	callID, _ := update["toolCallId"].(string)
	if callID == "" {
		return
	}
	call := d.toolCalls[callID]
	if call == nil {
		call = &nativeToolCall{}
		d.toolCalls[callID] = call
	}
	if name, ok := update["name"].(string); ok {
		call.name = name
		if name != "" && d.serverForTool(name) == "" {
			d.unexpected = append(d.unexpected, "unexpected tool call")
		}
	}
	if toolKind, ok := update["kind"].(string); ok {
		call.kind = toolKind
	}
	if status, ok := update["status"].(string); ok {
		call.status = status
	}
	if rawInput, ok := update["rawInput"]; ok {
		call.rawInput = rawInput
	}
	if rawOutput, ok := update["rawOutput"]; ok {
		call.rawOutput = rawOutput
	}
	if content, ok := update["content"]; ok {
		call.content = content
	}
}

func (d *nativeACPDriver) handlePermissionRequest(frame acpdbg.Frame) error {
	params, _ := frame["params"].(map[string]any)
	toolCall, _ := params["toolCall"].(map[string]any)
	d.mergeToolCall(toolCall)
	callID, _ := toolCall["toolCallId"].(string)
	call := d.toolCalls[callID]
	server := ""
	if call != nil {
		server = d.serverForTool(call.name)
	}
	allow := call != nil &&
		server != "" &&
		d.allowedServers[server] &&
		nativeToolNameMatchesServer(call.name, server) &&
		nativeInputIsEmpty(call.rawInput) &&
		d.sentinels != nil &&
		d.sentinels.allowsReadOnlyCall(server, call.name, call.rawInput) &&
		d.approved[server] == 0

	options, _ := params["options"].([]any)
	selectedKind := "reject_once"
	selectedID := ""
	for _, rawOption := range options {
		option, _ := rawOption.(map[string]any)
		kind, _ := option["kind"].(string)
		optionID, _ := option["optionId"].(string)
		if allow && kind == "allow_once" {
			selectedKind = kind
			selectedID = optionID
			break
		}
		if selectedID == "" && kind == "reject_once" {
			selectedID = optionID
		}
	}
	outcome := map[string]any{"outcome": "cancelled"}
	if selectedID != "" {
		outcome = map[string]any{
			"outcome":  "selected",
			"optionId": selectedID,
		}
	}
	response := acpdbg.Frame{
		"jsonrpc": "2.0",
		"id":      frame.ID(),
		"result": map[string]any{
			"outcome": outcome,
		},
	}
	if err := d.recorder.Sent(response); err != nil {
		d.recordCaptureError(err)
		return errors.New("native ACP permission response could not be captured")
	}
	if err := d.framer.Write(response); err != nil {
		return errors.New("native ACP permission response could not be written")
	}
	if allow && selectedKind == "allow_once" && selectedID != "" {
		d.approved[server]++
		return nil
	}
	if server != "" {
		d.rejected[server]++
	} else {
		d.unexpected = append(d.unexpected, "unidentified permission request")
	}
	return nil
}

func (d *nativeACPDriver) serverForTool(name string) string {
	for _, server := range d.knownServers {
		if nativeToolNameMatchesServer(name, server) {
			return server
		}
	}
	return ""
}

func nativeToolNameMatchesServer(name, server string) bool {
	name = normalizeNativeToolName(name)
	server = normalizeNativeToolName(server)
	return name == "mcp__"+server+"__"+nativeSentinelToolName ||
		name == server+"__"+nativeSentinelToolName ||
		name == server+"_"+nativeSentinelToolName
}

func normalizeNativeToolName(value string) string {
	value = strings.ToLower(value)
	return strings.NewReplacer("-", "_", ".", "_").Replace(value)
}

func nativeServerMentioned(value, server string) bool {
	value = normalizeNativeToolName(value)
	server = normalizeNativeToolName(server)
	if server == "" {
		return false
	}
	if strings.Contains(value, "mcp__"+server+"__") {
		return true
	}
	for offset := 0; offset < len(value); {
		index := strings.Index(value[offset:], server)
		if index < 0 {
			return false
		}
		index += offset
		end := index + len(server)
		beforeIsIdentifier := index > 0 && nativeIdentifierByte(value[index-1])
		afterIsIdentifier := end < len(value) && nativeIdentifierByte(value[end])
		if !beforeIsIdentifier && !afterIsIdentifier {
			return true
		}
		offset = index + 1
	}
	return false
}

func nativeIdentifierByte(value byte) bool {
	return value >= 'a' && value <= 'z' ||
		value >= '0' && value <= '9' ||
		value == '_'
}

func nativeInputIsEmpty(value any) bool {
	if value == nil {
		return false
	}
	input, ok := value.(map[string]any)
	return ok && len(input) == 0
}

func (d *nativeACPDriver) hasSuccessfulSentinelCall(server string) bool {
	for _, call := range d.toolCalls {
		if d.serverForTool(call.name) != server {
			continue
		}
		if call.status != "completed" && call.status != "complete" && call.status != "succeeded" {
			continue
		}
		if nativeValueContains(call.rawOutput, "sentinel") || nativeValueContains(call.content, "sentinel") {
			return true
		}
	}
	return false
}

func (d *nativeACPDriver) callCount(server string) int {
	count := 0
	for _, call := range d.toolCalls {
		if d.serverForTool(call.name) == server && call.status != "" {
			count++
		}
	}
	return count
}

func (d *nativeACPDriver) wasHarnessRejected(server string) bool {
	return d.rejected[server] > 0
}

func (d *nativeACPDriver) recordCaptureError(err error) {
	if err == nil {
		return
	}
	d.captureMu.Lock()
	if d.captureErr == nil {
		d.captureErr = err
	}
	d.captureMu.Unlock()
}

func (d *nativeACPDriver) captureFailed() bool {
	d.captureMu.Lock()
	defer d.captureMu.Unlock()
	return d.captureErr != nil
}

func (d *nativeACPDriver) hasNativeDenial(server string, managed bool) bool {
	for _, call := range d.toolCalls {
		if d.serverForTool(call.name) != server {
			continue
		}
		if call.status != "failed" && call.status != "error" {
			continue
		}
		evidence := strings.ToLower(fmt.Sprint(call.rawOutput, call.content))
		if managed {
			if (strings.Contains(evidence, "managed") || strings.Contains(evidence, "policy")) &&
				(strings.Contains(evidence, "denied") || strings.Contains(evidence, "blocked") || strings.Contains(evidence, "disallowed")) {
				return true
			}
		} else if strings.Contains(evidence, "disabled") || strings.Contains(evidence, "not enabled") {
			return true
		}
	}

	d.stderrMu.Lock()
	defer d.stderrMu.Unlock()
	for _, line := range d.stderrLog {
		lower := strings.ToLower(line)
		if !nativeServerMentioned(line, server) {
			continue
		}
		if managed {
			if (strings.Contains(lower, "managed") || strings.Contains(lower, "policy")) &&
				(strings.Contains(lower, "denied") || strings.Contains(lower, "blocked") || strings.Contains(lower, "disallowed")) {
				return true
			}
		} else if strings.Contains(lower, "disabled") || strings.Contains(lower, "not enabled") {
			return true
		}
	}
	return false
}

func nativeValueContains(value any, needle string) bool {
	switch typed := value.(type) {
	case string:
		return strings.Contains(strings.ToLower(typed), strings.ToLower(needle))
	case []any:
		for _, item := range typed {
			if nativeValueContains(item, needle) {
				return true
			}
		}
	case map[string]any:
		for _, item := range typed {
			if nativeValueContains(item, needle) {
				return true
			}
		}
	}
	return false
}

func (d *nativeACPDriver) close() error {
	var closeErr error
	d.closeOnce.Do(func() {
		if d.stdin != nil {
			_ = d.stdin.Close()
		}
		if err := terminateNativeProcessGroup(d.cmd, 250*time.Millisecond); err != nil {
			_ = killNativeProcessGroup(d.cmd)
		}
		closeErr = d.cmd.Wait()
		if d.stdout != nil {
			_ = d.stdout.Close()
		}
		if d.stderr != nil {
			_ = d.stderr.Close()
		}
		d.readWG.Wait()
		d.stderrWG.Wait()
		d.recordCaptureError(d.recorder.Meta("close", map[string]any{"exit_error": closeErr != nil}))
	})
	return closeErr
}

func nativeACPResponseID(frame acpdbg.Frame, want int) bool {
	switch value := frame.ID().(type) {
	case float64:
		return int(value) == want
	case int:
		return value == want
	case json.Number:
		parsed, err := value.Int64()
		return err == nil && int(parsed) == want
	case string:
		return value == fmt.Sprint(want)
	default:
		return false
	}
}

func nativeACPErrorMessage(frame acpdbg.Frame) (string, bool) {
	value, ok := frame["error"].(map[string]any)
	if !ok {
		return "", false
	}
	message, _ := value["message"].(string)
	return message, true
}

func normalizeACPError(method string, err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%s timed out", method)
	}
	if errors.Is(err, io.EOF) {
		return fmt.Errorf("%s ended before a response", method)
	}
	return fmt.Errorf("%s failed", method)
}

func nativeACPArgs(
	version string,
	paths nativeRunPaths,
	baseArgs []string,
	fixtures nativePluginFixture,
	sentinels *nativeTestSentinels,
	disabledServer string,
	includeNativeConfig bool,
	toolServers []string,
) []string {
	args := append(append([]string(nil), baseArgs...), nativeGlobalArgs(version, paths)...)
	args = append(args, "--acp", "--disable-builtin-mcps", "--disallow-temp-dir")
	args = append(args, "--max-ai-credits", "30")
	args = append(args,
		"--plugin-dir", fixtures.targetRoot,
		"--plugin-dir", fixtures.siblingRoot,
	)
	if includeNativeConfig {
		args = append(args, "--additional-mcp-config",
			nativeAdditionalMCPConfig(nativeConfigServer, sentinels.httpURL(nativeConfigServer)))
	}
	if disabledServer != "" {
		args = append(args, "--disable-mcp-server", disabledServer)
	}
	args = append(args,
		"--secret-env-vars",
		"HOME", "COPILOT_HOME", "PATH",
		"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy",
		"ALL_PROXY", "all_proxy", "NO_PROXY", "no_proxy",
		"SSL_CERT_FILE", "SSL_CERT_DIR", "NODE_EXTRA_CA_CERTS",
	)
	args = append(args, "--available-tools")
	args = append(args, nativeToolNames(toolServers...)...)
	return args
}

func configurePluginMCPFixtures(fixtures nativePluginFixture, sentinels *nativeTestSentinels) error {
	if err := writePluginMCPConfig(fixtures.targetRoot, nativeTargetServer, sentinels.httpURL(nativeTargetServer)); err != nil {
		return err
	}
	if err := writePluginMCPConfig(fixtures.siblingRoot, nativeSiblingServer, sentinels.httpURL(nativeSiblingServer)); err != nil {
		return err
	}
	return nil
}
