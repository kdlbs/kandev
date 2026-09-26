// Package ptyexec starts commands under a pseudo-terminal.
//
// It is the single home for PTY plumbing needed by more than one tier:
// the backend's login-PTY sessions (internal/agent/loginpty) and the agentctl
// sidecar's interactive terminals (internal/agentctl/server/process). Both
// previously carried byte-identical copies of the ConPTY wrapper and the Win32
// command-line quoting; the copies had already begun to drift, and only one of
// them was reachable by CI, so a bug fixed in one would not be fixed in the
// other.
//
// This package lives under internal/common because neither tier may own it:
// agentctl is uploaded into containers as a standalone binary, so it must not
// import backend packages, and the backend must not depend on the sidecar's
// internals. internal/common is the established neutral tier for exactly this
// (see internal/common/subproc, gitref, securityutil), and cmd/agentctl already
// depends on 15 packages from it.
//
// The Windows command-line helpers deliberately carry no build tag: they are
// pure string transformations, and the Win32 argument escaping they implement is
// security-sensitive enough that its tests should run on every CI runner rather
// than only the single Windows job.
package ptyexec

import "io"

// PtyHandle abstracts PTY operations across Unix and Windows.
// On Unix it wraps creack/pty (*os.File); on Windows it wraps ConPTY.
type PtyHandle interface {
	io.ReadWriteCloser
	// Resize changes the PTY window size.
	Resize(cols, rows uint16) error
}
