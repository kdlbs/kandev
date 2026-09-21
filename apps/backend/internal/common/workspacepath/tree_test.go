package workspacepath

import "testing"

func TestValidateTreePath(t *testing.T) {
	for _, path := range []string{"", "src", "src/components", ".codex/agents"} {
		t.Run("valid_"+path, func(t *testing.T) {
			if err := ValidateTreePath(path); err != nil {
				t.Fatalf("ValidateTreePath(%q) = %v, want nil", path, err)
			}
		})
	}

	for _, path := range []string{
		"/home/jcfs/project/src",
		`C:\workspace\src`,
		"C:/workspace/src",
		"file:///workspace/src",
		"file:/workspace/src",
		".",
		"..",
		"../src",
		"src/../outside",
		"src//components",
		"src/components/",
		"src\\components",
		"src\x00components",
	} {
		t.Run("invalid_"+path, func(t *testing.T) {
			if err := ValidateTreePath(path); err != ErrTreePathNotRelative {
				t.Fatalf("ValidateTreePath(%q) = %v, want %v", path, err, ErrTreePathNotRelative)
			}
		})
	}
}
