package mcpconfig

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
)

const nativeMCPDiagnosticMessageLimit = 1024

type NativeMCPDiagnostic struct {
	Operation      NativeMCPDiagnosticOperation `json:"operation"`
	Stage          NativeMCPDiagnosticStage     `json:"stage"`
	Kind           NativeMCPDiagnosticKind      `json:"kind"`
	Message        string                       `json:"message"`
	ExitCode       *int                         `json:"exit_code,omitempty"`
	CleanupMessage string                       `json:"cleanup_message,omitempty"`
}

type NativeMCPDiagnosticOperation string
type NativeMCPDiagnosticStage string
type NativeMCPDiagnosticKind string

const (
	nativeMCPDiagnosticEnable    NativeMCPDiagnosticOperation = "enable"
	nativeMCPDiagnosticListTools NativeMCPDiagnosticOperation = "list_tools"
)

const (
	nativeMCPDiagnosticResolve NativeMCPDiagnosticStage = "resolve"
	nativeMCPDiagnosticStart   NativeMCPDiagnosticStage = "start"
	nativeMCPDiagnosticWait    NativeMCPDiagnosticStage = "wait"
	nativeMCPDiagnosticCleanup NativeMCPDiagnosticStage = "cleanup"
	nativeMCPDiagnosticOutput  NativeMCPDiagnosticStage = "output"
)

const (
	nativeMCPDiagnosticExecutableUnavailable NativeMCPDiagnosticKind = "executable_unavailable"
	nativeMCPDiagnosticStartFailed           NativeMCPDiagnosticKind = "start_failed"
	nativeMCPDiagnosticWaitFailed            NativeMCPDiagnosticKind = "wait_failed"
	nativeMCPDiagnosticOutputWaitTimeout     NativeMCPDiagnosticKind = "output_wait_timeout"
	nativeMCPDiagnosticTimeout               NativeMCPDiagnosticKind = "timeout"
	nativeMCPDiagnosticCanceled              NativeMCPDiagnosticKind = "canceled"
	nativeMCPDiagnosticCleanupFailed         NativeMCPDiagnosticKind = "cleanup_failed"
	nativeMCPDiagnosticExitStatus            NativeMCPDiagnosticKind = "exit_status"
	nativeMCPDiagnosticOutputTruncated       NativeMCPDiagnosticKind = "output_truncated"
	nativeMCPDiagnosticUnrecognizedOutput    NativeMCPDiagnosticKind = "unrecognized_output"
)

var (
	nativeMCPANSISequence = regexp.MustCompile(`\x1b(?:\[[0-?]*[ -/]*[@-~\x40-\x7e]|\][^\x07]*(?:\x07|\x1b\\)|[@-_])`)
	nativeMCPURLSpan      = regexp.MustCompile(`(?i)(?:[a-z][a-z0-9+.-]*://|www\.)[^\s<>"']+`)
)

type nativeMCPCommandError struct {
	diagnostic       *NativeMCPDiagnostic
	cause            error
	cleanupErr       error
	exitCodeObserved bool
}

func (e *nativeMCPCommandError) Error() string {
	if e == nil || e.diagnostic == nil || e.diagnostic.Message == "" {
		return "native MCP command failed"
	}
	return e.diagnostic.Message
}

func (e *nativeMCPCommandError) Unwrap() error {
	if e == nil {
		return nil
	}
	if e.diagnostic != nil && (e.diagnostic.Kind == nativeMCPDiagnosticExecutableUnavailable || e.diagnostic.Kind == nativeMCPDiagnosticStartFailed) {
		return errors.Join(ErrNativeMCPExecutableUnavailable, e.cause, e.cleanupErr)
	}
	return errors.Join(e.cause, e.cleanupErr)
}

func nativeMCPWrapCommandError(stage NativeMCPDiagnosticStage, kind NativeMCPDiagnosticKind, cause, cleanupErr error, exitCode *int) error {
	diagnostic := nativeMCPDiagnosticForError("", stage, kind, cause, cleanupErr, exitCode)
	return &nativeMCPCommandError{
		diagnostic:       diagnostic,
		cause:            cause,
		cleanupErr:       cleanupErr,
		exitCodeObserved: exitCode != nil,
	}
}

func nativeMCPDiagnosticForError(
	operation NativeMCPDiagnosticOperation,
	stage NativeMCPDiagnosticStage,
	kind NativeMCPDiagnosticKind,
	cause, cleanupErr error,
	exitCode *int,
) *NativeMCPDiagnostic {
	message := nativeMCPDiagnosticMessage(kind, cause, exitCode)
	diagnostic := &NativeMCPDiagnostic{Operation: operation, Stage: stage, Kind: kind, Message: message}
	if exitCode != nil {
		code := *exitCode
		diagnostic.ExitCode = &code
	}
	if cleanupErr != nil {
		diagnostic.CleanupMessage = cleanupErr.Error()
	}
	if operation == "" {
		diagnostic.Message = sanitizeNativeMCPDiagnosticMessage(diagnostic.Message)
		diagnostic.CleanupMessage = sanitizeNativeMCPDiagnosticMessage(diagnostic.CleanupMessage)
		return diagnostic
	}
	return sanitizeNativeMCPDiagnostic(diagnostic, operation)
}

