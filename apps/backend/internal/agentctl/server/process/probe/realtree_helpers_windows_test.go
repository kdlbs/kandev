//go:build windows

package probe

import "os/exec"

// withOwnProcessGroup is a compile-only stub: Windows has no POSIX
// process-group concept, and the real-tree suite that would call this
// always skips on Windows before reaching it (see skipUnlessRealTreeSupported
// in probe_realtree_test.go — this platform has no process-table reader).
func withOwnProcessGroup(cmd *exec.Cmd) {}
