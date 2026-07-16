// Package pkgtar implements the kandev plugin package format described in
// docs/plans/plugins/GRPC-CONTRACT.md §6: a tar.gz archive containing
// manifest.yaml, a set of per-platform server/ executables, an optional
// ui/ bundle, and a checksums.txt covering every other file. Inspect reads
// only the manifest (no disk writes); Install verifies and atomically
// extracts a package into a per-plugin, per-version directory; Remove
// deletes an installed plugin's directory tree.
package pkgtar

import (
	"archive/tar"
	"bufio"
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
	"runtime"
	"strings"

	"github.com/kandev/kandev/internal/plugins/manifest"
)

const (
	manifestFileName     = "manifest.yaml"
	checksumsFileName    = "checksums.txt"
	checksumsSigFileName = "checksums.txt.sig"

	// maxManifestSize caps how much of manifest.yaml Inspect will read into
	// memory before giving up.
	maxManifestSize = 1 << 20 // 1 MiB
)

// maxPackageFileSize caps any single file's size during Install, and
// maxPackageTotalSize caps the sum of all files' ACTUAL (decompressed)
// bytes copied during Install — see readArchive. Declared as vars (not
// consts), mirroring VerifySignature below, purely so this package's own
// tests can override them to exercise the cap logic against small fixtures
// instead of allocating real 200MB payloads; production defaults are
// unchanged.
var (
	maxPackageFileSize  int64 = 200 << 20 // 200 MiB
	maxPackageTotalSize int64 = 200 << 20 // 200 MiB
)

// Sentinel errors returned (wrapped) by Install. Use errors.Is to check for
// a specific failure category.
var (
	// ErrManifestInvalid means manifest.yaml is missing, fails to parse,
	// fails manifest.Validate(), or is not a runtime-managed manifest
	// (manifest.IsManaged() == false).
	ErrManifestInvalid = errors.New("pkgtar: manifest invalid")
	// ErrMissingChecksums means the package has no checksums.txt entry.
	ErrMissingChecksums = errors.New("pkgtar: missing checksums.txt")
	// ErrUnlistedFile means the package contains a file that checksums.txt
	// does not list.
	ErrUnlistedFile = errors.New("pkgtar: file not listed in checksums.txt")
	// ErrChecksumMismatch means a file's sha256 does not match the value
	// recorded in checksums.txt.
	ErrChecksumMismatch = errors.New("pkgtar: checksum mismatch")
	// ErrPathTraversal means an archive entry's path escapes the package
	// root (absolute path, "..", or a symlink/hardlink entry).
	ErrPathTraversal = errors.New("pkgtar: unsafe archive path")
	// ErrPlatformNotSupported means the manifest does not declare an
	// executable for the current host's runtime.GOOS-runtime.GOARCH.
	ErrPlatformNotSupported = errors.New("pkgtar: host platform not supported by package")
	// ErrVersionExists means destRoot/<id>/<version> already exists.
	ErrVersionExists = errors.New("pkgtar: version already installed")
)

// VerifySignature is an injectable hook for verifying checksums.txt.sig
// against the checksums.txt bytes it signs. It is nil by default, meaning
// signature verification is not performed: a present checksums.txt.sig
// entry with no VerifySignature configured leaves InstallResult.Signed
// false (unverified, not merely "present") — Install still succeeds since
// signing is optional. Set this (e.g. to an ed25519 verifier) to enable
// real verification; a non-nil error from it fails Install outright, and a
// nil error sets InstallResult.Signed=true.
var VerifySignature func(sig, checksums []byte) error

// InstallResult describes the outcome of a successful Install.
type InstallResult struct {
	// Manifest is the parsed, validated manifest.yaml of the installed
	// package.
	Manifest *manifest.Manifest
	// InstallPath is the absolute path the package was extracted to:
	// destRoot/<id>/<version>.
	InstallPath string
	// Version is Manifest.Version, duplicated here for convenience.
	Version string
	// Signed reports whether the package included a checksums.txt.sig
	// entry AND a configured VerifySignature verifier successfully
	// verified it. A present-but-unverified signature (no verifier wired)
	// or a missing signature both leave this false. See VerifySignature.
	Signed bool
}

