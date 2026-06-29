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
		t.Fatalf("signed: got=%v ok=%v, want true true", got, ok)
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

func writeRemoteAgentctlHelpers(t *testing.T, dir string) {
	t.Helper()
	for _, helper := range requiredAgentctlRemoteHelpers {
		writeFile(t, filepath.Join(dir, "bin", helper.Name))
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
