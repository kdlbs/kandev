---
status: current
system: executors
requirements:
  - REQ-EXECUTORS-REPOSITORY-BRANCH-001
  - REQ-EXECUTORS-REPOSITORY-BRANCH-002
---

# Repository branch resolution

## Purpose and boundaries

`internal/scriptengine.RepositoryProvider` owns the repository branch placeholder.
Sprites, SSH, Docker, Kubernetes, and local setup scripts use this provider.
The existing URL resolver reads `origin`; this correction does not introduce remote discovery.

## Requirement mapping

| Criteria | Design section |
| --- | --- |
| AC-EXECUTORS-REPOSITORY-BRANCH-001.1, .2 | Branch conversion |
| AC-EXECUTORS-REPOSITORY-BRANCH-001.3 | Preserved state |
| AC-EXECUTORS-REPOSITORY-BRANCH-001.4 | Script integration |
| AC-EXECUTORS-REPOSITORY-BRANCH-001.5 | Failure behavior |

## Branch conversion

The provider retains its current precedence: nonempty `base_branch`, then `repository_branch`.
A package-private pure helper converts that value before `shellQuote`.
It removes at most one recognized prefix, in this order:

1. `refs/heads/`
2. `refs/remotes/origin/`
3. `origin/`

All other values remain unchanged. Conversion does not remove arbitrary slash components or repeatedly remove prefixes.
Thus `refs/heads/origin/topic` becomes `origin/topic`, and `origin/origin/topic` also becomes `origin/topic`.
Empty values retain existing behavior. Invalid values do not acquire a default branch.

The worktree package has a similar private normalizer. This change does not import that package or broaden its behavior.
The script helper serves the existing placeholder contract and requires no new public API or architectural boundary.

## Preserved state

Conversion operates on a local string. It does not mutate the metadata map or task records.
`WorktreeProvider` continues to receive the original base reference and task branch.
Local checkout and diff operations retain their existing inputs.
Custom setup and cleanup scripts receive a branch name through `repository.branch`; raw worktree references remain available through `worktree.base_branch`.

## Script integration

`SpritesExecutor.resolvePrepareScript` combines `RepositoryProvider` and `WorktreeProvider` before it resolves scripts.
The same provider supplies the other executor script resolvers.
The correction applies to saved templates without a stored-script migration.
Current Sprites defaults use fetch; older saved scripts use `git clone --branch`.
Both paths require the converted branch name.

Normalization precedes shell quoting. Existing quoting and command-injection regression tests remain mandatory.
Tests must include a quoted branch value and prove that shell evaluation preserves literal data.

## Failure behavior

The change performs no network lookup and adds no retry.
Git retains authority to reject missing branches. The existing launch error path records that failure.
This design does not repair unrelated credential, provisioning, or agent-start failures.

## Validation

Provider tests cover precedence, the conversion table, unchanged metadata, and shell quoting.
A lifecycle integration test resolves a Sprites script and executes repository preparation against a temporary Git remote.
It covers the current fetch template and a saved clone template, then checks HEAD and the task branch.
A missing branch case proves that preparation does not select the default branch.
These tests use isolated directories and require no Sprites account or user workspace.

## Implementation plans

- [Remote clone branch correction](../../../plans/remote-clone-branch/plan.md)
- [Remote PR review checkout](../../../plans/remote-pr-review-checkout/plan.md)

## Selected remote checkout

This section extends preparation for fresh non-worktree remote environments.
It does not change host worktree checkout or the editable remote-contribution contract.
The earlier branch-placeholder correction remains implemented.

### Selection and propagation

For a fresh environment, `nonWorktreeTaskBranch` shall prefer `CheckoutBranch` over a generated task branch.
An existing `WorktreeBranch` remains authoritative for resume.
`buildLaunchMetadata` shall project typed launch checkout fields into executor metadata.
Caller metadata and executor profile defaults must not override the typed selection.
The projection derives an exact source ref from the typed PR number or checkout branch.
These transient keys are rebuilt at launch, not added to persisted executor metadata.
Keep `BaseBranch` separate for comparison and initial repository materialization.
This implements AC-002.1 and AC-002.5.

### Preparation

The managed checkout fragment shall distinguish explicit selection from generated-branch preparation.
All script-based remote executors must resolve this fragment consistently, including saved custom prepare scripts.
For a selected GitHub PR, fetch `refs/pull/<number>/head` from the base repository's `origin` into a private local ref.
Use a validated positive integer from the typed launch request, not interpolated caller text.
Create the selected local branch at the fetched commit, then verify that HEAD identifies that commit.
Do not fetch the fork branch from `origin` as a normal branch: it need not exist there.
For a selected branch without a PR number, fetch the exact origin head ref and check it out.
Branch names remain shell-quoted data and must pass Git ref validation.
This implements AC-002.1, AC-002.2, and AC-002.4.

Explicit checkout errors must escape the current best-effort generated-branch postlude.
No selected-checkout path may use its `|| true` fallback or create a branch from an unrelated HEAD.
Ensure completion precedes agent startup and that the existing launch failure path carries the error.
Existing remote workspaces must not be reset during resume.
For built-in remote prepare templates, materialize the selected checkout immediately before `repository.setup_script` so dependency installation and generated files use the selected revision.
The shared `withBranchCheckout` wrapper records whether HEAD exists before the prepare template runs, using an exact `safe.directory` override so retained workspaces owned by another UID are still detected.
A recorded branch plus a pre-existing checkout selects resume preservation.
A recorded branch without a pre-existing checkout still requires strict materialization for recreated compute.
If fresh preparation encounters conflicting branch state, fail without overwriting it.

### Read-only PR source

Reuse the PR-ref mechanism already used by host worktree preparation.
Do not introduce `RemoteContribution` merely to review a selected PR.
That separate contract includes edit permission, pinned contribution identity, and push routing.
This extension does not grant push rights, add a fork credential scope, or configure a fork push destination.
When a validated contribution binding is already present, retain its existing preparation path without competing checkout fragments.
This implements AC-002.3 and preserves the task system's existing contribution boundary.

### Regression coverage

Use a temporary bare base repository with different commits on `main` and `refs/pull/3527/head`.
Do not create the fork's named branch in that repository.
Pass a real typed launch request through metadata construction and the Sprites script resolver.
Execute both current and saved clone-based prepare templates, then assert branch, HEAD, and unchanged comparison base.
Also cover same-repository PRs, plain explicit branches, missing PR refs, conflicting local branch state, shell metacharacters, no selection, and resume.
Shared resolver tests must cover Docker, SSH, and Kubernetes so selection propagation does not diverge by executor.
Abbreviated AC references in this section belong to `AC-EXECUTORS-REPOSITORY-BRANCH-002`.
