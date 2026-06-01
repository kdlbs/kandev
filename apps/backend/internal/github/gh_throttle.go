package github

import (
	"context"

	"github.com/kandev/kandev/internal/common/subproc"
)

// Why this throttle exists:
//
// Before this, every code path that wanted PR status spawned its own gh
// subprocess. With ~40 active PR watches and a 30s frontend refresh, the
// background sync could fire 40-70 gh invocations within a few seconds.
// On managed corporate macOS hosts (CrowdStrike Falcon + syspolicyd code
// signing) every fork/exec is intercepted, serialized, and validated,
// which turned the burst into a multi-second exec queue. Other processes
// waiting on the same queue (new terminal shells, kandev's own gh probes)
// then hit their 30s ctx deadline and were killed, causing a feedback
// loop that effectively froze the host.
//
// The cap, env var (KANDEV_GH_MAX_CONCURRENT), and singleton live in
// internal/common/subproc so all gh callers across the backend share
// one pool. This file only wraps the package-local accessors so existing
// call sites keep their short names and so the test seam stays here.

// acquireGHSlot blocks until a gh subprocess slot is available or ctx is
// cancelled. See subproc.Throttle.Acquire for release semantics.
func acquireGHSlot(ctx context.Context) (release func(), err error) {
	return subproc.GH().Acquire(ctx)
}

// setGHSemaphoreForTest replaces the gh throttle's slot pool with one of
// the given capacity and returns a restore function. Test-only — production
// code MUST NOT call this. Use cap=0 to disable throttling for a test.
func setGHSemaphoreForTest(cap int) func() {
	return subproc.GH().SetCapForTest(cap)
}
