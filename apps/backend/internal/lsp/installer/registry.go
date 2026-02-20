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

// languageConfig holds the binary name and CLI arguments for a language server.
type languageConfig struct {
	binary string
	args   []string
}

// languages is the single source of truth for supported LSP languages.
var languages = map[string]languageConfig{
	"typescript": {binary: "typescript-language-server", args: []string{"--stdio"}},
	"go":         {binary: "gopls", args: []string{"serve"}},
	"rust":       {binary: "rust-analyzer", args: nil},
	"python":     {binary: "pyright-langserver", args: []string{"--stdio"}},
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
	binDir string // resolved absolute path
	logger *logger.Logger
}

// NewRegistry creates a new installer registry.
func NewRegistry(log *logger.Logger) *Registry {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return &Registry{
		binDir: filepath.Join(home, DefaultBinDir),
		logger: log.WithFields(zap.String("component", "lsp-installer")),
	}
}

// StrategyFor returns the install strategy for a language.
func (r *Registry) StrategyFor(language string) (Strategy, error) {
	switch language {
	case "typescript":
		return tools.NewNpmStrategy(r.binDir, "typescript-language-server", []string{"typescript-language-server", "typescript"}, r.logger), nil
	case "go":
		return tools.NewGoInstallStrategy("gopls", "golang.org/x/tools/gopls@latest", r.logger), nil
	case "rust":
		return tools.NewGithubReleaseStrategy(r.binDir, "rust-analyzer", tools.GithubReleaseConfig{
			Owner:        "rust-lang",
			Repo:         "rust-analyzer",
			AssetPattern: "rust-analyzer-{target}.gz",
			Targets: map[string]string{
				"darwin/arm64": "aarch64-apple-darwin",
				"darwin/amd64": "x86_64-apple-darwin",
				"linux/amd64":  "x86_64-unknown-linux-gnu",
				"linux/arm64":  "aarch64-unknown-linux-gnu",
			},
		}, r.logger), nil
	case "python":
		return tools.NewNpmStrategy(r.binDir, "pyright-langserver", []string{"pyright"}, r.logger), nil
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

	// Check Go-specific paths for Go binaries
	if language == "go" {
		if p, err := tools.FindGoBinary(binary); err == nil {
			return p, nil
		}
	}

	return "", fmt.Errorf("%s not found", binary)
}
