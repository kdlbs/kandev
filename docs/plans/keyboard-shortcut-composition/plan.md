---
created: 2026-10-13
status: completed
requirements:
  - REQ-UI-KEYBOARD-SHORTCUT-COMPOSITION-001
system_design:
  - ../../specs/ui/system-design/keyboard-shortcut-composition.md
legacy_specs: []
---

# Implementation Plan: Keyboard Shortcut Composition

## Overview

Deliver one sequential work order: prove composition rejection at both reusable
hook paths and the actual Task Create boundary, add the private hook predicate,
then verify ordinary shortcuts and the existing desktop/phone surfaces. This is
The private hook guard now rejects composition in both supported event paths;
ordinary shortcut activation remains available through the existing task submit
wiring.

## Confirmed defect and limits

ROOT qualified the actual `useTaskCreateDialogSetup.handleKeyDown` with the real
`useKeyboardShortcutHandler` on authoritative main
`f932f360bb30b44848e6cf34a2ffcf765bc5083b`. Matching Ctrl+Enter with either
`isComposing=true/keyCode13` or `isComposing=false/keyCode229` invoked the submit
callback once and prevented the DOM event. Two causal RED cases and three
passing controls covered ordinary Ctrl+Enter, plain Enter, and compatibility
blocking. Evidence: `/tmp/kandev-root-task-create-ime-discovery-20261013-pdmmywpk/`
(`qualified-proof.json`, `proof.log`, `startup.json`); ROOT owns that evidence.
No actual task/API mutation or persisted creation was proved. Do not repeat
initial qualification or modify ROOT's private candidate.

The root cause is absent event admission in the reusable hooks; their matcher
checks only keys/modifiers. No timing hypothesis is needed.

## Scope

Own the UI requirement/design pair, hook admission, focused hook and actual
consumer regression tests, desktop/mobile E2E, and one small task-creation docs
clarification. Exclusions are those in the owning requirement. The matcher,
direct event consumers, renderer behavior, and existing UI geometry are unchanged.

## Technical approach

Use the private predicate and two early returns described in the
[system design](../../specs/ui/system-design/keyboard-shortcut-composition.md).
Do not change `lib/keyboard/utils.ts` or Task Create production code. Native
events and React `nativeEvent` are the two supported shapes. Neither rejection
consumes the event. Unsupported custom event objects are outside these typed
hook contracts; do not add a new fallback API.

## ASCII UI preview

UI-01: Desktop Create Task. Entry: New Task. State: eligible creation with a
prompt and manual title enabled, using the existing bounded modal.

```text
+-------------------------------------------------------+
| Create Task                                           |
| Source: [Repo / Remote / None] [branch/options]         |
| Title:  [Entered title                              ] |
| Prompt: [Composed text                              ] |
|         [                                           ] |
| [Workspace / Workflow / Step] [Agent / Executor]       |
| Existing advanced settings                            |
|-------------------------------------------------------|
|                         [Cancel] [Start Task] [v]     |
+-------------------------------------------------------+
```

UI-02: Phone Create Task. Entry: mobile FAB or navigation New Task. State:
the same eligible creation, using the shipped full-height surface.

```text
+---------------------------------+
| Create Task                     |
| Source: [Repo / Remote / None]   |
| [branch/options]                |
| Title:  [Entered title        ] |
| Prompt: [Composed text        ] |
|         [                     ] |
| [Workspace / Workflow / Step]   |
| [Agent / Executor]              |
| Existing advanced settings      |
|---------------------------------|
| [            Cancel           ] |
| [          Start Task         ] |
| [          Plan Mode          ] |
| [         Create Only         ] |
+---------------------------------+
```

Both previews are compact source-grounded excerpts, not a layout redesign or
pixel specification. Title visibility and footer alternatives retain existing
eligibility rules; the desktop arrow exposes existing secondary actions, while
the phone exposes its existing visible alternatives. Header/footer remain
outside the existing form-body scroll region. Preserve shipped safe-area and
touch behavior without new geometry assertions unrelated to this defect.

For both views, composition leaves the input text and dialog in place without
consuming the event. A later ordinary chord or the existing visible action
submits through the same eligibility checks. The phone touch control remains
usable. These annotations map to `AC-UI-KEYBOARD-SHORTCUT-COMPOSITION-001.4`
and `001.5`; compare the focused desktop/phone E2E captures with UI-01/UI-02.
Labels illustrate existing localized copy; no new product copy is introduced.

