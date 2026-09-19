---
id: "03-run-observation"
title: "Run and activity names"
status: done
wave: 3
depends_on: ["02-taskless-lifecycle"]
plan: "plan.md"
requirements:
  - REQ-OFFICE-RUN-OBSERVATION-001
  - REQ-OFFICE-DASHBOARD-001
acceptance_criteria:
  - AC-OFFICE-RUN-OBSERVATION-001.1
  - AC-OFFICE-RUN-OBSERVATION-001.2
  - AC-OFFICE-RUN-OBSERVATION-001.3
  - AC-OFFICE-RUN-OBSERVATION-001.4
  - AC-OFFICE-RUN-OBSERVATION-001.5
  - AC-OFFICE-DASHBOARD-001.7
  - AC-OFFICE-DASHBOARD-001.8
system_design:
  - ../../specs/office/system-design/run-observation.md
---

# Task 03: Run and activity names

## Summary

Expose readable run, skill, adapter and activity identity with safe historical fallbacks. Keep stable IDs for navigation and diagnostic use.

## In scope

Own additive skill label snapshot migration, actual invocation projection, workspace-scoped
activity name enrichment, normalization and UI rendering. Reuse the same activity
projection for dashboard and workspace feed. Include missing and renamed entities,
legacy runs and routed provider fallback. Localize authored fallbacks.

## Out of scope

Other work orders, unrelated refactors, live-instance changes and publication.

## Acceptance

- Named agent/skill/task labels render on direct entry without sidebar preloading; missing and foreign entities degrade safely.
- Adapter/model are actual recorded invocation evidence; legacy unknown values are explicit and new skill snapshots retain labels.
- Desktop/mobile long-name and missing-data E2E pass; IDs remain correct in links and no per-row request fanout is added.

## ASCII UI preview

[Full preview](plan.md#ascii-ui-preview).

```text
UI-01: Agent Runs / Activity, populated and missing identity
Desktop
[Agents > CEO                         Refresh pause  Pause workspace]
[Run status | CEO | duration | cost]
[Adapter: Codex   Model: <actual model>]
[Skills: Planning  v...  hash...]
[Activity: CEO recorded a decision on KAN-14 Build report]

Phone
[< CEO Runs                  Workspace actions]
[Status / duration / cost]
[Adapter: Codex]
[Skills: Planning / version / hash]
[CEO recorded a decision]
[KAN-14 Build report]

Missing: [Skill unavailable (04957907)]
Before invocation: [Adapter: Not started]
```

Control grouping and phone composition are required; spacing and wording are illustrative.

## Regression evidence

Add TestRunDetailActualInvocation, TestRunSkillSnapshotLabelRetention, TestPostgresRunSkillLabelMigration and TestActivityLabelsWorkspaceScoped. Extend runtime-panel/activity-row tests with rename/delete, legacy, scheduler actor and long-name cases.

## Verification

Run from the repository root; every command is independently rooted. New test files
named below are outputs of this work order. Record red/green evidence.

```bash
(cd apps/backend && go test ./internal/office/dashboard ./internal/office/repository/sqlite -run 'RunDetail|RunSkill|Activity.*Label|Activity.*Name|PostgresRunSkill' -count=1)
(cd apps/backend && test -n "$KANDEV_TEST_POSTGRES_DSN" && go test ./internal/office/repository/sqlite -run PostgresRunSkill -count=1)
(cd apps/web && pnpm exec vitest run 'app/office/agents/[id]/runs/components/runtime-panel.test.tsx' 'app/office/agents/[id]/runs/components/run-header.test.tsx' app/office/workspace/activity/activity-row.test.tsx lib/api/domains/office-activity-normalize.test.ts)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check)
make build-backend
make build-web
(cd apps/web && pnpm e2e:run --project=chromium e2e/tests/office/run-observation.spec.ts e2e/tests/office/runtime-skills.spec.ts e2e/tests/office/activity-page.spec.ts)
(cd apps/web && pnpm e2e:run --project=mobile-chrome e2e/tests/office/mobile-run-observation.spec.ts)
git diff --check
```

## Files likely touched

- `apps/backend/internal/office/dashboard/run_detail.go`
- `apps/backend/internal/office/dashboard/dto.go`
- `apps/backend/internal/office/dashboard/service.go`
- `apps/backend/internal/office/repository/sqlite/`
- `apps/backend/internal/office/service/scheduler_integration.go`
- `apps/web/lib/api/domains/office-runs-api.ts`
- `apps/web/lib/api/domains/office-activity-normalize.ts`
- `apps/web/lib/state/slices/office/types.ts`
- `apps/web/app/office/agents/[id]/runs/components/`
- `apps/web/app/office/workspace/activity/activity-row.tsx`
- `apps/web/src/locales/`
- `apps/web/e2e/tests/office/run-observation.spec.ts (new)`
- `apps/web/e2e/tests/office/mobile-run-observation.spec.ts (new)`

## Dependencies

02-taskless-lifecycle

## Risks

Historical labels and adapters may be unknowable; do not fabricate historical truth from mutable profiles. Keep enrichment within workspace ownership and bounded query count.

## Parallelism

`sequential`

## Inputs

Read linked requirements/designs in full, the plan evidence, scoped AGENTS.md,
TDD guidance and the adjacent existing tests before changing code. UI tasks also
read mobile-parity and E2E fixture guidance.

## Results

- Added persisted skill display labels and source metadata, actual invocation projection, workspace-scoped activity label enrichment and safe legacy fallbacks.
- Updated desktop/mobile run, skill and activity rendering while preserving stable IDs and avoiding per-row fetch fanout.
- Added backend label tests and frontend regressions. The focused frontend suite passed 5 files and 25 tests; typecheck and the complete i18n gate passed. Postgres migration evidence remains skipped without `KANDEV_TEST_POSTGRES_DSN`.