func nativeMCPDiagnosticMessage(kind NativeMCPDiagnosticKind, cause error, exitCode *int) string {
	switch kind {
	case nativeMCPDiagnosticExecutableUnavailable:
		return "Native MCP executable is unavailable."
	case nativeMCPDiagnosticTimeout:
		return "Native MCP command timed out."
	case nativeMCPDiagnosticCanceled:
		return "Native MCP command was canceled."
	case nativeMCPDiagnosticExitStatus:
		if exitCode != nil {
			return fmt.Sprintf("Native MCP command exited with status %d.", *exitCode)
		}
		return "Native MCP command exited unsuccessfully."
	case nativeMCPDiagnosticOutputTruncated:
		return "Native MCP command output exceeded the size limit."
	case nativeMCPDiagnosticUnrecognizedOutput:
		return "Native MCP verification output could not be recognized."
	case nativeMCPDiagnosticCleanupFailed:
		if cause != nil {
			return cause.Error()
		}
		return "Native MCP command cleanup failed."
	case nativeMCPDiagnosticOutputWaitTimeout:
		if cause != nil {
			return cause.Error()
		}
		return "Native MCP command output did not finish before the wait limit."
	default:
		if cause != nil {
			return cause.Error()
		}
		return "Native MCP command failed."
	}
}

func nativeMCPDiagnosticForRunnerError(
	ctx context.Context,
	operation NativeMCPDiagnosticOperation,
	result NativeMCPCommandResult,
	err error,
) *NativeMCPDiagnostic {
	var commandErr *nativeMCPCommandError
	if errors.As(err, &commandErr) {
		return sanitizeNativeMCPDiagnostic(commandErr.diagnostic, operation)
	}
	stage, kind := nativeMCPDiagnosticWait, nativeMCPDiagnosticWaitFailed
	switch {
	case errors.Is(err, ErrNativeMCPExecutableUnavailable):
		stage, kind = nativeMCPDiagnosticResolve, nativeMCPDiagnosticExecutableUnavailable
	case errors.Is(ctx.Err(), context.Canceled), errors.Is(err, context.Canceled):
		kind = nativeMCPDiagnosticCanceled
	case errors.Is(ctx.Err(), context.DeadlineExceeded), errors.Is(err, context.DeadlineExceeded):
		kind = nativeMCPDiagnosticTimeout
	case errors.Is(err, exec.ErrWaitDelay):
		kind = nativeMCPDiagnosticOutputWaitTimeout
	}
	return nativeMCPDiagnosticForError(operation, stage, kind, err, nil, nativeMCPExitCode(result))
}

func nativeMCPResultFailure(status NativeMCPStatus, reason string, result NativeMCPCommandResult, operation NativeMCPDiagnosticOperation, fallbackKind NativeMCPDiagnosticKind) NativeMCPReadiness {
	diagnostic := sanitizeNativeMCPDiagnostic(result.Diagnostic, operation)
	if diagnostic == nil {
		stage, kind := nativeMCPDiagnosticOutput, fallbackKind
		if result.Truncated {
			kind = nativeMCPDiagnosticOutputTruncated
		} else if result.ExitCode != 0 {
			stage, kind = nativeMCPDiagnosticWait, nativeMCPDiagnosticExitStatus
		}
		diagnostic = nativeMCPDiagnosticForError(operation, stage, kind, nil, nil, nativeMCPExitCode(result))
	}
	return nativeMCPFailureWithDiagnostic(status, reason, diagnostic)
}

func nativeMCPFailureWithDiagnostic(status NativeMCPStatus, reason string, diagnostic *NativeMCPDiagnostic) NativeMCPReadiness {
	return NativeMCPReadiness{Status: status, ReasonCode: reason, Diagnostic: diagnostic}
}

func nativeMCPFailedCommandResult(operation NativeMCPDiagnosticOperation, stage NativeMCPDiagnosticStage, kind NativeMCPDiagnosticKind, cause, cleanupErr error, exitCode *int) NativeMCPCommandResult {
	diagnostic := nativeMCPDiagnosticForError(operation, stage, kind, cause, cleanupErr, exitCode)
	result := NativeMCPCommandResult{ExitCode: -1, Diagnostic: diagnostic}
	if exitCode != nil {
		result.ExitCode = *exitCode
		result.ExitCodeObserved = true
	}
	return result
}

