package scriptengine

import (
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/task/models"
)

// RemoteContributionSetupScript returns the kandev-owned checkout fragment
// used by clone-based executors. The target repository is cloned by the
// normal prepare script; this fragment adds a separate source remote, fetches
// the exact provider head, and checks out a collision-safe local branch.
//
// All binding values have already passed the credential-free domain validator,
// but they are still shell-quoted here because branch names are provider data.
// The fragment intentionally does not print the remote URL or provider text.
func RemoteContributionSetupScript(binding *models.RemoteContribution) (string, error) {
	if binding == nil {
		return "", nil
	}
	if err := binding.Validate(); err != nil {
		return "", fmt.Errorf("validate remote contribution: %w", err)
	}
	remoteName := binding.ContributionRemoteName()
	remoteBranch := binding.HeadBranch
	remoteRef := "refs/remotes/" + remoteName + "/" + remoteBranch
	refspec := "+refs/heads/" + remoteBranch + ":" + remoteRef
	branchSuffix := strings.TrimPrefix(remoteName, "contrib-")

	return fmt.Sprintf(`

# ---- kandev-managed: materialize existing remote contribution ----
# The target repository remains origin. The source fork is a separate,
# binding-derived remote and the fetched head is verified before checkout.
(
  set -eu
  cd %s
  contribution_remote=%s
  contribution_url=%s
  contribution_branch=%s
  contribution_ref=%s
  contribution_refspec=%s
  expected_head=%s
  if configured_url=$(git config --get "remote.$contribution_remote.url" 2>/dev/null); then
    if [ "$configured_url" != "$contribution_url" ]; then
      echo 'kandev: contribution remote identity conflict' >&2
      exit 1
    fi
  else
    git remote add "$contribution_remote" "$contribution_url"
  fi
  if ! git fetch --no-tags "$contribution_remote" "$contribution_refspec" >/dev/null 2>&1; then
    echo 'kandev: contribution source branch is unavailable' >&2
    exit 1
  fi
  actual_head=$(git rev-parse --verify "$contribution_ref" 2>/dev/null || true)
  expected_head=$(printf '%%s' "$expected_head" | tr '[:upper:]' '[:lower:]')
  actual_head=$(printf '%%s' "$actual_head" | tr '[:upper:]' '[:lower:]')
  if [ -z "$actual_head" ] || [ "$actual_head" != "$expected_head" ]; then
    echo 'kandev: contribution source head changed' >&2
    exit 1
  fi
  checkout_branch="$contribution_branch"
  if git show-ref --verify --quiet "refs/heads/$checkout_branch"; then
    checkout_branch="$contribution_branch-kandev-%s"
    suffix=0
    while git show-ref --verify --quiet "refs/heads/$checkout_branch"; do
      suffix=$((suffix + 1))
      checkout_branch="$contribution_branch-kandev-%s-$suffix"
    done
  fi
  git checkout -b "$checkout_branch" "$contribution_ref"
  git branch --set-upstream-to="$contribution_remote/$contribution_branch" "$checkout_branch"
)
`, shellQuote("/workspace"), shellQuote(remoteName), shellQuote(binding.SourceRepository.RemoteURL),
		shellQuote(remoteBranch), shellQuote(remoteRef), shellQuote(refspec), shellQuote(binding.HeadSHA),
		branchSuffix, branchSuffix), nil
}

// ContributionDestinationSetupScript configures a server-bound fork as the
// push remote for the current branch. It never changes origin or the branch's
// upstream remote, so fetch and pull continue to use the canonical checkout.
func ContributionDestinationSetupScript(destination *models.ContributionDestination) (string, error) {
	return ContributionDestinationSetupScriptAt(destination, "/workspace")
}

// ContributionDestinationSetupScriptAt is the workspace-path variant used by
// host and SSH preparers whose checkout root is not /workspace.
func ContributionDestinationSetupScriptAt(destination *models.ContributionDestination, workspacePath string) (string, error) {
	if destination == nil {
		return "", nil
	}
	if err := destination.Validate(); err != nil {
		return "", fmt.Errorf("validate contribution destination: %w", err)
	}
	remoteName := destination.ContributionRemoteName()
	return fmt.Sprintf(`

# ---- kandev-managed: configure contribution destination ----
# origin and the branch upstream remain canonical; only ordinary pushes use
# this exact, server-verified destination repository.
(
  set -eu
  cd %s
  destination_remote=%s
  destination_url=%s
  if configured_url=$(git config --get "remote.$destination_remote.url" 2>/dev/null); then
    if [ "$configured_url" != "$destination_url" ]; then
      echo 'kandev: contribution destination identity conflict' >&2
      exit 1
    fi
  else
    git remote add "$destination_remote" "$destination_url"
  fi
  configured_push_urls=$(git config --get-all "remote.$destination_remote.pushurl" 2>/dev/null || true)
  if [ -n "$configured_push_urls" ]; then
    while IFS= read -r configured_push_url; do
      if [ "$configured_push_url" != "$destination_url" ]; then
        echo 'kandev: contribution destination push identity conflict' >&2
        exit 1
      fi
    done <<EOF
$configured_push_urls
EOF
  else
    git config --add "remote.$destination_remote.pushurl" "$destination_url"
  fi
  current_branch=$(git branch --show-current)
  if [ -n "$current_branch" ]; then
    git config "branch.$current_branch.pushRemote" "$destination_remote"
  fi
)
	`, shellQuote(workspacePath), shellQuote(remoteName), shellQuote(destination.TargetRepository.RemoteURL)), nil
}

