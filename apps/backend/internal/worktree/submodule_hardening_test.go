package worktree

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// buildMaliciousRepo writes an attacker-controlled .gitmodules + gitlink,
// commits it, then creates a checked-out worktree pointing at that commit —
// exactly mirroring production's `git worktree add <path> <pr.HeadBranch>`
// during a fork-PR review task. The returned worktree path is what
// production passes to initSubmodules, so tests can drive the real sink.
func buildMaliciousRepo(t *testing.T, submoduleURL, submodulePath string) (worktreePath string) {
	t.Helper()

	repo := t.TempDir()
	runGit(t, repo, "init", "-b", "main")
	runGit(t, repo, "config", "user.email", "attacker@evil.example")
	runGit(t, repo, "config", "user.name", "Attacker")
	runGit(t, repo, "config", "commit.gpgsign", "false")

	gitmodules := fmt.Sprintf("[submodule \"evil\"]\n\tpath = %s\n\turl = %s\n", submodulePath, submoduleURL)
	if err := os.WriteFile(filepath.Join(repo, ".gitmodules"), []byte(gitmodules), 0644); err != nil {
		t.Fatalf("write .gitmodules: %v", err)
	}
	runGit(t, repo, "add", ".gitmodules")

	// Register a gitlink (mode 160000) at the submodule path so
	// `git submodule update --init` treats it as a submodule to fetch. The
	// commit hash need not exist remotely; the clone (the sink) is attempted
	// before the checkout of the gitlink commit.
	fakeCommit := "0000000000000000000000000000000000000001"
	runGit(t, repo, "update-index", "--add", "--cacheinfo",
		fmt.Sprintf("160000,%s,%s", fakeCommit, submodulePath))
	runGit(t, repo, "commit", "-m", "totally normal PR, please review")

	wt := filepath.Join(t.TempDir(), "review-wt")
	runGit(t, repo, "worktree", "add", wt, "-b", "pr-head", "main")
	return wt
}

// TestInitSubmodules_BlocksExtRCE asserts the ext:: command-executing
// transport can no longer execute, even when an ambient/global git config
// tries to re-enable it (the hardened command-line -c outranks it).
func TestInitSubmodules_BlocksExtRCE(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "pwned")
	payload := "ext::sh -c touch% " + marker // CVE-2018-17456 arg encoding

	wt := buildMaliciousRepo(t, payload, "evil")

	// Simulate a host that would otherwise allow ext:: (older git, or an
	// operator/global protocol.ext.allow=always). Pre-fix this would RCE.
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "protocol.ext.allow")
	t.Setenv("GIT_CONFIG_VALUE_0", "always")

	m := &Manager{logger: newTestLogger()}
	m.initSubmodules(context.Background(), wt)

	if _, err := os.Stat(marker); err == nil {
		t.Fatalf("SECURITY REGRESSION: ext:: submodule executed attacker command (marker created)")
	}
}

// TestInitSubmodules_BlocksFileTransport asserts local-path submodules are
// denied by default (no explicit per-repo opt-in), closing the local-path /
// path-traversal surface for untrusted trees. Exercises both the bare-path
// form and the explicit file:// URL form — the latter is the shape an
// attacker's .gitmodules is most likely to use, and on some git versions the
// two take different transport-classification code paths (CVE-2022-39253).
func TestInitSubmodules_BlocksFileTransport(t *testing.T) {
	// A real local repo to point at; the point is that the transport is
	// refused, not that the target is missing.
	target := t.TempDir()
	runGit(t, target, "init", "-b", "main")
	runGit(t, target, "config", "user.email", "a@b.c")
	runGit(t, target, "config", "user.name", "a")
	runGit(t, target, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(target, "x.txt"), []byte("x\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, target, "add", ".")
	runGit(t, target, "commit", "-m", "init")

	for _, url := range []string{target, "file://" + target} {
		wt := buildMaliciousRepo(t, url, "evil")
		m := &Manager{logger: newTestLogger()}
		m.initSubmodules(context.Background(), wt)

		if _, err := os.Stat(filepath.Join(wt, "evil", "x.txt")); err == nil {
			t.Fatalf("SECURITY REGRESSION: local submodule %q initialized despite protocol.allow=never", url)
		}
	}
}

// TestInitSubmodules_BlocksPlainHTTPFetch asserts that a plain http:// (not
// https) submodule URL — the classic cloud-metadata SSRF vector, e.g.
// http://169.254.169.254/… — is refused. Only https and ssh are allowlisted,
// so protocol.allow=never denies http. The pre-fix code fetched it (see the
// PoC report); this test locks the fix in.
//
// The remaining residual is an attacker-chosen *https* (or ssh) URL, which is
// the transport real submodules use and cannot be blanket-denied without also
// breaking legitimate submodules. Closing that for untrusted (fork-PR)
// worktrees requires gating submodule init off entirely — see the report.
func TestInitSubmodules_BlocksPlainHTTPFetch(t *testing.T) {
	var hit int32
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.StoreInt32(&hit, 1)
		w.WriteHeader(http.StatusNotFound)
	})}
	defer func() { _ = srv.Close() }()
	go func() { _ = srv.Serve(ln) }()

	url := fmt.Sprintf("http://%s/evil.git", ln.Addr().String())
	wt := buildMaliciousRepo(t, url, "evil")

	m := &Manager{logger: newTestLogger()}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	m.initSubmodules(ctx, wt)

	time.Sleep(200 * time.Millisecond)
	if atomic.LoadInt32(&hit) == 1 {
		t.Fatalf("SECURITY REGRESSION: plain http:// submodule URL was fetched (SSRF); expected protocol.allow=never to block it")
	}
}

// TestNewSubmoduleUpdateCmd_HasHardeningFlags asserts the sink command
// carries every hardening flag and env, at the sink so all call sites
// benefit. This is the structural guard against silent regressions.
func TestNewSubmoduleUpdateCmd_HasHardeningFlags(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "wt")
	m := &Manager{logger: newTestLogger()}
	cmd := m.newSubmoduleUpdateCmd(context.Background(), dir)
	joined := strings.Join(cmd.Args, " ")

	wantFlags := []string{
		"-c protocol.allow=never",
		"-c protocol.https.allow=always",
		"-c protocol.ssh.allow=always",
		"-c protocol.ext.allow=never",
		"-c core.hooksPath=" + os.DevNull,
		"submodule update --init --recursive",
	}
	for _, want := range wantFlags {
		if !strings.Contains(joined, want) {
			t.Errorf("submodule command missing %q; got: %s", want, joined)
		}
	}

	// The full non-interactive env set must be present so an attacker ssh://
	// (or http) submodule URL cannot hang the runner on a prompt.
	wantEnv := []string{
		"GIT_TERMINAL_PROMPT=0",
		"GIT_SSH_COMMAND=ssh -oBatchMode=yes",
		"SSH_ASKPASS=/bin/false",
		"GCM_INTERACTIVE=Never",
	}
	haveEnv := make(map[string]bool, len(cmd.Env))
	for _, e := range cmd.Env {
		haveEnv[e] = true
	}
	for _, want := range wantEnv {
		if !haveEnv[want] {
			t.Errorf("submodule command env missing %q; got: %v", want, cmd.Env)
		}
	}
	if cmd.Dir != dir {
		t.Errorf("cmd.Dir = %q, want %q", cmd.Dir, dir)
	}
}
