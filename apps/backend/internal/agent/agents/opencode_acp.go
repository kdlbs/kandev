package agents

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/pkg/agent"
)

//go:embed logos/opencode_light.svg
var opencodeACPLogoLight []byte

//go:embed logos/opencode_dark.svg
var opencodeACPLogoDark []byte

const opencodeACPPackage = managedruntime.OpenCodeV1Package

// OpenCodeACPAgentID is the registry identifier for OpenCode's ACP provider.
const OpenCodeACPAgentID = "opencode-acp"

// opencodeNativeBinary is the standalone opencode CLI. When it is installed on
// PATH we launch it directly — mirroring CodeNomad's binary-first launch — so
// agent startups and refreshes never depend on per-launch `npx --prefer-
// offline` resolution (see ManagedNPMRuntimeSpec.NativeBinary).
const opencodeNativeBinary = "opencode"

const opencodeNativeVersionTimeout = 3 * time.Second

var opencodeNativeVersionPattern = regexp.MustCompile(`[0-9]+\.[0-9]+\.[0-9]+(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?`)

type OpenCodeNativeRuntime struct {
	Family  managedruntime.OpenCodeFamily
	Version string
}

var (
	_ Agent                     = (*OpenCodeACP)(nil)
	_ PassthroughAgent          = (*OpenCodeACP)(nil)
	_ InferenceAgent            = (*OpenCodeACP)(nil)
	_ HostUtilityInferenceAgent = (*OpenCodeACP)(nil)
	_ ManagedNPMRuntimeAgent    = (*OpenCodeACP)(nil)
	_ NativeBinaryAgent         = (*OpenCodeACP)(nil)
)

// OpenCodeACP is the ACP protocol variant of OpenCode.
// Uses JSON-RPC 2.0 over stdin/stdout via "opencode acp" instead of REST/SSE.
type OpenCodeACP struct {
	StandardPassthrough
	selectionReader managedruntime.OpenCodeSelectionReader
}

type OpenCodeRuntimeResolution struct {
	Family  managedruntime.OpenCodeFamily
	Source  managedruntime.OpenCodeSource
	Spec    ManagedNPMRuntimeSpec
	Version string
}

func (a *OpenCodeACP) SetOpenCodeSelectionReader(reader managedruntime.OpenCodeSelectionReader) {
	a.selectionReader = reader
}

func (a *OpenCodeACP) ResolveSelectedRuntime(ctx context.Context) (OpenCodeRuntimeResolution, error) {
	return a.ResolveSelectedRuntimeWithReader(ctx, a.selectionReader)
}

func (a *OpenCodeACP) ResolveSelectedRuntimeWithReader(
	ctx context.Context,
	reader managedruntime.OpenCodeSelectionReader,
) (OpenCodeRuntimeResolution, error) {
	if reader == nil {
		return OpenCodeRuntimeResolution{}, errors.New("OpenCode runtime selection is unavailable")
	}
	selection, found, err := reader.GetOpenCodeSelection(ctx)
	if err != nil {
		return OpenCodeRuntimeResolution{}, fmt.Errorf("read OpenCode runtime selection: %w", err)
	}
	if !found {
		return OpenCodeRuntimeResolution{}, errors.New("OpenCode runtime selection has not been initialized")
	}
	spec, err := a.ManagedNPMRuntimeForFamily(selection.Family)
	if err != nil {
		return OpenCodeRuntimeResolution{}, err
	}
	version := selection.SelectedVersion
	if version == "" {
		version = selection.AppliedDefaultVersion
	}
	return OpenCodeRuntimeResolution{
		Family:  selection.Family,
		Source:  selection.Source,
		Spec:    spec,
		Version: version,
	}, nil
}

func NewOpenCodeACP() *OpenCodeACP {
	return &OpenCodeACP{
		StandardPassthrough: StandardPassthrough{
			PermSettings: emptyPermSettings,
			Cfg: PassthroughConfig{
				Supported:      true,
				Label:          "CLI Passthrough",
				Description:    "Show terminal directly instead of chat interface",
				PassthroughCmd: NewCommand("opencode"),
				ModelFlag:      NewParam("--model", "{model}"),
				PromptFlag:     NewParam("--prompt", "{prompt}"),
				IdleTimeout:    3 * time.Second,
				BufferMaxBytes: DefaultBufferMaxBytes,
				ResumeFlag:     NewParam("-c"),
				// opencode has no MCP flag; write a temp opencode.json and point
				// it there via the OPENCODE_CONFIG env var (merges, never writes
				// ~/.config/opencode).
				MCPStrategy: mcpconfig.OpenCodeStrategy{},
			},
		},
	}
}

