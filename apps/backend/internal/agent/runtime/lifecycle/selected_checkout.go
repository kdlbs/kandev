package lifecycle

import "fmt"

const (
	metadataCheckoutBranch   = "selected_checkout_branch"
	metadataCheckoutRef      = "selected_checkout_ref"
	metadataPreserveCheckout = "preserve_selected_checkout"
)

// Typed launch selection is authoritative over caller and profile metadata.
func setSelectedCheckoutMetadata(req *LaunchRequest, metadata map[string]interface{}) {
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
		ref = ""
	}
	metadata[metadataCheckoutRef] = ref
	metadata[metadataPreserveCheckout] = getMetadataString(req.Metadata, MetadataKeyWorktreeBranch) != ""
}

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
		prefix += "kandev_existing_checkout=$(git -C {{workspace.path}} rev-parse --verify HEAD 2>/dev/null || true)\n"
	}
	selection := "\nkandev_checkout_branch=" + shellQuote(branch) +
		"\nkandev_checkout_ref=" + shellQuote(getMetadataString(req.Metadata, metadataCheckoutRef)) + "\n"
	return prefix + script + selection + selectedCheckoutPostlude
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
