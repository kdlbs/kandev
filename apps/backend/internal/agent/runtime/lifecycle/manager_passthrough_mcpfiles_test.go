package lifecycle

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
)

func TestSafePassthroughMCPConfigName(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{in: "", want: "session"},
		{in: "abcXYZ019", want: "abcXYZ019"},
		{in: "sess-1_2.json", want: "sess-1_2.json"},
		// Dots are legal in a filename; it is the separator that is neutralised.
		{in: "../../etc/passwd", want: ".._.._etc_passwd"},
		{in: "a b\tc", want: "a_b_c"},
		{in: "sess/../id", want: "sess_.._id"},
	} {
		require.Equal(t, tc.want, safePassthroughMCPConfigName(tc.in),
			"safePassthroughMCPConfigName(%q)", tc.in)
	}
}

func TestPassthroughMCPPathsFallBackToTempWhenNoDataDir(t *testing.T) {
	mgr := newTestManager(t)
	workspace := t.TempDir()

	paths := mgr.passthroughMCPPaths(&AgentExecution{
		ID: "exec-1", SessionID: "session-1", WorkspacePath: workspace,
	})

	require.Equal(t, filepath.Join(os.TempDir(), "kandev", "passthrough-mcp", "session-1.json"),
		paths.TempConfigPath, "an unset data dir falls back to the OS temp dir")
	require.Equal(t, workspace, paths.WorkspaceDir)
}

func TestPassthroughMCPPathsUsesExecutionIDWhenSessionMissing(t *testing.T) {
	mgr := newTestManager(t)
	mgr.dataDir = t.TempDir()

	paths := mgr.passthroughMCPPaths(&AgentExecution{ID: "exec-1"})

	require.Equal(t, filepath.Join(mgr.dataDir, "passthrough-mcp", "exec-1.json"), paths.TempConfigPath,
		"a session-less execution names its config after the execution")
}

func TestPassthroughMCPConfigPortPrefersLiveStandalonePort(t *testing.T) {
	require.Zero(t, passthroughMCPConfigPort(nil))

	exec := &AgentExecution{}
	exec.setMetadataValue("standalone_port", 45678)
	require.Equal(t, 45678, passthroughMCPConfigPort(exec),
		"a recovered execution reads the port back out of metadata")

	exec.standalonePort = 45679
	require.Equal(t, 45679, passthroughMCPConfigPort(exec),
		"the live port wins over the persisted metadata copy")
}

