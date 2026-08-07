package installer

import (
	"fmt"
	"path/filepath"
	"runtime"
	"sort"

	"github.com/kandev/kandev/internal/common/logger"
	tools "github.com/kandev/kandev/internal/tools/installer"
	"go.uber.org/zap"
)

// DefaultBinDir is where LSP binaries installed by Kandev are placed.
const DefaultBinDir = ".kandev/lsp-servers"

const (
	languageTypeScript = "typescript"
	languageGo         = "go"
	languageRust       = "rust"
	languagePython     = "python"
	languageKotlin     = "kotlin"

	typeScriptLanguageServer = "typescript-language-server"
	goLanguageServer         = "gopls"
	rustLanguageServer       = "rust-analyzer"
	pythonLanguageServer     = "pyright-langserver"
	stdioArgument            = "--stdio"
	darwinOS                 = "darwin"
	linuxOS                  = "linux"
	windowsOS                = "windows"
)

// languageConfig holds the binary name and CLI arguments for a language server.
type languageConfig struct {
	binary      string
	args        []string
	autoInstall bool
	fileNames   []string
	extensions  []string
}

// languages is the single source of truth for supported LSP languages.
var languages = map[string]languageConfig{
	languageTypeScript: {
		binary: typeScriptLanguageServer, args: []string{stdioArgument}, autoInstall: true,
		fileNames:  []string{"jsconfig.json", "package.json", "tsconfig.json"},
		extensions: []string{".cjs", ".cts", ".js", ".jsx", ".mjs", ".mts", ".ts", ".tsx"},
	},
	languageGo: {
		binary: goLanguageServer, args: []string{"serve"}, autoInstall: true,
		fileNames: []string{"go.mod", "go.work"}, extensions: []string{".go"},
	},
	languageRust: {
		binary: rustLanguageServer, autoInstall: true,
		fileNames: []string{"cargo.toml"}, extensions: []string{".rs"},
	},
	languagePython: {
		binary: pythonLanguageServer, args: []string{stdioArgument}, autoInstall: true,
		fileNames:  []string{"pipfile", "pyproject.toml", "requirements.txt", "setup.cfg", "setup.py"},
		extensions: []string{".py", ".pyi"},
	},
	languageKotlin: {
		binary: "kotlin-lsp", args: []string{stdioArgument},
		fileNames: []string{"build.gradle.kts", "settings.gradle.kts"}, extensions: []string{".kt", ".kts"},
	},
}

type DiscoverySignal struct {
	Language   string
	FileNames  []string
	Extensions []string
}

// DiscoverySignals returns deterministic, read-only file-name metadata for
// every registered language. Callers inspect names only; no manifest content
// or project-controlled executable is involved.
func DiscoverySignals() []DiscoverySignal {
	languageIDs := make([]string, 0, len(languages))
	for language := range languages {
		languageIDs = append(languageIDs, language)
	}
	sort.Strings(languageIDs)
	result := make([]DiscoverySignal, 0, len(languageIDs))
	for _, language := range languageIDs {
		cfg := languages[language]
		result = append(result, DiscoverySignal{
			Language:   language,
			FileNames:  append([]string(nil), cfg.fileNames...),
			Extensions: append([]string(nil), cfg.extensions...),
		})
	}
	return result
}

// SupportedLanguages returns the set of supported LSP language identifiers.
func SupportedLanguages() map[string]struct{} {
	result := make(map[string]struct{}, len(languages))
	for lang := range languages {
		result[lang] = struct{}{}
	}
	return result
}

// IsSupported returns true if the language has a registered LSP configuration.
func IsSupported(language string) bool {
	_, ok := languages[language]
	return ok
}

// SupportsAutoInstall reports whether a language may be stored as a global
// auto-install preference. The task host remains responsible for deciding
// whether its own platform has a usable strategy.
func SupportsAutoInstall(language string) bool {
	cfg, ok := languages[language]
	return ok && cfg.autoInstall
}

// CanAutoInstall reports whether this process's host platform has an install
// strategy for language. Call this only in agentctl, where installation runs.
func CanAutoInstall(language string) bool {
	return canAutoInstallOnPlatform(language, runtime.GOOS)
}

// AutoInstallPreferenceLanguages returns the stable, task-host-independent set
// of languages exposed by the global Editors preference.
func AutoInstallPreferenceLanguages() []string {
	result := make([]string, 0, len(languages))
	for language := range languages {
		if SupportsAutoInstall(language) {
			result = append(result, language)
		}
	}
	sort.Strings(result)
	return result
}

func canAutoInstallOnPlatform(language, goos string) bool {
	if !SupportsAutoInstall(language) {
		return false
	}
	return language != languageRust || goos == darwinOS || goos == linuxOS
}

// LspCommand returns the binary name and arguments for a language server.
func LspCommand(language string) (binary string, args []string) {
	cfg, ok := languages[language]
	if !ok {
		return "", nil
	}
	return cfg.binary, cfg.args
}

