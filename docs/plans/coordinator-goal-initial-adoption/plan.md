---
created: 2026-11-22
status: done
requirements:
  - REQ-COORDINATOR-GOALS-001
system_design:
  - ../../specs/coordinator/system-design/goals.md
legacy_specs: []
---

# Fix plan for initial goal editing

## Overview

A manager can lose the first edit to the Milestone field as the Goal section
loads. The repair makes the goal draft ready before the form can accept input.
GitHub shows `@carlosflorencio` as the assignee. [Issue #5425](https://github.com/kdlbs/kandev/issues/5425)

`REQ-COORDINATOR-GOALS-001` covers the empty form (`001.9`) and creation from
the entered name (`001.1`, `001.5`). The system design covers initialization
and save behavior. [Goal requirements](../../specs/coordinator/requirements/goals.md#req-coordinator-goals-001-setting-a-goal),
[Goal UI design](../../specs/coordinator/system-design/goals.md#goal-ui)

## Root cause and evidence

`GoalSection` renders editable controls as soon as the first goal read returns.
`useAdoptedGoalForm` still has a passive effect queued for that response. The
effect sees the first response as a goal change and replaces the draft and
saved snapshot. If an input event arrives in that gap, the replacement erases
the edit. An empty response still takes this branch because the previous goal
ID is `undefined` and the loaded goal ID is `null`.

On `c68b3340d72`, a temporary component probe held the first `GET goal`
response. A `MutationObserver` entered `Ship` when the name input first
appeared. The input ended up empty and **Set goal** was disabled. The same
probe reproduced the failure in the isolated Kandev app on desktop Chromium
and in the Pixel 5 project. Each probe dispatched a DOM input event at the
scheduled moment.

A temporary copy of the hook with `useLayoutEffect` passed both first-edit
component probes and all 15 existing Goal section cases. The implementation
work order records the regression in the suite and runs it against a fresh
browser build.

## Technical approach

Move goal adoption in `useAdoptedGoalForm` to `useLayoutEffect`. React applies
the draft and saved snapshot before paint. Keep the same data dependencies
and the existing rules for goal changes, forced reloads, and done states.
[React layout effect reference](https://react.dev/reference/react/useLayoutEffect)

The repair belongs to the settings editor. `GoalFormFields` also serves the
guided setup draft. The passive effect can queue a whole-form replacement
after the field update. Moving the replacement before paint closes that gap.

## ASCII UI preview

### UI-01 Goal name after the first edit

Open the coordinator settings and select **Goal** with no active goal. This
excerpt shows the state after entering `Ship` as the form appears. The same
inline form serves desktop and phone. On a phone, the existing settings page
scrolls and its section tabs scroll horizontally.

```text
Current                      Repaired
Milestone                    Milestone
[                       ]    [ Ship                  ]
[ Set goal ] disabled        [ Set goal ] enabled
```

`AC-COORDINATOR-GOALS-001.9` governs the form state. The work order includes
the desktop and phone browser checks for the action and request.

## Tests

| Criteria | Evidence |
| --- | --- |
| `AC-COORDINATOR-GOALS-001.1`, `.5`, `.9` | The Goal section component test reproduces the first edit and checks the submitted name. The `chromium` and `mobile-chrome` specs exercise the same flow against the Go-served app. |
| `AC-COORDINATOR-GOALS-001.2`, `.3`, `.10` | Existing Goal section tests cover saving an active goal, criterion toggles, and reload after a conflict. |

## Work orders

- [x] [Task 01. Preserve the first goal edit](task-01-preserve-first-goal-edit.md)

## Verification results

Task 01 is complete. The component and desktop/phone browser regressions failed
before the hook change, then passed after goal adoption moved to a layout effect.
The component suite, changed-file lint, formatting check, and managed desktop
build also passed.

## Risks

- Layout effects run before paint. Keep reconciliation limited to the existing
  in-memory draft and saved snapshot.
- React test helpers can change effect timing. Use the real scheduler during
  the input probe and restore the act flag during test cleanup.
