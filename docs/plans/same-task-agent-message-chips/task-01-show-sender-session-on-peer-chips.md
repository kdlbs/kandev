---
id: "01-show-sender-session-on-peer-chips"
title: "Show sender session on peer chips"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SAME-TASK-AGENT-ATTRIBUTION-001
acceptance_criteria:
  - AC-UI-SAME-TASK-AGENT-ATTRIBUTION-001.1
  - AC-UI-SAME-TASK-AGENT-ATTRIBUTION-001.2
  - AC-UI-SAME-TASK-AGENT-ATTRIBUTION-001.3
  - AC-UI-SAME-TASK-AGENT-ATTRIBUTION-001.4
  - AC-UI-SAME-TASK-AGENT-ATTRIBUTION-001.5
system_design:
  - ../../specs/ui/system-design/same-task-agent-message-attribution.md
---

# Task 01: Show Sender Session on Peer Chips

## Summary

Use the visible session-tab label to identify a same-task peer message in the
chat transcript and queue. Preserve cross-task source context, and prove the
new presentation on desktop and phone.

## In scope

- Add a failing component test for two sender sessions in one task, including
  unnamed sessions whose model labels differ.
- Share session label resolution with the tab, then update the shared sender
  badge and its transcript and queued callers.
- Localize new context and fallback strings in all maintained locales.
- Make same-task context available through a semantic keyboard and touch
  control while preserving cross-task source links.
- Add desktop and mobile browser scenarios that send through
  `message_task_kandev` to an explicit sibling session and inspect persisted
  queue metadata.

## Out of scope

- Backend message delivery and sender metadata changes.
- Session tab styling or rename behavior.
- Cross-task navigation redesign.

## Acceptance

- A transcript or queue message from a loaded sibling session shows the same
  readable label as that session's task tab; a different sender shows a
  different label when the tab labels differ.
- A renamed or unloaded sender follows the requirement's live/snapshot/ID
  fallback order. A long label keeps its full sender and task context available
  through keyboard, click, and touch interaction.
- Cross-task messages still show their source task and link to it; desktop and
  phone browser checks pass without horizontal overflow.

## ASCII UI preview

`UI-01: Same-task transcript` (see the [full preview](plan.md#ascii-ui-preview);
`AC-UI-SAME-TASK-AGENT-ATTRIBUTION-001.1`, `.3`):

```text
Before: [robot Review Contributor PR #3143]  Can you inspect the PR?
After:  [robot Luna]                         Can you inspect the PR?
```

`UI-02: Same-task queue and phone` (`AC-UI-SAME-TASK-AGENT-ATTRIBUTION-001.2`, `.3`):

```text
Phone: [robot Luna]  Can you inspect the PR?
Queue: [robot Astra] Here is the finding.
```

The existing message or queue surface remains the scroll owner. Full sender
and task context is accessible when the short chip truncates.

## Verification

Run from the repository root:

```bash
(cd apps/web && pnpm exec vitest run components/task/chat/messages/sender-task-badge.test.tsx components/task/chat/messages/chat-message.test.tsx components/task/chat/queued-ghost-message.test.tsx components/task/session-tab-title.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm exec eslint components/task/chat/messages/sender-task-badge.tsx components/task/chat/messages/chat-message.tsx components/task/chat/queued-ghost-message.tsx components/task/session-tab.tsx components/task/session-tab-title.ts)
(cd apps/web && pnpm e2e:run tests/chat/agent-message-attribution.spec.ts)
(cd apps/web && pnpm e2e:run --project=mobile-chrome tests/chat/mobile-agent-message-attribution.spec.ts)
git diff --check
```

In a fresh worktree, run `(cd apps && pnpm install --frozen-lockfile)` before
the first package command. Record actual test counts and failures here.

## Files likely touched

- `apps/web/components/task/chat/messages/sender-task-badge.tsx`
- `apps/web/components/task/chat/messages/sender-task-badge.test.tsx`
- `apps/web/components/task/chat/messages/chat-message.tsx`
- `apps/web/components/task/chat/messages/chat-message.test.tsx`
- `apps/web/components/task/chat/queued-ghost-message.tsx`
- `apps/web/components/task/chat/queued-ghost-message.test.tsx`
- `apps/web/components/task/session-tab.tsx`
- `apps/web/components/task/session-tab-title.ts`
- `apps/web/src/locales/*/task.json`
- `apps/web/e2e/tests/chat/agent-message-attribution.spec.ts`
- `apps/web/e2e/tests/chat/mobile-agent-message-attribution.spec.ts`

## Dependencies

None.

## Risks

- A sender session ID must be checked against the destination task before
  its label is displayed.
- A model switch can change a live tab label after the message was sent; the
  chip should reflect the same current label rather than a stale snapshot.

## Parallelism

`sequential`

## Inputs

- `docs/specs/ui/requirements/same-task-agent-message-attribution.md`
- `docs/specs/ui/system-design/same-task-agent-message-attribution.md`
- Existing `SenderTaskBadge`, `SessionTab`, and message attribution tests.

## Results

Review follow-up verification:

- `pnpm exec vitest run components/task/chat/messages/sender-task-badge.test.tsx components/task/chat/messages/chat-message.test.tsx components/task/chat/queued-ghost-message.test.tsx components/task/session-tab-title.test.ts`: 103 tests passed across four files.
- `pnpm run typecheck`: passed.
- `pnpm run i18n:check && pnpm run i18n:ratchet`: passed.
- `pnpm exec eslint components/task/chat/messages/chat-message.tsx components/task/chat/messages/sender-task-badge.test.tsx components/task/chat/messages/sender-task-badge.tsx components/task/chat/queued-ghost-message.tsx hooks/domains/session/use-sender-task-badge-model.ts e2e/tests/chat/agent-message-attribution.spec.ts e2e/tests/chat/mobile-agent-message-attribution.spec.ts`: passed.
- `pnpm exec prettier --check components/task/chat/messages/chat-message.tsx components/task/chat/messages/sender-task-badge.test.tsx components/task/chat/messages/sender-task-badge.tsx components/task/chat/queued-ghost-message.tsx hooks/domains/session/use-sender-task-badge-model.ts e2e/tests/chat/agent-message-attribution.spec.ts e2e/tests/chat/mobile-agent-message-attribution.spec.ts`: passed.
- `python3 scripts/list-docs.py validate` and `python3 scripts/lint-spec-files.py --all`: passed; 320 decisions and 1220 specifications validated.
- `git diff --check`: passed.
- `CAPTURE_PR_ASSETS=1 pnpm e2e:run tests/chat/agent-message-attribution.spec.ts`: 10 tests passed.
- `CAPTURE_PR_ASSETS=1 pnpm e2e:run --project=mobile-chrome tests/chat/mobile-agent-message-attribution.spec.ts`: 1 test passed.

Both browser scenarios send through `message_task_kandev` with an explicit
distinct sibling `session_id`, then verify persisted agent queue metadata
before checking the chip. The desktop scenario verifies normal tab access and
Enter/Space activation of the full context. The phone scenario verifies tap
access, a 44px touch target, popover viewport containment, and no overlap with
queue actions at 320px width. An empty sender session ID uses the existing task
attribution path. Sender model selectors now live in the session domain hook,
leaving the badge component below the 200-line limit.