// Inspect streams r (a kandev plugin tar.gz package), extracts only
// manifest.yaml into memory (capped at maxManifestSize), and parses +
// validates it. It performs no checksum verification and writes nothing to
// disk; it exists to preview a package's declared capabilities before
// installing it.
func Inspect(r io.Reader) (*manifest.Manifest, error) {
	tr, closeReader, err := openTarGz(r)
	if err != nil {
		return nil, err
	}
	defer closeReader()

	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("pkgtar: %s not found in package", manifestFileName)
		}
		if err != nil {
			return nil, fmt.Errorf("pkgtar: reading tar entry: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		name, err := cleanArchivePath(hdr.Name)
		if err != nil || name != manifestFileName {
			continue
		}
		return parseManifestEntry(tr, hdr.Size)
	}
}

// parseManifestEntry reads a manifest.yaml tar entry (capped at
// maxManifestSize) and parses + validates it.
func parseManifestEntry(tr *tar.Reader, size int64) (*manifest.Manifest, error) {
	if size > maxManifestSize {
		return nil, fmt.Errorf("pkgtar: %s exceeds max size of %d bytes", manifestFileName, maxManifestSize)
	}
	data, err := readCapped(tr, maxManifestSize)
	if err != nil {
		return nil, fmt.Errorf("pkgtar: reading %s: %w", manifestFileName, err)
	}
	m, err := manifest.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("pkgtar: parsing %s: %w", manifestFileName, err)
	}
	if err := m.Validate(); err != nil {
		return nil, fmt.Errorf("pkgtar: invalid %s: %w", manifestFileName, err)
	}
	return m, nil
}

// Install verifies and atomically extracts a kandev plugin package (tar.gz,
// read fully from r) into destRoot/<id>/<version>/, per
// docs/plans/plugins/GRPC-CONTRACT.md §6:
//
//  1. Read the whole archive (size-capped, path-traversal-rejected).
//  2. Require checksums.txt covering every other file; verify every hash
//     and reject unlisted files.
//  3. Parse + validate manifest.yaml; require it to be runtime-managed.
//  4. Require an executable declared for the current host platform.
//  5. Extract to a temp dir under destRoot/<id>/, chmod declared
//     executables to 0755, then atomically rename into place. Fails with
//     ErrVersionExists if destRoot/<id>/<version> already exists.
func Install(r io.Reader, destRoot string) (*InstallResult, error) {
	files, err := readArchive(r)
	if err != nil {
		return nil, err
	}

	signed, err := verifyPackageIntegrity(files)
	if err != nil {
		return nil, err
	}

	m, execPath, err := validateInstallManifest(files)
	if err != nil {
		return nil, err
	}

	versionDir, err := extractPackage(destRoot, m, files, execPath)
	if err != nil {
		return nil, err
	}

	return &InstallResult{
		Manifest:    m,
		InstallPath: versionDir,
		Version:     m.Version,
		Signed:      signed,
	}, nil
}

// verifyPackageIntegrity requires checksums.txt, verifies every listed
// file's hash, rejects unlisted files, and (when present) runs
// checksums.txt.sig through VerifySignature. signed is true only when a
// VerifySignature verifier is configured AND it actually succeeded — a
// present-but-unverified signature (no verifier wired, the default) is
// reported as unsigned rather than claiming a guarantee nothing checked. A
// present signature that fails verification fails Install outright (a
// tampered/invalid signature is worse than no signature at all); signing
// otherwise remains optional and never blocks an unsigned install.
func verifyPackageIntegrity(files map[string][]byte) (signed bool, err error) {
	checksumsData, ok := files[checksumsFileName]
	if !ok {
		return false, ErrMissingChecksums
	}
	checks, err := parseChecksums(checksumsData)
	if err != nil {
		return false, err
	}
	if err := verifyChecksums(files, checks); err != nil {
		return false, err
	}

	sigData, hasSig := files[checksumsSigFileName]
	if !hasSig || VerifySignature == nil {
		return false, nil
	}
	if err := VerifySignature(sigData, checksumsData); err != nil {
		return false, fmt.Errorf("pkgtar: signature verification failed: %w", err)
	}
	return true, nil
}

