package webapp

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/plugins/manifest"
	"gopkg.in/yaml.v3"
)

const MaxSourceArchiveBytes int64 = 30 << 20

const (
	distributionChecksumsFile = "checksums.txt"
	sourceEnvExample          = ".env.example"
)

var (
	ErrChecksumMissing     = errors.New("webapp: distribution checksums are missing")
	ErrChecksumMismatch    = errors.New("webapp: distribution checksum mismatch")
	ErrUnsafeSource        = errors.New("webapp: unsafe distribution source")
	ErrSourceUnavailable   = errors.New("webapp: distribution source is unavailable")
	ErrInvalidDistribution = errors.New("webapp: invalid distribution package")
)

// DistributionArchives contains the two deterministic artifacts produced
// from one validated immutable package snapshot.
type DistributionArchives struct {
	Bundle []byte
	Source []byte
}

// BuildDistributionPackage creates a validated package snapshot from a
// manifest and already-retained files. It rewrites only manifest.yaml and the
// generated checksum file; application/source bytes are copied unchanged.
func BuildDistributionPackage(m *manifest.Manifest, input map[string][]byte) (*Package, error) {
	if m == nil {
		return nil, ErrInvalidDistribution
	}
	if err := m.Validate(); err != nil {
		return nil, fmt.Errorf("%w: manifest: %v", ErrInvalidDistribution, err)
	}
	manifestData, err := yaml.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal manifest: %v", ErrInvalidDistribution, err)
	}
	if int64(len(manifestData)) > MaxManifestBytes {
		return nil, ErrManifestTooLarge
	}
	files := buildDistributionFiles(input, manifestData)
	expanded, err := validateDistributionFiles(files)
	if err != nil {
		return nil, err
	}
	files[distributionChecksumsFile] = checksumBytes(files)
	checksumBytesCount := int64(len(files[distributionChecksumsFile]))
	if expanded > MaxExpandedBytes-checksumBytesCount {
		return nil, ErrExpandedTooLarge
	}
	pkg := &Package{Manifest: m, Files: files, Digest: digestFiles(files), ExpandedBytes: expanded + checksumBytesCount}
	if err := ValidateDistribution(pkg); err != nil {
		return nil, err
	}
	return pkg, nil
}

func buildDistributionFiles(input map[string][]byte, manifestData []byte) map[string][]byte {
	files := make(map[string][]byte, len(input)+2)
	for name, data := range input {
		files[name] = append([]byte(nil), data...)
	}
	files["manifest.yaml"] = manifestData
	delete(files, distributionChecksumsFile)
	return files
}

func validateDistributionFiles(files map[string][]byte) (int64, error) {
	var expanded int64
	for name, data := range files {
		clean, err := normalizePackagePath(name, MaxPathBytes)
		if err != nil || clean != name {
			return 0, fmt.Errorf("%w: %s", ErrUnsafePath, name)
		}
		if strings.HasPrefix(name, "distribution/source/") {
			if err := ValidateDistributionSourcePath(name); err != nil {
				return 0, err
			}
		} else if !supportedFile(name) {
			return 0, fmt.Errorf("%w: %s", ErrUnsupportedFile, name)
		}
		if int64(len(data)) > MaxFileBytes || expanded > MaxExpandedBytes-int64(len(data)) {
			return 0, ErrFileTooLarge
		}
		expanded += int64(len(data))
	}
	return expanded, nil
}

// ValidateDistributionPackage validates a complete installable canvas bundle.
// It is the same entry point used by the offline registry inspector.
func ValidateDistributionPackage(r io.Reader) (*Package, error) {
	pkg, err := ValidatePackage(r)
	if err != nil {
		return nil, err
	}
	if err := ValidateDistribution(pkg); err != nil {
		return nil, err
	}
	return pkg, nil
}

