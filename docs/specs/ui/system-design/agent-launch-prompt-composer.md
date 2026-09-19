---
status: current
system: ui
created: 2026-09-11
updated: 2026-09-11
owners:
  - kandev
requirements:
  - REQ-UI-AGENT-LAUNCH-PROMPT-COMPOSER-001
---

# Agent launch prompt composer

## Boundary

This design extends the existing UI-owned launch-composer interaction contract.
It owns prompt initialization and user context selection in New Agent and handoff.
Task environment reuse and agent profile eligibility remain owned by their
[task](../../tasks/system-design/additional-session-workspace-reuse.md) and
[agent](../../agents/system-design/profile-recent-use.md) contracts.

## Components

`NewSessionDialog` in `apps/web/components/task/new-session-dialog.tsx` renders
`NewSessionForm` keyed by task, open state, and handoff source/target identity.
The form renders shared `TaskFormInputs` in session mode and reads its current
prompt and attachments through `TaskFormInputsHandle`.

`buildHandoffInitialState` in `handoff-types.ts` returns the target profile and
`contextValue: "blank"`. The source session remains part of `HandoffPreset` and
the form identity; it does not authorize summary generation.
`useSessionProfileSelection` independently preserves the existing compatible
handoff target and fallback behavior.

## Context lifecycle

The form initializes Blank and an empty composer for every fresh opening.
Keep the keyed reset on closing/reopening or changing handoff identity.
Ordinary rerenders and session/profile data hydration preserve the current draft.

Remove `useHandoffAutoSummarize` and its invocation. Mounting the form must not
call `useSessionContextChange`'s returned action. Only `ContextSelect` user
selection invokes that action:

- `blank` clears the composer and prompt-presence state.
- `copy_prompt` inserts the existing initial prompt.
- `summarize:<sessionId>` invokes `useSummarizeSession` for that selected session.

The summary hook reads the selected transcript and calls the existing
`builtin-summarize-session` utility. Opening the form may load session options,
but must not request a transcript for automatic summary generation or execute
the summary utility. There is no stored context preference or migration.

## Composer, launch, and failure behavior

Retain `useSessionPromptController` enhancement delivery and recovery.
`useSessionLaunchSubmit` reads the current shared handle and uses the existing
launch service. Empty-prompt, busy, attachment-upload, and compatible-profile
guards remain in force. Manual summary loading and
`applySummarizeSessionResult` success/failure behavior remain in force, including
Blank recovery and the existing error toast on failure. Broader asynchronous
summary cancellation or stale-result handling is outside this change.

## Responsive behavior

Desktop session actions and phone session actions open the same `NewSessionDialog`.
The nearest shipped phone exemplar is `mobile-handoff.spec.ts`, using
`SessionPage.openMobileHandoffDialog` through the mobile sessions controls.
This correction changes shared initialization only. It retains the existing
dialog, control order, scroll behavior, and touch presentation. Explicit summary
selection and typed-prompt launch receive desktop and Pixel 5 coverage.
Reuse the existing localized Blank and summary labels.

## Requirement mapping and evidence

| Acceptance criteria | Design boundary | Evidence |
| --- | --- | --- |
| AC-UI-AGENT-LAUNCH-PROMPT-COMPOSER-001.1 through .5 | Shared composer and launch | Existing composer and action tests |
| AC-UI-AGENT-LAUNCH-PROMPT-COMPOSER-001.6 through .8 | Guards and responsive launch | Dialog tests and desktop/mobile session E2E |
| AC-UI-AGENT-LAUNCH-PROMPT-COMPOSER-001.9 | Fresh-open initialization | Helper and dialog regressions, reopen E2E |
| AC-UI-AGENT-LAUNCH-PROMPT-COMPOSER-001.10 | Explicit context action | Summary spy/request counting and manual selection |
| AC-UI-AGENT-LAUNCH-PROMPT-COMPOSER-001.11 | Independent profile selection | Target preservation and typed-prompt launch on both viewports |

## Implementation Plans

[Blank handoff context](../../../plans/handoff-blank-context/plan.md) records the
completed correction and its implementation evidence.
