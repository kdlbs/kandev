---
id: agent-message-selection-release-01
title: Capture selections released outside the message
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-UI-AGENT-MESSAGE-COMMENTS-001
acceptance_criteria:
  - AC-UI-AGENT-MESSAGE-COMMENTS-001.1
  - AC-UI-AGENT-MESSAGE-COMMENTS-001.2
  - AC-UI-AGENT-MESSAGE-COMMENTS-001.3
system_design:
  - ../../specs/ui/system-design/agent-message-selection.md
---

# Capture selections released outside the message

## Scope and acceptance

- A valid selection exposes the comment trigger when mouse release occurs in
  surrounding padding, in Task Chat and Quick Chat.
- Outside, cross-message, active-turn, and sessionless selections remain rejected.
- Mouse release inside an open editor preserves the selected quote and feedback.
- An unrelated control cannot restore a dismissed trigger from a preserved selection.

Touch capture, layout, persistence, and delivery are outside the repair scope.

## Files

- `apps/web/components/task/chat/messages/message-comment-surface.tsx`
- `apps/web/components/task/chat/messages/message-comment-surface.test.tsx`
- `apps/web/e2e/tests/chat/agent-message-comments.spec.ts`
- `apps/web/e2e/tests/chat/agent-message-comments-helpers.ts`
- `apps/web/components/task/chat/messages/use-message-comment-dom.ts`

## Verification

Run from `apps/web`:

```bash
pnpm exec vitest run components/task/chat/messages/message-comment-surface.test.tsx lib/chat/agent-message-comments.test.ts
pnpm e2e:run --host e2e/tests/chat/agent-message-comments.spec.ts
pnpm e2e:run --host --no-build --project mobile-chrome e2e/tests/chat/mobile-agent-message-comments.spec.ts
NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck
pnpm exec eslint --max-warnings 0 components/task/chat/messages/message-comment-surface.tsx components/task/chat/messages/message-comment-surface.test.tsx e2e/tests/chat/agent-message-comments.spec.ts e2e/tests/chat/agent-message-comments-helpers.ts components/task/chat/messages/use-message-comment-dom.ts
pnpm run i18n:ratchet
```

## Results

The component regression and both real-drag browser cases failed before the
repair because the trigger was absent. After the repair, 20 focused tests,
four desktop cases, and two mobile cases passed. Static checks passed.
The desktop cases passed across targeted runs after correcting a test assertion
to allow the quotation marks rendered around the selected text.

Local browser runs used `KANDEV_SERVER_HOST=127.0.0.1`, IPv4-first DNS, and the
installed Nix browser libraries. These are runner settings, not product changes.

## Review remediation

CodeRabbit identified stale selection capture after an unrelated mouse click.
The unit and browser regressions reproduced the trigger returning after dismissal.
The shared DOM hook now pairs mouse-down origin with document-level mouse release.
Touch capture remains independent. After remediation, all 21 focused tests,
five desktop browser cases, and two mobile browser cases passed. Typecheck,
changed-file ESLint, formatting, translation checks, and specification lint passed.

## CI remediation

E2E shard 4 exposed a readiness race in the existing keyboard queue-reorder test.
The test tried to focus a disabled handle during a queue refresh. The handle later became
enabled but remained unfocused. The helper now waits for the enabled state before
focusing. Queue behavior and the comment UI do not change.

The CI trace supplied the failing sequence. A standalone Chromium check confirmed
that a focus attempt on a disabled button does not take effect after enabling it. The
original keyboard case passed three isolated runs. With the readiness assertion,
both queue-reorder cases passed three runs each, without retries, on the same
base-plus-head source tree as CI. Changed-file ESLint and formatting passed.

Run from `apps/web` with a current E2E build:

```bash
pnpm e2e:run --host --no-build e2e/tests/chat/message-queue-reorder.spec.ts -- --repeat-each 3 --retries 0
pnpm exec eslint --max-warnings 0 e2e/tests/chat/message-queue-reorder.spec.ts
```

Remote CI and review results are tracked on the PR.
