---
id: "07-automation-recovery"
title: "Park automation dispatches that require recovery"
status: complete
wave: 7
depends_on: ["06-office-recovery"]
plan: "plan.md"
requirements:
  - REQ-AGENTS-HARNESS-SESSION-CONTINUITY-006
acceptance_criteria:
  - AC-AGENTS-HARNESS-SESSION-CONTINUITY-006.1
  - AC-AGENTS-HARNESS-SESSION-CONTINUITY-006.2
  - AC-AGENTS-HARNESS-SESSION-CONTINUITY-006.3
  - AC-AGENTS-HARNESS-SESSION-CONTINUITY-006.4
system_design:
  - ../../specs/agents/system-design/harness-session-continuity.md
---

# Task 07: Park automation dispatches that require recovery

## Summary

Park automation dispatches that require recovery through the canonical session recovery block.
Preserve work identity and expose operator recovery without an automatic replacement conversation.

## In scope

- Preserve CI, PR, and MR automation queue rows, source events, coalescing keys, and dispatch-attempt identity under a session recovery block.
- Apply the same admission condition to ordinary queue startup, Send Now, edit-save drain, and lifecycle automation.
- Surface one actionable task/run notice with a link to the shared recovery surface. Retain historical failure and uncertainty evidence.
- Require explicit settlement before superseding an uncertain submission. Native continuation alone cannot release that old prompt.
- Add current-main regression coverage for queue edits, transfers, pending dispatch recovery, and completed-task follow-ups.
- Update public continuity and unattended-recovery guidance for milestone A. Do not claim durable transport before milestone B.

## Out of scope

Automatic context continuation, global agent unpause, synthetic workflow decisions, and a new retry scheduler.

## Acceptance

- Repeated automation events cannot resend blocked work or create duplicate recovery notices.
- Authorized recovery resumes only the selected pending work through normal admission. Unknown legacy dispatch outcomes remain blocked.
- Desktop and mobile operators can inspect the cause and reach the recovery action without a hidden flag.

## Verification

Run from the repository root. Use TDD and disposable fixtures.
New tests describe required future evidence, not existing passing results.

```bash
(cd apps/backend && rtk go test -tags fts5 -race ./internal/orchestrator/... ./internal/agent/runtime/lifecycle ./internal/automation/... -count=1)
(cd apps/backend && rtk go run ./cmd/sqlguard ./internal)
(cd apps/backend && rtk go test -tags fts5 -race ./internal/persistence ./internal/persistence/storeconformance -count=1)
(cd apps/web && rtk pnpm run typecheck)
(cd apps/web && rtk pnpm run i18n:zh-hant)
(cd apps/web && rtk pnpm run i18n:check)
(cd apps/web && rtk pnpm run i18n:ratchet)
(cd apps/web && rtk pnpm e2e:run --project chromium tests/session/autonomous-session-recovery.spec.ts tests/session/session-resume-prompt-queue.spec.ts)
(cd apps/web && rtk pnpm e2e:run --project mobile-chrome tests/session/mobile-autonomous-session-recovery.spec.ts)
rtk node --test scripts/validate-public-docs.test.mjs
rtk node scripts/validate-public-docs.mjs
rtk make -C apps/backend lint
(cd apps/web && rtk pnpm run lint)
rtk python3 scripts/lint-spec-files.py --all
rtk git diff --check
```

- `automation_session_recovery_test.go:TestAutomationRecoveryPreservesPendingWork`: AC-AGENTS-HARNESS-SESSION-CONTINUITY-006.1.
- `automation_session_recovery_test.go:TestAutomationRecoveryRequiresExplicitSettlement`: AC-AGENTS-HARNESS-SESSION-CONTINUITY-006.2.
- `automation_session_recovery_test.go:TestAutomationRecoveryBlocksRepeatedTriggers`: AC-AGENTS-HARNESS-SESSION-CONTINUITY-006.3.
- `autonomous-session-recovery.spec.ts` and `mobile-autonomous-session-recovery.spec.ts`: AC-AGENTS-HARNESS-SESSION-CONTINUITY-006.4.

Run persistence checks with a provisioned PostgreSQL database through `KANDEV_TEST_POSTGRES_DSN`.
A skipped PostgreSQL suite does not satisfy acceptance.
Office fixtures retain their existing Office feature configuration. This package introduces no continuity or delivery toggle.

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/event_handlers_automation.go`
- `apps/backend/internal/orchestrator/ci_automation_dispatch.go`
- `apps/backend/internal/orchestrator/event_handlers_github_pr_automation.go`
- `apps/backend/internal/orchestrator/event_handlers_gitlab_mr_automation.go`
- `apps/backend/internal/orchestrator/queue_dispatch_recovery.go`
- `apps/backend/internal/orchestrator/messagequeue/`
- `apps/web/components/automations/runs-section.tsx`
- `apps/web/components/task/ensure-session-error.tsx`
- `apps/web/src/locales/`
- `docs/public/sessions-and-review.md`
- `docs/public/agent-communication.md`
- `apps/backend/internal/orchestrator/automation_session_recovery_test.go (new)`
- `apps/web/e2e/tests/session/autonomous-session-recovery.spec.ts (new)`
- `apps/web/e2e/tests/session/mobile-autonomous-session-recovery.spec.ts (new)`

## Dependencies

[Task 06](task-06-office-recovery.md).

## Risks

- Generic retry or notification dismissal can accidentally release the recovery block.
- Recovery must preserve provenance, budget admission, and the current generation.
- A stale projection must not allow dispatch before the canonical block check.

## Parallelism

`sequential`

This work order does not authorize subagents. Preserve unrelated user edits.

## Inputs

- [Continuity design: autonomous consumers](../../specs/agents/system-design/harness-session-continuity.md#autonomous-consumers).
- [Existing Office status-only recovery](../../specs/office/system-design/agent-recovery.md).
- [Existing queue edit fencing](../../specs/ui/system-design/message-queue-edit.md).
- [Package manifest](plan.md).

## Results

Implemented CI, PR, and MR recovery parking and explicit settlement guards. Automation regression tests preserve pending work and block repeated background dispatch while recovery is open.
