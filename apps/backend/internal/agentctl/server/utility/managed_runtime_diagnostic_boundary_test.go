package utility

import (
	"strings"
	"testing"
)

func TestManagedRuntimeDiagnosticDiscardsOverlongLineSuffix(t *testing.T) {
	buffer := &stderrBuffer{}
	_, _ = buffer.Write([]byte(strings.Repeat("x", maxManagedRuntimeRawNPMDiagnosticBytes+1)))
	_, _ = buffer.Write([]byte("npm error code ENOENT\nnpm error path /cache/_npx/key/package.json\n"))
	diagnostic, _ := buffer.managedNPMDiagnosticSnapshot()
	if strings.Contains(diagnostic, "code ENOENT") {
		t.Fatalf("overlong line suffix was accepted as a fresh diagnostic: %q", diagnostic)
	}
}

func TestManagedRuntimeDiagnosticRedactsPathsContainingSpaces(t *testing.T) {
	input := "npm error code ENOENT\nnpm error syscall open\nnpm error path /home/Private User/cache/package.json\nnpm error enoent Could not read package.json: Error: ENOENT: no such file or directory, open '/home/Private User/cache/package.json'"
	got := sanitizeProbeDiagnostic(input)
	if strings.Contains(got, "Private") || strings.Contains(got, "User") || strings.Contains(got, "cache/package.json") {
		t.Fatalf("private path leaked: %s", got)
	}
	if !strings.Contains(got, "npm error code ENOENT") || !strings.Contains(got, "npm error syscall open") {
		t.Fatalf("safe diagnostic details missing: %s", got)
	}
}
