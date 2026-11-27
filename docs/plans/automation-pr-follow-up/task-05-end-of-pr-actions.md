---
id: "05-end-of-pr-actions"
title: "Archive or delete PR tasks when the PR ends"
status: done
wave: 4
depends_on:
  - "03-per-pr-continuation"
plan: "plan.md"
requirements:
  - REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-009
  - REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-006
acceptance_criteria:
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-009.1
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-009.2
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-009.3
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-009.4
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-009.5
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-009.6
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-006.3
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-006.4
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-006.5
system_design:
  - ../../specs/office/system-design/automations-pr-follow-up.md
---

# Task 05: Archive or delete PR tasks when the PR ends

## Summary

Add `pr_merged_action` and `pr_closed_action`. Each takes `keep`, `archive`,
or `delete`, and both default to `keep`. When a mapped PR leaves the open
listing, Kandev reads its final state and applies the matching action directly
to the PR task, waiting while the task is busy. It then removes the mapping.

## In scope

- The two columns, the model, payload, MCP, and retry snapshot fields, and
  validation (non-`keep` values require `reuse_per_pr`).
- `runEndOfPRActions` after a complete listing, once per automation and
  repository per cycle, using `GetAutomationPR`.
- Injected orchestrator `ArchiveAutomationPRTask` and `DeleteAutomationPRTask`,
  which reuse task service archive and lifecycle delete.
- Export `pr_merged_action` and `pr_closed_action`.
- Editor: two selects and a delete warning. Copy in all locales.
- Public docs, including how this relates to the `github_pr_merged` trigger.

## Out of scope

- Mapped PRs whose repository is no longer in any trigger.
- End-of-PR actions for policies other than `reuse_per_pr`.

## Acceptance

1. Evaluator integration tests cover these cases: merged and closed choose
   their own action; the action is postponed while busy; the mapping is
   removed after `keep`, `archive`, or `delete`; an already archived or
   missing task counts as done; a partial listing or a failed read does
   nothing.
2. Orchestrator tests prove that archive and delete use the existing task
   lifecycle. Delete stops nothing that is running, because a busy task is
   postponed.
3. The editor selects persist on desktop and phone. The delete warning shows
   only for `delete`. Export emits both keys.

## ASCII UI preview

See [UI-03](plan.md#ui-03-context-between-runs-with-a-github_pr-trigger).

```text
When the pull request is merged
[ Archive task                                                  v ]
When the pull request is closed without merging
[ Delete task                                                   v ]
  ! Deleting removes the task's conversation and files.
```

This covers AC-006.3, 006.4 (selectors), and 006.5.

## Verification

```bash
(cd apps/backend && go test ./internal/automation/ -run 'EndOfPR|PRContinuation|Export' -count=1)
(cd apps/backend && go test ./internal/orchestrator/ -run 'AutomationPRTask' -count=1)
(cd apps/backend && go test ./internal/mcp/server/ -run 'ConfigAutomation' -count=1)
(cd apps && pnpm --filter @kandev/web test -- components/automations)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/automations-pr-follow-up.spec.ts)
(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/mobile-automations-pr-follow-up.spec.ts)
```

## Files likely touched

- `apps/backend/internal/automation/evaluator_pr_observation.go` or
  `evaluator_pr_end_actions.go` (new), `models.go`, `service.go`, `store.go`,
  `store_pr_continuations.go`, `retry_snapshot.go`, `export.go`,
  `export_service.go`
- `apps/backend/internal/automation/evaluator_pr_end_actions_test.go` (new)
- `apps/backend/internal/orchestrator/automation_pr_continuation.go` and its
  test; `apps/backend/internal/backendapp/` (wiring)
- `apps/backend/internal/mcp/server/config_automation_handlers.go`
- `apps/web/components/automations/automation-continuation-section.tsx`,
  `automation-payload.ts`, and their tests; `apps/web/src/locales/*/automations.json`
- E2E specs and `docs/public/automation-and-mcp.md`

## Dependencies

Task 03. Task 04 is independent, but both touch the continuation section and
the automation model. Run them one after the other.

## Risks

- Delete is irreversible. Keep the default `keep`, show the warning, and never
  act while the task is busy.
- A PR that reopens before cleanup appears in the listing again, so no action
  runs. That is the intended behavior.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/office/requirements/automations-pr-follow-up.md)
  REQ-009 and REQ-006.
- [System design](../../specs/office/system-design/automations-pr-follow-up.md):
  End-of-PR actions, Editor, and Export.
- Patterns: GitHub review watch cleanup (`CleanupPolicy*`) and
  `DeleteTaskWithLifecycleAndReason`.

## Results

Done on 2026-11-24.

- `(cd apps/backend && go test -trimpath ./internal/automation/ -run 'EndOfPR' -count=1)`: ok (merged vs closed actions, keep, busy postponement, partial listing, failed read, other policies).
- `(cd apps/backend && go test -trimpath ./internal/orchestrator/ -run 'AutomationPRTask' -count=1)`: ok.
- `(cd apps && pnpm --filter @kandev/web test -- components/automations)`: 214 passed.
- `(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/automations-pr-follow-up.spec.ts)`: 2 passed.
- `(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/mobile-automations-pr-follow-up.spec.ts)`: 1 passed.