## Tests

| Evidence | Acceptance criteria |
| --- | --- |
| `hooks/use-keyboard-shortcut.test.ts`: native and React event matrices | 001.1, 001.2, 001.3 |
| `components/task-create-dialog-composition.test.tsx`: real setup/hook/submit, captured create boundary | 001.2, 001.4 |
| Existing hook, setup, submit, selectors, dialog suites | 001.3, 001.4 |

All criterion suffixes above refer to `AC-UI-KEYBOARD-SHORTCUT-COMPOSITION-`.
RED must independently fail for each composition signal at each hook path and
the consumer boundary, with passing ordinary controls before production edits.
Record actual assertion failures, not import/setup failures.

## E2E tests

`e2e/tests/task/task-create-composition.spec.ts` (`chromium`) and
`mobile-task-create-composition.spec.ts` (`mobile-chrome`) cover 001.2, 001.4,
and 001.5 through the real rendered input and disposable creation transport.
Share mechanics in `task-create-composition-helpers.ts`. Desktop and phone
exercise both signals and a later ordinary chord; phone additionally exercises
visible touch submission. Compare focused screenshots with UI-01/UI-02.
These are synthetic composition flags, not physical OS IME coverage.
Run one desktop causal rendered RED on unchanged production source. The shared
predicate already has independent native, React, and real-consumer RED proof;
do not repeat the same RED on mobile. Final GREEN must run desktop and phone,
including the phone touch control. An additional mobile diagnostic is justified
only by an observed distinct viewport failure. All runs are serial with one
bounded worker per command and fresh managed builds for the applicable phase.

## Work orders

- [x] [Task 01: Guard shortcut composition and prove Task Create](task-01-guard-composition.md)

## Documentation impact

Added a short paragraph under Create a task in
`docs/public/tasks-and-workflows.md` explaining composition confirmation and
subsequent explicit shortcut activation. This is an existing how-to page; no
new page, published documentation image, labels, or UI copy were introduced.

## Verification results

- Frozen workspace install completed once with Node 24.18.0 and pnpm 9.15.9.
- Independent native and React RED runs each failed four composition cases with
  passing ordinary controls. The real setup/submit/create boundary RED failed
  two composition cases; ordinary and compatibility controls passed.
- Desktop rendered RED observed one actual create request per rejected signal
  instead of zero. Browser stimuli were isolated from ProseMirror's independent
  Enter fallback and Ctrl+Enter hard-break keymap. Those renderer paths are
  unchanged; mounted regressions retain Ctrl/Meta and composing/code13 coverage.
- Final affected Vitest command: six suites, 103 tests passed with one worker.
  After adding the fixture's required `defaultStepId` and `steps` props, its
  focused four-test suite and targeted ESLint passed again.
- Desktop Chromium: two tests passed. Phone mobile-chrome: three tests passed,
  including visible touch submission. Both prove exactly one task persisted
  after ordinary activation and reload. Two current viewport captures were
  inspected and compressed. Phone final title verification uses the existing
  mobile task picker, rather than the desktop title selector.
- Scoped final ESLint passed. `pnpm run typecheck` passed with an 8192 MiB Node
  heap after the default 4 GiB run exhausted its heap and an 8 GiB run identified
  the missing required test props. No type suppression or source API change.
- Ratchets passed against actual staged new paths: i18n (one modified production
  file clean; tests excluded by its existing rules) and E2E waits (three added
  files clean).
- Catalog validation passed (381 decisions, 1587 specifications); both new UI
  paths were discovered; full specification lint and public-doc validation
  (48 pages) passed. No unchanged validator unit suites were run.
- Actual changed-path documentation-reference preflight passed as `covered`,
  with `ok=true`, no errors, and the owning requirement/design mapping accepted
  for all 11 changed files. Final working/index diff checks passed.

Managed production builds were fresh after the guard change. Test-only browser
diagnostic corrections reused that current build; final phone and desktop runs
remained serial and used one worker. Synthetic composition evidence does not
establish physical OS IME or Android hardware-keyboard rendering behavior.

## Risks

- Existing setup tests mock the shortcut hook; the new mounted test must keep
  it and submit wiring real or it cannot regress the defect.
- React and native hooks differ on already-prevented ordinary events; avoid
  unrequested behavioral harmonization.
- Legacy 229 requires inspecting the underlying DOM event independently.
- Browser synthesis cannot establish real OS IME behavior.
