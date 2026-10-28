---
created: 2026-10-26
status: implemented
requirements:
  - REQ-TASKS-RUNTIME-CLEANUP-001
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-001
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-003
system_design:
  - ../../specs/tasks/system-design/runtime-cleanup.md
  - ../../specs/tasks/system-design/workspace-repository-placement.md
legacy_specs: []
---

# Implementation Plan: Live attachment retry cache cleanup

## Overview

Make a failed workspace-source batch retryable in the same running backend when
multiple sessions share its task environment. Independently prove the failure
through public attachment behavior, then correct successful physical-removal
cache finalization and run the focused regression/control matrix in one work order.

Task Runtime Cleanup owns the contract because it defines task-owned physical
worktree lifetime and shared-reference preservation. Workspace repository
placement supplies the existing attachment rollback/retry acceptance criteria.
The cleanup requirement gains AC-TASKS-RUNTIME-CLEANUP-001.58 for observable
post-removal coherence; active attachment requirements remain unchanged.
The owning pair retains its existing draft lifecycle; this narrow repair does
not promote the entire unrelated cleanup contract. No new ADR is needed: the
existing task-owned lifetime decision and the design preserve the boundary and
rationale. Small reductions of duplicate prose keep both owning files below
their existing size limits without changing other guarantees.

## Confirmed root cause and evidence

The defect remains after merged PR #4807: force the second canonical
inventory insert to fail during public `AttachWorkspaceSources`, verify exact
rollback and physical removal, remove the trigger, obtain a fresh preview, and
retry through the same service/manager. With two sessions the retry fails with
`ErrBranchUnrecoverable`; otherwise identical single-session and reconstructed-
manager controls pass. This is the qualified failure that the independent
permanent regression must reproduce before the source correction.

`persistAndCacheWorktree` keys creation by creator session/repository/branch
slug. Canonical task inventory joins environment sessions and deduplicates by
`Worktree.ID`, so cleanup can receive another session projection. Successful
`finalizeRemovedWorktree` currently releases metadata and deletes only that
projection's session key. A creator alias can remain active in memory after its
physical path and generated branch have been removed; retry enters recovery of
that absent branch. The merged placement fields and tombstone-reactivation
predicate are separate and remain correct.

The shipped `workspace_source_materializer_inventory_test.go` and public fixture
helpers are available patterns for independently authored regression tests.

## Scope

### In scope

- Successful full physical-removal finalization in
  `apps/backend/internal/worktree/manager_cleanup.go`.
- New focused manager cache tests and public-service real SQLite/Git rollback
  and live-retry regression in separate files.
- Owning requirement/design updates, this manifest, and exactly one work order.

### Out of scope

- UI, MCP question files, frontend state, agent/session replacement, or restart
  workarounds. Mobile parity does not apply because no interface, interaction,
  copy, or frontend behavior changes; the backend outcome is shared by clients.
- Resume/reuse guard relaxation, canonical placement/SQL/schema/migrations,
  physical branch policy, reference authority, ownership locks, retained-checkout
  preparation, archive lifecycle redesign, and generic cache/lifecycle hardening.

## Technical approach

### Physical identity and finalization

All inspected removal entry points converge through `removeWorktreeWithReceipt`
and `finalizeRemovedWorktree`: `RemoveByID`, `RemoveByIDWithReceipt`, and batch
cleanup variants, including branch-preserving/archive paths. Preserve the
current snapshot, retention, target-path/repository locking, reference count,
audit, physical cleanup, metadata update, and branch-compaction order.

After physical cleanup and metadata release have succeeded, hold `m.mu` and
remove every cache entry whose non-nil value has the removed nonempty physical
`Worktree.ID`. This is a local bounded map scan, requiring no secondary index or
new cache authority. Keep the existing exact-key behavior for an empty-ID
legacy input; never treat equality of empty IDs as shared physical identity.
Do not evict by session, path prefix, repository, or branch alone. An entry for
another physical ID must survive even when it shares repository/branch labels
or a session with the removed worktree. No mutable pointer equality is required.

### Reference-only and failure boundaries

