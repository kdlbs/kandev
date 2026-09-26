---
spec: ../../specs/tasks/requirements/prevent-agent-autostart-on-open.md
created: 2026-08-13
status: done
---

# Implementation Plan: Start-Agent Composer Hint

## Overview

PR #2556 added the Start agent button for recovered-idle (resume-skipped)
sessions in the message-list footer. The button sits at the bottom of the
transcript and can be clipped (only a few pixels of its top visible) when the
transcript is not fully scrolled down, e.g. right after navigating into a
task whose auto-scroll did not reach the bottom.

Product decision (2026-08-13): for recovered-idle sessions the footer Start
agent button is REMOVED and replaced by a single informational hint line in
the chat composer: "Sending a message will auto-start the agent." Sending a
message already is the explicit user action that starts the agent, so the
hint describes real behavior and removes the clipped-button failure entirely
(the composer sits outside the transcript scroll container and is always
visible). The never-started (`CREATED`) task-description Start agent button is
UNCHANGED: an explicitly-never-started task keeps its explicit Start agent
affordance under the task description.

The hint's promise must also hold for messages RECEIVED by the session, not
only the user's composer sends: a message from another task (a sub-task via
`message_task_kandev`) lands on the same message queue
(`QueueAndInterruptForPeerMessage` → `QueueMessageWithMetadata` with
`QueuedByAgent`), and the queue dispatch
(`takeIfPromptableLocked` → `executeQueuedMessage` → `PromptTask` →
`ensureSessionRunning`) resumes a stopped-but-resumable agent. This path
already exists on the backend and is never gated by the preference; it has no
direct test pinning "peer message at a stopped session → agent resumed", so a
backend regression test is added (task-04).

## Behavior

| Surface | Before | After |
|---|---|---|
| Recovered-idle session (resume-skipped, non-FAILED, no recovery actions, not STARTING/RUNNING) | footer `SessionResumeStartButton` ("Start agent") | composer hint line, no button |
| FAILED-but-resumable session | recovery actions own the surface (no button) | unchanged; hint also hidden |
| Never-started (`CREATED`) session | `TaskDescriptionStartButton` under the task description | unchanged; no hint |

The composer hint condition is the exact condition set the footer button used
today: `resumeSkipped && !hasRecoveryActions && sessionState not in
{FAILED, RUNNING, STARTING} && sessionId != null`.

## Frontend Changes

### 1. Shared predicate (`apps/web/lib/session-state.ts`)

Add a pure, exported `shouldShowComposerAgentStartHint({ resumeSkipped,
sessionState, hasRecoveryActions, sessionId })` next to the existing
`isLaunchStateRegression`, unit-tested in `apps/web/lib/session-state.test.ts`.
This keeps the visibility decision testable without rendering the composer.

### 2. Remove the footer resume button

- Delete `apps/web/components/task/chat/session-resume-start-button.tsx` (its
  launch/resume logic dies with it; no other file imports it).
- `apps/web/components/task/chat/message-list-footer.tsx`: remove the
  `resumeSkipped` selector, the `hasRecoveryActions` memo, the
  `showResumeStartButton` computation, and the `SessionResumeStartButton`
  render.
- `apps/web/components/task/chat/message-list-footer.test.tsx`: remove the
  `SessionResumeStartButton` mock and the "resume-skipped start button"
  describe blocks.