// DetectOpenCodeNativeRuntime reads the bounded version output of the native
// OpenCode CLI and rejects unsupported majors before a launch can guess flags.
func DetectOpenCodeNativeRuntime(ctx context.Context) (OpenCodeNativeRuntime, bool, error) {
	path, err := exec.LookPath(opencodeNativeBinary)
	if err != nil {
		var execErr *exec.Error
		if errors.As(err, &execErr) && errors.Is(execErr.Err, exec.ErrNotFound) {
			return OpenCodeNativeRuntime{}, false, nil
		}
		if errors.Is(err, exec.ErrNotFound) {
			return OpenCodeNativeRuntime{}, false, nil
		}
		return OpenCodeNativeRuntime{}, false, fmt.Errorf("locate native OpenCode executable: %w", err)
	}
	versionCtx, cancel := context.WithTimeout(ctx, opencodeNativeVersionTimeout)
	defer cancel()
	output, err := exec.CommandContext(versionCtx, path, "--version").CombinedOutput()
	if err != nil {
		return OpenCodeNativeRuntime{}, true, fmt.Errorf("read native OpenCode version: %w", err)
	}
	match := opencodeNativeVersionPattern.FindString(string(output))
	parsed, err := managedruntime.ParseStableVersion(strings.TrimSpace(match))
	if err != nil {
		return OpenCodeNativeRuntime{}, true, fmt.Errorf("read native OpenCode version: unsupported output")
	}
	family := managedruntime.OpenCodeFamily("")
	switch parsed.Major() {
	case 1:
		family = managedruntime.OpenCodeFamilyV1
	case 2:
		family = managedruntime.OpenCodeFamilyV2
	default:
		return OpenCodeNativeRuntime{}, true, fmt.Errorf("native OpenCode major %d is not supported", parsed.Major())
	}
	return OpenCodeNativeRuntime{Family: family, Version: parsed.Original()}, true, nil
}

func (a *OpenCodeACP) ID() string          { return OpenCodeACPAgentID }
func (a *OpenCodeACP) Name() string        { return "OpenCode AI Agent (ACP)" }
func (a *OpenCodeACP) DisplayName() string { return "OpenCode" }
func (a *OpenCodeACP) Description() string {
	return "OpenCode coding agent using ACP protocol over stdin/stdout."
}
func (a *OpenCodeACP) Enabled() bool     { return true }
func (a *OpenCodeACP) DisplayOrder() int { return 4 }

func (a *OpenCodeACP) Logo(v LogoVariant) []byte {
	if v == LogoDark {
		return opencodeACPLogoDark
	}
	return opencodeACPLogoLight
}

func (a *OpenCodeACP) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	if a.selectionReader == nil {
		return a.isLegacyNativeInstalled(ctx)
	}
	selected, err := a.ResolveSelectedRuntime(ctx)
	if err != nil {
		return &DiscoveryResult{Available: false}, err
	}
	if selected.Source == managedruntime.OpenCodeSourceNative {
		result, installed, err := a.detectSelectedNative(ctx)
		if err != nil || installed {
			return result, err
		}
	}
	return a.detectManagedRuntime(ctx, selected.Spec, selected.Version)
}

func (a *OpenCodeACP) isLegacyNativeInstalled(ctx context.Context) (*DiscoveryResult, error) {
	result, err := Detect(ctx, WithCommand("opencode"))
	if err != nil || !result.Available {
		return result, err
	}
	result.SupportsMCP = true
	result.Capabilities = DiscoveryCapabilities{
		SupportsSessionResume: true,
	}
	return result, nil
}

func (a *OpenCodeACP) detectSelectedNative(ctx context.Context) (*DiscoveryResult, bool, error) {
	result, err := Detect(ctx, WithCommand(opencodeNativeBinary))
	if err != nil || !result.Available {
		return result, false, err
	}
	_, found, err := DetectOpenCodeNativeRuntime(ctx)
	if err != nil {
		return &DiscoveryResult{Available: false}, false, err
	}
	if !found {
		return result, false, nil
	}
	result.SupportsMCP = true
	result.Capabilities = DiscoveryCapabilities{SupportsSessionResume: true}
	return result, true, nil
}

func (a *OpenCodeACP) detectManagedRuntime(
	ctx context.Context,
	spec ManagedNPMRuntimeSpec,
	version string,
) (*DiscoveryResult, error) {
	cached, err := openCodeManagedRuntimeCached(ctx, spec, version)
	if err != nil || !cached {
		return &DiscoveryResult{Available: false}, err
	}
	return &DiscoveryResult{
		Available:    true,
		MatchedPath:  "managed OpenCode runtime",
		SupportsMCP:  true,
		Capabilities: DiscoveryCapabilities{SupportsSessionResume: true},
	}, nil
}

