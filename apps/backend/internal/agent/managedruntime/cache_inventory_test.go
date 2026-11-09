package managedruntime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManagedRuntimeCacheEvidence(t *testing.T) {
	cacheRoot := t.TempDir()
	packageSpec := "@scope/managed-acp@1.2.3"
	executionRoot := writeNpxInventoryFixture(t, cacheRoot, packageSpec,
		`{"name":"@scope/managed-acp","version":"1.2.3","bin":{"managed-acp":"bin/cli.js"}}`)
	stderr := missingRootManifestDiagnostic(executionRoot)
	evidence, ok := VerifyNpxRootManifestEvidence(cacheRoot, packageSpec, stderr)
	if !ok {
		t.Fatal("matching npm root-manifest ENOENT and exact installed package should establish cache evidence")
	}
	if evidence.PackageSpec != packageSpec || evidence.Condition != CacheConditionRootManifestMissing {
		t.Fatalf("cache evidence = %#v, want exact package and root_manifest_missing", evidence)
	}
}

func TestManagedRuntimeCacheEvidenceRejectsAmbiguousPackageBin(t *testing.T) {
	cacheRoot := t.TempDir()
	packageSpec := "@scope/managed-acp@1.2.3"
	manifest := `{"name":"@scope/managed-acp","version":"1.2.3","bin":{"first":"bin/first.js","second":"bin/second.js"}}`
	executionRoot := writeNpxInventoryFixture(t, cacheRoot, packageSpec, manifest)
	stderr := missingRootManifestDiagnostic(executionRoot)
	if _, ok := VerifyNpxRootManifestEvidence(cacheRoot, packageSpec, stderr); ok {
		t.Fatal("ambiguous package bin declarations must not authorize cache preparation")
	}
}

func TestManagedRuntimeCacheEvidenceRejectsUnmatchedInventory(t *testing.T) {
	const packageSpec = "@scope/managed-acp@1.2.3"
	for name, manifest := range map[string]string{
		"wrong package":              `{"name":"@scope/other","version":"1.2.3","bin":"bin/cli.js"}`,
		"wrong version":              `{"name":"@scope/managed-acp","version":"1.2.4","bin":"bin/cli.js"}`,
		"malformed package manifest": `{"name":`,
		"trailing garbage":           `{"name":"@scope/managed-acp","version":"1.2.3","bin":"bin/cli.js"}garbage`,
		"second JSON value":          `{"name":"@scope/managed-acp","version":"1.2.3","bin":"bin/cli.js"}{}`,
		"invalid bin path":           `{"name":"@scope/managed-acp","version":"1.2.3","bin":"../escape"}`,
	} {
		t.Run(name, func(t *testing.T) {
			cacheRoot := t.TempDir()
			executionRoot := writeNpxInventoryFixture(t, cacheRoot, packageSpec, manifest)
			if _, ok := VerifyNpxRootManifestEvidence(cacheRoot, packageSpec, missingRootManifestDiagnostic(executionRoot)); ok {
				t.Fatal("unmatched installed package inventory should not establish cache evidence")
			}
		})
	}

	t.Run("root manifest present", func(t *testing.T) {
		cacheRoot := t.TempDir()
		executionRoot := writeNpxInventoryFixture(t, cacheRoot, packageSpec,
			`{"name":"@scope/managed-acp","version":"1.2.3","bin":"bin/cli.js"}`)
		if err := os.WriteFile(filepath.Join(executionRoot, "package.json"), []byte(`{"private":true}`), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, ok := VerifyNpxRootManifestEvidence(cacheRoot, packageSpec, missingRootManifestDiagnostic(executionRoot)); ok {
			t.Fatal("present execution root manifest should not establish missing-manifest evidence")
		}
	})

	t.Run("symlinked execution root", func(t *testing.T) {
		cacheRoot := t.TempDir()
		keyPath := filepath.Join(cacheRoot, "_npx", NpxExecutionCacheKey(packageSpec))
		if err := os.MkdirAll(filepath.Dir(keyPath), 0o700); err != nil {
			t.Fatal(err)
		}
		outside := t.TempDir()
		if err := os.Symlink(outside, keyPath); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		if _, ok := VerifyNpxRootManifestEvidence(cacheRoot, packageSpec, missingRootManifestDiagnostic(keyPath)); ok {
			t.Fatal("symlinked execution root should not establish cache evidence")
		}
	})
}

func TestManagedRuntimeCacheNormalizesRelativeBinTargets(t *testing.T) {
	for _, target := range []string{"bin/cli.js", "./bin/cli.js", "./bin/opencode.exe"} {
		t.Run(target, func(t *testing.T) {
			_, got, valid := selectedNpmPackageBin("managed-acp", []byte(`{"managed-acp":"`+target+`"}`))
			if !valid || got != filepath.ToSlash(filepath.Clean(target)) {
				t.Fatalf("selected bin = %q, %v for %q", got, valid, target)
			}
		})
	}
	for _, target := range []string{"../escape", "bin/../escape", "/absolute", "./../escape", "C:/escape", "bin//cli.js"} {
		if _, _, valid := selectedNpmPackageBin("managed-acp", []byte(`{"managed-acp":"`+target+`"}`)); valid {
			t.Fatalf("accepted unsafe bin target %q", target)
		}
	}
}

func writeNpxInventoryFixture(t *testing.T, cacheRoot, packageSpec, manifest string) string {
	t.Helper()
	executionRoot := filepath.Join(cacheRoot, "_npx", NpxExecutionCacheKey(packageSpec))
	packageManifest := filepath.Join(executionRoot, "node_modules", "@scope", "managed-acp", "package.json")
	if err := os.MkdirAll(filepath.Dir(packageManifest), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(packageManifest, []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	return executionRoot
}

func missingRootManifestDiagnostic(executionRoot string) string {
	return "npm error code ENOENT\nnpm error syscall open\nnpm error path " + filepath.Join(executionRoot, "package.json")
}
