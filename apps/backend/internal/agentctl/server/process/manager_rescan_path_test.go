package process

import "testing"

// TestValidateRescanPath locks in the rescan endpoint's path-injection
// contract. The function is the sole mitigation for the CodeQL
// go/path-injection finding on /api/v1/workspace/rescan; its rejection
// paths (relative input, sibling/child of current workdir, empty input)
// are the security boundary, so they get explicit coverage here.
func TestValidateRescanPath(t *testing.T) {
	cases := []struct {
		name     string
		newPath  string
		current  string
		wantPath string
		wantOK   bool
	}{
		{"empty new path", "", "/task/repo", "", false},
		{"relative path", "task/repo", "/task/repo", "", false},
		{"exact match", "/task/repo", "/task/repo", "/task/repo", true},
		{"valid ancestor (promotion)", "/task", "/task/repo", "/task", true},
		{"deeper ancestor", "/", "/task/repo", "/", true},
		{"child (not ancestor)", "/task/repo/sub", "/task/repo", "", false},
		{"sibling repo", "/task/other", "/task/repo", "", false},
		{"unrelated absolute", "/etc", "/task/repo", "", false},
		{"traversal cleaned to ancestor", "/task/../etc", "/task/repo", "", false},
		{"no current workdir (first launch)", "/task/repo", "", "/task/repo", true},
		{"dotted segment is legal (not traversal)", "/home/u/..hidden", "/home/u/..hidden/repo", "/home/u/..hidden", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := validateRescanPath(tc.newPath, tc.current)
			if ok != tc.wantOK || got != tc.wantPath {
				t.Errorf("validateRescanPath(%q, %q) = (%q, %v); want (%q, %v)",
					tc.newPath, tc.current, got, ok, tc.wantPath, tc.wantOK)
			}
		})
	}
}
