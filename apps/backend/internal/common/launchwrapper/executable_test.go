package launchwrapper

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenericWrapperWindowsExecutableSkipsPOSIXNpmShim(t *testing.T) {
	for _, ext := range []string{"", ".CMD;.EXE", "cmd;exe"} {
		t.Run(ext, func(t *testing.T) {
			dir := t.TempDir()
			name := filepath.Join(dir, "npx")
			require.NoError(t, os.WriteFile(name, []byte("#!/bin/sh\n"), 0600))
			suffix := ".CMD"
			if ext == "cmd;exe" {
				suffix = ".cmd"
			}
			require.NoError(t, os.WriteFile(name+suffix, []byte("@echo off\r\n"), 0600))
			got, err := windowsExecutableFile(name, []string{"PATHEXT=" + ext})
			require.NoError(t, err)
			require.Equal(t, name+suffix, got)
		})
	}
}

func TestGenericWrapperWindowsPrepareSelectsNpmCommandShim(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("native Windows execution-host resolution")
	}
	dir := t.TempDir()
	wrapper, err := os.Executable()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "npx"), []byte("#!/bin/sh\n"), 0600))
	shim := filepath.Join(dir, "npx.cmd")
	require.NoError(t, os.WriteFile(shim, []byte("@echo off\r\n"), 0600))
	got, err := Prepare(Profile([]string{wrapper}), []string{"npx", "--version"}, Context{WorkDir: dir, Env: []string{"PATH=" + dir, "PATHEXT=.CMD"}})
	require.NoError(t, err)
	require.Equal(t, []string{wrapper, filepath.Join(dir, "npx.CMD"), "--version"}, got)
}
