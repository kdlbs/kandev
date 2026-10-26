package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/sessionmcp"
	"github.com/kandev/kandev/internal/secrets"
)

// MetadataKeyMCPTokenNonce is the execution metadata key holding the nonce the
// instance MCP token was derived from.
const MetadataKeyMCPTokenNonce = "mcp_token_nonce"

// MetadataKeyBrokerFingerprint holds the fingerprint of the broker upstream an
// instance was created to relay to; empty when it relays nothing.
const MetadataKeyBrokerFingerprint = "mcp_broker_fingerprint"

// Launch refusal codes surfaced at the start of the launch failure text.
const (
	CodeMCPTokenCarrierUnavailable = "mcp_token_carrier_unavailable"
	CodeMCPTokenUnsupported        = "mcp_token_unsupported"
	CodeMCPTokenKeyUnavailable     = "mcp_token_key_unavailable"
	CodeMCPTokenKeyReplaced        = "mcp_token_key_replaced"
)

// Launch refusal causes (the closed set of cause= values).
const (
	CauseRetainedInstanceDeferred     = "retained_instance_deferred"
	CauseAgentNotVerified             = "agent_not_verified"
	CauseWorkspaceFileCarrierDeferred = "workspace_file_carrier_deferred"
	CauseCustomTUIDeferred            = "custom_tui_deferred"
	CauseCodexWrapper                 = "codex_wrapper"
	CauseCodexVersion                 = "codex_version"
	CauseCodexProbeFailed             = "codex_probe_failed"
)

var refusalSentences = map[string]string{
	CodeMCPTokenCarrierUnavailable: "This agent cannot receive the per-session MCP token, so the session was not started.",
	CodeMCPTokenUnsupported:        "The agent runtime does not support per-session MCP tokens, so the session was not started.",
	CodeMCPTokenKeyUnavailable:     "The MCP token signing key could not be read, so the session was not started.",
}

// LaunchRefusal is a typed launch failure. Its text starts with the code.
type LaunchRefusal struct {
	Code  string
	Cause string
	Err   error
}

func (r *LaunchRefusal) Error() string {
	msg := r.Code + ": " + refusalSentences[r.Code]
	if r.Cause != "" {
		msg += " cause=" + r.Cause
	}
	return msg
}

func (r *LaunchRefusal) Unwrap() error { return r.Err }

const (
	createConflictAttempts = 3
	createConflictGap      = time.Second
	createIdentityTimeout  = 5 * time.Second
	createIdentityAttempts = 3
)

// createIdentityGap separates identity reads of one create.
var createIdentityGap = time.Second

// SetSessionMCPPolicy installs the policy the backend composition derived from
// the runtime flag. The zero policy is disabled.
func (m *Manager) SetSessionMCPPolicy(policy SessionMCPPolicy) {
	m.sessionMCPPolicy = policy
}

// sessionKeyStore returns the signing-key store behind the runtime secret
// store while the policy is enabled. A disabled policy reports no store, which
// every launch and carrier path treats as "no per-session token".
func (m *Manager) sessionKeyStore() (secrets.SessionKeyStore, bool) {
	if !m.sessionMCPPolicy.Enabled() {
		return nil, false
	}
	return m.rawSessionKeyStore()
}

// rawSessionKeyStore returns the signing-key store regardless of the policy.
func (m *Manager) rawSessionKeyStore() (secrets.SessionKeyStore, bool) {
	keys, ok := m.secretStore.(secrets.SessionKeyStore)
	return keys, ok
}

// EnsureSessionMCPKey creates or repairs the signing key. The backend calls it
// at startup before recovery so adoption only ever reads the key.
func (m *Manager) EnsureSessionMCPKey(ctx context.Context) error {
	keys, ok := m.sessionKeyStore()
	if !ok {
		return nil
	}
	_, replaced, err := keys.EnsureSessionMCPKey(ctx)
	if err != nil {
		return err
	}
	if replaced {
		m.logger.Warn(CodeMCPTokenKeyReplaced + ": stored signing key was unusable and was replaced; running sessions relaunch")
	}
	return nil
}

