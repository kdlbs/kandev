package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/ssh"

	"github.com/kandev/kandev/internal/githubauth"
	"github.com/kandev/kandev/internal/task/models"
)

// materializeSSHRemoteContribution prepares the primary SSH checkout after
// agentctl is alive. The SSH task directory already contains session runtime
// files, so it cannot be cloned into as an empty directory. Initialising the
// repository in place keeps the workspace root stable while preserving the
// target as origin and using the same source-remote contract as other remote
// executors.
func (r *SSHExecutor) materializeSSHRemoteContribution(
	ctx context.Context,
	client *ssh.Client,
	agentctlBin, taskDir string,
	req *ExecutorCreateRequest,
	platform SSHRemotePlatform,
	binding *models.RemoteContribution,
) error {
	if binding == nil {
		return errors.New("ssh: remote contribution binding is required")
	}
	if err := binding.Validate(); err != nil {
		return fmt.Errorf("ssh: validate remote contribution: %w", err)
	}
	targetURL := getMetadataString(req.Metadata, "repository_clone_url")
	if targetURL == "" {
		return errors.New("ssh: remote contribution target has no clone URL")
	}

	env := sshRemoteContributionEnv(req, agentctlBin)
	envScript, err := buildSSHEnvInitScript(env)
	if err != nil {
		return fmt.Errorf("ssh: prepare remote contribution environment: %w", err)
	}

	shell := sshShellForRemote(req.Metadata, platform)
	inner := sshRemoteContributionScript(taskDir, targetURL, binding)
	wrapped := WrapLoginShell(shell, "set -ae; . /dev/stdin; set +a\n"+inner)
	_, _, runErr := runSSHCommandStdin(ctx, client, wrapped, strings.NewReader(envScript))
	if runErr != nil {
		// Git may include a credentialed helper URL in stderr. Keep the error
		// deliberately generic; provider and credential data never leave the
		// remote preparation boundary in an error response.
		return errors.New("ssh: remote contribution checkout failed")
	}
	r.report(req.OnProgress, "Preparing contribution checkout", PrepareStepCompleted, "")
	return nil
}

func sshRemoteContributionEnv(req *ExecutorCreateRequest, agentctlBin string) map[string]string {
	env := sshRemoteAgentEnv(req)
	if env == nil {
		env = make(map[string]string)
	}
	if req == nil {
		return env
	}
	count := sshRemoteGitConfigCount(env)
	managedBroker := hasManagedGitHubBrokerEnv(req.Env)
	foundManagedHelper := rewriteSSHManagedGitCredentialHelpers(env, count, managedBroker, agentctlBin)
	if managedBroker && !foundManagedHelper && agentctlBin != "" {
		count = appendSSHGitConfig(env, count, gitHubCredentialHelperConfigKey, "!"+agentctlBin+" git-credential")
	}
	if !managedBroker && (env[envKeyGHToken] != "" || env[envKeyGitHubToken] != "") && count < 64 {
		count = appendSSHGitConfig(env, count, gitHubCredentialHelperConfigKey, `!f() { echo "username=x-access-token"; echo "password=${GH_TOKEN:-${GITHUB_TOKEN}}"; }; f`)
	}
	if count > 0 {
		env["GIT_CONFIG_COUNT"] = strconv.Itoa(count)
	}
	return env
}

func sshRemoteGitConfigCount(env map[string]string) int {
	count, err := strconv.Atoi(env["GIT_CONFIG_COUNT"])
	if err != nil || count < 0 || count >= 64 {
		return 0
	}
	return count
}

func rewriteSSHManagedGitCredentialHelpers(env map[string]string, count int, managed bool, agentctlBin string) bool {
	found := false
	for index := 0; index < count; index++ {
		keySuffix := strconv.Itoa(index)
		key := strings.ToLower(strings.TrimSpace(env["GIT_CONFIG_KEY_"+keySuffix]))
		if !strings.HasPrefix(key, "credential.https://") || !strings.HasSuffix(key, ".helper") {
			continue
		}
		valueKey := "GIT_CONFIG_VALUE_" + keySuffix
		if !managed || !isManagedSSHGitCredentialHelper(env[valueKey]) {
			continue
		}
		found = true
		env[valueKey] = "!" + agentctlBin + " git-credential"
	}
	return found
}

func isManagedSSHGitCredentialHelper(value string) bool {
	switch value {
	case githubauth.ManagedGitCredentialHelper,
		githubauth.LegacyShimGitCredentialHelper,
		githubauth.LegacyGitCredentialHelper:
		return true
	default:
		return false
	}
}

func appendSSHGitConfig(env map[string]string, count int, key, value string) int {
	if count >= 64 {
		return count
	}
	suffix := strconv.Itoa(count)
	env["GIT_CONFIG_KEY_"+suffix] = key
	env["GIT_CONFIG_VALUE_"+suffix] = value
	return count + 1
}

func sshRemoteContributionScript(taskDir, targetURL string, binding *models.RemoteContribution) string {
	remoteName, remoteBranch, remoteRef, refspec, suffix := sshRemoteContributionScriptValues(binding)
	lines := append(sshRemoteContributionSetupLines(), sshRemoteContributionCheckoutLines()...)
	return fmt.Sprintf(strings.Join(lines, "\n"), shellQuote(taskDir), shellQuote(targetURL), shellQuote(remoteName), shellQuote(binding.SourceRepository.RemoteURL), shellQuote(binding.BaseBranch), shellQuote(remoteBranch), shellQuote(remoteRef), shellQuote(refspec), shellQuote(binding.HeadSHA), suffix, suffix)
}

