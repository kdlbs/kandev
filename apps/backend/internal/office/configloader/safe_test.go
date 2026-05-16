package configloader

import "testing"

// TestValidateWorkspaceName pins the rejection branches of the validator
// that guards every os.Stat / os.ReadFile / os.ReadDir / exec.Command call
// in this package. Any regression that lets one of these slip through is a
// path-traversal vulnerability, so coverage matters here.
func TestValidateWorkspaceName(t *testing.T) {
	bad := []string{
		"",
		".",
		"..",
		"../escape",
		"a/b",
		"a\\b",
		"a\x00b",
		"a..b",
	}
	for _, name := range bad {
		if err := validateWorkspaceName(name); err == nil {
			t.Errorf("validateWorkspaceName(%q) = nil, want error", name)
		}
	}
	good := []string{
		"default",
		"my-workspace",
		"ws_1",
		"alpha.beta",
	}
	for _, name := range good {
		if err := validateWorkspaceName(name); err != nil {
			t.Errorf("validateWorkspaceName(%q) = %v, want nil", name, err)
		}
	}
}
