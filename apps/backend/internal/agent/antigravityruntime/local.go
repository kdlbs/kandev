package antigravityruntime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/kandev/kandev/internal/agent/hostcli"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
)

const minimumACPCLIVersion = "1.5.0"

var errCLITooOld = errors.New("antigravity CLI version predates ACP support")

// SavedRuntimeError identifies a selected runtime that can no longer be
// resolved for a native conversation. Callers must preserve the conversation
// ID and surface this as a resume failure.
type SavedRuntimeError struct {
	Variant string
	Cause   error
}

func (e *SavedRuntimeError) Error() string {
	return fmt.Sprintf("saved Antigravity %s runtime is unavailable: %v", e.Variant, e.Cause)
}

func (e *SavedRuntimeError) Unwrap() error { return e.Cause }

// LocalRuntime identifies the complete command selected for a host-local
// Antigravity operation. CLI version qualification happens once per selection.
type LocalRuntime struct {
	Variant string
	Args    []string
	Version string
	Bundle  *Bundle
}

// FindCLIPath returns an installed CLI candidate without starting a process.
// The vendor installer's user-local path is checked when it is not on PATH.
func FindCLIPath(opts ResolveOptions) string {
	goos := opts.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	name := "agy"
	if goos == windowsOS {
		name += ".exe"
	}
	lookPath := opts.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	if path, err := lookPath(name); err == nil {
		if absolute, ok := absoluteExecutable(path, goos); ok {
			return absolute
		}
	}
	userHome := opts.UserHomeDir
	if userHome == "" {
		userHome, _ = os.UserHomeDir()
	}
	if userHome == "" {
		return ""
	}
	for _, path := range cliInstallCandidates(userHome, name) {
		if absolute, ok := absoluteExecutable(path, goos); ok {
			return absolute
		}
	}
	return ""
}

func cliInstallCandidates(home, name string) []string {
	return []string{
		filepath.Join(home, ".local", "bin", name),
		filepath.Join(home, "bin", name),
	}
}

func absoluteExecutable(path, goos string) (string, bool) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	info, err := os.Stat(absolute)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	if goos != windowsOS && info.Mode().Perm()&0o111 == 0 {
		return "", false
	}
	return absolute, true
}

// ResolveLocal qualifies the host CLI before falling back to the complete
// standalone bundle. A failed version check cannot select an unqualified CLI.
func ResolveLocal(ctx context.Context, opts ResolveOptions) (*LocalRuntime, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cliPath := FindCLIPath(opts)
	var cliErr error
	if cliPath != "" {
		selected, err := resolveQualifiedCLI(ctx, cliPath, opts.VersionProbe)
		if err == nil {
			return selected, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		cliErr = err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return resolveStandalone(ctx, opts, cliErr)
}

// IsSelectedCLICommand admits only the currently qualified host CLI argv.
// Profile flags may follow the required executable and ACP prefix.
func IsSelectedCLICommand(ctx context.Context, args []string) bool {
	selected, err := ResolveLocal(ctx, ResolveOptions{})
	if err != nil || selected.Variant != acpcompat.AntigravityRuntimeCLI || len(args) < len(selected.Args) {
		return false
	}
	return slices.Equal(args[:len(selected.Args)], selected.Args)
}

// ResolveSaved resolves only the runtime recorded with a native conversation.
// Empty and unknown adapter versions retain the historical standalone path.
func ResolveSaved(ctx context.Context, opts ResolveOptions, adapterVersion string) (*LocalRuntime, error) {
	variant := acpcompat.AntigravityRuntimeForAdapterVersion(adapterVersion)
	selected, err := resolveVariant(ctx, opts, variant)
	if err != nil {
		return nil, &SavedRuntimeError{Variant: variant, Cause: err}
	}
	return selected, nil
}

func resolveVariant(ctx context.Context, opts ResolveOptions, variant string) (*LocalRuntime, error) {
	if variant == acpcompat.AntigravityRuntimeCLI {
		cliPath := FindCLIPath(opts)
		if cliPath == "" {
			return nil, errors.New("antigravity CLI was not found")
		}
		selected, err := resolveQualifiedCLI(ctx, cliPath, opts.VersionProbe)
		if err != nil {
			return nil, fmt.Errorf("qualify Antigravity CLI: %w", err)
		}
		return selected, nil
	}
	return resolveStandalone(ctx, opts, nil)
}

func resolveQualifiedCLI(
	ctx context.Context,
	cliPath string,
	probe func(context.Context, string) (string, error),
) (*LocalRuntime, error) {
	version, err := probeCLI(ctx, cliPath, probe)
	if err != nil {
		return nil, err
	}
	if compareVersion(version, minimumACPCLIVersion) < 0 {
		return nil, fmt.Errorf("%w: found %s, require %s or later", errCLITooOld, version, minimumACPCLIVersion)
	}
	return &LocalRuntime{
		Variant: acpcompat.AntigravityRuntimeCLI,
		Args:    []string{cliPath, "acp"},
		Version: version,
	}, nil
}

func resolveStandalone(ctx context.Context, opts ResolveOptions, cliErr error) (*LocalRuntime, error) {
	bundle, err := Resolve(ctx, opts)
	if err == nil {
		argv := []string{bundle.ServerPath}
		goos := opts.GOOS
		if goos == "" {
			goos = runtime.GOOS
		}
		if goos == "linux" {
			argv = append(argv, "--uid=")
		}
		return &LocalRuntime{
			Variant: acpcompat.AntigravityRuntimeStandalone,
			Args:    argv,
			Version: bundle.Version,
			Bundle:  bundle,
		}, nil
	}
	if cliErr != nil {
		return nil, fmt.Errorf("qualify Antigravity CLI: %w; standalone runtime: %w", cliErr, err)
	}
	return nil, err
}

func probeCLI(ctx context.Context, path string, probe func(context.Context, string) (string, error)) (string, error) {
	if probe != nil {
		return probe(ctx, path)
	}
	return hostcli.DetectVersion(ctx, hostcli.ExecRunner{}, path, []string{"--version"})
}

func compareVersion(left, right string) int {
	leftParts := strings.Split(left, ".")
	rightParts := strings.Split(right, ".")
	for index := 0; index < max(len(leftParts), len(rightParts)); index++ {
		leftPart, rightPart := 0, 0
		if index < len(leftParts) {
			leftPart, _ = strconv.Atoi(leftParts[index])
		}
		if index < len(rightParts) {
			rightPart, _ = strconv.Atoi(rightParts[index])
		}
		if leftPart < rightPart {
			return -1
		}
		if leftPart > rightPart {
			return 1
		}
	}
	return 0
}
