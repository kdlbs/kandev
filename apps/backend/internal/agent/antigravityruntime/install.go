package antigravityruntime

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	downloadTimeout = 15 * time.Minute
	maxArchiveFiles = 16
)

// InstallOptions supplies deterministic inputs for a managed installation.
type InstallOptions struct {
	GOOS       string
	GOARCH     string
	HomeDir    string
	HTTPClient *http.Client
	Artifact   *Artifact
	Progress   func(string)

	afterDownload func()
	afterExtract  func()
	duringExtract func()
}

// Install downloads, verifies, extracts, and atomically publishes the reviewed
// bundle. A valid existing destination is an idempotent success; a corrupt one
// is never overwritten.
func Install(ctx context.Context, opts InstallOptions) (*Bundle, error) {
	artifact, homeDir, dest, err := installTarget(opts)
	if err != nil {
		return nil, err
	}
	if existing, err := existingBundle(ctx, dest, artifact); existing != nil || err != nil {
		return existing, err
	}
	root := ManagedRoot(homeDir)
	if err := ensureSafeDir(filepath.Dir(homeDir), root); err != nil {
		return nil, fmt.Errorf("create antigravity runtime cache: %w", err)
	}
	staging, err := os.MkdirTemp(root, ".staging-")
	if err != nil {
		return nil, fmt.Errorf("create antigravity install staging directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(staging) }()
	progressf(opts.Progress, "Downloading Antigravity ACP %s for %s/%s", artifact.Version, artifact.OS, artifact.Arch)
	archivePath := filepath.Join(staging, "artifact.zip")
	if err := downloadArchive(ctx, opts.HTTPClient, artifact, archivePath); err != nil {
		return nil, err
	}
	if opts.afterDownload != nil {
		opts.afterDownload()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	progressf(opts.Progress, "Extracting verified Antigravity ACP bundle")
	bundleDir := filepath.Join(staging, "bundle")
	if err := os.MkdirAll(bundleDir, 0o755); err != nil {
		return nil, fmt.Errorf("create bundle directory: %w", err)
	}
	if err := extractArchive(ctx, archivePath, bundleDir, artifact, opts.duringExtract); err != nil {
		return nil, err
	}
	if opts.afterExtract != nil {
		opts.afterExtract()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := writeManifest(bundleDir, artifact); err != nil {
		return nil, err
	}
	return publishBundle(ctx, bundleDir, dest, artifact, root)
}

func installTarget(opts InstallOptions) (Artifact, string, string, error) {
	goos, goarch := opts.GOOS, opts.GOARCH
	if goos == "" {
		goos = runtime.GOOS
	}
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	artifact, err := ArtifactFor(goos, goarch)
	if err != nil {
		return Artifact{}, "", "", err
	}
	if opts.Artifact != nil {
		artifact = *opts.Artifact
	}
	if artifact.OS == "" {
		artifact.OS = goos
	}
	if artifact.Arch == "" {
		artifact.Arch = goarch
	}
	homeDir := opts.HomeDir
	if homeDir == "" {
		homeDir = DefaultHomeDir()
	}
	if homeDir == "" {
		return Artifact{}, "", "", fmt.Errorf("%w: cannot resolve the Kandev home directory", ErrUnavailable)
	}
	return artifact, homeDir, ManagedDestination(homeDir, artifact), nil
}

func existingBundle(ctx context.Context, dest string, artifact Artifact) (*Bundle, error) {
	info, err := os.Stat(dest)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%w: inspect %s: %v", ErrInvalidManagedBundle, dest, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%w: %s is not a directory", ErrInvalidManagedBundle, dest)
	}
	return validateManagedBundle(ctx, dest, artifact)
}

func publishBundle(ctx context.Context, bundleDir, dest string, artifact Artifact, root string) (*Bundle, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := ensureSafeDir(root, filepath.Dir(dest)); err != nil {
		return nil, fmt.Errorf("create managed runtime parent: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := os.Rename(bundleDir, dest); err != nil {
		if info, statErr := os.Stat(dest); statErr == nil && info.IsDir() {
			return validateManagedBundle(ctx, dest, artifact)
		}
		return nil, fmt.Errorf("publish antigravity bundle: %w", err)
	}
	return validateManagedBundle(ctx, dest, artifact)
}

func downloadArchive(ctx context.Context, client *http.Client, artifact Artifact, dest string) error {
	if client == nil {
		client = &http.Client{Timeout: downloadTimeout}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, artifact.URL, nil)
	if err != nil {
		return fmt.Errorf("create artifact request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download antigravity artifact: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download antigravity artifact: unexpected HTTP %d", resp.StatusCode)
	}
	file, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("create artifact file: %w", err)
	}
	hash := sha256.New()
	limited := io.LimitReader(resp.Body, artifact.CompressedLimit+1)
	written, copyErr := io.Copy(io.MultiWriter(file, hash), limited)
	syncErr := file.Sync()
	closeErr := file.Close()
	if copyErr != nil {
		return fmt.Errorf("write antigravity artifact: %w", copyErr)
	}
	if syncErr != nil {
		return fmt.Errorf("sync antigravity artifact: %w", syncErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close antigravity artifact: %w", closeErr)
	}
	if written > artifact.CompressedLimit {
		return fmt.Errorf("%w: compressed archive exceeds %d bytes", ErrSizeLimit, artifact.CompressedLimit)
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(digest, artifact.SHA256) {
		return fmt.Errorf("%w: got %s", ErrDigestMismatch, digest)
	}
	return nil
}

func extractArchive(ctx context.Context, archivePath, destDir string, artifact Artifact, hook func()) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("%w: open archive: %v", ErrUnsafeArchive, err)
	}
	defer func() { _ = reader.Close() }()
	if len(reader.File) > maxArchiveFiles {
		return fmt.Errorf("%w: too many archive entries", ErrUnsafeArchive)
	}
	expected := expectedArchiveNames(artifact)
	seen := make(map[string]bool, len(expected))
	var expanded int64
	for _, entry := range reader.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		base, size, err := validateArchiveEntry(entry, expected, seen, artifact.ExpandedLimit)
		if err != nil {
			return err
		}
		if base == "" {
			continue
		}
		expanded += size
		if expanded > artifact.ExpandedLimit {
			return fmt.Errorf("%w: expanded archive exceeds %d bytes", ErrSizeLimit, artifact.ExpandedLimit)
		}
		if err := extractEntry(ctx, entry, filepath.Join(destDir, base), artifact.ExpandedLimit); err != nil {
			return err
		}
		if hook != nil {
			hook()
		}
	}
	for base := range expected {
		if !seen[base] {
			return fmt.Errorf("%w: archive is missing %s", ErrUnsafeArchive, base)
		}
	}
	return nil
}

func expectedArchiveNames(artifact Artifact) map[string]string {
	expected := map[string]string{artifact.ServerName: artifact.ServerName}
	for _, name := range artifact.HarnessNames {
		expected[name] = name
	}
	return expected
}

func validateArchiveEntry(
	entry *zip.File,
	expected map[string]string,
	seen map[string]bool,
	expandedLimit int64,
) (string, int64, error) {
	name := strings.ReplaceAll(entry.Name, "\\", "/")
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, "../") || name == ".." {
		return "", 0, fmt.Errorf("%w: unsafe entry %q", ErrUnsafeArchive, entry.Name)
	}
	if entry.FileInfo().IsDir() {
		return "", 0, nil
	}
	if entry.Mode()&os.ModeSymlink != 0 || !entry.Mode().IsRegular() {
		return "", 0, fmt.Errorf("%w: entry %q is not a regular file", ErrUnsafeArchive, entry.Name)
	}
	base := filepath.Base(name)
	if _, ok := expected[base]; !ok {
		return "", 0, fmt.Errorf("%w: unexpected entry %q", ErrUnsafeArchive, entry.Name)
	}
	if seen[base] {
		return "", 0, fmt.Errorf("%w: duplicate entry %q", ErrUnsafeArchive, entry.Name)
	}
	if entry.UncompressedSize64 > uint64(expandedLimit) {
		return "", 0, fmt.Errorf("%w: entry %q exceeds the expanded limit", ErrSizeLimit, entry.Name)
	}
	seen[base] = true
	return base, int64(entry.UncompressedSize64), nil
}

