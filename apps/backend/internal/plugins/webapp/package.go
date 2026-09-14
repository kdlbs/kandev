// Package webapp validates and stores immutable static plugin web applications.
// It is deliberately separate from pkgtar: a static web application does not
// need a managed backend executable, and its files are served from a release
// directory rather than installed as a native plugin.
package webapp

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kandev/kandev/internal/plugins/manifest"
)

const (
	MaxCompressedBytes = int64(10 << 20)
	MaxExpandedBytes   = int64(25 << 20)
	MaxFiles           = 512
	MaxFileBytes       = int64(5 << 20)
	MaxManifestBytes   = int64(64 << 10)
	MaxPathBytes       = 240
)

var (
	ErrCompressedTooLarge  = errors.New("webapp: compressed package exceeds limit")
	ErrExpandedTooLarge    = errors.New("webapp: expanded package exceeds limit")
	ErrTooManyFiles        = errors.New("webapp: package contains too many files")
	ErrFileTooLarge        = errors.New("webapp: file exceeds limit")
	ErrManifestTooLarge    = errors.New("webapp: manifest exceeds limit")
	ErrUnsafePath          = errors.New("webapp: unsafe package path")
	ErrUnsafeEntry         = errors.New("webapp: unsafe archive entry")
	ErrDuplicatePath       = errors.New("webapp: duplicate normalized package path")
	ErrUnsupportedFile     = errors.New("webapp: unsupported file type")
	ErrEntryMissing        = errors.New("webapp: declared entry document is missing")
	ErrNotStaticWebApp     = errors.New("webapp: manifest does not declare a static web application")
	ErrArtifactUnavailable = errors.New("webapp: artifact unavailable")
)

// Limits is injectable so package tests and operators can exercise the same
// validation code with smaller bounds. Zero values use DefaultLimits.
type Limits struct {
	MaxCompressedBytes int64
	MaxExpandedBytes   int64
	MaxFiles           int
	MaxFileBytes       int64
	MaxManifestBytes   int64
	MaxPathBytes       int
}

func DefaultLimits() Limits {
	return Limits{
		MaxCompressedBytes: MaxCompressedBytes,
		MaxExpandedBytes:   MaxExpandedBytes,
		MaxFiles:           MaxFiles,
		MaxFileBytes:       MaxFileBytes,
		MaxManifestBytes:   MaxManifestBytes,
		MaxPathBytes:       MaxPathBytes,
	}
}

// Package is a fully validated release candidate. Files are keyed by
// normalized package-relative slash paths and are safe to copy to immutable
// artifact storage.
type Package struct {
	Manifest        *manifest.Manifest
	Files           map[string][]byte
	Digest          string
	CompressedBytes int64
	ExpandedBytes   int64
}

// ValidatePackage validates a gzip-compressed tar package using the
// production limits.
func ValidatePackage(r io.Reader) (*Package, error) {
	return ValidatePackageWithLimits(r, DefaultLimits())
}

// Validate is a short alias used by callers that already own a package
// validator abstraction.
func Validate(r io.Reader) (*Package, error) {
	return ValidatePackage(r)
}

// ValidatePackageWithLimits validates all archive entries before returning a
// release candidate. It does not write to disk or execute package content.
func ValidatePackageWithLimits(r io.Reader, limits Limits) (*Package, error) {
	limits = withDefaultLimits(limits)
	compressed, err := readCompressed(r, limits.MaxCompressedBytes)
	if err != nil {
		return nil, err
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, fmt.Errorf("webapp: open gzip package: %w", err)
	}
	defer func() { _ = reader.Close() }()
	files, expanded, err := readArchiveFiles(reader, limits)
	if err != nil {
		return nil, err
	}
	m, err := parseStaticManifest(files, limits)
	if err != nil {
		return nil, err
	}

	return &Package{
		Manifest:        m,
		Files:           files,
		Digest:          digestFiles(files),
		CompressedBytes: int64(len(compressed)),
		ExpandedBytes:   expanded,
	}, nil
}

