---
created: 2026-10-05
status: implemented
requirements:
  - REQ-OFFICE-AUTOMATIONS-SETTINGS-001
system_design:
  - ../../specs/office/system-design/automations-settings-02.md
legacy_specs: []
---

# Implementation Plan: Keep Automation Lists in Their Workspace

## Overview

Repair workspace isolation and refresh publication ordering in the settings
automation list. One sequential work order implements the existing-slice seam,
adapts the bounded consumers, and supplies real hook/store and rendered-list
evidence. ROOT reviewed the complete design package and explicitly released implementation
in this same primary session.

Office owns workspace automation definitions and settings. The existing
requirement lacks list ordering criteria, so this package amends AC-001.12
through AC-001.15 in that owner. Part 1 is 32,531 bytes and near its 32 KiB
limit; the new local design belongs in Part 2. Existing unrelated migration and
target-mode material stays outside the repair. No new ADR or incident requirement
is needed.

## Confirmed evidence and root cause

Proof base and local HEAD at design start:
`91da5a242f145b0bb7d0303b4969fdc1552811e5`.

Accepted read-only proof:
`/tmp/kandev-automation-list-ordering-repro.test.tsx`, SHA256
`a955e70c534c78ee4b97b763873ad2192a98bc5fc784c33030d1bc3b0f4cd342`.
ROOT's `/tmp/kandev-root32-automation-list-repro.log` and
`/tmp/kandev-root32-proof-receipt.json` record joined handle 74061, exit 1,
two expected failures and one current-load positive pass. Preliminary handle
9887 was a wrong authoring-path/no-tests setup failure, not meaningful RED.
Accept this proof without replaying, editing, archiving, or blindly copying it.

Production `use-automations.ts` SHA256:
`f731abe8d60c283e95bcb2019d410ec3a0d13d666d9ce1a6cef32044821f3b32`.
Its loaded/in-flight refs compare workspace strings. Returning to cached A skips
revoking B, so B later writes flat global items displayed under A. Two refreshes
for the same workspace have equal strings, so both can publish and the oldest
can replace the newest. Local refs also cannot coordinate separate hook
instances sharing the real store.

## Scope

### In scope

- Workspace-keyed settings list rows, flags, cached reuse, and store/workspace
  request authority shared by hook instances.
- Current and obsolete success/empty/failure settlement, initial single-flight,
  explicit refresh, null selection, and store-owned request lifecycle.
- Narrow sequential mutation cache maintenance, existing secret stripping,
  legacy flat invalidation projection, and workspace breadcrumb selection.
- Permanent adapted regressions and a real rendered list integration with
  automation transport mocks only.
- The owning requirement/design and this manifest/work order.

### Out of scope

- Backend runtime, DB/schema/API, migrations, trigger metadata, automation runs,
  sidebar's independent list cache, and unrelated writer/cache audits.
- Mutation-lifetime defect claims or list-versus-mutation/backend-event/tab
  coherence; no proof establishes those defects here.
- Layout, copy, touch targets, navigation composition, scroll/breakpoint changes,
  public docs changes, local browser/build/E2E, and broad suites/audits.
- New delegates, tasks, tabs, sessions, model switches, child30 work, callback
  routing changes, foreign resources, or automatic resource/hosted retries.

## Technical approach

