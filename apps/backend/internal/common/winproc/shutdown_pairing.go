package winproc

import "os"

// ShutdownPairing classifies stop requests for a process that listens to both
// OS signals and the cooperative shutdown event.
//
// When Kandev shares a console with its parent, one Ctrl+C reaches this
// process as an OS signal and also makes the parent set the cooperative event.
// Both belong to the same request, so whichever is taken first starts
// shutdown and the other must not count as the second signal that forces
// exit. A cooperative request taken after shutdown started stays unread on its
// own channel; ShutdownPairing covers the opposite order. The zero value is
// ready to use and must be confined to one goroutine.
type ShutdownPairing struct {
	pairPending bool
}

// CooperativeFirst records that the first stop request read was the
// cooperative one and returns the signal that starts shutdown. An OS signal
// that is already waiting is taken instead, so both requests being ready is
// classified the same way regardless of select order. Otherwise the next OS
// signal is treated as the console pair of this request.
func (p *ShutdownPairing) CooperativeFirst(cooperative os.Signal, osSignals <-chan os.Signal) os.Signal {
	select {
	case sig := <-osSignals:
		p.pairPending = false
		return sig
	default:
	}
	p.pairPending = true
	return cooperative
}

// Forces reports whether an OS signal read after shutdown started is a
// deliberate second request. It returns false once for the OS signal paired
// with a cooperative first request.
func (p *ShutdownPairing) Forces(os.Signal) bool {
	if p.pairPending {
		p.pairPending = false
		return false
	}
	return true
}
