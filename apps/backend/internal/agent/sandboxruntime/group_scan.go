package sandboxruntime

import (
	"context"
	"time"

	policy "github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/kandev/kandev/internal/common/processidentity"
)

// The interval and bound are variables so tests can shorten them.
var (
	groupScanInterval = 100 * time.Millisecond
	groupScanBound    = 5 * time.Second
)

// groupMembers reads the live members of an agentctl-owned process group.
// Tests replace it; the production reader is read-only and never signals.
var groupMembers = processidentity.GroupMembers

// awaitGroupEnded polls until the process group has no live member. A survivor
// at the bound, or any read error, leaves the group unproven.
func awaitGroupEnded(ctx context.Context, pgid int) error {
	ctx, cancel := context.WithTimeout(ctx, groupScanBound)
	defer cancel()
	ticker := time.NewTicker(groupScanInterval)
	defer ticker.Stop()
	for {
		members, err := groupMembers(pgid)
		if err != nil {
			return policy.ErrConflict
		}
		if len(members) == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return policy.ErrConflict
		case <-ticker.C:
		}
	}
}
