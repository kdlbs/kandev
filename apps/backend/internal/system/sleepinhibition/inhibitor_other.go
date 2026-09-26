//go:build !darwin && !windows && !linux

package sleepinhibition

import "context"

type unsupportedInhibitor struct{}

func NewPlatformInhibitor() Inhibitor { return unsupportedInhibitor{} }

func (unsupportedInhibitor) Platform() Platform { return PlatformOther }
func (unsupportedInhibitor) Supported() bool    { return false }
func (unsupportedInhibitor) Acquire(context.Context) (Lease, error) {
	return nil, ErrUnsupported
}
