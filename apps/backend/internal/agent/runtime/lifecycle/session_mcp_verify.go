package lifecycle

import (
	"context"
	"errors"
	"net/http"
	"time"

	"go.uber.org/zap"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/sessionmcp"
	"github.com/kandev/kandev/internal/secrets"
)

// tokenVerdict is the result of checking a retained instance's MCP token.
type tokenVerdict int

const (
	// verdictAdopt: the instance holds the token derived from the persisted nonce.
	verdictAdopt tokenVerdict = iota
	// verdictDeleted: the instance held a different or no token and was removed.
	verdictDeleted
	// verdictAbsent: the instance no longer exists.
	verdictAbsent
	// verdictUnknown: the token state could not be established; the token was
	// revoked (best effort) and the instance must be treated as unverified.
	verdictUnknown
)

// mcpTokenControl is the control-server surface the verifier needs.
type mcpTokenControl interface {
	DeleteInstanceMatching(ctx context.Context, instanceID, tokenFingerprint string) error
	RevokeMCPToken(ctx context.Context, instanceID string) error
}

// tokenVerifier checks one retained instance against the backend's key and the
// nonce persisted for it.
type tokenVerifier struct {
	InstanceID string
	Nonce      string
	// BrokerFingerprint is the broker upstream fingerprint recorded at create.
	BrokerFingerprint string
	// ReadKey reads the signing key without creating or replacing it.
	ReadKey  func() ([]byte, error)
	Identity func(ctx context.Context) (*agentctl.InstanceIdentity, error)
	Control  mcpTokenControl
	Log      *logger.Logger

	Timeout   time.Duration
	Attempts  int
	Gap       time.Duration
	Revokes   int
	RevokeGap time.Duration
}

// Verify returns the verdict, deleting a mismatched instance and revoking the
// token of one whose state is unknown.
func (v tokenVerifier) Verify(ctx context.Context) tokenVerdict {
	identity, readErr := v.readIdentity(ctx)
	switch {
	case readErr == nil:
	case agentctl.StatusOf(readErr) == http.StatusNotFound:
		return verdictAbsent
	case errors.Is(readErr, agentctl.ErrIdentityMalformed):
		return v.deleteMismatched(ctx, "")
	default:
		return v.unknown(ctx, "identity_unreadable")
	}

	observed := identity.MCPTokenFingerprint
	key, keyErr := v.ReadKey()
	switch {
	case keyErr == nil:
	case errors.Is(keyErr, secrets.ErrNotFound), errors.Is(keyErr, secrets.ErrSessionKeyUnusable):
		return v.deleteMismatched(ctx, observed)
	default:
		return v.unknown(ctx, "key_read_failed")
	}

	expected := sessionmcp.Fingerprint(sessionmcp.Token(key, v.InstanceID, v.Nonce))
	if v.Nonce != "" && expected != "" && observed == expected && identity.BrokerUpstreamFingerprint == v.BrokerFingerprint {
		return verdictAdopt
	}
	return v.deleteMismatched(ctx, observed)
}

func (v tokenVerifier) readIdentity(ctx context.Context) (*agentctl.InstanceIdentity, error) {
	var err error
	for attempt := 0; attempt < v.Attempts; attempt++ {
		if attempt > 0 && !sleepCtx(ctx, v.Gap) {
			return nil, ctx.Err()
		}
		attemptCtx, cancel := context.WithTimeout(ctx, v.Timeout)
		var identity *agentctl.InstanceIdentity
		identity, err = v.Identity(attemptCtx)
		cancel()
		if err == nil {
			return identity, nil
		}
		if agentctl.StatusOf(err) == http.StatusNotFound || errors.Is(err, agentctl.ErrIdentityMalformed) || ctx.Err() != nil {
			return nil, err
		}
	}
	return nil, err
}

// deleteMismatched removes the instance, conditioned on the fingerprint
// observed (unconditional when none was).
func (v tokenVerifier) deleteMismatched(ctx context.Context, observed string) tokenVerdict {
	if v.Control == nil {
		return v.unknown(ctx, "no_control_client")
	}
	if err := v.Control.DeleteInstanceMatching(context.WithoutCancel(ctx), v.InstanceID, observed); err != nil {
		return v.unknown(ctx, "delete_failed")
	}
	return verdictDeleted
}

func (v tokenVerifier) unknown(ctx context.Context, reason string) tokenVerdict {
	if v.Control == nil {
		return verdictUnknown
	}
	if !revokeMCPToken(ctx, v.Control, v.InstanceID, v.Revokes, v.RevokeGap) && v.Log != nil {
		v.Log.Warn("mcp token revoke failed for unverified instance",
			zap.String("instance_id", v.InstanceID), zap.String("reason", reason))
	}
	return verdictUnknown
}

// revokeAttemptTimeout bounds one revoke request.
const revokeAttemptTimeout = 5 * time.Second

// revokeMCPToken asks the instance to revoke its token, retrying up to
// attempts times with gap between them. It reports whether the instance is no
// longer reachable with its token (revoked, or already absent). The revoke
// must finish even when the caller's context ends, so it runs on a detached
// context bounded by the worst-case duration of all attempts, and the wait
// between attempts is a timer on that context rather than a bare sleep.
func revokeMCPToken(ctx context.Context, control mcpTokenControl, instanceID string, attempts int, gap time.Duration) bool {
	revokeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Duration(attempts)*(gap+revokeAttemptTimeout))
	defer cancel()
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 && !sleepCtx(revokeCtx, gap) {
			return false
		}
		err := control.RevokeMCPToken(revokeCtx, instanceID)
		if err == nil || agentctl.StatusOf(err) == http.StatusNotFound {
			return true
		}
	}
	return false
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
