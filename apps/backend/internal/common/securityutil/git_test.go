package securityutil

import "testing"

func TestIsValidBaseBranchRef(t *testing.T) {
	cases := map[string]bool{
		"main":              true,
		"release/v2":        true,
		"feature/foo-bar":   true,
		"origin/main":       true,
		"origin/release/v2": true,
		"":                  false,
		"main/":             false, // trailing slash
		"origin/main/":      false, // trailing slash after origin strip
		"a//b":              false, // consecutive slashes
		"bad..ref":          false, // path traversal
		"feature.lock":      false, // .lock suffix
		"bad branch":        false, // space
		"bad;rm -rf":        false, // shell metacharacters
		"-flag":             false, // leading dash (regex requires alphanumeric first)
		"/leading":          false, // leading slash
	}
	for ref, want := range cases {
		if got := IsValidBaseBranchRef(ref); got != want {
			t.Errorf("IsValidBaseBranchRef(%q) = %v, want %v", ref, got, want)
		}
	}
}
