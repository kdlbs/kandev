---
status: current
system: workspaces
requirements:
  - REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001
---

# Worktree base selection

## Purpose and boundaries

The workspace system owns the source ref that creates a task worktree.
This design extends [base refresh](worktree-base-refresh.md) with substitution
eligibility. The task system supplies selection intent and retains launch-error
ownership. Provider credentials and remote-executor scripts keep their current
contracts.

This amendment is implemented. It preserves the same-ref local refresh behavior
from the existing design.

## Requirement mapping

| Criteria | Design section |
| --- | --- |
| AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.6, .25 | Selection contract and resolution gate |
| AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.26 | Failure and retry |
| AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.27 | Producers, persistence, and compatibility |
| AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.8, .28 | Runtime propagation and validation |
| AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.1, .3, .10, .13, .19 | Compatibility |

## Selection contract

The following identifiers are proposed additions. Define a versioned
`BaseBranchSelection` value in `internal/task/models`, stored under
`task_repositories.metadata.base_branch_selection`. It contains `version: 1`,
the exact effective `branch`, and `source` with three supported values:

| Source | Meaning | Missing-base substitution |
| --- | --- | --- |
| `explicit` | A supplied base or a selected branch-policy snapshot | Forbidden |
| `inherited` | Server-selected parent/current-task repository inheritance | Existing repository-default recovery |
| `repository_default` | The caller omitted the base and the server selected a default | Existing repository-default recovery |

Use a typed internal `BaseBranchFallbackPolicy` for preparation. Its zero value
is strict. Its sole permissive value is `repository_default`.
Derive permission only from supported server-authored provenance bound to the
current attachment base. Missing, unknown, malformed, or mismatched metadata
returns strict. The existing `HasManualBaseBranchOverride` marker always wins.
An explicit qualified PR base remains strict regardless of the derived policy.

Branch equality validates the record's binding. It never proves selection
intent. In particular, equality with `repositories.default_branch`, a parent
branch, or a session branch grants no permission.

An `explicit` record may defer its branch text while a PR-linked target or a
selected policy resolves the effective base at launch. The empty binding still
grants no substitution. An omitted non-PR base records `repository_default`.

## Producers and persistence

Add an internal-only selection-source field to `service.TaskRepositoryInput`
with `json:"-"`. Public transports cannot request permissive recovery.
Ordinary non-empty inputs default to explicit. Unknown internal values also
resolve strictly. Capture omission before helpers fill defaults or normalize
provider inputs.

`buildResolvedTaskRepositoryRow` records the source against the final branch,
after branch-policy resolution. A selected branch policy is strict. An omitted
branch that still awaits clone-time default discovery remains distinguishable
from a supplied non-empty branch.

These existing producers need explicit treatment:

- `Service.inheritParentRepositories` and MCP `inheritedRepoInputs` author
  inherited intent for each new attachment. They retain existing checkout and
  hierarchy behavior.
- MCP `explicitRepoInputsWithDefaults` retains whether the original entry
  omitted its base before filling the repository default.
- `Service.AddBranchToTask` retains the original supplied-versus-default
  distinction before reference resolution. Explicit workspace-source bases
  remain strict through their existing constructor.
- Automatic project inputs with no base use default intent. Saved repository
  set bases and policy snapshots remain configured selections and are strict.
- Server conversions in `backendapp/repository_selection.go` preserve trusted
  source intent. Public input adapters do not accept it from JSON.

The existing metadata serialization stores the record. No column, backfill, or
database migration is required. Reopening the database must preserve provenance.
Do not infer old provenance from parent IDs or current repository defaults.

An explicit base edit records strict intent even when the branch text is
unchanged. Perform this with the existing locked metadata write in
`UpdateTaskRepositoryBaseBranchAndClearComparisonTarget`. Its manual override
marker remains an independent provider-comparison contract.
Ordinary task updates that omit repository replacement preserve the record.
A supplied replacement list derives fresh selection intent.

System updates never create permissive provenance from missing or explicit
intent. A successful permitted fallback can rebind existing permissive intent
to its resolved default inside the existing locked row update. A changed or
ambiguous binding becomes strict. Preserve unrelated metadata and existing
comparison-target checks. This design does not change provider-retarget or
prepared-start write ordering from issue #4477.

## Runtime propagation

Resolve policy alongside the exact `TaskRepositoryID` in executor `repoInfo`.
When an unset attachment base actually resolves from a repository default,
that producer can supply default intent. A legacy non-empty session override
without matching provenance stays strict. Never recover policy from another
attachment with the same `RepositoryID`.