// ValidateTarPackageWithLimits validates an uncompressed tar stream such as
// the bounded source stream returned by an agentctl workspace. Directory
// entries are accepted and discarded because tar walkers commonly emit them;
// the extracted release still contains only regular files.
func ValidateTarPackageWithLimits(r io.Reader, limits Limits, wireBytes int64) (*Package, error) {
	limits = withDefaultLimits(limits)
	if r == nil {
		return nil, errors.New("webapp: package reader is nil")
	}
	if wireBytes < 0 || wireBytes > limits.MaxCompressedBytes {
		return nil, ErrCompressedTooLarge
	}
	files, expanded, err := readArchiveFilesWithOptions(r, limits, true)
	if err != nil {
		return nil, err
	}
	m, err := parseStaticManifest(files, limits)
	if err != nil {
		return nil, err
	}
	return &Package{
		Manifest:        m,
		Files:           files,
		Digest:          digestFiles(files),
		CompressedBytes: wireBytes,
		ExpandedBytes:   expanded,
	}, nil
}

// ValidateTarPackage validates an uncompressed tar source stream using the
// normal static web-app limits.
func ValidateTarPackage(r io.Reader, wireBytes int64) (*Package, error) {
	return ValidateTarPackageWithLimits(r, DefaultLimits(), wireBytes)
}

func readArchiveFiles(reader io.Reader, limits Limits) (map[string][]byte, int64, error) {
	return readArchiveFilesWithOptions(reader, limits, false)
}

func readArchiveFilesWithOptions(reader io.Reader, limits Limits, allowDirectories bool) (map[string][]byte, int64, error) {
	files := make(map[string][]byte)
	var expanded int64
	tarReader := tar.NewReader(reader)
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			return files, expanded, nil
		}
		if err != nil {
			return nil, 0, fmt.Errorf("webapp: read archive: %w", err)
		}
		name, data, nextExpanded, err := readArchiveEntry(tarReader, header, files, expanded, limits, allowDirectories)
		if err != nil {
			return nil, 0, err
		}
		if name == "" {
			continue
		}
		files[name] = data
		expanded = nextExpanded
	}
}

func readArchiveEntry(reader *tar.Reader, header *tar.Header, files map[string][]byte, expanded int64, limits Limits, allowDirectories bool) (string, []byte, int64, error) {
	if len(files) >= limits.MaxFiles && (!allowDirectories || header.Typeflag != tar.TypeDir) {
		return "", nil, 0, ErrTooManyFiles
	}
	if allowDirectories && header.Typeflag == tar.TypeDir {
		return readArchiveDirectory(header, files, expanded, limits)
	}
	if header.Typeflag != tar.TypeReg && header.Typeflag != 0 {
		return "", nil, 0, fmt.Errorf("%w: %s", ErrUnsafeEntry, header.Name)
	}
	return readArchiveRegularFile(reader, header, files, expanded, limits)
}

func readArchiveDirectory(header *tar.Header, files map[string][]byte, expanded int64, limits Limits) (string, []byte, int64, error) {
	name, err := normalizePackagePath(strings.TrimSuffix(header.Name, "/"), limits.MaxPathBytes)
	if err != nil {
		return "", nil, 0, err
	}
	if _, exists := files[name]; exists {
		return "", nil, 0, fmt.Errorf("%w: %s", ErrDuplicatePath, name)
	}
	return "", nil, expanded, nil
}

func readArchiveRegularFile(reader *tar.Reader, header *tar.Header, files map[string][]byte, expanded int64, limits Limits) (string, []byte, int64, error) {
	name, err := normalizePackagePath(header.Name, limits.MaxPathBytes)
	if err != nil {
		return "", nil, 0, err
	}
	if _, exists := files[name]; exists {
		return "", nil, 0, fmt.Errorf("%w: %s", ErrDuplicatePath, name)
	}
	if !supportedFile(name) {
		return "", nil, 0, fmt.Errorf("%w: %s", ErrUnsupportedFile, name)
	}
	if header.Size < 0 || header.Size > limits.MaxFileBytes {
		return "", nil, 0, fmt.Errorf("%w: %s", ErrFileTooLarge, name)
	}
	if expanded > limits.MaxExpandedBytes-header.Size {
		return "", nil, 0, ErrExpandedTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(reader, header.Size+1))
	if err != nil {
		return "", nil, 0, fmt.Errorf("webapp: read %s: %w", name, err)
	}
	if int64(len(data)) != header.Size {
		return "", nil, 0, fmt.Errorf("webapp: short file %s", name)
	}
	return name, data, expanded + int64(len(data)), nil
}

