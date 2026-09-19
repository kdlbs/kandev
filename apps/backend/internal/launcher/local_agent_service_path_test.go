package launcher

import (
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"testing"
)

func TestLocalAgentPathSelectedServiceAccount(t *testing.T) {
	home, _ := localAgentEnvironment(t, "/usr/bin:/bin")
	plist := renderLaunchdPlist(nativeServiceUnitInput{
		Executable: "/opt/kandev/bin/kandev", HomeDir: t.TempDir(), System: true, SystemUser: "alice",
	})
	if got := launchdPlistString(t, plist, "UserName"); got != "alice" {
		t.Fatalf("service user = %q", got)
	}
	original := lookupLocalAgentUser
	t.Cleanup(func() { lookupLocalAgentUser = original })
	lookupLocalAgentUser = func(uid string) (*user.User, error) {
		if uid != strconv.Itoa(os.Geteuid()) {
			t.Fatalf("looked up uid %q instead of effective uid", uid)
		}
		return &user.User{Uid: "1001", Username: "alice", HomeDir: home}, nil
	}
	t.Setenv("HOME", t.TempDir())
	env := backendEnv(portConfig{}, "", "", false, "test", []string{
		"PATH=" + launchdPlistString(t, plist, "PATH"),
		"KANDEV_SERVICE_MODE=" + launchdPlistString(t, plist, "KANDEV_SERVICE_MODE"),
		"KANDEV_TEST_LOCAL_AGENT_CHILD=1",
	})
	installLocalAgentFixture(t, home)
	runLocalAgentChild(t, env)
}

func TestLocalAgentPathServiceLookupFailure(t *testing.T) {
	_, _ = localAgentEnvironment(t, "/usr/bin:/bin")
	original := lookupLocalAgentUser
	t.Cleanup(func() { lookupLocalAgentUser = original })
	for _, tc := range []struct {
		name, home string
		err        error
	}{
		{name: "lookup failure", err: errors.New("account not found")},
		{name: "missing account home"},
		{name: "relative account home", home: "relative"},
		{name: "separator in account home", home: filepath.Join(t.TempDir(), "bad:home")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lookupLocalAgentUser = func(string) (*user.User, error) { return &user.User{HomeDir: tc.home}, tc.err }
			env := backendEnv(portConfig{}, "", "", false, "test", []string{"KANDEV_SERVICE_MODE=system"})
			if got := processEnvValue(env, "PATH"); got != "/usr/bin:/bin" {
				t.Fatalf("failed lookup changed PATH to %q", got)
			}
		})
	}
}
