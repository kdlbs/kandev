package process

import (
	"path/filepath"
	"runtime"
	"testing"
)

// TestResolveRescanPath locks in the rescan endpoint's path-injection
// contract. The function is the sole mitigation for the CodeQL
// go/path-injection finding on /api/v1/workspace/rescan: it maps the
// HTTP-supplied work_dir to a path DERIVED from the manager's existing
// cfg.WorkDir, so the value that reaches os.Stat is never the raw HTTP
// input. Only two transitions are allowed: no-op (equal to current) and
// promotion-up-one-level (equal to parent of current).
//
// Inputs are constructed via filepath.Join so the same table covers
// Windows (`C:\...`) and POSIX (`/...`) without per-OS branches in
// every case.
func TestResolveRescanPath(t *testing.T) {
	root := filepath.Join(t.TempDir(), "root")
	repo := join(root, "task", "repo")
	task := join(root, "task")
	other := join(root, "task", "other")
	sub := join(root, "task", "repo", "sub")
	etc := join(root, "etc")
	dotted := join(root, "home", "u", "..hidden")
	dottedRepo := join(dotted, "repo")

	cases := []struct {
		name     string
		newPath  string
		current  string
		wantPath string
		wantOK   bool
	}{
		{"empty new path", "", repo, "", false},
		{"empty current (no anchor)", repo, "", "", false},
		{"relative path", "task/repo", repo, "", false},
		{"exact match (no-op)", repo, repo, repo, true},
		{"promotion to parent (allowed)", task, repo, task, true},
		{"two-level ancestor (rejected)", root, repo, "", false},
		{"child of current (rejected)", sub, repo, "", false},
		{"sibling repo (rejected)", other, repo, "", false},
		{"unrelated absolute (rejected)", etc, repo, "", false},
		{"current is anchor root", root, root, root, true},
		{"current is anchor root, child rejected", join(root, "foo"), root, "", false},
		{"dotted segment is legal as exact match", dotted, dotted, dotted, true},
		{"dotted segment is legal as promotion", dotted, dottedRepo, dotted, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := resolveRescanPath(tc.newPath, tc.current)
			if ok != tc.wantOK || got != tc.wantPath {
				t.Errorf("resolveRescanPath(%q, %q) = (%q, %v); want (%q, %v)",
					tc.newPath, tc.current, got, ok, tc.wantPath, tc.wantOK)
			}
		})
	}

	// Traversal-after-Clean is a POSIX-only invariant — on Windows the
	// input would also need a volume to be considered absolute; covering
	// it once on POSIX is enough since filepath.Clean is the shared
	// primitive.
	if runtime.GOOS != "windows" {
		t.Run("traversal cleaned to ancestor (rejected after Clean)", func(t *testing.T) {
			got, ok := resolveRescanPath("/task/../etc", "/task/repo")
			if ok || got != "" {
				t.Errorf("expected rejection for /task/../etc, got (%q, %v)", got, ok)
			}
		})
	}
}

func TestResolveRescanPath_RejectsUnrelatedDirectoryWithGitChild(t *testing.T) {
	current := filepath.Join(t.TempDir(), "workspace", "repo")
	unrelated := t.TempDir()
	initGitRepoAt(t, filepath.Join(unrelated, "unrelated-repository"))

	got, ok := resolveRescanPath(unrelated, current)
	if ok || got != "" {
		t.Fatalf("resolveRescanPath accepted unrelated git directory: (%q, %v)", got, ok)
	}
}

// join wraps filepath.Join so callers can express paths with separate
// segments without thinking about platform separators.
func join(root string, segs ...string) string {
	parts := append([]string{root}, segs...)
	return filepath.Join(parts...)
}
