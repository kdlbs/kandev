package lifecycle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/remoteauth"
	"github.com/kandev/kandev/internal/common/logger"
)

// FileUploader abstracts writing files to a remote environment. Used by
// UploadCredentialFiles to seed agent auth files into the kandev-managed
// per-container session dir (local) or sprite (remote).
type FileUploader interface {
	WriteFile(ctx context.Context, path string, data []byte, mode os.FileMode) error
}

const credentialFileMode os.FileMode = 0o600

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
			if err := uploader.WriteFile(ctx, targetPath, data, credentialFileMode); err != nil {
				return fmt.Errorf("failed to upload %s: %w", targetPath, err)
			}
			log.Debug("uploaded credential file",
				zap.String("method_id", method.MethodID),
				zap.String("target", targetPath))
		}
	}

	return nil
}
