package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type storedAuditManifest struct {
	Version  int
	Binding  string
	Artifact string
	Sources  []AuditSourceReceipt
}

// StoreAuditArchive retains frozen evidence independently of runtime cleanup.
// Binding is a core-generated digest of the complete captured attempt context.
func StoreAuditArchive(ctx context.Context, base, binding string, archive *AuditArchive) (string, error) {
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if !validRuntimeDirectory(base) || !validLaunchDigest(binding) || archive == nil || !validStoredSources(archive.Sources, archive.Digest) {
		return "", ErrInvalidPolicy
	}
	key := storedAuditKey(binding, archive.Digest)
	if _, err := os.Lstat(filepath.Join(base, key)); err == nil {
		read, loadErr := LoadAuditArchive(ctx, base, base, key, binding, archive.Digest)
		if loadErr != nil {
			return "", loadErr
		}
		return key, read.Close()
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	temp, err := os.MkdirTemp(base, ".audit-store-")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(temp) }()
	if err = prepareStoredArchive(ctx, temp, binding, archive); err != nil {
		return "", err
	}
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err = os.Rename(temp, filepath.Join(base, key)); err != nil {
		return "", err
	}
	return key, syncAuditDirectory(base)
}

func prepareStoredArchive(ctx context.Context, temp, binding string, archive *AuditArchive) error {
	for index := range archive.Sources {
		if err := writeStoredAuditSource(ctx, temp, index, archive); err != nil {
			return err
		}
	}
	manifest := storedAuditManifest{Version: 1, Binding: binding, Artifact: archive.Digest, Sources: archive.Sources}
	raw, err := json.Marshal(manifest)
	if err != nil || len(raw) > 64<<10 {
		return ErrInvalidPolicy
	}
	if err = writeSyncedAuditFile(filepath.Join(temp, "manifest.json"), raw); err != nil {
		return err
	}
	return syncAuditDirectory(temp)
}

// LoadAuditArchive validates retained evidence and freezes a new private read.
// Closing the returned read never removes the retained historical archive.
func LoadAuditArchive(ctx context.Context, base, scratch, key, binding, artifact string) (*AuditArchive, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if !validRuntimeDirectory(base) || !validRuntimeDirectory(scratch) || !validLaunchDigest(key) || !validLaunchDigest(binding) || !validLaunchDigest(artifact) || key != storedAuditKey(binding, artifact) {
		return nil, ErrInvalidPolicy
	}
	root, err := openStoredAuditRoot(base, key)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	manifest, err := readStoredAuditManifest(root, binding, artifact)
	if err != nil {
		return nil, err
	}
	temp, err := os.MkdirTemp(scratch, "audit-read-")
	if err != nil {
		return nil, err
	}
	result := &AuditArchive{root: temp, Digest: artifact, Sources: manifest.Sources}
	for index, source := range manifest.Sources {
		if err = result.loadStoredSource(ctx, root, index, source); err != nil {
			return nil, errors.Join(err, result.Close())
		}
	}
	return result, nil
}

func storedAuditKey(binding, artifact string) string {
	return runtimeDigest([]byte(binding + "\x00" + artifact))
}

func writeStoredAuditSource(ctx context.Context, directory string, index int, archive *AuditArchive) error {
	file, err := os.OpenFile(filepath.Join(directory, fmt.Sprintf("source-%02d", index)), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0400)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	for offset := int64(0); ; {
		content, final, readErr := archive.ReadChunk(ctx, index, offset, 256<<10)
		if readErr != nil {
			return readErr
		}
		if _, err = file.Write(content); err != nil {
			return err
		}
		offset += int64(len(content))
		if final {
			break
		}
	}
	return file.Sync()
}

func writeSyncedAuditFile(path string, raw []byte) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0400)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	if _, err = file.Write(raw); err != nil {
		return err
	}
	return file.Sync()
}

func syncAuditDirectory(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	return file.Sync()
}
