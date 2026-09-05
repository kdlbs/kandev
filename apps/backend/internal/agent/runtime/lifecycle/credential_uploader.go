package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/remoteauth"
	"github.com/kandev/kandev/internal/common/logger"
)

// FileUploader abstracts writing files to a remote environment. Used by
// UploadCredentialFiles to seed agent auth files into the kandev-managed
// per-container session dir (local) or sprite (remote).
type FileUploader interface {
	WriteFile(ctx context.Context, path string, data []byte, mode os.FileMode) error
}

type fileReader interface {
	ReadFile(ctx context.Context, path string) ([]byte, error)
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
			uploadData, mergeErr := credentialFileUploadData(ctx, uploader, method, targetPath, data)
			if mergeErr != nil {
				return fmt.Errorf("failed to prepare %s: %w", targetPath, mergeErr)
			}
			if err := uploader.WriteFile(ctx, targetPath, uploadData, credentialFileMode); err != nil {
				return fmt.Errorf("failed to upload %s: %w", targetPath, err)
			}
			log.Debug("uploaded credential file",
				zap.String("method_id", method.MethodID),
				zap.String("target", targetPath))
		}
	}

	return nil
}

func credentialFileUploadData(
	ctx context.Context,
	uploader FileUploader,
	method remoteauth.Method,
	targetPath string,
	source []byte,
) ([]byte, error) {
	if method.FileConflictPolicy != agents.RemoteAuthFileConflictPolicyMergeJSONObject {
		return source, nil
	}

	sourceObject, err := decodeCredentialJSONObject(source)
	if err != nil {
		return nil, fmt.Errorf("source is not a JSON object: %w", err)
	}
	reader, ok := uploader.(fileReader)
	if !ok {
		return json.Marshal(sourceObject)
	}
	target, err := reader.ReadFile(ctx, targetPath)
	if errors.Is(err, fs.ErrNotExist) {
		return json.Marshal(sourceObject)
	}
	if err != nil {
		return nil, fmt.Errorf("read existing target: %w", err)
	}
	targetObject, err := decodeCredentialJSONObject(target)
	if err != nil {
		return nil, fmt.Errorf("existing target is not a JSON object: %w", err)
	}
	for provider, credential := range sourceObject {
		targetObject[provider] = credential
	}
	return json.Marshal(targetObject)
}

func decodeCredentialJSONObject(data []byte) (map[string]json.RawMessage, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return nil, err
	}
	if object == nil {
		return nil, fmt.Errorf("value is null")
	}
	return object, nil
}