func extractEntry(ctx context.Context, entry *zip.File, dest string, expandedLimit int64) error {
	source, err := entry.Open()
	if err != nil {
		return fmt.Errorf("%w: open entry %q: %v", ErrUnsafeArchive, entry.Name, err)
	}
	defer func() { _ = source.Close() }()
	file, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("create extracted file: %w", err)
	}
	if _, err := copyWithContext(ctx, file, io.LimitReader(source, expandedLimit+1)); err != nil {
		_ = file.Close()
		return fmt.Errorf("extract entry %q: %w", entry.Name, err)
	}
	if written, err := file.Seek(0, io.SeekCurrent); err == nil && written > expandedLimit {
		_ = file.Close()
		return fmt.Errorf("%w: entry %q exceeds the expanded limit", ErrSizeLimit, entry.Name)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close extracted entry %q: %w", entry.Name, err)
	}
	if err := os.Chmod(dest, 0o755); err != nil {
		return fmt.Errorf("mark extracted entry executable: %w", err)
	}
	return nil
}

func copyWithContext(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	buf := make([]byte, 32*1024)
	var written int64
	for {
		if err := ctx.Err(); err != nil {
			return written, err
		}
		n, readErr := src.Read(buf)
		if n > 0 {
			if _, err := dst.Write(buf[:n]); err != nil {
				return written, err
			}
			written += int64(n)
		}
		if readErr == io.EOF {
			return written, nil
		}
		if readErr != nil {
			return written, readErr
		}
	}
}

func writeManifest(bundleDir string, artifact Artifact) error {
	manifest := Manifest{
		Version:     artifact.Version,
		OS:          artifact.OS,
		Arch:        artifact.Arch,
		Digest:      artifact.SHA256,
		Server:      artifact.ServerName,
		Harness:     artifact.HarnessNames[0],
		ArtifactURL: artifact.URL,
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode antigravity manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(bundleDir, ManifestName), raw, 0o644); err != nil {
		return fmt.Errorf("write antigravity manifest: %w", err)
	}
	return nil
}

func ensureSafeDir(root, target string) error {
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("path %s escapes %s", target, root)
	}
	current := root
	for _, part := range strings.Split(rel, string(os.PathSeparator)) {
		if part == "." || part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			if err := os.Mkdir(current, 0o755); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("path component %s is not a real directory", current)
		}
	}
	return nil
}

func progressf(progress func(string), format string, args ...any) {
	if progress != nil {
		progress(fmt.Sprintf(format, args...))
	}
}
