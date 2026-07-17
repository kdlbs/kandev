package pkgtar

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/plugins/pkgtar/pkgtartest"
)

// hostPlatformKey is the "<goos>-<goarch>" key the current test process
// runs under, matching manifest.ExecutableFor(runtime.GOOS, runtime.GOARCH).
var hostPlatformKey = runtime.GOOS + "-" + runtime.GOARCH

const managedManifestTemplate = `
id: "kandev-plugin-hello"
api_version: 1
version: %q
display_name: "Hello Plugin"
description: "A runtime-managed example plugin"
author: "kandev"
categories: ["tools"]

runtime:
  type: binary
  executables:
    %s: "server/plugin-%s"

capabilities:
  state: true
`

// managedManifestYAML returns a valid runtime-managed manifest YAML for the
// given version, declaring exactly one executable for the host platform.
func managedManifestYAML(version string) []byte {
	return []byte(fmt.Sprintf(managedManifestTemplate, version, hostPlatformKey, hostPlatformKey))
}

// buildValidFiles returns the file set for a minimal, valid, multi-file
// package: manifest.yaml, the host-platform executable, and a UI asset.
func buildValidFiles(version string) map[string][]byte {
	return map[string][]byte{
		"manifest.yaml":                    managedManifestYAML(version),
		"server/plugin-" + hostPlatformKey: []byte("#!/bin/sh\necho hello\n"),
		"ui/bundle.js":                     []byte("export default {};\n"),
		"assets/icon.svg":                  []byte("<svg></svg>"),
	}
}

// writeValidPackage builds a valid signed-less package and returns its
// tar.gz bytes.
func writeValidPackage(t *testing.T, version string) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := pkgtartest.WritePackage(&buf, buildValidFiles(version)); err != nil {
		t.Fatalf("pkgtartest.WritePackage() unexpected error: %v", err)
	}
	return buf.Bytes()
}

func TestInstall_HappyPathMultiFileHostPlatform(t *testing.T) {
	destRoot := t.TempDir()
	pkg := writeValidPackage(t, "1.0.0")

	result, err := Install(bytes.NewReader(pkg), destRoot)
	if err != nil {
		t.Fatalf("Install() unexpected error: %v", err)
	}

	wantPath := filepath.Join(destRoot, "kandev-plugin-hello", "1.0.0")
	if result.InstallPath != wantPath {
		t.Fatalf("InstallPath = %q, want %q", result.InstallPath, wantPath)
	}
	if result.Version != "1.0.0" {
		t.Fatalf("Version = %q, want %q", result.Version, "1.0.0")
	}
	if result.Signed {
		t.Fatal("Signed = true, want false (no checksums.txt.sig in package)")
	}
	if result.Manifest == nil || result.Manifest.ID != "kandev-plugin-hello" {
		t.Fatalf("Manifest = %+v, want ID kandev-plugin-hello", result.Manifest)
	}

	for _, rel := range []string{"manifest.yaml", "server/plugin-" + hostPlatformKey, "ui/bundle.js", "assets/icon.svg", "checksums.txt"} {
		if _, err := os.Stat(filepath.Join(wantPath, rel)); err != nil {
			t.Fatalf("expected extracted file %s: %v", rel, err)
		}
	}
}

func TestInstall_ChmodsExecutableTo0755(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file mode bits are not meaningful on windows")
	}
	destRoot := t.TempDir()
	pkg := writeValidPackage(t, "1.0.0")

	result, err := Install(bytes.NewReader(pkg), destRoot)
	if err != nil {
		t.Fatalf("Install() unexpected error: %v", err)
	}

	execPath := filepath.Join(result.InstallPath, "server", "plugin-"+hostPlatformKey)
	info, err := os.Stat(execPath)
	if err != nil {
		t.Fatalf("stat executable: %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("executable mode = %o, want %o", info.Mode().Perm(), 0o755)
	}

	// A non-executable file should not have been made executable.
	assetPath := filepath.Join(result.InstallPath, "assets", "icon.svg")
	assetInfo, err := os.Stat(assetPath)
	if err != nil {
		t.Fatalf("stat asset: %v", err)
	}
	if assetInfo.Mode().Perm() == 0o755 {
		t.Fatalf("asset mode = %o, want an unexecuted mode (e.g. 0644)", assetInfo.Mode().Perm())
	}
}