Carry the typed field through `LaunchAgentRequest`, `RepoSpec`, the lifecycle
adapter, `LaunchRequest`, and `RepoLaunchSpec`.
Continue through `WorkspaceRepositorySpec`, `EnvPrepareRequest`,
`RepoPrepareSpec`, and `worktree.CreateRequest`.
Audit single-repository, multi-repository, prepared-start, workspace-only,
reconciliation, and branch-replacement copies. In particular, `prepareOneRepo`
must overwrite the copied top-level policy with that attachment's policy.
Direct materialization in `backendapp/branch_materializer.go` derives policy
from its loaded attachment instead of unconditionally authorizing fallback.

Valid worktree reuse remains earlier than materialization. Do not add remote
queries to reuse. A checkout's recorded branch can already provide its source.
When replacement needs a new base, use the
same resolution gate before creating a replacement branch.

## Resolution gate

Keep `FallbackBaseBranch` as a candidate name. It cannot authorize substitution.
`CreateRequest.Validate` can retain the existing no-selection/default fill.
It must not grant permissive policy merely because a fallback string exists.

`resolveBaseRefWithFallback` must enforce policy at every substitution site:

1. Keep qualified target materialization first and strict.
2. Refresh only through the existing selected route and refresh setting.
3. Pass no substitute candidate to `resolveRefreshedBaseRefWithFallback` or
   `pullBaseBranchWithFallback` for a strict selection.
4. Resolve and verify the selected ref. Strictness does not forbid the existing
   local-ref fallback for that same selected branch after refresh failure.
5. If the selected ref is unavailable, stop a strict request before consulting
   the configured fallback or `ResolveRemoteDefaultBranch`.
6. For permitted inherited/default recovery, retain existing refresh,
   verification, warning, and cancellation rules.

Clearing `FallbackBaseBranch` alone is insufficient: the current resolver can
still query and select the live remote default. The policy must guard that path
as well as early refreshed-ref and PR fallback helpers.

## Failure and retry

For an unresolved strict selection, return a bounded error naming the requested
ref. Retain `ErrInvalidBaseBranch` where applicable and preserve context and
existing required-refresh error identities. The caller wraps repository and
attachment identity through the existing preparation-error envelope.

The gate runs before worktree creation, persistence, creation callbacks, and
repository setup for the unresolved attachment. Existing launch handling must
record failure without publishing a ready workspace or starting an agent.
Multi-repository rollback removes only worktrees created by this attempt and
preserves valid reused checkouts.

No new retry, credential route, branch reset, or recovery control is added.
The operator repairs or refreshes the registered repository, then retries the
same task. A retry resolves the original selection. Strict failure never
self-heals the selected attachment to the default branch.

Existing desktop and phone launch-error surfaces receive the same backend
error. There is no rendered layout, interaction, or localization change.
Public Git guidance changes with implementation, not with this draft.

## Compatibility

The new policy qualifies branch-only substitution under criteria .13 and .19.
It preserves separately identified inherited/default recovery while removing
unconditional substitution for explicit and ambiguous legacy bases.
Old non-empty attachments without provenance can now stop on a missing base.
Available bases, valid reused checkouts, and authenticated empty-remote bootstrap
retain their existing behavior.

An ordinary branch supplied alongside PR metadata remains strict. A PR number
does not establish inherited intent. Existing stacked-PR fallback fixtures must
opt into their documented recovery policy. Qualified PR targets never opt in.
Provider refresh, local-only commits, ancestry selection, and cancellation keep
the contracts in the base-refresh design.

## Validation

Use distinct sentinel commits on `main` and a selected branch in a temporary
native Git remote. Clone the cache before publishing the selected branch.
With refresh disabled, real worktree preparation must reject the selected ref
without a default lookup, worktree record, setup callback, or agent startup.
After deliberate fetch, retry must use the selected commit and sentinel.

Producer tests must use task creation/inheritance and persistence rather than
manually supplying permissive launch fields. Adapter tests cover every typed
copy. Backend integration must combine those producers with real Git
preparation and observe startup admission and stored launch failure.

Include explicit selection equal to the default, missing/invalid provenance,
manual reselection, and restarted persistence.
Include early provider/PR fallback, replacement, and permitted inherited recovery.
Include one valid attachment
and one unresolved explicit attachment, then repeat with the same repository
attached twice. Preserve existing local-refresh and qualified-PR controls.

## Related decisions

- [Explicit worktree bases require strict resolution](../../../decisions/2026-11-07-explicit-worktree-base-selection.md)
- [Local refresh remains best effort](../../../decisions/2026-08-31-local-worktree-refresh-best-effort.md)

## Implementation plans

- [Explicit worktree base selection](../../../plans/explicit-worktree-base-selection/plan.md)