// ValidateDistribution validates distribution-only requirements after the
// normal web-app package validator has checked archive safety and manifest
// syntax.
func ValidateDistribution(pkg *Package) error {
	if pkg == nil || pkg.Manifest == nil || !pkg.Manifest.IsCanvasDistribution() {
		return ErrInvalidDistribution
	}
	if _, ok := pkg.Files["README.md"]; !ok {
		return fmt.Errorf("%w: README.md", ErrInvalidDistribution)
	}
	if _, ok := pkg.Files[distributionChecksumsFile]; !ok {
		return ErrChecksumMissing
	}
	if requiresLicenseFile(pkg.Manifest.Distribution.License) {
		if _, ok := pkg.Files["LICENSE.txt"]; !ok {
			return fmt.Errorf("%w: LICENSE.txt", ErrInvalidDistribution)
		}
	}
	if err := validateChecksums(pkg.Files); err != nil {
		return err
	}
	if err := validateSourceMode(pkg.Manifest.Distribution.SourceMode, pkg.Files); err != nil {
		return err
	}
	return nil
}

// ValidateCanvasSourcePath applies the authoring-transfer policy to every
// workspace-relative source entry. Directories are allowed after component
// exclusions; regular files must use the retained-source extension allowlist.
func ValidateCanvasSourcePath(name string, isDir bool) error {
	clean, err := normalizePackagePath(name, MaxPathBytes)
	if err != nil {
		return err
	}
	if sourcePathForbidden(clean) {
		return fmt.Errorf("%w: %s", ErrUnsafeSource, clean)
	}
	if !isDir && !sourceFileExtensionAllowed(clean) {
		return fmt.Errorf("%w: %s", ErrUnsupportedFile, clean)
	}
	return nil
}

// ValidateDistributionSourcePath applies the source subtree policy during
// package validation. It rejects an unsafe path instead of silently dropping
// it, so a retained project cannot appear complete when it is not.
func ValidateDistributionSourcePath(name string) error {
	clean, err := normalizePackagePath(name, MaxPathBytes)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(clean, "distribution/source/") {
		return nil
	}
	return ValidateCanvasSourcePath(clean, false)
}

func validateSourceMode(sourceMode string, files map[string][]byte) error {
	var sourceFiles int
	for name := range files {
		if strings.HasPrefix(name, "distribution/source/") {
			sourceFiles++
		}
	}
	switch sourceMode {
	case manifest.SourceModeStatic:
		if sourceFiles != 0 {
			return fmt.Errorf("%w: static distributions cannot contain distribution/source", ErrInvalidDistribution)
		}
	case manifest.SourceModeProject:
		if sourceFiles == 0 {
			return ErrSourceUnavailable
		}
		for _, required := range []string{"distribution/source/manifest.yaml", "distribution/source/README.md"} {
			if _, ok := files[required]; !ok {
				return fmt.Errorf("%w: %s", ErrSourceUnavailable, required)
			}
		}
	default:
		return fmt.Errorf("%w: unsupported source mode", ErrInvalidDistribution)
	}
	return nil
}

func validateChecksums(files map[string][]byte) error {
	checksums, ok := files[distributionChecksumsFile]
	if !ok {
		return ErrChecksumMissing
	}
	want, err := parseChecksums(checksums, len(files)-1)
	if err != nil {
		return err
	}
	return validateChecksumCoverage(files, want)
}

func parseChecksums(checksums []byte, fileCount int) (map[string]string, error) {
	want := make(map[string]string, fileCount)
	scanner := bufio.NewScanner(bytes.NewReader(checksums))
	for scanner.Scan() {
		rawLine := scanner.Text()
		if strings.TrimSpace(rawLine) == "" {
			continue
		}
		digest, rawName, found := strings.Cut(rawLine, "  ")
		digest = strings.TrimSpace(digest)
		rawName = strings.TrimSpace(rawName)
		if !found || rawName == "" || len(digest) != sha256.Size*2 {
			return nil, fmt.Errorf("%w: malformed %s", ErrChecksumMismatch, distributionChecksumsFile)
		}
		name, err := normalizePackagePath(rawName, MaxPathBytes)
		if err != nil || name == distributionChecksumsFile || !isLowerHex(digest) {
			return nil, fmt.Errorf("%w: malformed %s", ErrChecksumMismatch, distributionChecksumsFile)
		}
		if _, exists := want[name]; exists {
			return nil, fmt.Errorf("%w: duplicate %s", ErrChecksumMismatch, name)
		}
		want[name] = strings.ToLower(digest)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("%w: read %s: %v", ErrChecksumMismatch, distributionChecksumsFile, err)
	}
	return want, nil
}