// packageWithFakeSig returns buildValidFiles(version) plus an unverifiable
// checksums.txt.sig entry (its bytes don't need to be a real signature
// unless the test also wires a VerifySignature that inspects them).
func packageWithFakeSig(version string) map[string][]byte {
	files := buildValidFiles(version)
	withSig := make(map[string][]byte, len(files)+1)
	for name, data := range files {
		withSig[name] = data
	}
	withSig["checksums.txt.sig"] = []byte("fake-signature-bytes")
	return withSig
}

// TestInstall_SignedFalseWhenSigPresentButUnverified pins the fix for
// Install claiming Signed=true just because a checksums.txt.sig entry
// exists, even with no VerifySignature verifier configured (the default).
// Signed must only be true once a verifier actually ran and succeeded — a
// present-but-unverified signature is not a "signed" guarantee.
func TestInstall_SignedFalseWhenSigPresentButUnverified(t *testing.T) {
	destRoot := t.TempDir()
	pkg := buildRawPackageWithChecksums(t, packageWithFakeSig("1.0.0"))

	result, err := Install(bytes.NewReader(pkg), destRoot)
	if err != nil {
		t.Fatalf("Install() unexpected error: %v", err)
	}
	if result.Signed {
		t.Fatal("Signed = true, want false: checksums.txt.sig is present but no VerifySignature verifier is configured")
	}
}

// TestInstall_SignedTrueWhenSignatureVerifies pins that Signed=true only
// once a configured VerifySignature verifier actually ran and succeeded.
func TestInstall_SignedTrueWhenSignatureVerifies(t *testing.T) {
	destRoot := t.TempDir()
	pkg := buildRawPackageWithChecksums(t, packageWithFakeSig("1.0.0"))

	var gotSig, gotChecksums []byte
	VerifySignature = func(sig, checksums []byte) error {
		gotSig, gotChecksums = sig, checksums
		return nil
	}
	t.Cleanup(func() { VerifySignature = nil })

	result, err := Install(bytes.NewReader(pkg), destRoot)
	if err != nil {
		t.Fatalf("Install() unexpected error: %v", err)
	}
	if !result.Signed {
		t.Fatal("Signed = false, want true: a configured VerifySignature verifier succeeded")
	}
	if string(gotSig) != "fake-signature-bytes" {
		t.Fatalf("VerifySignature sig arg = %q, want the checksums.txt.sig bytes", gotSig)
	}
	if len(gotChecksums) == 0 {
		t.Fatal("VerifySignature checksums arg was empty, want the checksums.txt bytes")
	}
}

// TestInstall_ErrorsWhenSignatureVerificationFails pins that Install fails
// outright (package not extracted) when a configured VerifySignature
// verifier rejects the signature — this is a "signed but tampered/invalid"
// package, not merely "unsigned".
func TestInstall_ErrorsWhenSignatureVerificationFails(t *testing.T) {
	destRoot := t.TempDir()
	pkg := buildRawPackageWithChecksums(t, packageWithFakeSig("1.0.0"))

	VerifySignature = func(_, _ []byte) error { return errors.New("bad signature") }
	t.Cleanup(func() { VerifySignature = nil })

	_, err := Install(bytes.NewReader(pkg), destRoot)
	if err == nil {
		t.Fatal("Install() expected error when signature verification fails, got nil")
	}
	assertNoPartialInstall(t, destRoot, "kandev-plugin-hello")
}

