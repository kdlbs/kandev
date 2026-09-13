---
created: 2026-09-11
status: complete
requirements:
  - REQ-UI-AGENT-LAUNCH-PROMPT-COMPOSER-001
system_design:
  - ../../specs/ui/system-design/agent-launch-prompt-composer.md
legacy_specs: []
---

# Implementation Plan: Blank handoff context

## Overview

Start every handoff with Blank context and an empty prompt. Generate summaries
only after explicit context selection. One sequential work order owns the small
correction, regression coverage, and public instructions.

## Confirmed root cause and reproduction

On 2026-09-11, a read-only Node 24 invocation imported the real
`apps/web/components/task/handoff-types.ts` and called
`buildHandoffInitialState({sourceSessionId: "session-a", targetProfileId: "profile-b"})`.
It returned `{selectedProfileId: "profile-b", contextValue: "summarize:session-a"}`,
which conflicts with the requested Blank default.

Source trace: `NewSessionForm` initializes from that helper, then
`useHandoffAutoSummarize` calls `handleContextChange` in a mount effect.
`useSessionContextChange` calls `summarize` for the prefixed value. The summary
hook fetches the transcript and can invoke the summary utility immediately.
The unit test `buildHandoffInitialState selects target profile and summarize context`,
the component test `writes the handoff summary into the fresh-open dialog prompt`,
and desktop `session-handoff.spec.ts` all encode this automatic behavior.

Smallest UI reproduction: open a completed task session, choose Handoff and a
compatible target profile, then observe summary selection/loading without any
context action. A configured utility and nonempty transcript allow generation.
The supplied screenshot shows Blank at capture time; it does not establish the
preceding transition. The helper execution and source trace confirm the defect.
No browser or live-instance reproduction was run for this design-only package.

Classification: intended product behavior changes. The existing requirement's
handoff scenario expressly preserved automatic summaries. The user has now
replaced that expectation; AC-001.9 through AC-001.11 specify the correction.

## Scope

### In scope

- Blank initialization, no summary side effect on opening, and fresh-open reset.
- Explicit summary selection, compatible target preservation, and typed launch.
- Focused desktop/mobile regressions and updated public handoff instructions.

### Out of scope

- Backend APIs, utility configuration, summary content quality, and persistence.
- Profile precedence, environment reuse, subtask and Quick Chat behavior.
- Dialog redesign, new copy/translation keys, or asynchronous summary cancellation.

## Technical approach

In `handoff-types.ts`, return `blank` from `buildHandoffInitialState` while
retaining the target profile. Remove `summarizeContextValue` if the confirmed
reference search still shows only its helper/test consumers.
In `new-session-dialog.tsx`, delete `useHandoffAutoSummarize` and its call;
retain the keyed fresh-open reset and existing profile resolver. Keep
`useSessionContextChange` attached to user selection in `ContextSelect`.
Do not replace the effect with an effect that clears the prompt on mount.

Update both handoff descriptions in `docs/public/sessions-and-review.md`
(a how-to guide): opening selects Blank and the target profile; the user may
type or explicitly choose a session summary before Start Agent.

The completed [composer package](../agent-launch-prompt-composer/plan.md) and its
two done work orders were inventoried. Preserve their historical results;
their automatic-summary preservation scope is superseded by this package.
There is no pending companion work to merge. The ADR index contains no separate
handoff-context decision requiring amendment. This local default correction
does not require a new architectural boundary or ADR.

## ASCII UI preview

UI-01: Handoff context on fresh opening. Desktop enters from session-tab actions;
phone enters from the mobile sessions action menu. Both use the existing dialog.

```text
Before (source-confirmed):             After (desktop and phone):
Hand off to <target>                  Hand off to <target>
Environment / Agent Profile           Environment / Agent Profile
Context [Summarizing... v]            Context [Blank          v]
Prompt  [generated summary]           Prompt  [empty           ]
                                     [Cancel] [Start Agent: disabled]

After explicit summary choice:
Context [Summarizing... v] -> [Summarize <chosen session> v]
Prompt  [summary text, editable]
[Cancel] [Start Agent]
```