// binaryName returns the expected binary name for a language.
func binaryName(language string) (string, error) {
	cfg, ok := languages[language]
	if !ok {
		return "", fmt.Errorf("unsupported language: %s", language)
	}
	return cfg.binary, nil
}

// Registry maps language IDs to install strategies and resolves binary paths.
type Registry struct {
	binDir        string // resolved absolute path
	logger        *logger.Logger
	commandRunner tools.CommandRunner
}

// RegistryOption customizes an installer registry.
type RegistryOption func(*Registry)

// WithCommandRunner routes task-environment lookup and npm/Go installs through
// an external process owner.
func WithCommandRunner(runner tools.CommandRunner) RegistryOption {
	return func(registry *Registry) {
		registry.commandRunner = runner
	}
}

// NewRegistry creates a new installer registry.
// An absolute dataDir stores LSP binaries under dataDir+"/lsp-servers".
// With no dataDir, the registry uses ~/.kandev/lsp-servers when the home
// directory resolves to an absolute path. Otherwise the managed cache is
// disabled so a project-relative path can never be treated as trusted.
func NewRegistry(dataDir string, log *logger.Logger, options ...RegistryOption) *Registry {
	registry := &Registry{
		logger: log.WithFields(zap.String("component", "lsp-installer")),
	}
	for _, option := range options {
		option(registry)
	}
	registry.binDir = managedBinDir(dataDir, registry.commandRunner)
	return registry
}

func managedBinDir(dataDir string, runner tools.CommandRunner) string {
	if dataDir != "" {
		if filepath.IsAbs(dataDir) {
			return filepath.Join(dataDir, "lsp-servers")
		}
		return ""
	}
	home, err := tools.CommandEnvironmentValue(runner, "HOME")
	if err != nil {
		return ""
	}
	if home == "" && runtime.GOOS == windowsOS {
		home, err = tools.CommandEnvironmentValue(runner, "USERPROFILE")
		if err != nil {
			return ""
		}
	}
	if !filepath.IsAbs(home) {
		return ""
	}
	return filepath.Join(home, DefaultBinDir)
}

// StrategyFor returns the install strategy for a language.
func (r *Registry) StrategyFor(language string) (Strategy, error) {
	if !CanAutoInstall(language) {
		if IsSupported(language) {
			return nil, fmt.Errorf("%s auto-install is not supported; install the language server on the task host", language)
		}
		return nil, fmt.Errorf("no installer for language: %s", language)
	}
	if r.binDir == "" && language != languageGo {
		return nil, fmt.Errorf("LSP install cache is unavailable: no absolute home or data directory")
	}
	switch language {
	case languageTypeScript:
		return tools.NewNpmStrategy(r.binDir, typeScriptLanguageServer, []string{typeScriptLanguageServer, languageTypeScript}, r.logger, r.commandRunner), nil
	case languageGo:
		return tools.NewGoInstallStrategy(goLanguageServer, "golang.org/x/tools/gopls@latest", r.logger, r.commandRunner), nil
	case languageRust:
		return tools.NewGithubReleaseStrategy(r.binDir, rustLanguageServer, tools.GithubReleaseConfig{
			Owner:        "rust-lang",
			Repo:         rustLanguageServer,
			AssetPattern: rustLanguageServer + "-{target}.gz",
			Targets: map[string]string{
				"darwin/arm64": "aarch64-apple-darwin",
				"darwin/amd64": "x86_64-apple-darwin",
				"linux/amd64":  "x86_64-unknown-linux-gnu",
				"linux/arm64":  "aarch64-unknown-linux-gnu",
			},
		}, r.logger), nil
	case languagePython:
		return tools.NewNpmStrategy(r.binDir, pythonLanguageServer, []string{"pyright"}, r.logger, r.commandRunner), nil
	default:
		return nil, fmt.Errorf("no installer for language: %s", language)
	}
}

// BinaryPath checks if a language server binary is installed.
// It checks the task PATH, the Kandev bin directory, and Go-specific paths.
func (r *Registry) BinaryPath(language string) (string, error) {
	binary, err := binaryName(language)
	if err != nil {
		return "", err
	}

	// Check the task PATH first.
	if p, err := tools.FindCommand(binary, r.commandRunner); err == nil {
		return p, nil
	}

	if r.binDir != "" {
		// Check Kandev bin directory (npm node_modules/.bin/)
		npmBinDir := filepath.Join(r.binDir, "node_modules", ".bin")
		if p, err := tools.FindCommandInDirectory(binary, npmBinDir, r.commandRunner); err == nil {
			return p, nil
		}

		// Check Kandev bin directory (direct binary)
		if p, err := tools.FindCommandInDirectory(binary, r.binDir, r.commandRunner); err == nil {
			return p, nil
		}
	}

	// Check Go-specific paths for Go binaries
	if language == languageGo {
		if p, err := tools.FindGoBinaryWithRunner(binary, r.commandRunner); err == nil {
			return p, nil
		}
	}

	return "", fmt.Errorf("%s not found", binary)
}