func TestInspect_ReturnsManifestWithoutSideEffects(t *testing.T) {
	pkg := writeValidPackage(t, "2.3.4")
	watchDir := t.TempDir()

	m, err := Inspect(bytes.NewReader(pkg))
	if err != nil {
		t.Fatalf("Inspect() unexpected error: %v", err)
	}
	if m.ID != "kandev-plugin-hello" {
		t.Fatalf("m.ID = %q, want %q", m.ID, "kandev-plugin-hello")
	}
	if m.Version != "2.3.4" {
		t.Fatalf("m.Version = %q, want %q", m.Version, "2.3.4")
	}

	entries, err := os.ReadDir(watchDir)
	if err != nil {
		t.Fatalf("ReadDir() unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("Inspect() wrote %d entries to an unrelated dir, want 0 (no disk side effects)", len(entries))
	}
}

func TestInspect_RejectsInvalidManifest(t *testing.T) {
	files := map[string][]byte{
		"manifest.yaml": []byte("id: \"Bad Id!\"\napi_version: 1\nversion: \"1.0.0\"\n"),
	}
	var buf bytes.Buffer
	if err := pkgtartest.WritePackage(&buf, files); err != nil {
		t.Fatalf("pkgtartest.WritePackage() unexpected error: %v", err)
	}

	if _, err := Inspect(bytes.NewReader(buf.Bytes())); err == nil {
		t.Fatal("Inspect() expected error for invalid manifest, got nil")
	}
}

func TestInstall_BadChecksumRejected(t *testing.T) {
	destRoot := t.TempDir()
	files := buildValidFiles("1.0.0")
	pkg := buildRawPackageWithBadChecksum(t, files, "ui/bundle.js")

	_, err := Install(bytes.NewReader(pkg), destRoot)
	if err == nil {
		t.Fatal("Install() expected error for bad checksum, got nil")
	}
	if !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("Install() error = %v, want ErrChecksumMismatch", err)
	}
	assertNoPartialInstall(t, destRoot, "kandev-plugin-hello")
}

func TestInstall_MissingChecksumsRejected(t *testing.T) {
	destRoot := t.TempDir()
	files := buildValidFiles("1.0.0")
	pkg := buildRawPackage(t, files) // no checksums.txt entry

	_, err := Install(bytes.NewReader(pkg), destRoot)
	if err == nil {
		t.Fatal("Install() expected error for missing checksums.txt, got nil")
	}
	if !errors.Is(err, ErrMissingChecksums) {
		t.Fatalf("Install() error = %v, want ErrMissingChecksums", err)
	}
}

func TestInstall_UnlistedFileRejected(t *testing.T) {
	destRoot := t.TempDir()
	files := buildValidFiles("1.0.0")

	var base bytes.Buffer
	baseFiles := map[string][]byte{
		"manifest.yaml":                    files["manifest.yaml"],
		"server/plugin-" + hostPlatformKey: files["server/plugin-"+hostPlatformKey],
	}
	if err := pkgtartest.WritePackage(&base, baseFiles); err != nil {
		t.Fatalf("pkgtartest.WritePackage() unexpected error: %v", err)
	}
	extracted := extractAll(t, base.Bytes())

	// Add a file that is NOT covered by checksums.txt.
	withExtra := map[string][]byte{
		"manifest.yaml":                    files["manifest.yaml"],
		"server/plugin-" + hostPlatformKey: files["server/plugin-"+hostPlatformKey],
		"ui/bundle.js":                     files["ui/bundle.js"], // unlisted
		"checksums.txt":                    extracted["checksums.txt"],
	}
	pkg := buildRawPackage(t, withExtra)

	_, err := Install(bytes.NewReader(pkg), destRoot)
	if err == nil {
		t.Fatal("Install() expected error for unlisted file, got nil")
	}
	if !errors.Is(err, ErrUnlistedFile) {
		t.Fatalf("Install() error = %v, want ErrUnlistedFile", err)
	}
}

func TestInstall_TraversalPathRejected(t *testing.T) {
	destRoot := t.TempDir()
	files := buildValidFiles("1.0.0")
	files["../evil.txt"] = []byte("pwned")
	pkg := buildRawPackageWithChecksums(t, files)

	_, err := Install(bytes.NewReader(pkg), destRoot)
	if err == nil {
		t.Fatal("Install() expected error for path traversal entry, got nil")
	}
	if !errors.Is(err, ErrPathTraversal) {
		t.Fatalf("Install() error = %v, want ErrPathTraversal", err)
	}
}

