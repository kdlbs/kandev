---
id: "02-recovery-resolution"
title: "Resolve the matching startup warning"
status: done
wave: 2
depends_on:
  - 01-submission-replay
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-002
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.4
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.8
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.9
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.16
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.17
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.19
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.32
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.33
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-002.2
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-002.3
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-002.5
system_design:
  - ../../specs/agents/system-design/session-startup-failure-explanations.md
  - ../../specs/tasks/system-design/task-launch-failure-recovery.md
---

# Task 02: Resolve the matching startup warning

## Summary

Prove that successful failed-start fresh recovery retires the captured startup
warning before replay. Preserve dated history and any successor failure.

## In scope

- Preserve attempt identity and captured error stamp through fresh provider reset.
- Reuse boot-ready resolution, bounded durable proof, and stamp-CAS dismissal.
- Publish the matching inactive error projection and metadata after durable success.
- Cover missing optional notices, reversed events, reload, siblings, and late failures.
- Correct frontend reconciliation only if focused tests expose another conformance gap.
- Preserve composer drafts, selected attachments, and user-initiated focus return.

## Out of scope

New error stores, global timestamp heuristics, layout redesign, model changes,
workspace-only success claims, and manual dismissal as evidence of recovery.

## Acceptance

1. Provider-confirmed readiness retires only the captured failure. Admission,
   cancelled startup, and read-only restoration retain the active warning.
2. A new delivery failure or sibling failure survives delayed prior success.
   Durable history retains its original cause/time after reload without a notice.
3. Desktop and phone restore the usable composer and draft after matching recovery,
   with no duplicate mutation controls or forced focus from background updates.

## ASCII UI preview

UI-01: Task Chat after failed initial startup. Source evidence supports the before state.

```text
Desktop, before retry
  Transcript: [Screenshot] original request
  [Startup failure record]
  [Recovery card: Start fresh session | Restore workspace]

Desktop, after successful retry
  Transcript: [Screenshot] original request (one user row)
  [Original startup failure: historical details]
  [Agent response]
  [Editable composer: existing draft and selected attachments]

Phone, after successful retry
  [Task header / Chat]
  [Screenshot and file labels]
  [Original request: one row]
  [Historical error / details]
  [Agent response]
  [Editable composer]
  [Phone navigation]
```

UI-02: Pending recovery and successor failure use the current composer region.

```text
Desktop pending: [Recovery cause | Working... | disabled alternatives]
Phone pending:
  [Recovery cause]
  [Working...]
  [Disabled alternatives, stacked]

Delivery failure after readiness:
  [New delivery error, its own details and eligible actions]
```

The transcript owns vertical message scrolling. The composer/recovery region and
phone navigation retain their current layout and safe-area clearance. Historical
errors retain readable details without recovery controls. These structural
outcomes are required; spacing and example copy are illustrative.
Phone targets remain at least 44px. No new picker, drawer, or route is introduced.
The existing image preview uses its current dismissal and focus-return behavior.
Maps to AC-TASKS-PROMPT-ATTACHMENTS-002.6/.8 and agent criteria 006.16/.17/.19/.33.

Full view: [plan](plan.md#ascii-ui-preview). UI-01 and UI-02 apply to agent
criteria 006.16/.17/.19/.33 and task history criteria 002.2/.3/.5.

## Verification

From the repository root, run:

```bash
(cd apps/backend && go test -trimpath ./internal/orchestrator -run 'TestFreshStartRecovery' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/orchestrator -run 'TestFreshStartRecoveryPreservesSuccessorFailure|TestFreshStartRecoveryCancelledBoot' -count=1)
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run lib/session-last-agent-error.test.ts lib/session-recovery-presentation.test.ts hooks/domains/session/use-session-recovery-actions.test.ts)
(cd apps/web && pnpm run typecheck)
```

Write `TestFreshStartRecoveryResolvesCapturedError` against the current fresh
failed-start path before the correction. It must fail on unresolved durable error
state, rather than a missing selector. Task 01 can make this regression GREEN;
in that case record the pre-task-01 RED evidence and do not invent another patch.
Add browser evidence in task 03. Run installation once per fresh worktree.

## Files likely touched

- `apps/backend/internal/orchestrator/resume_attempt.go`
- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/fresh_start_resolution_test.go` (new)
- `apps/web/lib/session-last-agent-error.ts` and its existing test
- `apps/web/lib/session-recovery-presentation.ts` and its existing test
- `apps/web/hooks/domains/session/use-session-recovery-actions.ts` and its existing test
- `apps/web/components/task/chat/session-recovery-card.tsx` only for a proven gap

## Dependencies

Task 01 provides the prompt-free owned recovery startup.

## Risks

A stale boot callback can clear a newer error unless durable resolution checks
both the captured stamp and the owned attempt. Keep current success-proof bounds.

## Parallelism

`sequential`

## Inputs

Read the fresh-start readiness repair in the agent design and task history criteria.
Use current `markRecoveryResolvedForAttempt`, `RecordSessionRecoveryResolution`,
and `dismissRecoveredAgentErrorForAttempt` instead of another resolution mechanism.

## Results

Done. Fresh recovery resolves only the captured startup error after provider readiness.
The stamp comparison preserves a later delivery error. Failed boot and read-only
workspace restore keep the startup warning. Reload retains dated error history and
restores the existing composer with its draft.

- Focused backend tests passed for replay, cancelled boot, and successor failure.
- Three recovery-presentation Vitest files passed with 55 tests.
- Web typecheck passed.
- Desktop and phone browser runs confirm that successful recovery leaves no active
  startup controls and retains the history entry after reload.
