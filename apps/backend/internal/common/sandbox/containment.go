package sandbox

import (
	"context"
	"github.com/kandev/kandev/internal/common/processidentity"
	"os/exec"
)

// ConfigureContainment returns a lease which must remain open through Start.
// Ordinary launches do not need kernel sandbox containment.
func ConfigureContainment(cmd *exec.Cmd, launch *Launch) (func(), error) {
	if launch == nil {
		return func() {}, nil
	}
	file, err := launch.Containment.Configure(cmd)
	if err != nil {
		return nil, err
	}
	return func() { _ = file.Close() }, nil
}

// RetireDescendants terminates the captured container only after its supervisor
// has exited. A process-session scan cannot account for detached children.
func RetireDescendants(ctx context.Context, launch *Launch, owner *ProcessOwner) error {
	if launch == nil || !receiptOwnerMatches(owner, launch) {
		return ErrInvalidPolicy
	}
	state, err := processidentity.Inspect(owner.Supervisor)
	if err != nil || state != processidentity.StateExited {
		return ErrInvalidPolicy
	}
	return launch.Containment.Kill(ctx)
}