| Path/outcome | Cache and resource behavior |
| --- | --- |
| Successful full removal and metadata finalization | Evict all aliases of that nonempty physical ID; preserve other IDs. |
| `ReleaseWorktreeReference` alone | Preserve current canonical status/recovery metadata update and exact session-key eviction; no physical-ID-wide eviction. Its historical association wording does not establish a separate session-owned table. |
| `retainReferencedWorktree` | Return before removal/finalization; shared physical inventory, path, branch, and usable aliases survive. |
| Reference lookup/count, retention, audit, or physical cleanup fails | Preserve existing error/retry behavior; no new eviction. |
| Metadata release fails after physical cleanup | Preserve existing failure behavior and ordering; do not add broad eviction or branch compaction on this path. |
| No store / no projected session | Successful physical finalization still clears nonempty-ID aliases; do not make eviction depend on the selected session. |

The production-call-site search found `ReleaseWorktreeReference` called from
finalization; the task service also declares its interface. Direct tests exercise
it independently. This does not authorize changing its standalone behavior.

### Public attachment regression

Independently author `TestWorkspaceSourceMaterializer_LiveAttachmentRetry` in
`apps/backend/internal/backendapp/workspace_source_materializer_live_retry_test.go`.
Use the real SQLite repository, real Git worktrees, existing shipped
`inventoryRollbackFixture` helpers, and public preview/attach calls. Keep the
same service and manager throughout the causal cases; no cache injection,
manager reconstruction, or reuse-guard changes may make those cases pass.

The matrix is:

| Failure boundary | Same manager, two sessions | Single-session control | Reconstructed-manager control |
| --- | --- | --- | --- |
| Second canonical inventory insert | RED before fix; retry succeeds after fix | Same insert failpoint, live retry passes | Two-session retry passes |
| Second session rescan after creation | RED before fix; retry succeeds after fix | Not applicable: needs two session rescans | Two-session retry passes |

Both points invoke full compensation and are relevant. Assert the injected
failure and completed rollback before attempting retry. Compare exact prior task
sources, active canonical rows and physical IDs, root/layout, primary branch,
exclusions/configuration, and unrelated `keep.txt`. Prove every new checkout is
physically absent before retry; inactive history is allowed. Then remove/reset
the failpoint, obtain a fresh preview, and retry. Prove every source has exactly
one active canonical slot, correct recorded path/order, and matching actual Git
registration/branch/path. Capture committed state and assert an exact repeated
request is a no-op: unchanged rows/IDs/paths/timestamps and rescan count, no
duplicate worktrees, root/session changes, or restart. Reopen SQLite and verify
the same durable/physical agreement; reopen is an assertion after live success,
not the causal retry boundary.

### Focused manager regression

Add `apps/backend/internal/worktree/manager_cleanup_cache_test.go` with:

- `TestCleanupWorktrees_CacheAliases`: create a real worktree through one
  session, establish aliases through session reads, and clean using a different
  session projection of that same ID. Explicitly assert the projection differs
  from creator; do not rely on undefined SQL join ordering. Cover store-backed
  and no-store finalization and both remove-branch/preserve-branch entry points.
  All removed-ID lookups must miss, while distinct physical IDs in sibling
  repositories, branch slots, and sessions remain usable. Include an unrelated
  physical ID sharing repository/branch labels in a different task/session.
- `TestReleaseWorktreeReference_CacheScope`: standalone release keeps the
  existing key-scoped eviction and leaves physical path/branch plus other
  aliases intact; assert the unchanged canonical status behavior separately.
- `TestCleanupWorktrees_CacheAliasesRetainedOnFailure`: audit/removal rejection,
  active foreign reference, and injected metadata-update failure preserve
  existing cache/failure semantics. Exercise existing error mechanisms rather
  than adding production failpoints. Include empty-ID isolation at the small
  cache-finalization boundary so unknown identity never groups entries.

Use assertions on observable manager lookups and physical/canonical outcomes;
small internal cache assertions are appropriate only for cache isolation.
Do not build a broad implementation-mirroring suite. Existing shared-reference,
branch recovery, release-failure retry, and removal safety tests are controls.

## Tests and traceability

