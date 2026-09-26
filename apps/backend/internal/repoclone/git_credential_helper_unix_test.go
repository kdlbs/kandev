//go:build !windows

package repoclone

import (
	"os"
	"os/exec"
	"testing"
)

func TestGitCredentialHelperSupportsRepeatedRequests(t *testing.T) {
	t.Parallel()

	helper, files, helperEnv, cleanup, err := gitCredentialHelperCommand(&cloneAuth{
		username: "x-token-auth", password: "transient-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	cmd := exec.Command("sh", "-c", `"$1" get; "$1" get`, "credential-helper-test", helper)
	cmd.ExtraFiles = files
	cmd.Env = append(os.Environ(), helperEnv...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run credential helper twice: %v: %s", err, output)
	}
	want := "username=x-token-auth\npassword=transient-token\n" +
		"username=x-token-auth\npassword=transient-token\n"
	if string(output) != want {
		t.Fatalf("repeated credential helper output = %q, want %q", output, want)
	}
}
