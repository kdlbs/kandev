//go:build !unix && !windows

package launcher

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/kandev/kandev/internal/common/processidentity"
)

func (l *Launcher) installChildLifecycle(_ *exec.Cmd) error { return nil }

func (l *Launcher) releaseChildLifecycle() {}

func (l *Launcher) containOwnedChildren(_ context.Context, identity processidentity.Identity) error {
	if err := identity.Validate(); err != nil {
		return err
	}
	return fmt.Errorf("process-tree containment is unsupported: %w", processidentity.ErrUnverifiableIdentity)
}
