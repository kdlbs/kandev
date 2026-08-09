//go:build windows

package process

import (
	"context"
	"time"
)

// walkProcessTree returns "unknown" on Windows. The spec's platform table
// (docs/specs/disambiguate-waiting/spec.md) pins this: Windows enumeration is
// "Not implemented" and MUST always answer unknown, never live or settled —
// AC-27 covers any platform that cannot enumerate the agent's descendants
// with their start times, and the *Failure modes* table names Windows
// explicitly. Do not add a real Toolhelp32Snapshot walk here without also
// updating the spec; the AC-80 truncate-down/inclusive->= rules and the
// zombie-exclusion rule were written and tested only for Linux and Darwin.
func walkProcessTree(_ context.Context, _ int, _ time.Time) string {
	return probeResultUnknown
}