// RepositoryProvider returns git-related placeholders from metadata and environment.
// Parameters:
//   - metadata: executor create request metadata (contains "repository_path", "base_branch", etc.)
//   - env: environment variables (contains "GITHUB_TOKEN", etc.)
//   - repoURLResolver: resolves a local repo path to its remote URL (e.g., `git remote get-url origin`)
//   - tokenInjector: injects auth token into a clone URL
func RepositoryProvider(
	metadata map[string]any,
	env map[string]string,
	repoURLResolver func(string) (string, error),
	tokenInjector func(string, map[string]string) string,
) PlaceholderProvider {
	return func() map[string]string {
		vars := make(map[string]string)

		// SECURITY: repository.path/name/branch/clone_url/ssh_url are DATA that
		// land in shell text (git clone args, etc.). shellQuote emits a fully
		// single-quoted, self-contained shell token so a hostile value (e.g. a
		// fork PR branch named "$(...)") is a literal string even when a
		// template (including an OLD stored prepare_script) references the
		// placeholder BARE, e.g. `--branch {{repository.branch}}`.
		repoPath := getMetaString(metadata, "repository_path")
		if repoPath != "" {
			vars["repository.path"] = shellQuote(repoPath)
			vars["repository.name"] = shellQuote(repoNameFromPath(repoPath))
		}

		branch := getMetaString(metadata, "base_branch")
		if branch == "" {
			branch = getMetaString(metadata, "repository_branch")
		}
		vars["repository.branch"] = shellQuote(branch)

		// repository.setup_script is a script FRAGMENT (intentional multi-line
		// shell), not data — do NOT quote it.
		setupScript := getMetaString(metadata, "repository_setup_script")
		vars["repository.setup_script"] = setupScript

		// Clone URL: prefer explicit metadata, fall back to resolving from local repo
		cloneURL := getMetaString(metadata, "repository_clone_url")
		if cloneURL == "" && repoPath != "" && repoURLResolver != nil {
			if remoteURL, err := repoURLResolver(repoPath); err == nil && remoteURL != "" {
				vars["repository.ssh_url"] = shellQuote(remoteURL)
				cloneURL = remoteURL
			}
		}
		if cloneURL != "" {
			vars["repository.clone_url"] = shellQuote(injectToken(cloneURL, env, tokenInjector))
		}

		return vars
	}
}

// AgentctlProvider returns kandev agentctl-related placeholders.
func AgentctlProvider(agentctlPort int, workspacePath string) PlaceholderProvider {
	return func() map[string]string {
		portStr := fmt.Sprintf("%d", agentctlPort)
		return map[string]string{
			"kandev.agentctl.port":    portStr,
			"kandev.agentctl.install": "chmod +x /usr/local/bin/agentctl",
			"kandev.agentctl.start": fmt.Sprintf(
				"nohup agentctl --port %s --workdir %s > /tmp/agentctl.log 2>&1 &\nsleep 1",
				portStr, workspacePath,
			),
		}
	}
}

// WorkspaceProvider returns workspace path placeholder.
//
// SECURITY: workspace.path is DATA in shell text; shellQuote emits a
// self-contained single-quoted token so it stays a literal even if it ever
// carries metacharacters and even when referenced bare in a template.
func WorkspaceProvider(workspacePath string) PlaceholderProvider {
	return func() map[string]string {
		return map[string]string{
			"workspace.path": shellQuote(workspacePath),
		}
	}
}

// WorktreeProvider returns placeholders that describe the selected worktree context.
//
// SECURITY: worktree paths and the (untrusted, possibly fork-PR-controlled)
// branch names are DATA that land in shell text. shellQuote emits a
// self-contained single-quoted token so a hostile branch like "$(touch pwned)"
// cannot inject commands even when a stored/legacy template references the
// placeholder bare. worktree.id is a kandev-generated UUID (not shell data),
// left as-is.
func WorktreeProvider(basePath, path, id, branch, baseBranch string) PlaceholderProvider {
	return func() map[string]string {
		return map[string]string{
			"worktree.base_path":   shellQuote(basePath),
			"worktree.path":        shellQuote(path),
			"worktree.id":          id,
			"worktree.branch":      shellQuote(branch),
			"worktree.base_branch": shellQuote(baseBranch),
		}
	}
}

