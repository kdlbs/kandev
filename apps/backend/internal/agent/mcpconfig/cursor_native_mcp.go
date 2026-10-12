package mcpconfig

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const nativeMCPOutputLimit = 32 * 1024

var (
	ErrNativeMCPExecutableUnavailable = errors.New("native MCP executable unavailable")
)

// NativeMCPStatus is the closed readiness state returned by native adapters.
type NativeMCPStatus string

const (
	NativeMCPStatusReady                  NativeMCPStatus = "ready"
	NativeMCPStatusAuthenticationRequired NativeMCPStatus = "authentication_required"
	NativeMCPStatusApprovalFailed         NativeMCPStatus = "approval_failed"
	NativeMCPStatusConnectionFailed       NativeMCPStatus = "connection_failed"
	NativeMCPStatusUnavailable            NativeMCPStatus = "unavailable"
)

// NativeMCPReadiness contains only safe, bounded readiness metadata. Raw native
// command output is deliberately not exposed past the adapter boundary.
type NativeMCPReadiness struct {
	Status            NativeMCPStatus
	ReasonCode        string
	ToolCount         *int
	ApprovalSucceeded bool
	Diagnostic        *NativeMCPDiagnostic `json:"mcp_diagnostic,omitempty"`
}

// NativeMCPCommandResult is the bounded result of one native CLI invocation.
type NativeMCPCommandResult struct {
	Stdout           []byte
	Stderr           []byte
	ExitCode         int
	Truncated        bool
	ExitCodeObserved bool                 `json:"-"`
	Diagnostic       *NativeMCPDiagnostic `json:"mcp_diagnostic,omitempty"`
}

// NativeMCPCommandRunner executes a native agent MCP command without a shell.
type NativeMCPCommandRunner interface {
	Run(ctx context.Context, executable string, args []string, workspace string, env map[string]string) (NativeMCPCommandResult, error)
}

// ExecNativeMCPCommandRunner invokes a local executable with a bounded output
// buffer. Runtime environment values override the backend environment so native
// CLI commands use the same HOME and PATH as the task process.
type ExecNativeMCPCommandRunner struct {
	OutputLimit int
}

func (r ExecNativeMCPCommandRunner) Run(ctx context.Context, executable string, args []string, workspace string, env map[string]string) (NativeMCPCommandResult, error) {
	resolved, err := resolveExecutable(executable, env)
	if err != nil {
		return nativeMCPFailedCommandResult(nativeMCPOperation(args), nativeMCPDiagnosticResolve, nativeMCPDiagnosticExecutableUnavailable, err, nil, nil), nativeMCPWrapCommandError(nativeMCPDiagnosticResolve, nativeMCPDiagnosticExecutableUnavailable, err, nil, nil)
	}
	cmd := exec.CommandContext(ctx, resolved, args...)
	cmd.Dir = workspace
	cmd.Env = mergeProcessEnvironment(os.Environ(), env)
	cmd.WaitDelay = nativeMCPWaitDelay
	limit := r.OutputLimit
	if limit <= 0 || limit > nativeMCPOutputLimit {
		limit = nativeMCPOutputLimit
	}
	stdout := &limitedOutput{limit: limit}
	stderr := &limitedOutput{limit: limit}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	exitCode, diagnostic, runErr := runNativeMCPCommand(cmd)
	result := NativeMCPCommandResult{
		Stdout:           stdout.Bytes(),
		Stderr:           stderr.Bytes(),
		ExitCode:         exitCode,
		ExitCodeObserved: exitCode >= 0,
		Truncated:        stdout.Truncated() || stderr.Truncated(),
	}
	if diagnostic != nil {
		result.Diagnostic = sanitizeNativeMCPDiagnostic(diagnostic, nativeMCPOperation(args))
	}
	var commandErr *nativeMCPCommandError
	if errors.As(runErr, &commandErr) {
		result.ExitCodeObserved = commandErr.exitCodeObserved
		result.Diagnostic = sanitizeNativeMCPDiagnostic(commandErr.diagnostic, nativeMCPOperation(args))
	}
	if ctx.Err() != nil {
		kind := nativeMCPDiagnosticCanceled
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			kind = nativeMCPDiagnosticTimeout
		}
		var cleanupErr error
		if commandErr != nil {
			cleanupErr = commandErr.cleanupErr
		}
		result.Diagnostic = sanitizeNativeMCPDiagnostic(nativeMCPDiagnosticForError(
			nativeMCPOperation(args), nativeMCPDiagnosticWait, kind, ctx.Err(), cleanupErr, nativeMCPExitCode(result),
		), nativeMCPOperation(args))
		return result, nativeMCPWrapCommandError(nativeMCPDiagnosticWait, kind, ctx.Err(), cleanupErr, nativeMCPExitCode(result))
	}
	if runErr == nil {
		return result, nil
	}
	return result, runErr
}

