package launchwrapper

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func nodeJSON(t *testing.T, argv ...string) []byte {
	t.Helper()
	data, err := json.Marshal(Spec{Version: Version, Source: SourceNode, Argv: argv})
	require.NoError(t, err)
	return data
}

// @covers AC-AGENTS-NODEWRAP-001.2
func TestDecodeNodeAcceptsOnlyNodeSource(t *testing.T) {
	spec, err := DecodeNode(nodeJSON(t, "/opt/wrap", "--root", WorkDir, "--"))
	require.NoError(t, err)
	require.Equal(t, SourceNode, spec.Source)
	require.NoError(t, spec.ValidateNode())

	for name, data := range map[string]string{
		"profile source": `{"version":1,"source":"profile","argv":["/opt/wrap"]}`,
		"unknown source": `{"version":1,"source":"other","argv":["/opt/wrap"]}`,
		"unknown field":  `{"version":1,"source":"node","argv":["/opt/wrap"],"extra":1}`,
		"bad version":    `{"version":2,"source":"node","argv":["/opt/wrap"]}`,
		"null":           `null`,
		"empty argv":     `{"version":1,"source":"node","argv":[]}`,
		"relative exe":   `{"version":1,"source":"node","argv":["wrap"]}`,
		"flag exe":       `{"version":1,"source":"node","argv":["-x"]}`,
		"bad token":      `{"version":1,"source":"node","argv":["/opt/wrap","{{kandev.secret}}"]}`,
		"partial token":  `{"version":1,"source":"node","argv":["/opt/wrap","a{{kandev.work_dir}}"]}`,
		"token as exe":   `{"version":1,"source":"node","argv":["{{kandev.work_dir}}"]}`,
		"not json":       `{`,
	} {
		_, err := DecodeNode([]byte(data))
		require.ErrorIs(t, err, ErrInvalid, name)
	}
}

// @covers AC-AGENTS-NODEWRAP-001.2
func TestProfileValidationStillRejectsNodeSource(t *testing.T) {
	require.ErrorIs(t, Spec{Version: Version, Source: SourceNode, Argv: []string{"/opt/wrap"}}.Validate(), ErrInvalid)
	var spec Spec
	require.ErrorIs(t, json.Unmarshal(nodeJSON(t, "/opt/wrap"), &spec), ErrInvalid)
	require.Error(t, Spec{Version: Version, Source: SourceProfile, Argv: []string{"/opt/wrap"}}.ValidateNode())
}

func writeExecutable(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o700))
	return path
}

func secureDir(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	trustBoundaryForTest(t, root)
	dir := filepath.Join(root, "node")
	require.NoError(t, os.Mkdir(dir, 0o755))
	require.NoError(t, os.Chmod(dir, 0o755))
	return dir
}

func writeNodeFile(t *testing.T, dir string, mode os.FileMode, argv ...string) string {
	t.Helper()
	path := filepath.Join(dir, "node-wrapper.json")
	require.NoError(t, os.WriteFile(path, nodeJSON(t, argv...), 0o600))
	require.NoError(t, os.Chmod(path, mode))
	return path
}

