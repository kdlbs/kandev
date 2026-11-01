---
created: 2026-11-01
status: in_progress
requirements:
  - REQ-COSTS-USAGE-IMPORT-001
  - REQ-COSTS-USAGE-IMPORT-002
  - REQ-COSTS-USAGE-IMPORT-003
  - REQ-COSTS-USAGE-IMPORT-004
  - REQ-COSTS-USAGE-IMPORT-005
  - REQ-COSTS-USAGE-HISTORY-002
  - REQ-COSTS-USAGE-HISTORY-004
  - REQ-COSTS-USAGE-HISTORY-005
  - REQ-COSTS-USAGE-HISTORY-006
  - REQ-COSTS-USAGE-HISTORY-007
system_design:
  - ../../specs/costs/system-design/usage-history-import.md
  - ../../specs/costs/system-design/usage-history.md
  - ../../specs/costs/system-design/token-usage.md
legacy_specs: []
---

# Implementation Plan: Usage History Import and PR Remediation

## Overview

Let users import tokscale history for existing sessions from the Session Cost settings page, with visible progress, and resolve the review findings on [core #3660](https://github.com/kdlbs/kandev/pull/3660) and [plugin #10](https://github.com/kdlbs/kandev-plugin-session-cost/pull/10).

The order is: make existing sessions eligible, stop one session blocking the others, rebuild the import, add the UI, then clean up the pre-release API, add the release toggle, and close the delivery gates.
Isolation comes before the import rebuild because it removes the live collection outage on its own.
API removal changes the SDK tree, so the plugin's final SDK pin comes last.

Reviewed baselines: core `a0d13b9`, plugin `8998081`. All work lands on the existing PR branches, both named `feature/investigate-token-co-9dc`. This package is written on top of the core head because the retained usage specifications exist only there.

## Scope

### In scope

- One-time adoption of existing sessions in core, with an [ADR](../../decisions/2026-11-01-usage-source-adoption.md).
- Per-session isolation in the collector, import, and composer.
- A two-step, resumable, queued import, and its progress card on desktop and phone.
- Review findings: unused write API, raw Host errors, same-binary compatibility shims, unbounded snapshot rows, the release toggle, PR scope and evidence size, open PR gates, stale comments, the SDK pin, and the minimum Host version.

### Out of scope

- Remote transcript reads, reconstruction of deleted data, and a one-click import of every workspace.
- A session-count preview before Start, and any additional plugin locale beyond the existing `en` and `pt-pt`.
- Promoting the release toggle to on in `prod`, merging the PRs, and publishing the plugin. These need separate user decisions.

## Technical approach

The [import design](../../specs/costs/system-design/usage-history-import.md) owns adoption, the worker, its lifecycle, and the status contract.

**Core adoption (Task 01).** Add `BackfillUsageSourceAdoption` as the last step of `BackfillUsageHistory`, with a `source-adoption` cursor and `usage_collection_contexts.evidence`. `prepareUsageCollectionTargetsTx` stays not ready until the cursor completes.

**Isolation (Task 02).** In `server/collector.go`, skip per-session errors from `resolveUsageTarget` and recheck active skipped sessions. In `server/source_preflight.go`, add `source_ineligible`. In `ui/bundle.js`, add the composer copy.

**Import (Tasks 03 to 05).** Rewrite `server/import.go` around a persisted `scanning`, `daily`, `completed` phase model and a schema version 3 checkpoint. Add the queue and restart handling, then extend `historicalImportView` and `makeHistoricalImportSettings`.

**Pre-release API removal (Tasks 07 and 08).** Remove `UsageReader.UpsertBatch`, `UsageReader.List`, the `UpsertSessionUsage` RPC, and `usageReader.UpsertBatch`. Then remove the `UsageRepository.UpsertSessionUsage` production path and the `normalizeLegacyUsage*` helpers, moving test fixtures to `Report` admission or a test helper. No released SDK or plugin has ever used this write path: it is absent from base `aebac5d`, and session-cost uses `Report`.

**Shims and retention (Task 09).** Remove the `httpGetTokenUsage` empty-report fallback, `listSessionsForPluginFallback`, and the `ColumnExistsContext`/`TableExistsContext` probes in target capture. Bound `usage_collection_snapshots` by deleting a source's older completed snapshots when it creates a new one for the same scope. A snapshot in use by an import stays valid while it is that source's newest for the scope.

**Release toggle (Task 10).** Add `features.usageHistory` and `KANDEV_FEATURES_USAGE_HISTORY`, restart required, with `prod: "false"`, `dev: "false"`, and a documented `e2e: "true"` exception because the retained usage suites depend on it. Gate native admission, preservation, `UsageHistoryWorker`, `/stats/token-usage`, its navigation entry, the report route, `SetUsageSource`, and `UsageHistory()`. Erasure and live-link detachment stay unconditional, so `EraseWorkspaceUsageTx` and `CompleteTaskUsageTx` keep running while it is off.

**Delivery (Tasks 11 to 13).** Extract unrelated fixes into their own PRs, and remove raw evidence with a normal commit. Do not force-push: GitHub keeps `refs/pull/3660/head` and its ancestors, so existing evidence stays readable at permanent commit links. Close the PostgreSQL and flaky-test gates, then update the plugin's SDK pin and `min_kandev_version`.

## ASCII UI preview

The structure, states, and actions below are requirements. Wording, spacing, and numbers are illustrative. The card keeps its existing primitives and one-column layout, so phone uses the same composition with full-width wrapping actions and 44 px touch targets. Status text is a polite live region.

UI-01: Plugin settings, import card, daily step (Settings > Plugins > Session Cost). AC-003.1, .2, .3, .5.

```text
+----------------------------------------------------------+
| Import historical usage                                  |
| Read local tokscale sessions into Token Usage.           |
|                                                          |
| Workspace  [ Acme                                  v ]   |
|                                                          |
| Step 2 of 2: Adding daily history                        |
| [==================>                  ] 142 of 238 days  |
| Working on 2026-03-12 - about 40 min left                |
|                                                          |
| Sessions found            312                            |
|   Imported                287                            |
|   No local transcript      21                            |
|   Skipped (not safe)        4                            |
|                                                          |
| All-time totals are ready. Charts fill in as days finish.|
| You can leave this page.                                 |
|                                                          |
| [ Cancel import ]                                        |
+----------------------------------------------------------+
```

UI-02: Card status region and action by state. AC-002.8, 003.1, 003.6, 003.7, 004.4, 005.3.

```text
Not started  History has not been imported.          [ Import history ]
Preparing    Preparing session metadata...  [~~~~~]  [ Cancel import ]
Step 1       Step 1 of 2: Reading all-time totals    [ Cancel import ]
             [~~~~~] (no percentage yet)
Queued       Waiting for another import to finish.   [ Cancel import ]
Failed       Import stopped on 2026-03-12.           [ Resume ]
             Check tokscale and try again.
Disabled     Enable statistics collection to import. [ Resume ]
Completed    Imported 287 of 312 sessions.           [ Import again ]
             21 no local transcript, 4 not safe.
             Last imported 2026-11-01 14:02
```

UI-03: Composer cost details for an ineligible session. AC-005.5. Only this message region changes.

```text
+----------------------------------------------+
| Session cost                                 |
| Cost can't be attributed safely for this     |
| session because its transcript may include   |
| other work. Saved usage stays available.     |
+----------------------------------------------+
```

## Tests

| Criteria | Evidence |
| --- | --- |
| IMPORT-001.1 to .7 | `apps/backend/internal/analytics/repository/sqlite/usage_history_source_adoption_test.go`: `TestSourceAdoption*`, on SQLite and PostgreSQL |
| IMPORT-002.1 to .8 | `server/import_engine_test.go`: `TestImportEngine*`; core `usage_history_import_sequence_test.go`: `TestLifetimeThenDatedImportCountsOnce` |
| IMPORT-003.1 to .3, .6, .7 | `server/import_status_test.go`: `TestImportStatus*`; `test/bundle.test.mjs` import-card cases |
| IMPORT-004.1 to .6 | `server/import_lifecycle_test.go`: `TestImportLifecycle*`, including the blocked composer waiter case |
| IMPORT-005.1, .2, .6 | `server/collector_isolation_test.go`: `TestCollectorIsolation*` |
| IMPORT-005.3 | `server/import_engine_test.go`: `TestImportEngineSkipsIneligibleTarget` |
| IMPORT-005.4, .5 | `server/source_preflight_test.go`: `TestComposer*`; `test/responsive-bundle.test.mjs` |
| HISTORY-002.5 | `apps/backend/internal/plugins/host_usage_history_test.go`: `TestUsageHostErrorsArePublicOnly` |
| HISTORY-006.5 | `apps/backend/internal/analytics/handlers/usage_history_test.go`: unavailable repository returns an error, not an empty report |
| HISTORY-007.8 to .10 | `apps/backend/internal/backendapp/usage_history_toggle_test.go`: `TestUsageHistoryToggle*`; `apps/web/lib/state/slices/features/features-contract.test.ts` |

## E2E tests

| Flow | Criteria | File and project |
| --- | --- | --- |
| Pre-upgrade session imports with progress, survives a restart, and appears in Token Usage on desktop and phone | IMPORT-001.1, 002.2, 002.4, 003.1, 003.2, 003.5, 004.1 | Plugin `test/historical-import-smoke.spec.mjs`, projects `chromium` and `mobile-chrome` in `test/host-action-smoke.playwright.config.mjs` |
| Unsafe session is skipped while others complete, and the composer shows the safe-attribution message | IMPORT-001.2, 005.1, 005.3, 005.5 | Same file |
| Second workspace queues while the first continues | IMPORT-003.6, 004.3 | Same file |
| Statistics are unavailable when the toggle is off | HISTORY-007.8 | Core `apps/web/e2e/tests/layout/usage-history-toggle.spec.ts`, `chromium` and `mobile-chrome` |

The installed-Host flows use a fixture tokscale command, so they do not measure real tokscale run time.

## Work orders

- [x] [Task 01: Adopt existing sessions as eligible sources](task-01-source-adoption.md)
- [x] [Task 02: Isolate ineligible sessions](task-02-session-isolation.md)
- [x] [Task 03: Rebuild the two-step import engine](task-03-import-engine.md)
- [x] [Task 04: Add import queue and restart recovery](task-04-import-lifecycle.md)
- [x] [Task 05: Show import progress in settings](task-05-import-progress-ui.md)
- [x] [Task 06: Verify import end to end and update docs](task-06-import-e2e-docs.md)
- [x] [Task 07: Remove the unused Host write API](task-07-remove-host-write-api.md)
- [x] [Task 08: Retire the repository legacy write path](task-08-retire-legacy-write-path.md)
- [x] [Task 09: Remove compatibility shims and bound snapshots](task-09-shims-and-snapshots.md)
- [x] [Task 10: Add the usage history release toggle](task-10-release-toggle.md)
- [x] (moved) [Task 11: Narrow PR scope and remove raw evidence](task-11-pr-scope-evidence.md)
- [x] (moved) [Task 12: Close the PostgreSQL and flaky-test gates](task-12-close-pr-gates.md)
- [x] (moved) [Task 13: Finalize the plugin SDK pin and compatibility](task-13-plugin-release-readiness.md)

Dependency order: 01 before 03. 02 before 03. 03 before 04. 04 before 05. 01 to 05 before 06. 07 before 08. 08 and 09 before 10. 01, 07, 08, 09, and 10 before 11. 11 before 12. 06, 07, and 12 before 13.
Wave 1 candidates: 01 and 02 (different repositories). Wave 2 candidates: 03 and 07 (different repositories). Waves do not authorize subagents.

## Verification results

Remaining delivery work moved to the [merge readiness package](../usage-history-merge-readiness/plan.md) on 2026-11-01.


Core tasks 01, 07, 08, 09, and 10 are implemented and verified on SQLite.
Task 01 uses `TestSourceAdoption*` on SQLite and the PostgreSQL-gated adoption test.
Task 03's core proof is `TestLifetimeThenDatedImportCountsOnce`; the plugin engine tests cover one lifetime report, one dated report per active day, skips, reclassification, and unchanged reruns.
Tasks 02, 03, 04, and 05 are implemented in the plugin checkout.
Plugin validation passed `make test`, including Go tests, `node --check ui/bundle.js`, package verification, release-version checks, and 71 Node tests. Installed-Host historical-import E2E passed on `chromium` and `mobile-chrome` with retries disabled, using a non-empty synthetic tokscale report; both flows asserted imported Token Usage values, queueing, restart recovery, and safe handling of an ineligible session.
The real tokscale timing sample covered 5,193 sessions across 129 days. The slowest sampled lifetime report took 72.01 seconds, a warm lifetime report took 24.56 seconds, and a bounded-day report took 18.58 seconds; all were below the 120-second timeout. This sample is machine-specific and does not guarantee the same time for larger histories.
The web typecheck, ESLint, statistics Vitest suites, i18n check, specification validation, and Go lint on the changed files passed. The core toggle E2E passed on `chromium` and `mobile-chrome`; the focused legacy-binding fixture test, harness lint, public-doc validation, and docs validation also passed.
Review remediation added regression coverage for disabled Resume, refused starts on finished checkpoints, transient config retries, status polls that create no snapshot, shared-transcript indexing, open-session date ranges, percentage progress, committed-only adoption metrics, and off-period catch-up after the toggle is re-enabled.
Task 06 is complete: installed-Host desktop and phone flows passed, real tokscale timing was recorded, and public documentation was updated.
Task 11 removed raw evidence from the current diff and changed its summary links to the permanent evidence commit, but unrelated fixes have not all been extracted into separate PRs. Task 12 remains open for the full PostgreSQL gate and zero-FLAKY audit.
Task 13 remains blocked on a merged core release and its final SDK pin and `min_kandev_version`.

## Risks

- **tokscale run time varies by machine and history.** Each report has a 120-second timeout. The measured sample stayed below it, but larger histories may take longer.
- **Adoption reads current ownership.** A cross-organization move with no audit row cannot be detected. The [ADR](../../decisions/2026-11-01-usage-source-adoption.md) records the limits.
- **Overlap with native usage.** Imported sessions that also have native events can show as incomplete where equivalence is unproven. This is correct under AC-COSTS-USAGE-HISTORY-004.2, but users may read it as an import problem.
- **Toggle off stops preservation.** Deletions while the toggle is off lose usage that was not yet retained. AC-COSTS-USAGE-HISTORY-007.10 makes this a disclosed gap, and promotion remains a user decision.
- **Fixture migration size.** Task 08 touches about 20 test files, and a rewrite that weakens them would hide regressions. It keeps existing assertions.
- **Forked or transferred sessions with native usage.** [Merge readiness Task 02](../usage-history-merge-readiness/task-02-deletion-of-unprovable-usage.md) replaces the permanent deletion block with a recorded gap.
- **Moving base.** Main has advanced beyond the reviewed base. Reconcile before each PR push.
- **Full PostgreSQL gate may keep failing.** Task 12 cannot finish until it passes with unchanged fixtures, and that may require query work beyond this package.