func runNativeMCPCommand(cmd *exec.Cmd) (int, *NativeMCPDiagnostic, error) {
	if err := prepareNativeMCPCommand(cmd); err != nil {
		return -1, nil, nativeMCPWrapCommandError(nativeMCPDiagnosticStart, nativeMCPDiagnosticStartFailed, err, nil, nil)
	}
	var guardMu sync.Mutex
	var guard *nativeMCPProcessGuard
	cmd.Cancel = func() error {
		guardMu.Lock()
		current := guard
		guardMu.Unlock()
		if current != nil {
			cancelCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			return current.Terminate(cancelCtx)
		}
		if cmd.Process != nil {
			if fallbackGuard, err := attachNativeMCPCommand(cmd); err == nil {
				cancelCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				return fallbackGuard.Terminate(cancelCtx)
			}
			return cmd.Process.Kill()
		}
		return os.ErrProcessDone
	}
	if err := cmd.Start(); err != nil {
		return -1, nil, nativeMCPWrapCommandError(nativeMCPDiagnosticStart, nativeMCPDiagnosticStartFailed, err, nil, nil)
	}
	processGuard, err := attachNativeMCPCommand(cmd)
	if err != nil {
		killErr := cmd.Process.Kill()
		waitErr := cmd.Wait()
		cleanupErr := errors.Join(killErr, waitErr)
		return nativeMCPProcessExitCode(cmd), nil, nativeMCPWrapCommandError(nativeMCPDiagnosticStart, nativeMCPDiagnosticStartFailed, err, cleanupErr, nativeMCPProcessExitCodePointer(cmd))
	}
	guardMu.Lock()
	guard = &processGuard
	guardMu.Unlock()
	runErr := cmd.Wait()
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 2*time.Second)
	cleanupErr := processGuard.Cleanup(cleanupCtx)
	cleanupCancel()
	exitCode := nativeMCPProcessExitCode(cmd)
	exitCodePointer := nativeMCPProcessExitCodePointer(cmd)
	return nativeMCPCommandResult(runErr, cleanupErr, exitCode, exitCodePointer)
}

func nativeMCPCommandResult(runErr, cleanupErr error, exitCode int, exitCodePointer *int) (int, *NativeMCPDiagnostic, error) {
	if runErr == nil {
		if cleanupErr != nil {
			return exitCode, nil, nativeMCPWrapCommandError(nativeMCPDiagnosticCleanup, nativeMCPDiagnosticCleanupFailed, cleanupErr, nil, exitCodePointer)
		}
		return exitCode, nil, nil
	}
	if errors.Is(runErr, exec.ErrWaitDelay) {
		return exitCode, nil, nativeMCPWrapCommandError(nativeMCPDiagnosticWait, nativeMCPDiagnosticOutputWaitTimeout, runErr, cleanupErr, exitCodePointer)
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) && exitErr.ExitCode() != 0 {
		if cleanupErr != nil {
			observedExitCode := exitErr.ExitCode()
			return observedExitCode, nativeMCPDiagnosticForError("", nativeMCPDiagnosticWait, nativeMCPDiagnosticExitStatus, runErr, cleanupErr, &observedExitCode), nil
		}
		return exitErr.ExitCode(), nil, nil
	}
	return exitCode, nil, nativeMCPWrapCommandError(nativeMCPDiagnosticWait, nativeMCPDiagnosticWaitFailed, runErr, cleanupErr, exitCodePointer)
}

// CursorNativeMCPAdapter prepares one exact native server identity and verifies
// it through Cursor's non-mutating list-tools command.
type CursorNativeMCPAdapter struct {
	Executable string
	Runner     NativeMCPCommandRunner
	Timeout    time.Duration
}

func (a CursorNativeMCPAdapter) EnableAndVerify(ctx context.Context, workspace string, env map[string]string, serverID string) NativeMCPReadiness {
	approval := a.Enable(ctx, workspace, env, serverID)
	if !approval.ApprovalSucceeded {
		return approval
	}
	result := a.Verify(ctx, workspace, env, serverID)
	result.ApprovalSucceeded = true
	return result
}

