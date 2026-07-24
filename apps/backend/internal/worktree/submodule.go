package worktree

import (
	"context"
	"os"
	"os/exec"
	"strings"

	"go.uber.org/zap"
)

// hardenedSubmoduleConfig are the `-c` git config overrides applied when
// initializing submodules. Submodule init parses .gitmodules from the
// worktree's HEAD, which for a fork-PR review task is fully
// attacker-controlled. A bare `git submodule update --init --recursive`
// would let that .gitmodules pick the transport git uses to fetch each
// submodule — including command-executing transports (`ext::sh -c …`,
// CVE-2018-17456) and local `file://` paths. These overrides lock the
// transport down before any attacker content is read:
//
//   - protocol.allow=never          default-deny every transport…
//   - protocol.https.allow=always   …then re-enable only https…
//   - protocol.ssh.allow=always     …and ssh (the transports real
//     submodules use).
//   - protocol.ext.allow=never      hard-pin the command-executing
//     transport off. Passed on the command
//     line it outranks any ambient/global
//     `protocol.ext.allow=always`, so it can
//     never be re-enabled out from under us.
//
// file:// and git:// fall through to protocol.allow=never and are denied.
// Because these are command-line `-c` overrides, they outrank repo/global
// config files — so a host with `protocol.file.allow=always` in .git/config
// or ~/.gitconfig will NOT re-enable file submodules here. That is
// intentional: the worktree content is untrusted, so we do not want ambient
// config to widen the transport surface. If a Kandev deployment ever needs
// local-path submodules for trusted repos, that must be a deliberate
// per-repo opt-in that appends `-c protocol.file.allow=always`, not an
// implicit reliance on host git config.
var hardenedSubmoduleConfig = []string{
	"protocol.allow=never",
	"protocol.https.allow=always",
	"protocol.ssh.allow=always",
	"protocol.ext.allow=never",
	// Neutralize attacker-planted hooks (CVE-2018-11235 / CVE-2024-32002
	// class): point core.hooksPath at an empty location so no hook in the
	// submodule's .git/modules tree can execute during checkout.
	"core.hooksPath=" + os.DevNull,
}

// newSubmoduleUpdateCmd builds the hardened `git submodule update --init
// --recursive` command for dir. It reuses newNonInteractiveGitCmd so the
// submodule fetch inherits the full non-interactive env set (GIT_SSH_COMMAND
// with BatchMode, SSH/GCM askpass suppression, GIT_TERMINAL_PROMPT=0) and
// WaitDelay — critical because an attacker `ssh://…` submodule URL would
// otherwise hang the review runner on host-key/passphrase prompts until the
// context deadline. Kept separate from initSubmodules so tests can assert the
// hardening flags/env are present at the sink.
func (m *Manager) newSubmoduleUpdateCmd(ctx context.Context, dir string) *exec.Cmd {
	args := make([]string, 0, len(hardenedSubmoduleConfig)*2+4)
	for _, c := range hardenedSubmoduleConfig {
		args = append(args, "-c", c)
	}
	args = append(args, "submodule", "update", "--init", "--recursive")
	return m.newNonInteractiveGitCmd(ctx, dir, args...)
}

// getSubmodulePaths returns the paths of all submodules registered in HEAD.
// It reads from the git object store (git ls-tree), so it works in --no-checkout worktrees.
// Returns nil (not an error) if there are no submodules.
func getSubmodulePaths(ctx context.Context, dir string) ([]string, error) {
	cmd := newGitCommand(ctx, "ls-tree", "-r", "HEAD")
	cmd.Dir = dir
	output, err := runGitCmdOutput(ctx, cmd)
	if err != nil {
		return nil, err
	}

	var paths []string
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		// Format: "<mode> <type> <hash>\t<path>"
		// Submodules have mode 160000 and type "commit".
		if strings.HasPrefix(line, "160000 ") {
			if _, path, ok := strings.Cut(line, "\t"); ok {
				paths = append(paths, path)
			}
		}
	}
	return paths, nil
}

// initSubmodules runs "git submodule update --init --recursive" in the given directory.
// Failures are non-fatal: submodule URLs may be unreachable (private repos,
// missing credentials), but the worktree is still usable for non-submodule files.
func (m *Manager) initSubmodules(ctx context.Context, dir string) {
	cmd := m.newSubmoduleUpdateCmd(ctx, dir)
	output, err := runGitCmdCombinedOutput(ctx, cmd)
	if err != nil {
		m.logger.Warn("git submodule update --init failed (non-fatal)",
			zap.String("dir", dir),
			zap.String("output", string(output)),
			zap.Error(err))
		return
	}
	m.logger.Debug("initialized submodules in worktree", zap.String("dir", dir))
}
