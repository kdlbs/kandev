package opencodeplugin

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// StageArtifact materializes the embedded server plugin under root using a
// content-hash directory. An existing verified directory is reused; anything
// that does not match the manifest is replaced by an atomic rename.
func StageArtifact(root string) (string, error) {
	if root == "" {
		return "", errors.New("opencode plugin artifact root is empty")
	}
	manifest, err := ArtifactManifestValue()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", fmt.Errorf("create opencode plugin artifact root: %w", err)
	}
	target := filepath.Join(root, manifest.SHA256)
	if verified, verifyErr := verifyStagedArtifact(target, manifest.SHA256); verifyErr != nil {
		return "", verifyErr
	} else if verified {
		return target, nil
	}
	if err := os.RemoveAll(target); err != nil {
		return "", fmt.Errorf("remove stale opencode plugin artifact: %w", err)
	}
	staging, err := os.MkdirTemp(root, ".staging-")
	if err != nil {
		return "", fmt.Errorf("create opencode plugin staging directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(staging) }()
	for _, name := range []string{ArtifactEntrypoint, ArtifactPackageFile, ArtifactManifestFile} {
		raw, readErr := artifactFiles.ReadFile("assets/" + name)
		if readErr != nil {
			return "", fmt.Errorf("read embedded artifact %s: %w", name, readErr)
		}
		if writeErr := os.WriteFile(filepath.Join(staging, name), raw, 0o600); writeErr != nil {
			return "", fmt.Errorf("write staged artifact %s: %w", name, writeErr)
		}
	}
	if err := os.Rename(staging, target); err != nil {
		// A concurrent launch may have published the same immutable content.
		if verified, verifyErr := verifyStagedArtifact(target, manifest.SHA256); verifyErr == nil && verified {
			return target, nil
		}
		return "", fmt.Errorf("publish opencode plugin artifact: %w", err)
	}
	return target, nil
}

func verifyStagedArtifact(dir, expectedHash string) (bool, error) {
	info, err := os.Lstat(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("inspect staged opencode plugin artifact: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return false, nil
	}
	for _, name := range []string{ArtifactEntrypoint, ArtifactPackageFile, ArtifactManifestFile} {
		verified, verifyErr := stagedFileMatches(filepath.Join(dir, name), "assets/"+name)
		if verifyErr != nil || !verified {
			return verified, verifyErr
		}
	}
	raw, err := os.ReadFile(filepath.Join(dir, ArtifactEntrypoint))
	if err != nil {
		return false, fmt.Errorf("read staged opencode plugin entrypoint: %w", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]) == expectedHash, nil
}

func stagedFileMatches(path, embeddedPath string) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("inspect staged opencode plugin file: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return false, nil
	}
	staged, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("read staged opencode plugin file: %w", err)
	}
	embedded, err := artifactFiles.ReadFile(embeddedPath)
	if err != nil {
		return false, fmt.Errorf("read embedded opencode plugin file: %w", err)
	}
	return bytes.Equal(staged, embedded), nil
}
