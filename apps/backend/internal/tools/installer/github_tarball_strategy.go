package installer

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// GithubTarballConfig configures a GitHub release tarball download.
type GithubTarballConfig struct {
	Owner        string            // e.g. "coder"
	Repo         string            // e.g. "code-server"
	Version      string            // e.g. "4.96.4"
	AssetPattern string            // e.g. "code-server-{version}-{os}-{arch}.tar.gz"
	BinaryPath   string            // relative path inside tarball, e.g. "code-server-{version}-{os}-{arch}/bin/code-server"
	Targets      map[string]string // "darwin/arm64" -> "macos-arm64", "linux/amd64" -> "linux-amd64"
}

// GithubTarballStrategy downloads and extracts tar.gz archives from GitHub releases.
type GithubTarballStrategy struct {
	installDir string // base directory for extracted files
	binary     string // binary name for logging
	config     GithubTarballConfig
	logger     *logger.Logger
}

// NewGithubTarballStrategy creates a new GitHub tarball download strategy.
func NewGithubTarballStrategy(installDir, binary string, config GithubTarballConfig, log *logger.Logger) *GithubTarballStrategy {
	return &GithubTarballStrategy{
		installDir: installDir,
		binary:     binary,
		config:     config,
		logger:     log,
	}
}

func (s *GithubTarballStrategy) Name() string {
	return fmt.Sprintf("github tarball %s/%s v%s", s.config.Owner, s.config.Repo, s.config.Version)
}

func (s *GithubTarballStrategy) Install(ctx context.Context) (*InstallResult, error) {
	target, err := s.resolveTarget()
	if err != nil {
		return nil, err
	}

	binaryPath := s.resolveBinaryPath(target)
	completionMarker := binaryPath + ".install-complete"
	binaryExists, err := pathExists(binaryPath)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect installed binary %s: %w", binaryPath, err)
	}
	markerExists, err := pathExists(completionMarker)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect install completion marker %s: %w", completionMarker, err)
	}

	// The archive's entrypoint can be extracted before its runtime dependencies.
	// Only a marker written after the full extraction proves the install is usable.
	if binaryExists && markerExists {
		s.logger.Info("binary already installed, skipping download", zap.String("binary", binaryPath))
		return &InstallResult{BinaryPath: binaryPath}, nil
	}
	if binaryExists {
		s.logger.Warn("incomplete binary install found, reinstalling", zap.String("binary", binaryPath))
	}
	if err := os.Remove(completionMarker); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("failed to clear install completion marker: %w", err)
	}

	url := s.buildURL(target)
	s.logger.Info("downloading tarball from GitHub releases",
		zap.String("url", url),
		zap.String("target", target))

	if err := s.download(ctx, url); err != nil {
		return nil, err
	}

	if _, err := os.Stat(binaryPath); err != nil {
		return nil, fmt.Errorf("binary not found after extraction %s: %w", binaryPath, err)
	}
	if err := os.WriteFile(completionMarker, nil, 0o644); err != nil {
		return nil, fmt.Errorf("failed to write install completion marker: %w", err)
	}

	s.logger.Info("tarball install completed", zap.String("binary", binaryPath))
	return &InstallResult{BinaryPath: binaryPath}, nil
}

func pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func (s *GithubTarballStrategy) resolveTarget() (string, error) {
	targetKey := runtime.GOOS + "/" + runtime.GOARCH
	target, ok := s.config.Targets[targetKey]
	if !ok {
		return "", fmt.Errorf("unsupported platform: %s", targetKey)
	}
	return target, nil
}

func (s *GithubTarballStrategy) buildURL(target string) string {
	asset := s.expandTemplate(s.config.AssetPattern, target)
	return fmt.Sprintf("https://github.com/%s/%s/releases/download/v%s/%s",
		s.config.Owner, s.config.Repo, s.config.Version, asset)
}

func (s *GithubTarballStrategy) resolveBinaryPath(target string) string {
	relPath := s.expandTemplate(s.config.BinaryPath, target)
	return filepath.Join(s.installDir, relPath)
}

func (s *GithubTarballStrategy) expandTemplate(tmpl, target string) string {
	r := strings.NewReplacer(
		"{version}", s.config.Version,
		"{os}-{arch}", target,
	)
	return r.Replace(tmpl)
}

func (s *GithubTarballStrategy) download(ctx context.Context, url string) error {
	resp, err := getDownload(ctx, url)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d for %s", resp.StatusCode, url)
	}

	if err := os.MkdirAll(s.installDir, 0o755); err != nil {
		return fmt.Errorf("failed to create install directory %s: %w", s.installDir, err)
	}

	return extractTarGz(resp.Body, s.installDir)
}

