package installer

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

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
)

// languageConfig holds the binary name and CLI arguments for a language server.
type languageConfig struct {
	binary      string
	args        []string
	autoInstall bool
}

// languages is the single source of truth for supported LSP languages.
var languages = map[string]languageConfig{
	languageTypeScript: {binary: typeScriptLanguageServer, args: []string{stdioArgument}, autoInstall: true},
	languageGo:         {binary: goLanguageServer, args: []string{"serve"}, autoInstall: true},
	languageRust:       {binary: rustLanguageServer, autoInstall: true},
	languagePython:     {binary: pythonLanguageServer, args: []string{stdioArgument}, autoInstall: true},
	languageKotlin:     {binary: "kotlin-lsp", args: []string{stdioArgument}},
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

// CanAutoInstall reports whether Kandev has an install strategy for language.
// A language can be supported for manually installed binaries without being
// safe or practical for Kandev to install automatically.
func CanAutoInstall(language string) bool {
	cfg, ok := languages[language]
	return ok && cfg.autoInstall
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

// WithCommandRunner routes npm/go installs through an external process owner.
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
	var binDir string
	if dataDir != "" && filepath.IsAbs(dataDir) {
		binDir = filepath.Join(dataDir, "lsp-servers")
	} else if dataDir == "" {
		home, err := os.UserHomeDir()
		if err == nil && filepath.IsAbs(home) {
			binDir = filepath.Join(home, DefaultBinDir)
		}
	}
	registry := &Registry{
		binDir: binDir,
		logger: log.WithFields(zap.String("component", "lsp-installer")),
	}
	for _, option := range options {
		option(registry)
	}
	return registry
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
// It checks the system PATH, the Kandev bin directory, and Go-specific paths.
func (r *Registry) BinaryPath(language string) (string, error) {
	binary, err := binaryName(language)
	if err != nil {
		return "", err
	}

	// Check system PATH first
	if p, err := exec.LookPath(binary); err == nil {
		return p, nil
	}

	if r.binDir != "" {
		// Check Kandev bin directory (npm node_modules/.bin/)
		npmBinPath := filepath.Join(r.binDir, "node_modules", ".bin", binary)
		if _, err := os.Stat(npmBinPath); err == nil {
			return npmBinPath, nil
		}

		// Check Kandev bin directory (direct binary)
		directPath := filepath.Join(r.binDir, binary)
		if _, err := os.Stat(directPath); err == nil {
			return directPath, nil
		}
	}

	// Check Go-specific paths for Go binaries
	if language == languageGo {
		if p, err := tools.FindGoBinary(binary); err == nil {
			return p, nil
		}
	}

	return "", fmt.Errorf("%s not found", binary)
}
