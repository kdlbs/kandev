package lifecycle

import (
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/githubauth"
)

const (
	metadataCheckoutBranch   = "selected_checkout_branch"
	metadataCheckoutRef      = "selected_checkout_ref"
	metadataPreserveCheckout = "preserve_selected_checkout"
	selectedCheckoutMarker   = "KANDEV_SELECTED_PR_CHECKOUT"
)

// Typed launch selection is authoritative over caller and profile metadata.
func setSelectedCheckoutMetadata(req *LaunchRequest, metadata map[string]interface{}) {
	previousBranch := getMetadataString(metadata, metadataCheckoutBranch)
	previousRef := getMetadataString(metadata, metadataCheckoutRef)
	delete(metadata, metadataCheckoutBranch)
	delete(metadata, metadataCheckoutRef)
	delete(metadata, metadataPreserveCheckout)
	if req.CheckoutBranch == "" {
		return
	}
	metadata[metadataCheckoutBranch] = req.CheckoutBranch
	ref := "refs/heads/" + req.CheckoutBranch
	if req.PRNumber > 0 {
		ref = fmt.Sprintf("refs/pull/%d/head", req.PRNumber)
	} else if req.PRNumber < 0 {
		// Negative PR number is invalid caller input; an empty ref fails closed.
		ref = ""
	}
	metadata[metadataCheckoutRef] = ref
	preserve := getMetadataString(req.Metadata, MetadataKeyWorktreeBranch) != ""
	// A retained remote workspace is safe to keep only while the explicit
	// selection is unchanged. Persisted selection metadata lets a PR update (or
	// a switch from a branch to a PR) force strict materialization on resume,
	// while a user-renamed branch still remains resumable.
	if previousBranch != "" && previousBranch != req.CheckoutBranch {
		preserve = false
	}
	if previousRef != "" && previousRef != ref {
		preserve = false
	}
	metadata[metadataPreserveCheckout] = preserve
}

func selectedCheckoutIsPullRequest(metadata map[string]interface{}) bool {
	return strings.HasPrefix(getMetadataString(metadata, metadataCheckoutRef), "refs/pull/")
}

// selectedCheckoutAgentEnv removes credentials from the long-lived agent
// process for fork PRs. The prepare process may need them for the trusted
// clone/fetch phase, but agent code runs from the selected ref and must not
// inherit the provider credential or broker lease.
func selectedCheckoutAgentEnv(env map[string]string, metadata map[string]interface{}) map[string]string {
	result := cloneStringMap(env)
	if !selectedCheckoutIsPullRequest(metadata) {
		return result
	}
	for _, key := range append([]string{"GITHUB_TOKEN", "GH_TOKEN", githubauth.CredentialHelperPathEnv, githubauth.CredentialCLIShimDirEnv, githubauth.CredentialCLIBashEnvEnv, githubauth.CredentialParentBashEnv}, managedGitCredentialBrokerEnvKeys...) {
		delete(result, key)
	}
	return result
}

func selectedCheckoutCredentialScrubScript(metadata map[string]interface{}) string {
	if !selectedCheckoutIsPullRequest(metadata) {
		return ""
	}
	return selectedCheckoutCredentialScrubCommands
}

const selectedCheckoutCredentialScrubCommands = `unset GITHUB_TOKEN GH_TOKEN KANDEV_GITHUB_CREDENTIAL_BROKER_URL KANDEV_GITHUB_CREDENTIAL_LEASE KANDEV_GITHUB_CREDENTIAL_REISSUE_CAPABILITY KANDEV_GITHUB_CREDENTIAL_TASK_ID KANDEV_GITHUB_CREDENTIAL_SESSION_ID KANDEV_GITHUB_CREDENTIAL_REPOSITORY_ID KANDEV_GITHUB_CREDENTIAL_OWNER KANDEV_GITHUB_CREDENTIAL_REPO KANDEV_GITHUB_CREDENTIAL_HOST KANDEV_GITHUB_CREDENTIAL_SCOPES KANDEV_GITHUB_CREDENTIAL_HELPER_PATH KANDEV_GITHUB_CLI_SHIM_DIR KANDEV_GITHUB_CLI_BASH_ENV KANDEV_GITHUB_PARENT_BASH_ENV`

// withBranchCheckout keeps explicit selection strict and contribution checkout independent.
func withBranchCheckout(req *ExecutorCreateRequest, script string) string {
	if _, ok := req.RemoteContributions[""]; ok {
		return script
	}
	branch := getMetadataString(req.Metadata, metadataCheckoutBranch)
	if branch == "" {
		return script + KandevBranchCheckoutPostlude()
	}
	// Capture reuse before the prepare template initializes a new repository.
	preserve, _ := req.Metadata[metadataPreserveCheckout].(bool)
	prefix := "\nkandev_existing_checkout=''\n"
	if preserve {
		prefix += "kandev_existing_checkout=$(git -c safe.directory={{workspace.path}} -C {{workspace.path}} rev-parse --verify HEAD 2>/dev/null || true)\n"
	}
	selection := "\nkandev_checkout_branch=" + shellQuote(branch) +
		"\nkandev_checkout_ref=" + shellQuote(getMetadataString(req.Metadata, metadataCheckoutRef)) + "\n"
	prepared := prefix + selection + script
	if scrub := selectedCheckoutCredentialScrubScript(req.Metadata); scrub != "" {
		prepared = strings.Replace(prepared, "{{kandev.agentctl.start}}", scrub+"\n{{kandev.agentctl.start}}", 1)
	}
	if strings.Contains(prepared, "{{repository.setup_script}}") {
		replacement := selectedCheckoutPostlude + "\n"
		if scrub := selectedCheckoutCredentialScrubScript(req.Metadata); scrub != "" {
			replacement += scrub + "\n"
		}
		replacement += "{{repository.setup_script}}"
		return strings.Replace(prepared, "{{repository.setup_script}}", replacement, 1)
	}
	return prepared + selectedCheckoutPostlude
}

const selectedCheckoutPostlude = `
# ---- kandev-managed: materialize explicit checkout ----
if [ -z "$kandev_existing_checkout" ]; then
  (
    set -eu
    cd {{workspace.path}}
    git check-ref-format --branch "$kandev_checkout_branch" >/dev/null
    git check-ref-format "$kandev_checkout_ref" >/dev/null
    if [ "$(git rev-parse --is-shallow-repository)" = true ]; then
      git fetch --unshallow --no-tags origin
    fi
    git fetch --no-tags origin "+${kandev_checkout_ref}:refs/kandev/selected-checkout"
    selected_head=$(git rev-parse --verify 'refs/kandev/selected-checkout^{commit}')
    if git show-ref --verify --quiet "refs/heads/$kandev_checkout_branch"; then
      local_head=$(git rev-parse --verify "refs/heads/$kandev_checkout_branch")
      if [ "$local_head" != "$selected_head" ]; then
        echo 'kandev: selected checkout conflicts with an existing local branch' >&2
        exit 1
      fi
      git checkout "$kandev_checkout_branch"
    else
      git checkout --no-track -b "$kandev_checkout_branch" "$selected_head"
    fi
    test "$(git rev-parse HEAD)" = "$selected_head"
  )
  kandev_checkout_status=$?
  if [ "$kandev_checkout_status" -ne 0 ]; then
    echo 'kandev: selected checkout failed' >&2
    exit "$kandev_checkout_status"
  fi
fi
`
