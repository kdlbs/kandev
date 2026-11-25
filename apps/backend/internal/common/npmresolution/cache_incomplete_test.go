package npmresolution

import (
	"path/filepath"
	"testing"
)

func TestMatchesMissingExecutionRootManifestRequiresExactRawENOENT(t *testing.T) {
	manifest := filepath.Join(t.TempDir(), "_npx", "0123456789abcdef", "package.json")
	diagnostic := "npm error code ENOENT\nnpm error syscall open\nnpm error path " + manifest
	if !MatchesMissingExecutionRootManifest(diagnostic, manifest) {
		t.Fatal("matching raw npm ENOENT open path should match the expected execution root manifest")
	}
	if IsTransientManagedStartupCode("ENOENT") {
		t.Fatal("ENOENT must stay outside the generic startup retry allowlist")
	}

	for name, stderr := range map[string]string{
		"different path":       "npm error code ENOENT\nnpm error syscall open\nnpm error path " + manifest + ".elsewhere",
		"missing code":         "npm error syscall open\nnpm error path " + manifest,
		"missing open syscall": "npm error code ENOENT\nnpm error path " + manifest,
		"wrong syscall":        "npm error code ENOENT\nnpm error syscall stat\nnpm error path " + manifest,
		"permanent code":       "npm error code ENOENT\nnpm error code EACCES\nnpm error syscall open\nnpm error path " + manifest,
		"wrong root shape":     "npm error code ENOENT\nnpm error syscall open\nnpm error path " + filepath.Join(t.TempDir(), "node_modules", "package.json"),
	} {
		t.Run(name, func(t *testing.T) {
			if MatchesMissingExecutionRootManifest(stderr, manifest) {
				t.Fatal("incomplete npm evidence should not match")
			}
		})
	}
}
