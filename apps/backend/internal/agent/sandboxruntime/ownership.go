package sandboxruntime

import (
	"context"
	"errors"
	"time"

	policy "github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/kandev/kandev/internal/common/processidentity"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/plugins"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

func (p *Prepared) BindOwner(ctx context.Context, agentctlIdentity string, owner *contract.ProcessOwner) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.validateOwner(owner); err != nil {
		return err
	}
	ownedSupervisor := *owner
	p.owner = &ownedSupervisor
	if err := p.persistSupervisor(ctx, agentctlIdentity, owner); err != nil {
		return err
	}
	identity, err := p.readIdentity(ctx, owner)
	if err != nil {
		return err
	}
	child, err := processidentity.Capture(int(identity.ChildPid))
	if err != nil {
		return policy.ErrConflict
	}
	owned, err := processidentity.IsDescendant(ctx, owner.Supervisor, child)
	if err != nil || !owned {
		return policy.ErrConflict
	}
	contained, err := p.launch.Containment.Contains(child.PID)
	if err != nil || !contained {
		return policy.ErrConflict
	}
	current, err := p.manager.policies.ReadAttempt(ctx, p.orgID, p.launch.AttemptID)
	if err != nil {
		return err
	}
	_, err = p.manager.policies.TransitionAttempt(ctx, p.orgID, current.ID, current.Version, policy.AttemptRunning, &policy.AttemptOwner{AgentctlIdentity: agentctlIdentity, SupervisorPID: owner.Supervisor.PID, SupervisorBirth: owner.Supervisor.BirthToken, ChildPID: child.PID, ChildBirth: child.BirthToken, ProviderSessionID: identity.ProviderSessionId})
	return err
}

func (p *Prepared) readIdentity(ctx context.Context, owner *contract.ProcessOwner) (*pluginsdk.SandboxRuntimeIdentity, error) {
	var matched *pluginsdk.SandboxRuntimeIdentity
	for _, source := range p.description.RuntimeSources {
		if source.Kind != "identity" {
			continue
		}
		records, err := contract.ReadRuntimeSource(ctx, p.files.Directories.State, contract.RuntimeSource{Key: source.Key, Kind: source.Kind, Directory: source.RelativeDirectory, Suffix: source.FilenameSuffix, MaxDepth: source.MaxDepth, MaxFiles: source.MaxFiles, MaxBytes: source.MaxBytes})
		if err != nil {
			return nil, err
		}
		for _, record := range records {
			normalized, err := p.manager.providers.NormalizeSandboxStatus(ctx, p.binding, &pluginsdk.NormalizeSandboxStatusRequest{Context: p.operation(), Policy: p.policy, Process: &pluginsdk.SandboxProcessIdentity{Pid: int64(owner.Supervisor.PID), BirthId: owner.Supervisor.BirthToken}, Evidence: record.Content, EvidenceSchema: source.Key})
			if err != nil || normalized.Identity == nil {
				continue
			}
			if matched != nil {
				return nil, policy.ErrConflict
			}
			matched = normalized.Identity
		}
	}
	if matched == nil {
		return nil, policy.ErrConflict
	}
	return matched, nil
}

func (m *Manager) VerifyLive(ctx context.Context, attempt *policy.Attempt) error {
	if attempt == nil || attempt.State != policy.AttemptRunning || attempt.Owner == nil || attempt.Owner.ChildPID <= 0 || attempt.Owner.ProviderSessionID == "" {
		return policy.ErrConflict
	}
	if err := m.policies.ValidateAttemptAuthority(ctx, attempt); err != nil {
		return err
	}
	prepared, err := m.retained(attempt.Snapshot.OrgID, attempt.ID)
	if err != nil {
		return err
	}
	binding, err := m.selectProvider(ctx, prepared.binding.InstallationID, prepared.binding.ProviderKey)
	if err != nil || !sameProviderBinding(binding, prepared.binding) {
		return policy.ErrConflict
	}
	return prepared.verifyProcesses(ctx, attempt.Owner)
}

func (p *Prepared) verifyProcesses(ctx context.Context, owner *policy.AttemptOwner) error {
	p.mu.Lock()
	supervisor := p.owner
	closed := p.closed
	p.mu.Unlock()
	if closed || supervisor == nil || supervisor.Supervisor.PID != owner.SupervisorPID || supervisor.Supervisor.BirthToken != owner.SupervisorBirth {
		return policy.ErrConflict
	}
	child, err := processidentity.Capture(owner.ChildPID)
	if err != nil || child.BirthToken != owner.ChildBirth {
		return policy.ErrConflict
	}
	owned, err := processidentity.IsDescendant(ctx, supervisor.Supervisor, child)
	if err != nil || !owned {
		return policy.ErrConflict
	}
	contained, err := p.launch.Containment.Contains(child.PID)
	if err != nil || !contained {
		return policy.ErrConflict
	}
	return nil
}

