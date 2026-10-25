//go:build linux

package processidentity

import "context"

// IsDescendant checks live birth identities and a stable, bounded kernel parent chain.
func IsDescendant(ctx context.Context, root, child Identity) (bool, error) {
	if root.PID == child.PID {
		return false, nil
	}
	if err := inspectLivePair(root, child); err != nil {
		return false, err
	}
	chain, err := processParentChain(ctx, root.PID, child.PID)
	if err != nil {
		return false, err
	}
	if len(chain) == 0 {
		return false, nil
	}
	for _, previous := range chain {
		if err = ctx.Err(); err != nil {
			return false, err
		}
		current, readErr := readProcStat(previous.pid)
		if readErr != nil || current.parentPID != previous.parentPID || current.startTime != previous.startTime || current.state == 'Z' || current.state == 'X' {
			return false, ErrUnverifiableIdentity
		}
	}
	if err = inspectLivePair(root, child); err != nil {
		return false, err
	}
	return true, nil
}

func inspectLivePair(root, child Identity) error {
	for _, identity := range []Identity{root, child} {
		state, err := Inspect(identity)
		if err != nil || state != StateAlive {
			return ErrUnverifiableIdentity
		}
	}
	return nil
}

func processParentChain(ctx context.Context, rootPID, childPID int) ([]procStat, error) {
	chain := make([]procStat, 0, 8)
	currentPID := childPID
	for range 64 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		current, err := readProcStat(currentPID)
		if err != nil || current.state == 'Z' || current.state == 'X' {
			return nil, ErrUnverifiableIdentity
		}
		if len(chain) > 0 && current.startTime > chain[len(chain)-1].startTime {
			return nil, ErrUnverifiableIdentity
		}
		chain = append(chain, current)
		if current.pid == rootPID {
			return chain, nil
		}
		if current.parentPID <= 1 {
			return nil, nil
		}
		currentPID = current.parentPID
	}
	return nil, ErrUnverifiableIdentity
}
