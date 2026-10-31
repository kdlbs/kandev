package launchwrapper

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func allocFixtureDir(t *testing.T, mode os.FileMode) string {
	t.Helper()
	if runtime.GOOS == windowsOS {
		t.Skip("POSIX ownership model")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	dir := filepath.Join(root, "alloc")
	require.NoError(t, os.Mkdir(dir, 0o700))
	require.NoError(t, os.Chmod(dir, mode))
	return dir
}

// @covers AC-AGENTS-NODEWRAP-006.1
func TestValidateTokensAcceptsAllocDirOnlyAsWholeArgument(t *testing.T) {
	require.NoError(t, ValidateTokens([]string{"/opt/wrap", "--allocation", AllocDir, "--"}))
	require.NoError(t, ValidateTokens([]string{"/opt/wrap", "{{{kandev.alloc_dir}}}"}), "escaped reference stays literal")
	require.ErrorIs(t, ValidateTokens([]string{AllocDir, "--"}), ErrInvalid, "never the executable")
	for _, mangled := range []string{
		"a" + AllocDir, AllocDir + "/x", "{{kandev.alloc_dir}", "{{kandev.alloc_dir}}}",
		"{{kandev.alloc_dirs}}", "{{kandev.alloc_dir }}", "{{kandev.allocdir}}",
	} {
		require.ErrorIs(t, ValidateTokens([]string{"/opt/wrap", mangled}), ErrInvalid, mangled)
	}
}

// @covers AC-AGENTS-NODEWRAP-006.1
func TestDecodeNodeAcceptsAllocDirAndProfileDoesNot(t *testing.T) {
	spec, err := DecodeNode(nodeJSON(t, "/opt/wrap", "--workspace", WorkDir, "--allocation", AllocDir, "--"))
	require.NoError(t, err)
	require.NoError(t, spec.ValidateNode())

	profile := Spec{Version: Version, Source: SourceProfile, Argv: []string{"prof", AllocDir}}
	require.ErrorIs(t, profile.Validate(), ErrInvalid, "a profile wrapper cannot reference the allocation directory")
	_, err = Prepare(profile, []string{"/bin/echo"}, Context{WorkDir: t.TempDir()})
	require.ErrorIs(t, err, ErrInvalid)
	var decoded Spec
	require.ErrorIs(t, json.Unmarshal([]byte(`{"version":1,"source":"profile","argv":["prof","{{kandev.alloc_dir}}"]}`), &decoded), ErrInvalid)
}

// @covers AC-AGENTS-NODEWRAP-006.2
func TestResolveAllocDir(t *testing.T) {
	dir := allocFixtureDir(t, 0o755)
	got, err := ResolveAllocDir(dir)
	require.NoError(t, err)
	require.Equal(t, dir, got)

	link := filepath.Join(filepath.Dir(dir), "link")
	require.NoError(t, os.Symlink(dir, link))
	got, err = ResolveAllocDir(link)
	require.NoError(t, err)
	require.Equal(t, dir, got, "symlinks are resolved")
}

// @covers AC-AGENTS-NODEWRAP-006.2 AC-AGENTS-NODEWRAP-006.3
func TestResolveAllocDirRefusals(t *testing.T) {
	dir := allocFixtureDir(t, 0o755)
	root := filepath.Dir(dir)
	file := filepath.Join(root, "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))
	unsafe := filepath.Join(root, "unsafe")
	require.NoError(t, os.Mkdir(unsafe, 0o700))
	require.NoError(t, os.Chmod(unsafe, 0o777))
	escape := filepath.Join(root, "escape")
	require.NoError(t, os.Symlink(unsafe, escape))

	for name, path := range map[string]string{
		"empty":                    "",
		"relative":                 "alloc",
		"dot relative":             "./alloc",
		"missing":                  filepath.Join(root, "absent"),
		"regular file":             file,
		"symlink to a file":        mustSymlink(t, file, filepath.Join(root, "filelink")),
		"symlink to an unsafe dir": escape,
		"dangling symlink":         mustSymlink(t, filepath.Join(root, "gone"), filepath.Join(root, "dangling")),
	} {
		_, err := ResolveAllocDir(path)
		require.ErrorIs(t, err, ErrAllocDir, name)
		if path != "" {
			require.NotContains(t, err.Error(), root, name+": no path in the message")
		}
	}
	for _, mode := range []os.FileMode{0o775, 0o757, 0o777, 0o720, 0o702} {
		require.NoError(t, os.Chmod(dir, mode))
		_, err := ResolveAllocDir(dir)
		require.ErrorIs(t, err, ErrAllocDir, mode.String())
	}
}

func mustSymlink(t *testing.T, target, link string) string {
	t.Helper()
	require.NoError(t, os.Symlink(target, link))
	return link
}

// @covers AC-AGENTS-NODEWRAP-006.2
func TestResolveAllocDirRefusesOtherOwner(t *testing.T) {
	dir := allocFixtureDir(t, 0o755)
	previous := allocDirOwnerUID
	t.Cleanup(func() { allocDirOwnerUID = previous })
	allocDirOwnerUID = func() int { return os.Geteuid() + 1 }
	_, err := ResolveAllocDir(dir)
	require.ErrorIs(t, err, ErrAllocDir)
	allocDirOwnerUID = previous
	_, err = ResolveAllocDir(dir)
	require.NoError(t, err)
}

type allocLaunch struct {
	node  Spec
	child string
	ctx   Context
}

func newAllocLaunch(t *testing.T, argv ...string) allocLaunch {
	t.Helper()
	if runtime.GOOS == windowsOS {
		t.Skip("POSIX fixture")
	}
	root := t.TempDir()
	wrap := writeExecutable(t, root, "node-wrap", "#!/bin/sh\n")
	child := writeExecutable(t, root, "child", "#!/bin/sh\n")
	return allocLaunch{
		node:  Spec{Version: Version, Source: SourceNode, Argv: append([]string{wrap}, argv...)},
		child: child,
		ctx:   Context{WorkDir: root, Env: []string{"PATH=" + root}},
	}
}

// @covers AC-AGENTS-NODEWRAP-006.3
func TestNodePrepareResolvesAllocDirFromSuppliedValue(t *testing.T) {
	l := newAllocLaunch(t, "--workspace", WorkDir, "--allocation", AllocDir, "--allocation", "{{{kandev.alloc_dir}}}", "--")
	alloc := allocFixtureDir(t, 0o755)
	link := filepath.Join(filepath.Dir(alloc), "link")
	require.NoError(t, os.Symlink(alloc, link))

	argv, err := Node{Spec: l.node, AllocDir: link}.Prepare(Spec{}, []string{l.child}, l.ctx)
	require.NoError(t, err)
	require.Equal(t, []string{l.node.Argv[0], "--workspace", l.ctx.WorkDir, "--allocation", alloc, "--allocation", AllocDir, "--", l.child}, argv)
	require.NotContains(t, l.ctx.WorkDir, alloc)
}

// @covers AC-AGENTS-NODEWRAP-006.4
func TestNodePrepareRefUsesAllocDirTokenWithoutUsableValue(t *testing.T) {
	l := newAllocLaunch(t, "--allocation", AllocDir, "--")
	alloc := allocFixtureDir(t, 0o755)
	t.Setenv("NOMAD_ALLOC_DIR", alloc)
	t.Setenv("KANDEV_ALLOC_DIR", alloc)
	t.Setenv("ALLOC_DIR", alloc)
	l.ctx.Env = append(l.ctx.Env, "NOMAD_ALLOC_DIR="+alloc, "KANDEV_ALLOC_DIR="+alloc)

	for name, value := range map[string]string{
		"not supplied": "", "relative": "alloc", "missing": filepath.Join(alloc, "absent"),
	} {
		argv, err := Node{Spec: l.node, AllocDir: value}.Prepare(Spec{}, []string{l.child}, l.ctx)
		require.ErrorIs(t, err, ErrAllocDir, name)
		require.Nil(t, argv, name)
	}
	require.NoError(t, os.Chmod(alloc, 0o777))
	_, err := Node{Spec: l.node, AllocDir: alloc}.Prepare(Spec{}, []string{l.child}, l.ctx)
	require.ErrorIs(t, err, ErrAllocDir, "checked at every launch, not only at startup")
}

// @covers AC-AGENTS-NODEWRAP-006.4
func TestNodePrepareWithoutTokenIgnoresAllocDir(t *testing.T) {
	l := newAllocLaunch(t, "--workspace", WorkDir, "--")
	want, err := Node{Spec: l.node}.Prepare(Spec{}, []string{l.child}, l.ctx)
	require.NoError(t, err)
	got, err := Node{Spec: l.node, AllocDir: "relative-and-unused"}.Prepare(Spec{}, []string{l.child}, l.ctx)
	require.NoError(t, err)
	require.Equal(t, want, got)
}

// @covers AC-AGENTS-NODEWRAP-006.5
func TestNodeAllocDirIsNeverSerialized(t *testing.T) {
	data, err := json.Marshal(Node{AllocDir: "/secret/alloc"})
	require.NoError(t, err)
	require.NotContains(t, string(data), "alloc")
	var node Node
	require.NoError(t, json.Unmarshal([]byte(`{"AllocDir":"/x","alloc_dir":"/x"}`), &node))
	require.Empty(t, node.AllocDir, "a request cannot set it")
}

// @covers AC-AGENTS-NODEWRAP-006.3
func TestNodePrepareRefUsesAllocDirInsideWorkspace(t *testing.T) {
	l := newAllocLaunch(t, "--allocation", AllocDir, "--")
	inside := filepath.Join(l.ctx.WorkDir, "alloc")
	require.NoError(t, os.Mkdir(inside, 0o700))
	link := filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(inside, link))
	resolvedWork, err := filepath.EvalSymlinks(l.ctx.WorkDir)
	require.NoError(t, err)
	for name, value := range map[string]string{"inside": inside, "symlink into it": link, "the workspace itself": resolvedWork} {
		_, err := Node{Spec: l.node, AllocDir: value}.Prepare(Spec{}, []string{l.child}, l.ctx)
		require.ErrorIs(t, err, ErrAllocDir, name)
	}
	outside := allocFixtureDir(t, 0o755)
	_, err = Node{Spec: l.node, AllocDir: outside}.Prepare(Spec{}, []string{l.child}, l.ctx)
	require.NoError(t, err)
	l.ctx.WorkDir = filepath.Join(outside, "ws")
	require.NoError(t, os.Mkdir(l.ctx.WorkDir, 0o700))
	_, err = Node{Spec: l.node, AllocDir: outside}.Prepare(Spec{}, []string{l.child}, l.ctx)
	require.NoError(t, err, "a workspace under the allocation directory is the wrapper's concern")
}
