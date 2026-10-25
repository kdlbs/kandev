package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

const (
	remoteTokenTimeout  = 5 * time.Second
	remoteTokenAttempts = 3
	remoteTokenGap      = time.Second
)

// errRetainedTokenUnverified reports that a retained remote instance was not
// adopted because its session token could not be confirmed.
var errRetainedTokenUnverified = errors.New("retained instance session token not verified")

// deferredTokenRuntime reports runtimes whose retained instances cannot be
// token-verified; they refuse reuse rather than adopt an unverified instance.
func deferredTokenRuntime(runtime agentruntime.Runtime) bool {
	return runtime == agentruntime.RuntimeKubernetes || runtime == agentruntime.RuntimeSSH
}

// remoteTokenControl deletes through the owning runtime and revokes through the
// instance's control handle when the runtime exposes one.
type remoteTokenControl struct {
	m  *Manager
	ri *ExecutorInstance
}

func (c remoteTokenControl) DeleteInstanceMatching(ctx context.Context, _, fingerprint string) error {
	if c.ri.tokenControl != nil {
		return c.ri.tokenControl.DeleteInstanceMatching(ctx, c.ri.InstanceID, fingerprint)
	}
	rt, err := c.m.backendForRuntime(c.ri.RuntimeName)
	if err != nil {
		return err
	}
	return rt.StopInstance(agentctl.WithExpectedTokenFingerprint(ctx, fingerprint), c.ri, true)
}

func (c remoteTokenControl) RevokeMCPToken(ctx context.Context, instanceID string) error {
	if c.ri.tokenControl == nil {
		return errors.New("runtime exposes no token revoke handle")
	}
	return c.ri.tokenControl.RevokeMCPToken(ctx, instanceID)
}

// verifyRetainedSessionToken adopts a retained remote instance only while it
// still holds the token derived from the nonce persisted for it.
func (m *Manager) verifyRetainedSessionToken(ctx context.Context, record *models.ExecutorRunning, ri *ExecutorInstance) error {
	keys, ok := m.sessionKeyStore()
	if !ok {
		return nil
	}
	control := remoteTokenControl{m: m, ri: ri}
	if deferredTokenRuntime(ri.RuntimeName) {
		if ri.tokenControl != nil {
			revokeUnverified(ctx, ri.tokenControl, ri.InstanceID)
		}
		return &LaunchRefusal{Code: CodeMCPTokenUnsupported, Cause: CauseRetainedInstanceDeferred, Err: errRetainedTokenUnverified}
	}
	v := tokenVerifier{
		InstanceID:        ri.InstanceID,
		Nonce:             getMetadataString(record.Metadata, MetadataKeyMCPTokenNonce),
		BrokerFingerprint: getMetadataString(record.Metadata, MetadataKeyBrokerFingerprint),
		ReadKey:           func() ([]byte, error) { return keys.SessionMCPKey(ctx) },
		Control:           control,
		Log:               m.logger,
		Timeout:           remoteTokenTimeout,
		Attempts:          remoteTokenAttempts,
		Gap:               remoteTokenGap,
		Revokes:           tokenRevokeAttempts,
		RevokeGap:         tokenRevokeGap,
	}
	if ri.Client == nil {
		return fmt.Errorf("%w: no instance client", errRetainedTokenUnverified)
	}
	v.Identity = ri.Client.GetInstanceIdentity
	if verdict := v.Verify(ctx); verdict != verdictAdopt {
		return fmt.Errorf("%w: %s", errRetainedTokenUnverified, verdictName(verdict))
	}
	return nil
}

func revokeUnverified(ctx context.Context, control mcpTokenControl, instanceID string) {
	for attempt := 0; attempt < tokenRevokeAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(tokenRevokeGap)
		}
		if control.RevokeMCPToken(context.WithoutCancel(ctx), instanceID) == nil {
			return
		}
	}
}

func verdictName(v tokenVerdict) string {
	switch v {
	case verdictAdopt:
		return "adopt"
	case verdictDeleted:
		return "deleted"
	case verdictAbsent:
		return "absent"
	default:
		return string(RecoveryOutcomeUnknown)
	}
}

func (m *Manager) backendForRuntime(runtime agentruntime.Runtime) (ExecutorBackend, error) {
	if m.executorRegistry == nil {
		return nil, errors.New("no executor registry")
	}
	return m.executorRegistry.GetBackend(runtime)
}
