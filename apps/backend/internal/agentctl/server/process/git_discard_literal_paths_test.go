package process

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

type discardLiteralCase struct {
	name, path, decoy, kind string
	multiple                bool
}

var discardLiteralCases = []discardLiteralCase{
	{name: "ordinary-tracked", path: "selected.txt", decoy: "other.txt", kind: "tracked"},
	{name: "tracked-bracket-control", path: "new[ab].txt", decoy: "newa.txt", kind: "tracked"},
	{name: "ordinary-untracked", path: "selected.txt", decoy: "other.txt", kind: "untracked"},
	{name: "ordinary-added", path: "selected.txt", decoy: "other.txt", kind: "added"},
	{name: "untracked-bracket", path: "new[ab].txt", decoy: "newa.txt", kind: "untracked"},
	{name: "added-bracket", path: "new[ab].txt", decoy: "newa.txt", kind: "added"},
	{name: "native-glob", path: ":(glob)a*.txt", decoy: "alpha.txt", kind: "tracked"},
	{name: "native-exclude", path: ":(exclude)alpha.txt", decoy: "alpha.txt", kind: "tracked"},
	{name: "multiple", path: "new[ab].txt", decoy: "newa.txt", kind: "untracked", multiple: true},
}

// @covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.44
func TestGitOperatorDiscardLiteralSelections(t *testing.T) {
	isolateTestGitEnv(t)
	for _, tc := range discardLiteralCases {
		t.Run(tc.name, func(t *testing.T) {
			skipNativeInvalidLiteralPath(t, tc.path)
			dir, selected, want := seedDiscardLiteralRepo(t, tc)
			op := NewGitOperator(dir, newTestLogger(t), nil)
			result, err := op.Discard(context.Background(), selected)
			if err != nil || !result.Success {
				t.Errorf("Discard result=%+v err=%v", result, err)
			}
			assertDiscardLiteralFiles(t, dir, want)
		})
	}
	for _, paths := range [][]string{nil, {}, {""}} {
		t.Run("invalid/"+strings.Join(paths, ","), func(t *testing.T) {
			dir, _, want := seedDiscardLiteralRepo(t, discardLiteralCases[0])
			want["selected.txt"] = snapshotLiteralFile(t, dir, "selected.txt")
			result, err := NewGitOperator(dir, newTestLogger(t), nil).Discard(context.Background(), paths)
			if err != nil || result.Success || result.Error == "" {
				t.Errorf("invalid selection result=%+v err=%v", result, err)
			}
			assertDiscardLiteralFiles(t, dir, want)
		})
	}
}

func seedDiscardLiteralRepo(t *testing.T, tc discardLiteralCase) (string, []string, map[string]literalFileSnapshot) {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "--initial-branch=main")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "core.autocrlf", "false")
	headPaths := []string{tc.decoy, "keep.txt", "second.txt", "addeda.txt"}
	if tc.kind == "tracked" {
		headPaths = append(headPaths, tc.path)
	}
	for _, path := range headPaths {
		writeFile(t, dir, path, path+" HEAD\n")
	}
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", "discard fixture")
	want := make(map[string]literalFileSnapshot)
	for _, path := range headPaths {
		want[path] = snapshotLiteralFile(t, dir, path)
	}
	writeFile(t, dir, tc.path, "selected index\n")
	if tc.kind != "untracked" && tc.name != "tracked-bracket-control" {
		runGit(t, dir, "add", "--", ":(literal)"+tc.path)
	}
	writeFile(t, dir, tc.path, "selected worktree\n")
	if tc.kind != "tracked" {
		want[tc.path] = literalFileSnapshot{}
	}
	selected := []string{tc.path}
	if tc.multiple {
		selected = append(selected, "second.txt", "added[ab].txt")
		writeFile(t, dir, "second.txt", "second index\n")
		writeFile(t, dir, "added[ab].txt", "added index\n")
		runGit(t, dir, "add", "--", "second.txt", ":(literal)added[ab].txt")
		writeFile(t, dir, "second.txt", "second worktree\n")
		writeFile(t, dir, "added[ab].txt", "added worktree\n")
		want["added[ab].txt"] = literalFileSnapshot{}
	}
	for _, path := range []string{tc.decoy, "keep.txt", "addeda.txt"} {
		writeFile(t, dir, path, path+" decoy index\n")
		if tc.name != "tracked-bracket-control" {
			runGit(t, dir, "add", "--", ":(literal)"+path)
		}
		writeFile(t, dir, path, path+" decoy worktree\n")
		want[path] = snapshotLiteralFile(t, dir, path)
	}
	return dir, selected, want
}

func assertDiscardLiteralFiles(t *testing.T, dir string, want map[string]literalFileSnapshot) {
	t.Helper()
	for path, expected := range want {
		if got := snapshotLiteralFile(t, dir, path); got != expected {
			t.Errorf("file %q = %+v, want %+v", path, got, expected)
		}
	}
}

// @covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.44
func TestGitOperatorDiscardLiteralEnvironment(t *testing.T) {
	isolateTestGitEnv(t)
	settings := []map[string]string{
		{"GIT_LITERAL_PATHSPECS": "1"}, {"GIT_ICASE_PATHSPECS": "1"},
		{"GIT_LITERAL_PATHSPECS": "1", "GIT_ICASE_PATHSPECS": "1"},
		{"GIT_GLOB_PATHSPECS": "1"}, {"GIT_NOGLOB_PATHSPECS": "1"},
	}
	cases := []discardLiteralCase{discardLiteralCases[4], discardLiteralCases[5],
		{name: "case", path: "Foo.txt", decoy: "foo.txt", kind: "tracked"}}
	for i, setting := range settings {
		for _, provider := range []bool{false, true} {
			for _, tc := range cases {
				t.Run(fmt.Sprintf("%s/setting-%d/provider-%t", tc.name, i, provider), func(t *testing.T) {
					checkDiscardLiteralEnvironment(t, tc, setting, provider)
				})
			}
		}
	}
}

func checkDiscardLiteralEnvironment(t *testing.T, tc discardLiteralCase, setting map[string]string, provider bool) {
	t.Helper()
	dir, selected, want := seedDiscardLiteralRepo(t, tc)
	if tc.name == "case" {
		skipCaseInsensitiveLiteralFilesystem(t, dir, [2]string{tc.path, tc.decoy})
	}
	for _, key := range []string{"GIT_LITERAL_PATHSPECS", "GIT_ICASE_PATHSPECS", "GIT_GLOB_PATHSPECS", "GIT_NOGLOB_PATHSPECS"} {
		t.Setenv(key, "0")
	}
	captured := withEnvironmentOverrides(filterTestGitEnv(os.Environ()), setting)
	op := NewGitOperator(dir, newTestLogger(t), nil)
	if provider {
		op.setEnvironmentProvider(func() []string { return captured })
	} else {
		for key, value := range setting {
			t.Setenv(key, value)
		}
	}
	beforeCaptured, beforeAmbient := append([]string(nil), captured...), os.Environ()
	result, err := op.Discard(context.Background(), selected)
	if err != nil || !result.Success {
		t.Errorf("Discard result=%+v err=%v", result, err)
	}
	assertDiscardLiteralFiles(t, dir, want)
	if !reflect.DeepEqual(captured, beforeCaptured) || !reflect.DeepEqual(os.Environ(), beforeAmbient) {
		t.Error("Discard changed the provider or process environment")
	}
}
