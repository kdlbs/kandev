//go:build unix

package subproc

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitSupportsConfigHooks reports whether the installed Git runs hooks defined
// through hook.<name>.event and hook.<name>.command configuration.
func gitSupportsConfigHooks(t *testing.T) bool {
	t.Helper()
	dir := t.TempDir()
	marker := filepath.Join(t.TempDir(), "probe")
	plainGit(t, dir, "init", "-q", "-b", "main")
	cmd := exec.Command("git", "-c", "hook.probe.event=pre-commit",
		"-c", "hook.probe.command=touch '"+marker+"'", "commit", "--allow-empty", "-q", "-m", "p")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=n", "GIT_AUTHOR_EMAIL=n@example.com",
		"GIT_COMMITTER_NAME=n", "GIT_COMMITTER_EMAIL=n@example.com")
	_ = cmd.Run()
	_, err := os.Stat(marker)
	return err == nil
}

func TestManagedGitDoesNotRunConfigDefinedHooksUnlessOptedIn(t *testing.T) {
	if !gitSupportsConfigHooks(t) {
		t.Skip("installed Git has no configuration-defined hooks")
	}
	repo := newPolicyRepo(t)
	plainGit(t, repo.dir, "config", "hook.mark.event", "pre-commit")
	plainGit(t, repo.dir, "config", "hook.mark.command", "echo config-hook >> '"+repo.marker+"'")
	if err := os.WriteFile(filepath.Join(repo.dir, "f.txt"), []byte("f\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(context.Background(), t, repo.dir, "add", "f.txt")
	mustGit(context.Background(), t, repo.dir, "commit", "-m", "off")
	if repo.marked() {
		t.Fatalf("a configuration-defined hook ran with hooks off:\n%s", repo.markerText())
	}

	ctx := WithGitHooks(context.Background(), true)
	if err := os.WriteFile(filepath.Join(repo.dir, "g.txt"), []byte("g\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(ctx, t, repo.dir, "add", "g.txt")
	mustGit(ctx, t, repo.dir, "commit", "-m", "on")
	if !strings.Contains(repo.markerText(), "config-hook") {
		t.Fatalf("configuration-defined hook did not run with hooks enabled:\n%s", repo.markerText())
	}
}

func TestManagedGitProtocolAllowEnvironmentCannotEnableExtTransport(t *testing.T) {
	repo := newPolicyRepo(t)
	for _, hooks := range []bool{false, true} {
		ctx := WithGitHooks(context.Background(), hooks)
		cmd := NewGitCommand(ctx, "ls-remote", "ext::sh -c 'echo ext >> "+repo.marker+"'")
		cmd.Dir = repo.dir
		cmd.Env = append(os.Environ(), "GIT_ALLOW_PROTOCOL=ext", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
		if _, err := RunGitCombinedOutputClass(ctx, GitLifecycle, cmd); err == nil {
			t.Fatalf("hooks=%v: ext transport was accepted", hooks)
		}
		if repo.marked() {
			t.Fatalf("hooks=%v: the ext transport program ran", hooks)
		}
	}
}

func TestManagedGitNeutralizesLFSProgramKeys(t *testing.T) {
	repo := newPolicyRepo(t)
	plainGit(t, repo.dir, "config", "lfs.standalonetransferagent", "evil")
	plainGit(t, repo.dir, "config", "lfs.customtransfer.evil.path", "/bin/true")
	vector, err := ManagedGitArgs(context.Background(), append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null"),
		[]string{"-C", repo.dir, "push", "origin"})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(vector, "\x00")
	for _, want := range []string{"lfs.standalonetransferagent=", "lfs.customtransfer.evil.path="} {
		if !strings.Contains(joined, "\x00"+want+"\x00") {
			t.Fatalf("vector %q lacks the %q override", vector, want)
		}
	}
}

func TestManagedGitLFSExtensionRemovesLFSFromAllowlist(t *testing.T) {
	set, err := parseDriverOutput([]byte("lfs.extension.x.clean\nprog\x00" +
		"filter.lfs.clean\ngit-lfs clean -- %f\x00filter.lfs.smudge\ngit-lfs smudge -- %f\x00"))
	if err != nil {
		t.Fatal(err)
	}
	pairs, lfs, err := driverPolicy(set, classContentWriting, os.Environ(), false)
	if err != nil {
		t.Fatal(err)
	}
	if lfs {
		t.Fatal("lfs filter stayed allowlisted with an extension program configured")
	}
	if !strings.Contains(strings.Join(pairs, " "), "filter.lfs.clean=") {
		t.Fatalf("lfs filter was not neutralized: %v", pairs)
	}
}

func TestManagedGitRefUpdatingCommandsRunConfigDefinedHooksOnlyWhenOptedIn(t *testing.T) {
	if !gitSupportsConfigHooks(t) {
		t.Skip("installed Git has no configuration-defined hooks")
	}
	commands := map[string][]string{
		"update-ref":   {"update-ref", "refs/heads/zz", "HEAD"},
		"symbolic-ref": {"symbolic-ref", "HEAD", "refs/heads/zz"},
		"fetch":        {"fetch", "--tags", "."},
		"branch":       {"branch", "yy"},
	}
	for name, args := range commands {
		for _, optedIn := range []bool{false, true} {
			t.Run(name, func(t *testing.T) {
				repo := newPolicyRepo(t)
				plainGit(t, repo.dir, "commit", "--allow-empty", "-q", "-m", "base")
				if name == "fetch" {
					plainGit(t, repo.dir, "tag", "tt")
				}
				plainGit(t, repo.dir, "config", "hook.mark.event", "reference-transaction")
				plainGit(t, repo.dir, "config", "hook.mark.command", "echo config-hook >> '"+repo.marker+"'")
				ctx := WithGitHooks(context.Background(), optedIn)
				if name == "fetch" {
					args = []string{"fetch", "--tags", "--force", repo.dir, "refs/heads/main:refs/heads/fetched"}
				}
				if out, err := runGit(ctx, t, repo.dir, args...); err != nil {
					t.Fatalf("%s: %v\n%s", name, err, out)
				}
				if got := strings.Contains(repo.markerText(), "config-hook"); got != optedIn {
					t.Fatalf("%s opted-in=%v: config hook ran=%v\n%s", name, optedIn, got, repo.markerText())
				}
			})
		}
	}
}

func TestManagedGitWriteTreeRunsConfigDefinedHooksOnlyWhenOptedIn(t *testing.T) {
	if !gitSupportsConfigHooks(t) {
		t.Skip("installed Git has no configuration-defined hooks")
	}
	for _, optedIn := range []bool{false, true} {
		repo := newPolicyRepo(t)
		plainGit(t, repo.dir, "commit", "--allow-empty", "-q", "-m", "base")
		plainGit(t, repo.dir, "read-tree", "HEAD")
		plainGit(t, repo.dir, "config", "hook.mark.event", "post-index-change")
		plainGit(t, repo.dir, "config", "hook.mark.command", "echo config-hook >> '"+repo.marker+"'")
		ctx := WithGitHooks(context.Background(), optedIn)
		if out, err := runGit(ctx, t, repo.dir, "write-tree"); err != nil {
			t.Fatalf("write-tree: %v\n%s", err, out)
		}
		if got := strings.Contains(repo.markerText(), "config-hook"); got != optedIn {
			t.Fatalf("opted-in=%v: write-tree config hook ran=%v\n%s", optedIn, got, repo.markerText())
		}
	}
}
