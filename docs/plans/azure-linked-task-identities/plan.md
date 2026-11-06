---
created: 2026-10-18
status: implemented
requirements:
  - REQ-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001
system_design:
  - ../../specs/integrations/system-design/azure-devops-integration-02.md
legacy_specs: []
---

# Implementation Plan: Azure linked-task project identities

## Overview

Restore linked tasks in Azure work-item detail when normal first-party task
creation stores a project name and Board selection supplies its GUID. One
sequential work order adds actual-hook regression coverage and corrects only
the display predicate. The integration system owns this provider association
contract, including the desktop and phone outcome.

## Scope

- Match the selected project ID or the current provider item's usable project
  name, retaining workspace and work-item isolation and every distinct link.
- Update the [existing requirement](../../specs/integrations/requirements/azure-devops-integration.md)
  (`AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.12` through `.14`) and
  [task/detail association design](../../specs/integrations/system-design/azure-devops-integration-02.md#taskdetail-association-identity).
- No backend validation/canonicalization, migration, provider mutation, auth,
  pagination, new API, stale-read/cache/save companions, layout, or new copy.

## Evidence and assumptions

Confirmed: normal name-valued associations must be visible alongside existing
ID-valued rows. No material intent question remains. Verified source:
`convertWorkItem` reads `System.TeamProject`; the launcher passes
`payload.item.project`; `AssociateTaskWorkItem` stores the validated caller
project; Board selects `project.id`; `loadLinkedTasks` currently compares only
`task.projectId === projectId` and the work-item ID.

Receipt-only qualification lives in
`/tmp/kandev-root-azure-linked-task-discovery-18ruiqkw/qualified-proof.json`,
`receipt.json`, and `post136-admission.json`. The actual REST/Service/ephemeral
SQLite producer passed. Its serialized name-valued row was excluded by the
actual React hook with selected GUID (one causal failure); GUID and unrelated
project/item controls passed. These are supplied observations, not tests run
by this package. No live Azure, controller HTTP, browser, or mobile layout
coverage is claimed. Private producer/consumer/harness sources must never be
read, copied, replayed, or deleted.

Design admission: local HEAD and actual remote main were
`a8e982cffbb10705db5b5e03a97a4229fa67b972`; all seven qualified source blobs
matched the receipt. No private reproduction was rerun. Existing Azure
integration Tasks 22 and 27 are historical completed work; this package alone
owns the bounded repair and its new checks, without reopening their broad
validation commands.

## Technical approach

Change `loadLinkedTasks` in
`apps/web/hooks/domains/azure-devops/use-azure-devops-work-item-detail.ts`.
Use exact nonblank candidate references from the selected `projectId` and
`initialItem.project`. Filter once by row workspace, work-item ID, and either
candidate project value. Do not normalize compared values or concatenate
separate alias result sets. Preserve request/generation behavior and the
workspace-scoped API. No new helper abstraction is required.

| Input shape | Expected detail result | Evidence |
| --- | --- | --- |
| Selected GUID, provider name, stored name | Include linked row | Actual-hook regression |
| Selected GUID, stored matching GUID | Include linked row | Compatibility control |
| Name omitted, empty, or whitespace-only | Match selected ID only | Negative alias cases |
| Selected and item project references coincide | Include each association once | Alias coincidence case |
| Multiple matching tasks plus foreign workspace/project/item rows | Include all and only matching rows | Mixed collection control |
| Different case or padded project reference | No inferred match | Exact identity control |

## UI outcome and mobile parity

Entry point: open a board card or work-item result. The existing Linked Kandev
tasks section displays the matching task titles rather than its empty message.
Desktop dialog and full-height phone drawer share the hook and markup.
Composition, navigation, scrolling, and touch behavior do not change, so this
data-only package needs no geometry preview or new Playwright scenario.
Targeted hook tests and this explicit note use the mobile-parity exception.
Any newly necessary copy must use i18n; none is planned.

## Tests

Extend the existing
`apps/web/hooks/domains/azure-devops/use-azure-devops-work-item-detail.test.ts`
without removing its comment-merging test. Mount the actual hook using
`renderHook` and coherent typed endpoint responses, following the nearby board
hook test. Independently author fixtures from the public shapes and first-party
flow, not the private ROOT experiment. Test names and criterion mapping:

- `shows name-valued first-party links when the selected project is a GUID`
  and `keeps existing GUID links`: `.12`.
- `falls back to the selected ID without a usable item project name` and
  `returns each association once when project aliases coincide`: `.13`.
- `retains all matching links and excludes foreign workspace project and item rows`
  and `does not normalize unrelated project references`: `.12`, `.14`.

Make the name/GUID regression fail against the unchanged hook before the
production edit. Wait for linked-task loading to finish and assert a nonempty
expected row set, not merely eventual loading flags. Mock only endpoint
responses; exercise the real hook's filter and effects. Workspace controls
assert the requested workspace and reject foreign rows within a mixed response.

The existing test file is in the `browser` list in
`apps/web/scripts/vitest-reviewed-projects.json`, not the Node list.
`vitest.config.ts` assigns that project `happy-dom` and `vitest.setup.ts`,
pins `NODE_ENV=test`, and the setup checks `React.act`. This supports actual
`renderHook` coverage without changing project registration or configuration.

## E2E evidence boundary

The supplied qualification connects actual first-party projection/persistence
to the real hook, using synthetic HTTP and ephemeral SQLite. Permanent consumer
regressions use coherent equivalent data independently. Together they cover the
bounded data path; they are not browser E2E. No unrelated desktop/mobile suite
is required for this data-only correction.

## Work orders

- [x] [Task 01: Match current project identities in work-item detail](task-01-linked-task-identities.md)

## Delivery controls

This SAME primary session performs the single order only after ROOT's later
explicit implementation interrupt. No delegates, sessions, or model switch.
Use the sole global local-heavy lease for install/tests/lint/typecheck/hooks,
returning it only after actual command joins. Install frozen dependencies once
if missing. Use canonical Go 1.26.0 (if needed), Node 24.21.0, pnpm 9.15.9;
`GOTOOLCHAIN=local`. No full backend lint is part of this frontend-only scope.
Every new `run-quiet` invocation gets a fresh task-owned
`KANDEV_RUN_QUIET_DIR`; never use shared retention or prune foreign resources.
Retain every native session handle, actual exit/join receipt, and current owned
process-group absence. Preserve protected tasks/worktrees/refs/dependencies,
caches, processes, volumes, and private evidence.

After task checks, use normal active hooks and authorized commit/push/PR flow.
No bypass and no routine FULL review on every later SHA. Assess automatic
feedback and substantive reports; optional style/report work does not delay
READY. Do not end after publication or await a separate release ceremony.
Immediately launch one direct `scripts/pr-await` observer in that same delivery
turn with the confirmed command
`timeout --kill-after=10s 91m scripts/pr-await <PR> --deadline-min 90 --mode all-terminal --interval-sec 60 --format json`.

Observer files are under task-owned
`/tmp/kandev-azure-linked-task-identities-8c223f1b-MD1mkD0i/`:
`pr-await.startup.json`, `pr-await.progress.stderr`,
`pr-await.terminal.stdout`, and `pr-await.join.json`. Record real PR/head,
start/deadline and native handle at launch. Progress is stderr; terminal JSON
is stdout. Poll to actual completion and report at most 60 seconds apart.

READY requires a fresh, complete, error-free current-head snapshot, six required
contexts and actual current Backend/Frontend/E2E parent runs SUCCESS, no
actionable visible/hidden finding, requested change, or human gate, and a clean
local HEAD equal to actual remote branch and PR head. ROOT's separate retry
authorization is required; job-name retry budgets persist across heads. Merge
is excluded until ROOT's later expected-head normal squash authorization after
static compatibility. No admin or branch deletion flag.

## Verification results

Task 01 is done. The production diff changes only `loadLinkedTasks`; independent
actual-hook coverage retains the original comment-merging test. Canonical
Node 24.21.0 and pnpm 9.15.9 were used. Frozen install ran ONCE because this
worktree had no dependencies.

- RED: focused Vitest command in Task 01, 2 failed and 7 passed. The causal
  name-valued link returned `[]` against unchanged production.
- GREEN: the same focused command, all 9 tests passed.
- Scoped ESLint and staged `pnpm run i18n:ratchet`: passed.
- `pnpm run typecheck`: the first attempt exhausted Node's default 4 GB heap
  (exit 134); the identical check with process-local
  `NODE_OPTIONS=--max-old-space-size=8192` passed. No source/config change was
  made for that resource limit.
- Catalog validation (386 decisions and 1,633 specifications), all-spec lint,
  actual SIX-path documentation coverage (`COVERED`, `errors: []`, one order),
  whitespace and package status checks: passed.

Command receipts and raw logs are under task-owned
`/tmp/kandev-azure-linked-task-identities-8c223f1b-MD1mkD0i/`:
`install-latest.json`, `red-latest.json`, `green-latest.json`,
`lint-latest.json`, `typecheck-latest.json`, `typecheck-8gb-latest.json`, and
`ratchet-latest.json`. Native handles 53100, 25595, 6633, 40662, 12274,
62756 and 19132 were actually joined with their documented exit codes;
current owned groups were absent at join. Light spec/coverage handle 59079
also joined with exit 0. Normal commit hooks and hosted delivery follow these
checks; their receipts belong to the operational task plan.

Design receipts remain in `design-checks.json` and `design-final-checks.json`.
The design-only actual four-path coverage was `EXEMPT`; the earlier prospective
rehearsal is separate historical evidence. The 36 passing spec-linter self-tests
were historical design validation only and were not replayed during
implementation. This package makes no live Azure or browser/mobile layout claim.
Public `docs/public/integrations.md` already promises linked-task visibility;
this repair restores that contract without new configuration or terminology.

## Risks

An overly broad alias rule could show another project's task. Exact aliases,
nonblank guards, workspace/item checks, and mixed negative controls constrain
that risk. Project renames do not establish historical aliases. Endpoint-mocked
hook tests prove consumer behavior, not backend acceptance of GUID association
requests or real provider/browser behavior.
