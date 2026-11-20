---
created: 2026-11-03
status: implemented
requirements:
  - REQ-EXECUTORS-IDLE-PARKING-001
system_design:
  - ../../specs/executors/system-design/idle-runtime-parking.md
legacy_specs: []
---

# Implementation Plan: Honor Prevent Auto-Start for Idle-Suspended Sessions

## Overview

Fix [#5033](https://github.com/kdlbs/kandev/issues/5033). With **Prevent
auto-start on open** enabled, opening an idle task must not start its agent.
That includes a session suspended by the workspace ACP idle-suspension
policy. The session stays suspended and the composer shows the existing
auto-start hint. An accepted message still resumes the same conversation.

## Confirmed root cause

- The workspace idle-suspension policy (#4042) suspends settled ACP sessions
  after the configured timeout. A task left idle for about 7 hours matches this
  case.
- `performResumeAction` in
  `apps/web/hooks/domains/session/use-session-resumption.ts` handles
  `status.is_idle_suspended && !status.is_agent_running` first. It calls
  `resumeWithSilentFallback(..., { activationSource: "session_focus" })` and
  returns before `decideResumeAction(status, preventAutoStart)` runs. As a
  result, the preference is never consulted for idle-suspended sessions.
- The same function also handles window-focus/visibility return
  (`handleVisibleFocus` → `performResumeAction`), so returning to the browser
  tab also resumes the agent.
- Before this package, the behavior was intentional. Under the old wording,
  AC-EXECUTORS-IDLE-PARKING-001.5 resumed idle-suspended sessions on focus
  "also … when the general prevent-auto-start-on-open preference is enabled".
  The test `resumes the selected session without a prompt despite the
  preference` (`use-session-resumption.test.ts`) asserts that behavior.
- Classification: an intended product behavior change (fix skill class 3).
  This package already amends the requirements and design:
  - `docs/specs/executors/requirements/idle-runtime-parking.md`:
    AC-EXECUTORS-IDLE-PARKING-001.5 and 001.10.
  - `docs/specs/executors/system-design/idle-runtime-parking.md`: "Message
    and focus recovery".
  - `docs/specs/tasks/requirements/prevent-agent-autostart-on-open.md`:
    "Idle-suspended sessions", state table, and scenario.

Compatibility effect: users with the preference **off** (the default) see no
change. Users with it **on** now see an idle-suspended task open stopped. The
composer hint explains that a message starts the agent. Message-driven resume
(AC-EXECUTORS-IDLE-PARKING-001.4) is unchanged.

## Scope

### In scope

- Gate the idle-suspended branch of `performResumeAction` on
  `preventAutoStart`. This one change covers both open-time and window-focus
  recovery.
- Update the settings help copy so the preference also describes agents
  paused for inactivity, in all seven locales.
- Unit and E2E regression coverage on desktop and phone.

### Out of scope

- Backend enforcement of the preference. Per the tasks requirement's "Out of
  scope", the preference remains a web-client gate. Backend
  `focusTaskSessionAdmitted` is unchanged.
- The idle-suspension policy itself, message-driven resume, and FAILED/stale
  session handling from #4152.
- Office task pages, which do not run `useSessionResumption`.

## Technical approach

In `performResumeAction`, inside the idle-suspended branch, add the
preference check before resuming:

```ts
if (status.is_idle_suspended && !status.is_agent_running) {
  if (preventAutoStart) {
    recordResumeSkipIfStopped(setters, sessionId);
    setters.setResumptionState("idle");
    return false;
  }
  // existing auto_resume_allowed / visibility checks, then resumeWithSilentFallback
}
```

`recordResumeSkipIfStopped` sets `resumeSkippedSessionIds[sessionId]`. A
non-terminal idle-suspended session is in `WAITING_FOR_INPUT`/`IDLE`, so
`shouldShowComposerAgentStartHint` renders the existing hint with no UI change.
A `COMPLETED` idle-suspended session keeps its terminal recovery actions
instead of the composer hint, because sending into a terminal session is
rejected. Turning the preference off does not immediately retry an already
skipped resume: reopening the task or returning focus to its window checks the
session again and can resume it. That matches non-idle behavior.

The preference also gates the early visible-restart focus used for sessions
with an `agent_delivery` recovery block. That path cannot read the status
endpoint first, so it skips the focus when the stored session state can be
idle-suspended (`WAITING_FOR_INPUT`, `IDLE`, or `COMPLETED`). Sessions in any
other state cannot be idle-suspended and keep their restart-priority focus.

## ASCII UI preview

`UI-01: Task chat after opening an idle-suspended task` (preference ON). The
layout is the same on desktop and phone. Only the composer region changes.
The hint already exists (`composer-agent-start-hint`). This fix makes it
appear for this state.

```text
Before (bug)                              After (fix)
+--------------------------------------+  +--------------------------------------+
| ...transcript...                     |  | ...transcript...                     |
| [agent spinner: Starting agent...]   |  |                                      |
+--------------------------------------+  +--------------------------------------+
| [ composer ........................ ]|  | Sending a message will auto-start    |
|                                      |  | the agent.                           |
|                                      |  | [ composer ........................ ]|
+--------------------------------------+  +--------------------------------------+
```

Phone: the same hint renders above the touch composer. No new controls are
added. The hint maps to AC-EXECUTORS-IDLE-PARKING-001.5 and the
prevent-auto-start state-table row for idle-suspended sessions.

## Work orders

| ID | Work order | Depends on |
|---|---|---|
| 01 | [Gate idle-suspended focus resume](task-01-gate-idle-suspended-focus-resume.md) | none |
| 02 | [E2E coverage for idle-suspended open with the preference](task-02-e2e-idle-suspended-prevent-autostart.md) | 01 |

## Dependency order

01 → 02, sequential.

## Verification strategy

- Unit (Task 01): flip the existing "despite the preference" test so that
  no `session.launch` is sent and `setResumeSkipped(SESSION_ID, true)` is
  called. Add a window-focus regression and keep a preference-off test that
  proves the resume still happens.
- E2E (Task 02): desktop and phone. Enable the preference, let the mock
  session idle-suspend, open the task, and assert that no focus launch is
  sent, the state is still `suspended`, and the hint is visible. Then send a
  message and assert a single resume and delivery.
- The existing `session-idle-parking.spec.ts` (preference off) must stay
  green, unchanged.

## Risks

- **Race with a settled live session.** For a session that is live, the
  client sends a `session_focus` launch only to refresh the idle interval. If
  the backend suspends the session between the status read and that launch,
  backend `focusTaskSessionAdmitted` resumes it. The window is narrow, and the
  user had the session open while it was live. It is accepted as out of scope
  because the preference is not server-enforced.
- **Copy change touches seven locales.** Use `pnpm run i18n:zh-hant` for the
  Traditional Chinese pair. `pnpm run i18n:check` gates the result.

## Verification results

Implemented on 2026-11-03.

- Task 01: `performResumeAction` now records the resume-skip marker and returns
  before the idle-suspended resume when the preference is on. The rewritten
  unit test failed on the unfixed source (2 failures) and passes with the fix;
  `pnpm exec vitest run hooks/domains/session/use-session-resumption.test.ts`
  reports 54 passed. Window-focus cases cover preference on and off, and the
  in-flight startup join test runs with the preference off to keep its resume
  assertion.
- Fixup: visible restart priority for an `agent_delivery` recovery block now
  honors the preference for stored states that can be idle-suspended.
  Non-suspendable states keep their early recovery focus, and a live settled
  session still receives one focus. The new restart-priority test failed before
  this change. A completed idle-suspended session is documented as keeping its
  terminal recovery actions rather than the composer hint.
- Task 01: `preventAutoStartAgentOnOpenHelp` and `preventStartShort` describe
  the inactivity-paused case in all seven catalogs, including the reviewed
  Traditional Chinese overrides. The help now states that the agent stays
  stopped until the user starts it or sends a message instead of promising a
  button. `pnpm run i18n:check` passes.
- Task 02: desktop
  `e2e/tests/session/session-idle-parking.spec.ts` and phone
  `e2e/tests/session/mobile-session-idle-parking.spec.ts` pass with the new
  assertions. The desktop spec also proves zero `session_focus` launches while
  the session stays suspended, then one message-driven resume and delivery.
- Static checks: `tsc --noEmit`, targeted ESLint, and `e2e:sleep-ratchet`
  pass.