// GitIdentityProvider returns placeholders for git identity setup in remote executors.
func GitIdentityProvider(metadata map[string]any) PlaceholderProvider {
	return func() map[string]string {
		name := getMetaString(metadata, "git_user_name")
		email := getMetaString(metadata, "git_user_email")

		vars := map[string]string{
			"git.user_name":      name,
			"git.user_email":     email,
			"git.identity_setup": "",
		}
		if name == "" || email == "" {
			return vars
		}

		lines := []string{
			fmt.Sprintf("git config --global user.name '%s'", shellSingleQuote(name)),
			fmt.Sprintf("git config --global user.email '%s'", shellSingleQuote(email)),
		}
		vars["git.identity_setup"] = strings.Join(lines, "\n")
		return vars
	}
}

// injectToken applies token injection to a URL if an injector is provided.
func injectToken(url string, env map[string]string, injector func(string, map[string]string) string) string {
	if injector != nil {
		return injector(url, env)
	}
	return url
}

// getMetaString extracts a string value from a metadata map.
func getMetaString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	if v, ok := metadata[key].(string); ok {
		return v
	}
	return ""
}

// repoNameFromPath extracts the repository name from a file path.
func repoNameFromPath(repoPath string) string {
	if repoPath == "" {
		return ""
	}
	// Find last path component
	for i := len(repoPath) - 1; i >= 0; i-- {
		if repoPath[i] == '/' {
			return repoPath[i+1:]
		}
	}
	return repoPath
}

// AgentInstallProvider returns a placeholder with combined install scripts for multiple agents.
func AgentInstallProvider(installScripts []string) PlaceholderProvider {
	return func() map[string]string {
		seen := map[string]bool{}
		var lines []string
		for _, s := range installScripts {
			s = strings.TrimSpace(s)
			if s != "" && !seen[s] {
				seen[s] = true
				lines = append(lines, s)
			}
		}
		return map[string]string{
			"kandev.agents.install": strings.Join(lines, "\n"),
		}
	}
}

// shellSingleQuote escapes embedded single quotes for use INSIDE an existing
// pair of single quotes supplied by the caller/template (e.g. '%s'). It does
// NOT add the surrounding quotes.
func shellSingleQuote(value string) string {
	return strings.ReplaceAll(value, "'", `'"'"'`)
}

// shellQuote returns value as a complete, self-contained single-quoted shell
// token (surrounding quotes included, embedded quotes escaped). The result is
// safe to drop into a script UNQUOTED — `--branch <shellQuote(v)>` cannot be
// broken out of by shell metacharacters ($(), backticks, ;, |, whitespace).
//
// Prefer this over shellSingleQuote for any DATA placeholder, because stored
// prepare_script templates (snapshotted before a template was hardened) may
// reference the placeholder bare, so the value itself must carry its quoting.
func shellQuote(value string) string {
	return "'" + shellSingleQuote(value) + "'"
}

// GitHubAuthProvider returns placeholders for GitHub authentication setup.
// It configures both the gh CLI and git credential helper to use the GitHub token.
func GitHubAuthProvider(env map[string]string) PlaceholderProvider {
	return func() map[string]string {
		vars := map[string]string{
			"github.auth_setup": "",
		}

		// Check for GitHub token in env (either GH_TOKEN or GITHUB_TOKEN)
		token := env["GH_TOKEN"]
		if token == "" {
			token = env["GITHUB_TOKEN"]
		}
		if token == "" {
			// No token - output fallback commands that may work if gh is pre-authenticated
			vars["github.auth_setup"] = `# No GitHub token configured - trying fallback methods
gh auth setup-git 2>/dev/null || true
gh config set git_protocol https --host github.com 2>/dev/null || true`
			return vars
		}

		// Build auth setup script that configures both gh CLI and git
		// Use /bin/sh (not /bin/bash) for Alpine compatibility
		// Use ${GH_TOKEN:-${GITHUB_TOKEN}} to support either env var being set
		lines := []string{
			"# GitHub token authentication",
			"# Configure git credential helper for GitHub HTTPS authentication",
			// --replace-all is required, not cosmetic. `gh auth setup-git` (run
			// below, and by the no-token branch above) leaves the key
			// multi-valued: an empty entry that resets the helper chain plus
			// gh's own helper. A plain `git config --global <key> <value>`
			// refuses to overwrite a multi-valued key and exits 5, which aborts
			// the whole prepare script under `set -euo pipefail`. That makes the
			// no-token branch poison the with-token branch: on a host whose home
			// directory persists between runs (SSH, unlike an ephemeral Docker
			// container) exactly one launch succeeds and every later one fails.
			`git config --global --replace-all credential.https://github.com.helper '!/bin/sh -c "echo username=x-access-token; echo password=${GH_TOKEN:-${GITHUB_TOKEN}}"'`,
			"# Configure gh CLI to use HTTPS protocol",
			"gh config set git_protocol https --host github.com 2>/dev/null || true",
			"# Register gh as git credential helper (backup method)",
			"gh auth setup-git 2>/dev/null || true",
		}
		vars["github.auth_setup"] = strings.Join(lines, "\n")
		return vars
	}
}