func (m *Manager) normalizeApproval(ctx context.Context, attempt *policy.Attempt, event []byte) (policy.ApprovalOperation, error) {
	prepared, err := m.retained(attempt.Snapshot.OrgID, attempt.ID)
	if err != nil {
		return policy.ApprovalOperation{}, err
	}
	response, err := m.providers.NormalizeSandboxApproval(ctx, prepared.binding, &pluginsdk.NormalizeSandboxApprovalRequest{Context: prepared.operation(), Policy: prepared.policy, VerifiedEvent: event})
	if err != nil {
		return policy.ApprovalOperation{}, err
	}
	operation := response.Operation
	if operation.Kind != pluginsdk.SandboxOperationFilesystem {
		return policy.ApprovalOperation{}, policy.ErrIneligible
	}
	return policy.ApprovalOperation{RequestID: operation.RequestId, ProviderSessionID: operation.ProviderSessionId, ChildPID: int(operation.ChildPid), Access: operation.Access, Target: operation.Target}, nil
}

// Close removes artifacts only after the owned process session has terminated.
func (p *Prepared) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	channelErr := p.channel.Close()
	if p.mcp != nil {
		channelErr = errors.Join(channelErr, p.mcp.Close())
	}
	if p.egress != nil {
		channelErr = errors.Join(channelErr, p.egress.Close())
	}
	if err := p.canRemoveArtifacts(); err != nil {
		return errors.Join(channelErr, err, p.markRecoveryRequired())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	attempt, err := p.manager.policies.ReadAttempt(ctx, p.orgID, p.launch.AttemptID)
	if err != nil {
		return errors.Join(channelErr, err)
	}
	if err = p.launch.Containment.Remove(); err != nil {
		return errors.Join(channelErr, err, p.markRecoveryRequired())
	}
	if attempt.State != policy.AttemptStopped && attempt.State != policy.AttemptFailed {
		if _, err = p.manager.policies.TransitionAttempt(ctx, p.orgID, attempt.ID, attempt.Version, policy.AttemptStopped, nil); err != nil {
			return errors.Join(channelErr, err)
		}
	}
	if _, err = p.captureReport(ctx); err != nil {
		return errors.Join(channelErr, err)
	}
	if err = p.files.Close(); err != nil {
		return errors.Join(channelErr, err)
	}
	if err = p.releaseAgentState(); err != nil {
		return errors.Join(channelErr, err)
	}
	p.closed = true
	p.manager.mu.Lock()
	delete(p.manager.attempts, p.launch.AttemptID)
	p.manager.mu.Unlock()
	return channelErr
}

func sameProviderBinding(a, b plugins.SandboxProviderBinding) bool {
	return a.PluginID == b.PluginID && a.InstallationID == b.InstallationID && a.ProviderKey == b.ProviderKey && a.Version == b.Version && a.PackageDigest == b.PackageDigest && a.InstalledAt.Equal(b.InstalledAt)
}

func (p *Prepared) persistSupervisor(ctx context.Context, agentctlIdentity string, owner *contract.ProcessOwner) error {
	attempt, err := p.manager.policies.ReadAttempt(ctx, p.orgID, p.launch.AttemptID)
	if err != nil {
		return err
	}
	if attempt.State != policy.AttemptStarting && attempt.State != policy.AttemptRunning {
		return policy.ErrConflict
	}
	if attempt.Owner != nil {
		if attempt.Owner.AgentctlIdentity != agentctlIdentity || attempt.Owner.SupervisorPID != owner.Supervisor.PID || attempt.Owner.SupervisorBirth != owner.Supervisor.BirthToken {
			return policy.ErrConflict
		}
		return nil
	}
	_, err = p.manager.policies.TransitionAttempt(ctx, p.orgID, attempt.ID, attempt.Version, policy.AttemptStarting, &policy.AttemptOwner{AgentctlIdentity: agentctlIdentity, SupervisorPID: owner.Supervisor.PID, SupervisorBirth: owner.Supervisor.BirthToken})
	return err
}

func (p *Prepared) markRecoveryRequired() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	attempt, err := p.manager.policies.ReadAttempt(ctx, p.orgID, p.launch.AttemptID)
	if err != nil {
		return err
	}
	if attempt.State == policy.AttemptRecoveryRequired {
		return nil
	}
	_, err = p.manager.policies.TransitionAttempt(ctx, p.orgID, attempt.ID, attempt.Version, policy.AttemptRecoveryRequired, nil)
	return err
}

func (p *Prepared) validateOwner(owner *contract.ProcessOwner) error {
	if p.closed || !p.startDispatched || owner == nil || owner.AttemptID != p.launch.AttemptID || owner.Generation != p.launch.Generation || owner.PolicyDigest != p.launch.PolicyDigest {
		return policy.ErrConflict
	}
	if _, err := owner.LiveSnapshot(); err != nil {
		return err
	}
	if owner.Supervisor.SessionID != owner.Supervisor.PID {
		return policy.ErrIneligible
	}
	contained, err := p.launch.Containment.Contains(owner.Supervisor.PID)
	if err != nil || !contained {
		return policy.ErrConflict
	}
	if p.owner != nil && *p.owner != *owner {
		return policy.ErrConflict
	}
	return nil
}