func TestInstall_AbsolutePathRejected(t *testing.T) {
	destRoot := t.TempDir()
	files := buildValidFiles("1.0.0")
	files["/etc/passwd"] = []byte("pwned")
	pkg := buildRawPackageWithChecksums(t, files)

	_, err := Install(bytes.NewReader(pkg), destRoot)
	if err == nil {
		t.Fatal("Install() expected error for absolute path entry, got nil")
	}
	if !errors.Is(err, ErrPathTraversal) {
		t.Fatalf("Install() error = %v, want ErrPathTraversal", err)
	}
}

// TestInstall_BackslashTraversalRejected pins the fix for a Windows-style
// traversal entry: cleanArchivePath previously only treated "/" as a path
// separator, so "server/..\\..\\escape.exe" passed path.Clean unchanged (it
// has no leading ".." segment under POSIX splitting) and would later be
// interpreted by filepath.Join/filepath.Clean on Windows as an actual
// escape via the backslash-delimited "..\\.." segments.
func TestInstall_BackslashTraversalRejected(t *testing.T) {
	destRoot := t.TempDir()
	files := buildValidFiles("1.0.0")
	files[`server/..\..\escape.exe`] = []byte("pwned")
	pkg := buildRawPackageWithChecksums(t, files)

	_, err := Install(bytes.NewReader(pkg), destRoot)
	if err == nil {
		t.Fatal("Install() expected error for a backslash-traversal entry, got nil")
	}
	if !errors.Is(err, ErrPathTraversal) {
		t.Fatalf("Install() error = %v, want ErrPathTraversal", err)
	}
}

func TestCleanArchivePath_RejectsBackslash(t *testing.T) {
	for _, name := range []string{
		`server\evil.exe`,
		`server/..\..\escape.exe`,
		`..\evil.exe`,
	} {
		if _, err := cleanArchivePath(name); err == nil {
			t.Fatalf("cleanArchivePath(%q) expected error, got nil", name)
		} else if !errors.Is(err, ErrPathTraversal) {
			t.Fatalf("cleanArchivePath(%q) error = %v, want ErrPathTraversal", name, err)
		}
	}
}

// withSmallSizeCaps temporarily overrides maxPackageFileSize/
// maxPackageTotalSize (test-only vars, see pkgtar.go) so cap-enforcement
// tests can use small fixtures instead of allocating real 200MB payloads.
func withSmallSizeCaps(t *testing.T, perFile, total int64) {
	t.Helper()
	origFile, origTotal := maxPackageFileSize, maxPackageTotalSize
	maxPackageFileSize, maxPackageTotalSize = perFile, total
	t.Cleanup(func() { maxPackageFileSize, maxPackageTotalSize = origFile, origTotal })
}

func TestInstall_SingleFileExceedingPerFileCapRejected(t *testing.T) {
	withSmallSizeCaps(t, 100, 10_000)
	destRoot := t.TempDir()

	files := buildValidFiles("1.0.0")
	files["assets/icon.svg"] = bytes.Repeat([]byte("A"), 200) // exceeds the 100-byte per-file cap
	pkg := buildRawPackageWithChecksums(t, files)

	_, err := Install(bytes.NewReader(pkg), destRoot)
	if err == nil {
		t.Fatal("Install() expected error for a file exceeding the per-file size cap, got nil")
	}
}

