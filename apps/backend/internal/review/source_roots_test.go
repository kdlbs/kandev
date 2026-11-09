package review

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSourceWithinRoots(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "repo")
	inside := filepath.Join(root, "nested")
	sibling := filepath.Join(base, "repo-evil")
	outside := filepath.Join(base, "other")
	for _, dir := range []string{inside, sibling, outside} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	roots := []string{root}

	cases := []struct {
		name string
		path string
		want bool
	}{
		{name: "root itself", path: root, want: true},
		{name: "directory inside the root", path: inside, want: true},
		{name: "unrelated directory", path: outside, want: false},
		{name: "sibling sharing the root prefix", path: sibling, want: false},
		{name: "traversal out of the root", path: root + string(filepath.Separator) + ".." + string(filepath.Separator) + "other", want: false},
		{name: "relative path", path: "repo", want: false},
		{name: "missing path", path: filepath.Join(root, "missing"), want: false},
	}
	if runtime.GOOS == "windows" {
		cases = append(cases, struct {
			name string
			path string
			want bool
		}{name: "different case on Windows", path: strings.ToUpper(root), want: true})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SourceWithinRoots(tc.path, roots); got != tc.want {
				t.Fatalf("SourceWithinRoots(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
	if SourceWithinRoots(root, nil) {
		t.Fatal("no roots must admit nothing")
	}
}

func TestSourceWithinRootsResolvesSymlinks(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "repo")
	outside := filepath.Join(base, "other")
	for _, dir := range []string{root, outside} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	escape := filepath.Join(root, "escape")
	if err := os.Symlink(outside, escape); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if SourceWithinRoots(escape, []string{root}) {
		t.Fatal("a link inside the root that points outside must be rejected")
	}

	linkedRoot := filepath.Join(base, "linked-repo")
	if err := os.Symlink(root, linkedRoot); err != nil {
		t.Fatal(err)
	}
	if !SourceWithinRoots(root, []string{linkedRoot}) {
		t.Fatal("a root named through a link must admit its target")
	}
	if !SourceWithinRoots(linkedRoot, []string{root}) {
		t.Fatal("a path naming the root through a link must be admitted")
	}
}
