package compatibility

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func frontendFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	pkg := filepath.Join(root, "node_modules", "@kdlbs", "plugin-sdk")
	writeJSON(t, filepath.Join(root, "package.json"), map[string]any{"dependencies": map[string]string{"@kdlbs/plugin-sdk": "0.1.0"}})
	writeJSON(t, filepath.Join(pkg, "package.json"), map[string]string{"name": "@kdlbs/plugin-sdk", "version": "0.1.0"})
	writeJSON(t, filepath.Join(pkg, "sdk-release.json"), releasedRecord())
	writeNPMLock(t, root, "0.1.0", "https://registry.npmjs.org/@kdlbs/plugin-sdk/-/plugin-sdk-0.1.0.tgz", false)
	return root, pkg
}
func writeNPMLock(t *testing.T, root, version, resolved string, link bool) {
	t.Helper()
	writeJSON(t, filepath.Join(root, "package-lock.json"), map[string]any{"lockfileVersion": 3, "packages": map[string]any{
		"":                               map[string]any{"dependencies": map[string]string{"@kdlbs/plugin-sdk": "0.1.0"}},
		"node_modules/@kdlbs/plugin-sdk": map[string]any{"version": version, "resolved": resolved, "integrity": "sha512-" + strings.Repeat("A", 86) + "==", "link": link},
	}})
}

// @covers AC-PLUGINS-SDK-DISTRIBUTION-002.2
func TestInstalledNPMLockEvidence(t *testing.T) {
	tests := []struct {
		name, want string
		mutate     func(t *testing.T, root, pkg string)
	}{
		{"registry install", "", func(*testing.T, string, string) {}},
		{"missing lock", "lock", func(t *testing.T, r, p string) { require.NoError(t, os.Remove(filepath.Join(r, "package-lock.json"))) }},
		{"unsupported lock", "lockfileVersion", func(t *testing.T, r, p string) {
			writeJSON(t, filepath.Join(r, "package-lock.json"), map[string]int{"lockfileVersion": 1})
		}},
		{"file dependency", "registry", func(t *testing.T, r, p string) { writeNPMLock(t, r, "0.1.0", "file:../sdk", false) }},
		{"workspace link", "registry", func(t *testing.T, r, p string) { writeNPMLock(t, r, "0.1.0", "../sdk", true) }},
		{"installed mismatch", "version", func(t *testing.T, r, p string) {
			writeNPMLock(t, r, "0.2.0", "https://registry.npmjs.org/sdk.tgz", false)
		}},
		{"metadata mismatch", "version", func(t *testing.T, r, p string) {
			record := releasedRecord()
			record.SDKVersion = "0.2.0"
			writeJSON(t, filepath.Join(p, "sdk-release.json"), record)
		}},
		{"pending record", "host_release", func(t *testing.T, r, p string) {
			record := releasedRecord()
			record.HostRelease = ""
			writeJSON(t, filepath.Join(p, "sdk-release.json"), record)
		}},
		{"manifest substitution", "registry", func(t *testing.T, r, p string) {
			writeJSON(t, filepath.Join(r, "package.json"), map[string]any{"dependencies": map[string]string{"@kdlbs/plugin-sdk": "file:../sdk"}})
		}},
		{"outside installed tree", "installed", func(t *testing.T, r, p string) {
			require.NoError(t, os.Rename(p, p+"-outside"))
			require.NoError(t, os.Symlink(p+"-outside", p))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, pkg := frontendFixture(t)
			tt.mutate(t, root, pkg)
			before, err := os.ReadFile(filepath.Join(root, "package.json"))
			require.NoError(t, err)
			got, err := ResolveFrontend(root, pkg)
			if tt.want != "" {
				require.ErrorContains(t, err, tt.want)
			} else {
				require.NoError(t, err)
				require.Equal(t, releasedRecord(), got)
			}
			after, readErr := os.ReadFile(filepath.Join(root, "package.json"))
			require.NoError(t, readErr)
			require.Equal(t, before, after)
		})
	}
}
func pnpmFrontendFixture(t *testing.T) (string, string) {
	t.Helper()
	root, pkg := frontendFixture(t)
	require.NoError(t, os.Remove(filepath.Join(root, "package-lock.json")))
	store := filepath.Join(root, "node_modules", ".pnpm", "@kdlbs+plugin-sdk@0.1.0", "node_modules", "@kdlbs", "plugin-sdk")
	require.NoError(t, os.MkdirAll(filepath.Dir(store), 0755))
	require.NoError(t, os.Rename(pkg, store))
	require.NoError(t, os.Symlink(store, pkg))
	lock := `lockfileVersion: '9.0'
importers:
  .:
    dependencies:
      '@kdlbs/plugin-sdk':
        specifier: 0.1.0
        version: 0.1.0
packages:
  '@kdlbs/plugin-sdk@0.1.0':
    resolution:
      integrity: sha512-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==
snapshots:
  '@kdlbs/plugin-sdk@0.1.0': {}
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "pnpm-lock.yaml"), []byte(lock), 0600))
	return root, pkg
}

func TestPNPMManagedStoreIsNotALocalSubstitution(t *testing.T) {
	root, pkg := pnpmFrontendFixture(t)
	got, err := ResolveFrontend(root, pkg)
	require.NoError(t, err)
	require.Equal(t, releasedRecord(), got)
	require.NoError(t, os.WriteFile(filepath.Join(root, "pnpm-lock.yaml"), []byte("lockfileVersion: '6.0'\n"), 0600))
	_, err = ResolveFrontend(root, pkg)
	require.ErrorContains(t, err, "lockfileVersion")
}
func TestGoResolutionRejectsReplacementsAndDoesNotChangeConsumer(t *testing.T) {
	root := t.TempDir()
	sdk := filepath.Join(root, "sdk")
	require.NoError(t, os.Mkdir(sdk, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(sdk, "go.mod"), []byte("module github.com/kdlbs/kandev/pluginsdk\n\ngo 1.26.0\n"), 0600))
	writeJSON(t, filepath.Join(sdk, "sdk-release.json"), releasedRecord())
	mod := []byte("module example.com/consumer\n\ngo 1.26.0\n\nrequire github.com/kdlbs/kandev/pluginsdk v0.1.0\nreplace github.com/kdlbs/kandev/pluginsdk => ./sdk\n")
	path := filepath.Join(root, "go.mod")
	require.NoError(t, os.WriteFile(path, mod, 0600))
	t.Setenv("GOWORK", filepath.Join(root, "nonexistent.work"))
	_, err := ResolveGo(root)
	require.ErrorContains(t, err, "replacement")
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, mod, after)
}

func TestRegistryIntegrityMustBeARealSHA512Digest(t *testing.T) {
	require.ErrorContains(t, registryResolution("https://registry.npmjs.org/sdk.tgz", "sha512-not-a-digest"), "integrity")
}
