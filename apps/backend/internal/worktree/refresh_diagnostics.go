package worktree

import (
	"context"
	"errors"
	"os/exec"
	"strings"

	"go.uber.org/zap"
)

const (
	refreshDiagnosticAuthentication = "authentication_failed"
	refreshDiagnosticSSHKey         = "ssh_public_key_rejected"
	refreshDiagnosticSSHHost        = "ssh_host_verification_failed"
	refreshDiagnosticDNS            = "dns_resolution_failed"
	refreshDiagnosticConnection     = "connection_failed"
	refreshDiagnosticTLS            = "tls_verification_failed"
	refreshDiagnosticMissingRef     = "missing_remote_ref"
	refreshDiagnosticNonFastForward = "non_fast_forward"
	refreshDiagnosticRepository     = "repository_access_failed"
	refreshDiagnosticLock           = "lock_contention"
	refreshDiagnosticTimeout        = "timeout"
	refreshDiagnosticCanceled       = "canceled"
	refreshDiagnosticUnknown        = "unknown"
)

type refreshDiagnostic struct {
	code        string
	detail      string
	exitCode    int
	hasExitCode bool
}

func classifyRefreshDiagnostic(output string, runErr, contextErr error) refreshDiagnostic {
	if errors.Is(contextErr, context.Canceled) || errors.Is(runErr, context.Canceled) {
		return newRefreshDiagnostic(refreshDiagnosticCanceled, "The Git refresh was canceled.", runErr)
	}
	if errors.Is(contextErr, context.DeadlineExceeded) || errors.Is(runErr, context.DeadlineExceeded) {
		return newRefreshDiagnostic(refreshDiagnosticTimeout, "The Git refresh timed out.", runErr)
	}

	code, detail := classifyRefreshDiagnosticOutput(output)
	return newRefreshDiagnostic(code, detail, runErr)
}

func classifyRefreshDiagnosticOutput(output string) (string, string) {
	lower := strings.ToLower(output)
	switch {
	case strings.Contains(lower, "permission denied (publickey)"),
		strings.Contains(lower, "no supported authentication methods available"):
		return refreshDiagnosticSSHKey, "The SSH server rejected the configured public key."
	case strings.Contains(lower, "host key verification failed"),
		strings.Contains(lower, "remote host identification has changed"):
		return refreshDiagnosticSSHHost, "The SSH host could not be verified."
	case strings.Contains(lower, "could not resolve host"),
		strings.Contains(lower, "could not resolve hostname"),
		strings.Contains(lower, "name or service not known"),
		strings.Contains(lower, "temporary failure in name resolution"):
		return refreshDiagnosticDNS, "The remote host name could not be resolved."
	case strings.Contains(lower, "ssl certificate problem"),
		strings.Contains(lower, "certificate verify failed"),
		strings.Contains(lower, "server certificate verification failed"):
		return refreshDiagnosticTLS, "The remote TLS certificate could not be verified."
	case strings.Contains(lower, "connection refused"),
		strings.Contains(lower, "connection reset"),
		strings.Contains(lower, "network is unreachable"),
		strings.Contains(lower, "no route to host"),
		strings.Contains(lower, "failed to connect"),
		strings.Contains(lower, "couldn't connect"):
		return refreshDiagnosticConnection, "The connection to the Git remote failed."
	case strings.Contains(lower, "couldn't find remote ref"),
		strings.Contains(lower, "could not find remote ref"),
		strings.Contains(lower, "remote ref does not exist"),
		strings.Contains(lower, "no such ref"):
		return refreshDiagnosticMissingRef, "The requested remote branch was not found."
	case strings.Contains(lower, "not possible to fast-forward"):
		return refreshDiagnosticNonFastForward, "The local branch could not be fast-forwarded."
	case strings.Contains(lower, "non-fast-forward"):
		return refreshDiagnosticNonFastForward, "The Git refresh encountered a non-fast-forward update."
	case strings.Contains(lower, "index.lock"),
		strings.Contains(lower, "cannot lock ref"),
		strings.Contains(lower, "another git process seems to be running"),
		// Require both fragments so a generic "unable to create" error stays unknown.
		strings.Contains(lower, "unable to create") && strings.Contains(lower, "lock"):
		return refreshDiagnosticLock, "Git could not acquire the repository lock."
	case strings.Contains(lower, "repository not found"),
		strings.Contains(lower, "does not appear to be a git repository"),
		strings.Contains(lower, "could not read from remote repository"):
		return refreshDiagnosticRepository, "The Git repository could not be accessed."
	case strings.Contains(lower, "could not read username"),
		strings.Contains(lower, "authentication failed"),
		strings.Contains(lower, "terminal prompts disabled"),
		strings.Contains(lower, "authentication required"):
		return refreshDiagnosticAuthentication, "Git could not authenticate to the remote."
	default:
		return refreshDiagnosticUnknown, "The Git refresh failed for an unknown reason."
	}
}

func newRefreshDiagnostic(code, detail string, runErr error) refreshDiagnostic {
	diagnostic := refreshDiagnostic{code: code, detail: detail}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) && exitErr.ExitCode() >= 0 {
		diagnostic.exitCode = exitErr.ExitCode()
		diagnostic.hasExitCode = true
	}
	return diagnostic
}

func (m *Manager) logRefreshDiagnostic(
	operation, repoPath, branch, reason string, output []byte, runErr, contextErr error,
) {
	diagnostic := classifyRefreshDiagnostic(string(output), runErr, contextErr)
	fields := []zap.Field{
		zap.String("operation", operation),
		zap.String("repository_path", repoPath),
		zap.String("branch", branch),
		zap.String("reason", reason),
		zap.String("diagnostic_code", diagnostic.code),
		zap.String("detail", diagnostic.detail),
	}
	if diagnostic.hasExitCode {
		fields = append(fields, zap.Int("exit_code", diagnostic.exitCode))
	}
	m.logger.Warn("git refresh failed", fields...)
}
