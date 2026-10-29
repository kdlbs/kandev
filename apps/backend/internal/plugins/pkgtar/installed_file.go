package pkgtar

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
)

const maxInstalledArtifactBytes int64 = 128 << 20

type InstalledFileSpec struct {
	Name          string
	PackageDigest string
	Digest        string
	MaxBytes      int64
}

// ReadInstalledFile returns bytes bound to the recorded package and file digests.
func ReadInstalledFile(ctx context.Context, installPath string, spec InstalledFileSpec) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	name, err := cleanArchivePath(spec.Name)
	if err != nil || name != spec.Name || !filepath.IsAbs(installPath) {
		return nil, ErrPathTraversal
	}
	if spec.MaxBytes <= 0 || spec.MaxBytes > maxInstalledArtifactBytes || len(spec.PackageDigest) != 64 || len(spec.Digest) != 64 {
		return nil, ErrChecksumMismatch
	}
	root, err := os.OpenRoot(installPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	if err := verifyInstalledManifest(ctx, root, spec); err != nil {
		return nil, err
	}
	content, err := readInstalledRootFile(ctx, root, filepath.FromSlash(name), spec.MaxBytes)
	if err != nil {
		return nil, err
	}
	if !installedDigestMatches(content, spec.Digest) {
		return nil, ErrChecksumMismatch
	}
	return content, nil
}

func verifyInstalledManifest(ctx context.Context, root *os.Root, spec InstalledFileSpec) error {
	manifest, err := readInstalledRootFile(ctx, root, checksumsFileName, 1<<20)
	if err != nil {
		return err
	}
	if !installedDigestMatches(manifest, spec.PackageDigest) {
		return ErrChecksumMismatch
	}
	entries, err := parseChecksums(manifest)
	if err != nil {
		return err
	}
	if entries[spec.Name] != spec.Digest {
		return ErrChecksumMismatch
	}
	return nil
}

func installedDigestMatches(content []byte, expected string) bool {
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:]) == expected
}

func readInstalledRootFile(ctx context.Context, root *os.Root, name string, limit int64) ([]byte, error) {
	original, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !original.Mode().IsRegular() || original.Size() > limit {
		return nil, ErrPathTraversal
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	opened, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(original, opened) || !opened.Mode().IsRegular() {
		return nil, ErrPathTraversal
	}
	content, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if int64(len(content)) > limit {
		return nil, ErrChecksumMismatch
	}
	return content, nil
}
