package lifecycle

import "strings"

// withWindowsBranchCheckout is the PowerShell form of withBranchCheckout. An
// explicit selection is materialized strictly before repository setup; a task
// without one keeps the best-effort generated-branch postlude.
func withWindowsBranchCheckout(req *ExecutorCreateRequest, script string) string {
	if req == nil {
		return script
	}
	if _, ok := req.RemoteContributions[""]; ok {
		return script
	}
	branch := getMetadataString(req.Metadata, metadataCheckoutBranch)
	if branch == "" {
		return script + "\n" + windowsBranchCheckoutPostlude()
	}
	// Capture reuse before the prepare template initializes a new repository.
	prefix := "$__kExistingCheckout = ''\n"
	if preserve, _ := req.Metadata[metadataPreserveCheckout].(bool); preserve {
		prefix += windowsExistingCheckoutProbe + "\n"
	}
	selection := "$__kCheckoutBranch = " + powerShellQuote(branch) + "\n" +
		"$__kCheckoutRef = " + powerShellQuote(getMetadataString(req.Metadata, metadataCheckoutRef)) + "\n"
	prepared := prefix + selection + script
	if !strings.Contains(prepared, "{{repository.setup_script}}") {
		return prepared + "\n" + windowsSelectedCheckoutPostlude
	}
	replacement := windowsSelectedCheckoutPostlude + "\n"
	if scrub := windowsSelectedCheckoutCredentialScrub(req.Metadata); scrub != "" {
		replacement += scrub + "\n"
	}
	return strings.Replace(prepared, "{{repository.setup_script}}", replacement+"{{repository.setup_script}}", 1)
}

// windowsSelectedCheckoutCredentialScrub removes the provider credential from
// the prepare process before repository setup runs code from a fork PR. It is
// the last statement when the setup script is empty, so it must leave $? true
// for an absent variable; Set-Item with a null value does.
func windowsSelectedCheckoutCredentialScrub(metadata map[string]interface{}) string {
	if !selectedCheckoutIsPullRequest(metadata) {
		return ""
	}
	names := strings.Fields(strings.TrimPrefix(selectedCheckoutCredentialScrubCommands, "unset "))
	quoted := make([]string, len(names))
	for i, name := range names {
		quoted[i] = powerShellQuote("Env:" + name)
	}
	return "Set-Item -LiteralPath " + strings.Join(quoted, ", ") + " -Value $null"
}

const windowsExistingCheckoutProbe = `$__kExistingCheckout = (Invoke-KandevNative { & git -c ('safe.directory=' + ({{workspace.path}} -replace '\\', '/')) -C {{workspace.path}} rev-parse --verify HEAD 2>$null } | Out-String).Trim()`

const windowsSelectedCheckoutPostlude = `
# ---- kandev-managed: materialize explicit checkout ----
if ([string]::IsNullOrEmpty($__kExistingCheckout)) {
  $__kSelectedWorkspace = {{workspace.path}}
  try {
    Invoke-KandevNative { & git -C $__kSelectedWorkspace check-ref-format --branch $__kCheckoutBranch *>$null }
    if ($LASTEXITCODE -ne 0) { throw 'kandev: selected checkout branch is invalid' }
    Invoke-KandevNative { & git -C $__kSelectedWorkspace check-ref-format $__kCheckoutRef *>$null }
    if ($LASTEXITCODE -ne 0) { throw 'kandev: selected checkout ref is invalid' }
    $__kSelectedShallow = (Invoke-KandevNative { & git -C $__kSelectedWorkspace rev-parse --is-shallow-repository 2>$null } | Out-String).Trim()
    if ($__kSelectedShallow -eq 'true') {
      Invoke-KandevNative { & git -C $__kSelectedWorkspace fetch --unshallow --no-tags origin *>$null }
      if ($LASTEXITCODE -ne 0) { throw 'kandev: selected checkout history is unavailable' }
    }
    Invoke-KandevNative { & git -C $__kSelectedWorkspace fetch --no-tags origin ('+' + $__kCheckoutRef + ':refs/kandev/selected-checkout') *>$null }
    if ($LASTEXITCODE -ne 0) { throw 'kandev: selected checkout ref is unavailable' }
    $__kSelectedHead = (Invoke-KandevNative { & git -C $__kSelectedWorkspace rev-parse --verify 'refs/kandev/selected-checkout^{commit}' 2>$null } | Out-String).Trim()
    if (-not $__kSelectedHead) { throw 'kandev: selected checkout has no commit' }
    Invoke-KandevNative { & git -C $__kSelectedWorkspace show-ref --verify --quiet ('refs/heads/' + $__kCheckoutBranch) *>$null }
    if ($LASTEXITCODE -eq 0) {
      $__kLocalHead = (Invoke-KandevNative { & git -C $__kSelectedWorkspace rev-parse --verify ('refs/heads/' + $__kCheckoutBranch) 2>$null } | Out-String).Trim()
      if ($__kLocalHead -ne $__kSelectedHead) { throw 'kandev: selected checkout conflicts with an existing local branch' }
      Invoke-KandevNative { & git -C $__kSelectedWorkspace checkout $__kCheckoutBranch *>$null }
    } else {
      Invoke-KandevNative { & git -C $__kSelectedWorkspace checkout --no-track -b $__kCheckoutBranch $__kSelectedHead *>$null }
    }
    if ($LASTEXITCODE -ne 0) { throw 'kandev: selected checkout could not switch branches' }
    $__kCheckedOutHead = (Invoke-KandevNative { & git -C $__kSelectedWorkspace rev-parse HEAD 2>$null } | Out-String).Trim()
    if ($__kCheckedOutHead -ne $__kSelectedHead) { throw 'kandev: selected checkout HEAD does not match the selection' }
  } catch {
    [Console]::Error.WriteLine($_.Exception.Message)
    [Console]::Error.WriteLine('kandev: selected checkout failed')
    exit 1
  }
}
`