func validateChecksumCoverage(files map[string][]byte, want map[string]string) error {
	if len(want) != len(files)-1 {
		return fmt.Errorf("%w: incomplete file coverage", ErrChecksumMismatch)
	}
	for name, data := range files {
		if name == distributionChecksumsFile {
			continue
		}
		expected, exists := want[name]
		if !exists {
			return fmt.Errorf("%w: missing %s", ErrChecksumMismatch, name)
		}
		actual := sha256.Sum256(data)
		if expected != hex.EncodeToString(actual[:]) {
			return fmt.Errorf("%w: %s", ErrChecksumMismatch, name)
		}
	}
	for name := range want {
		if _, exists := files[name]; !exists {
			return fmt.Errorf("%w: unknown %s", ErrChecksumMismatch, name)
		}
	}
	return nil
}

func checksumBytes(files map[string][]byte) []byte {
	names := sortedFileNames(files)
	var output bytes.Buffer
	for _, name := range names {
		sum := sha256.Sum256(files[name])
		_, _ = fmt.Fprintf(&output, "%x  %s\n", sum, name)
	}
	return output.Bytes()
}

func isLowerHex(value string) bool {
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

func requiresLicenseFile(license string) bool {
	switch strings.ToUpper(strings.TrimSpace(license)) {
	case "MIT", "ISC", "UNLICENSE", "APACHE-2.0", "BSD-2-CLAUSE", "BSD-3-CLAUSE", "MPL-2.0", "GPL-2.0-ONLY", "GPL-3.0-ONLY", "LGPL-2.1-ONLY", "LGPL-3.0-ONLY":
		return false
	default:
		return true
	}
}

func sourcePathForbidden(name string) bool {
	for _, component := range strings.Split(name, "/") {
		if component == ".git" || component == "node_modules" || component == ".kandev" {
			return true
		}
		if strings.HasPrefix(component, ".env") && component != sourceEnvExample {
			return true
		}
	}
	ext := strings.ToLower(path.Ext(name))
	switch ext {
	case ".zip", ".tar", ".gz", ".tgz", ".bz2", ".xz", ".7z", ".rar", ".exe", ".dll", ".so", ".dylib", ".bin", ".wasm":
		return true
	default:
		return false
	}
}

func sourceFileExtensionAllowed(name string) bool {
	base := path.Base(name)
	if base == "Dockerfile" || base == "Makefile" || base == "README" || base == "LICENSE" || base == sourceEnvExample {
		return true
	}
	switch strings.ToLower(path.Ext(name)) {
	case ".css", ".html", ".htm", ".js", ".jsx", ".mjs", ".cjs", ".json", ".map", ".md", ".txt", ".ts", ".tsx", ".yaml", ".yml", ".lock", ".toml",
		".png", ".jpg", ".jpeg", ".gif", ".webp", ".avif", ".svg", ".ico", ".woff", ".woff2", ".ttf", ".otf", ".eot", sourceEnvExample:
		return true
	default:
		return false
	}
}

// BuildDistributionArchives creates a deterministic installable bundle and a
// source zip from the same validated package. The source zip is intentionally
// not accepted by ValidateDistributionPackage.
func BuildDistributionArchives(pkg *Package) (DistributionArchives, error) {
	if err := ValidateDistribution(pkg); err != nil {
		return DistributionArchives{}, err
	}
	bundle, err := writeBundle(pkg.Files)
	if err != nil {
		return DistributionArchives{}, err
	}
	source, err := writeSourceZip(pkg.Files)
	if err != nil {
		return DistributionArchives{}, err
	}
	return DistributionArchives{Bundle: bundle, Source: source}, nil
}

func writeBundle(files map[string][]byte) ([]byte, error) {
	names := sortedFileNames(files)
	var output bytes.Buffer
	zw := gzip.NewWriter(&output)
	zw.ModTime = time.Unix(0, 0)
	zw.OS = 255
	tw := tar.NewWriter(zw)
	for _, name := range names {
		data := files[name]
		header := &tar.Header{Name: name, Mode: 0o600, Size: int64(len(data)), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(header); err != nil {
			return nil, fmt.Errorf("webapp: write bundle header: %w", err)
		}
		if _, err := tw.Write(data); err != nil {
			return nil, fmt.Errorf("webapp: write bundle file: %w", err)
		}
	}
	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("webapp: close bundle: %w", err)
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("webapp: close bundle gzip: %w", err)
	}
	return output.Bytes(), nil
}

func writeSourceZip(files map[string][]byte) ([]byte, error) {
	var output bytes.Buffer
	zw := zip.NewWriter(&output)
	for _, name := range sortedFileNames(files) {
		if name == distributionChecksumsFile {
			continue
		}
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.Modified = time.Unix(0, 0)
		header.SetMode(0o600)
		writer, err := zw.CreateHeader(header)
		if err != nil {
			return nil, fmt.Errorf("webapp: create source file: %w", err)
		}
		if _, err := writer.Write(files[name]); err != nil {
			return nil, fmt.Errorf("webapp: write source file: %w", err)
		}
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("webapp: close source zip: %w", err)
	}
	return output.Bytes(), nil
}

// ValidateSourceArchive validates the non-installable source zip emitted by
// BuildDistributionArchives without executing or extracting it to disk.
func ValidateSourceArchive(r io.Reader) (*Package, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxSourceArchiveBytes+1))
	if err != nil {
		return nil, fmt.Errorf("webapp: read source archive: %w", err)
	}
	if int64(len(data)) > MaxSourceArchiveBytes {
		return nil, ErrCompressedTooLarge
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("webapp: open source archive: %w", err)
	}
	files := make(map[string][]byte, len(reader.File))
	var expanded int64
	for _, entry := range reader.File {
		name, content, err := readSourceArchiveEntry(entry, files)
		if err != nil {
			return nil, err
		}
		if expanded > MaxExpandedBytes-int64(len(content)) {
			return nil, ErrFileTooLarge
		}
		files[name] = content
		expanded += int64(len(content))
	}
	return validateSourceArchiveFiles(files, expanded, int64(len(data)))
}

