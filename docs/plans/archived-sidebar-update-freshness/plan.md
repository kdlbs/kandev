---
created: 2026-10-08
status: implemented
requirements:
  - REQ-UI-SIDEBAR-ARCHIVED-FILTER-002
system_design:
  - ../../specs/ui/system-design/sidebar-shared-task-state.md
legacy_specs: []
---

# Implementation Plan: Keep Newer Archived Task Updates During Sidebar Loading

## Overview

Opening an archived sidebar view starts a bounded page read. A previously
unloaded task can receive a newer rename, then an older rename, before the
response arrives. The journal currently retains the last arrival, so an older
response can publish the superseded title. Coalesce partial journal updates
using the existing task and summary freshness rules.

One sequential work order adds a causal transport regression, corrects the
existing merge module, and runs affected checks. ROOT reviewed this package and
released implementation to the same primary on 2026-10-08. Local implementation
is complete; hosted delivery and a separate ROOT merge grant remain pending.

## Ownership and settled assumptions

UI owns this reusable client overview/paging contract; task lifecycle stays
with Tasks. The supplied `sidebar-archived-filter.md` was inspected and owns
`.001`, while [sidebar-task-pagination.md](../../specs/ui/requirements/sidebar-task-pagination.md)
actually owns `.002/.20`. Amend that existing criterion rather than duplicate
its ID. The requirement remains active and the shared-state design retains its
existing draft status; this repair does not promote the wider package.

ROOT's reproduction is accepted as qualified supplied evidence, without
reading, importing, copying, executing, editing, or deleting the original proof:

- `/tmp/kandev-root-sidebar-event-discovery-20261008/registered-archived.test.ts`,
  regular0400, SHA256 `4af83d986d1a56c08419b36a7674f6008c82183694379846d95a2e84546602c9`.
  Native11345 actually joined18d631; actual Vitest exit1, one causal RED and
  chronological positive PASS, 6.680 seconds. Group1533189 and temporary source absent.
- Supporting `candidate.test.ts` in that directory, regular0400, SHA256
  `8e22b85e979aaef710f20a8203825706349525db36f03f168392b30a5dc43d8e`.
  Native36781 joined6fc747; actual exit1, one causal RED and two positive PASS.
- `wire-root-qualified.json` and `root-qualified.json` are ROOT's qualification
  receipts. Attempt30900 selected no tests and is not RED. ROOT owns all releases.

Confirmed outcome: newest accepted task fields survive reversed events and
older HTTP; newer HTTP can win. Summary revisions remain independent. Active
board events already upsert residents and protect freshness. No unresolved
material choice remains after the assumption check.

## Scope

### In scope

- Minimal reconciliation of AC .20 and its existing shared-state design.
- Repeated partial patches within `recordTaskOverviewChange`, sharing the
  existing `mergeTaskOverview` rules without treating a patch as a full task.
- Permanent real registered-handler/store/cache tests with deferred transport,
  plus existing directly affected controls.
- Normal scoped validation and later ROOT-controlled delivery evidence.

### Out of scope

Event-order frameworks, backend/schema/API changes, archive policy, navigation,
membership or source eligibility changes, new clocks, page retention tuning,
board/settings/runtime audits, new flags, new public copy, new ADRs, full local
product suites, delegates, recursive tasks, sessions, tabs, and model changes.

## Technical approach

Keep production scope in `apps/web/lib/state/slices/task-overview-merge.ts`.
If typing requires it, share a private partial-safe implementation in that
module while retaining the resident merge's public contract and reference reuse.
`TaskOverviewPatch` already exists; immediate type glue is allowed only if
causally necessary. Never cast an accumulated partial patch to a full overview.
Keep null handling ahead of merge and calculate bytes from the accepted result.

Audited registered path:

1. `registerTasksHandlers` registers `task.updated`; `handleTaskUpdated` detects
   archived payloads through `getTaskUpdatedArchiveContext`.
2. `applyTaskUpdatedCache` routes them to `applyTaskOverviewEvent`, which maps
   own wire fields using `taskOverviewPatch` and records the patch.
3. `recordTaskOverviewChange` merges resident `byId` values with freshness but
   currently spreads repeated `read.changes` entries without freshness.
4. `SidebarTaskPageCache.request` opens a read. `settle` calls
   `reconcileSidebarPage` and `reconcileTaskOverviewRead`, then retains records.