// @covers AC-AGENTS-NODEWRAP-001.3
func TestLoadNodeFileOwnerAndMode(t *testing.T) {
	if runtime.GOOS == windowsOS {
		t.Skip("POSIX ownership model")
	}
	uid := os.Geteuid()
	dir := secureDir(t)
	wrapper := writeExecutable(t, dir, "wrap", "#!/bin/sh\nexec \"$@\"\n")
	path := writeNodeFile(t, dir, 0o600, wrapper, WorkDir)

	spec, err := LoadNodeFile(path, -1)
	require.NoError(t, err, "default owner is the process owner")
	require.Equal(t, []string{wrapper, WorkDir}, spec.Argv)
	_, err = LoadNodeFile(path, uid)
	require.NoError(t, err)

	for name, mutate := range map[string]func(path, dir, wrapper string){
		"file group writable": func(path, _, _ string) { require.NoError(t, os.Chmod(path, 0o660)) },
		"file world writable": func(path, _, _ string) { require.NoError(t, os.Chmod(path, 0o666)) },
		"ancestor writable":   func(_, dir, _ string) { require.NoError(t, os.Chmod(dir, 0o777)) },
		"ancestor group":      func(_, dir, _ string) { require.NoError(t, os.Chmod(dir, 0o775)) },
		"wrapper writable":    func(_, _, wrapper string) { require.NoError(t, os.Chmod(wrapper, 0o722)) },
		"wrapper not exec":    func(_, _, wrapper string) { require.NoError(t, os.Chmod(wrapper, 0o600)) },
		"file missing":        func(path, _, _ string) { require.NoError(t, os.Remove(path)) },
	} {
		t.Run(name, func(t *testing.T) {
			dir := secureDir(t)
			wrapper := writeExecutable(t, dir, "wrap", "#!/bin/sh\nexec \"$@\"\n")
			path := writeNodeFile(t, dir, 0o600, wrapper)
			_, err := LoadNodeFile(path, -1)
			require.NoError(t, err)
			mutate(path, dir, wrapper)
			_, err = LoadNodeFile(path, -1)
			require.ErrorIs(t, err, ErrNodeFile)
		})
	}

	t.Run("wrong owner", func(t *testing.T) {
		if uid == 0 {
			t.Skip("root owns everything the test creates")
		}
		_, err := LoadNodeFile(path, uid+1)
		require.ErrorIs(t, err, ErrNodeFile)
	})
	t.Run("relative wrapper", func(t *testing.T) {
		rel := writeNodeFile(t, secureDir(t), 0o600, "wrap")
		_, err := LoadNodeFile(rel, -1)
		require.Error(t, err)
	})
	t.Run("symlinked file is judged at its target", func(t *testing.T) {
		d := secureDir(t)
		w := writeExecutable(t, d, "wrap", "#!/bin/sh\n")
		target := writeNodeFile(t, d, 0o600, w)
		link := filepath.Join(d, "link.json")
		require.NoError(t, os.Symlink(target, link))
		_, err := LoadNodeFile(link, -1)
		require.NoError(t, err)
		require.NoError(t, os.Chmod(target, 0o666))
		_, err = LoadNodeFile(link, -1)
		require.ErrorIs(t, err, ErrNodeFile)
	})
}

// @covers AC-AGENTS-NODEWRAP-001.2
func TestLoadNodeFileRejectsBadContent(t *testing.T) {
	if runtime.GOOS == windowsOS {
		t.Skip("POSIX ownership model")
	}
	dir := secureDir(t)
	wrapper := writeExecutable(t, dir, "wrap", "#!/bin/sh\n")
	for name, content := range map[string]string{
		"profile source": `{"version":1,"source":"profile","argv":["` + wrapper + `"]}`,
		"invalid json":   `{`,
		"bad token":      `{"version":1,"source":"node","argv":["` + wrapper + `","{{kandev.nope}}"]}`,
	} {
		path := filepath.Join(dir, "bad.json")
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
		_, err := LoadNodeFile(path, -1)
		require.ErrorIs(t, err, ErrNodeFile, name)
	}
}

// @covers AC-AGENTS-NODEWRAP-002.1
func TestPrepareNodeIsOutermost(t *testing.T) {
	if runtime.GOOS == windowsOS {
		t.Skip("POSIX fixture")
	}
	root := t.TempDir()
	nodeWrap := writeExecutable(t, root, "node-wrap", "#!/bin/sh\n")
	profileWrap := writeExecutable(t, root, "profile-wrap", "#!/bin/sh\n")
	child := writeExecutable(t, root, "child", "#!/bin/sh\n")
	ctx := Context{WorkDir: root, Env: []string{"PATH=" + root}}
	node := Spec{Version: Version, Source: SourceNode, Argv: []string{nodeWrap, "--root", WorkDir, "--"}}
	profile := Profile([]string{profileWrap, "--p", "--"})
	inner := []string{child, "--acp", ""}

	argv, err := Node{Spec: node}.Prepare(profile, inner, ctx)
	require.NoError(t, err)
	require.Equal(t, []string{nodeWrap, "--root", root, "--", profileWrap, "--p", "--", child, "--acp", ""}, argv)
	require.Equal(t, nodeWrap, argv[0], "node argv is the first argv of the final command")

	argv, err = Node{Spec: node}.Prepare(Spec{}, inner, ctx)
	require.NoError(t, err)
	require.Equal(t, []string{nodeWrap, "--root", root, "--", child, "--acp", ""}, argv)

	want, err := Prepare(profile, inner, ctx)
	require.NoError(t, err)
	got, err := Node{}.Prepare(profile, inner, ctx)
	require.NoError(t, err)
	require.Equal(t, want, got, "no node wrapper is exactly today's behavior")
	want, err = Prepare(Spec{Version: Version, Source: SourceProfile, Argv: []string{profileWrap}}, inner, ctx)
	require.NoError(t, err)
	require.NotEqual(t, nodeWrap, want[0])
}

