---
id: "01-conflict-observation"
title: "Preserve conflict observations in task status"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-CONFLICT-INDICATOR-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-PR-CONFLICT-INDICATOR-001.1
  - AC-INTEGRATIONS-GITHUB-PR-CONFLICT-INDICATOR-001.2
  - AC-INTEGRATIONS-GITHUB-PR-CONFLICT-INDICATOR-001.3
  - AC-INTEGRATIONS-GITHUB-PR-CONFLICT-INDICATOR-001.4
  - AC-INTEGRATIONS-GITHUB-PR-CONFLICT-INDICATOR-001.5
system_design:
  - ../../specs/integrations/system-design/github-pr-conflict-indicator.md
---

# Task 01: Preserve conflict observations in task status

## Summary

Record GitHub's raw conflict result independently from effective mergeability.
Publish it in linked-PR payloads and bounded task summaries so even compact
task rows can render a current warning.

## In scope

- Add an optional, synchronized conflict observation to GitHub `TaskPR`
  persistence, API/WS payloads, and change detection.
- Project the observation across open PRs into bounded task status.
- Update TypeScript wire types and compact projection mapping contracts.
- Test draft plus raw dirty, failure plus dirty, clearing, unknown, legacy
  dirty, several PRs, and terminal PRs.

## Out of scope

- Glyph rendering, localization, and browser checks.
- Changes to merge, auto-fix, queue, or polling decisions.

## Acceptance

- Draft normalization still applies to effective mergeability while an
  independently confirmed raw conflict survives.
- Authoritative conflict transitions update the stored PR and task summary;
  unknown observations do not claim a conflict.
- Compact summaries flag any open conflicted PR without carrying full PR data.

## Verification

```bash
(cd apps/backend && go test ./internal/github ./internal/task/statussummary ./internal/backendapp)
(cd apps/web && pnpm exec vitest run components/github/pr-task-icon.test.ts)
```

Run `pnpm install --frozen-lockfile` from `apps/` before the first frontend
command in a fresh worktree.

## Files likely touched

- `apps/backend/internal/github/models.go`
- `apps/backend/internal/github/service_pr_watch.go`
- `apps/backend/internal/github/store.go`
- `apps/backend/internal/github/service_test.go`
- `apps/backend/internal/github/store_taskpr_schema_drift_test.go`
- `apps/backend/internal/task/statussummary/model.go`
- `apps/backend/internal/task/statussummary/projector_pr.go`
- `apps/backend/internal/task/statussummary/projector_events.go`
- `apps/backend/internal/task/statussummary/projector.go`
- `apps/backend/internal/task/statussummary/rebuild.go`
- `apps/backend/internal/backendapp/status_summary_adapter.go`
- `apps/web/lib/types/github.ts`
- `apps/web/lib/types/task-status-summary.ts`
- `apps/web/lib/task-pr-info.ts`

## Dependencies

None.

## Risks

Association writes and event projection must not turn an unknown provider
result into a confirmed false. SQLite migration and round-trip paths must agree.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/integrations/requirements/github-pr-conflict-indicator.md)
- [System design](../../specs/integrations/system-design/github-pr-conflict-indicator.md)
- Existing GitHub sync/store and bounded status-summary tests.

## Results

The GitHub sync/store, status-summary, and backend app packages pass. Tests
cover draft plus dirty, initial draft association, unknown preservation,
authoritative clearing, and open-PR aggregation.

PR fixup also covers conflict evidence carried by passive draft snapshots and
conflict updates and clearing on unwatched PRs without replacing their richer
check and review aggregates. The mock provider also carries explicit conflict
observations through synthetic PR hydration. Verification passed with
`cd apps/backend && go test ./internal/github ./internal/task/statussummary ./internal/backendapp`.
