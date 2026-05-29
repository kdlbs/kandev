package agents

import (
	"context"
	"os/exec"
)

// DetectOption is a detection strategy. Returns (found, matchedPath, err).
type DetectOption func(ctx context.Context) (bool, string, error)

// WithCommand checks if a command is in PATH (exec.LookPath).
func WithCommand(name string) DetectOption {
	return func(ctx context.Context) (bool, string, error) {
		path, err := exec.LookPath(name)
		if err != nil {
			return false, "", nil
		}
		return true, path, nil
	}
}

// WithNpxRunnable reports the agent as runnable via `npx -y <pkg>` whenever
// npx is on PATH. MatchedPath is tagged "npx <pkg>" so the settings page
// renders "Detected at npx <pkg>", giving users an honest hint that the
// package isn't globally installed but will be fetched on launch.
//
// Only `npx` is checked, not `node`: every npm-distributed agent's launch
// command is `npx -y <pkg>`, so a node-only setup would pass detection but
// fail at session start with "exec: npx: executable file not found in PATH".
//
// Use this AFTER a WithCommand check for the global binary — Detect is
// first-match-wins, so the global install (if present) is preferred and
// reports its real path.
//
// NOTE: not currently wired into any agent's IsInstalled. The host-utility
// manager treats Available=true as a green light to spawn and probe the
// agent at boot, so claiming availability via npx alone caused unwanted
// downloads and misleading auth_required/failed states for agents the user
// never installed. The helper is parked here until the host-utility manager
// gains a "skip probe for npx-fallback detections" path; at that point this
// can be re-wired into the npm-distributed agents' IsInstalled.
func WithNpxRunnable(pkg string) DetectOption {
	return func(ctx context.Context) (bool, string, error) {
		if _, err := exec.LookPath("npx"); err == nil {
			return true, "npx " + pkg, nil
		}
		return false, "", nil
	}
}

// Detect runs options in order, returns first match.
// If none match, returns DiscoveryResult{Available: false}.
func Detect(ctx context.Context, opts ...DetectOption) (*DiscoveryResult, error) {
	for _, opt := range opts {
		found, matched, err := opt(ctx)
		if err != nil {
			return &DiscoveryResult{Available: false}, err
		}
		if found {
			return &DiscoveryResult{
				Available:   true,
				MatchedPath: matched,
			}, nil
		}
	}
	return &DiscoveryResult{Available: false}, nil
}
