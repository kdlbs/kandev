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

func TestInstall_SignedTrueWhenSigPresent(t *testing.T) {
	destRoot := t.TempDir()
	files := buildValidFiles("1.0.0")

	// checksums.txt.sig is unverified by default (VerifySignature is nil,
	// meaning verification is skipped) — its bytes don't need to be a real
	// signature for this test.
	withSig := make(map[string][]byte, len(files)+1)
	for name, data := range files {
		withSig[name] = data
	}
	withSig["checksums.txt.sig"] = []byte("fake-signature-bytes")
	pkg := buildRawPackageWithChecksums(t, withSig)

	result, err := Install(bytes.NewReader(pkg), destRoot)
	if err != nil {
		t.Fatalf("Install() unexpected error: %v", err)
	}
	if !result.Signed {
		t.Fatal("Signed = false, want true (checksums.txt.sig present)")
	}
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
