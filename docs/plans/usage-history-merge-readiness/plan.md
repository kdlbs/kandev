---
created: 2026-11-01
status: in_progress
requirements:
  - REQ-COSTS-USAGE-HISTORY-005
  - REQ-COSTS-USAGE-HISTORY-007
  - REQ-COSTS-USAGE-IMPORT-005
system_design:
  - ../../specs/costs/system-design/usage-history.md
  - ../../specs/costs/system-design/usage-history-import.md
legacy_specs: []
---

# Implementation Plan: Usage History Merge Readiness

## Overview

Finish what blocks [core #3660](https://github.com/kdlbs/kandev/pull/3660) and [plugin #10](https://github.com/kdlbs/kandev-plugin-session-cost/pull/10) from merging.
Fix the two data problems first, because they change code the gate measures. Then split unrelated fixes, run the full database gate on the final code, update records, and finish the plugin after core merges.

Reviewed heads: core `15d0ddc`, plugin `6b103d2`. All core work lands on `feature/investigate-token-co-9dc`, the #3660 branch. All plugin work lands on the same branch name in the plugin repository. This package is uncommitted on `feature/review-usage-history-9c9`, which sits at the core head, so bring it onto the PR branch first.

### Rules for the implementing agent

- Follow the work orders in numeric order. Do not change scope.
- Stop and report when a work order says STOP, or when the same check fails after three focused attempts. Do not change benchmark fixtures, samples, deadlines, or the preservation rule beyond Task 02.
- Commit, push, and open PRs only if the implementation request explicitly authorizes them. Never force-push. Never merge a PR.
- Do not spawn subagents unless the implementation request authorizes them.

## Scope

### In scope

- Pre-upgrade native usage reaching retained history (blocker found on 2026-11-01).
- Deletion of tasks whose native usage cannot be retained.
- Accurate plugin messaging when retained usage is unavailable.
- Moving unrelated fixes to separate PRs.
- The full SQLite and PostgreSQL gate, the CI flake audit, and PR descriptions.
- The plugin's SDK pin and `min_kandev_version`.

### Out of scope

- Turning `features.usageHistory` on in `prod`, publishing plugin 0.5.0, and merging PRs. These stay user decisions.
- Live cost in the composer while the retained API is unavailable.
- Query tuning. If the gate fails, this package stops so the design can be revisited.
- Repairing gap rows on development installs that already ran an earlier branch build.

## Technical approach

- **Task 01:** in `BackfillUsageHistory` (`usage_history_migration.go`), call `BackfillUsageSourceAdoption` first. Delete the event's gap row after a successful retention in `persistScopedNativeUsageTx`.
- **Task 02:** add one shared check used by `preserveNativeUsageRowTx` and `requireNativeUsagePreservedTx` in `usage_history_native_preservation.go`.
- **Task 03:** cherry-pick unrelated commits to new branches from `origin/main`, then merge `main` back after they land.
- **Task 04:** run `BenchmarkUsageHistoryFiveYears` and `BenchmarkUsageHistoryPostgresFiveYears` with the C12 settings, detached from the calling tool.
- **Task 05:** audit exact-head CI and rewrite the PR description sections.
- **Task 06:** update `.kandev-sdk-ref` and `manifest.yaml` after core merges and a release exists.
- **Task 07:** change two translated plugin messages.

The [import design](../../specs/costs/system-design/usage-history-import.md) owns the behavior for Tasks 01, 02, and 07.

## ASCII UI preview

UI-01: Composer cost details and import card when the retained API is unavailable. AC-COSTS-USAGE-IMPORT-005.7. Only the message text changes. Copy is illustrative, but it must name both remedies. Phone uses the same wrapping text.

```text
+------------------------------------------------------+
| Session cost                                         |
| Retained usage isn't available. Update Kandev, or    |
| turn on Usage history in Settings > System > Feature |
| Toggles. Previous saved usage is preserved.          |
+------------------------------------------------------+
```

## Tests

| Criteria | Evidence |
| --- | --- |
| HISTORY-007.11 | `apps/backend/internal/task/repository/sqlite/usage_history_native_backfill_test.go`: `TestPreUpgradeNativeUsageAppearsAfterMigration` |
| HISTORY-007.12, 007.2 | `apps/backend/internal/task/repository/sqlite/usage_history_forked_native_test.go`: `TestUsageHistoryForkedNativeUsageDeletesWithGap`, `TestUsageHistoryUnprovenNativeBlocksDeletionBeforeAdoption` |
| HISTORY-005.2, 005.5 | Task 04 benchmark receipts |
| IMPORT-005.7 | Plugin `test/bundle.test.mjs`: unavailable-message case |

## E2E tests

No new browser flow. Task 05 audits the existing E2E suites on the final head. The plugin's installed-Host smokes rerun in Task 06 against the final pin.

## Work orders

- [x] [Task 01: Retain pre-upgrade native usage during migration](task-01-native-backfill-order.md)
- [x] [Task 02: Allow deletion of unprovable native usage with a gap](task-02-deletion-of-unprovable-usage.md)
- [ ] [Task 03: Split unrelated fixes out of #3660](task-03-split-unrelated-fixes.md)
- [ ] [Task 04: Run the full database gate](task-04-database-gate.md)
- [ ] [Task 05: Audit CI and refresh PR descriptions](task-05-ci-audit-and-pr-records.md)
- [ ] [Task 06: Finalize the plugin pin and minimum Host version](task-06-plugin-pin-and-floor.md)
- [x] [Task 07: Name both remedies when retained usage is unavailable](task-07-unavailable-message.md)

Order: 01, 02, 07, 03, 04, 05, then the user merges core, then 06. Task 07 is numbered last but runs early because it touches only the plugin.

## Verification results

Tasks 01 and 02 passed their targeted, package, race, and Go lint checks. Task 02's public documentation validators also passed. Task 07 passed the plugin's targeted tests and full `make test` gate. PostgreSQL-specific checks were skipped because no PostgreSQL 18.6 image or configured test DSN was available.

## Risks

- **Gate may fail.** C12 failed four five-year PostgreSQL cases, and later query changes have never been measured at that scale. Task 04 stops on failure; fixing it needs a stronger design pass.
- **Task 02 drops usage.** Events that stay ineligible are deleted with their task and leave only a workspace-level notice. Forked or transferred sessions are the main source. Events recorded inside a tracked turn are retained normally.
- **Split PRs add time.** Task 03 depends on other PRs merging first, and some fixes may be what keeps CI green. Keep a fix in #3660 and list it in Results if removing it breaks CI.
- **Flake rule changes.** Task 05 replaces the old zero-flake rule with "no flake in usage history code". Latest runs: #3660 had 2 unrelated flakes, `main` had 4.
- **Plugin release order.** Publishing 0.5.0 before the toggle is on in `prod` removes composer cost for existing users.