func readSourceArchiveEntry(entry *zip.File, files map[string][]byte) (string, []byte, error) {
	if entry.FileInfo().IsDir() || entry.Mode()&0o170000 != 0 {
		return "", nil, ErrUnsafeEntry
	}
	name, err := normalizePackagePath(entry.Name, MaxPathBytes)
	if err != nil {
		return "", nil, err
	}
	if _, exists := files[name]; exists {
		return "", nil, ErrDuplicatePath
	}
	if name != "manifest.yaml" {
		if strings.HasPrefix(name, "distribution/source/") {
			if err := ValidateDistributionSourcePath(name); err != nil {
				return "", nil, err
			}
		} else if !supportedFile(name) {
			return "", nil, fmt.Errorf("%w: %s", ErrUnsupportedFile, name)
		}
	}
	if entry.UncompressedSize64 > uint64(MaxFileBytes) {
		return "", nil, ErrFileTooLarge
	}
	file, err := entry.Open()
	if err != nil {
		return "", nil, err
	}
	content, readErr := io.ReadAll(io.LimitReader(file, MaxFileBytes+1))
	_ = file.Close()
	if readErr != nil || uint64(len(content)) != entry.UncompressedSize64 {
		return "", nil, ErrFileTooLarge
	}
	return name, content, nil
}

func validateSourceArchiveFiles(files map[string][]byte, expanded, compressed int64) (*Package, error) {
	manifestData, ok := files["manifest.yaml"]
	if !ok {
		return nil, ErrManifestInvalid
	}
	m, err := manifest.Parse(manifestData)
	if err != nil {
		return nil, err
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	if !m.IsCanvasDistribution() {
		return nil, ErrInvalidDistribution
	}
	if err := validateSourceMode(m.Distribution.SourceMode, files); err != nil {
		return nil, err
	}
	return &Package{Manifest: m, Files: files, Digest: digestFiles(files), ExpandedBytes: expanded, CompressedBytes: compressed}, nil
}

func sortedFileNames(files map[string][]byte) []string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