func sanitizeNativeMCPDiagnostic(diagnostic *NativeMCPDiagnostic, operation NativeMCPDiagnosticOperation) *NativeMCPDiagnostic {
	if diagnostic == nil || !nativeMCPValidOperation(operation) ||
		(diagnostic.Operation != "" && diagnostic.Operation != operation) ||
		!nativeMCPValidStage(diagnostic.Stage) || !nativeMCPValidKind(diagnostic.Kind) {
		return nil
	}
	copy := *diagnostic
	copy.Operation = operation
	if copy.ExitCode != nil {
		if *copy.ExitCode < 0 {
			return nil
		}
		code := *copy.ExitCode
		copy.ExitCode = &code
	}
	copy.Message = sanitizeNativeMCPDiagnosticMessage(copy.Message)
	if copy.Message == "" {
		copy.Message = sanitizeNativeMCPDiagnosticMessage(nativeMCPDiagnosticMessage(copy.Kind, nil, copy.ExitCode))
	}
	copy.CleanupMessage = sanitizeNativeMCPDiagnosticMessage(copy.CleanupMessage)
	return &copy
}

// NormalizeNativeMCPDiagnostic validates the closed diagnostic fields and
// returns a redacted, bounded copy for persistence or publication.
func NormalizeNativeMCPDiagnostic(diagnostic *NativeMCPDiagnostic, operation NativeMCPDiagnosticOperation) *NativeMCPDiagnostic {
	return sanitizeNativeMCPDiagnostic(diagnostic, operation)
}

func sanitizeNativeMCPDiagnosticMessage(message string) string {
	message = strings.ToValidUTF8(message, "�")
	message = nativeMCPANSISequence.ReplaceAllString(message, "")
	message = routingerr.Sanitize(message)
	message = nativeMCPURLSpan.ReplaceAllString(message, "[url-redacted]")
	var normalized strings.Builder
	for _, char := range message {
		if unicode.IsControl(char) {
			normalized.WriteByte(' ')
			continue
		}
		normalized.WriteRune(char)
	}
	message = strings.Join(strings.Fields(normalized.String()), " ")
	return truncateNativeMCPDiagnosticMessage(message)
}

func truncateNativeMCPDiagnosticMessage(message string) string {
	if len(message) <= nativeMCPDiagnosticMessageLimit {
		return message
	}
	message = message[:nativeMCPDiagnosticMessageLimit]
	for !utf8.ValidString(message) {
		message = message[:len(message)-1]
	}
	return message
}

func nativeMCPExitCode(result NativeMCPCommandResult) *int {
	if !result.ExitCodeObserved && result.ExitCode == 0 {
		return nil
	}
	code := result.ExitCode
	if code < 0 {
		return nil
	}
	return &code
}

func nativeMCPOperation(args []string) NativeMCPDiagnosticOperation {
	if len(args) < 2 || args[0] != "mcp" {
		return ""
	}
	switch args[1] {
	case "enable":
		return nativeMCPDiagnosticEnable
	case "list-tools":
		return nativeMCPDiagnosticListTools
	default:
		return ""
	}
}

func nativeMCPValidOperation(operation NativeMCPDiagnosticOperation) bool {
	return operation == nativeMCPDiagnosticEnable || operation == nativeMCPDiagnosticListTools
}

func nativeMCPValidStage(stage NativeMCPDiagnosticStage) bool {
	switch stage {
	case nativeMCPDiagnosticResolve, nativeMCPDiagnosticStart, nativeMCPDiagnosticWait, nativeMCPDiagnosticCleanup, nativeMCPDiagnosticOutput:
		return true
	default:
		return false
	}
}

func nativeMCPValidKind(kind NativeMCPDiagnosticKind) bool {
	switch kind {
	case nativeMCPDiagnosticExecutableUnavailable, nativeMCPDiagnosticStartFailed, nativeMCPDiagnosticWaitFailed,
		nativeMCPDiagnosticOutputWaitTimeout, nativeMCPDiagnosticTimeout, nativeMCPDiagnosticCanceled,
		nativeMCPDiagnosticCleanupFailed, nativeMCPDiagnosticExitStatus, nativeMCPDiagnosticOutputTruncated,
		nativeMCPDiagnosticUnrecognizedOutput:
		return true
	default:
		return false
	}
}

func nativeMCPProcessExitCode(cmd *exec.Cmd) int {
	if cmd == nil || cmd.ProcessState == nil {
		return -1
	}
	return cmd.ProcessState.ExitCode()
}

func nativeMCPProcessExitCodePointer(cmd *exec.Cmd) *int {
	code := nativeMCPProcessExitCode(cmd)
	if code < 0 {
		return nil
	}
	return &code
}
