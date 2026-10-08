package mcpconfig

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

type oneShotNativeMCPRunner struct {
	result NativeMCPCommandResult
	err    error
}

func (r oneShotNativeMCPRunner) Run(context.Context, string, []string, string, map[string]string) (NativeMCPCommandResult, error) {
	return r.result, r.err
}

func TestNativeMCPDiagnosticRetainsWaitDelay(t *testing.T) {
	readiness := (CursorNativeMCPAdapter{
		Executable: "cursor-agent",
		Runner:     oneShotNativeMCPRunner{err: exec.ErrWaitDelay},
	}).Verify(context.Background(), "/workspace", nil, "server")

	encoded, err := json.Marshal(readiness)
	if err != nil {
		t.Fatalf("marshal readiness: %v", err)
	}
	var payload struct {
		Diagnostic *struct {
			Operation      string `json:"operation"`
			Stage          string `json:"stage"`
			Kind           string `json:"kind"`
			Message        string `json:"message"`
			ExitCode       *int   `json:"exit_code"`
			CleanupMessage string `json:"cleanup_message"`
		} `json:"mcp_diagnostic"`
	}
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("decode readiness: %v", err)
	}
	if payload.Diagnostic == nil {
		t.Fatalf("readiness JSON %s has no retained MCP diagnostic", encoded)
	}
	got := payload.Diagnostic
	if got.Operation != "list_tools" || got.Stage != "wait" || got.Kind != "output_wait_timeout" {
		t.Fatalf("diagnostic operation/stage/kind = %q/%q/%q, want list_tools/wait/output_wait_timeout", got.Operation, got.Stage, got.Kind)
	}
	if !strings.Contains(got.Message, "WaitDelay") {
		t.Fatalf("diagnostic message = %q, want the retained wait cause", got.Message)
	}
	if got.ExitCode != nil || got.CleanupMessage != "" {
		t.Fatalf("diagnostic invented unavailable process details: %#v", got)
	}
}

func TestNativeMCPDiagnosticRunnerFailurePreservesCause(t *testing.T) {
	_, err := (ExecNativeMCPCommandRunner{}).Run(context.Background(), "", nil, "/workspace", nil)
	if !errors.Is(err, ErrNativeMCPExecutableUnavailable) {
		t.Fatalf("runner error = %v, want executable unavailable", err)
	}
}

func TestNativeMCPDiagnosticClassifiesResolutionAndStartFailures(t *testing.T) {
	missingWorkspace := filepath.Join(t.TempDir(), "missing")
	command, _ := hostShellCommand(t)
	result, err := (ExecNativeMCPCommandRunner{}).Run(context.Background(), command, []string{"mcp", "enable", "server"}, missingWorkspace, nil)
	if err == nil || !errors.Is(err, ErrNativeMCPExecutableUnavailable) {
		t.Fatalf("start error = %v, want executable-unavailable sentinel", err)
	}
	var pathErr *os.PathError
	var execErr *exec.Error
	if !errors.As(err, &pathErr) && !errors.As(err, &execErr) {
		t.Fatalf("start error %T does not preserve the operating-system cause", err)
	}
	if result.Diagnostic == nil || result.Diagnostic.Operation != nativeMCPDiagnosticEnable ||
		result.Diagnostic.Stage != nativeMCPDiagnosticStart || result.Diagnostic.Kind != nativeMCPDiagnosticStartFailed {
		t.Fatalf("start diagnostic = %#v, want enable/start/start_failed", result.Diagnostic)
	}
	if result.Diagnostic.ExitCode != nil {
		t.Fatalf("start diagnostic invented exit code: %#v", result.Diagnostic.ExitCode)
	}

	unavailable := (CursorNativeMCPAdapter{Runner: oneShotNativeMCPRunner{err: ErrNativeMCPExecutableUnavailable}}).
		Verify(context.Background(), "/workspace", nil, "server")
	if unavailable.Diagnostic == nil || unavailable.Diagnostic.Stage != nativeMCPDiagnosticResolve ||
		unavailable.Diagnostic.Kind != nativeMCPDiagnosticExecutableUnavailable {
		t.Fatalf("resolution diagnostic = %#v, want resolve/executable_unavailable", unavailable.Diagnostic)
	}
}