// TestMaterializePassthroughFileOverwritesKandevOwnedTempFile pins the
// relaunch path: an existing kandev-owned config (no MergeKey) is overwritten
// and stays tracked for cleanup.
func TestMaterializePassthroughFileOverwritesKandevOwnedTempFile(t *testing.T) {
	mgr := newTestManager(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"stale":true}`), 0o600))

	owned, err := mgr.materializePassthroughFile(&AgentExecution{ID: "exec-1"}, mcpconfig.PassthroughConfigFile{
		Path:    path,
		Content: []byte(`{"mcpServers":{"kandev":{}}}`),
	})

	require.NoError(t, err)
	require.True(t, owned, "a kandev-owned temp config stays tracked so teardown removes it")
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.JSONEq(t, `{"mcpServers":{"kandev":{}}}`, string(content))
}

// TestMaterializePassthroughFileSkipsPathEscapingWorkspace pins the symlink
// containment guard: a config path that resolves outside the worktree is
// skipped, not written.
func TestMaterializePassthroughFileSkipsPathEscapingWorkspace(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation is not reliably available on Windows CI")
	}
	mgr := newTestManager(t)
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	outside := filepath.Join(root, "outside")
	require.NoError(t, os.MkdirAll(workspace, 0o700))
	require.NoError(t, os.MkdirAll(outside, 0o700))
	require.NoError(t, os.Symlink(outside, filepath.Join(workspace, ".cursor")))

	target := filepath.Join(workspace, ".cursor", "mcp.json")
	owned, err := mgr.materializePassthroughFile(
		&AgentExecution{ID: "exec-1", WorkspacePath: workspace},
		mcpconfig.PassthroughConfigFile{Path: target, Content: []byte(`{"mcpServers":{}}`), MergeKey: "mcpServers"},
	)

	require.NoError(t, err)
	require.False(t, owned)
	_, statErr := os.Stat(filepath.Join(outside, "mcp.json"))
	require.True(t, os.IsNotExist(statErr),
		"a symlinked parent escaping the worktree must not receive kandev's config")
}

func TestMaterializePassthroughFileSurfacesUnreadableParent(t *testing.T) {
	mgr := newTestManager(t)
	dir := t.TempDir()
	// A regular file where a directory is expected: Lstat on the child fails
	// with ENOTDIR, which is neither "exists" nor "not exist".
	blocker := filepath.Join(dir, "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o600))

	_, err := mgr.materializePassthroughFile(&AgentExecution{ID: "exec-1"}, mcpconfig.PassthroughConfigFile{
		Path:    filepath.Join(blocker, "config.json"),
		Content: []byte(`{}`),
	})

	require.Error(t, err, "an unexpected lstat failure must be surfaced, not silently skipped")
}

func TestWritePassthroughMCPFilesSkipsEmptyPaths(t *testing.T) {
	mgr := newTestManager(t)
	execution := &AgentExecution{ID: "exec-1"}

	require.NoError(t, mgr.writePassthroughMCPFiles(execution, []mcpconfig.PassthroughConfigFile{
		{Path: "", Content: []byte(`{}`)},
	}))

	require.Empty(t, getPassthroughMCPFiles(execution),
		"a strategy entry with no path is skipped and never tracked")
}

// TestWriteFileNoFollowLeavesConcurrentlyCreatedFileAlone pins the O_EXCL
// contract: a file that appeared between the lstat and the create is left
// untouched and NOT tracked for cleanup.
func TestWriteFileNoFollowLeavesConcurrentlyCreatedFileAlone(t *testing.T) {
	mgr := newTestManager(t)
	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(path, []byte("user content"), 0o600))

	owned, err := mgr.writeFileNoFollow(path, []byte("kandev content"))

	require.NoError(t, err)
	require.False(t, owned, "kandev does not own a file it did not create")
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "user content", string(content), "the pre-existing file must not be clobbered")
}

func TestWriteFileNoFollowSurfacesCreateFailure(t *testing.T) {
	mgr := newTestManager(t)
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o600))

	owned, err := mgr.writeFileNoFollow(filepath.Join(blocker, "config.json"), []byte("{}"))

	require.ErrorContains(t, err, "create passthrough MCP config")
	require.False(t, owned)
}

func TestMergePassthroughConfigLeavesUnreadableFileUntouched(t *testing.T) {
	mgr := newTestManager(t)
	// A directory cannot be read as a file: os.ReadFile fails, and the merge
	// must degrade to a warning rather than clobbering anything.
	dir := t.TempDir()

	require.NoError(t, mgr.mergePassthroughConfig(mcpconfig.PassthroughConfigFile{
		Path:     dir,
		Content:  []byte(`{"mcpServers":{"kandev":{}}}`),
		MergeKey: "mcpServers",
	}))
}

func TestMergePassthroughConfigLeavesMalformedFileUntouched(t *testing.T) {
	mgr := newTestManager(t)
	path := filepath.Join(t.TempDir(), "mcp.json")
	require.NoError(t, os.WriteFile(path, []byte("not json at all"), 0o600))

	require.NoError(t, mgr.mergePassthroughConfig(mcpconfig.PassthroughConfigFile{
		Path:     path,
		Content:  []byte(`{"mcpServers":{"kandev":{}}}`),
		MergeKey: "mcpServers",
	}), "a malformed user config is a warning, not a launch failure")

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "not json at all", string(content),
		"a config kandev cannot parse must never be overwritten")
}

func TestMergePassthroughConfigPreservesUserEntries(t *testing.T) {
	mgr := newTestManager(t)
	path := filepath.Join(t.TempDir(), "mcp.json")
	require.NoError(t, os.WriteFile(path,
		[]byte(`{"otherKey":"keep me","mcpServers":{"user-server":{"url":"http://user"}}}`), 0o600))

	require.NoError(t, mgr.mergePassthroughConfig(mcpconfig.PassthroughConfigFile{
		Path:     path,
		Content:  []byte(`{"mcpServers":{"kandev":{"url":"http://localhost:1/mcp"}}}`),
		MergeKey: "mcpServers",
	}))

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.JSONEq(t,
		`{"otherKey":"keep me","mcpServers":{"user-server":{"url":"http://user"},"kandev":{"url":"http://localhost:1/mcp"}}}`,
		string(content))
}

func TestWorkspacePathEscapesIgnoresPathsOutsideWorkspace(t *testing.T) {
	workspace := t.TempDir()

	escapes, err := workspacePathEscapes("", filepath.Join(workspace, "anything.json"))
	require.NoError(t, err)
	require.False(t, escapes, "an empty workspace dir disables the check")

	escapes, err = workspacePathEscapes(workspace, filepath.Join(t.TempDir(), "kandev-temp.json"))
	require.NoError(t, err)
	require.False(t, escapes, "kandev's own temp configs are not workspace-relative and are exempt")

	escapes, err = workspacePathEscapes(workspace, filepath.Join(workspace, "nested", "deep", "mcp.json"))
	require.NoError(t, err)
	require.False(t, escapes, "not-yet-created parents resolve to the workspace itself")
}

func TestWorkspacePathEscapesSurfacesUnresolvableWorkspace(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "never-created")

	_, err := workspacePathEscapes(missing, filepath.Join(missing, "mcp.json"))

	require.ErrorContains(t, err, "resolve workspace dir")
}

func TestCleanupPassthroughMCPConfigToleratesRemovalFailure(t *testing.T) {
	mgr := newTestManager(t)
	dir := t.TempDir()
	nonEmpty := filepath.Join(dir, "not-a-file")
	require.NoError(t, os.MkdirAll(filepath.Join(nonEmpty, "child"), 0o700))
	removable := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(removable, []byte(`{}`), 0o600))

	execution := &AgentExecution{ID: "exec-1"}
	setPassthroughMCPFiles(execution, []string{nonEmpty, removable, filepath.Join(dir, "already-gone.json")})
	setPassthroughMCPEnv(execution, map[string]string{"OPENCODE_CONFIG": removable})

	mgr.cleanupPassthroughMCPConfig(execution)

	_, err := os.Stat(removable)
	require.True(t, os.IsNotExist(err), "a removable tracked file must still be deleted")
	require.Empty(t, getPassthroughMCPFiles(execution), "the tracking keys are always cleared")
	require.Empty(t, getPassthroughMCPEnv(execution))
}

func TestAppendUniqueKeepsFirstOccurrence(t *testing.T) {
	list := appendUnique(nil, "a")
	list = appendUnique(list, "b")
	list = appendUnique(list, "a")

	require.Equal(t, []string{"a", "b"}, list)
}

func TestGetPassthroughMCPHelpersHandleNilExecution(t *testing.T) {
	require.Nil(t, getPassthroughMCPFiles(nil))
	require.Nil(t, getPassthroughMCPEnv(nil))
}

func TestSetPassthroughMCPEnvIgnoresEmptyMap(t *testing.T) {
	execution := &AgentExecution{ID: "exec-1"}

	setPassthroughMCPEnv(execution, nil)
	setPassthroughMCPEnv(execution, map[string]string{})

	_, exists := execution.metadataValue(metadataKeyPassthroughMCPEnv)
	require.False(t, exists, "an empty strategy env must not write a metadata key at all")
}
