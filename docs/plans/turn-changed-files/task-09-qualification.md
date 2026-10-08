---
id: turn-changed-files-09
title: Executor qualification, performance evidence, and public docs
status: in_progress
wave: 9
depends_on:
  - turn-changed-files-01
  - turn-changed-files-02
  - turn-changed-files-03
  - turn-changed-files-04
  - turn-changed-files-05
  - turn-changed-files-06
  - turn-changed-files-07
  - turn-changed-files-08
plan: plan.md
requirements:
  - REQ-TASKS-TURN-CHANGES-002
  - REQ-TASKS-TURN-CHANGES-003
  - REQ-TASKS-TURN-CHANGES-004
  - REQ-TASKS-TURN-CHANGES-006
  - REQ-TASKS-TURN-CHANGES-007
acceptance_criteria:
  - AC-TASKS-TURN-CHANGES-002.7
  - AC-TASKS-TURN-CHANGES-002.9
  - AC-TASKS-TURN-CHANGES-003.2
  - AC-TASKS-TURN-CHANGES-003.3
  - AC-TASKS-TURN-CHANGES-003.4
  - AC-TASKS-TURN-CHANGES-003.5
  - AC-TASKS-TURN-CHANGES-003.6
  - AC-TASKS-TURN-CHANGES-004.5
  - AC-TASKS-TURN-CHANGES-004.6
  - AC-TASKS-TURN-CHANGES-004.7
  - AC-TASKS-TURN-CHANGES-006.4
  - AC-TASKS-TURN-CHANGES-006.6
  - AC-TASKS-TURN-CHANGES-007.1
  - AC-TASKS-TURN-CHANGES-007.2
  - AC-TASKS-TURN-CHANGES-007.3
  - AC-TASKS-TURN-CHANGES-007.4
system_design:
  - ../../specs/tasks/system-design/turn-changed-files.md
---

# Executor qualification, performance evidence, and public docs

## Summary

Qualify actual executor lifetime behavior, measure bounded capture, and document the delivered feature without overstating coverage.

## Scope and owned files

- Focused container/SSH/Kind E2E scenarios, available Sprites/plugin executor qualification, and retention/restart integration tests.
- New capture/comparison/export benchmarks and deterministic representative Git fixtures.
- Bounded metric labels, cancellation/cleanup telemetry, and retention growth measurements.
- `docs/public/git-operations.md` and `docs/public/tasks-and-workflows.md` sections for preference, interval meaning, availability, and historical navigation.
- Scoped AGENTS.md updates only where new package/contract ownership makes existing guidance inaccurate.
- Package result records and a new `performance-results.md` under this plan, created only from measurements.

## Exclusions

No unrelated broad-suite audit, new public retention-control UI, speculative performance guarantees, or completion policy changes.

## Implementation acceptance

1. Promised history survives real ephemeral teardown and restart on supported executors; missing environments and unsupported capabilities remain explicit acceptance limitations.
2. Measured cases establish actual capture/comparison duration, bytes, failure behavior, retention growth, and cancellation bounds against proposed limits.
3. Public how-to guidance and the final report match observed behavior, preference defaults, content preservation, and remaining acceptance failures.

## Verification

New container specs must follow existing daemon-backed fixtures and agentctl capability readiness; do not merely seed ready summaries.
Run these commands sequentially after infrastructure is ready:

```bash
cd apps/backend
go test -trimpath -race ./internal/task/changes ./internal/agent/runtime/lifecycle -run 'TurnChangeExecutor|TurnChangeRetentionRestart|TurnChangeCleanup' -count=1
go test -trimpath ./internal/agentctl/server/process ./internal/task/changes -run '^$' -bench 'TurnCheckpoint|TurnChangeExport' -benchtime=10x -benchmem
```

```bash
cd apps/web
KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --project containers tests/git/turn-changed-files-executors.spec.ts
```

Add capability-specific external tests for Sprites/plugin executors where credentials and environments exist.
Report the exact command and blocker if an environment cannot run; shared unit tests do not prove its support.

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

## Performance protocol

Measure actual end-to-end service wall time, including Git admission and executor transport, alongside package benchmarks.
Use a 100-file small repository, a 20,000-file repository, large text/binary changes, multi-checkout capture, and shared-checkout contention.
Use warm/cold cases and ten observations per representative case after setup. Record median, p95, maximum, and failures.
Record Git/OS/CPU/filesystem/executor/database versions, tracked/untracked counts, changed bytes, new object bytes, and compressed retained bytes.
Include staged/committed changes and repeated turns to measure deduplication growth.
Use slow storage where available; controlled latency injection must be labeled synthetic rather than physical-storage evidence.
Also measure size-limit failures and Stop/Cancel deadline behavior. Do not report source inspection as performance evidence.