// validateInstallManifest parses manifest.yaml out of files, validates it,
// requires it to be runtime-managed, and resolves the current host
// platform's executable path (which must itself be present in files).
func validateInstallManifest(files map[string][]byte) (*manifest.Manifest, string, error) {
	manifestData, ok := files[manifestFileName]
	if !ok {
		return nil, "", fmt.Errorf("%w: missing %s", ErrManifestInvalid, manifestFileName)
	}
	m, err := manifest.Parse(manifestData)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrManifestInvalid, err)
	}
	if err := m.Validate(); err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrManifestInvalid, err)
	}
	if !m.IsManaged() {
		return nil, "", fmt.Errorf("%w: manifest is not runtime-managed (runtime.type must be \"binary\")", ErrManifestInvalid)
	}

	execPath, ok := m.ExecutableFor(runtime.GOOS, runtime.GOARCH)
	if !ok {
		return nil, "", fmt.Errorf("%w: %s-%s", ErrPlatformNotSupported, runtime.GOOS, runtime.GOARCH)
	}
	if _, ok := files[execPath]; !ok {
		return nil, "", fmt.Errorf("%w: declared executable %q not found in package", ErrManifestInvalid, execPath)
	}
	return m, execPath, nil
}

// extractPackage writes files into a temp dir under destRoot/<id>/, chmods
// every declared runtime executable to 0755, and atomically renames the
// temp dir to destRoot/<id>/<version>. It fails with ErrVersionExists if
// that directory already exists, and cleans up the temp dir on any error.
func extractPackage(destRoot string, m *manifest.Manifest, files map[string][]byte, hostExecPath string) (string, error) {
	pluginDir, err := securejoin(destRoot, m.ID)
	if err != nil {
		return "", err
	}
	versionDir, err := securejoin(pluginDir, m.Version)
	if err != nil {
		return "", err
	}

	if _, err := os.Stat(versionDir); err == nil {
		return "", fmt.Errorf("%w: %s", ErrVersionExists, versionDir)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("pkgtar: checking install path: %w", err)
	}

	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		return "", fmt.Errorf("pkgtar: creating plugin dir: %w", err)
	}
	tmpDir, err := os.MkdirTemp(pluginDir, ".tmp-*")
	if err != nil {
		return "", fmt.Errorf("pkgtar: creating temp dir: %w", err)
	}
	succeeded := false
	defer func() {
		if !succeeded {
			_ = os.RemoveAll(tmpDir)
		}
	}()

	_ = hostExecPath // the whole executables set is chmodded, not just the host one
	execSet := executablePaths(m)
	if err := writePackageFiles(tmpDir, files, execSet); err != nil {
		return "", err
	}

	if err := os.Rename(tmpDir, versionDir); err != nil {
		return "", fmt.Errorf("pkgtar: finalizing install: %w", err)
	}
	succeeded = true
	return versionDir, nil
}

// writePackageFiles writes every entry in files under root, creating
// parent directories as needed, and chmods entries listed in execSet to
// 0755. Every entry name is archive-controlled, so each destination path
// is re-validated with securejoin at this write sink even though
// cleanArchivePath already rejected unsafe names when the archive was
// read (defense in depth: this is the actual filesystem write).
func writePackageFiles(root string, files map[string][]byte, execSet map[string]bool) error {
	for name, data := range files {
		dest, err := securejoin(root, filepath.FromSlash(name))
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return fmt.Errorf("pkgtar: creating dir for %s: %w", name, err)
		}
		mode := os.FileMode(0o644)
		if execSet[name] {
			mode = 0o755
		}
		if err := os.WriteFile(dest, data, mode); err != nil {
			return fmt.Errorf("pkgtar: writing %s: %w", name, err)
		}
	}
	return nil
}

