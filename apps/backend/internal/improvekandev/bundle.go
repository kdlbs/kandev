// Package improvekandev exposes the HTTP endpoints that bootstrap a hidden
// improve-kandev workflow and lease an identity-owned diagnostic ZIP into the
// temporary context directory referenced by the resulting task.
package improvekandev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/system/storage"
	"github.com/kandev/kandev/internal/system/storage/tempartifacts"
)

const (
	bundlePrefix       = "kandev-improve-"
	ownerMarkerName    = ".kandev-owner.json"
	diagnosticFileName = "diagnostic-bundle.zip"
	bundleLeaseAge     = 24 * time.Hour
)

type ownerMarker struct {
	Owner string `json:"owner"`
}

// createBundleDir creates an owner-only temporary context directory and a
// server-owned marker used to authorize the later diagnostic-bundle lease.
func createBundleDir(owner string) (string, error) {
	if owner == "" {
		return "", errors.New("owner is required")
	}
	dir, err := os.MkdirTemp("", bundlePrefix+"*")
	if err != nil {
		return "", err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}
	data, err := json.Marshal(ownerMarker{Owner: owner})
	if err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, ownerMarkerName), data, 0o600); err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}
	return dir, nil
}

func createBundleDirWithRegistry(
	ctx context.Context,
	owner string,
	registry *tempartifacts.Registry,
) (string, error) {
	dir, err := createBundleDir(owner)
	if err != nil {
		return "", err
	}
	lease, err := registry.RegisterExisting(
		ctx, storage.TemporaryArtifactKindImproveBundle, dir, nil,
	)
	if err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}
	if err := lease.Close(ctx); err != nil {
		_ = lease.Remove(context.Background())
		return "", fmt.Errorf("close improve bundle artifact lease: %w", err)
	}
	return dir, nil
}

// validateBundleDir accepts only a server-created temp directory whose
// owner-only marker matches the authenticated caller.
func validateBundleDir(dir, owner string) (string, error) {
	if dir == "" {
		return "", errors.New("bundle_dir is required")
	}
	if owner == "" {
		return "", errors.New("authenticated owner is required")
	}
	abs, err := filepath.Abs(filepath.Clean(dir))
	if err != nil {
		return "", fmt.Errorf("invalid bundle_dir: %w", err)
	}
	tempDir, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		tempDir = os.TempDir()
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", errors.New("bundle_dir does not exist")
	}
	if !strings.HasPrefix(resolved, tempDir+string(os.PathSeparator)) {
		return "", errors.New("bundle_dir must be inside the OS temp directory")
	}
	if !strings.HasPrefix(filepath.Base(resolved), bundlePrefix) {
		return "", errors.New("bundle_dir name must start with " + bundlePrefix)
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", errors.New("bundle_dir does not exist or is not a directory")
	}
	markerPath := filepath.Join(resolved, ownerMarkerName)
	markerInfo, err := os.Lstat(markerPath)
	if err != nil || !markerInfo.Mode().IsRegular() {
		return "", errors.New("bundle_dir owner marker is missing")
	}
	var marker ownerMarker
	data, err := os.ReadFile(markerPath)
	if err != nil || json.Unmarshal(data, &marker) != nil || marker.Owner != owner {
		return "", errors.New("bundle_dir is not owned by the authenticated user")
	}
	return resolved, nil
}
