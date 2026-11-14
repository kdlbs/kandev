---
id: "01-gate-idle-suspended-focus-resume"
title: "Gate idle-suspended focus resume on the preference"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-IDLE-PARKING-001
acceptance_criteria:
  - AC-EXECUTORS-IDLE-PARKING-001.5
system_design:
  - ../../specs/executors/system-design/idle-runtime-parking.md
---

# Task 01: Gate Idle-Suspended Focus Resume

## Summary

With `preventAutoStartAgentOnOpen` enabled, opening a task or returning focus
to it must not resume an idle-suspended session. Record the resume-skipped
marker so the existing composer hint is shown.

## In scope

- `apps/web/hooks/domains/session/use-session-resumption.ts`, function
  `performResumeAction`: at the start of the
  `status.is_idle_suspended && !status.is_agent_running` branch, when
  `preventAutoStart` is true, call `recordResumeSkipIfStopped(setters,
  sessionId)`, set the resumption state to `"idle"`, and return `false`. This
  covers both `processResumeStatus` (open) and `handleVisibleFocus` (window
  focus or visibility return).
- `apps/web/hooks/domains/session/use-session-resumption.test.ts`:
  - Rewrite `resumes the selected session without a prompt despite the
    preference` as `keeps an idle-suspended session stopped when the
    preference is on`. Assert that no `session.launch` is sent and that
    `mockSetResumeSkipped` is called with `(SESSION_ID, true)`. Run it before
    the fix and confirm that it fails because the launch was sent.
  - Add a window-focus case: the preference is on, the status is
    idle-suspended, and a `visibilitychange`/`focus` event fires. Assert that
    no `session.launch` is sent.
  - Keep or add a preference-off case that proves the `session_focus` resume
    still happens.
- Settings copy: update `preventAutoStartAgentOnOpenHelp` and
  `preventStartShort` in `apps/web/src/locales/en/settings.json` to also cover
  agents paused for inactivity, for example "...after a restart, after being
  paused for inactivity, or from the final workflow step." Do not use em
  dashes. Translate the same keys in `pt-pt`, `zh-cn`, `ja`, and `ko`, then run
  `pnpm run i18n:zh-hant` for `zh-tw` and `zh-hk`.

## Out of scope

- Backend `focusTaskSessionAdmitted` and the idle reaper.
- `decideResumeAction`, which already returns `"idle"` for
  `is_idle_suspended`. Leave it unchanged.
- E2E coverage (Task 02).

## Acceptance

- With the preference on, an idle-suspended status on open or on window focus
  sends no `session.launch` and marks the session resume-skipped.
- With the preference off, the existing `session_focus` resume behavior is
  unchanged.
- The settings help text describes the idle-paused case in all locales, and
  `i18n:check` passes.

## Verification

```bash
cd apps/web && pnpm exec vitest run hooks/domains/session/use-session-resumption.test.ts
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run i18n:check
cd apps && pnpm --filter @kandev/web lint
```

## Likely files

- `apps/web/hooks/domains/session/use-session-resumption.ts`
- `apps/web/hooks/domains/session/use-session-resumption.test.ts`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-tw,zh-hk,ja,ko}/settings.json`

## Results

Completed 2026-11-03.

- `performResumeAction` gates the idle-suspended branch on `preventAutoStart`,
  records `setResumeSkipped(sessionId, true)`, sets `"idle"`, and returns
  before any launch. The existing preferred-off path is unchanged.
- Visible restart priority for a session with an `agent_delivery` recovery
  block applies the same preference gate when the stored session state can be
  idle-suspended; non-suspendable states keep their early recovery focus.
- Unit coverage: preference on keeps the session stopped on open and on window
  focus; preference off still sends `activation_source=session_focus`; restart
  priority is skipped only for suspendable states and a live settled session
  still receives one focus; the hidden-view test now exercises its own
  visibility guard, request invalidation still suppresses the launch after an
  error, and the in-flight startup join test runs with the preference off.
- Locale copy updated in `en`, `pt-pt`, `zh-cn`, `ja`, `ko`, the reviewed
  `zh-tw`/`zh-hk` overrides, and generated `pseudo`. The help now says the
  agent stays stopped until the user starts it or sends a message, matching
  the composer hint instead of promising a button.
- Verification: `vitest` 54 passed (new restart-priority test failed before the
  fix), `tsc --noEmit`, targeted ESLint, `i18n:check`, and `e2e:sleep-ratchet`
  pass.