## Dependencies and risks

Depends on all preceding orders. Docker/SSH/Kind require a real daemon; Sprites can require credentials.
Final completion requires either passing promised executor acceptance or an explicit product-scope correction reviewed by the user.
Leave unverified cross-executor criteria open rather than calling partial support complete.
No new rendered UI belongs to this order; its existing UI coverage comes from 07 and 08.

## Results

Public Git and task guidance is implemented. The Docker retention E2E passed after removing the executor container and restarting the backend. The SSH retention E2E passed after removing the remote task checkout. Local capture/export benchmarks passed for all six recorded cases; updated ten-observation medians and maxima are in [performance-results.md](performance-results.md).

Still open: Kind-backed Kubernetes, Sprites, and plugin executor qualification; database-backed compressed-retention growth and deduplication; end-to-end transport timing; cold/slow storage; size-limit and cancellation-bound measurements; PostgreSQL-specific tests. The Kubernetes fixture currently seeds no repository, and no Sprites or plugin environment was exercised in this run. Do not treat the remaining executor matrix or performance criteria as complete.

### Review follow-up verification (2026-10-08)

Targeted review regressions passed: six focused Go package groups, seven Vitest files (107 tests), web typecheck, `i18n:check`, ESLint on the updated E2E files, and the desktop and mobile historical-turn E2Es. The managed desktop E2E run rebuilt backend and Vite assets successfully. No full test-suite rerun or cross-executor/performance qualification was performed for this review; all gaps listed above remain open, so this work order stays `in_progress`.

### PR remediation verification (2026-10-08)

- Changed-package Go lint and changed-file Web ESLint passed.
- `cd apps/backend && go test -race ./internal/task/changes` passed.
- `cd apps/backend && go test -race ./internal/task/repository/sqlite -run 'Test(TurnChange(Content|History|Set|Retention)|TurnRepositoryStart|FinalizeTurnChangeSet)'` passed.
- `cd apps/backend && go test ./internal/agentctl/server/process -run 'Test(TurnCheckpoint(CapturesExactIntervalWithoutMutatingUserGitState|CompareAndExportIgnoreMovedOrRemovedReachabilityRefs|ExportsImmutablePatchesAndRenderingBlobs|DeletesOnlyOwnedRefsWithAcceptedOIDCAS|ConcurrentCaptureSharesOneImmutableEndpoint)|ValidateTurnCheckpointCandidateBytesEnforcesEntryAndByteLimits)'` passed.
- `cd apps/web && pnpm exec vitest run components/task/chat/message-list-native.test.tsx components/task/chat/turn-changed-files-card.test.tsx components/task/task-changes-panel-historical.test.tsx hooks/domains/session/use-turn-changes.test.ts hooks/domains/session/use-turn-changes-pagination.test.tsx lib/state/slices/session/turn-changes-actions.test.ts lib/turn-changes/history-scope.test.ts lib/turn-changes/projection.test.ts lib/turn-changes/tree.test.ts lib/turn-changes/view-state.test.ts lib/api/domains/turn-changes-api.test.ts`: 11 files and 117 tests passed.
- `cd apps/web && pnpm exec tsc --noEmit` and `pnpm run i18n:check` passed.
- Desktop and mobile changed-files and settings E2Es passed after rebuilding the backend and E2E plugin fixture: two tests per viewport. The captured screenshots were refreshed and validated.
- No completed full-suite rerun or cross-executor/performance qualification was performed for this remediation. A broad SQLite race run was cancelled before completion; the focused SQLite and process tests above passed. All qualification gaps listed above remain open, so this work order stays `in_progress`.

An initial PR CI run found an invalid `binary` column identifier in PostgreSQL and stale generated settings-catalog snapshots; both are corrected (`is_binary` in the database schema and refreshed generated contracts). Eight work orders also had invalid list frontmatter for `depends_on`, which is corrected. Focused lifecycle, change-coordinator, SQLite repository, and transcript tests passed locally after these fixes. The PostgreSQL test DSN is absent locally, so the targeted PostgreSQL case skipped; fresh-head CI is the next check. No broad local suite was rerun. Kind/Kubernetes, Sprites, plugin executors, PostgreSQL-specific acceptance, transport timing, cold/slow storage, retention-growth evidence, and size/cancellation bounds remain open.

The current-head frontend CI run found that `show_turn_changed_files` was present in the mutable DTO inventory but absent from the settings-discovery field catalog. Added the field descriptor and independent coverage-inventory entry, regenerated the contract, and passed the focused backend catalog test and frontend catalog/coverage tests (20 tests). The corrected PR CI run remains pending; all qualification gaps listed above remain open.
