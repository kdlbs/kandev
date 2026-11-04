package launchwrapper

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/managednpm"
)

// @covers AC-AGENTS-WRAPPERS-001.1 AC-AGENTS-WRAPPERS-002.3 AC-AGENTS-WRAPPERS-004.3
func TestGenericWrapperPrepare(t *testing.T) {
	root := t.TempDir()
	executable, err := os.Executable()
	require.NoError(t, err)
	name := "unrelated wrapper"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	bytes, err := os.ReadFile(executable)
	require.NoError(t, err)
	wrapper := filepath.Join(root, name)
	require.NoError(t, os.WriteFile(wrapper, bytes, 0o700))
	env := []string{"PATH=" + root, "KANDEV_TEST_WRAPPER=1", "WRAPPER_VALUE=value with spaces"}
	inner := []string{executable, "", "$(false); 'quoted'", "tail"}
	spec := Spec{Version: 1, Source: "profile", Argv: []string{name, "-test.run=^TestGenericWrapperReceipt$", "--", WorkDir}}
	original := slices.Clone(spec.Argv)
	for _, command := range []string{name, wrapper, "." + string(filepath.Separator) + name} {
		spec.Argv[0] = command
		original[0] = command
		argv, prepareErr := Prepare(spec, inner, Context{WorkDir: root, Env: env})
		require.NoError(t, prepareErr)
		require.Equal(t, wrapper, argv[0])
		cmd := exec.Command(argv[0], argv[1:]...)
		cmd.Dir, cmd.Env = root, env
		output, runErr := cmd.Output()
		require.NoError(t, runErr)
		var got struct {
			Args       []string
			Dir, Value string
		}
		require.NoError(t, json.Unmarshal(output, &got))
		require.Equal(t, append([]string{root}, inner...), got.Args)
		require.Equal(t, root, got.Dir)
		require.Equal(t, "value with spaces", got.Value)
		require.Equal(t, original, spec.Argv)
	}
}

func TestGenericWrapperReceipt(t *testing.T) {
	if os.Getenv("KANDEV_TEST_WRAPPER") != "1" {
		return
	}
	index := slices.Index(os.Args, "--")
	dir, err := os.Getwd()
	if err != nil || index < 0 {
		os.Exit(2)
	}
	if err := json.NewEncoder(os.Stdout).Encode(struct {
		Args       []string
		Dir, Value string
	}{
		os.Args[index+1:], dir, os.Getenv("WRAPPER_VALUE"),
	}); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

// @covers AC-AGENTS-WRAPPERS-002.1 AC-AGENTS-WRAPPERS-002.2 AC-AGENTS-WRAPPERS-002.4
func TestGenericWrapperRuntimePaths(t *testing.T) {
	temp := t.TempDir()
	for _, key := range []string{"TMPDIR", "TEMP", "TMP"} {
		t.Setenv(key, temp)
	}
	executable, err := os.Executable()
	require.NoError(t, err)
	inner := []string{executable, "--prefix", managednpm.NPMProjectPrefix}
	spec := Spec{1, "profile", []string{executable, ManagedNPMDir, WorkDir, "{{{kandev.work_dir}}}", "--prefix", managednpm.NPMProjectPrefix}}
	argv, err := Prepare(spec, inner, Context{WorkDir: t.TempDir()})
	require.NoError(t, err)
	require.Equal(t, argv[1], argv[len(argv)-1])
	require.Equal(t, WorkDir, argv[3])
	require.Equal(t, managednpm.NPMProjectPrefix, argv[5], "wrapper arguments are not trusted npm markers")
	require.Equal(t, managednpm.NPMProjectPrefix, inner[2], "input template is immutable")
	info, err := os.Lstat(argv[1])
	require.NoError(t, err)
	require.True(t, info.IsDir())
	require.Equal(t, temp, filepath.Dir(argv[1]))
	if runtime.GOOS != "windows" {
		require.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	}
}

func TestGenericWrapperRefusesInvalidContext(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)
	root := t.TempDir()
	tests := []struct {
		name  string
		spec  Spec
		inner []string
		ctx   Context
	}{
		{"missing wrapper", Spec{1, "profile", []string{"not-installed-wrapper"}}, []string{executable}, Context{WorkDir: root}},
		{"unknown version", Spec{2, "profile", []string{executable}}, []string{executable}, Context{WorkDir: root}},
		{"unknown source", Spec{1, "request", []string{executable}}, []string{executable}, Context{WorkDir: root}},
		{"unknown reference", Spec{1, "profile", []string{executable, "{{kandev.secret}}"}}, []string{executable}, Context{WorkDir: root}},
		{"missing cwd", Spec{1, "profile", []string{executable}}, []string{executable}, Context{WorkDir: filepath.Join(root, "missing")}},
		{"empty child", Spec{1, "profile", []string{executable}}, nil, Context{WorkDir: root}},
		{"missing child", Spec{1, "profile", []string{executable}}, []string{"not-installed-child"}, Context{WorkDir: root}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Prepare(tt.spec, tt.inner, tt.ctx)
			require.Error(t, err)
			require.NotContains(t, err.Error(), executable)
		})
	}
}

func TestGenericWrapperDoesNotSearchImplicitCurrentDirectory(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)
	root := t.TempDir()
	name := "only-in-cwd"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	data, err := os.ReadFile(executable)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, name), data, 0o700))
	_, err = Prepare(Spec{1, "profile", []string{name}}, []string{executable}, Context{root, []string{"PATH=.", "PATHEXT=.EXE"}})
	require.ErrorIs(t, err, ErrUnavailable)
}
