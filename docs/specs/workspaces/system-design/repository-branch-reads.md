---
status: current
system: workspaces
requirements:
  - REQ-WORKSPACES-BRANCH-READS-001
---

# Repository Branch Reads System Design

## Purpose and boundaries

The workspace system owns the source identity and accepted branch lists used by
`useBranches` in `apps/web/hooks/domains/workspace/use-repository-branches.ts`.
Its request coordination must have the same sharing boundary as the Zustand
cache. This design covers reads, not selection or Git execution.

Existing [repository sets](repository-sets.md) and
[branch policies](branch-policies.md) consume branch information but do not own
its general read ordering. [Workspace read recovery](workspace-read-recovery.md)
owns route context and navigation; its context generation is not a replacement
for source-keyed ordering here.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-WORKSPACES-BRANCH-READS-001 | Source identity; Request ownership; Failure and lifecycle; Consumers and verification |

## Source identity and compatibility

Preserve `BranchSource`, `cacheKeyFor`, and all branch slice shapes and setters:

| Source | Cache and request key | Initial read | Explicit refresh |
| --- | --- | --- | --- |
| Saved repository | Existing repository ID | `listBranches(workspaceId, { repositoryId })` | `listRepositoryBranches(repositoryId, { refresh: true })` |
| Folder | Existing `path::workspaceId::path` | `listBranches(workspaceId, { path })` | Same path list call |
| Null source | No key | No request | `refresh` is undefined |

Do not add workspace IDs to repository-ID keys, normalize paths differently,
change endpoint arguments, or introduce cross-source invalidation.
`UseBranchesResult` retains `branches`, `isLoaded`, `isLoading`, and the optional
`refresh(): Promise<void>`. Both successful initial reads and refreshes mark the
cache loaded, including empty responses, as the current production setter does.

## Request ownership

Use `useAppStoreApi()` to address the actual `StoreApi<AppState>` instance.
Follow the store-scoped WeakMap idiom in `use-office-workspace-data.ts` and
`lib/state/prompts-loader.ts`, with a small branch-specific implementation
inside the existing hook module. Do not introduce a fetching framework.

Maintain a `WeakMap<StoreApi<AppState>, Map<string, symbol>>`. Each started read
creates a fresh `Symbol()` token and replaces the key's owner before setting
loading true or calling the network. Distinct stores have independent maps;
different keys have independent owners. Token identity avoids reusing numeric
generations after cleanup.

Initial effects check the live store's loaded status and existing request owner
before starting. Sibling consumers and React effect replay reuse a pending read
instead of launching competing automatic reads. Explicit refresh always starts
a new owner, including when an initial load or earlier refresh is pending.
The initial retry loop is one logical request and keeps one token across all
attempts; retain the existing retry implementation and delays.

Every completion compares its token with the current owner for its store/key:

1. A successful owner calls the current store's `setRepositoryBranches`.
   A non-owner performs no cache write. This guard must precede the setter,
   which itself marks loaded and clears loading.
2. A failure preserves all accepted data and loaded status.
3. In `finally`, only the matching owner removes its entry and clears loading.
   Recheck ownership after any success write: store subscribers can start a
   newer request synchronously during publication.

When the newest read finishes, its entry can be deleted even while an older
request is pending: the older request's unique token cannot match an absent or
new owner. In particular, newest failure must not make an older success eligible
again. Do not delete entries merely because the corresponding hook unmounts.

## Failure and lifecycle

Preserve `listBranchesUntilSettled`'s four retry delays (100, 250, 500, and
1,000 ms) followed by its final attempt. Explicit refresh retains its single
attempt and swallowed rejection, preserving the manual retry API. A terminal
initial failure remains unloaded; loading transitions alone must not initiate
an unbounded new effect/retry cycle.

The shared store owns an outstanding read, not the initiating component.
Unmount, disable, and source changes leave that read available to its original
cache key. No abort controller, observer registry, or new cancellation semantics
are necessary. Release ownership in the request's guarded `finally`; empty maps
hold no historical key/token inventory and the WeakMap does not retain a store
after its consumers and outstanding work are gone. Superseded network attempts
may settle normally, but cannot publish or release newer ownership.

The existing workspace slice remains the data/status source of truth. No
tokens enter hydration, persistence, or public state. No new subscriptions,
timers, authorization rules, metrics, or logs are required.

## Consumers and verification

The five direct consumers are workspace repository chips, watcher repository
fields, task launch branch picker, task base branch picker, and settings branch
policies. Their production code remains unchanged.

Use deferred network replies through the actual hook, real `StateProvider`, and
production `createAppStore`/Zustand actions. Replace the existing mocked-store
hook tests with production-store tests and retain their transient retry and
source-switch controls. Tests must assert accepted cache data, loaded status,
and every consumer's loading state, not only setter calls. Test both completion
orders, newest rejection followed by older success, sibling consumers, separate
keys and stores, remount, successful empty lists, and demand loading. Resolve or
reject every deferred request and use fake timers only for the existing retries.

Include a rendered `useBranches` to `BranchPickerList` integration regression
inside the hook suite, plus existing `branch-picker-list.test.tsx` and
`watcher-repository-fields.test.tsx` coverage for presentation and selection.
`BranchPickerList` is shared by desktop popovers and phone picker sheets.

Mobile parity uses the explicit state/data-only exception in
`.agents/skills/mobile-parity/SKILL.md`: no layout, touch, scroll, navigation,
breakpoint, or copy change occurs. Targeted real hook/store and component tests
are sufficient; no browser E2E, app launch, or build is required for this repair.

## Decision assessment

Apply the existing store-scoped coordination pattern locally. Hook-local
counters cannot fence siblings; a global key map couples independent stores;
store-schema generations add unnecessary hydration surface. The symbol owner
map preserves the rationale and lifetime within this design. `/record`'s ADR
threshold is not met because this local correction introduces no new global
boundary or convention beyond existing patterns and the design records its
alternatives sufficiently.
