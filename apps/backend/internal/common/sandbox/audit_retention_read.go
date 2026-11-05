package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func openStoredAuditRoot(base, key string) (*os.Root, error) {
	parent, err := os.OpenRoot(base)
	if err != nil {
		return nil, err
	}
	defer func() { _ = parent.Close() }()
	info, err := parent.Lstat(key)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, ErrInvalidPolicy
	}
	root, err := parent.OpenRoot(key)
	if err != nil {
		return nil, err
	}
	after, err := root.Stat(".")
	if err != nil || !os.SameFile(info, after) {
		_ = root.Close()
		return nil, ErrInvalidPolicy
	}
	return root, nil
}

func readStoredAuditManifest(root *os.Root, binding, artifact string) (*storedAuditManifest, error) {
	file, info, err := openStoredAuditFile(root, "manifest.json", 64<<10)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	raw, err := io.ReadAll(io.LimitReader(file, 1+64<<10))
	if err != nil || int64(len(raw)) != info.Size() {
		return nil, ErrInvalidPolicy
	}
	var manifest storedAuditManifest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&manifest) != nil || decoder.Decode(new(any)) != io.EOF || !validStoredManifest(manifest, binding, artifact) {
		return nil, ErrInvalidPolicy
	}
	canonical, err := json.Marshal(manifest)
	if err != nil || !bytes.Equal(canonical, raw) {
		return nil, ErrInvalidPolicy
	}
	entries, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer func() { _ = entries.Close() }()
	names, err := entries.ReadDir(maximumAuditArchiveSources + 2)
	if err != nil && err != io.EOF || len(names) != len(manifest.Sources)+1 {
		return nil, ErrInvalidPolicy
	}
	return &manifest, nil
}

func validStoredManifest(manifest storedAuditManifest, binding, artifact string) bool {
	return manifest.Version == 1 && manifest.Binding == binding && manifest.Artifact == artifact && validStoredSources(manifest.Sources, artifact)
}

func validStoredSources(sources []AuditSourceReceipt, artifact string) bool {
	if len(sources) == 0 || len(sources) > maximumAuditArchiveSources || !validLaunchDigest(artifact) {
		return false
	}
	var total int64
	seen := map[string]bool{}
	for _, source := range sources {
		if !validStoredSource(source) || seen[source.ID] {
			return false
		}
		total += source.Size
		seen[source.ID] = true
	}
	digest, err := AuditArtifactSetDigest(sources)
	return err == nil && digest == artifact && total <= maximumAuditArchiveBytes
}

func validStoredSource(source AuditSourceReceipt) bool {
	return validLaunchDigest(source.ID) && validLaunchDigest(source.Digest) && source.Size >= 0 && source.Size <= maximumAuditArchiveBytes && source.Schema != "" && len(source.Schema) <= 128 && source.Identity != "" && len(source.Identity) <= 256 && filepath.IsLocal(source.Name) && !strings.ContainsAny(source.Name, "\\\x00")
}

func openStoredAuditFile(root *os.Root, name string, limit int64) (*os.File, os.FileInfo, error) {
	info, err := root.Lstat(name)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() < 0 || info.Size() > limit {
		return nil, nil, ErrInvalidPolicy
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, nil, err
	}
	after, err := file.Stat()
	if err != nil || !os.SameFile(info, after) {
		_ = file.Close()
		return nil, nil, ErrInvalidPolicy
	}
	return file, after, nil
}

func (a *AuditArchive) loadStoredSource(ctx context.Context, root *os.Root, index int, source AuditSourceReceipt) error {
	original, info, err := openStoredAuditFile(root, fmt.Sprintf("source-%02d", index), source.Size)
	if err != nil {
		return err
	}
	defer func() { _ = original.Close() }()
	if info.Size() != source.Size {
		return ErrInvalidPolicy
	}
	copied, err := os.CreateTemp(a.root, "source-")
	if err != nil {
		return err
	}
	digest, err := copyFrozenAudit(ctx, original, copied, info, source.Size)
	if err != nil || digest != source.Digest {
		_ = copied.Close()
		return ErrInvalidPolicy
	}
	a.files = append(a.files, copied)
	return nil
}