// Enable runs only the native per-server approval command. Callers that need
// progress between approval and verification can then invoke Verify separately.
func (a CursorNativeMCPAdapter) Enable(ctx context.Context, workspace string, env map[string]string, serverID string) NativeMCPReadiness {
	if !validNativeMCPServerID(serverID) {
		return nativeMCPFailure(NativeMCPStatusUnavailable, "unavailable")
	}
	commandCtx, cancel := a.commandContext(ctx)
	defer cancel()
	approved, err := a.run(commandCtx, workspace, env, nativeMCPDiagnosticEnable, "mcp", "enable", serverID)
	if err != nil {
		return nativeMCPRunnerFailure(commandCtx, approved, err, nativeMCPDiagnosticEnable)
	}
	if approved.ExitCode != 0 || approved.Truncated {
		return nativeMCPResultFailure(NativeMCPStatusApprovalFailed, "approval_failed", approved, nativeMCPDiagnosticEnable, nativeMCPDiagnosticOutputTruncated)
	}
	return NativeMCPReadiness{Status: NativeMCPStatusReady, ApprovalSucceeded: true}
}

func (a CursorNativeMCPAdapter) Verify(ctx context.Context, workspace string, env map[string]string, serverID string) NativeMCPReadiness {
	if !validNativeMCPServerID(serverID) {
		return nativeMCPFailure(NativeMCPStatusUnavailable, "unavailable")
	}
	commandCtx, cancel := a.commandContext(ctx)
	defer cancel()
	return a.verify(commandCtx, workspace, env, serverID)
}

func (a CursorNativeMCPAdapter) verify(ctx context.Context, workspace string, env map[string]string, serverID string) NativeMCPReadiness {
	result, err := a.run(ctx, workspace, env, nativeMCPDiagnosticListTools, "mcp", "list-tools", serverID)
	if err != nil {
		return nativeMCPRunnerFailure(ctx, result, err, nativeMCPDiagnosticListTools)
	}
	if result.ExitCode == 0 {
		if result.Truncated {
			return nativeMCPResultFailure(NativeMCPStatusConnectionFailed, "connection_failed", result, nativeMCPDiagnosticListTools, nativeMCPDiagnosticOutputTruncated)
		}
		count, ok := parseCursorListToolsHeader(result.Stdout, serverID)
		if !ok {
			return nativeMCPResultFailure(NativeMCPStatusConnectionFailed, "connection_failed", result, nativeMCPDiagnosticListTools, nativeMCPDiagnosticUnrecognizedOutput)
		}
		return NativeMCPReadiness{Status: NativeMCPStatusReady, ToolCount: &count}
	}
	output := append(append([]byte(nil), result.Stderr...), result.Stdout...)
	if status := classifyNativeMCPText(output, serverID); status != "" {
		switch status {
		case string(NativeMCPStatusAuthenticationRequired):
			return nativeMCPResultFailure(NativeMCPStatusAuthenticationRequired, "authentication_required", result, nativeMCPDiagnosticListTools, nativeMCPDiagnosticOutputTruncated)
		case string(NativeMCPStatusApprovalFailed):
			return nativeMCPResultFailure(NativeMCPStatusApprovalFailed, "approval_failed", result, nativeMCPDiagnosticListTools, nativeMCPDiagnosticOutputTruncated)
		}
	}
	if result.Truncated || result.ExitCode != 0 {
		return nativeMCPResultFailure(NativeMCPStatusConnectionFailed, "connection_failed", result, nativeMCPDiagnosticListTools, nativeMCPDiagnosticOutputTruncated)
	}
	return nativeMCPResultFailure(NativeMCPStatusConnectionFailed, "connection_failed", result, nativeMCPDiagnosticListTools, nativeMCPDiagnosticUnrecognizedOutput)
}

func (a CursorNativeMCPAdapter) run(ctx context.Context, workspace string, env map[string]string, operation NativeMCPDiagnosticOperation, args ...string) (NativeMCPCommandResult, error) {
	if strings.TrimSpace(a.Executable) == "" || strings.TrimSpace(workspace) == "" {
		err := ErrNativeMCPExecutableUnavailable
		result := nativeMCPFailedCommandResult(operation, nativeMCPDiagnosticResolve, nativeMCPDiagnosticExecutableUnavailable, err, nil, nil)
		return result, err
	}
	runner := a.Runner
	if runner == nil {
		runner = ExecNativeMCPCommandRunner{}
	}
	result, err := runner.Run(ctx, a.Executable, args, workspace, env)
	if result.Diagnostic == nil && err != nil {
		result.Diagnostic = nativeMCPDiagnosticForRunnerError(ctx, operation, result, err)
	}
	if result.Diagnostic != nil {
		result.Diagnostic = sanitizeNativeMCPDiagnostic(result.Diagnostic, operation)
	}
	return result, err
}

