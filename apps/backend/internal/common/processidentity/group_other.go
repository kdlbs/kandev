//go:build !darwin

package processidentity

// GroupMembers is only implemented where a process group replaces kernel containment.
func GroupMembers(int) ([]int, error) { return nil, ErrUnverifiableIdentity }