The Before prompt represents successful completion of the automatic request.
Blank, empty prompt, target preservation, and explicit selection are required
structure (AC-001.6, .9-.11); spacing is illustrative. No geometry changes are
planned. Phone retains the same vertical form and existing footer/scroll
behavior, with touch selection through its current session entry. The focused
phone test checks reachable actions, containment, and no horizontal overflow.
Manual summary failure retains existing Blank recovery and error feedback.

## Tests

All criteria below belong to REQ-UI-AGENT-LAUNCH-PROMPT-COMPOSER-001.

| Criteria | File under apps/web | Planned evidence |
| --- | --- | --- |
| .9, .11 | components/task/handoff-types.test.ts | `buildHandoffInitialState selects target profile and blank context` fails before correction |
| .6, .9-.11 | components/task/new-session-dialog.test.tsx | `opens handoff with Blank context without summarizing`; empty prompt, disabled launch, target retained; close/reopen and source/target changes reset; rerender/session hydration does not summarize or erase typed draft |
| .4, .10 | components/task/new-session-dialog.test.tsx | Explicit source and other-session summary choices invoke only the chosen session; copy and Blank remain manual |
| .4-.6 | components/task/new-session-form-actions.test.ts; components/task/session-context-summary.test.ts | Retain manual summary result/failure and current prompt submission coverage |

## E2E tests

Update existing `apps/web/e2e/tests/session/session-handoff.spec.ts` in `chromium`
and `mobile-handoff.spec.ts` in `mobile-chrome`. Arm summary utility request
counting before opening. Assert Blank, empty prompt, correct target, disabled
Start Agent, and no summary execution before any context selection. Type and
launch without a summary. Separately reopen after a summary choice and prove
Blank reset; explicitly select the source summary and prove one request plus
editable summary text. Mobile uses touch selection and proves launch outcome.
These scenarios cover AC-001.6, .7, and .9-.11.

Count only `builtin-summarize-session` requests, so unrelated utility traffic
does not fail the test. Use the existing causal-wait helpers for selected-summary
responses and documented negative assertions. Preserve existing unhealthy-profile
coverage, which remains a focused compatibility check.

## Work orders

- [done] [Task 01: Make handoff context explicit](task-01-explicit-context.md)

## Verification results

Implementation evidence is complete. The helper and mount-effect causes were
removed, explicit context actions remain manual, and the public handoff guide
now describes the delivered Blank default.

- TDD RED: the helper expectation failed against `summarize:session-a`; the
  unchanged automatic-summary dialog test then failed after the helper was
  corrected, confirming both causes before the final production change.
- `pnpm install --frozen-lockfile`: passed from `apps`.
- Targeted Vitest suite: 5 files, 38 tests passed for the implementation;
  the post-review fixup suite passed with 5 files and 39 tests.
- `pnpm run typecheck`: passed.
- Targeted ESLint: the implementation passed with 0 errors and 2 duplicate-
  string warnings in `new-session-dialog.test.tsx`; the final fixup passed
  with 0 errors and 0 warnings after deduplicating those test values.
- `pnpm run i18n:ratchet`: passed; 0 new violations and guard allowlist intact.
- Desktop managed E2E: 2 tests passed, including handoff and unhealthy-profile
  coverage.
- Mobile managed E2E: 1 test passed on `mobile-chrome`, including touch launch
  and the no-horizontal-overflow assertion.
- Public-doc validation: 62 tests passed and 46 published pages validated.
- Specification tests: 36 passed; all specification files passed lint.
- `git diff --check`: passed.
- The managed E2E runners completed isolated cleanup without repository or
  instance artifacts. The tracked work-order package is the only plan output.
- Owning UI index: 14,745 bytes, within the configured 16 KiB limit.

## Risks

- Changing only the displayed default leaves the mount action in place; remove both.
- Existing automatic-summary tests must become explicit-choice tests, not be dropped.
- Keep compatible target selection independent from Blank context initialization.
- Public docs must ship with implementation, not claim pending behavior is live.