func parseStaticManifest(files map[string][]byte, limits Limits) (*manifest.Manifest, error) {
	manifestData, ok := files["manifest.yaml"]
	if !ok {
		return nil, fmt.Errorf("%w: manifest.yaml", ErrManifestInvalid)
	}
	if int64(len(manifestData)) > limits.MaxManifestBytes {
		return nil, ErrManifestTooLarge
	}
	m, err := manifest.Parse(manifestData)
	if err != nil {
		return nil, fmt.Errorf("webapp: parse manifest: %w", err)
	}
	if err := m.Validate(); err != nil {
		return nil, fmt.Errorf("webapp: invalid manifest: %w", err)
	}
	if !m.IsStaticWebAppOnly() {
		return nil, ErrNotStaticWebApp
	}
	for _, app := range m.UI.WebApps {
		if _, ok := files[app.Entry]; !ok {
			return nil, fmt.Errorf("%w: %s", ErrEntryMissing, app.Entry)
		}
	}
	return m, nil
}

// ErrManifestInvalid is kept separate so callers can map malformed package
// input to one stable validation result without exposing file contents.
var ErrManifestInvalid = errors.New("webapp: invalid manifest")

func withDefaultLimits(limits Limits) Limits {
	defaults := DefaultLimits()
	if limits.MaxCompressedBytes <= 0 {
		limits.MaxCompressedBytes = defaults.MaxCompressedBytes
	}
	if limits.MaxExpandedBytes <= 0 {
		limits.MaxExpandedBytes = defaults.MaxExpandedBytes
	}
	if limits.MaxFiles <= 0 {
		limits.MaxFiles = defaults.MaxFiles
	}
	if limits.MaxFileBytes <= 0 {
		limits.MaxFileBytes = defaults.MaxFileBytes
	}
	if limits.MaxManifestBytes <= 0 {
		limits.MaxManifestBytes = defaults.MaxManifestBytes
	}
	if limits.MaxPathBytes <= 0 {
		limits.MaxPathBytes = defaults.MaxPathBytes
	}
	return limits
}

func readCompressed(r io.Reader, max int64) ([]byte, error) {
	if r == nil {
		return nil, errors.New("webapp: package reader is nil")
	}
	data, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, fmt.Errorf("webapp: read compressed package: %w", err)
	}
	if int64(len(data)) > max {
		return nil, ErrCompressedTooLarge
	}
	return data, nil
}

func normalizePackagePath(name string, maxBytes int) (string, error) {
	if name == "" || len(name) > maxBytes || strings.ContainsRune(name, '\x00') || strings.Contains(name, "\\") {
		return "", fmt.Errorf("%w: %q", ErrUnsafePath, name)
	}
	if path.IsAbs(name) {
		return "", fmt.Errorf("%w: %q", ErrUnsafePath, name)
	}
	clean := path.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("%w: %q", ErrUnsafePath, name)
	}
	return clean, nil
}

func supportedFile(name string) bool {
	if name == "manifest.yaml" {
		return true
	}
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".html", ".htm", ".css", ".js", ".mjs", ".cjs", ".json", ".map",
		".png", ".jpg", ".jpeg", ".gif", ".webp", ".avif", ".svg", ".ico",
		".woff", ".woff2", ".ttf", ".otf", ".eot", ".md", ".txt":
		return true
	default:
		return false
	}
}