// securejoin joins root and rel (a path derived from archive-controlled
// input: an archive entry name or a manifest-declared executable path)
// and verifies the result stays within root. It is the sink-level guard
// against path traversal / "zip slip": callers upstream (cleanArchivePath,
// manifest.Validate) already reject unsafe names, but this re-checks
// containment right before the filesystem operation that uses the path.
func securejoin(root, rel string) (string, error) {
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("%w: absolute path %q", ErrPathTraversal, rel)
	}
	dst := filepath.Join(root, rel)
	cleanRoot := filepath.Clean(root)
	relToRoot, err := filepath.Rel(cleanRoot, filepath.Clean(dst))
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrPathTraversal, rel)
	}
	if relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(os.PathSeparator)) || filepath.IsAbs(relToRoot) {
		return "", fmt.Errorf("%w: %s", ErrPathTraversal, rel)
	}
	return dst, nil
}

// executablePaths returns the set of package-relative paths declared by
// any of the manifest's runtime.executables entries (not just the host
// platform's), so a multi-platform package has every included executable
// chmodded regardless of which one the host will run.
func executablePaths(m *manifest.Manifest) map[string]bool {
	set := make(map[string]bool, len(m.Runtime.Executables))
	for _, p := range m.Runtime.Executables {
		set[p] = true
	}
	return set
}

// Remove deletes destRoot/<id>/ entirely: every installed version plus the
// plugin's data directory.
func Remove(destRoot, id string) error {
	if id == "" || id == "." || id == ".." || strings.ContainsAny(id, "/\\") {
		return fmt.Errorf("pkgtar: invalid plugin id %q", id)
	}
	// filepath.Base pins id to a single path segment (a barrier the taint
	// analysis recognizes); securejoin then re-verifies containment.
	dir, err := securejoin(destRoot, filepath.Base(id))
	if err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("pkgtar: removing %s: %w", dir, err)
	}
	return nil
}

// readArchive fully decompresses and un-tars r into a name->contents map,
// enforcing per-file and total size caps and rejecting any entry that is
// not a plain file at a clean, package-relative path (no symlinks,
// hardlinks, absolute paths, or "..").
//
// Both caps are enforced against bytes ACTUALLY copied out of the
// decompressed stream, not the archive-declared hdr.Size: hdr.Size comes
// from the (attacker-controlled) tar header, and a gzip stream can be
// crafted to decompress to far more data than a small compressed payload
// suggests. Every entry's read is independently bounded by
// io.LimitReader inside readCapped regardless of what hdr.Size claims, and
// the running `total` is accumulated from len(data) — the real byte count
// — after that bounded read, so a mismatched/lied Size cannot smuggle more
// decompressed data past either cap.
func readArchive(r io.Reader) (map[string][]byte, error) {
	tr, closeReader, err := openTarGz(r)
	if err != nil {
		return nil, err
	}
	defer closeReader()

	files := make(map[string][]byte)
	var total int64
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("pkgtar: reading tar entry: %w", err)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			continue
		case tar.TypeReg:
			// handled below
		default:
			return nil, fmt.Errorf("%w: entry %q has unsupported type (symlinks/hardlinks/devices are not allowed)", ErrPathTraversal, hdr.Name)
		}

		name, err := cleanArchivePath(hdr.Name)
		if err != nil {
			return nil, err
		}
		if hdr.Size > maxPackageFileSize {
			// Fast pre-check: reject an implausible declared size before
			// even attempting the (still independently bounded) read.
			return nil, fmt.Errorf("pkgtar: file %s exceeds max size of %d bytes", name, maxPackageFileSize)
		}
		data, err := readCapped(tr, maxPackageFileSize)
		if err != nil {
			return nil, fmt.Errorf("pkgtar: reading %s: %w", name, err)
		}
		total += int64(len(data))
		if total > maxPackageTotalSize {
			return nil, fmt.Errorf("pkgtar: package exceeds max total size of %d bytes", maxPackageTotalSize)
		}
		files[name] = data
	}
	return files, nil
}

