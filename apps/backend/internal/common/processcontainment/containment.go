// Package processcontainment owns kernel-enforced descendant membership for sandbox attempts.
package processcontainment

import "errors"

var ErrUnavailable = errors.New("sandbox process containment is unavailable")

// Identity is private execution-host evidence, never a policy or user-supplied path.
type Identity struct {
	Path   string `json:"path"`
	BootID string `json:"boot_id"`
	Inode  uint64 `json:"inode"`
}
