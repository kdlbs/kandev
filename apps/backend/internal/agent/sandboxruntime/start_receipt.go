package sandboxruntime

import (
	"context"

	policy "github.com/kandev/kandev/internal/agent/sandbox"
	contract "github.com/kandev/kandev/internal/common/sandbox"
)

// ApplyStartReceipt accepts evidence only from the authenticated execution host.
// It never infers a failed spawn from a missing owner or a transport error.
func (p *Prepared) ApplyStartReceipt(ctx context.Context, agentctlIdentity string, receipt *contract.StartReceipt) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || !p.startDispatched || agentctlIdentity == "" || len(agentctlIdentity) > 128 || receipt.ValidateStopped(p.launch) != nil {
		return policy.ErrConflict
	}
	current, err := p.manager.policies.ReadAttempt(ctx, p.orgID, p.launch.AttemptID)
	if err != nil {
		return err
	}
	owner, err := p.receiptOwner(current, agentctlIdentity, receipt)
	if err != nil {
		return err
	}
	empty, err := p.launch.Containment.Empty()
	if err != nil || !empty {
		return policy.ErrConflict
	}
	if err = p.launch.Containment.Remove(); err != nil {
		return err
	}
	if current.State != policy.AttemptStopped {
		if _, err = p.manager.policies.TransitionAttempt(ctx, p.orgID, current.ID, current.Version, policy.AttemptStopped, owner); err != nil {
			return err
		}
	}
	if receipt.Owner != nil {
		copied := *receipt.Owner
		p.owner = &copied
	} else {
		p.startDispatched = false
	}
	return nil
}

func (p *Prepared) receiptOwner(current *policy.Attempt, agentctlIdentity string, receipt *contract.StartReceipt) (*policy.AttemptOwner, error) {
	if receipt.Outcome == contract.StartNotStarted {
		if current.Owner != nil || p.owner != nil || current.State == policy.AttemptRunning {
			return nil, policy.ErrConflict
		}
		return nil, nil
	}
	if p.owner != nil && *p.owner != *receipt.Owner {
		return nil, policy.ErrConflict
	}
	if current.Owner != nil {
		if current.Owner.AgentctlIdentity != agentctlIdentity || current.Owner.SupervisorPID != receipt.Owner.Supervisor.PID || current.Owner.SupervisorBirth != receipt.Owner.Supervisor.BirthToken {
			return nil, policy.ErrConflict
		}
		return current.Owner, nil
	}
	return &policy.AttemptOwner{AgentctlIdentity: agentctlIdentity, SupervisorPID: receipt.Owner.Supervisor.PID, SupervisorBirth: receipt.Owner.Supervisor.BirthToken}, nil
}