// TestReadArchive_TotalSizeCapUsesActualBytesCopied pins the fix that the
// running total is accumulated from bytes actually copied out of the
// decompressed stream (readCapped's return value), not the archive-declared
// hdr.Size: three files that individually fit under the per-file cap but
// whose combined ACTUAL bytes exceed the total cap must still be rejected.
func TestReadArchive_TotalSizeCapUsesActualBytesCopied(t *testing.T) {
	withSmallSizeCaps(t, 1000, 2000)
	destRoot := t.TempDir()

	files := buildValidFiles("1.0.0")
	files["assets/a"] = bytes.Repeat([]byte("A"), 900)
	files["assets/b"] = bytes.Repeat([]byte("B"), 900)
	files["assets/c"] = bytes.Repeat([]byte("C"), 900) // pushes the real total past the 2000-byte cap
	pkg := buildRawPackageWithChecksums(t, files)

	_, err := Install(bytes.NewReader(pkg), destRoot)
	if err == nil {
		t.Fatal("Install() expected error once actual bytes copied exceed the total size cap, got nil")
	}
}

func TestReadArchive_UnderTotalCapSucceeds(t *testing.T) {
	withSmallSizeCaps(t, 1000, 100_000)
	destRoot := t.TempDir()

	pkg := writeValidPackage(t, "1.0.0") // well under 100_000 bytes total
	if _, err := Install(bytes.NewReader(pkg), destRoot); err != nil {
		t.Fatalf("Install() unexpected error under the total size cap: %v", err)
	}
}

func TestInstall_SymlinkEntryRejected(t *testing.T) {
	destRoot := t.TempDir()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	files := buildValidFiles("1.0.0")
	for name, data := range files {
		writeRawEntry(t, tw, name, data, tar.TypeReg)
	}
	writeRawEntry(t, tw, "server/evil-link", nil, tar.TypeSymlink)
	checksums := checksumsFor(files)
	writeRawEntry(t, tw, "checksums.txt", checksums, tar.TypeReg)
	if err := tw.Close(); err != nil {
		t.Fatalf("tar Close() error: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip Close() error: %v", err)
	}

	_, err := Install(bytes.NewReader(buf.Bytes()), destRoot)
	if err == nil {
		t.Fatal("Install() expected error for symlink entry, got nil")
	}
}

func TestInstall_MissingHostPlatformKeyRejected(t *testing.T) {
	destRoot := t.TempDir()
	otherPlatform := "plan9-arm"
	manifestYAML := []byte(fmt.Sprintf(managedManifestTemplate, "1.0.0", otherPlatform, otherPlatform))
	files := map[string][]byte{
		"manifest.yaml":                  manifestYAML,
		"server/plugin-" + otherPlatform: []byte("binary"),
	}
	pkg := buildRawPackageWithChecksums(t, files)

	_, err := Install(bytes.NewReader(pkg), destRoot)
	if err == nil {
		t.Fatal("Install() expected error for missing host platform key, got nil")
	}
	if !errors.Is(err, ErrPlatformNotSupported) {
		t.Fatalf("Install() error = %v, want ErrPlatformNotSupported", err)
	}
}

func TestInstall_DuplicateVersionRejected(t *testing.T) {
	destRoot := t.TempDir()
	pkg := writeValidPackage(t, "1.0.0")

	if _, err := Install(bytes.NewReader(pkg), destRoot); err != nil {
		t.Fatalf("first Install() unexpected error: %v", err)
	}

	pkg2 := writeValidPackage(t, "1.0.0")
	_, err := Install(bytes.NewReader(pkg2), destRoot)
	if err == nil {
		t.Fatal("second Install() expected ErrVersionExists, got nil")
	}
	if !errors.Is(err, ErrVersionExists) {
		t.Fatalf("Install() error = %v, want ErrVersionExists", err)
	}
}

func TestInstall_InvalidManifestRejected(t *testing.T) {
	destRoot := t.TempDir()
	files := map[string][]byte{
		"manifest.yaml": []byte("id: \"Bad Id!\"\napi_version: 1\nversion: \"1.0.0\"\n"),
	}
	pkg := buildRawPackageWithChecksums(t, files)

	_, err := Install(bytes.NewReader(pkg), destRoot)
	if err == nil {
		t.Fatal("Install() expected error for invalid manifest, got nil")
	}
	if !errors.Is(err, ErrManifestInvalid) {
		t.Fatalf("Install() error = %v, want ErrManifestInvalid", err)
	}
}

