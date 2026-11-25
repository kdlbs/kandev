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
	"time"

	"github.com/kandev/kandev/internal/agent/hostcli"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
)

const minimumACPCLIVersion = "1.5.0"
const workerModeFlag = "--process-per-session"

const acpHelpTimeout = 5 * time.Second

var errCLITooOld = errors.New("antigravity CLI version predates ACP support")
var errCLIWorkerModeUnsupported = errors.New("antigravity CLI does not advertise --process-per-session")

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
// Antigravity operation. CLI qualification happens once per selection.
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
// standalone bundle. A failed qualification cannot select an unqualified CLI.
func ResolveLocal(ctx context.Context, opts ResolveOptions) (*LocalRuntime, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cliPath := FindCLIPath(opts)
	var cliErr error
	if cliPath != "" {
		selected, err := resolveQualifiedCLI(ctx, cliPath, opts.VersionProbe, opts.ACPHelpProbe)
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

// RequireWorkerMode ensures the worker option wins over profile arguments.
// The caller applies wrappers afterward.
func RequireWorkerMode(args []string) []string {
	if len(args) == 0 {
		return nil
	}
	optionEnd := len(args)
	workerEnabled := false
	workerOptionIndex := -1
	for index := 1; index < len(args); index++ {
		argument := args[index]
		if argument == "--" {
			optionEnd = index
			break
		}
		option, value, hasValue := strings.Cut(argument, "=")
		if isAntigravityValueOption(option) {
			if !hasValue && index+1 < len(args) {
				index++
			}
			continue
		}
		if option == workerModeFlag {
			workerEnabled = !hasValue || isTrueValue(value)
			workerOptionIndex = index
		}
	}
	if workerEnabled && workerOptionIndex == optionEnd-1 {
		return slices.Clone(args)
	}
	result := make([]string, 0, len(args)+1)
	result = append(result, args[:optionEnd]...)
	result = append(result, "--process-per-session")
	result = append(result, args[optionEnd:]...)
	return result
}

// isAntigravityValueOption identifies options that consume the next argv token.
func isAntigravityValueOption(option string) bool {
	switch option {
	case "--add-dir", "--agent", "--app-data-dir", "--app_data_dir", "--cli_dir_name",
		"--cli_log_file", "--cloud_code_endpoint", "--gemini-dir", "--gemini_dir",
		"--ide-mcp-port", "--ide_mcp_port", "--log-file", "--override_user_agent_name":
		return true
	default:
		return false
	}
}

func isTrueValue(value string) bool {
	switch strings.ToLower(value) {
	case "true", "1", "yes", "on":
		return true
	default:
		return false
	}
}

// ResolveSaved resolves only the runtime recorded with a native conversation.
// Empty and unknown adapter versions retain the historical standalone path.
func ResolveSaved(ctx context.Context, opts ResolveOptions, adapterVersion string) (*LocalRuntime, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
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
		selected, err := resolveQualifiedCLI(ctx, cliPath, opts.VersionProbe, opts.ACPHelpProbe)
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
	versionProbe func(context.Context, string) (string, error),
	helpProbe func(context.Context, string) (string, error),
) (*LocalRuntime, error) {
	version, err := probeCLI(ctx, cliPath, versionProbe)
	if err != nil {
		return nil, err
	}
	if compareVersion(version, minimumACPCLIVersion) < 0 {
		return nil, fmt.Errorf("%w: found %s, require %s or later", errCLITooOld, version, minimumACPCLIVersion)
	}
	help, err := probeCLIACPHelp(ctx, cliPath, helpProbe)
	if err != nil {
		return nil, err
	}
	if !hasWorkerModeFlag(help) {
		return nil, errCLIWorkerModeUnsupported
	}
	return &LocalRuntime{
		Variant: acpcompat.AntigravityRuntimeCLI,
		Args:    []string{cliPath, "acp", workerModeFlag},
		Version: version,
	}, nil
}

func probeCLIACPHelp(
	ctx context.Context,
	path string,
	probe func(context.Context, string) (string, error),
) (string, error) {
	helpCtx, cancel := context.WithTimeout(ctx, acpHelpTimeout)
	defer cancel()
	var (
		output string
		err    error
	)
	if probe != nil {
		output, err = probe(helpCtx, path)
	} else {
		output, err = (hostcli.ExecRunner{}).Output(helpCtx, []string{path, "acp", "--help"})
	}
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if errors.Is(helpCtx.Err(), context.DeadlineExceeded) {
		return "", fmt.Errorf("antigravity CLI ACP help check timed out after %s", acpHelpTimeout)
	}
	if err != nil {
		return "", fmt.Errorf("antigravity CLI ACP help check failed: %w", err)
	}
	return output, nil
}

func hasWorkerModeFlag(output string) bool {
	inDeclarations := false
	for _, line := range strings.Split(output, "\n") {
		declaration := strings.TrimSpace(line)
		if strings.HasSuffix(declaration, ":") && !strings.HasPrefix(declaration, "-") {
			inDeclarations = declaration == "Flags:" || declaration == "Options:"
			continue
		}
		if !inDeclarations {
			continue
		}
		fields := strings.Fields(declaration)
		if len(fields) == 0 {
			continue
		}
		option := strings.Trim(fields[0], "[]{}()<>,;:'\"`")
		if option == workerModeFlag || strings.HasPrefix(option, workerModeFlag+"=") {
			return true
		}
	}
	return false
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
