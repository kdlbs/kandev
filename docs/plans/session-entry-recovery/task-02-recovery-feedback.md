---
id: "02-recovery-feedback"
title: "Present conversation recovery"
status: pending
wave: 2
depends_on: ["01-entry-recovery"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-002
  - REQ-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-001
acceptance_criteria:
  - AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-002.1
  - AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-002.2
  - AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-002.3
  - AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-002.4
  - AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-002.5
  - AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-002.6
  - AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-002.7
  - AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-002.8
  - AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-002.9
  - AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-002.10
system_design:
  - ../../specs/platform/system-design/session-subscription-recovery.md
---

# Task 02: Present conversation recovery

## Summary

Connect entry recovery to compact chat feedback on desktop, phone, and preview.
Prove the reported delay and automatic recovery with isolated browser tests.

## In scope

- Explicit history loading/error/empty rendering and retry controls.
- Typed status-check feedback with collapsed technical details.
- Localized copy, accessible progress, phone targets, and request-aware browser tests.

## Out of scope

Changes to provider failures, launch policy, server transport, or global alert styling.

## Acceptance

- Unknown history never renders the empty invitation; cached messages remain visible after errors.
- Status timeouts use accurate compact feedback, and successful recovery clears that feedback without changing launch-error actions.
- Desktop and phone recover without reload; Retry and Details work through visible controls with correct touch geometry.

## ASCII UI preview

UI-01: Open existing task, current failure (desktop and phone).

```text
[!] Couldn't start a session
    WebSocket request timed out: task.session.status
    [Retry]

    No messages yet. Start the conversation!
```

UI-02: Open existing task, proposed desktop chat region.

```text
Loading:   (spinner) Loading conversation...
Retrying:  (spinner) Taking longer than usual. Retrying...
Exhausted: Conversation could not load.  [Retry] [Details v]
Ready:     <conversation, or confirmed empty invitation>
```

UI-03: Same task, proposed phone chat region.

```text
+------------------------------------+
| Conversation could not load.        |
| [ Retry ]  [ Details v ]            |
|                                    |
| <cached conversation stays visible>|
+------------------------------------+
| <existing composer and navigation> |
+------------------------------------+
```

UI-04: Status-only exhaustion with a readable transcript.

```text
Session status is unavailable. [Retry] [Details v]
<conversation remains visible and scrollable>
```

Details expands inline with wrapped technical text. Loading and retrying use a
neutral status region. Neither uses a destructive alert or the empty invitation.
Phone actions occupy their own row and have at least 44-pixel targets.
Desktop actions use the normal 28-pixel size. The chat remains the scroll owner;
existing composer placement and safe-area behavior remain unchanged.
These hierarchy and state choices are required; wording and spacing are illustrative.
All text is localized. Covers AC-002.4 through AC-002.6 and AC-002.8 through AC-002.10
(where AC-002 denotes AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-002).

Full combined preview: [plan](plan.md#ascii-ui-preview).

## Verification

Write component RED for the false empty state before rendering changes.
Use a controlled response barrier for E2E RED where feasible; record any fixture limitation.

```bash
(cd apps/web && pnpm exec vitest run components/task/chat/session-entry-feedback.test.tsx components/task/ensure-session-error.test.tsx hooks/domains/session/use-session-messages.test.ts hooks/domains/session/use-session-resumption.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint --max-warnings 0 components/task/ensure-session-error.tsx components/task/task-page-inner.tsx components/task/preview-session-tabs.tsx components/task/chat components/task/task-chat-panel.tsx e2e/helpers/session-entry-recovery.ts e2e/tests/session/session-entry-recovery.spec.ts e2e/tests/session/mobile-session-entry-recovery.spec.ts)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/session/session-entry-recovery.spec.ts tests/session/session-resume-recovery.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-session-entry-recovery.spec.ts tests/session/mobile-session-resume-recovery.spec.ts)
git diff --check
```

Managed E2E commands rebuild production assets and use isolated backends.
Run desktop and mobile commands sequentially. Inspect their screenshots against UI-02 through UI-04.
The desktop suite also exercises preview; phone uses the existing direct task route.
Capture request IDs/actions without payload contents. Preserve unrelated frames in the proxy.

## Files likely touched

- `apps/web/components/task/ensure-session-error.tsx` and its tests
- `apps/web/components/task/task-page-inner.tsx`
- `apps/web/components/task/preview-session-tabs.tsx`
- `apps/web/components/task/chat/use-chat-panel-state.ts`
- `apps/web/components/task/chat/message-list-shared.tsx`
- `apps/web/components/task/task-chat-panel.tsx`
- New `apps/web/components/task/chat/session-entry-feedback.test.tsx`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw}/task.json`
- New `apps/web/e2e/helpers/session-entry-recovery.ts`
- New `apps/web/e2e/tests/session/session-entry-recovery.spec.ts`
- New `apps/web/e2e/tests/session/mobile-session-entry-recovery.spec.ts`

## Dependencies

Task 01 supplies typed recovery state and bounded retry actions.

## Risks

Duplicate feedback from page and transcript consumers, desktop sizing leaking to phone, and untranslated diagnostic copy.

## Parallelism

`sequential`

## Inputs

- Requirement AC-002.1 through AC-002.10 and system design Presentation section.
- Task 01 hook results; existing shared transcript loading renderer.
- Mobile exemplar: `components/task/task-layout.tsx` dedicated phone chat composition.
- Browser routing exemplar: `e2e/helpers/session-capabilities.ts`.

## Results

Pending.
