package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh/knownhosts"
)

// SSHReachabilityReason classifies why a probe or launch-time SSH dial
// failed. The set is closed: ClassifyDialError always returns one of these
// six values, walking a total order so a deadline is never reported as a
// network failure and a rejected host key is never reported as an
// authentication failure.
type SSHReachabilityReason string

const (
	SSHReachabilityReasonConfig  SSHReachabilityReason = "config"
	SSHReachabilityReasonTimeout SSHReachabilityReason = "timeout"
	SSHReachabilityReasonHostKey SSHReachabilityReason = "host_key"
	SSHReachabilityReasonAuth    SSHReachabilityReason = "auth"
	SSHReachabilityReasonNetwork SSHReachabilityReason = "network"
	SSHReachabilityReasonUnknown SSHReachabilityReason = "unknown"
)

// SSHProbeOutcome is the result of a single reachability probe, or of a
// launch's own dial attempt classified through the same rules. Host always
// names the target executor's host, even when a failure occurred on a
// bastion hop. Cancelled means the caller's context ended the attempt before
// it produced an observation about the host; callers must discard a
// cancelled outcome rather than recording it as a failure.
type SSHProbeOutcome struct {
	Success   bool
	Cancelled bool
	Host      string
	Reason    SSHReachabilityReason
	Message   string
}

// ProbeSSHHost opens an authenticated SSH transport to target using its
// pinned fingerprint, closes it without running a remote command, and
// returns a typed outcome. It bounds both the TCP dial and the SSH handshake
// by timeout, and never dials when the target has no pinned fingerprint to
// verify against.
func ProbeSSHHost(ctx context.Context, target *SSHTarget, timeout time.Duration) SSHProbeOutcome {
	outcome := SSHProbeOutcome{Host: target.Host}
	if strings.TrimSpace(target.PinnedFingerprint) == "" {
		outcome.Reason = SSHReachabilityReasonConfig
		outcome.Message = "no pinned host key fingerprint is configured"
		return outcome
	}

	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client, err := dialSSH(probeCtx, target)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			outcome.Cancelled = true
			return outcome
		}
		outcome.Reason = ClassifyDialError(err)
		outcome.Message = err.Error()
		return outcome
	}
	defer func() { _ = client.Close() }()

	outcome.Success = true
	return outcome
}

// ClassifyDialError maps a non-nil SSH dial error to exactly one reason from
// the closed set, walking timeout, host_key, auth, network, unknown in that
// order — first match wins. context.Canceled is deliberately not classified
// here: it is not an observation about the host, and callers check for it
// (see ProbeSSHHost) before reaching this function.
func ClassifyDialError(err error) SSHReachabilityReason {
	switch {
	case isSSHTimeoutError(err):
		return SSHReachabilityReasonTimeout
	case isSSHHostKeyMismatch(err):
		return SSHReachabilityReasonHostKey
	case isSSHAuthFailure(err):
		return SSHReachabilityReasonAuth
	case isSSHNetworkError(err):
		return SSHReachabilityReasonNetwork
	default:
		return SSHReachabilityReasonUnknown
	}
}

func isSSHTimeoutError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

// isSSHHostKeyMismatch confines the host_key discriminator to Kandev's own
// errHostKeyMismatch (the pinned-fingerprint check) and knownhosts.KeyError
// with a non-empty Want (a genuine mismatch, not an unknown-host TOFU
// accept, which returns nil and never reaches this function) — the bastion
// host-key check. golang.org/x/crypto/ssh has no stable exported error type
// for a rejected key; keying off our own types keeps a library upgrade from
// turning a security event into auth or unknown.
func isSSHHostKeyMismatch(err error) bool {
	var mismatch *errHostKeyMismatch
	if errors.As(err, &mismatch) {
		return true
	}
	var keyErr *knownhosts.KeyError
	return errors.As(err, &keyErr) && len(keyErr.Want) > 0
}

// isSSHAuthFailure relies on string-matching because
// golang.org/x/crypto/ssh v0.52.0 has no exported error type for an
// authentication failure — it returns a plain fmt.Errorf.
func isSSHAuthFailure(err error) bool {
	return strings.Contains(err.Error(), "unable to authenticate")
}

func isSSHNetworkError(err error) bool {
	var opErr *net.OpError
	return errors.As(err, &opErr)
}

// SSHTargetFromExecutorConfig projects an SSH executor's persisted config map
// into a resolved SSHTarget, applying the same ~/.ssh/config inheritance as
// ResolveSSHTarget. A missing host or fingerprint fails resolution here,
// which callers should classify as SSHReachabilityReasonConfig without
// attempting a dial.
func SSHTargetFromExecutorConfig(cfg map[string]string) (*SSHTarget, error) {
	if cfg == nil {
		return nil, fmt.Errorf("ssh executor has no config")
	}
	port := 0
	if p := strings.TrimSpace(cfg["ssh_port"]); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return nil, fmt.Errorf("invalid ssh_port %q", p)
		}
		port = n
	}
	return ResolveSSHTarget(SSHConnConfig{
		HostAlias:         cfg["ssh_host_alias"],
		Host:              cfg["ssh_host"],
		Port:              port,
		User:              cfg["ssh_user"],
		IdentitySource:    SSHIdentitySource(cfg["ssh_identity_source"]),
		IdentityFile:      cfg["ssh_identity_file"],
		ProxyJump:         cfg["ssh_proxy_jump"],
		PinnedFingerprint: cfg["ssh_host_fingerprint"],
	})
}