| Criteria | Evidence after implementation release |
| --- | --- |
| Cleanup AC-001.58 | Live retry and all-alias manager regressions; single-session/rebuilt controls. |
| Cleanup AC-001.2/.3 | Shared-reference and standalone release controls; unrelated physical-ID preservation. |
| Attachment AC-001.4 and AC-003.6 | Both injected failures, exact pre-retry rollback, same-manager success, committed no-op. |
| Attachment AC-003.7 | Preserved root/primary/unrelated entries and matching durable/physical placement after retry/reopen. |

End-to-end proof is at the public backend service with real SQLite and Git;
there is no new browser workflow. The one work order names exact race, lint,
catalog, spec, coverage, and diff commands. SQLguard/store conformance are
conditional only if an authorized actual persistence change makes them relevant;
this design changes no persistence code. No PostgreSQL runtime coverage is
claimed without an actual configured DSN and executed tests.

## Work orders

- [x] [Task 01: Finalize cache aliases after physical cleanup](task-01-finalize-cache-aliases.md)

The single sequential work order is complete following an explicit
implementation request.

## Related packages and overlap

The existing `attached-worktree-inventory` package is implemented and records
this live-manager limitation honestly; do not rewrite its historical results.
`backend-failure-containment`, `worktree-resume-after-unarchive`,
`archive-resume-identity`, and `compact-integrated-managed-branches` are completed
companion packages with separate scope. No work order or evidence in them needs
reopening for this repair.

Retained checkout lifecycle/identity, session/queue history cleanup, and
clarification UI are adjacent responsibilities outside this package. Check
current ownership of the proposed paths before implementation.

Public docs impact: internal docs only. Searches of `docs/public/**`, root
README and screenshot catalog found no restart workaround to remove. This
restores existing retry behavior without a new command, API, setting, or UI
instruction. Reassess only if implementation changes that scope.

## Verification results

Design checks on 2026-10-26 passed: catalog validation (403 decisions, 1740
specifications), 36 spec-linter self-tests, full spec lint, tracked diff whitespace,
and actual added/modified-path documentation preflight (`exempt`, no errors).
The prospective seven-path source/test/docs preflight is `covered`, no errors,
and resolves all three requirements through the two listed designs. Product
RED/GREEN, lint, builds, hooks, and publication were not executed or authorized
in the design turn. No dependency install occurred.

The first preflight launch found no `node` on inherited PATH and executed no
checker. Selecting the existing Node binary completed the checks without an
install. The work order now names verified Go 1.26.0/Node 24.21.0 tool paths.
All seven Bash command blocks pass syntax validation; both added files pass
explicit whitespace checks, and the initiative contains one manifest and one
work order.

Implementation verification completed on 2026-10-26:

- Independently authored public-service RED: both live shared-session retries
  failed with the expected missing-branch recovery error after exact rollback;
  the single-session and both reconstructed-manager controls passed. Focused
  GREEN passed all five cases (backendapp 4.766s).
- Focused manager RED: all four successful-removal modes retained stale aliases;
  standalone release, shared-reference, dirty-audit, post-audit path replacement,
  metadata-release failure, and empty-ID controls passed.
- The exact affected race command in Task 01 passed after the final production
  edit: backendapp 8.440s, worktree 22.313s. It covers every new regression plus
  the named existing attachment/cleanup/branch-recovery families.
- Canonical changed-Go package lint passed with zero issues using its separate
  3GiB budget. No SQL/store/schema change occurred, so SQLguard and store
  conformance were not rerun; no PostgreSQL runtime coverage is claimed.
- One frozen workspace dependency install completed without manifest or lockfile
  changes. All local product/lint commands joined successfully after GREEN;
  command receipts and initial test-fixture assertion corrections are retained
  externally. The initial mixed fixture failures are not the clean causal RED.

The owning cleanup pair retains its pre-existing draft lifecycle because this
order verifies only its focused acceptance criteria, not the entire broad
cleanup requirement. Implementation matches the new cache-finalization section
and the unchanged active attachment contract.

## Risks

- Moving ID-wide eviction into reference-only release would exceed removal
  authority; keep it at successful physical finalization.
- Grouping by empty ID or by session/repository/branch could evict unrelated
  physical worktrees. Explicit isolation cases must prevent that.
- Store projection order is not an API guarantee. Prove the differing session
  in the focused test without changing production selection rules.
- Existing cleanup failure after physical removal is deliberately unchanged;
  this package makes no new promise for that partial-failure window.