func digestFiles(files map[string][]byte) string {
	paths := make([]string, 0, len(files))
	for name := range files {
		paths = append(paths, name)
	}
	sort.Strings(paths)
	h := sha256.New()
	for _, name := range paths {
		_, _ = io.WriteString(h, name)
		_, _ = h.Write([]byte{0})
		_, _ = h.Write(files[name])
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Artifact describes one immutable extracted release directory.
type Artifact struct {
	Digest       string
	RelativePath string
	Bytes        int64
	Available    bool
	Reason       string
}

// ArtifactStore owns release files below one Kandev-home recovery boundary.
type ArtifactStore struct{ root string }

func NewArtifactStore(root string) (*ArtifactStore, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("webapp: artifact root is empty")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("webapp: create artifact root: %w", err)
	}
	return &ArtifactStore{root: root}, nil
}

// Path returns the absolute path for an artifact after validating its stored
// relative path. An invalid artifact returns an empty path.
func (s *ArtifactStore) Path(artifact Artifact) string {
	if s == nil || artifact.RelativePath == "" {
		return ""
	}
	clean, err := normalizePackagePath(artifact.RelativePath, MaxPathBytes*4)
	if err != nil {
		return ""
	}
	return filepath.Join(s.root, filepath.FromSlash(clean))
}

// Open opens one regular file below an immutable artifact. The caller must
// still close the returned file. No path supplied by a web application is
// accepted here; runtime handlers pass the manifest entry or a package path
// that has already been normalized.
func (s *ArtifactStore) Open(artifact Artifact, name string) (*os.File, error) {
	root := s.Path(artifact)
	clean, err := normalizePackagePath(name, MaxPathBytes)
	if root == "" || err != nil {
		return nil, fmt.Errorf("%w: %q", ErrUnsafePath, name)
	}
	filePath := filepath.Join(root, filepath.FromSlash(clean))
	relative, err := filepath.Rel(root, filePath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("%w: %q", ErrUnsafePath, name)
	}
	info, err := os.Lstat(filePath)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %q", ErrUnsafeEntry, name)
	}
	return os.Open(filePath)
}

// Remove deletes one validated release directory. Lifecycle services call
// this only after the corresponding database cleanup job commits.
func (s *ArtifactStore) Remove(artifact Artifact) error {
	return s.RemoveRelativePath(artifact.RelativePath)
}

// RemoveRelativePath removes one release directory identified by durable
// cleanup metadata. Only descendants of releases/ are accepted, so a corrupt
// cleanup row cannot remove the artifact-store root or another store.
func (s *ArtifactStore) RemoveRelativePath(relativePath string) error {
	clean, err := normalizePackagePath(relativePath, MaxPathBytes*4)
	if s == nil || err != nil || clean == "releases" || !strings.HasPrefix(clean, "releases/") {
		return fmt.Errorf("%w: artifact path", ErrUnsafePath)
	}
	return os.RemoveAll(filepath.Join(s.root, filepath.FromSlash(clean)))
}

// Put extracts a validated package into a temporary directory and atomically
// renames it into the digest-addressed release directory.
func (s *ArtifactStore) Put(pkg *Package) (Artifact, error) {
	artifact, _, err := s.PutWithCreated(pkg)
	return artifact, err
}