func openCodeManagedRuntimeCached(ctx context.Context, spec ManagedNPMRuntimeSpec, version string) (bool, error) {
	if _, err := exec.LookPath("npm"); err != nil {
		return false, nil
	}
	cacheCtx, cancel := context.WithTimeout(ctx, commandCheckTimeout)
	defer cancel()
	output, err := exec.CommandContext(cacheCtx, "npm", "config", "get", "cache").Output()
	if err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		return false, nil
	}
	cacheRoot := strings.TrimSpace(string(output))
	if cacheRoot == "" || !filepath.IsAbs(cacheRoot) {
		return false, nil
	}
	packageSpec := spec.PackageSpec(version)
	cacheTree := filepath.Join(cacheRoot, "_npx", managedruntime.NpxExecutionCacheKey(packageSpec))
	info, err := os.Lstat(cacheTree)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return info.IsDir() && info.Mode()&os.ModeSymlink == 0, nil
}

// NativeBinaryName returns the standalone opencode CLI name probed for in the
// execution environment. See NativeBinaryAgent.
func (a *OpenCodeACP) NativeBinaryName() string { return opencodeNativeBinary }

func (a *OpenCodeACP) BuildCommand(opts CommandOptions) Command {
	if opts.ManagedRuntimeFamily != "" {
		spec, err := a.ManagedNPMRuntimeForFamily(opts.ManagedRuntimeFamily)
		if err != nil {
			return Command{}
		}
		if opts.ManagedRuntimeSource == managedruntime.OpenCodeSourceNative && opts.PreferNativeBinary {
			args, err := OpenCodeACPArgsForVersion(opts.NativeRuntimeVersion)
			if err != nil {
				return Command{}
			}
			spec.ACPArgs = args
			return spec.NativeCommand()
		}
		return spec.ACPCommand(opts.ManagedRuntimeVersion)
	}
	// Prefer the standalone opencode binary when the lifecycle probe found it
	// on PATH. The managed npm runtime remains the fallback for containerized
	// and remote runtimes.
	if opts.PreferNativeBinary {
		return a.ManagedNPMRuntime().NativeCommand()
	}
	return a.ManagedNPMRuntime().ACPCommand(opts.ManagedRuntimeVersion)
}

func (a *OpenCodeACP) ManagedNPMRuntime() ManagedNPMRuntimeSpec {
	spec, err := a.ManagedNPMRuntimeForFamily(managedruntime.OpenCodeFamilyV1)
	if err != nil {
		panic(fmt.Sprintf("OpenCode v1 runtime defaults are unavailable: %v", err))
	}
	return spec
}

// ManagedNPMRuntimeForFamily returns the trusted package and ACP arguments
// for one supported OpenCode distribution.
func (a *OpenCodeACP) ManagedNPMRuntimeForFamily(family managedruntime.OpenCodeFamily) (ManagedNPMRuntimeSpec, error) {
	packageName := ""
	args := []string{"acp", "--print-logs"}
	switch family {
	case managedruntime.OpenCodeFamilyV1:
		packageName = opencodeACPPackage
	case managedruntime.OpenCodeFamilyV2:
		packageName = managedruntime.OpenCodeV2Package
	default:
		return ManagedNPMRuntimeSpec{}, fmt.Errorf("unsupported OpenCode runtime family %q", family)
	}
	version, err := DefaultManagedNPMRuntimeVersion(packageName)
	if err != nil {
		return ManagedNPMRuntimeSpec{}, err
	}
	return ManagedNPMRuntimeSpec{
		Package:        packageName,
		DefaultVersion: version,
		ACPArgs:        args,
		NativeBinary:   opencodeNativeBinary,
	}, nil
}

// OpenCodeACPArgsForVersion validates an observed supported CLI version before
// returning the arguments accepted by both supported major releases.
func OpenCodeACPArgsForVersion(version string) ([]string, error) {
	parsed, err := managedruntime.ParseStableVersion(version)
	if err != nil {
		return nil, fmt.Errorf("unsupported native OpenCode version")
	}
	major := parsed.Major()
	if major != 1 && major != 2 {
		return nil, fmt.Errorf("native OpenCode major %d is not supported", major)
	}
	return []string{"acp", "--print-logs"}, nil
}

