---
status: current
system: workspaces
requirements:
  - REQ-WORKSPACES-BRANCH-INVENTORY-001
---

# Repository Branch Inventory System Design

## Purpose and boundaries

The workspace system owns local repository branch inventories. The backend
implementation currently lives in
`apps/backend/internal/task/service/repository_discovery.go`.
`Service.ListBranches` and `Service.ListBranchesWithCurrent` share
`listGitBranches` after existing local path resolution. The saved-repository
and explicit-path entry points therefore receive the same enumeration fix.
Provider-backed reads dispatched by `listRemoteBranchesIfApplicable` retain
their existing behavior.

[Repository branch reads](repository-branch-reads.md) owns client request
freshness. [Branch policies](branch-policies.md) consumes the inventory but owns
selection and policy state. Executor
[repository branch resolution](../../executors/system-design/repository-branch-resolution.md)
owns conversion at the script boundary. None of these contracts needs a
behavior or ownership change for this inventory correction.

## Requirement mapping

| Criteria under REQ-WORKSPACES-BRANCH-INVENTORY-001 | Design sections |
| --- | --- |
| AC-WORKSPACES-BRANCH-INVENTORY-001.1 | Internal branch identity; Enumeration flow |
| AC-WORKSPACES-BRANCH-INVENTORY-001.2 | Internal branch identity; Enumeration flow; Verification |
| AC-WORKSPACES-BRANCH-INVENTORY-001.3 | Returned data and consumers; Verification |
| AC-WORKSPACES-BRANCH-INVENTORY-001.4 | Enumeration flow; Compatibility boundaries; Verification |

## Internal branch identity

Use full qualified ref names as the private `branchMap` keys:

| Source | Internal key | Existing returned Branch |
| --- | --- | --- |
| Local head | `refs/heads/<name>` | `Name: <name>, Type: local, Remote: empty` |
| Remote-tracking ref | `refs/remotes/<remote>/<name>` | `Name: <name>, Type: remote, Remote: <remote>` |

For example, `refs/heads/remotes/origin/topic` and
`refs/remotes/origin/topic` are independent keys. Loose and packed forms of
either identity use exactly the same qualified key. Keys are constructed from
the already parsed namespace and literal name; no extra normalization,
validation, escaping, public identity field, or identity abstraction is needed.

## Enumeration flow

Keep the existing flow and collector signatures:

1. Resolve the repository Git directory with `resolveGitDir` and the shared
   refs directory with `resolveCommonGitDir`.
2. `collectLocalBranches` traverses `refs/heads` and inserts local entries under
   their qualified head keys.
3. `collectRemoteBranches` traverses `refs/remotes`, retains the current remote
   and branch split and `HEAD` exclusion, and inserts qualified remote keys.
4. `parsePackedRefs` keeps its existing line eligibility and projection rules.
   Use the qualified ref itself for both namespace lookups and insertions.
   Insert only absent keys so a loose representation retains precedence.
5. Convert map values to `[]Branch` and use the existing sort unchanged.

The key change must cover all three collectors together. Changing only loose
collection would break exact-identity deduplication when packed refs are merged.

## Returned data and consumers

Keep `Branch`, `BranchListResult`, `Name`, `Type`, `Remote`, and
`CurrentBranch` unchanged. `ListBranchesWithCurrent` continues to read current
branch metadata from the selected working tree after enumeration, independently
of shared ref storage. No response field exposes a commit tip or an internal key.

The production `branchToOption` / `branchOptionValue` functions in
`apps/web/components/branch-picker-options.tsx` and `BranchSelector` in
`apps/web/components/branch-selector.tsx` consume these fields. The admitted
local `remotes/origin/topic` and remote `origin/topic` produce different existing
option values once both entries are returned. Frontend production stays
read-only. The separate ambiguity of local `origin/topic` versus remote
`origin/topic` option values is excluded.

No rendering, copy, layout, touch, scrolling, navigation, or breakpoint behavior
changes. The backend inventory is shared by desktop and phone consumers; no new
UI requirement, screenshot, Playwright run, or product build is needed. If a
later finding requires a frontend data-only test, use the explicit state/data
exception in the mobile-parity skill without broadening production scope.

## Compatibility boundaries

Keep path canonicalization, saved local-path grants, authentication, repository
visibility, provider dispatch, refresh routing, common-directory resolution,
and detached/unreadable current-branch behavior intact. The change introduces
no Git command into production enumeration, no network operation, persistence,
migration, caching, retry, new telemetry, or error-handling policy. Preserve
existing no-branches errors and best-effort ref-file read behavior.

This is a local map-key correction within an existing boundary. The design
records its identity contract sufficiently; it does not meet the record skill's
threshold for a separate architectural decision.

## Verification

Author a new service regression file independently of private qualification
fixtures. Use existing native Git test helpers and `t.TempDir()` to create real
commits, refs, and linked worktrees. Invoke actual
`Service.ListBranchesWithCurrent` through both local entry points; compare the
complete ordered `[]Branch` and `CurrentBranch` to independently stated expected
values. Also verify fixture tips and checkout via native Git so identical or
missing tips cannot disguise invalid setup. Do not substitute a fake Git parser
or a helper-only assertion for service integration.

Cover local `remotes/origin/topic` and remote `origin/topic` at distinct tips in
loose and `git pack-refs --all --prune` forms, with disjoint
`feature/topic` plus remote `origin/topic` controls in both forms. Add a mixed
form by updating the exact local and remote refs after packing to create loose
overrides while retaining packed entries. Assert one entry for each exact
identity and preservation of all other entries. Native Git proves the updated
loose tips; the service assertion proves metadata and cardinality, since its
public result does not expose tips. Preserve loose-before-packed insertion
order and absent-only packed insertion in the implementation.

Use a real linked worktree to verify shared inventory and worktree-specific
current branch. Include nested names, a second remote for sorting, and remote
`HEAD` exclusion as compatibility controls. Existing package tests continue to
cover path grants, provider routing, detached HEAD, and discovery behavior.
All fixture mutations stay inside disposable temporary repositories; never
fetch, push, or modify an existing remote or developer checkout.

## Implementation plan

- [Branch inventory identities](../../../plans/branch-inventory-identities/plan.md)
