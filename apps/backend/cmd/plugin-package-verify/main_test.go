package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/kandev/kandev/internal/plugins/pkgtar/pkgtartest"
)

func writeTestPackage(t *testing.T, id, version string) (string, string) {
	t.Helper()
	platform := runtime.GOOS + "-" + runtime.GOARCH
	manifest := fmt.Sprintf(`id: %q
api_version: 1
version: %q
display_name: "Verifier fixture"
runtime:
  type: binary
  executables:
    %s: "server/plugin-%s"
`, id, version, platform, platform)
	var archive bytes.Buffer
	if err := pkgtartest.WritePackage(&archive, map[string][]byte{
		"manifest.yaml":             []byte(manifest),
		"server/plugin-" + platform: []byte("binary"),
	}); err != nil {
		t.Fatalf("WritePackage() error: %v", err)
	}
	archivePath := filepath.Join(t.TempDir(), id+"-"+version+".tar.gz")
	if err := os.WriteFile(archivePath, archive.Bytes(), 0o600); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}
	sum := sha256.Sum256(archive.Bytes())
	return archivePath, hex.EncodeToString(sum[:])
}

func TestRunVerifiesExpectedIdentityAndEmitsDigest(t *testing.T) {
	archivePath, wantDigest := writeTestPackage(t, "kandev-plugin-test", "1.2.3")
	var stdout, stderr bytes.Buffer

	exitCode := run([]string{
		"--archive", archivePath,
		"--expected-id", "kandev-plugin-test",
		"--expected-version", "1.2.3",
	}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("run() exit = %d, stderr = %q", exitCode, stderr.String())
	}
	var got output
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error: %v", err)
	}
	if got.ID != "kandev-plugin-test" || got.Version != "1.2.3" || got.SHA256 != wantDigest {
		t.Fatalf("output = %+v, want verified identity and digest %s", got, wantDigest)
	}
}

func TestRunRejectsManifestVersionMismatch(t *testing.T) {
	archivePath, _ := writeTestPackage(t, "kandev-plugin-test", "1.2.3")
	var stdout, stderr bytes.Buffer

	exitCode := run([]string{
		"--archive", archivePath,
		"--expected-id", "kandev-plugin-test",
		"--expected-version", "1.2.4",
	}, &stdout, &stderr)
	if exitCode == 0 {
		t.Fatalf("run() exit = 0, stdout = %q", stdout.String())
	}
	if got := stderr.String(); got == "" {
		t.Fatal("run() emitted no mismatch evidence")
	}
}