Follow [Workspace list publication](../../specs/office/system-design/automations-settings-02.md#workspace-list-publication).
Add `byWorkspace` entries and guarded begin/finish actions in the existing
automations slice. Admission and settlement are atomic in the owning store;
initial loads deduplicate, explicit refresh advances per-workspace generation.
The hook selects scoped state and removes local workspace-string refs. It
continues exposing the same methods and API responses.

Accepted entries remain in memory for the store lifetime, one per visited
workspace. No eviction/reset API, persistence, retry framework, timers, or
coordinator is introduced. Requests remain store-owned after unmount so sibling
consumers and remounts can receive their own valid cache completion. Newest
generation alone owns scoped rows and flags. A valid inactive B response may
cache B but cannot change A's selected output. Filter foreign response rows at
scoped publication.

Retain flat compatibility fields/actions and their reference-change signal.
Mirror only accepted list results; derive legacy loading from scoped entries.
Breadcrumbs prefer their workspace entry with explicit workspace validation and
legacy flat fallback only when the entry is absent. Mutation actions update
already-created scoped entries alongside their existing flat behavior, preserving
webhook-secret stripping and caller returns. Do not create partial loaded lists
from mutation responses or claim to fence concurrent mutation/list races.

### Bounded client inventory

| Client | Current boundary | Repair/control |
| --- | --- | --- |
| `components/automations/automations-list-page.tsx` and `automations-table.tsx` | Real page renders hook rows without filtering | Read-only production; real page/table integration proves scoped rows and row navigation |
| `components/automations/automation-editor.tsx` | Starts `useAutomations`, uses mutation methods | Read-only production; sibling hook and sequential mutation controls preserve its contract |
| `components/settings/use-settings-breadcrumbs.ts` | Flat items with explicit workspace check | Prefer scoped entry, retain check and legacy fallback; real hook integration plus existing breadcrumb controls |
| `hooks/domains/sidebar/use-sidebar-shortcut-catalog.ts` | Flat items reference invalidates its own scoped fetch | Read-only production; retain reference updates and run existing catalog controls |
| `components/runs/use-workspace-automations.ts` | Own list fetch and local workspace/request guards | Read-only reference pattern; no cache consolidation |
| `hooks/use-automation-trigger-types.ts` | Existing store-owned workspace generation pattern | Read-only reference; trigger types/run state compatibility assertions |

`mergeInitialState` already spreads default and supplied automation fields;
`buildStateOverrides` carries the merged slice. Legacy initial-state shape must
continue working without changing those files. Any demonstrated need to widen
that boundary requires a checkpoint for ROOT direction.

## Tests

| Criteria | Permanent evidence |
| --- | --- |
| AC-001.12 | `use-automations.test.tsx`: cached A/B/A, pending switch, null, mixed eligible/foreign rows; `automations-list-page.integration.test.tsx`: real visible rows and row navigation |
| AC-001.13 | `use-automations.test.tsx`: overlap in both settlement orders, newer empty, current/obsolete failures and pending flags; sibling refresh ordering |
| AC-001.14 | Real provider/store: same-workspace pending deduplication, shared cache/flags, simultaneous A/B, independent stores, unmount/remount and StrictMode |
| AC-001.15 | Initial failure empty/loaded, refresh failure retaining baseline, explicit recovery, obsolete failures harmless |
| Compatibility | `automations-slice.test.ts` and real hook: sequential create/update/remove/enable/disable, trigger result, one-time secret returned but absent from both stores; unrelated trigger/run fields intact; legacy initial-state and flat reference updates |
| Breadcrumb/sidebar | Real scoped breadcrumb name/empty fallback assertion in hook suite; existing settings breadcrumb/layout and shortcut catalog tests |

Every test uses distinct row IDs/names and deferred transport, asserts observable
state rather than private token values, and settles every started promise in
owned cleanup. Keep obsolete responses pending while checking current loading,
then settle them to prove they cannot alter accepted data or flags.

## Rendered integration and mobile parity

Mount real `StateProvider`/`createAppStore`, `SettingsSaveProvider`,
`TooltipProvider`, production `AutomationsListPage`, and real
`AutomationsTable`. Mock only automation transport; keep the browser-history
router and settings state real. Rerender page workspace A/B/A, settle B late,
assert A's unique row name/ID remains, B's row is absent, and clicking A's row
targets A's editor path. Include first-load and accepted-empty rendering and
restore history/storage on cleanup. Existing mocked-hook page tests remain
controls; they cannot establish the regression alone.

Mobile assessment: pure state/data normalization under unchanged desktop/phone
composition, copy, navigation, touch, scrolling, and breakpoints. The
mobile-parity unit/component exception applies. No ASCII composition or new
Playwright file is appropriate. No browser, build, or E2E runs are planned.

Public docs already describe workspace automation lists in
`docs/public/automation-and-mcp.md` and `docs/screenshots.md`. The repair restores
that contract without new instructions, labels, or API; internal docs suffice.

## Work orders

- [x] [Task 01: Scope and order automation list publication](task-01-scoped-list-publication.md)

One work order, wave 1, no dependencies, sequential in this same session.
Exact commands, meaningful RED/GREEN requirements, and prospective documentation
coverage preflight are in its Verification section.

## Resource and delivery checkpoints

Design cannot install dependencies or edit production/permanent tests, stage,
commit, or deliver. End WAITING and let ROOT poll this transcript/versioned plan;
parent callback queue is full, so no callback messages/questions/retries.

After explicit implementation release, use Node 24.21.0 at the supplied path,
`/bin/bash` with `login:false`, and one conditional pinned pnpm 9.15.9 frozen
install from `apps/` only if dependencies are missing. One local heavy command
at a time. Retain and actually join every returned handle before the next
dependent/heavy step. Lost/interrupted/timeout/no-tests results are NO PASS.
Checkpoint for ROOT direction on resource failure; no automatic retry.

Later delivery uses active hooks, Conventional Commits, push and ready PR under
the already authorized loop. Freeze the published SHA except required findings;
no optional polish or moving-main rebase. One owned all-terminal `pr-await`,
joined before replacement. Discover and require all six actual required checks
terminal clean. Require exact-head authenticated configured CodeRabbit App
347564 substantive full review of every actual changed file; inspect automatic
completed coverage before at most one necessary full request for a real gap.
ACK/skip is not semantic coverage. Inspect findings while CI runs and disposition
every actual thread/grouped finding; no optional extra reviewer waits. Hosted
failure requires exact logs/artifacts and ROOT explicit bounded retry/scope
authorization. Normal expected-head squash only, no admin/bypass; independently
verify merge SHA/tree/owned blobs/authoritative remote and join owned cleanup
before COMPLETE. Keep worktree/deps/proof for ROOT archive; foreign resources
remain untouched.

## Verification results

Implementation local checks: eight files / 55 tests passed, with changed hook
(23 tests) and page integration (two tests) rechecked after test-only lint/type
corrections. Focused ESLint, required typecheck, i18n check and ratchet passed.
See the work order for exact commands, joined handles, RED failures and logs.
Final documentation catalog/spec lint/diff and actual production-path coverage
preflight passed (`covered`, no errors). No active local process handle; hosted
delivery is still pending.

Design validation completed on 2026-10-05:

- `python3 scripts/list-docs.py validate`: PASS, 351 decisions and 1,348 specs.
- `python3 scripts/lint-spec-files.test.py`: PASS, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: PASS.
- Repository `validateCoverage` preflight over the four design files and four
  prospective production paths: PASS, `covered`, no errors.
- `git diff --check`: PASS. Status confirms only the owning requirement/design
  and new manifest/work order, all unstaged and uncommitted.
- Production source and read-only ROOT proof SHA256 still match supplied values.
  No reproduction replay, install, package command, production/permanent test
  change, staging, commit or active process handle.

Task 01 is `done`; the package is `implemented`. Local implementation evidence
is recorded above and in the work order. Delivery is not complete yet.

## Risks

- An instance-only guard would leave sibling refresh publication unordered.
- Flat cache compatibility must not become the source of scoped list output or
  erase an authoritative empty breadcrumb entry.
- Unmount cancellation would abandon a sibling's shared pending request; test
  store-owned settlement explicitly.
- The additive cache retains snapshots for visited workspaces until the store
  is discarded. No eviction policy or cross-tab freshness guarantee is added.
- Mutation/list concurrency remains outside this proof and repair; cache
  maintenance controls must not be presented as proving that stronger contract.