func (a CursorNativeMCPAdapter) commandContext(parent context.Context) (context.Context, context.CancelFunc) {
	timeout := a.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	return context.WithTimeout(parent, timeout)
}

func nativeMCPRunnerFailure(ctx context.Context, result NativeMCPCommandResult, err error, operation NativeMCPDiagnosticOperation) NativeMCPReadiness {
	diagnostic := sanitizeNativeMCPDiagnostic(result.Diagnostic, operation)
	if errors.Is(err, ErrNativeMCPExecutableUnavailable) {
		return nativeMCPFailureWithDiagnostic(NativeMCPStatusUnavailable, "unavailable", diagnostic)
	}
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		return nativeMCPFailureWithDiagnostic(NativeMCPStatusUnavailable, "canceled", diagnostic)
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return nativeMCPFailureWithDiagnostic(NativeMCPStatusUnavailable, "unavailable", diagnostic)
	}
	return nativeMCPFailureWithDiagnostic(NativeMCPStatusConnectionFailed, "connection_failed", diagnostic)
}

func nativeMCPFailure(status NativeMCPStatus, reason string) NativeMCPReadiness {
	return NativeMCPReadiness{Status: status, ReasonCode: reason}
}

func validNativeMCPServerID(serverID string) bool {
	return serverID != "" && strings.TrimSpace(serverID) == serverID && len(serverID) <= 512 &&
		!strings.HasPrefix(serverID, "-") && !strings.ContainsAny(serverID, "\x00\r\n")
}

// ValidNativeMCPServerID reports whether an exact native ID is safe to pass as
// one argv value to a native MCP adapter.
func ValidNativeMCPServerID(serverID string) bool {
	return validNativeMCPServerID(serverID)
}

func classifyNativeMCPText(output []byte, serverID string) string {
	lines := strings.Split(strings.ReplaceAll(string(output), "\r\n", "\n"), "\n")
	lowerID := strings.ToLower(serverID)
	for index, line := range lines {
		line = strings.ToLower(strings.TrimSpace(line))
		if line == "mcp '"+lowerID+"' requires authentication." && index+1 < len(lines) &&
			strings.ToLower(strings.TrimSpace(lines[index+1])) == "please run: agent mcp login "+lowerID {
			return string(NativeMCPStatusAuthenticationRequired)
		}
		if strings.HasPrefix(line, "failed to list tools: authentication required") {
			return string(NativeMCPStatusAuthenticationRequired)
		}
		unapproved := "failed to list tools: failed to load mcp '" + lowerID + "': mcp server \"" + lowerID + "\" has not been approved"
		if line == unapproved {
			return string(NativeMCPStatusApprovalFailed)
		}
	}
	return ""
}

func parseCursorListToolsHeader(output []byte, serverID string) (int, bool) {
	firstLine, _, _ := bytes.Cut(output, []byte{'\n'})
	line := strings.TrimSpace(string(firstLine))
	prefix := "Tools for " + serverID + " ("
	if !strings.HasPrefix(line, prefix) || !strings.HasSuffix(line, "):") {
		return 0, false
	}
	countText := strings.TrimSuffix(strings.TrimPrefix(line, prefix), "):")
	count, err := strconv.Atoi(countText)
	if err != nil || count < 0 || count > 10000 {
		return 0, false
	}
	return count, true
}

func mergeProcessEnvironment(base []string, overlay map[string]string) []string {
	values := make(map[string]string, len(base)+len(overlay))
	for _, item := range base {
		key, value, ok := strings.Cut(item, "=")
		if ok && key != "" {
			values[key] = value
		}
	}
	for key, value := range overlay {
		if key != "" && !strings.ContainsAny(key, "=\x00") && !strings.ContainsRune(value, '\x00') {
			values[key] = value
		}
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, key+"="+values[key])
	}
	return result
}

type limitedOutput struct {
	buffer bytes.Buffer
	limit  int
	large  bool
}

func (o *limitedOutput) Write(data []byte) (int, error) {
	if len(data) > o.limit-o.buffer.Len() {
		remaining := o.limit - o.buffer.Len()
		if remaining > 0 {
			_, _ = o.buffer.Write(data[:remaining])
		}
		o.large = true
		return len(data), nil
	}
	return o.buffer.Write(data)
}

func (o *limitedOutput) Bytes() []byte {
	return append([]byte(nil), o.buffer.Bytes()...)
}

func (o *limitedOutput) Truncated() bool {
	return o.large
}

var _ NativeMCPCommandRunner = ExecNativeMCPCommandRunner{}

var _ io.Writer = (*limitedOutput)(nil)
