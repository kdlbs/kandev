package process

import "testing"

// TestResolveRescanPath locks in the rescan endpoint's path-injection
// contract. The function is the sole mitigation for the CodeQL
// go/path-injection finding on /api/v1/workspace/rescan: it maps the
// HTTP-supplied work_dir to a path DERIVED from the manager's existing
// cfg.WorkDir, so the value that reaches os.Stat is never the raw HTTP
// input. Only two transitions are allowed: no-op (equal to current) and
// promotion-up-one-level (equal to parent of current).
func TestResolveRescanPath(t *testing.T) {
	cases := []struct {
		name     string
		newPath  string
		current  string
		wantPath string
		wantOK   bool
	}{
		{"empty new path", "", "/task/repo", "", false},
		{"empty current (no anchor)", "/task/repo", "", "", false},
		{"relative path", "task/repo", "/task/repo", "", false},
		{"exact match (no-op)", "/task/repo", "/task/repo", "/task/repo", true},
		{"promotion to parent (allowed)", "/task", "/task/repo", "/task", true},
		{"two-level ancestor (rejected)", "/", "/task/repo", "", false},
		{"child of current (rejected)", "/task/repo/sub", "/task/repo", "", false},
		{"sibling repo (rejected)", "/task/other", "/task/repo", "", false},
		{"unrelated absolute (rejected)", "/etc", "/task/repo", "", false},
		{"traversal cleaned to ancestor (rejected after Clean)", "/task/../etc", "/task/repo", "", false},
		{"current is filesystem root", "/", "/", "/", true},
		{"current is root, promotion impossible", "/foo", "/", "", false},
		{"dotted segment is legal as exact match", "/home/u/..hidden", "/home/u/..hidden", "/home/u/..hidden", true},
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
}
