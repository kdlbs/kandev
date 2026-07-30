package gitinit

import "testing"

func TestRunHelperIgnoresPublicArguments(t *testing.T) {
	if code, handled := RunHelper([]string{"--help"}); handled || code != 0 {
		t.Fatalf("RunHelper = (%d, %t), want (0, false)", code, handled)
	}
}

func TestRunHelperRejectsMissingGitPath(t *testing.T) {
	if code, handled := RunHelper([]string{helperArgument}); !handled || code != 2 {
		t.Fatalf("RunHelper = (%d, %t), want (2, true)", code, handled)
	}
}