// openTarGz wraps r in a gzip reader and a tar reader, returning a closer
// that releases the gzip reader.
func openTarGz(r io.Reader) (*tar.Reader, func(), error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, nil, fmt.Errorf("pkgtar: invalid gzip stream: %w", err)
	}
	return tar.NewReader(gz), func() { _ = gz.Close() }, nil
}

// readCapped reads at most maxSize+1 bytes from r and errors if that many
// were available, i.e. the entry exceeds maxSize bytes.
func readCapped(r io.Reader, maxSize int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxSize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxSize {
		return nil, fmt.Errorf("entry exceeds max size of %d bytes", maxSize)
	}
	return data, nil
}

// cleanArchivePath cleans and validates a tar entry name: it must not be
// empty, absolute, contain a backslash, or escape the package root via
// "..". Backslashes are rejected outright rather than treated as a path
// separator: path.Clean/path.IsAbs only understand "/", so an entry like
// "server/..\\..\\escape.exe" would pass through unchanged (no leading ".."
// segment under "/"-only splitting) and could then be reinterpreted as an
// actual traversal by filepath.Join/filepath.Clean on a host where "\\" is
// a separator (Windows) — securejoin (the write-sink guard) runs
// filepath.Join too, so the same risk applies there.
func cleanArchivePath(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("%w: empty entry name", ErrPathTraversal)
	}
	if strings.ContainsRune(name, '\\') {
		return "", fmt.Errorf("%w: backslash in entry name %q", ErrPathTraversal, name)
	}
	if path.IsAbs(name) {
		return "", fmt.Errorf("%w: absolute path %q", ErrPathTraversal, name)
	}
	cleaned := path.Clean(name)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || path.IsAbs(cleaned) {
		return "", fmt.Errorf("%w: %q", ErrPathTraversal, name)
	}
	return cleaned, nil
}

// parseChecksums parses "sha256  path" lines (sha256sum-style: hex digest,
// whitespace, path) into a map of clean path -> lowercase hex digest.
func parseChecksums(data []byte) (map[string]string, error) {
	checks := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return nil, fmt.Errorf("pkgtar: malformed %s line: %q", checksumsFileName, line)
		}
		name, err := cleanArchivePath(fields[1])
		if err != nil {
			return nil, err
		}
		checks[name] = strings.ToLower(fields[0])
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("pkgtar: reading %s: %w", checksumsFileName, err)
	}
	return checks, nil
}

// verifyChecksums checks that every file in files (other than checksums.txt
// and checksums.txt.sig) is listed in checks with a matching sha256, that
// no file is unlisted, and that checksums.txt doesn't reference a file that
// isn't in the archive.
func verifyChecksums(files map[string][]byte, checks map[string]string) error {
	for name, data := range files {
		if name == checksumsFileName || name == checksumsSigFileName {
			continue
		}
		want, ok := checks[name]
		if !ok {
			return fmt.Errorf("%w: %s", ErrUnlistedFile, name)
		}
		sum := sha256.Sum256(data)
		got := hex.EncodeToString(sum[:])
		if got != want {
			return fmt.Errorf("%w: %s", ErrChecksumMismatch, name)
		}
	}
	for name := range checks {
		if _, ok := files[name]; !ok {
			return fmt.Errorf("pkgtar: %s references missing file %s", checksumsFileName, name)
		}
	}
	return nil
}