- `apps/web/components/task/chat/message-renderer.tsx`: remove the now
  unreachable `resumeSkipped` branch in `TaskDescriptionStartButton`'s
  `handleStart` (the description button only renders for CREATED sessions,
  which are never resume-skipped; the resume intent can no longer reach this
  button). Keep `shouldShowDescriptionStartButton` CREATED-only. Update the
  stale test comment in `message-renderer.test.ts` ("the footer owns that
  surface" → "the composer hint owns that surface").
- i18n: no copy change here; `task:startAgent` stays (used by the CREATED
  description button).

### 3. Composer hint UI

- New hook `useComposerAgentStartHint(sessionId, sessionState, messages,
  footerActionMessages)` in `apps/web/components/task/chat/` (+ test): reads
  `state.tasks.resumeSkippedSessionIds[sessionId] === true` from the store,
  derives `hasRecoveryActions` from `messages`/`footerActionMessages`
  (`metadata.recovery_actions === true`, same rule the footer used), and
  calls the shared predicate.
- `apps/web/components/task/task-chat-panel.tsx`: compute
  `showAgentStartHint` from the hook (session id, `session?.state`,
  `allMessages`, `footerActionMessages`) and thread it through `ChatFooter`
  into `ChatInputArea` (`apps/web/components/task/chat/chat-input-area.tsx`)
  as a new `showAgentStartHint?: boolean` prop.
- `chat-input-area.tsx`: render a muted one-line hint above
  `QueueAffordance`, e.g.
  `<p data-testid="composer-agent-start-hint" className="...">` with
  `t("task:composerStartAgentHint")`. No click action.
- i18n: add `composerStartAgentHint` to
  `apps/web/src/locales/{en,pseudo,pt-pt,zh-cn}/task.json`. Copy: "Sending a
  message will auto-start the agent." (plain punctuation, no em dash).
- Component test: extend `chat-input-area.test.tsx` (or a focused test) to
  assert the hint renders when `showAgentStartHint` is true and is absent
  when false.

## E2E Changes

- `apps/web/e2e/tests/settings/prevent-auto-start-on-open.spec.ts`,
  "post-restart session does not auto-resume when the preference is on":
  - assert `composer-agent-start-hint` visible and
    `session-resume-start-button` count 0 (no "Resumed agent" on its own);
  - send `/e2e:simple-message` via the composer (`session.sendMessage`) and
    assert the mock response appears and "Resumed agent" becomes visible
    (sending a message is now the explicit start).
  - The two final-step (CREATED) tests keep asserting
    `task-description-start-button`; unchanged.
- Mobile (`mobile-chrome` project): new
  `apps/web/e2e/tests/settings/mobile-agent-start-hint.spec.ts` using the
  same restart pattern (task + first turn → `backend.restart()` →
  `testPage.reload()`) on a phone viewport: hint visible, no resume button,
  send message → mock response. The existing
  `mobile-prevent-auto-start-on-open.spec.ts` (CREATED button) stays green.

## Risks

- The desktop restart test's send-message path must prove auto-start-on-send
  (same mock flow as today's post-click send; the mock agent answers
  `/e2e:simple-message` after a resumed launch).
- Mobile restart flow is slower (desktop restart test already needs
  `test.setTimeout(180_000)`); reuse the same timeouts.
- i18n ratchet: the hint copy must go through `t()` in the component, and the
  new key must exist in all four locale files (`i18n:check` fails otherwise).

## Verification

```bash
(cd apps/web && pnpm run typecheck)
```

```bash
(cd apps/web && pnpm vitest run lib/session-state.test.ts components/task/chat/message-list-footer.test.tsx components/task/chat/message-renderer.test.ts components/task/chat/chat-input-area.test.tsx)
```

```bash
(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)
```

```bash
(cd apps/backend && go test ./internal/orchestrator/ -run TestQueueAndInterruptForPeerMessage_StoppedSessionResumesAgent -race)
```

```bash
(cd apps/web && KANDEV_E2E_MOCK=true pnpm e2e:raw --project=chromium settings/prevent-auto-start-on-open.spec.ts)
(cd apps/web && KANDEV_E2E_MOCK=true pnpm e2e:raw --project=mobile-chrome settings/mobile-agent-start-hint.spec.ts settings/mobile-prevent-auto-start-on-open.spec.ts)
```

## Implementation Waves And Parallel Candidates

```
Wave 1 (sequential):
- [x] [task-01-shared-predicate-and-removal](task-01-shared-predicate-and-removal.md)
- [x] [task-02-composer-hint-ui](task-02-composer-hint-ui.md)
- [x] [task-03-e2e](task-03-e2e.md)
- [x] [task-04-peer-message-resumes-stopped-session](task-04-peer-message-resumes-stopped-session.md)
```

Tasks 01→03 are sequential (02 depends on 01's predicate, 03 depends on 02's
rendered hint). Task 04 is backend-only and independent of the frontend
tasks; it can run at any point in the wave.

## Verification Results

- Frontend: `make typecheck` and `make fmt` clean; `make lint` clean (0
  warnings). Targeted vitest (session-state, message-list-footer,
  message-renderer, use-composer-agent-start-hint, composer-agent-start-hint)
  38/38 passing. `pnpm run i18n:check` + `pnpm run i18n:ratchet` clean.
- Backend: `go test ./internal/orchestrator/ -count=1` clean (75s, includes
  the new `TestQueueAndInterruptForPeerMessage_StoppedSessionResumesAgent`
  with `-race`).
- E2E (mock agent): `settings/prevent-auto-start-on-open.spec.ts` on
  chromium 3/3; `settings/mobile-agent-start-hint.spec.ts` +
  `settings/mobile-prevent-auto-start-on-open.spec.ts` on mobile-chrome 2/2.
- Full `make test` was NOT clean on this VM, but every failure is unrelated
  to this change: `internal/agentctl/server/{api,config,process}`,
  `internal/launcher`, `internal/system/updates` reproduce on a clean tree
  (host init-system/process-output environment tests), and the web suite's
  18 failures are load-flaky 5-10s timeouts (git-server, azure-devops,
  sentry, file-browser, settings, git-base) that all pass in isolation on
  both the clean tree and this branch. `lib/http-git-server.test.ts` fails
  on the clean tree too.