// createInstanceWithSessionMCP creates the instance with a freshly minted
// token and verifies, before the agent starts, that the instance holds it.
func (m *Manager) createInstanceWithSessionMCP(ctx context.Context, rt ExecutorBackend, req *ExecutorCreateRequest) (*ExecutorInstance, error) {
	keys, ok := m.sessionKeyStore()
	if !ok {
		return rt.CreateInstance(ctx, req)
	}
	key, replaced, err := keys.EnsureSessionMCPKey(ctx)
	if err != nil {
		m.logger.Error(CodeMCPTokenKeyUnavailable, zap.Error(err))
		return nil, &LaunchRefusal{Code: CodeMCPTokenKeyUnavailable, Err: err}
	}
	if replaced {
		m.logger.Warn(CodeMCPTokenKeyReplaced + ": stored signing key was unusable and was replaced")
	}
	var lastErr error
	for attempt := 0; attempt < createConflictAttempts; attempt++ {
		if attempt > 0 && !sleepCtx(ctx, createConflictGap) {
			return nil, ctx.Err()
		}
		inst, retry, err := m.createOnce(ctx, rt, req, key)
		if err == nil {
			return inst, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, lastErr
}

// createOnce makes one create attempt with a fresh nonce. retry reports a
// transient conflict worth another attempt with a new nonce.
func (m *Manager) createOnce(ctx context.Context, rt ExecutorBackend, req *ExecutorCreateRequest, key []byte) (*ExecutorInstance, bool, error) {
	nonce, err := sessionmcp.NewNonce()
	if err != nil {
		return nil, false, &LaunchRefusal{Code: CodeMCPTokenKeyUnavailable, Err: err}
	}
	token := sessionmcp.Token(key, req.InstanceID, nonce)
	minted := sessionmcp.Fingerprint(token)
	req.SessionMCPToken, req.SessionMCPNonce = token, nonce
	if req.Metadata == nil {
		req.Metadata = make(map[string]interface{})
	}
	req.Metadata[MetadataKeyMCPTokenNonce] = nonce
	brokerFP := sessionmcp.BrokerFingerprint(req.BrokerUpstreamURL)
	req.Metadata[MetadataKeyBrokerFingerprint] = brokerFP

	inst, err := rt.CreateInstance(ctx, req)
	if err != nil {
		var conflict *agentctl.CreateConflictError
		return nil, errors.As(err, &conflict), err
	}
	if inst.Metadata == nil {
		inst.Metadata = make(map[string]interface{})
	}
	inst.Metadata[MetadataKeyMCPTokenNonce] = nonce
	inst.Metadata[MetadataKeyBrokerFingerprint] = brokerFP
	if inst.Client == nil {
		return inst, false, nil
	}
	// A runtime that reconnected to a retained process returned an instance
	// this launch did not create: it is refused, never stopped.
	retained := reconnectedInstance(req, inst)
	if retained && deferredTokenRuntime(inst.RuntimeName) {
		return nil, false, &LaunchRefusal{Code: CodeMCPTokenUnsupported, Cause: CauseRetainedInstanceDeferred, Err: errRetainedTokenUnverified}
	}
	reader := tokenVerifier{
		Identity: inst.Client.GetInstanceIdentity,
		Timeout:  createIdentityTimeout,
		Attempts: createIdentityAttempts,
		Gap:      createIdentityGap,
	}
	identity, err := reader.readIdentity(ctx)
	switch {
	case err != nil || !identity.HasCapability(agentctlSessionMCPCapability):
		if retained {
			return nil, false, &LaunchRefusal{Code: CodeMCPTokenUnsupported, Err: errRetainedTokenUnverified}
		}
		m.rollbackCreate(ctx, rt, inst, minted)
		return nil, false, &LaunchRefusal{Code: CodeMCPTokenUnsupported, Err: err}
	case identity.BrokerUpstreamFingerprint != brokerFP:
		m.rollbackCreate(ctx, rt, inst, identity.MCPTokenFingerprint)
		return nil, false, fmt.Errorf("instance %s relays a different broker after create", req.InstanceID)
	case identity.MCPTokenFingerprint != minted:
		m.rollbackCreate(ctx, rt, inst, identity.MCPTokenFingerprint)
		return nil, true, fmt.Errorf("instance %s holds a different token after create", req.InstanceID)
	}
	return inst, false, nil
}

// reconnectedInstance reports whether the runtime returned an instance that
// already existed. A Kubernetes reconnect marks nothing, but it keeps the
// recorded Pod identity of the request; a recreated Pod carries a new one.
func reconnectedInstance(req *ExecutorCreateRequest, inst *ExecutorInstance) bool {
	if getMetadataBool(inst.Metadata, MetadataKeyReuseExistingProcess) {
		return true
	}
	if inst.RuntimeName != agentruntime.RuntimeKubernetes {
		return false
	}
	recorded := getMetadataString(req.Metadata, MetadataKeyKubernetesPodUID)
	return recorded != "" && recorded == getMetadataString(inst.Metadata, MetadataKeyKubernetesPodUID)
}

// agentctlSessionMCPCapability is advertised by instances that enforce the token.
const agentctlSessionMCPCapability = "session_mcp_token"

func (m *Manager) rollbackCreate(ctx context.Context, rt ExecutorBackend, inst *ExecutorInstance, fingerprint string) {
	stopCtx := agentctl.WithExpectedTokenFingerprint(context.WithoutCancel(ctx), fingerprint)
	if err := rt.StopInstance(stopCtx, inst, true); err != nil {
		m.logger.Warn("rollback of instance with unverified mcp token failed",
			zap.String("instance_id", inst.InstanceID), zap.Error(err))
	}
}
