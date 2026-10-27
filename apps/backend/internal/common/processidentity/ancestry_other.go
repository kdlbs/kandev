//go:build !linux

package processidentity

import "context"

// IsDescendant fails closed on hosts without the verified ancestry implementation.
func IsDescendant(context.Context, Identity, Identity) (bool, error) {
	return false, ErrUnverifiableIdentity
}