func sshRemoteContributionSetupLines() []string {
	return []string{
		"set -eu",
		"workspace=%s",
		"target_url=%s",
		"contribution_remote=%s",
		"contribution_url=%s",
		"base_branch=%s",
		"contribution_branch=%s",
		"contribution_ref=%s",
		"contribution_refspec=%s",
		"expected_head=%s",
		"if git -C \"$workspace\" rev-parse --git-dir >/dev/null 2>&1; then",
		"  if configured_url=$(git -C \"$workspace\" config --get remote.origin.url 2>/dev/null); then",
		"    if [ \"$configured_url\" != \"$target_url\" ]; then",
		"      echo 'kandev: target origin identity conflict' >&2",
		"      exit 1",
		"    fi",
		"  else",
		"    git -C \"$workspace\" remote add origin \"$target_url\"",
		"  fi",
		"else",
		"  git init -q \"$workspace\"",
		"  git -C \"$workspace\" remote add origin \"$target_url\"",
		"fi",
		"exclude_file=$(git -C \"$workspace\" rev-parse --git-path info/exclude)",
		"mkdir -p \"$(dirname \"$exclude_file\")\"",
		"touch \"$exclude_file\"",
		"grep -Fqx '/.kandev/' \"$exclude_file\" || printf '%%s\\n' '/.kandev/' >>\"$exclude_file\"",
		"if ! git -C \"$workspace\" fetch --no-tags origin \"+refs/heads/$base_branch:refs/remotes/origin/$base_branch\" >/dev/null 2>&1; then",
		"  echo 'kandev: target base branch is unavailable' >&2",
		"  exit 1",
		"fi",
		"if configured_url=$(git -C \"$workspace\" config --get \"remote.$contribution_remote.url\" 2>/dev/null); then",
		"  if [ \"$configured_url\" != \"$contribution_url\" ]; then",
		"    echo 'kandev: contribution remote identity conflict' >&2",
		"    exit 1",
		"  fi",
		"else",
		"  git -C \"$workspace\" remote add \"$contribution_remote\" \"$contribution_url\"",
		"fi",
		"push_urls=$(git -C \"$workspace\" config --get-all \"remote.$contribution_remote.pushurl\" 2>/dev/null || true)",
		"if [ -n \"$push_urls\" ] && [ \"$push_urls\" != \"$contribution_url\" ]; then",
		"  echo 'kandev: contribution push identity conflict' >&2",
		"  exit 1",
		"fi",
		"if ! git -C \"$workspace\" fetch --no-tags \"$contribution_remote\" \"$contribution_refspec\" >/dev/null 2>&1; then",
		"  echo 'kandev: contribution source branch is unavailable' >&2",
		"  exit 1",
		"fi",
		"actual_head=$(git -C \"$workspace\" rev-parse --verify \"$contribution_ref\" 2>/dev/null || true)",
		"expected_head=$(printf '%%s' \"$expected_head\" | tr '[:upper:]' '[:lower:]')",
		"actual_head=$(printf '%%s' \"$actual_head\" | tr '[:upper:]' '[:lower:]')",
		"if [ -z \"$actual_head\" ] || [ \"$actual_head\" != \"$expected_head\" ]; then",
		"  echo 'kandev: contribution source head changed' >&2",
		"  exit 1",
		"fi",
	}
}

func sshRemoteContributionCheckoutLines() []string {
	return []string{
		"current_branch=$(git -C \"$workspace\" branch --show-current 2>/dev/null || true)",
		"current_upstream=$(git -C \"$workspace\" rev-parse --abbrev-ref --symbolic-full-name '@{upstream}' 2>/dev/null || true)",
		"checkout_branch=\"$contribution_branch\"",
		"if [ -n \"$current_branch\" ] && [ \"$current_upstream\" = \"$contribution_remote/$contribution_branch\" ] && git -C \"$workspace\" merge-base --is-ancestor \"$expected_head\" HEAD >/dev/null 2>&1; then",
		"  checkout_branch=\"$current_branch\"",
		"elif git -C \"$workspace\" show-ref --verify --quiet \"refs/heads/$checkout_branch\"; then",
		"  if ! git -C \"$workspace\" merge-base --is-ancestor \"$expected_head\" \"refs/heads/$checkout_branch\" >/dev/null 2>&1; then",
		"    checkout_branch=\"$contribution_branch-kandev-%s\"",
		"    suffix_number=0",
		"    while git -C \"$workspace\" show-ref --verify --quiet \"refs/heads/$checkout_branch\"; do",
		"      suffix_number=$((suffix_number + 1))",
		"      checkout_branch=\"$contribution_branch-kandev-%s-$suffix_number\"",
		"    done",
		"  fi",
		"fi",
		"if [ \"$current_branch\" != \"$checkout_branch\" ]; then",
		"  if git -C \"$workspace\" show-ref --verify --quiet \"refs/heads/$checkout_branch\"; then",
		"    git -C \"$workspace\" checkout \"$checkout_branch\" >/dev/null 2>&1",
		"  else",
		"    git -C \"$workspace\" checkout -b \"$checkout_branch\" \"$contribution_ref\" >/dev/null 2>&1",
		"  fi",
		"fi",
		"git -C \"$workspace\" branch --set-upstream-to=\"$contribution_remote/$contribution_branch\" \"$checkout_branch\" >/dev/null 2>&1",
	}
}

func sshRemoteContributionScriptValues(binding *models.RemoteContribution) (string, string, string, string, string) {
	remoteName := binding.ContributionRemoteName()
	remoteBranch := binding.HeadBranch
	remoteRef := "refs/remotes/" + remoteName + "/" + remoteBranch
	refspec := "+refs/heads/" + remoteBranch + ":" + remoteRef
	suffix := strings.TrimPrefix(remoteName, "contrib-")
	return remoteName, remoteBranch, remoteRef, refspec, suffix
}