func TestNativeMCPDiagnosticKeepsWaitCauseAndCleanupErrorSeparate(t *testing.T) {
	cleanupErr := errors.New("cleanup token=cleanup-secret")
	exitCode := 0
	err := nativeMCPWrapCommandError(nativeMCPDiagnosticWait, nativeMCPDiagnosticOutputWaitTimeout, exec.ErrWaitDelay, cleanupErr, &exitCode)
	if !errors.Is(err, exec.ErrWaitDelay) {
		t.Fatalf("wrapped error = %v, want primary WaitDelay cause", err)
	}
	var commandErr *nativeMCPCommandError
	if !errors.As(err, &commandErr) {
		t.Fatalf("wrapped error %T lost native command error type", err)
	}
	diagnostic := sanitizeNativeMCPDiagnostic(commandErr.diagnostic, nativeMCPDiagnosticListTools)
	if diagnostic == nil || !strings.Contains(diagnostic.Message, "WaitDelay") ||
		!strings.Contains(diagnostic.CleanupMessage, "cleanup token: ***") {
		t.Fatalf("wait and cleanup diagnostics were not retained separately: %#v", diagnostic)
	}
	if diagnostic.ExitCode == nil || *diagnostic.ExitCode != 0 {
		t.Fatalf("diagnostic exit code = %v, want observed zero", diagnostic.ExitCode)
	}
}

func TestNativeMCPDiagnosticClassifiesCanceledAndTimedOutCommands(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		kind NativeMCPDiagnosticKind
	}{
		{name: "canceled", err: context.Canceled, kind: nativeMCPDiagnosticCanceled},
		{name: "timeout", err: context.DeadlineExceeded, kind: nativeMCPDiagnosticTimeout},
	} {
		t.Run(test.name, func(t *testing.T) {
			readiness := (CursorNativeMCPAdapter{
				Executable: "cursor-agent",
				Runner:     oneShotNativeMCPRunner{err: test.err},
			}).Enable(context.Background(), "/workspace", nil, "server")
			if readiness.Diagnostic == nil || readiness.Diagnostic.Stage != nativeMCPDiagnosticWait || readiness.Diagnostic.Kind != test.kind {
				t.Fatalf("readiness diagnostic = %#v, want wait/%s", readiness.Diagnostic, test.kind)
			}
		})
	}
}

func TestNativeMCPDiagnosticRedactsAndBoundsCauses(t *testing.T) {
	cause := errors.New("native helper failed token=credential-secret at https://private.example/private/path?key=query-secret /home/alice/private.txt \x1b[31mFAILED\x1b[0m\x00" + strings.Repeat("界", 500) + string([]byte{0xff}))
	readiness := (CursorNativeMCPAdapter{
		Executable: "cursor-agent",
		Runner:     oneShotNativeMCPRunner{err: cause},
	}).Verify(context.Background(), "/workspace", nil, "server")
	if readiness.Diagnostic == nil {
		t.Fatalf("Verify() = %#v, want retained diagnostic", readiness)
	}
	message := readiness.Diagnostic.Message
	if strings.Contains(message, "credential-secret") || strings.Contains(message, "query-secret") ||
		strings.Contains(message, "private.example") || strings.Contains(message, "/private/path") || strings.Contains(message, "/home/alice") {
		t.Fatalf("diagnostic retained private content: %q", message)
	}
	if !strings.Contains(message, "[url-redacted]") || !strings.Contains(message, "FAILED") || strings.Contains(message, "[31m") {
		t.Fatalf("diagnostic did not redact URLs and terminal controls: %q", message)
	}
	if len(message) > nativeMCPDiagnosticMessageLimit || !utf8.ValidString(message) {
		t.Fatalf("diagnostic is not valid bounded UTF-8: bytes=%d message=%q", len(message), message)
	}
}

