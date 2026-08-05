package launcher

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateRuntimeBundleAcceptsSingleBinaryLayout(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "bin", "kandev"))
	writeFile(t, filepath.Join(dir, "bin", "agentctl"))
	writeRemoteAgentctlHelpers(t, dir)

	bundle, err := validateRuntimeBundle(dir, "test")
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Launcher != filepath.Join(dir, "bin", "kandev") {
		t.Fatalf("Launcher = %q", bundle.Launcher)
	}
}

func TestValidateRuntimeBundleRejectsMissingLauncher(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "bin", "agentctl"))

	if _, err := validateRuntimeBundle(dir, "test"); err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateRuntimeBundleRejectsMissingRemoteHelper(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "bin", "kandev"))
	writeFile(t, filepath.Join(dir, "bin", "agentctl"))
	writeFile(t, filepath.Join(dir, "bin", "agentctl-linux-amd64"))
	writeFile(t, filepath.Join(dir, "bin", "agentctl-linux-arm64"))
	writeFile(t, filepath.Join(dir, "bin", "agentctl-darwin-amd64"))

	_, err := validateRuntimeBundle(dir, "test")
	if err == nil {
		t.Fatal("expected error")
	}
	if got, want := err.Error(), "agentctl darwin/arm64 helper not found"; !strings.Contains(got, want) {
		t.Fatalf("error = %q, want substring %q", got, want)
	}
}

func TestValidateRuntimeBundleRejectsUnsignedDarwinArm64(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "bin", "kandev"))
	writeFile(t, filepath.Join(dir, "bin", "agentctl"))
	writeFile(t, filepath.Join(dir, "bin", "agentctl-linux-amd64"))
	writeFile(t, filepath.Join(dir, "bin", "agentctl-linux-arm64"))
	writeFile(t, filepath.Join(dir, "bin", "agentctl-darwin-amd64"))
	// A real-shaped but unsigned Mach-O must be rejected for darwin/arm64.
	writeMachO(t, filepath.Join(dir, "bin", "agentctl-darwin-arm64"), false)

	_, err := validateRuntimeBundle(dir, "test")
	if err == nil {
		t.Fatal("expected error for unsigned darwin/arm64 helper")
	}
	if got, want := err.Error(), "not code-signed"; !strings.Contains(got, want) {
		t.Fatalf("error = %q, want substring %q", got, want)
	}
}

func TestValidateRuntimeBundleRejectsUnparsableDarwinArm64(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "bin", "kandev"))
	writeFile(t, filepath.Join(dir, "bin", "agentctl"))
	writeFile(t, filepath.Join(dir, "bin", "agentctl-linux-amd64"))
	writeFile(t, filepath.Join(dir, "bin", "agentctl-linux-arm64"))
	writeFile(t, filepath.Join(dir, "bin", "agentctl-darwin-amd64"))
	// A non-Mach-O stub (e.g. a truncated/corrupt upload) can't be verified as
	// signed and must fail closed rather than silently pass.
	writeFile(t, filepath.Join(dir, "bin", "agentctl-darwin-arm64"))

	_, err := validateRuntimeBundle(dir, "test")
	if err == nil {
		t.Fatal("expected error for unparsable darwin/arm64 helper")
	}
	if got, want := err.Error(), "not a parsable"; !strings.Contains(got, want) {
		t.Fatalf("error = %q, want substring %q", got, want)
	}
}

func TestValidateRuntimeBundleAcceptsSignedDarwinArm64(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "bin", "kandev"))
	writeFile(t, filepath.Join(dir, "bin", "agentctl"))
	writeFile(t, filepath.Join(dir, "bin", "agentctl-linux-amd64"))
	writeFile(t, filepath.Join(dir, "bin", "agentctl-linux-arm64"))
	writeFile(t, filepath.Join(dir, "bin", "agentctl-darwin-amd64"))
	writeMachO(t, filepath.Join(dir, "bin", "agentctl-darwin-arm64"), true)

	if _, err := validateRuntimeBundle(dir, "test"); err != nil {
		t.Fatalf("unexpected error for signed darwin/arm64 helper: %v", err)
	}
}

func TestMachoHasCodeSignature(t *testing.T) {
	dir := t.TempDir()
	signed := filepath.Join(dir, "signed")
	unsigned := filepath.Join(dir, "unsigned")
	writeMachO(t, signed, true)
	writeMachO(t, unsigned, false)

	if got, ok := machoHasCodeSignature(signed); !ok || !got {
		t.Fatalf("signed: got=%v ok=%v, want signed=true ok=true", got, ok)
	}
	if got, ok := machoHasCodeSignature(unsigned); !ok || got {
		t.Fatalf("unsigned: got=%v ok=%v, want false true", got, ok)
	}
	// A non-Mach-O (e.g. an ELF or stub) must be reported as undeterminable.
	if _, ok := machoHasCodeSignature(filepath.Join(dir, "missing")); ok {
		t.Fatal("missing file: ok=true, want false")
	}
}

