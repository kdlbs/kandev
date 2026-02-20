package installer

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
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

	// Skip download if the binary already exists from a previous install.
	if _, err := os.Stat(binaryPath); err == nil {
		s.logger.Info("binary already installed, skipping download", zap.String("binary", binaryPath))
		return &InstallResult{BinaryPath: binaryPath}, nil
	}

	url := s.buildURL(target)
	s.logger.Info("downloading tarball from GitHub releases",
		zap.String("url", url),
		zap.String("target", target))

	if err := s.download(ctx, url); err != nil {
		return nil, err
	}

	if _, err := os.Stat(binaryPath); err != nil {
		return nil, fmt.Errorf("binary not found after extraction: %s", binaryPath)
	}

	s.logger.Info("tarball install completed", zap.String("binary", binaryPath))
	return &InstallResult{BinaryPath: binaryPath}, nil
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
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
func extractTarGz(r io.Reader, destDir string) error {
	gzReader, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer func() { _ = gzReader.Close() }()

	tarReader := tar.NewReader(gzReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar read error: %w", err)
		}

		if err := extractTarEntry(tarReader, header, destDir); err != nil {
			return err
		}
	}
	return nil
}

func extractTarEntry(tr *tar.Reader, header *tar.Header, destDir string) error {
	cleanName, err := sanitizeTarPath(header.Name, destDir)
	if err != nil {
		return err
	}

	target := filepath.Join(destDir, cleanName)

	switch header.Typeflag {
	case tar.TypeDir:
		return os.MkdirAll(target, os.FileMode(header.Mode))
	case tar.TypeReg:
		return writeFileFromTar(tr, target, os.FileMode(header.Mode))
	case tar.TypeSymlink:
		// Validate symlink target doesn't escape destDir
		linkTarget := filepath.Join(filepath.Dir(target), header.Linkname)
		if !strings.HasPrefix(filepath.Clean(linkTarget), filepath.Clean(destDir)) {
			return fmt.Errorf("symlink %s -> %s escapes install directory", header.Name, header.Linkname)
		}
		// Remove existing symlink/file before creating (handles re-installs)
		_ = os.Remove(target)
		return os.Symlink(header.Linkname, target)
	default:
		// Skip unsupported types (block devices, char devices, etc.)
		return nil
	}
}

func writeFileFromTar(tr *tar.Reader, target string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", target, err)
	}
	defer func() { _ = f.Close() }()

	// Limit copy size to prevent decompression bombs (1 GB)
	const maxFileSize = 1 << 30
	if _, err := io.Copy(f, io.LimitReader(tr, maxFileSize)); err != nil {
		return fmt.Errorf("failed to write file %s: %w", target, err)
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
