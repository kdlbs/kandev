package docker

import (
	"errors"
	"fmt"
	"strings"
)

// Remote daemon failure causes. Each one needs a different fix on the remote
// host, so they stay distinguishable instead of collapsing into a single
// "daemon unavailable".
var (
	// ErrRemoteSocketAccessDenied means the SSH user reached the host but is
	// not permitted on the Docker socket, normally because it is outside the
	// docker group. This is the most common first-run failure.
	ErrRemoteSocketAccessDenied = errors.New("docker: remote user cannot access the Docker socket")

	// ErrRemoteDockerCLIMissing means the host has no docker command. The
	// SSH transport runs `docker system dial-stdio`, so the CLI is required
	// on the remote, not only the daemon.
	ErrRemoteDockerCLIMissing = errors.New("docker: remote host has no docker command")

	// ErrRemoteDaemonUnreachable means the CLI ran but could not reach a
	// daemon, and is also the fallback for an unrecognized failure.
	ErrRemoteDaemonUnreachable = errors.New("docker: remote daemon is unreachable")

	// ErrRemoteDaemonAddressScheme rejects a Docker host URL where an SSH
	// target is expected.
	ErrRemoteDaemonAddressScheme = errors.New("docker: remote daemon address must be a host or OpenSSH alias, not a URL")

	// ErrRemoteDaemonAddressEmpty rejects a blank target rather than letting
	// the dialer decide what it meant.
	ErrRemoteDaemonAddressEmpty = errors.New("docker: remote daemon address is required")
)

// shellNotFoundExitCode is the POSIX shell's status for a missing command.
const shellNotFoundExitCode = 127

// ValidateRemoteDaemonAddress checks that a remote Docker profile stores an
// SSH target rather than a Docker host URL.
//
// Rejecting every scheme is deliberate: it excludes tcp:// (an unsecured
// daemon port is remote root) by construction rather than through a denylist
// that a new scheme could slip past.
func ValidateRemoteDaemonAddress(address string) error {
	trimmed := strings.TrimSpace(address)
	if trimmed == "" {
		return ErrRemoteDaemonAddressEmpty
	}
	if strings.Contains(trimmed, "://") {
		return fmt.Errorf("%w: %q", ErrRemoteDaemonAddressScheme, trimmed)
	}
	return nil
}

// ClassifyRemoteDialError maps the result of `docker system dial-stdio` on the
// remote to a typed cause, preserving the remote's own message for diagnosis.
// A zero exit code reports no error.
func ClassifyRemoteDialError(exitCode int, stderr string) error {
	if exitCode == 0 {
		return nil
	}

	detail := strings.TrimSpace(stderr)
	lowered := strings.ToLower(detail)

	switch {
	case strings.Contains(lowered, "permission denied"):
		return wrapRemoteDialCause(ErrRemoteSocketAccessDenied, detail)
	case exitCode == shellNotFoundExitCode, strings.Contains(lowered, "command not found"):
		return wrapRemoteDialCause(ErrRemoteDockerCLIMissing, detail)
	default:
		return wrapRemoteDialCause(ErrRemoteDaemonUnreachable, detail)
	}
}

func wrapRemoteDialCause(cause error, detail string) error {
	if detail == "" {
		return cause
	}
	return fmt.Errorf("%w: %s", cause, detail)
}
