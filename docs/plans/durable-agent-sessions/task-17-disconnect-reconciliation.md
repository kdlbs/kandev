---
id: "17-disconnect-reconciliation"
title: "Reconcile disconnected submissions"
status: complete
wave: 17
depends_on: ["16-workflow-effect-dedup"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.2
system_design:
  - ../../specs/platform/system-design/durable-agent-delivery.md
---

# Task 17: Reconcile disconnected submissions

## Summary

Replace immediate disconnect failure with bounded reconciliation and visible uncertainty.

## In scope

- Reconcile the original owner, stream, and submission before terminal completion or another prompt admission.
- Reconnect within the bounded window, then preserve interrupted_unknown when evidence remains insufficient.
- Reuse recovery blocks for Office and automation consumers. Scheduler retries and queue edit-save drains cannot bypass them.
- Own agent_delivery_uncertain_submissions_total at the first transition, not on each reconciliation poll.
- Keep Stop available. Make reconnect retry a state query rather than prompt redispatch.
- Extend existing recovery notices and queue guards with localized reconnecting/uncertain states and desktop/mobile tests.

## Out of scope

New idle-timeout policy, synthetic completion signals, and automatic uncertain-prompt replay.

## Acceptance

- A transport break cannot immediately fail a completed-but-unobserved prompt or release queued work.
- Recovered terminal output appears once. Uncertain work blocks provider retry and queue auto-run.
- Desktop and mobile users can stop or retry connection recovery without another prompt dispatch.

## Verification

Run from the repository root. New test names describe required evidence, not existing passing tests.
Use TDD for implementation. Record the failing assertion before the implementation result.

```bash
(cd apps/backend && rtk go test -tags fts5 -race ./internal/agent/runtime/lifecycle ./internal/agent/runtime/agentctl ./internal/orchestrator/... -count=1)
(cd apps/web && rtk pnpm run typecheck)
(cd apps/web && rtk pnpm run i18n:zh-hant)
(cd apps/web && rtk pnpm run i18n:check)
(cd apps/web && rtk pnpm run i18n:ratchet)
(cd apps/web && rtk pnpm e2e:run --project chromium tests/session/durable-agent-delivery.spec.ts tests/session/session-resume-prompt-queue.spec.ts)
(cd apps/web && rtk pnpm e2e:run --project mobile-chrome tests/session/mobile-durable-agent-delivery.spec.ts)
rtk make -C apps/backend lint
(cd apps/web && rtk pnpm run lint)
rtk python3 scripts/lint-spec-files.py --all
rtk git diff --check
```

Target evidence in `apps/backend/internal/agent/runtime/lifecycle/stream_reconciliation_test.go`:

- `TestDisconnectReconcilesBeforeTerminalOutcome`: `AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.1`.
- `TestUncertainSubmissionBlocksQueueAndKeepsStop`: `AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.2`.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/streams.go`
- `apps/backend/internal/agent/runtime/agentctl/agent.go`
- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/queue_dispatch_recovery.go`
- `apps/backend/internal/office/service/`
- `AGENTS.md` (complete backend delivery metric ownership).
- `apps/web/components/task/ensure-session-error.tsx`
- `apps/web/components/task/chat/messages/status-message.tsx`
- `apps/web/src/locales/`
- `apps/backend/internal/agent/runtime/lifecycle/stream_reconciliation_test.go` (new tests or extensions).

## Dependencies

[Task 16](task-16-workflow-effect-dedup.md).

## Risks

- Existing provider retry can bypass a new UI state unless backend admission owns the uncertainty guard.

## Parallelism

`sequential`

The primary session owns integration. This work order does not authorize subagents.
Preserve existing user edits and unrelated changes.

## Inputs

- [Owned system design](../../specs/platform/system-design/durable-agent-delivery.md).
- [Package manifest](plan.md), including shared regression gates and test prerequisites.
- Existing source and adjacent tests in the listed files.
- [Boundary decision](../../decisions/2026-09-10-durable-harness-session-boundaries.md).

## Results

Implemented accepted-submission identity propagation, one bounded state query, replay reconnect for
completed submissions, explicit uncertain errors, persisted recovery-block projection, and queue
retry suppression. Stop and reconnect recovery remain available on desktop and mobile. Focused
lifecycle/agentctl/orchestrator race tests, web lint and i18n checks, and mobile recovery coverage
pass. The full browser matrix remains environment-dependent.
