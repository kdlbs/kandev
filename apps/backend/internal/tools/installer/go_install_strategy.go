package installer

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// GoInstallStrategy installs tools via `go install`.
type GoInstallStrategy struct {
	binary     string // "gopls"
	importPath string // "golang.org/x/tools/gopls@latest"
	logger     *logger.Logger
	runner     CommandRunner
}

// NewGoInstallStrategy creates a new go install strategy.
func NewGoInstallStrategy(binary, importPath string, log *logger.Logger, runners ...CommandRunner) *GoInstallStrategy {
	strategy := &GoInstallStrategy{
		binary:     binary,
		importPath: importPath,
		logger:     log,
	}
	if len(runners) > 0 {
		strategy.runner = runners[0]
	}
	return strategy
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

	output, err := combinedOutput(ctx, s.runner, CommandSpec{
		Path: goPath,
		Args: []string{installSubcommand, s.importPath},
	})
	if err != nil {
		return nil, fmt.Errorf("go install failed: %w\nOutput: %s", err, string(output))
	}

	// Find the installed binary using the shared Go binary lookup
	binaryPath, err := FindGoBinary(s.binary)
	if err != nil {
		return nil, err
	}

	s.logger.Info("go install completed", zap.String("binary", binaryPath))
	return &InstallResult{
		BinaryPath: binaryPath,
	}, nil
}

// FindGoBinary looks for a Go binary in GOBIN, GOPATH/bin, and ~/go/bin.
func FindGoBinary(binary string) (string, error) {
	if gobin := os.Getenv("GOBIN"); gobin != "" {
		p := filepath.Join(gobin, binary)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	if gopath := os.Getenv("GOPATH"); gopath != "" {
		p := filepath.Join(gopath, "bin", binary)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	home, _ := os.UserHomeDir()
	if home != "" {
		p := filepath.Join(home, "go", "bin", binary)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("%s not found in GOBIN/GOPATH/~/go/bin", binary)
}