// @covers AC-AGENTS-NODEWRAP-001.2 AC-AGENTS-NODEWRAP-003.3
func TestPrepareNodeRefusesInvalidNode(t *testing.T) {
	root := t.TempDir()
	child := writeExecutable(t, root, "child", "#!/bin/sh\n")
	ctx := Context{WorkDir: root, Env: []string{"PATH=" + root}}
	for name, node := range map[string]Spec{
		"profile source": Profile([]string{child}),
		"missing exe":    {Version: Version, Source: SourceNode, Argv: []string{filepath.Join(root, "absent")}},
	} {
		_, err := Node{Spec: node}.Prepare(Spec{}, []string{child}, ctx)
		require.Error(t, err, name)
	}
}

// @covers AC-AGENTS-NODEWRAP-003.3 AC-AGENTS-NODEWRAP-003.4
func TestNodeRequiredWithoutSpecRefuses(t *testing.T) {
	root := t.TempDir()
	child := writeExecutable(t, root, "child", "#!/bin/sh\n")
	ctx := Context{WorkDir: root, Env: []string{"PATH=" + root}}
	for _, profile := range []Spec{{}, Profile([]string{child})} {
		_, err := Node{Required: true}.Prepare(profile, []string{child}, ctx)
		require.ErrorIs(t, err, ErrNodeRequired)
	}
	require.False(t, Node{}.Active())
	require.True(t, Node{Required: true}.Active())
	require.True(t, Node{Spec: Spec{Version: Version, Source: SourceNode, Argv: []string{child}}}.Active())
}

// TestLoadNodeFileValidatesTheWrapperWithoutRunningIt pins that loading a node
// wrapper only reads and stats it. The wrapper records every run in a sentinel
// file, so any execution of it, directly or through a shell, leaves evidence.
func TestLoadNodeFileValidatesTheWrapperWithoutRunningIt(t *testing.T) {
	if runtime.GOOS == windowsOS {
		t.Skip("POSIX ownership model")
	}
	dir := secureDir(t)
	sentinel := filepath.Join(dir, "wrapper-ran")
	wrapper := writeExecutable(t, dir, "wrap", "#!/bin/sh\necho ran >> '"+sentinel+"'\nexec \"$@\"\n")
	path := writeNodeFile(t, dir, 0o600, wrapper, "--")

	spec, err := LoadNodeFile(path, -1)
	require.NoError(t, err)
	require.Equal(t, []string{wrapper, "--"}, spec.Argv)
	require.NoError(t, spec.ValidateNode())
	require.NoError(t, ValidateTokens(spec.Argv))
	_, err = DecodeNode(nodeJSON(t, wrapper, "--"))
	require.NoError(t, err)

	require.NoFileExists(t, sentinel, "loading or validating the node wrapper executed it")
}

func TestNodeEnforcedNeedsRequiredAndValidLoadedWrapper(t *testing.T) {
	valid := Spec{Version: Version, Source: SourceNode, Argv: []string{"/usr/bin/true", "--"}}
	tests := []struct {
		name string
		node Node
		want bool
	}{
		{"zero node", Node{}, false},
		{"required without a wrapper", Node{Required: true}, false},
		{"optional wrapper loaded", Node{Spec: valid}, false},
		{"required with relative argv0", Node{Required: true, Spec: Spec{Version: Version, Source: SourceNode, Argv: []string{"true"}}}, false},
		{"required with wrong source", Node{Required: true, Spec: Spec{Version: Version, Source: "profile", Argv: []string{"/usr/bin/true"}}}, false},
		{"required with valid wrapper", Node{Required: true, Spec: valid}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.node.Enforced(); got != tt.want {
				t.Fatalf("Enforced() = %v, want %v", got, tt.want)
			}
		})
	}
}
