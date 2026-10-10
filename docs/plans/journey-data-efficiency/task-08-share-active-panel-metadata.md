---
id: "08-share-active-panel-metadata"
title: "Share metadata reads across active task panels"
status: complete
wave: 8
depends_on:
  - "04-scope-visible-session-demand"
  - "07-share-navigation-refresh-ownership"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-JOURNEY-LOADING-003
  - REQ-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001
acceptance_criteria:
  - AC-PLATFORM-JOURNEY-LOADING-003.1
  - AC-PLATFORM-JOURNEY-LOADING-003.2
  - AC-PLATFORM-JOURNEY-LOADING-003.3
  - AC-PLATFORM-JOURNEY-LOADING-003.4
  - AC-PLATFORM-JOURNEY-LOADING-003.5
  - AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.12
system_design:
  - ../../specs/platform/system-design/journey-data-loading.md
---

# Task 08: Share metadata reads across active task panels

## Summary

Two visible chats sharing a task/profile reuse task/repository/settings reads. Different scopes or request options cannot receive the wrong cached projection.

## In scope

Route repository, workspace/workflow, settings, CI-option, and MCP-config consumers through existing or narrowly scoped shared owners. Preserve response-option distinctions such as includeScripts.
Remove duplicate route enrichment and per-panel requests. Reuse agent-list and integration-health owners without broad cache-library migration. Keep metadata loading tied to actual visible controls and explicit consumers.
Complete the evidence comparison for these integrated journeys using the same fixtures and production builds. Extend existing journey E2E captures with per-resource generation counts; rerun the seeded HTTP benchmark under idle and write load. Record remaining 503/health failures rather than claiming universal recovery.

## Out of scope

Other work orders, live installation mutation, unrelated refactors, pool increases, new runtime flags, and deployment.

## Acceptance

- Two visible chats sharing a task/profile reuse task/repository/settings reads. Different scopes or request options cannot receive the wrong cached projection.
- Production-build desktop and phone traces satisfy resource budgets and subscription gates, with no growing requests after repeated navigation or reconnect.
- Ten final warm benchmark samples per mode are compared with Task 01 baseline. Structural gates pass, and any repeatable route/health regression is resolved.

## ASCII UI preview