func TestInstall_LegacyRemoteManifestRejected(t *testing.T) {
	destRoot := t.TempDir()
	legacyManifest := []byte(`
id: "kandev-plugin-legacy"
api_version: 1
version: "1.0.0"
display_name: "Legacy"
description: "legacy remote plugin"
author: "kandev"
categories: ["tools"]
base_url: "http://localhost:9100"
endpoints:
  health: "/health"
  events: "/events"
  tools: "/tools/{tool_name}"
  webhooks: "/webhooks/{webhook_key}"
`)
	files := map[string][]byte{"manifest.yaml": legacyManifest}
	pkg := buildRawPackageWithChecksums(t, files)

	_, err := Install(bytes.NewReader(pkg), destRoot)
	if err == nil {
		t.Fatal("Install() expected error for legacy-remote (non-managed) manifest, got nil")
	}
	if !errors.Is(err, ErrManifestInvalid) {
		t.Fatalf("Install() error = %v, want ErrManifestInvalid", err)
	}
}

func TestInstall_AtomicNoPartialDirOnFailure(t *testing.T) {
	destRoot := t.TempDir()
	files := buildValidFiles("1.0.0")
	pkg := buildRawPackageWithBadChecksum(t, files, "manifest.yaml")

	if _, err := Install(bytes.NewReader(pkg), destRoot); err == nil {
		t.Fatal("Install() expected error, got nil")
	}
	assertNoPartialInstall(t, destRoot, "kandev-plugin-hello")
}

func TestRemove_DeletesAllVersionsAndData(t *testing.T) {
	destRoot := t.TempDir()
	pkg1 := writeValidPackage(t, "1.0.0")
	if _, err := Install(bytes.NewReader(pkg1), destRoot); err != nil {
		t.Fatalf("Install() unexpected error: %v", err)
	}
	pkg2 := writeValidPackage(t, "2.0.0")
	if _, err := Install(bytes.NewReader(pkg2), destRoot); err != nil {
		t.Fatalf("Install() unexpected error: %v", err)
	}

	pluginDir := filepath.Join(destRoot, "kandev-plugin-hello")
	dataDir := filepath.Join(pluginDir, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(data) error: %v", err)
	}

	if err := Remove(destRoot, "kandev-plugin-hello"); err != nil {
		t.Fatalf("Remove() unexpected error: %v", err)
	}

	if _, err := os.Stat(pluginDir); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("plugin dir still exists after Remove(): err = %v", err)
	}
}

func TestRemove_RejectsUnsafeID(t *testing.T) {
	destRoot := t.TempDir()
	if err := Remove(destRoot, "../escape"); err == nil {
		t.Fatal("Remove() expected error for unsafe id, got nil")
	}
}

// --- test helpers: raw archive construction for negative-path scenarios ---

func buildRawPackage(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, data := range files {
		writeRawEntry(t, tw, name, data, tar.TypeReg)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar Close() error: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip Close() error: %v", err)
	}
	return buf.Bytes()
}

func buildRawPackageWithChecksums(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	withChecksums := make(map[string][]byte, len(files)+1)
	for name, data := range files {
		withChecksums[name] = data
	}
	withChecksums["checksums.txt"] = checksumsFor(files)
	return buildRawPackage(t, withChecksums)
}

func buildRawPackageWithBadChecksum(t *testing.T, files map[string][]byte, corrupt string) []byte {
	t.Helper()
	checksums := checksumsFor(files)
	// Flip a character in the recorded hash for `corrupt` so verification fails.
	lines := strings.Split(string(checksums), "\n")
	for i, line := range lines {
		if strings.HasSuffix(line, "  "+corrupt) {
			lines[i] = "0000000000000000000000000000000000000000000000000000000000000000  " + corrupt
		}
	}
	withChecksums := make(map[string][]byte, len(files)+1)
	for name, data := range files {
		withChecksums[name] = data
	}
	withChecksums["checksums.txt"] = []byte(strings.Join(lines, "\n"))
	return buildRawPackage(t, withChecksums)
}

