---
id: "08-steering-composer"
title: "Explicit composer delivery on desktop and phone"
status: pending
wave: 7
depends_on:
  - "05-shared-ui"
  - "07-native-turn-steering"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-EXPLICIT-STEERING-001
  - REQ-PLATFORM-EXPLICIT-STEERING-002
  - REQ-PLATFORM-EXPLICIT-STEERING-003
acceptance_criteria:
  - AC-PLATFORM-EXPLICIT-STEERING-001.1
  - AC-PLATFORM-EXPLICIT-STEERING-001.2
  - AC-PLATFORM-EXPLICIT-STEERING-001.3
  - AC-PLATFORM-EXPLICIT-STEERING-001.4
  - AC-PLATFORM-EXPLICIT-STEERING-001.5
  - AC-PLATFORM-EXPLICIT-STEERING-002.1
  - AC-PLATFORM-EXPLICIT-STEERING-002.2
  - AC-PLATFORM-EXPLICIT-STEERING-002.3
  - AC-PLATFORM-EXPLICIT-STEERING-002.4
  - AC-PLATFORM-EXPLICIT-STEERING-002.5
  - AC-PLATFORM-EXPLICIT-STEERING-003.1
  - AC-PLATFORM-EXPLICIT-STEERING-003.2
  - AC-PLATFORM-EXPLICIT-STEERING-003.3
  - AC-PLATFORM-EXPLICIT-STEERING-003.4
system_design:
  - ../../specs/platform/system-design/explicit-turn-steering.md
---

# Task 08: Explicit composer delivery on desktop and phone

## Summary

Expose Send now and Queue for later for native same-turn sessions in the shared
main composer. Preserve ACP's truthful delivery semantics and route explicit
intent through the backend even when Kandev has queued messages.

## In scope

- Normalized capability modes, immutable turn_ref capture and explicit submit
  intent in session-input-mode/use-message-handler and domain API, independent
  from background-job counts and existing supports_steering boolean fallback.
- Main Send now action plus delivery choice menu for same_turn, including with
  nonempty queue; Queue for later always invokes existing queue admission.
  Share logic across Task chat and Quick Chat; preserve passthrough behavior.
- Desktop split-button/menu and phone inset selection Drawer with visible 44px
  trigger, keyboard/safe-area clearance, focus return and localized reasons.
- Pending/accepted/rejected/uncertain message status, target invalidation and
  preserved draft revision/attachments. Duplicate clicks reuse the admitted ID;
  no implicit retry, queue insertion, or mode switch. Later deliberate input is
  a distinct action with truthful uncertainty feedback when relevant.
- Feature-off, old metadata, provider_managed and unsupported modes retain
  existing behavior/copy. Incompatible new model/options/attachments disable
  same-turn submission with a reason rather than being silently ignored.
- Native fake-server browser scenarios prove two user inputs are attributed to
  one root turn; queue unchanged by Send now, later queue drain intact, echo
  dedup and child/background-work tabs unaffected. Add conditional ACP fixtures.
- Six language catalogs plus pseudo; use existing Traditional Chinese generator.

## Out of scope

A composer in child-agent tabs, provider-specific UI branches, new task/session
creation, child prompting, and background-process input controls.

## Acceptance

1. Unit/integration tests cover mode/capability/queue combinations, explicit
   intent serialization, target change, pending acknowledgement, busy rejection,
   uncertain receipt replay and late response versus newer draft/message state.
2. Desktop and phone E2E deliver to the active fake Codex turn with two existing
   queued messages, leave them intact, send another steer after ack, and verify
   one root completion plus no duplicate echoed user message.
3. Old ACP/off-flag/unsupported behavior stays unchanged. Phone mode choice has
   measured 44px targets and keyboard clearance; target expiry/error preserves
   the draft, and all translation checks pass.

## ASCII UI preview

UI-04 / UI-05 from [the plan](plan.md#ui-04-main-conversation-delivery-choice):

```text
Desktop: main conversation still running
 ( 2 Background Jobs )
+----------------------------------------------------+
| Keep the public API unchanged...                   |
|                                   [Send now] [v]   |
+----------------------------------------------------+
 Queue: 2 messages for later
                         +--------------------------+
                         | Send now                 |
                         | Queue for later          |
                         +--------------------------+

Phone: main conversation still running
 ( 2 Background Jobs )
+-----------------------------------+
| Keep the public API unchanged...  |
|                    [Send now] [v] |
+-----------------------------------+
 [2 queued for later]
 Tap v: inset choice drawer
+-----------------------------------+
| Send now                          |
| Add to the current turn           |
| Queue for later                   |
| Run after current work            |
+-----------------------------------+
```

The pill remains compact. Main-chat delivery is independent from job-detail
panels, which gain no composer. Desktop uses existing toolbar density; phone
has at least 44px targets, one short choice drawer, focus return, and keyboard
clearance. Structure and distinct actions are required; example copy is localized.

```text
Pending:     Sending to active turn...       [Send now disabled]
Accepted:    Sent to active turn
Rejected:    That turn ended. Draft retained. [Choose delivery]
Uncertain:   Delivery unknown. It may already have arrived.
Unsupported: Same-turn delivery unavailable. [Queue for later]
```

These states cover 001.1-.5, 002.1-.3 and 003.1-.4. Never render Accepted as
"agent acted on it" or label a silently queued message as sent to the active turn.

## TDD and verification

Write behavioral tests first. Run from repository root; install workspace
dependencies first only if this is a fresh worktree. Use causal event/HTTP waits.

```bash
(cd apps/web && pnpm test -- hooks/domains/session/session-input-mode.test.ts hooks/use-message-handler.test.ts lib/api/domains/steering-delivery.test.ts components/task/chat/steering-delivery-controls.test.tsx)
(cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/native-steering.spec.ts tests/chat/background-work.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-native-steering.spec.ts tests/chat/mobile-background-work.spec.ts)
git diff --check
```

## Files likely touched

- Existing `apps/web/hooks/use-message-handler.ts`,
  `hooks/domains/session/session-input-mode.ts`, session-state projection,
  sendMessageRequest client and focused tests.
- Proposed `apps/web/lib/api/domains/steering-delivery.test.ts` targeting the
  existing submission client, not a parallel transport implementation.
- Existing chat toolbar/input-container/composer hooks and Quick Chat integration;
  proposed `components/task/chat/steering-delivery-controls.tsx` and `.test.tsx`.
- Session capability/message-delivery types, reducers and websocket handling;
  locale catalogs, native fake-server fixture helpers.
- Proposed `apps/web/e2e/tests/chat/native-steering.spec.ts` and
  `mobile-native-steering.spec.ts`.

## Dependencies

Tasks 05 and 07. Task 06's executor fixture/public-doc delivery follows this work.

## Risks

The old queued-count branch can intercept explicit Send now. A delayed accepted
response must not clear newer edits. Retrying a timed-out send can duplicate it.
The native capability must not accidentally promise same-turn folding for ACP.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/platform/requirements/explicit-turn-steering.md).
- [Design](../../specs/platform/system-design/explicit-turn-steering.md).
- [ADR](../../decisions/2026-09-27-explicit-same-turn-steering.md).
- Existing session-input-mode, use-message-handler and composer tests; existing
  mobile picker primitives and native background-work E2E fixture from Task 05.

## Results

Pending.
