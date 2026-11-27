---
id: "03-per-pr-continuation"
title: "Continue one task per PR"
status: done
wave: 3
depends_on:
  - "01-new-commits-event"
plan: "plan.md"
requirements:
  - REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-004
  - REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-006
  - REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-008
acceptance_criteria:
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-004.1
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-004.2
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-004.3
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-004.4
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-004.5
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-004.6
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-004.7
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-004.8
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-004.9
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-004.10
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-006.1
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-006.3
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-006.4
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-006.5
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-008.1
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-008.2
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-008.3
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-008.4
system_design:
  - ../../specs/office/system-design/automations-pr-follow-up.md
---

# Task 03: Continue one task per PR

## Summary

Add the `reuse_per_pr` policy and the follow-up prompt. A `github_pr` firing
creates or continues the task mapped to its PR. The firing waits while that
task is busy. A stale task is replaced. A continued task receives the
follow-up prompt when one is set.

## In scope

- `ContinuationPolicyReusePerPR` and its validation, including the
  managed-conversation rejection and the rule that per-PR-only fields require
  the policy.
- The `automation_pr_continuations` store and `HasActiveRunForTask`.
- The `PRTaskBusyChecker` injection, the orchestrator's
  `AutomationPRTaskBusy`, and the `backendapp` wiring.
- The deferral in `admitTriggerLocked`, after dedup and before capacity.
- The orchestrator per-PR branch in `automation_pr_continuation.go`: create
  and map, resume, replace, fail when busy before dispatch, and fall back to
  `new_task`. The relaxed step check applies to this policy only.
- The `FollowUpPrompt` field and column. Render both prompts and select by
  action. The retry snapshot holds both resolved prompts and the policy
  fields.
- The MCP schema enum and `follow_up_prompt`. Export of
  `continuation_policy: reuse_per_pr` and `follow_up_prompt`.
- Editor: the third radio card and a **Follow-up prompt** `PromptSection`,
  with values cleared when the policy changes. Copy in all locales.
- Public docs: the per-PR policy, the busy-task wait, and the follow-up
  prompt.

## Out of scope

- The continuation step (task 04) and end-of-PR actions (task 05).

## Acceptance

1. Admission tests prove four things. An active run defers. A busy session
   defers. A checker error counts as busy. A deferral writes no run row, leaves
   the dedup key free, and fires on the first poll after the task stops being
   busy. The `max_concurrent_runs` limit still applies.
2. Orchestrator tests prove the create, map, resume, replace, busy-fail,
   non-PR, and retry paths, with the correct `thread_action` and reason, and no
   duplicate task per subject. The follow-up prompt is sent only on `resumed`.
3. The editor shows the third card and the follow-up prompt as specified on
   desktop and phone. The values persist through save and reload, MCP, and
   export.

## ASCII UI preview

See [UI-02](plan.md#ui-02-then-section-with-reuse_per_pr-selected-and-the-normal-task-target)
and [UI-03](plan.md#ui-03-context-between-runs-with-a-github_pr-trigger).
This task adds the follow-up prompt and the third card.

This covers AC-006.1, 006.3, 006.4 (prompt), and 006.5.

## Verification

```bash
(cd apps/backend && go test ./internal/automation/ -run 'ReusePerPR|PRContinuation|FollowUpPrompt|Export' -count=1)
(cd apps/backend && go test ./internal/orchestrator/ -run 'PRContinuation|FindAutomationContinuation' -count=1)
(cd apps/backend && go test ./internal/mcp/server/ -run 'ConfigAutomation' -count=1)
(cd apps && pnpm --filter @kandev/web test -- components/automations)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/automations-pr-follow-up.spec.ts)
(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/mobile-automations-pr-follow-up.spec.ts)
```

## Files likely touched

- `apps/backend/internal/automation/models.go`, `service.go`, `store.go`,
  `store_pr_continuations.go` (new), `retry_snapshot.go`, `export.go`,
  `export_service.go`
- `apps/backend/internal/automation/pr_continuation_admission_test.go` (new),
  `service_validation_test.go`, `export_service_test.go`
- `apps/backend/internal/orchestrator/automation_pr_continuation.go` (new),
  `event_handlers_automation.go` (branch and prompt hook),
  `automation_pr_continuation_test.go` (new)
- `apps/backend/internal/backendapp/` (busy-checker wiring)
- `apps/backend/internal/mcp/server/config_automation_handlers.go` and its test
- `apps/web/components/automations/automation-continuation-section.tsx`
  (extracted), `automation-editor-sections.tsx`, `automation-editor.tsx`,
  `automation-payload.ts`, and tests; `apps/web/lib/types/automation.ts`;
  `apps/web/src/locales/*/automations.json`
- E2E specs and `docs/public/automation-and-mcp.md`

## Dependencies

Task 01, which provides `pr_subject` and the deferred-outcome handling.

## Risks

- The retry ledger's committed and ambiguous states must not adopt a
  different task or prompt.
- The busy check must stay cheap. It runs under the per-automation admission
  lock.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/office/requirements/automations-pr-follow-up.md)
  REQ-004, 006, and 008.
- [System design](../../specs/office/system-design/automations-pr-follow-up.md):
  Per-PR continuation policy, Busy check and admission deferral, Dispatch,
  Follow-up prompt, Editor, and Export.
- Patterns: `findAutomationContinuation`, `SetContinuationTaskID`, and the
  `TestFindAutomationContinuation*` tests.

## Results

Done on 2026-11-24.

- `(cd apps/backend && go test -trimpath ./internal/automation/ -run 'ReusePerPR|ParsePRSubject|TestExportPerPRSettings|TestAC22' -count=1)`: ok.
- `(cd apps/backend && go test -trimpath ./internal/orchestrator/ -run 'PRContinuation|AutomationPRTask' -count=1)`: ok.
- `(cd apps/backend && go test -trimpath ./internal/mcp/server/ -run 'ConfigAutomation' -count=1)`: ok.
- `(cd apps && pnpm --filter @kandev/web test -- components/automations)`: 28 files, 214 tests passed.
- `(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/automations-pr-follow-up.spec.ts)`: 2 passed.
- `(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/mobile-automations-pr-follow-up.spec.ts)`: 1 passed.

Deviation: the busy check is a `PRTaskController` interface (`AutomationPRTaskBusy`) instead of a function field, and it covers `CREATED`, `STARTING`, and `RUNNING` sessions without a separate queued-turn read. The design was updated.
