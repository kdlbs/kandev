package sandbox

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
)

func verifyRuntimeFiles(rootPath string, digests map[string]string) error {
	if len(digests) == 0 || len(digests) > 33 || !validLaunchDigest(digests["runtime"]) {
		return ErrInvalidPolicy
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return ErrInvalidPolicy
	}
	defer func() { _ = root.Close() }()
	for name, digest := range digests {
		if !validLaunchDigest(digest) || (name != "runtime" && !validRuntimeArtifactName(name)) {
			return ErrInvalidPolicy
		}
		if err := verifyRuntimeFile(root, rootPath, name, digest); err != nil {
			return err
		}
	}
	return nil
}

func verifyRuntimeFile(root *os.Root, rootPath, name, digest string) error {
	fullPath := filepath.Join(rootPath, filepath.FromSlash(name))
	canonical, err := filepath.EvalSymlinks(fullPath)
	if err != nil || canonical != fullPath {
		return ErrInvalidPolicy
	}
	info, err := root.Lstat(name)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0222 != 0 {
		return ErrInvalidPolicy
	}
	limit := int64(maxArtifactBytes)
	if name == "runtime" {
		limit = 128 << 20
		if info.Mode().Perm()&0100 == 0 {
			return ErrInvalidPolicy
		}
	}
	if info.Size() > limit {
		return ErrInvalidPolicy
	}
	file, err := root.Open(name)
	if err != nil {
		return ErrInvalidPolicy
	}
	defer func() { _ = file.Close() }()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return ErrInvalidPolicy
	}
	hash := sha256.New()
	count, err := io.Copy(hash, io.LimitReader(file, limit+1))
	if err != nil || count > limit || hex.EncodeToString(hash.Sum(nil)) != digest {
		return ErrInvalidPolicy
	}
	return nil
}
