---
id: "07-overload-recovery"
title: "Surface stream failures and preserve Stop on desktop and phone"
status: done
wave: 7
depends_on: ["06-submission-rollover"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-004
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-007
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-004.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.3
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.2
system_design:
  - ../../specs/platform/system-design/durable-agent-delivery.md
  - ../../specs/platform/system-design/durable-agent-stream-processing.md
---

# Task 07: Surface stream failures and preserve Stop on desktop and phone

## Summary

Surface stream failures and preserve Stop on desktop and phone. Preserve the existing durable identity and admission boundaries.

## In scope

Connect storage, projection, sequence, and overload failures to the existing session recovery state and admission guard.
Bound intake by bytes as well as item count. For durable streams, resume from the committed cursor after overload.
For legacy streams, use bounded flow control where control traffic remains responsive. If that cannot hold, surface uncertainty explicitly.
Never promise replay for legacy output or restore an unbounded queue. Preserve the harness and original submission identity where possible.
Keep Stop reachable while reconnecting. Retry connection queries the original state; it never resends a prompt.
A storage error alone must not offer context continuation. Preserve separately authorized native recovery actions.
Reuse existing recovery presentation and shared view-models. Add localized status only where existing reasons cannot convey the error.
Keep Office and automation parked through their existing guards. Exercise queued send, Send Now, steer, and scheduler re-entry.
Update public recovery documentation and root/scoped guidance only where the completed behavior changes their descriptions.

## Out of scope

Other work orders, automatic prompt resend, native conversation replacement, and unrelated cleanup.

## Acceptance

- Injected projection, ACK, journal, and queue failures produce accurate desktop/mobile state with preserved transcript and usable Stop.
- Retry performs state-only recovery, and uncertain work blocks every automatic dispatch entry point.
- Durable overload catches up without gaps; legacy overflow is visible and cannot silently claim complete history.

## Regression evidence

Add TestDeliveryOverloadRecovery and TestDeliveryFailureAdmissionFences. Extend existing recovery hook/presentation tests. Add tests/session/durable-stream-recovery.spec.ts and tests/session/mobile-durable-stream-recovery.spec.ts with controlled backend fault fixtures, never production fault switches.

## ASCII UI preview

UI-01: Existing session chat, delivery recovery status. Labels are illustrative and must use localization.

```text
Desktop: [Committed conversation output]
         Delivery interrupted. Outcome uncertain.
         [Retry connection] [Stop]

Phone:   [Committed conversation output]
         Delivery interrupted.
         Outcome uncertain.
         [Retry connection]
         [Stop]
```

The same region shows Reconnecting while bounded recovery runs. On success, normal chat returns without duplicate text.
An ACK-only retry after successful projection does not hide output or claim the prompt failed.
Use the session recovery surfaces as exemplars; do not route a streaming error through bootstrap-only predicates.
The existing chat owns scrolling. Do not add a modal, nested scroll area, or fixed overlay.
Phone controls retain 44px touch targets and safe-area clearance; desktop controls retain their existing compact size.
Stop remains available during retry. No generic Resume action may resend the uncertain submission.
Map this view to AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.2 and 006.3.

See [combined preview](plan.md#ascii-ui-preview).

## Verification

Run from the repository root after implementation.

```bash
(cd apps/backend && go test -race ./internal/agent/runtime/agentctl ./internal/agent/runtime/lifecycle ./internal/orchestrator ./internal/office/service ./internal/automation -count=1)
make -C apps/backend lint
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm test lib/session-recovery-presentation.test.ts hooks/domains/session/use-session-recovery-actions.test.ts hooks/domains/session/use-session-recovery-actions-guard.test.ts)
(cd apps/web && pnpm lint)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/session/durable-stream-recovery.spec.ts tests/session/session-stream-overload-isolation.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-durable-stream-recovery.spec.ts tests/session/mobile-session-stream-overload-isolation.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/agent/runtime/agentctl/agent.go`
- `apps/backend/internal/agent/runtime/lifecycle/streams.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go`
- `apps/backend/internal/orchestrator/agent_delivery_submission.go`
- `apps/web/hooks/domains/session/use-session-recovery-actions.ts`
- `apps/web/components/task/chat/session-stopped-banner.tsx`
- `apps/web/lib/session-recovery-presentation.ts`

Add named regression files beside the relevant production package.

## Dependencies

Task 06: Bound submission retention with safe idle rollover.

## Risks

Partial failure must preserve ownership, original submission identity, and replay evidence. Do not clear uncertainty solely because transport reconnects.

## Parallelism

`sequential`

## Inputs

- [Plan and source evidence](plan.md).
- [Delivery requirements](../../specs/platform/requirements/durable-agent-delivery.md).
- [Delivery design](../../specs/platform/system-design/durable-agent-delivery.md).
- Existing tests beside the listed files.

## Results

Implemented byte-bounded agent intake, durable overload reconnect from the SQL
cursor, explicit uncertain prompt outcomes, and state-only Retry connection
recovery. Added localized desktop and phone controls that keep Stop available,
plus Office and automation admission coverage through the existing guards.

Validation passed:

- `(cd apps/backend && go test -race ./internal/agent/runtime/agentctl ./internal/agent/runtime/lifecycle ./internal/orchestrator ./internal/office/service ./internal/automation -count=1)`
- `make -C apps/backend lint`
- `(cd apps && pnpm install --frozen-lockfile)`
- `(cd apps/web && pnpm test lib/session-recovery-presentation.test.ts hooks/domains/session/use-session-recovery-actions.test.ts hooks/domains/session/use-session-recovery-actions-guard.test.ts)`
- `(cd apps/web && pnpm lint)`
- `(cd apps/web && pnpm run typecheck)`
- `(cd apps/web && pnpm run i18n:check)`
- `(cd apps/web && pnpm run i18n:ratchet)`
- `(cd apps/web && pnpm e2e:run --project chromium tests/session/durable-stream-recovery.spec.ts tests/session/session-stream-overload-isolation.spec.ts)`
- `(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-durable-stream-recovery.spec.ts tests/session/mobile-session-stream-overload-isolation.spec.ts)`
- `python3 scripts/list-docs.py validate`
- `python3 scripts/lint-spec-files.py --all`
- `git diff --check`
