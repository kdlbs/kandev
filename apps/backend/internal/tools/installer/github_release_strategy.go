package installer

import (
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

// GithubReleaseConfig configures a GitHub release download.
type GithubReleaseConfig struct {
	Owner        string            // e.g. "rust-lang"
	Repo         string            // e.g. "rust-analyzer"
	AssetPattern string            // e.g. "rust-analyzer-{target}.gz"
	Targets      map[string]string // "darwin/arm64" -> "aarch64-apple-darwin"
}

// GithubReleaseStrategy downloads prebuilt binaries from GitHub releases.
type GithubReleaseStrategy struct {
	binDir string
	binary string // "rust-analyzer"
	config GithubReleaseConfig
	logger *logger.Logger
}

// NewGithubReleaseStrategy creates a new GitHub release download strategy.
func NewGithubReleaseStrategy(binDir, binary string, config GithubReleaseConfig, log *logger.Logger) *GithubReleaseStrategy {
	return &GithubReleaseStrategy{
		binDir: binDir,
		binary: binary,
		config: config,
		logger: log,
	}
}

func (s *GithubReleaseStrategy) Name() string {
	return fmt.Sprintf("github release %s/%s", s.config.Owner, s.config.Repo)
}

func (s *GithubReleaseStrategy) Install(ctx context.Context) (*InstallResult, error) {
	// Resolve target from runtime OS/ARCH
	targetKey := runtime.GOOS + "/" + runtime.GOARCH
	target, ok := s.config.Targets[targetKey]
	if !ok {
		return nil, fmt.Errorf("unsupported platform: %s", targetKey)
	}

	// Build download URL
	asset := strings.Replace(s.config.AssetPattern, "{target}", target, 1)
	url := fmt.Sprintf("https://github.com/%s/%s/releases/latest/download/%s",
		s.config.Owner, s.config.Repo, asset)

	s.logger.Info("downloading from GitHub releases",
		zap.String("url", url),
		zap.String("target", target))

	// Download
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	// Ensure bin directory exists
	if err := os.MkdirAll(s.binDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create directory %s: %w", s.binDir, err)
	}

	outPath := filepath.Join(s.binDir, s.binary)
	if err := writeReleaseAsset(ctx, resp.Body, asset, outPath); err != nil {
		return nil, err
	}

	s.logger.Info("GitHub release download completed", zap.String("binary", outPath))
	return &InstallResult{
		BinaryPath: outPath,
	}, nil
}

func writeReleaseAsset(ctx context.Context, body io.Reader, asset, outPath string) error {
	tempFile, err := os.CreateTemp(filepath.Dir(outPath), "."+filepath.Base(outPath)+".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary download: %w", err)
	}
	tempPath := tempFile.Name()
	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("failed to close temporary download: %w", err)
	}
	defer func() { _ = os.Remove(tempPath) }()

	reader := &contextReader{ctx: ctx, reader: body}
	if strings.HasSuffix(asset, ".gz") {
		err = WriteGzipBody(reader, tempPath)
	} else {
		err = WriteBody(reader, tempPath)
	}
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Chmod(tempPath, 0o755); err != nil {
		return fmt.Errorf("failed to chmod: %w", err)
	}
	if err := os.Rename(tempPath, outPath); err != nil {
		return fmt.Errorf("failed to publish downloaded binary: %w", err)
	}
	return nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(data []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(data)
}

// WriteGzipBody decompresses a gzip reader into outPath.
func WriteGzipBody(body io.Reader, outPath string) error {
	gzReader, err := gzip.NewReader(body)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer func() { _ = gzReader.Close() }()

	outFile, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer func() { _ = outFile.Close() }()

	if _, err := io.Copy(outFile, gzReader); err != nil {
		return fmt.Errorf("failed to decompress: %w", err)
	}
	return nil
}

// WriteBody writes a plain reader into outPath.
func WriteBody(body io.Reader, outPath string) error {
	outFile, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer func() { _ = outFile.Close() }()

	if _, err := io.Copy(outFile, body); err != nil {
		return fmt.Errorf("failed to write binary: %w", err)
	}
	return nil
}
