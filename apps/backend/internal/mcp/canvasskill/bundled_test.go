package canvasskill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/stretchr/testify/require"
)

func TestEnsureMaterialized_WritesKandevOwnedCanvasSkill(t *testing.T) {
	home := t.TempDir()

	require.NoError(t, EnsureMaterialized(home))

	root := filepath.Join(home, "system-skills", Slug)
	skill, err := os.ReadFile(filepath.Join(root, "SKILL.md"))
	require.NoError(t, err)
	require.Contains(t, string(skill), "./_kandev/v1")
	require.Contains(t, string(skill), "browser storage")
	require.FileExists(t, filepath.Join(root, "references", "browser-api.md"))
	require.NoDirExists(t, filepath.Join(home, "skills", Slug))
}

func TestReadMaterialized_UsesAllowlistedInventory(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, EnsureMaterialized(home))

	content, err := ReadMaterialized(home, "SKILL.md")
	require.NoError(t, err)
	require.NotEmpty(t, content)
	require.Equal(t, Version, MaterializedVersion(home))
	require.Equal(t, "2", MaterializedVersion(home))

	for _, path := range []string{"../outside.txt", filepath.Join(string(os.PathSeparator), "etc", "passwd"), "missing.md"} {
		_, err := ReadMaterialized(home, path)
		require.Error(t, err, "path %q should be rejected", path)
	}
}

func TestReadMaterialized_RejectsSymlinkedSupportFile(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, EnsureMaterialized(home))
	root := filepath.Join(home, "system-skills", Slug)
	outside := filepath.Join(home, "outside.txt")
	require.NoError(t, os.WriteFile(outside, []byte("secret"), 0o600))
	link := filepath.Join(root, "references", "outside.md")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}

	_, err := ReadMaterialized(home, "references/outside.md")
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "allowlisted") || strings.Contains(err.Error(), "symlink"))
}

func TestInventory_IsStableAndContainsSupportingReferences(t *testing.T) {
	inventory := Inventory()
	require.Equal(t, []string{
		"SKILL.md",
		"references/browser-api.md",
		"references/data-and-state.md",
		"references/events-and-recovery.md",
		"references/manifest.md",
		"references/security.md",
		"references/ui-patterns.md",
		"scaffold/appearance.js",
		"scaffold/index.html",
		"scaffold/script.js",
		"scaffold/styles.css",
	}, inventory)
	require.NotEmpty(t, Version)
}

func TestCoreInventory_IsOneReadAuthoringBundle(t *testing.T) {
	require.Equal(t, []string{
		"SKILL.md",
		"scaffold/index.html",
		"scaffold/appearance.js",
		"scaffold/script.js",
		"scaffold/styles.css",
	}, CoreInventory())
}

func TestScaffoldInventory_IsTheGeneratedSourceContract(t *testing.T) {
	require.Equal(t, []string{
		"manifest.yaml",
		"index.html",
		"appearance.js",
		"script.js",
		"styles.css",
	}, ScaffoldInventory())

	files, err := ScaffoldTemplateFiles()
	require.NoError(t, err)
	require.Equal(t, []string{
		"index.html",
		"appearance.js",
		"script.js",
		"styles.css",
	}, filePaths(files))
	for _, file := range files {
		require.NotEmpty(t, file.Content, file.Path)
	}
}

func filePaths(files []BundleFile) []string {
	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, file.Path)
	}
	return paths
}

func TestAuthoringReferencesMatchValidatedManifestAndProtocol(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, EnsureMaterialized(home))

	manifestText, err := ReadMaterialized(home, "references/manifest.md")
	require.NoError(t, err)
	manifestSource := string(manifestText)
	start := strings.Index(manifestSource, "```yaml\n")
	require.GreaterOrEqual(t, start, 0, "manifest reference must contain a YAML example")
	start += len("```yaml\n")
	end := strings.Index(manifestSource[start:], "\n```")
	require.GreaterOrEqual(t, end, 0, "manifest YAML example must be closed")
	parsed, err := manifest.Parse([]byte(manifestSource[start : start+end]))
	require.NoError(t, err)
	require.NoError(t, parsed.Validate())
	require.Equal(t, manifest.CurrentAPIVersion, parsed.APIVersion)
	require.Len(t, parsed.UI.WebApps, 1)
	require.Equal(t, "ui/index.html", parsed.UI.WebApps[0].Entry)

	browserAPI, err := ReadMaterialized(home, "references/browser-api.md")
	require.NoError(t, err)
	browserSource := string(browserAPI)
	for _, route := range []string{
		"GET ./_kandev/v1/context",
		"GET | `./_kandev/v1/data/tasks`",
		"GET | `./_kandev/v1/data/tasks/{task_id}`",
		"PATCH | `./_kandev/v1/data/tasks/{task_id}`",
		"POST | `./_kandev/v1/data/tasks/{task_id}/messages`",
		"GET | `./_kandev/v1/data/workflows`",
		"GET | `./_kandev/v1/data/workflows/{workflow_id}/steps`",
		"GET | `./_kandev/v1/state`",
		"GET | `./_kandev/v1/state/{key}`",
		"PUT | `./_kandev/v1/state/{key}`",
		"DELETE | `./_kandev/v1/state/{key}`",
		"GET ./_kandev/v1/events",
		"POST ./_kandev/v1/actions/{key}",
	} {
		require.Contains(t, browserSource, route, "browser API reference is missing %q", route)
	}
	for _, contract := range []string{
		"If-Match",
		"plugin_state_conflict",
		"runtime.resync_required",
		"workflow_step_id",
		"Last-Event-ID",
		"permission",
	} {
		require.Contains(t, browserSource, contract)
	}
}