5. `useSidebarTaskPage` checks request/view/context identity and owns display
   retention; `useWorkspaceSidebarTasks` resolves entries through canonical
   `taskOverview.byId`. Its summary projection has its own revision selection.

The registered `task.status_summary.updated` handler uses
`applyTaskOverviewPatch` without a task timestamp. Workflow snapshot consumers
also reconcile this same journal; their semantics remain intact. No handler,
cache, hook, board, or compatibility-owner change is planned.

Preserve strict nanosecond timestamps and offset equivalence, the existing
missing/malformed/equal timestamp fallback, absent-field preservation, explicit
eligible clears, independent summary revisions including equal-revision queue
fields, sticky deletion tombstones, 1,000 IDs / 1 MiB per read, generation and
overflow rejection, source owners, account/workspace barriers, and read cleanup.

## Tests

The new permanent file is
`apps/web/lib/sidebar/sidebar-archived-update-freshness.test.ts`.
Its describe name is `archived sidebar update freshness`. Only
`querySidebarTasks` is replaced with controllable promises; the registered
handler, `createAppStore`, and `SidebarTaskPageCache` remain real. Assertions
follow returned membership through its canonical task record, the same input
used by the sidebar row projection. Test setup must not seed the missing
nonresident task or call the journal helper instead of the registered update.

| Cases in the work order | Acceptance | Observable evidence |
| --- | --- | --- |
| Reversed and chronological archived updates; old/new HTTP; resident variant | .20, .8, .23 | Accepted title/time, archived row ID, no active-board insertion, provisional response |
| Conflicting task/summary clocks and summary-only updates | .20 | Independent title/time and summary revision/queue fields |
| Nano/offset/tie/fallback and partial fields/explicit clears | .20 | Correct accepted row values without fabricated fields |
| Empty/failed transport, deletions, ID/byte limits | .16, .18, .23, .24 | No resurrected row or reusable unsafe page; old read rejected; protection released |
| Separate reads, workspace/account/store barriers | .7, .16, .20 | No cross-read or cross-context publication |

All acceptance references in this package have prefix
`AC-UI-SIDEBAR-ARCHIVED-FILTER-002`.

Existing directly affected controls: `task-overview.test.ts`,
`task-overview-mutations.test.ts`, `sidebar-page-overviews.test.ts`,
`sidebar-task-page-cache.test.ts`, `tasks-update-ordering.test.ts`,
`task-status-summary.test.ts`, and `use-sidebar-task-progress.test.tsx`.
Exact paths and commands are in the work order. Do not add broad suites.

## Mobile and E2E

This is purely state/data normalization. Desktop sidebar and phone
`SessionTaskSwitcherSheet` / `MobileTaskList` retain their existing shared
projection, layout, navigation, touch targets, scroll owner, and breakpoints.
The mobile-parity data-only exception is satisfied by the real transport
integration and existing affected hook controls. No new browser fixture,
Playwright run, or ASCII layout preview is needed for this scope. A later
visual/interaction expansion requires a new ROOT checkpoint before work.

## Public documentation and related records

Audited `docs/public/**`, root README, and `docs/screenshots.md`, including the
sidebar paging/loading/reuse explanation in `tasks-and-workflows.md`. This
repair restores already documented task-change behavior; it changes no user
operation, copy, public transport, configuration, screenshot, or navigation.
Public documentation edits are unnecessary. Internal docs only; no new ADR
because the existing freshness/ownership boundary is retained.

[Shared query memory](../sidebar-query-memory/plan.md) and its tasks 03/04 are
the normalization source. [Archive loading](../archived-sidebar-loading/plan.md),
[view loading repair](../sidebar-view-loading-repair/plan.md),
[collapse continuity](../sidebar-collapse-continuity/plan.md), and
[task navigation](../task-navigation-responsiveness/plan.md) remain companion
history. Their statuses and recorded results are not evidence for this repair
and are not reopened. The [shared-state ADR](../../decisions/2026-09-29-shared-sidebar-task-state.md)
is retained without alteration.

## Work orders

- [x] [Task 01: Preserve freshness in sidebar read journals](task-01-preserve-journal-freshness.md)

One sequential order, no delegation or additional workspaces.

## Execution and delivery checkpoint

