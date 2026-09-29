---
id: "06-shared-pr-feedback"
title: "Share PR feedback across task surfaces"
status: complete
wave: 6
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SESSION-REFRESH-EFFICIENCY-005
acceptance_criteria:
  - AC-UI-SESSION-REFRESH-EFFICIENCY-005.1
  - AC-UI-SESSION-REFRESH-EFFICIENCY-005.2
  - AC-UI-SESSION-REFRESH-EFFICIENCY-005.3
system_design:
  - ../../specs/ui/system-design/session-refresh-efficiency.md
---

# Task 06: Share PR feedback across task surfaces

## Summary

Give PR detail, status, and popover readers one scoped request owner. Investigate the remaining single-request header latency separately.

## In scope

- Add a store/auth/workspace/PR-keyed feedback coordinator and migrate detail, background status, popover, and cache consumers together.
- Join same-generation reads, queue one follow-up on real invalidation, retain same-PR data on errors, and bound retention.
- Correlate one isolated backend feedback request with upstream stages. Report the measured stage; propose a separate server remedy only if evidence warrants it.

## Out of scope

No backend API, deployment, dependency replacement, or general cache migration.

## Acceptance

`three consumers share one deferred request`, `invalidation queues one follow-up`, `workspace switch rejects prior feedback`, and `error retains same-PR data` in new resource tests. Browser tests hold feedback while switching tasks, then verify fresh checks after invalidation.

## Verification

Start with the named failing behavioral tests; record RED/GREEN evidence.
New test paths below are created by this order. Install workspace dependencies
once before the first package check if this checkout lacks them.

```bash
(cd apps/web && pnpm exec vitest run hooks/domains/github/use-pr-feedback.test.ts hooks/domains/github/use-pr-ci-popover.test.ts hooks/domains/github/pr-feedback-resource.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --project chromium tests/task/task-switch-efficiency.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-task-switch-efficiency.spec.ts)
```

## Files likely touched

- `apps/web/hooks/domains/github/use-pr-feedback.ts`
- `apps/web/hooks/domains/github/use-pr-feedback.test.ts`
- `apps/web/hooks/domains/github/use-pr-ci-popover.ts`
- `apps/web/hooks/domains/github/use-pr-ci-popover.test.ts`
- `apps/web/hooks/domains/github/pr-feedback-resource.ts (new)`
- `apps/web/lib/state/slices/github/github-slice.ts`
- `apps/web/lib/state/slices/github/types.ts`
- `apps/web/components/github/pr-detail-panel.tsx`
- `apps/web/e2e/tests/task/task-switch-efficiency.spec.ts`
- `apps/web/e2e/tests/task/mobile-task-switch-efficiency.spec.ts`

## Dependencies

No functional dependency on session ETags or the environment polling work. Run sequentially in this package; Task 04 introduces the shared browser scenarios.

## Risks

Never drop a real update to achieve a request-count target. Preserve authorization scope in all cache readers, not only the request key.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/session-refresh-efficiency.md)
- [System design](../../specs/ui/system-design/session-refresh-efficiency.md)
- [Trace and source evidence](trace-2026-09-29-task-switch.md)

## Results

The shared `pr-feedback-resource` now owns reads keyed by store, authentication,
backend, workspace, repository, and PR. PR detail and status/popover consumers
join an outstanding generation; invalidation queues one follow-up and an error
retains usable same-PR data.

Focused frontend tests pass, including three consumers, invalidation, scope
change, and stale-on-error retention. Desktop and phone task-switch E2Es pass
with PR feedback held during navigation and updated checks visible after
invalidation. These E2Es use the mock GitHub backend, so they do not attribute
the original multi-second header wait to a backend or upstream stage. No server
change or timing improvement is claimed.
