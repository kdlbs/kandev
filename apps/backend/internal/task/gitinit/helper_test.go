package gitinit

import "testing"

func TestRunHelperIgnoresPublicArguments(t *testing.T) {
	if code, handled := runHelper([]string{"--help"}); handled || code != 0 {
		t.Fatalf("runHelper = (%d, %t), want (0, false)", code, handled)
	}
}

func TestRunHelperRejectsMissingGitPath(t *testing.T) {
	if code, handled := runHelper([]string{helperArgument}); !handled || code != 2 {
		t.Fatalf("runHelper = (%d, %t), want (2, true)", code, handled)
	}
}