Task `ad2e47a5-b7eb-4499-b481-0f3907e6afee`, primary session
`88f5cd87-df4d-491d-850d-3ae750b19742`; preserve the live task plan's
`<kandev-system>` marker and user edits. ROOT owns the title. Admission main was
`b7341134fe3307f3d0d022ea08c3864c20e53a9d`. Child76 was independently verified,
archived, and absent-board read back before admission; children77/78 were the
two admitted active children. This is a checkpoint, not permission to create a slot.

After a later reviewed-package implementation interrupt, read this plan and
the work order directly, then request/use only ROOT's one global local-heavy
lease. Keep one retained handle per original command; record PID/group, UTC
cutoff, exit, and actual join. No automatic retries, passing-test replay, foreign
resource cleanup, or uncontrolled overlap. Resource, timeout, transport,
unknown, and out-of-scope conditions checkpoint ROOT. Child-to-ROOT messages
are queued; never interrupt ROOT, wait for a queue acknowledgment, or retry
a full queue. Critical parent questions end the turn at the tool barrier.

Later delivery, only after ROOT release, preserves this order:

1. Meaningful scoped RED/GREEN, affected checks, docs traceability, and normal
   hooks. No hook bypass or amend; restage hook reformats into a new commit.
2. Commit/push/PR under the standing release; immutable published SHA except
   an actual valid corrective finding. No moving-main rebase or optional polish.
3. Discover and prove all six actual required contexts and Backend/Frontend/E2E
   parent workflows terminal SUCCESS. Evidence is fresh, complete, `errors: []`,
   and has no hidden/actionable issues. Record actual names/IDs, not guesses.
4. Authenticated configured CodeRabbit App347564 must provide substantive FULL
   exact-head evidence for all changed files. Automatic review is accepted;
   inspect skip/gap before one necessary request. No acknowledgment evidence,
   optional second review, or synthetic tests.
5. Retain one original 90-minute all-terminal collector (GNU timeout 91 minutes,
   kill-after 10 seconds, cadence 60 seconds) and actually join it before a
   ROOT-authorized replacement. Hosted retries require explicit bounded ROOT
   grants keyed by workflow and job NAME across attempts/heads; only causal
   failed leaves/dependents after the containing workflow is terminal.
6. Discover/read caller-bound canonical PR association: complete, `errors: []`,
   all five automation flags FALSE. A separate serial ROOT MERGE lease precedes
   normal expected-head squash. Prove actual merge, content, remote head, every
   original join, and only-owned cleanup. END checkpoint ROOT before ROOT
   archives and reads back absence. An optional queued callback is no progress gate.

## Verification results

Local implementation completed on 2026-10-08 under the reviewed-package release
and sole ROOT local-heavy grant. Production changes are confined to the existing
merge module: resident and journal values share partial-safe freshness selection,
while summary availability, reference reuse, tombstones and budgets remain intact.

The original anchored RED ran both named registered-path cases: one causal
latest-title failure and one chronological PASS. The original eight-file GREEN
ran 76 tests: 74 passed and two new partial-field expectations failed because the
existing wire mapper normalizes a cleared parent to `undefined`. Correcting only
that expectation passed both anchored cases. Scoped lint then required smaller
test registration functions and one shared title constant; the affected new suite
passed all 24 tests after that structural correction. The seven unchanged suites
passed their 52 tests in the original run and were not replayed.

Changed-file eslint, typecheck, i18n check/ratchet, catalog validation (363 decisions,
1,437 specifications), specification-linter tests (36), all-spec lint and whitespace
passed. Typecheck performed the normal generated-note preparation without changing
tracked generated files. Actual six-file documentation coverage was `covered`,
with one work order and `errors: []`. Public docs remain unchanged for the audit
reason above; mobile/E2E uses the recorded data-only exception.

One conditional pinned frozen install completed. Original PID/group/deadline/exit
and native join receipts are retained in `/tmp/kandev-child78-design-20261008/`;
all completed groups are absent. ROOT original proof remains untouched. A shell
quoting error in the first launcher ended before pnpm ran and was corrected;
this is not a second package installation or resource retry.

Normal hooks, publication, canonical association and exact-head CI/review evidence
are delivery gates, recorded in the live task plan after their actual outcomes.
No merge authority is held. The wider shared-state design retains draft status.

## Risks

- A full-overview cast can hide incomplete journal data; preserve patch typing.
- A single task clock can discard a newer summary or regress task time.
- Equal summary revisions can carry updated queue fields; preserve incoming tie behavior.
- Stale coalescence must not inflate bytes or remove tombstone protection.
- Dependency installation completed once from `apps/` under the ROOT lease.
