package lifecycle

import (
	"context"
	"sync"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/task/models"
)

const (
	standaloneTokenParallelism = 8
	standaloneTokenTimeout     = 2 * time.Second
	standaloneTokenAttempts    = 2
	standaloneTokenGap         = 500 * time.Millisecond
	tokenRevokeAttempts        = 3
	tokenRevokeGap             = time.Second
)

// SetSessionKeyReader supplies the read-only signing-key accessor adoption
// checks use. The key is never created or replaced during recovery.
func (r *StandaloneExecutor) SetSessionKeyReader(read func() ([]byte, error)) {
	r.readSessionKey = read
}

// verifyWinnerTokens checks each recovered winner holds the token derived from
// its persisted nonce. A winner that fails is removed from winners and reported
// through the returned outcomes: a deleted or vanished instance is no longer
// matching, an unestablished one is unknown.
func (r *StandaloneExecutor) verifyWinnerTokens(
	ctx context.Context,
	control *agentctl.ControlClient,
	lease *agentctl.RuntimeLease,
	winners map[string]*agentctl.InstanceInfo,
	recordBySession map[string]*models.ExecutorRunning,
) map[string]RecoveryCandidateOutcome {
	if r.readSessionKey == nil || len(winners) == 0 {
		return nil
	}
	var (
		mu       sync.Mutex
		wg       sync.WaitGroup
		outcomes = make(map[string]RecoveryCandidateOutcome)
		sem      = make(chan struct{}, standaloneTokenParallelism)
	)
	for sessionID, inst := range winners {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			verdict := r.tokenVerifierFor(control, lease, sessionID, inst, recordBySession[sessionID]).Verify(ctx)
			if verdict == verdictAdopt {
				return
			}
			outcome := RecoveryOutcomeNoMatchingInstance
			if verdict == verdictUnknown {
				outcome = RecoveryOutcomeUnknown
			}
			mu.Lock()
			outcomes[sessionID] = outcome
			mu.Unlock()
		}()
	}
	wg.Wait()
	for sessionID := range outcomes {
		delete(winners, sessionID)
	}
	return outcomes
}

func (r *StandaloneExecutor) tokenVerifierFor(
	control *agentctl.ControlClient,
	lease *agentctl.RuntimeLease,
	sessionID string,
	inst *agentctl.InstanceInfo,
	record *models.ExecutorRunning,
) tokenVerifier {
	var client *agentctl.Client
	if lease != nil {
		client = lease.NewBoundInstanceClient(inst.Port, r.logger,
			agentctl.WithExecutionID(inst.ID), agentctl.WithSessionID(sessionID))
	} else {
		client = agentctl.NewClient(r.host, inst.Port, r.logger,
			agentctl.WithExecutionID(inst.ID), agentctl.WithSessionID(sessionID),
			agentctl.WithAuthToken(r.authToken))
	}
	var nonce string
	if record != nil {
		nonce = getMetadataString(record.Metadata, MetadataKeyMCPTokenNonce)
	}
	v := tokenVerifier{
		InstanceID: inst.ID,
		Nonce:      nonce,
		ReadKey:    r.readSessionKey,
		Identity:   client.GetInstanceIdentity,
		Log:        r.logger,
		Timeout:    standaloneTokenTimeout,
		Attempts:   standaloneTokenAttempts,
		Gap:        standaloneTokenGap,
		Revokes:    tokenRevokeAttempts,
		RevokeGap:  tokenRevokeGap,
	}
	if control != nil {
		v.Control = control
	}
	return v
}
