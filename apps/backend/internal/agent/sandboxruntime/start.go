package sandboxruntime

import (
	"context"
	"time"

	policy "github.com/kandev/kandev/internal/agent/sandbox"
	contract "github.com/kandev/kandev/internal/common/sandbox"
)

// MarkStart persists dispatch intent before sending a subprocess start request.
// A lost reply must retain artifacts until the original process owner is known.
func (p *Prepared) MarkStart(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.startDispatched {
		return policy.ErrConflict
	}
	attempt, err := p.manager.policies.ReadAttempt(ctx, p.orgID, p.launch.AttemptID)
	if err != nil {
		return err
	}
	if err = p.manager.policies.ValidateAttemptAuthority(ctx, attempt); err != nil {
		return err
	}
	binding, err := p.manager.selectProvider(ctx, p.binding.InstallationID, p.binding.ProviderKey)
	if err != nil || !sameProviderBinding(binding, p.binding) {
		return policy.ErrConflict
	}
	if _, err = p.manager.policies.TransitionAttempt(ctx, p.orgID, attempt.ID, attempt.Version, policy.AttemptStarting, nil); err != nil {
		return err
	}
	p.startDispatched = true
	return nil
}

func (p *Prepared) canRemoveArtifacts() error {
	if p.owner == nil {
		if p.startDispatched {
			return policy.ErrConflict
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return contract.RetireDescendants(ctx, p.launch, p.owner)
}

// ValidateCommand checks final argv and the only environment keys the child can receive.
func (p *Prepared) ValidateCommand(arguments []string, environment map[string]string) error {
	approved := make(map[string]string, len(p.launch.EnvironmentKeys))
	for _, key := range p.launch.EnvironmentKeys {
		if value, ok := environment[key]; ok {
			approved[key] = value
		}
	}
	return contract.ValidateOverrides(arguments, approved, p.constraints)
}
