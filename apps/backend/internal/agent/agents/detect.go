package agents

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// DetectOption is a detection strategy. Returns (found, matchedPath, err).
type DetectOption func(ctx context.Context) (bool, string, error)

// WithFileExists checks if any of the given paths exist (supports ~ expansion).
func WithFileExists(paths ...string) DetectOption {
	return func(ctx context.Context) (bool, string, error) {
		for _, p := range paths {
			expanded := expandHomePath(p)
			if expanded == "" {
				continue
			}
			if _, err := os.Stat(expanded); err == nil {
				return true, expanded, nil
			}
		}
		return false, "", nil
	}
}

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

// WithCommandOutput runs a command and checks stdout matches regex.
func WithCommandOutput(pattern string, name string, args ...string) DetectOption {
	return func(ctx context.Context) (bool, string, error) {
		cmd := exec.CommandContext(ctx, name, args...)
		out, err := cmd.Output()
		if err != nil {
			return false, "", nil
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return false, "", err
		}
		if re.Match(out) {
			return true, name, nil
		}
		return false, "", nil
	}
}

// WithEnvVar checks if an environment variable is set and non-empty.
func WithEnvVar(name string) DetectOption {
	return func(ctx context.Context) (bool, string, error) {
		val := os.Getenv(name)
		if val != "" {
			return true, name, nil
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

// expandHomePath expands ~ to the user's home directory.
func expandHomePath(path string) string {
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~"))
	}
	return filepath.Clean(filepath.FromSlash(path))
}

// OSPaths holds per-OS path lists. Use Resolve() to get the paths for the current OS.
type OSPaths struct {
	Linux   []string
	MacOS   []string
	Windows []string
}

// Resolve returns the raw paths for the current operating system.
func (p OSPaths) Resolve() []string {
	switch runtime.GOOS {
	case "darwin":
		return p.MacOS
	case "windows":
		return p.Windows
	default:
		return p.Linux
	}
}

// Expanded returns the paths for the current OS with ~ expanded to the home directory.
func (p OSPaths) Expanded() []string {
	paths := p.Resolve()
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		if expanded := expandHomePath(path); expanded != "" {
			result = append(result, expanded)
		}
	}
	return result
}
