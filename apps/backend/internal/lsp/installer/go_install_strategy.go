package installer

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// GoInstallStrategy installs language servers via `go install`.
// Used for Go (gopls).
type GoInstallStrategy struct {
	binary     string // "gopls"
	importPath string // "golang.org/x/tools/gopls@latest"
	logger     *logger.Logger
}

// NewGoInstallStrategy creates a new go install strategy.
func NewGoInstallStrategy(binary, importPath string, log *logger.Logger) *GoInstallStrategy {
	return &GoInstallStrategy{
		binary:     binary,
		importPath: importPath,
		logger:     log,
	}
}

func (s *GoInstallStrategy) Name() string {
	return fmt.Sprintf("go install %s", s.importPath)
}

func (s *GoInstallStrategy) Install(ctx context.Context) (*InstallResult, error) {
	goPath, err := exec.LookPath("go")
	if err != nil {
		return nil, fmt.Errorf("go not found: %w", err)
	}

	s.logger.Info("installing via go install", zap.String("import_path", s.importPath))

	cmd := exec.CommandContext(ctx, goPath, "install", s.importPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("go install failed: %w\nOutput: %s", err, string(output))
	}

	// Find the installed binary using the shared Go binary lookup
	binaryPath, err := findGoBinary(s.binary)
	if err != nil {
		return nil, err
	}

	s.logger.Info("go install completed", zap.String("binary", binaryPath))
	return &InstallResult{
		BinaryPath: binaryPath,
	}, nil
}
