---
id: "02-composer-hint-ui"
title: "Composer hint UI and i18n"
status: done
wave: 1
parallelism: sequential
depends_on: ["01-shared-predicate-and-removal"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/prevent-agent-autostart-on-open.md"
---

# Task 02: Composer hint UI and i18n

## Acceptance

- `useComposerAgentStartHint(sessionId, sessionState, messages,
  footerActionMessages)` (new hook in `apps/web/components/task/chat/`)
  returns `shouldShowComposerAgentStartHint(...)` with `resumeSkipped` read
  from `state.tasks.resumeSkippedSessionIds[sessionId] === true` and
  `hasRecoveryActions` derived from `messages`/`footerActionMessages` using
  the same rule the footer used (`metadata.recovery_actions === true` on any
  message). `sessionId === null` short-circuits to false. Hook unit tests
  cover: resume-skipped + `WAITING_FOR_INPUT` + no recovery → true; FAILED /
  STARTING / RUNNING → false; recovery action present in `messages` or
  `footerActionMessages` → false; no resume-skipped flag → false.
- `apps/web/components/task/task-chat-panel.tsx` computes
  `showAgentStartHint` from the hook (`session?.state`, `allMessages`,
  `footerActionMessages`) and passes it through `ChatFooter` to
  `ChatInputArea` (`apps/web/components/task/chat/chat-input-area.tsx`) as a
  new optional `showAgentStartHint` prop.
- `chat-input-area.tsx` renders, above `QueueAffordance`, a single muted
  one-line hint when `showAgentStartHint` is true:
  `<p data-testid="composer-agent-start-hint" …>{t("task:composerStartAgentHint")}</p>`.
  It is absent when the prop is false/undefined. No click handler.
- Component test (extend `chat-input-area.test.tsx` or a focused test)
  asserts the hint element renders for `showAgentStartHint` true and is
  absent for false.
- i18n: `composerStartAgentHint` added to
  `apps/web/src/locales/{en,pseudo,pt-pt,zh-cn}/task.json` with copy "Sending
  a message will auto-start the agent." (plain punctuation; pseudo and
  translations follow the existing per-locale style). `pnpm run i18n:check`
  and `pnpm run i18n:ratchet` pass.

## Verification

```bash
(cd apps/web && pnpm run typecheck)
```

```bash
(cd apps/web && pnpm vitest run components/task/chat/chat-input-area.test.tsx components/task/chat/use-composer-agent-start-hint.test.ts)
```

```bash
(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)
```

## Files Likely Touched

- `apps/web/components/task/chat/use-composer-agent-start-hint.ts` (new, + test)
- `apps/web/components/task/task-chat-panel.tsx` (`ChatFooter` props)
- `apps/web/components/task/chat/chat-input-area.tsx` (+ test)
- `apps/web/src/locales/en/task.json`, `pseudo/task.json`,
  `pt-pt/task.json`, `zh-cn/task.json`

## Dependencies

Task 01 (the `shouldShowComposerAgentStartHint` predicate and the removal of
the footer button, so no second affordance competes with the hint).

## Inputs

- Spec: "What" bullet 3 (hint visibility rules), scenarios 6 and 7.
- `ChatInputArea` render at `chat-input-area.tsx:629-659` (the
  `data-testid="chat-input-area"` wrapper above `QueueAffordance`).
- `ChatFooter` prop threading at `task-chat-panel.tsx:400-418` and
  `484-528`.

## Output Contract

A recovered-idle session shows exactly one affordance: the composer hint
("Sending a message will auto-start the agent."), always visible because the
composer is outside the transcript scroll container. The hint disappears when
the session starts/runs, fails, or recovery actions are visible. The CREATED
description button remains the never-started affordance.