// checksumsFor computes "sha256  path" lines for every entry in files,
// excluding checksums.txt and checksums.txt.sig themselves (per the
// package format: checksums.txt lists every OTHER file).
func checksumsFor(files map[string][]byte) []byte {
	var buf bytes.Buffer
	for name, data := range files {
		if name == "checksums.txt" || name == "checksums.txt.sig" {
			continue
		}
		sum := sha256.Sum256(data)
		fmt.Fprintf(&buf, "%s  %s\n", hex.EncodeToString(sum[:]), name)
	}
	return buf.Bytes()
}

func writeRawEntry(t *testing.T, tw *tar.Writer, name string, data []byte, typeflag byte) {
	t.Helper()
	hdr := &tar.Header{
		Name:     name,
		Typeflag: typeflag,
		Mode:     0o644,
		Size:     int64(len(data)),
	}
	if typeflag == tar.TypeSymlink {
		hdr.Linkname = "/etc/passwd"
		hdr.Size = 0
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("WriteHeader(%s) error: %v", name, err)
	}
	if len(data) > 0 {
		if _, err := tw.Write(data); err != nil {
			t.Fatalf("Write(%s) error: %v", name, err)
		}
	}
}

// extractAll un-gzips and un-tars pkg into a name->data map, for tests that
// need to inspect or reuse a generated checksums.txt.
func extractAll(t *testing.T, pkg []byte) map[string][]byte {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(pkg))
	if err != nil {
		t.Fatalf("gzip.NewReader() error: %v", err)
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	out := make(map[string][]byte)
	for {
		hdr, err := tr.Next()
		if err != nil {
			break
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatalf("reading tar entry %s: %v", hdr.Name, err)
		}
		out[hdr.Name] = data
	}
	return out
}

func assertNoPartialInstall(t *testing.T, destRoot, id string) {
	t.Helper()
	pluginDir := filepath.Join(destRoot, id)
	entries, err := os.ReadDir(pluginDir)
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	if err != nil {
		t.Fatalf("ReadDir(%s) error: %v", pluginDir, err)
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), ".tmp-") {
			t.Fatalf("found non-tmp entry %q after failed Install(), want no partial version dir", e.Name())
		}
	}
}

func TestSecureJoin_RejectsTraversalRelativePath(t *testing.T) {
	root := t.TempDir()

	_, err := securejoin(root, "../evil.txt")
	if err == nil {
		t.Fatal("securejoin() expected error for traversal path, got nil")
	}
	if !errors.Is(err, ErrPathTraversal) {
		t.Fatalf("securejoin() error = %v, want ErrPathTraversal", err)
	}
}

func TestSecureJoin_RejectsDeeplyNestedTraversal(t *testing.T) {
	root := t.TempDir()

	_, err := securejoin(root, "server/../../evil.txt")
	if err == nil {
		t.Fatal("securejoin() expected error for nested traversal path, got nil")
	}
	if !errors.Is(err, ErrPathTraversal) {
		t.Fatalf("securejoin() error = %v, want ErrPathTraversal", err)
	}
}

func TestSecureJoin_RejectsAbsolutePath(t *testing.T) {
	root := t.TempDir()

	_, err := securejoin(root, "/etc/passwd")
	if err == nil {
		t.Fatal("securejoin() expected error for absolute path, got nil")
	}
	if !errors.Is(err, ErrPathTraversal) {
		t.Fatalf("securejoin() error = %v, want ErrPathTraversal", err)
	}
}

func TestSecureJoin_AllowsCleanRelativePath(t *testing.T) {
	root := t.TempDir()

	got, err := securejoin(root, filepath.Join("server", "plugin-linux-amd64"))
	if err != nil {
		t.Fatalf("securejoin() unexpected error: %v", err)
	}
	want := filepath.Join(root, "server", "plugin-linux-amd64")
	if got != want {
		t.Fatalf("securejoin() = %q, want %q", got, want)
	}
}
