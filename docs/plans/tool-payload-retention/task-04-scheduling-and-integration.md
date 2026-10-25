---
id: "04-scheduling-and-integration"
title: "Schedule cleanup and verify the complete retention flow"
status: completed
wave: 4
depends_on:
  - "03-settings-and-transcript"
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003
acceptance_criteria:
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003.1
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003.2
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003.3
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003.4
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003.5
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003.6
system_design:
  - ../../specs/system-page/system-design/tool-payload-retention.md
---

# Task 04: Scheduling and Integration

## Summary

Connect the daily worker to application lifecycle and verify the complete
feature. Publish accurate documentation with the implementation.

## In scope

- Add daily due-time persistence, bounded continuation scheduling, operation
  serialization, startup delay, cancellation, and graceful shutdown.
- Recheck policy before every batch. Exercise restart during preparation, after
  approval, during cleanup, and after disable. Avoid catchup bursts.
- Integrate worker quiescence with restore/reset. Coordinate competing maintenance
  jobs and validate foreground write progress under generated database load.
- Verify all requirement mappings and browser flows from the earlier orders.
  Update public Data & Logs/backup documentation through `/docs-maintainer`.
- Reconcile the existing route design's save-contributor description. Record final
  scope, unsupported payload shapes, test evidence, and specification lifecycle.

## Out of scope

Production database maintenance, deployment, commit/push, automatic compaction,
and UI layout changes. Existing UI-01 through UI-04 remain the rendered contract.

## Acceptance

1. Enabled cleanup runs daily after readiness, respects work bounds, and resumes
   safely after interruption. Disable prevents subsequent committed batches.
2. A generated large database demonstrates bounded memory/transaction behavior,
   foreground writes between batches, and no full scan in page-status polling.
3. Desktop, phone, permissions, backup failure, and replay checks pass. Documents
   describe actual supported behavior and identify any remaining implementation gaps.

## Tests and verification

Use fake clocks and deterministic barriers for scheduling and restart tests.
Use temporary databases for scale/compaction comparisons. Record logical bytes,
freelist changes, and compacted size separately. Do not infer a backup speedup.

Run independently from the repository root:

```bash
(cd apps/backend && go test ./internal/system/... ./internal/task/models/... ./internal/task/repository/sqlite/... ./internal/task/service/...)
(cd apps/backend && go test -race ./internal/system/toolretention/...)
make -C apps/backend lint
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/system/tool-payload-retention.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/system/mobile-tool-payload-retention.spec.ts)
(cd apps/web && pnpm e2e:run --project auth tests/auth/system-data-storage-member-gating.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/auth/mobile-system-data-storage-member-gating.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Confirm test discovery. Run only changed relevant suites once earlier checks
pass, unless new integration changes justify repetition.

## Files likely touched

- `apps/backend/internal/system/toolretention/scheduler.go` and integration tests.
- Backend startup/shutdown and system restore/reset composition.
- Existing desktop/mobile/auth E2E files from Task 03.
- Relevant `docs/public/` pages, specification pair, route design, and this plan.

## Dependencies and parallelism

Depends on Task 03. The primary session owns final verification; an authorized
native subagent runs the browser suites sequentially.

## Inputs

Read the plan's test matrix, previous work-order results, and lifecycle design.
Use `/tdd` for timer/runtime changes and `/docs-maintainer` for public docs.

## Risks

The job tracker is in memory. It cannot replace durable preparation and progress.
Scheduling errors must not make readiness depend on scanning a large database.

## Results

Implemented the single runtime worker, one-minute startup delay, daily due time,
100ms batch yields, 30-second work bursts, bounded continuation, and shutdown.
Restore/reset stop the worker before database quiescence. Changed-message events
publish after commit through the existing task service.

- Fake-clock and restart tests cover due-time persistence, interrupted backup,
  Start/Stop ownership, idle-to-work burst limits, stale cancellation, and
  committed progress. The package uses goleak for worker lifetime checks.
- Full backend lint reports zero issues. SQL portability checks pass with narrow
  SQLite-only date validation exemptions. Broad system/task/persistence tests pass.
- Public operations guidance explains activation, supported removal, partial
  estimates, whole-database recovery, unchanged backups, and explicit compaction.
  The route design records the separate tool-payload save contributor.
- Documentation checks pass: 268 decisions and 904 specs validate; all 36 spec
  validator tests pass; all specs lint; 62 public-doc validator tests pass;
  all 46 published pages validate.

Final build, browser, admission, and storage fixture evidence is in the plan.

Review remediation makes the next serialized worker tick recover an orphaned
running preparation after its backup invocation ends. Recovery marks preparation
failed and retryable, clears approval, and checks the observed revision and
operation ID inside the transaction. It does not repeat the backup or approve
cleanup. The failure handler also terminates preparation when the backup
operation is already terminal. Tests cover failed final writes, cleanup-start
failure, cancelled request persistence, and replacement policy/operation safety.