func TestNativeMCPDiagnosticUsesFixedMessagesForExitAndOutputFailures(t *testing.T) {
	tests := []struct {
		name       string
		result     NativeMCPCommandResult
		wantStage  NativeMCPDiagnosticStage
		wantKind   NativeMCPDiagnosticKind
		wantText   string
		wantHidden string
	}{
		{name: "nonzero exit", result: NativeMCPCommandResult{ExitCode: 7, ExitCodeObserved: true, Stderr: []byte("private provider response")}, wantStage: nativeMCPDiagnosticWait, wantKind: nativeMCPDiagnosticExitStatus, wantText: "status 7", wantHidden: "private provider response"},
		{name: "truncated verification", result: NativeMCPCommandResult{ExitCode: 0, ExitCodeObserved: true, Stdout: []byte("partial private output"), Truncated: true}, wantStage: nativeMCPDiagnosticOutput, wantKind: nativeMCPDiagnosticOutputTruncated, wantText: "size limit", wantHidden: "partial private output"},
		{name: "unrecognized verification", result: NativeMCPCommandResult{ExitCode: 0, ExitCodeObserved: true, Stdout: []byte("private unrecognized output")}, wantStage: nativeMCPDiagnosticOutput, wantKind: nativeMCPDiagnosticUnrecognizedOutput, wantText: "could not be recognized", wantHidden: "private unrecognized output"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			readiness := (CursorNativeMCPAdapter{
				Executable: "cursor-agent",
				Runner:     oneShotNativeMCPRunner{result: tt.result},
			}).Verify(context.Background(), "/workspace", nil, "server")
			if readiness.Diagnostic == nil {
				t.Fatalf("Verify() = %#v, want retained diagnostic", readiness)
			}
			got := readiness.Diagnostic
			if got.Stage != tt.wantStage || got.Kind != tt.wantKind || !strings.Contains(got.Message, tt.wantText) {
				t.Fatalf("diagnostic = %#v, want stage %q kind %q and message containing %q", got, tt.wantStage, tt.wantKind, tt.wantText)
			}
			if got.ExitCode == nil || *got.ExitCode != tt.result.ExitCode {
				t.Fatalf("diagnostic exit code = %v, want observed %d", got.ExitCode, tt.result.ExitCode)
			}
			if strings.Contains(got.Message, tt.wantHidden) {
				t.Fatalf("diagnostic contains native output: %q", got.Message)
			}
		})
	}
}

func TestNativeMCPDiagnosticNormalizesInjectedRunnerMetadata(t *testing.T) {
	readiness := (CursorNativeMCPAdapter{
		Executable: "cursor-agent",
		Runner: oneShotNativeMCPRunner{result: NativeMCPCommandResult{
			ExitCode: 1,
			Diagnostic: &NativeMCPDiagnostic{
				Operation:      nativeMCPDiagnosticListTools,
				Stage:          nativeMCPDiagnosticWait,
				Kind:           nativeMCPDiagnosticWaitFailed,
				Message:        "failed token=runner-secret https://private.example/account/path",
				CleanupMessage: "cleanup token=cleanup-secret /home/alice/private.txt",
			},
		}},
	}).Verify(context.Background(), "/workspace", nil, "server")
	if readiness.Diagnostic == nil {
		t.Fatalf("Verify() = %#v, want normalized injected diagnostic", readiness)
	}
	encoded, err := json.Marshal(readiness)
	if err != nil {
		t.Fatalf("marshal readiness: %v", err)
	}
	for _, private := range []string{"runner-secret", "cleanup-secret", "private.example", "/account/path", "/home/alice"} {
		if strings.Contains(string(encoded), private) {
			t.Fatalf("serialized diagnostic contains %q: %s", private, encoded)
		}
	}
}