[UI-02: full composition and states](plan.md#ascii-ui-preview). Applies to the acceptance IDs in this work order.

```text
UI-02 desktop: [Agent A | Agent B | Agent C] [optional visible split]
               [Visible chat + draft]        [Visible chat + draft]
UI-02 phone:   [Task / session picker]
               [One chat scroll region]
               [Composer + retained draft]
```

Existing controls and scroll ownership remain. Detail demand follows visibility, not mounting or keyboard focus. Preview spacing is illustrative.

## Verification

Use TDD. New test filenames and named methods below are planned deliverables, not existing passing evidence.
Run from the repository root. Install `apps/` dependencies first only if absent. Each command is independently rooted.

```bash
(cd apps/web && pnpm exec vitest run hooks/journey-metadata-resources.test.ts hooks/domains/settings/agent-list-resource.test.ts hooks/domains/session/environment-live-resource.test.ts hooks/domains/github/use-task-ci-options.test.tsx lib/ssr/session-page-state.test.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/task/journey-boot-loading.spec.ts tests/session/visible-session-demand.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-journey-boot-loading.spec.ts tests/session/mobile-visible-session-demand.spec.ts)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/backendapp -run '^$' -bench '^BenchmarkJourneyReadLoad$' -benchtime=1x -count=10)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint --max-warnings 0 hooks/domains/settings/agent-list-resource.ts hooks/domains/session/environment-live-resource.ts hooks/domains/session/use-session-mcp.ts hooks/domains/github/use-task-ci-options.ts lib/ssr/session-page-state.ts hooks/journey-metadata-resources.ts hooks/journey-metadata-resources.test.ts e2e/tests/task/journey-boot-loading.spec.ts e2e/tests/task/mobile-journey-boot-loading.spec.ts e2e/tests/session/visible-session-demand.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the web typecheck and targeted ESLint for every changed TS/TSX file after the listed tests. If localized copy changes, run `pnpm run i18n:check` and `pnpm run i18n:ratchet` from `apps/web`. Managed E2E commands build fresh assets and enforce resource limits.

## Files likely touched

- `apps/web/src/kanban-route.tsx`
- `apps/web/src/spa-routes.tsx`
- `apps/backend/internal/backendapp/boot_state_task_detail.go (integrated regression and extraction)`
- `apps/backend/internal/backendapp/boot_state_task_projection.go (extraction)`
- `apps/backend/internal/task/service/service_status_summary_reconcile.go (extraction)`
- `apps/backend/internal/task/service/service_status_summary_rebuild_input.go (extraction)`
- `apps/backend/internal/task/repository/sqlite/sidebar_task_query_snapshot.go (flat-tree planner regression)`
- `apps/backend/internal/task/repository/sqlite/sidebar_task_query_snapshot_test.go`
- `apps/web/hooks/domains/settings/agent-list-resource.ts (reuse pattern/owner)`
- `apps/web/hooks/domains/session/environment-live-resource.ts (preserve existing owner)`
- `apps/web/hooks/domains/session/use-session-mcp.ts`
- `apps/web/hooks/domains/github/use-task-ci-options.ts`
- `apps/web/lib/ssr/session-page-state.ts`
- `apps/web/hooks/journey-metadata-resources.ts (new narrow owners if required)`
- `apps/web/hooks/journey-metadata-resources.test.ts (new)`
- `apps/web/e2e/tests/task/journey-boot-loading.spec.ts`
- `apps/web/e2e/tests/task/mobile-journey-boot-loading.spec.ts`
- `apps/web/e2e/tests/session/visible-session-demand.spec.ts`
- `docs/plans/journey-data-efficiency/implementation-evidence.md (new results)`

## Dependencies

[Task 04](task-04-scope-visible-session-demand.md), [Task 07](task-07-share-navigation-refresh-ownership.md)

## Risks

Broad resource keys can mix compact and rich response options. Integration/provider failures must retain their current independent recovery behavior.

## Parallelism

`sequential`

## Inputs

- [Plan, contract inventory, and test mapping](plan.md).
- [Journey loading design](../../specs/platform/system-design/journey-data-loading.md).
- [Measured baseline](evidence.md) and `evidence/` artifacts.
- Read the owned source and nearby tests before the first edit. Preserve existing user changes.

## Results

Implemented shared repository, workspace/workflow, settings, CI-option, and
MCP reads, including route bootstraps. Tests preserve response projections,
settings overlays, retry, authorization generations, and other consumers.
The synchronous final-release regression failed before and passed after the
fix. Production desktop and phone traces preserve bounded requests through
repeated selection and reconnect. Desktop split chats remain supported.

Integrated checks also fixed hidden prompt subscriptions, compact sibling
permission freshness, and the repeatable flat-tree SQLite planner regression.
The flat-tree test failed before and passed after removing scratch ANALYZE.
Native sidebar reader, cancellation, cleanup, queue, and scale checks passed.
No main-database migration or PostgreSQL query change was required.

Ten warm HTTP fixture samples per mode match the Task 01 baseline. Write-loaded
route p50 fell from 858.458 to 1.460 ms. The final 10,000-task boot carries
1,025,986 bytes for the board and 265,602–270,306 for detail. Eight of ten
write-loaded health probes still hit deadlines. This is a recorded remaining
writer-health limit, not evidence of universal 503 recovery. See
[implementation evidence](implementation-evidence.md#task-08-shared-metadata-and-integrated-evidence)
and its retained browser, route, and health data for commands and limitations.

Final builds, backend race suites, PostgreSQL 18 parity, and health/admission
regressions passed. Frontend verification passed 665 tests, TypeScript, and
ESLint across 120 changed files. Backend lint reports zero issues. Documentation
validation passed. Owned temporary runtimes and data were removed.

### Review correction

Review corrections retain the metadata owner and its post-response scope checks. Combined regression and browser validation now also covers task-session scope retirement, turn-window races, bounded client navigation, and board visibility ownership. Corrected source fingerprints are recorded separately from historical benchmark captures. The eight write-loaded probe deadlines, unresolved historical writer ownership, and separate agentctl filesystem failures remain limitations. See the [review correction results](implementation-evidence.md#review-remediation).