// writeMachO writes a minimal thin LE 64-bit Mach-O with a single load command
// that is either LC_CODE_SIGNATURE (signed) or LC_UUID (unsigned).
func writeMachO(t *testing.T, path string, signed bool) {
	t.Helper()
	const (
		magic64LE       = 0xfeedfacf
		lcCodeSignature = 0x1d
		lcUUID          = 0x1b
		hdrSize         = 32
		lcSize          = 16
	)
	buf := make([]byte, hdrSize+lcSize)
	binary.LittleEndian.PutUint32(buf[0:4], magic64LE)
	binary.LittleEndian.PutUint32(buf[16:20], 1) // ncmds = 1
	cmd := uint32(lcUUID)
	if signed {
		cmd = lcCodeSignature
	}
	binary.LittleEndian.PutUint32(buf[hdrSize:hdrSize+4], cmd)
	binary.LittleEndian.PutUint32(buf[hdrSize+4:hdrSize+8], lcSize)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf, 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestRequiredAgentctlRemoteHelpers(t *testing.T) {
	got := make([]string, 0, len(requiredAgentctlRemoteHelpers))
	for _, helper := range requiredAgentctlRemoteHelpers {
		got = append(got, helper.Name)
	}
	want := []string{
		"agentctl-linux-amd64",
		"agentctl-linux-arm64",
		"agentctl-darwin-arm64",
		"agentctl-darwin-amd64",
	}
	if len(got) != len(want) {
		t.Fatalf("helpers = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("helpers = %v, want %v", got, want)
		}
	}
}

func TestBundleDirForLauncherPathAcceptsBinLayout(t *testing.T) {
	dir := filepath.Join("opt", "kandev")
	got, ok := bundleDirForLauncherPath(filepath.Join(dir, "bin", executableName("kandev")))
	if !ok {
		t.Fatal("expected the <bundle>/bin/kandev layout to be accepted")
	}
	if got != dir {
		t.Fatalf("bundle dir = %q, want %q", got, dir)
	}
}

func TestBundleDirForLauncherPathRejectsLauncherOutsideBin(t *testing.T) {
	for _, exe := range []string{
		filepath.Join("opt", "kandev", executableName("kandev")),
		filepath.Join("usr", "local", "sbin", executableName("kandev")),
		executableName("kandev"),
	} {
		if got, ok := bundleDirForLauncherPath(exe); ok {
			t.Fatalf("exe %q was accepted as bundle %q", exe, got)
		}
	}
}

func TestResolveRuntimeBundlePrefersEnvOverExecutable(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "bin", executableName("kandev")))
	writeFile(t, filepath.Join(dir, "bin", executableName("agentctl")))
	writeRemoteAgentctlHelpers(t, dir)
	t.Setenv("KANDEV_BUNDLE_DIR", dir)

	bundle, err := resolveRuntimeBundle()
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Source != "env" {
		t.Fatalf("Source = %q, want %q", bundle.Source, "env")
	}
	if bundle.Dir != dir {
		t.Fatalf("Dir = %q, want %q", bundle.Dir, dir)
	}
}

func TestResolveRuntimeBundleFallsBackToExecutable(t *testing.T) {
	dir := t.TempDir()
	launcher := filepath.Join(dir, "bin", executableName("kandev"))
	writeFile(t, launcher)
	writeFile(t, filepath.Join(dir, "bin", executableName("agentctl")))
	writeRemoteAgentctlHelpers(t, dir)
	t.Setenv("KANDEV_BUNDLE_DIR", "")

	original := executablePath
	t.Cleanup(func() { executablePath = original })
	executablePath = func() (string, error) { return launcher, nil }

	bundle, err := resolveRuntimeBundle()
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Source != "executable" {
		t.Fatalf("Source = %q, want %q", bundle.Source, "executable")
	}
	// The lookup resolves symlinks, so compare against a resolved path. On
	// Windows t.TempDir can sit under an 8.3 short name (JOOMAN~1) that
	// EvalSymlinks expands, and on macOS /var is a symlink to /private/var.
	wantDir := resolvedPath(t, dir)
	if bundle.Dir != wantDir {
		t.Fatalf("Dir = %q, want %q", bundle.Dir, wantDir)
	}
	wantLauncher := filepath.Join(wantDir, "bin", executableName("kandev"))
	if bundle.Launcher != wantLauncher {
		t.Fatalf("Launcher = %q, want %q", bundle.Launcher, wantLauncher)
	}
}

func resolvedPath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return path
	}
	return resolved
}

func TestResolveRuntimeBundleReportsBothLookupsInError(t *testing.T) {
	t.Setenv("KANDEV_BUNDLE_DIR", "")

	// The test binary does not live in a <bundle>/bin directory, so the
	// executable-relative lookup finds nothing and the error must say so rather
	// than blaming the environment variable alone.
	_, err := resolveRuntimeBundle()
	if err == nil {
		t.Fatal("expected an error when neither lookup resolves")
	}
	if !strings.Contains(err.Error(), "KANDEV_BUNDLE_DIR") {
		t.Fatalf("error does not mention the env var: %v", err)
	}
	if !strings.Contains(err.Error(), "next to the launcher") {
		t.Fatalf("error does not mention the executable-relative lookup: %v", err)
	}
}

func writeRemoteAgentctlHelpers(t *testing.T, dir string) {
	t.Helper()
	for _, helper := range requiredAgentctlRemoteHelpers {
		path := filepath.Join(dir, "bin", helper.Name)
		// MustBeSigned helpers now fail closed on an unparsable artifact, so a
		// plain stub no longer passes validation — write a signed Mach-O.
		if helper.MustBeSigned {
			writeMachO(t, path, true)
			continue
		}
		writeFile(t, path)
	}
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
}
