//go:build !windows

package winproc

import "os"

// ListenForShutdown leaves the platform's existing OS signals in control.
func ListenForShutdown(_ chan<- os.Signal) (func(), error) { return func() {}, nil }
