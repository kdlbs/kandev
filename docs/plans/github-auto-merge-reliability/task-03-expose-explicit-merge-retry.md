---
id: "03-expose-explicit-merge-retry"
title: "Expose explicit merge retry"
status: done
wave: 3
depends_on:
  - "02-journal-automatic-merge-attempts"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.4
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.9
system_design:
  - ../../specs/integrations/system-design/github-pr-merge-queue.md
---
# Task 03: Expose Explicit Merge Retry

## Summary

Add a task-scoped command that rearms one failed automatic merge attempt. Wire
the shared desktop and mobile controls to that command.

## In scope

- Add the retry route, controller, service method, and exact-PR authorization.
- Persist retry authorization and publish the normal evaluation event.
- Return `202 Accepted` without claiming provider success.
- Extend frontend API types and automation state with the typed error kind.
- Call explicit retry only for an `auto_merge` error.
- Label other stored errors as Refresh and keep load-error refresh behavior.
- Keep the existing desktop popover and inset mobile status drawer.
- Keep one mobile scroll owner and a 44-by-44-pixel shared action target.
- Add backend, API-client, hook, component, and locale tests.
- Add copy to the five required locale catalogs.

## Out of scope

- Direct provider calls from the controller.
- A new dialog, drawer, route, or automation switch.
- Relaxing any readiness or provider rule.

## Acceptance

- Retry addresses only the named pull request linked to the authorized task.
- The command reevaluates every current readiness gate.
- Desktop and mobile expose the same action and feedback.
- A state-load failure refreshes state and never rearms a merge attempt.

## Verification

```bash
go test -tags fts5 ./internal/github -run 'Test.*Retry.*Merge'
cd apps/web && pnpm test -- components/github/pr-ci-automation-controls.test.tsx
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run i18n:check
```

Run Go from `apps/backend`. Run pnpm from the locations shown.

## Files likely touched

- `apps/backend/internal/github/controller.go`
- `apps/backend/internal/github/service_ci_automation.go`
- Backend controller and service test files.
- `apps/web/lib/api/domains/github-api.ts`
- `apps/web/lib/types/github.ts`
- `apps/web/hooks/domains/github/use-task-ci-options.ts`
- `apps/web/components/github/pr-ci-automation-controls.tsx`
- Focused frontend test files.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw}/github.json`

## Dependencies

- Task 02 provides the durable attempt and retry-authorization state.

## Risks

- The endpoint must not accept a repository or pull request outside the task.
- Error-kind copy must remain complete across all required locales.
- The mobile drawer must keep a single scroll owner.

## Parallelism

`sequential`

## Inputs

- Task 02 attempt journal.
- Integration acceptance criteria 002.4 and 002.9.
- Existing shared desktop and mobile automation controls.

## Results

- Added an authorized task-scoped `202 Accepted` retry command for one linked
  pull request and published the normal PR evaluation event.
- Persisted a single-use retry authorization that the attempt reservation
  consumes atomically, including duplicate and stale in-flight guards.
- Added typed frontend error handling so only `auto_merge` failures rearm a
  merge. Other stored and loading errors retain state refresh.
- Kept the shared desktop and mobile controls and added a 44-pixel mobile
  action target with complete localized copy.
- Passed the focused backend retry tests, component/API/hook tests, TypeScript
  typecheck, and the complete i18n check.