// extractTarGz decompresses and extracts a tar.gz stream into destDir.
//
// Every write goes through an os.Root anchored at destDir, so containment is
// enforced by the OS-level path walk rather than by lexical validation of the
// entry name alone. Lexical checks are not sufficient on their own: an archive
// can ship a symlink whose target passes containment (e.g. "a" -> ".") and then
// write through it with a later entry that really resolves outside destDir.
func extractTarGz(r io.Reader, destDir string) error {
	gzReader, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer func() { _ = gzReader.Close() }()

	root, err := os.OpenRoot(destDir)
	if err != nil {
		return fmt.Errorf("failed to open destination %s: %w", destDir, err)
	}
	defer func() { _ = root.Close() }()

	tarReader := tar.NewReader(gzReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar read error: %w", err)
		}

		if err := extractTarEntry(tarReader, header, root); err != nil {
			return err
		}
	}
	return nil
}

func extractTarEntry(tr *tar.Reader, header *tar.Header, root *os.Root) error {
	cleanName, err := sanitizeTarPath(header.Name, root.Name())
	if err != nil {
		return err
	}

	switch header.Typeflag {
	case tar.TypeDir:
		return root.MkdirAll(cleanName, os.FileMode(header.Mode))
	case tar.TypeReg:
		return writeFileFromTar(tr, root, cleanName, os.FileMode(header.Mode))
	case tar.TypeSymlink:
		return extractSymlinkEntry(root, cleanName, header)
	default:
		// Skip unsupported types (block devices, char devices, etc.)
		return nil
	}
}

// extractSymlinkEntry creates header's symlink under root. root.Symlink already
// confines where the link itself lands, and root refuses to follow a link out of
// the tree later; rejecting escaping targets up front keeps the archive honest
// and surfaces a clear error instead of a confusing failure further along.
func extractSymlinkEntry(root *os.Root, cleanName string, header *tar.Header) error {
	if filepath.IsAbs(header.Linkname) {
		return fmt.Errorf("symlink target must not be absolute: %s -> %s", header.Name, header.Linkname)
	}

	if err := rejectSymlinkedParents(root, cleanName); err != nil {
		return err
	}

	// Resolve the target relative to the symlink's own directory, both
	// expressed relative to destDir. Anything that climbs to ".." has left it.
	// Sound only because the parent check above pins the link's on-disk
	// location to its archive path.
	linkTarget := filepath.Join(filepath.Dir(cleanName), header.Linkname)
	if linkTarget == ".." || strings.HasPrefix(linkTarget, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("symlink target escapes destination: %s -> %s", header.Name, header.Linkname)
	}

	// Remove existing symlink/file before creating (handles re-installs)
	_ = root.Remove(cleanName)
	return root.Symlink(header.Linkname, cleanName)
}

// rejectSymlinkedParents requires every parent component of name to be a real
// directory rather than a symlink.
//
// root.Symlink follows symlinked parents, so an entry named "a/x" lands at "x"
// when "a" is itself a symlink to ".". The link is still created inside destDir
// — root guarantees that much — but one directory level shallower than its
// archive path, which changes what a relative target resolves to: "a/x" -> ".."
// passes a check computed against parent "a" and then resolves to destDir's
// parent from its real home. Nothing can be written through such a link during
// extraction (root refuses to follow it), but it persists in the install tree
// for any later consumer that walks it with ordinary os calls.
//
// Requiring real parents keeps an entry's archive path and its on-disk location
// identical, which is the assumption the target check depends on.
func rejectSymlinkedParents(root *os.Root, name string) error {
	dir := filepath.Dir(name)
	if dir == "." {
		return nil
	}

	prefix := ""
	for _, part := range strings.Split(filepath.ToSlash(dir), "/") {
		if prefix == "" {
			prefix = part
		} else {
			prefix += "/" + part
		}

		info, err := root.Lstat(prefix)
		if os.IsNotExist(err) {
			// Not created yet; root.Symlink will fail on the missing parent
			// rather than resolving through anything unexpected.
			continue
		}
		if err != nil {
			return fmt.Errorf("failed to inspect symlink parent %s: %w", prefix, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink %s has a symlinked parent directory: %s", name, prefix)
		}
	}
	return nil
}

func writeFileFromTar(tr *tar.Reader, root *os.Root, name string, mode os.FileMode) error {
	if dir := filepath.Dir(name); dir != "." {
		if err := root.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("failed to create parent directory: %w", err)
		}
	}

	f, err := root.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", name, err)
	}
	defer func() { _ = f.Close() }()

	// Limit copy size to prevent decompression bombs (1 GB)
	const maxFileSize = 1 << 30
	if _, err := io.Copy(f, io.LimitReader(tr, maxFileSize)); err != nil {
		return fmt.Errorf("failed to write file %s: %w", name, err)
	}
	return nil
}

// sanitizeTarPath prevents path traversal attacks in tar archives.
func sanitizeTarPath(name, destDir string) (string, error) {
	cleanName := filepath.Clean(name)
	if strings.HasPrefix(cleanName, "..") || strings.HasPrefix(cleanName, "/") {
		return "", fmt.Errorf("invalid tar entry path: %s", name)
	}
	absTarget := filepath.Join(destDir, cleanName)
	if !strings.HasPrefix(absTarget, filepath.Clean(destDir)+string(os.PathSeparator)) && absTarget != filepath.Clean(destDir) {
		return "", fmt.Errorf("tar entry %s would escape destination directory", name)
	}
	return cleanName, nil
}