func (a *OpenCodeACP) Runtime() *RuntimeConfig {
	canRecover := true
	return &RuntimeConfig{
		Cmd:             a.ManagedNPMRuntime().CachedACPCommand(),
		WorkingDir:      "{workspace}",
		Env:             map[string]string{},
		ResourceLimits:  ResourceLimits{MemoryMB: 4096, CPUCores: 2.0, Timeout: time.Hour},
		Protocol:        agent.ProtocolACP,
		ProjectSkillDir: ".agents/skills",
		UserSkillDir:    ".config/opencode/skills",
		// opencode acp runs its HTTP server + MCP child tree alongside the
		// ACP stdin/stdout. Closing stdin doesn't terminate the process, so
		// skip the graceful wait and reap its process group immediately.
		// See GH issue #1247.
		RequiresProcessKill: true,
		SessionConfig: SessionConfig{
			NativeSessionResume:         true,
			NewSessionOnWorkspaceRebind: true,
			CanRecover:                  &canRecover,
			// Auth lives under .local/share/opencode and configuration under
			// .config/opencode. Mount the isolated executor home so both trees
			// remain visible without mounting the host home.
			SessionDirTemplate: "{home}",
			SessionDirTarget:   "/root",
		},
	}
}

func (a *OpenCodeACP) RemoteAuth() *RemoteAuth {
	return &RemoteAuth{
		Methods: []RemoteAuthMethod{
			{
				Type:               remoteAuthMethodTypeFiles,
				Label:              remoteAuthLabelCopyFiles,
				FileConflictPolicy: RemoteAuthFileConflictPolicyMergeJSONObject,
				SourceFiles: map[string][]string{
					"darwin": {".local/share/opencode/auth.json"},
					"linux":  {".local/share/opencode/auth.json"},
				},
				TargetRelDir: ".local/share/opencode",
			},
		},
	}
}

func (a *OpenCodeACP) PortableConfig() *PortableConfig {
	return &PortableConfig{Bundles: []PortableConfigBundle{
		{
			ID:    "opencode.config",
			Label: "Copy OpenCode configuration",
			Files: []PortableConfigFile{
				{SourcePaths: map[string]string{
					"darwin":  ".config/opencode/opencode.json",
					"linux":   ".config/opencode/opencode.json",
					"windows": ".config/opencode/opencode.json",
				}, TargetPath: ".config/opencode/opencode.json"},
				{SourcePaths: map[string]string{
					"darwin":  ".config/opencode/opencode.jsonc",
					"linux":   ".config/opencode/opencode.jsonc",
					"windows": ".config/opencode/opencode.jsonc",
				}, TargetPath: ".config/opencode/opencode.jsonc"},
			},
		},
	}}
}

func (a *OpenCodeACP) InstallScript() string {
	return "npm install -g " + opencodeACPPackage
}

func (a *OpenCodeACP) SettingsInstallCommand(selection managedruntime.OpenCodeSelection) (Command, error) {
	spec, err := a.ManagedNPMRuntimeForFamily(selection.Family)
	if err != nil {
		return Command{}, err
	}
	if selection.Package != spec.Package {
		return Command{}, fmt.Errorf("OpenCode runtime selection package %q does not match family %q", selection.Package, selection.Family)
	}
	version := selection.SelectedVersion
	if version == "" {
		version = selection.AppliedDefaultVersion
	}
	if _, err := managedruntime.ParseStableVersion(version); err != nil {
		return Command{}, fmt.Errorf("resolve selected OpenCode install version: %w", err)
	}
	if selection.Source == managedruntime.OpenCodeSourceNative {
		return spec.NativeUpdateCommand(version), nil
	}
	if selection.Source != managedruntime.OpenCodeSourceManaged {
		return Command{}, fmt.Errorf("unsupported OpenCode runtime source %q", selection.Source)
	}
	return spec.CacheUpdateCommand(version), nil
}

func (a *OpenCodeACP) BillingType() usage.BillingType { return defaultBillingType() }

func (a *OpenCodeACP) PermissionSettings() map[string]PermissionSetting {
	return emptyPermSettings
}

// InferenceConfig returns the executor-safe configuration for one-shot
// inference. Session inference can run inside a container or on SSH, so it
// keeps the managed npm command unless the executor selects native use.
func (a *OpenCodeACP) InferenceConfig() *InferenceConfig {
	return &InferenceConfig{
		Supported: true,
		Command:   a.ManagedNPMRuntime().CachedACPCommand(),
	}
}

// HostUtilityInferenceConfig prefers the native binary because host utility
// instances run on the backend host and do not cross an executor boundary.
func (a *OpenCodeACP) HostUtilityInferenceConfig() *InferenceConfig {
	spec := a.ManagedNPMRuntime()
	command := spec.CachedACPCommand()
	if spec.NativeBinaryOnPath() {
		command = spec.NativeCommand()
	}
	return &InferenceConfig{
		Supported: true,
		Command:   command,
	}
}
