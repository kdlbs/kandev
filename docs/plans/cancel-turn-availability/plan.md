---
created: 2026-09-15
status: draft
requirements:
  - REQ-UI-CANCEL-TURN-PROGRESS-001
system_design:
  - ../../specs/ui/system-design/cancel-turn-availability.md
legacy_specs: []
---

# Implementation Plan: Cancel control availability

## Overview

Restore cancellation for steering and background-working sessions in task and
Quick Chat composers. Then add a correctly scoped Quick Chat palette fallback.
[Issue #3700](https://github.com/kdlbs/kandev/issues/3700) is assigned to
`carlosflorencio`. Implementation awaits a later explicit request.

## Evidence and root cause

Read-only source trace at `6c5ab15980ab3d39660134073d724addae9c8579` confirms the
reported failure. The issue has no image attachments or comments. No live
instance, private logs, or temporary test was needed for this deterministic trace.

| Input / source | Result |
| --- | --- |
| RUNNING, generating, supports_steering=true, empty queue | `deriveSessionInputMode` returns direct |
| `deriveSessionFlags` | isAgentBusy=false; isWorking=true |
| `useComposerProps` | forwards isAgentBusy; omits isWorking |
| `buildEditorAreaProps` | calls shouldShowCancelAgent(false, null), returning false |
| `SubmitButton` | no cancel control; empty-input send remains visible |
| RUNNING, background | same mismatch, independent of steering |
| STARTING | queue and working are both true; cancel is already offered |
| QuickChatContent | cancellation handler exists, but no session command registration |
| SessionCommands / CommandRegistryProvider | task registration only; sources concatenate without deduplication |

Smallest reproduction: open a structured Quick Chat with a steering-capable
agent, start a long turn, leave the queue and editor empty, and inspect cancel.
Repeat with background activity. The current source deterministically hides it.
Runtime/browser reproduction and permanent regression tests are implementation
work, not claimed as completed investigation evidence.

The gap extends existing AC-UI-CANCEL-TURN-PROGRESS-001.7 and .8. Criteria .9-.12
make availability, clarification compatibility, palette scope, and mobile
reachability explicit. No requirement or design conflict blocks this repair.
STARTING remains cancellable because that is current behavior. The package
preserves connected/disconnected clarification rules and existing progress.

## Scope

### In scope

- Shared composer availability, including Quick Chat configuration conversations.
- Quick Chat cancel-only palette registration and task-cancel suppression.
- Queue helper correction and focused desktop/phone regression coverage.

### Out of scope

- Backend cancellation, provider negotiation, prompt ordering, or flag changes.
- Passthrough terminal gestures and full task command sets in Quick Chat.
- Cancellation-progress persistence and unrelated command palette actions.

## Technical approach

Task 01 passes working state through `use-composer-props.ts` into
`chat-input-container.tsx`, keeps the clarification override, and verifies
`chat-input-toolbar-primitives.tsx` receives independent cancel/send signals.
Use a required prop and update all direct callers/test fixtures; do not silently
fall back to `isAgentBusy`. Add the localized accessible name to the cancel icon.

Task 02 adds `quick-chat-cancel-commands.tsx` beside `quick-chat-content.tsx`.
Reuse `buildSessionCommands` and the existing callback. Suppress task cancellation
while Quick Chat is open. Prove tab switching, closed state, idle/setup/terminal
state, and an underlying running task cannot create an ambiguous cancel target.

No new ADR is needed: this restores the established session-scoped cancellation
contract without changing runtime ownership. The earlier
[cancel progress package](../cancel-turn-progress/plan.md) remains completed;
this package supplements its coverage without rewriting historical results.
The [steering package](../mid-turn-steering/plan.md) retains its delivery contract.

## ASCII UI preview

UI-01: Task or Quick Chat composer, generating with steering, empty editor.
Shared control order on desktop and phone:

```text
Before: [Send now; delivered to the running turn]       [Send]
After:  [Send now; delivered to the running turn] [Cancel][Send]
Pending:[Send now; delivered to the running turn] [Busy  ][Send]
Idle:   [Message input                         ]        [Send]
```

UI-02: Command panel while Quick Chat is open:

```text
Search: cancel
Agent
  Cancel turn -> active Quick Chat session
```

Cancel precedes Send in the existing right-hand action group. Send keeps its
current empty-input disabled state. Busy is the existing disabled spinner, not
new copy. Background work uses ordinary direct-input copy. Non-steering empty
queue-mode input retains the existing cancel-only action group.

Phone uses the existing compact bottom toolbar with 44px hitboxes; desktop uses
28px icons. Both keep the editor above the action row and messages as the scroll
owner. The phone control must remain reachable with the keyboard visible and
without horizontal overflow. No additional sheet is needed for a frequent action.
These control order and reachability requirements are structural; spacing and
labels in the sketch are illustrative and use existing translations.
UI-01 maps to criteria .7-.10 and .12; UI-02 maps to .11.

## Tests

- Task 01: container prop integration must fail before the fix for
  `isWorking=true, isAgentBusy=false`. Cover steering, background, STARTING,
  preparation, idle, missing session, connected and disconnected clarification.
- Existing toolbar tests cover pending state and cancellation callback. Assert
  independent send behavior, a localized accessible name, and session isolation.
- Task 02: component/registry tests assert zero or one cancel entry and the exact
  session_id after tab switches, closing, terminal/setup selection, and pending.

## E2E tests

Task 01 owns `cancel-turn-availability.spec.ts` and
`mobile-cancel-turn-availability.spec.ts` under `apps/web/e2e/tests/chat/`.
Cover task and Quick Chat steering with empty input/queue; click/tap cancel and
observe the selected session settle. Cover background work and preserve direct
submission. Use existing generating-session and parked-background fixtures.
Retain cancellation reload checks and steering/queue delivery checks.

Task 02 owns `quick-chat-cancel-palette.spec.ts` and
`mobile-quick-chat-cancel-palette.spec.ts` in the same directory. Prove exact
session targeting with a running task beneath Quick Chat, active-chat switching,
closed-chat restoration, and no underlying action for idle/setup/terminal tabs.
Use the existing phone command-panel entry and verify touch composer fallback.
All mobile files run in `mobile-chrome`; other files run in `chromium`.
Arm causal transport waits before actions and assert session outcomes afterward.

## Work orders

- [ ] [Task 01: Restore shared cancellation availability](task-01-composer-availability.md)
- [ ] [Task 02: Scope Quick Chat palette cancellation](task-02-quick-chat-palette.md)

Sequential execution; Task 02 depends on Task 01's eligibility wiring.

## Verification results

Planning validation on 2026-09-15:

- `python3 scripts/list-docs.py validate`: passed (272 decisions, 937 specifications).
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check -- docs/specs docs/plans/cancel-turn-availability`: passed.
- `git status --short -- docs/plans/cancel-turn-availability`: all three new
  package files identified and included for commit.

All work-order requirement IDs and design paths resolve. New production/test
paths are explicitly marked new. Product tests have not run; implementation is pending.

## Risks

- Hook working state includes executor preparation; preserve session identity.
- Disconnected clarifications must retain their explicit suppression.
- Multiple palette sources can register identical IDs; priority is insufficient.
- The queue helper currently mistakes cancellation availability for queue mode.
- New mobile test filenames must use the mobile prefix to match the project.

## Documentation impact

Internal specifications and plans only in this turn. Existing cancellation
copy and transport remain unchanged. Public docs do not need speculative
instructions before implementation; implementation should recheck the public
interaction guide if its documented palette behavior needs updating.
