package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/remoteconfig"
	"github.com/kandev/kandev/internal/common/logger"
)

// selectedPortableConfigBundleIDs decodes the executor-profile value without
// accepting file paths or file data from task metadata.
func selectedPortableConfigBundleIDs(metadata map[string]interface{}) []string {
	if metadata == nil {
		return nil
	}
	raw, ok := metadata[MetadataKeyAgentConfigBundles]
	if !ok {
		return nil
	}
	if value, ok := raw.(string); ok {
		var ids []string
		if json.Unmarshal([]byte(value), &ids) == nil {
			return ids
		}
		return nil
	}
	if values, ok := raw.([]string); ok {
		return append([]string(nil), values...)
	}
	values, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	ids := make([]string, 0, len(values))
	for _, value := range values {
		if id, ok := value.(string); ok && id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

const (
	portableConfigMaxFileBytes           int64 = 1 << 20
	portableConfigMaxLaunchBytes         int64 = 4 << 20
	portableConfigReasonUnsafeSourcePath       = "unsafe_source_path"
)

const portableConfigFileMode os.FileMode = 0o600

// PortableConfigWarning describes an optional configuration copy that was
// skipped. It deliberately contains paths relative to the host home only.
type PortableConfigWarning struct {
	BundleID   string `json:"bundle_id"`
	SourcePath string `json:"source_path,omitempty"`
	TargetPath string `json:"target_path,omitempty"`
	Reason     string `json:"reason"`
}

func reportPortableConfigWarnings(onProgress PrepareProgressCallback, warnings []PortableConfigWarning) {
	if onProgress == nil || len(warnings) == 0 {
		return
	}
	step := beginStep("Copy agent configuration")
	details := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		details = append(details, fmt.Sprintf("%s: %s", warning.SourcePath, warning.Reason))
	}
	step.Warning = details[0]
	if len(details) > 1 {
		step.WarningDetail = strings.Join(details, "\n")
	}
	completeStepSuccess(&step)
	onProgress(step, 0, 0)
}

// UploadPortableConfigBundles copies selected, backend-declared configuration
// files from the host home into an executor home. Every copy is optional:
// invalid, missing, oversized, or failed files produce warnings and the next
// file continues. The raw data never enters metadata or the browser.
func UploadPortableConfigBundles(
	ctx context.Context,
	uploader FileUploader,
	ag agents.Agent,
	selectedIDs []string,
	targetHomeDir string,
	log *logger.Logger,
) []PortableConfigWarning {
	if ag == nil || uploader == nil || len(selectedIDs) == 0 || targetHomeDir == "" {
		return nil
	}
	hostHome, err := os.UserHomeDir()
	if err != nil || hostHome == "" {
		return []PortableConfigWarning{{Reason: "host_home_unavailable"}}
	}
	catalog := remoteconfig.BuildCatalogForHost([]agents.Agent{ag}, runtime.GOOS, hostHome)
	warnings := make([]PortableConfigWarning, 0)
	var copiedBytes int64
	seen := make(map[string]struct{}, len(selectedIDs))
	for _, bundleID := range selectedIDs {
		if _, exists := seen[bundleID]; exists {
			continue
		}
		seen[bundleID] = struct{}{}
		bundle, ok := catalog.FindBundle(bundleID)
		if !ok {
			warnings = append(warnings, PortableConfigWarning{BundleID: bundleID, Reason: "unknown_bundle"})
			continue
		}
		for _, file := range bundle.Files {
			warning, copied, size := uploadPortableConfigFile(
				ctx, uploader, hostHome, targetHomeDir, bundle.ID, file, copiedBytes,
			)
			if warning.Reason != "" {
				warnings = append(warnings, warning)
				if log != nil {
					log.Warn("portable agent configuration copy skipped",
						zap.String("bundle_id", warning.BundleID),
						zap.String("source_path", warning.SourcePath),
						zap.String("reason", warning.Reason))
				}
			}
			if copied {
				copiedBytes += size
			}
		}
	}
	return warnings
}

func uploadPortableConfigFile(
	ctx context.Context,
	uploader FileUploader,
	hostHome, targetHome, bundleID string,
	file remoteconfig.File,
	copiedBytes int64,
) (PortableConfigWarning, bool, int64) {
	warning := PortableConfigWarning{
		BundleID:   bundleID,
		SourcePath: file.SourcePath,
		TargetPath: file.TargetPath,
	}
	if !isSafePortableRelativePath(file.SourcePath) {
		warning.Reason = portableConfigReasonUnsafeSourcePath
		return warning, false, 0
	}
	if !isSafePortableRelativePath(file.TargetPath) {
		warning.Reason = "unsafe_target_path"
		return warning, false, 0
	}
	sourcePath, err := containedPath(hostHome, filepath.Join(hostHome, filepath.FromSlash(file.SourcePath)))
	if err != nil {
		warning.Reason = portableConfigReasonUnsafeSourcePath
		return warning, false, 0
	}
	targetPath, err := containedPath(targetHome, filepath.Join(targetHome, filepath.FromSlash(file.TargetPath)))
	if err != nil {
		warning.Reason = "unsafe_target_path"
		return warning, false, 0
	}
	info, reason := portableConfigSourceInfo(hostHome, sourcePath)
	if reason != "" {
		warning.Reason = reason
		return warning, false, 0
	}
	if info.Size() > portableConfigMaxFileBytes {
		warning.Reason = "file_too_large"
		return warning, false, 0
	}
	if copiedBytes > portableConfigMaxLaunchBytes-info.Size() {
		warning.Reason = "launch_size_limit"
		return warning, false, 0
	}
	data, err := readPortableConfigFile(sourcePath, info)
	if err != nil {
		warning.Reason = "source_read_failed"
		return warning, false, 0
	}
	if err := uploader.WriteFile(ctx, targetPath, data, portableConfigFileMode); err != nil {
		warning.Reason = "target_write_failed"
		return warning, false, 0
	}
	return PortableConfigWarning{}, true, int64(len(data))
}

func portableConfigSourceInfo(root, path string) (os.FileInfo, string) {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, portableConfigReasonUnsafeSourcePath
	}

	current := filepath.Clean(root)
	var info os.FileInfo
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err = os.Lstat(current)
		if err != nil {
			return nil, "source_missing"
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, "source_symlink"
		}
	}
	if info == nil || !info.Mode().IsRegular() {
		return nil, "source_not_regular"
	}
	return info, ""
}

func readPortableConfigFile(path string, expectedInfo os.FileInfo) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	actualInfo, err := file.Stat()
	if err != nil || !actualInfo.Mode().IsRegular() || !os.SameFile(expectedInfo, actualInfo) {
		return nil, fmt.Errorf("file changed while opening")
	}
	data, err := io.ReadAll(io.LimitReader(file, portableConfigMaxFileBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > portableConfigMaxFileBytes || int64(len(data)) != expectedInfo.Size() {
		return nil, fmt.Errorf("file changed while reading")
	}
	return data, nil
}

func isSafePortableRelativePath(path string) bool {
	if path == "" || filepath.IsAbs(filepath.FromSlash(path)) {
		return false
	}
	clean := filepath.Clean(filepath.FromSlash(path))
	return clean != "." && clean != ".." && !strings.HasPrefix(clean, ".."+string(filepath.Separator))
}