// PutWithCreated is the lifecycle-aware form of Put. created is true only
// when this call installed the digest directory; callers can then remove that
// directory if the following database commit fails without deleting an
// artifact another release already owns.
func (s *ArtifactStore) PutWithCreated(pkg *Package) (Artifact, bool, error) {
	artifact, target, err := s.artifactTarget(pkg)
	if err != nil {
		return Artifact{}, false, err
	}
	exists, err := artifactExists(target)
	if err != nil {
		return Artifact{}, false, err
	}
	if exists {
		return artifact, false, nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return Artifact{}, false, fmt.Errorf("webapp: create release parent: %w", err)
	}
	tmp, err := os.MkdirTemp(filepath.Dir(target), ".release-")
	if err != nil {
		return Artifact{}, false, fmt.Errorf("webapp: create release temp: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if err := writeArtifactFiles(tmp, pkg.Files); err != nil {
		return Artifact{}, false, err
	}
	created, err := commitArtifact(tmp, target)
	if err != nil {
		return Artifact{}, false, err
	}
	return artifact, created, nil
}

func (s *ArtifactStore) artifactTarget(pkg *Package) (Artifact, string, error) {
	if s == nil || pkg == nil || pkg.Digest == "" {
		return Artifact{}, "", errors.New("webapp: package or artifact store is nil")
	}
	if !isSafeDigest(pkg.Digest) {
		return Artifact{}, "", fmt.Errorf("%w: invalid digest", ErrUnsafePath)
	}
	artifact := Artifact{Digest: pkg.Digest, RelativePath: filepath.ToSlash(filepath.Join("releases", pkg.Digest)), Bytes: pkg.ExpandedBytes, Available: true}
	target := s.Path(artifact)
	if target == "" {
		return Artifact{}, "", fmt.Errorf("%w: artifact path", ErrUnsafePath)
	}
	return artifact, target, nil
}

func artifactExists(target string) (bool, error) {
	info, err := os.Stat(target)
	if err == nil {
		if !info.IsDir() {
			return false, fmt.Errorf("%w: artifact target is not a directory", ErrArtifactUnavailable)
		}
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("webapp: inspect artifact: %w", err)
}

func writeArtifactFiles(root string, files map[string][]byte) error {
	for name, data := range files {
		clean, err := normalizePackagePath(name, MaxPathBytes)
		if err != nil {
			return err
		}
		filePath := filepath.Join(root, filepath.FromSlash(clean))
		if err := os.MkdirAll(filepath.Dir(filePath), 0o700); err != nil {
			return fmt.Errorf("webapp: create release directory: %w", err)
		}
		if err := os.WriteFile(filePath, data, 0o600); err != nil {
			return fmt.Errorf("webapp: write release file: %w", err)
		}
	}
	return nil
}

func commitArtifact(tmp, target string) (bool, error) {
	if err := os.Rename(tmp, target); err != nil {
		if exists, statErr := artifactExists(target); statErr == nil && exists {
			return false, nil
		}
		return false, fmt.Errorf("webapp: commit release artifact: %w", err)
	}
	return true, nil
}

// Reconcile verifies the digest-addressed artifact without removing it. A
// mismatch, unsafe entry, or missing directory becomes unavailable so callers
// can preserve recovery metadata and avoid executing it.
func (s *ArtifactStore) Reconcile(artifact Artifact) (Artifact, error) {
	artifact.Available = false
	artifact.Reason = "missing"
	root := s.Path(artifact)
	if root == "" {
		artifact.Reason = "unsafe_path"
		return artifact, nil
	}
	info, err := os.Stat(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return artifact, nil
		}
		return artifact, fmt.Errorf("%w: stat artifact: %v", ErrArtifactUnavailable, err)
	}
	if !info.IsDir() {
		artifact.Reason = "not_directory"
		return artifact, nil
	}
	files, err := readArtifactFiles(root)
	if err != nil {
		artifact.Reason = "unsafe_artifact"
		return artifact, nil
	}
	if digestFiles(files) != artifact.Digest {
		artifact.Reason = "digest_mismatch"
		return artifact, nil
	}
	artifact.Bytes = 0
	artifact.Bytes = artifactBytes(files)
	artifact.Available = true
	artifact.Reason = ""
	return artifact, nil
}

// ReadFiles verifies and reads an immutable release artifact for a trusted
// backend caller. The returned paths are package-relative and the method does
// not expose the artifact root to the caller, so callers can transfer the
// bytes to a remote agent workspace without assuming that workspace is on the
// backend host.
func (s *ArtifactStore) ReadFiles(artifact Artifact) (map[string][]byte, error) {
	reconciled, err := s.Reconcile(artifact)
	if err != nil {
		return nil, err
	}
	if !reconciled.Available {
		if reconciled.Reason == "" {
			reconciled.Reason = "unavailable"
		}
		return nil, fmt.Errorf("%w: %s", ErrArtifactUnavailable, reconciled.Reason)
	}
	root := s.Path(reconciled)
	if root == "" {
		return nil, fmt.Errorf("%w: unsafe artifact path", ErrArtifactUnavailable)
	}
	files, err := readArtifactFiles(root)
	if err != nil {
		return nil, fmt.Errorf("%w: read artifact: %v", ErrArtifactUnavailable, err)
	}
	if digestFiles(files) != reconciled.Digest {
		return nil, fmt.Errorf("%w: digest mismatch", ErrArtifactUnavailable)
	}
	return files, nil
}

func readArtifactFiles(root string) (map[string][]byte, error) {
	files := make(map[string][]byte)
	err := filepath.WalkDir(root, func(filePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || filePath == root || entry.IsDir() {
			return walkErr
		}
		relative, err := filepath.Rel(root, filePath)
		if err != nil {
			return err
		}
		name, err := normalizePackagePath(filepath.ToSlash(relative), MaxPathBytes)
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return ErrUnsafeEntry
		}
		data, err := os.ReadFile(filePath)
		if err != nil {
			return err
		}
		files[name] = data
		return nil
	})
	return files, err
}

func artifactBytes(files map[string][]byte) int64 {
	var total int64
	for _, data := range files {
		total += int64(len(data))
	}
	return total
}

func isSafeDigest(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
