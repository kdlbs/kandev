package lifecycle

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/ownershipproof"
)

// newAdoptionChallenge returns a fresh challenge for one adoption attempt.
// A proof is only ever accepted for the challenge generated for that
// attempt, so this must never be derived from anything an observer could
// predict or replay.
func newAdoptionChallenge() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// controlServerHoldsCredential reports whether the process answering the
// recorded endpoint can demonstrate it already holds credential. Until it
// does, this backend has established only that something answers and echoes
// the values the identity endpoint hands out unauthenticated; sending the
// credential to it, or storing a replacement it invents, would hand an
// installation-wide secret to whatever took the port.
//
// A failure to answer is not a proof: the caller refuses adoption.
func controlServerHoldsCredential(
	ctx context.Context,
	client AdoptionControlClient,
	credential string,
	binding string,
	recoveryReadTimeout time.Duration,
	recoveryReadRetries int,
) bool {
	challenge, err := newAdoptionChallenge()
	if err != nil {
		return false
	}

	proofs, err := withAdoptionRetry(ctx, recoveryReadTimeout, recoveryReadRetries,
		func(ctx context.Context) ([]string, error) {
			return client.ProveOwnership(ctx, challenge)
		})
	if err != nil {
		return false
	}

	return ownershipproof.Matches(credential, challenge, binding, proofs)
}

// recordedDiagnosticLogPath reads the adopted server's diagnostic log
// location, which AC-EXECUTORS-CONTROL-OWNERSHIP-001.1 requires the record
// to name. It is retrieved after authentication because the endpoint that
// decides whether to authenticate discloses no filesystem path.
func recordedDiagnosticLogPath(
	ctx context.Context,
	client AdoptionControlClient,
	recoveryReadTimeout time.Duration,
	recoveryReadRetries int,
) string {
	details, err := withAdoptionRetry(ctx, recoveryReadTimeout, recoveryReadRetries,
		func(ctx context.Context) (*agentctl.ServerDetails, error) {
			return client.GetServerDetails(ctx)
		})
	if err != nil || details == nil {
		return ""
	}
	return details.DiagnosticLogPath
}
