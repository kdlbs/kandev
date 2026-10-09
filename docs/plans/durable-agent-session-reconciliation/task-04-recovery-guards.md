---
id: "04-recovery-guards"
title: "Preserve recovery admission and UI guards"
status: complete
wave: 4
depends_on:
  - 03-terminal-replay
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
  - REQ-AGENTS-HARNESS-SESSION-CONTINUITY-001
  - REQ-AGENTS-HARNESS-SESSION-CONTINUITY-005
  - REQ-AGENTS-HARNESS-SESSION-CONTINUITY-006
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.2
  - AC-AGENTS-HARNESS-SESSION-CONTINUITY-001.2
  - AC-AGENTS-HARNESS-SESSION-CONTINUITY-005.1
  - AC-AGENTS-HARNESS-SESSION-CONTINUITY-005.2
  - AC-AGENTS-HARNESS-SESSION-CONTINUITY-006.1
  - AC-AGENTS-HARNESS-SESSION-CONTINUITY-006.2
  - AC-AGENTS-HARNESS-SESSION-CONTINUITY-006.3
  - AC-AGENTS-HARNESS-SESSION-CONTINUITY-006.4
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.3
system_design:
  - ../../specs/platform/system-design/durable-agent-delivery.md
  - ../../specs/agents/system-design/harness-session-continuity.md
---

# Task 04: Preserve recovery admission and UI guards

## Summary

Compose startup adoption guards with durable recovery requirements across every entry point.

## In scope

Compose startup adoption guards with durable recovery requirements across every entry point.
- Trace manual resume, workspace restore, context continuation, fresh start, queue drain, Send Now, steer, Office, and automation.
- Preserve startup ownership guards before control-server contact and orchestrator subscriptions before recovery publication.
- Do not clear durable recovery blocks when transient adoption guards release.
- Preserve queue claims, turn identity, and autonomous budget/provenance checks during recovery.
- Restore Stop behavior for an adopted quiet prompt and cancellation escalation. A late completion cannot overwrite cancellation.
- Combine typed guard errors and continuation errors in handlers, services, hooks, and banners.
- Apply guard precedence to replacement actions. Preserve both resume and workspace errors and reject stale responses after session switching.
- Keep the recovery UI shared across desktop/mobile. Use the preview without adding a separate recovery page.

## Out of scope

New survival runtimes, new flags, automatic context continuation, and unrelated CI fixes.

## Acceptance

1. No automatic or manual replacement bypasses an unresolved owner or delivery block. Stop remains reachable.
2. Successful adoption clears transient progress only. Explicit continuation retains its existing authorization and generation checks.
3. Desktop and mobile show equivalent eligible actions, localized errors, and current-session state.

## Test cases

Add table-driven `TestDurableAdoptionAdmissionGuards` covering every listed dispatcher and explicit API action.
Add `TestDurableAdoptionCancelEscalation` with an original submission, successor attempt, and late completion.
Extend recovery hook tests for guard plus continuation, unblock, dual failures, stale responses, and cancellation cleanup.
Retain merged `use-session-recovery-actions-guard.test.ts` and current continuation tests.
Task 05 owns real desktop/mobile restart evidence for UI-01.

## ASCII UI preview

See [the full UI-01 contract](plan.md#ui-01-session-recovery-after-backend-restart).

### UI-01: Session recovery after backend restart

Entry: existing task-session chat and its recovery banner. Labels show intent, not new literal copy.

```text
Desktop: existing chat region
[Recovery status and reason]                      [Stop]
[Retry connection]  [Continue from saved context*]

Phone: existing session chat
[Recovery status and reason]
[Stop                         ]
[Retry connection             ]
[Continue from saved context* ]
```

`*` Only a typed continuation result and resolved ownership permit this action.
During adoption, show recovery progress and suppress replacement actions.
For an unstoppable prior owner, show the guard error and suppress replacement.
For unresolved delivery, keep Stop and a state-only retry available.
After successful adoption, remove the recovery notice without adding a resume message.

Reuse `session-stopped-banner.tsx` and `mobile-session-resume-recovery.spec.ts` as the nearest surfaces.
Keep status before actions and reuse shared recovery state on both viewports.
Use inline presentation because the user needs the conversation while resolving recovery.
The chat remains the scroll owner. Add no drawer, nested scroller, or fixed overlay.
Phone actions stack with at least 44px touch targets and no horizontal overflow.
Desktop controls retain their existing compact size. Preserve focus during asynchronous updates.
All copy uses existing translation keys or complete locale updates.
Spacing is illustrative. State precedence, action eligibility, and mobile access are required.
This view covers continuity criteria 005.1 and 005.2 and delivery criterion 006.2.

## Verification

Run from the repository root. Proposed test files must exist before these commands pass.

```bash
(cd apps/backend && go test -race ./internal/orchestrator ./internal/orchestrator/handlers ./internal/agent/runtime/lifecycle ./internal/office/service ./internal/automation -run 'TestDurableAdoption|Test.*Recovery|Test.*Cancel|Test.*SendNow|Test.*Steer' -count=1)
(cd apps/web && pnpm exec vitest run hooks/domains/session/use-session-recovery-actions.test.ts hooks/domains/session/use-session-recovery-actions-guard.test.ts lib/services/session-recovery-service.test.ts hooks/domains/session/use-session-resumption-guard-refusal.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps && pnpm --filter @kandev/web lint)
(cd apps/web && pnpm run i18n:check)
(cd apps/backend && golangci-lint run ./... --new-from-rev=origin/main --timeout=5m)
git diff --check
```

## Files likely touched

- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/orchestrator/session_launch.go`
- `apps/backend/internal/orchestrator/queue_dispatch_recovery.go`
- `apps/backend/internal/orchestrator/queue_send_now.go`
- `apps/backend/internal/orchestrator/handlers/handlers.go`
- `apps/backend/internal/orchestrator/handlers/handlers_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go`
- `apps/backend/internal/office/service/scheduler_integration.go`
- `apps/web/lib/services/session-recovery-service.ts`
- `apps/web/hooks/domains/session/use-session-recovery-actions.ts`
- `apps/web/components/task/chat/session-stopped-banner.tsx`
- `apps/web/src/locales/en/task.json`

## Dependencies

Task 03 must pass its acceptance before this work begins.

## Inputs

- [Plan and baseline](plan.md).
- [Adoption contract](../../specs/platform/system-design/durable-agent-delivery.md#surviving-process-adoption).
- Read the scoped AGENTS.md files and the tests next to every changed implementation.

## Risks

Backend startup and UI actions can race. A hidden button is not a backend admission fence.
Do not turn a transient adoption guard into a persisted lifecycle state.

## Parallelism

`sequential`

## Results

Completed 2026-09-14. Recovery admission preserves identity, Stop, queue,
budget, provenance, Office, automation, and explicit continuation guards across
desktop and mobile surfaces. The new durable adoption errors remain typed and
do not authorize replacement or automatic resend.

The exact Task 04 backend race suite, focused web recovery tests, typecheck,
web lint, i18n validation, changed-package Go lint, and whitespace checks
passed.
