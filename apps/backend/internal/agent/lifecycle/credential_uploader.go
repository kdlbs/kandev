package lifecycle

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/remoteauth"
	"github.com/kandev/kandev/internal/common/logger"
)

// FileUploader abstracts writing files and running commands on a remote environment.
type FileUploader interface {
	WriteFile(ctx context.Context, path string, data []byte, mode os.FileMode) error
	RunCommand(ctx context.Context, name string, args ...string) error
}

// UploadCredentialFiles reads local credential files and uploads them to the remote environment.
func UploadCredentialFiles(
	ctx context.Context,
	uploader FileUploader,
	methods []remoteauth.Method,
	targetHomeDir string,
	log *logger.Logger,
) error {
	if len(methods) == 0 {
		return nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	for _, method := range methods {
		if method.Type != "files" {
			continue
		}

		for _, relPath := range method.SourceFiles {
			srcPath := filepath.Join(home, relPath)
			data, readErr := os.ReadFile(srcPath)
			if readErr != nil {
				log.Warn("credential source file not found, skipping",
					zap.String("method_id", method.MethodID),
					zap.String("path", srcPath))
				continue
			}

			targetPath := filepath.Join(targetHomeDir, method.TargetRelDir, filepath.Base(relPath))
			if err := uploader.WriteFile(ctx, targetPath, data, 0o644); err != nil {
				return fmt.Errorf("failed to upload %s: %w", targetPath, err)
			}
			log.Debug("uploaded credential file",
				zap.String("method_id", method.MethodID),
				zap.String("target", targetPath))
		}
	}

	return nil
}

// DetectGHToken runs `gh auth token` locally and returns the GitHub OAuth token.
func DetectGHToken() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", "auth", "token")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("gh auth token failed: %w", err)
	}
	token := strings.TrimSpace(string(out))
	if token == "" {
		return "", fmt.Errorf("gh auth token returned empty")
	}
	return token, nil
}
