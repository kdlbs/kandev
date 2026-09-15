---
created: 2026-09-15
status: complete
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
`carlosflorencio`. Implementation was explicitly requested on 2026-09-15.

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
Passthrough keeps its Escape callback as composer dismissal and explicitly hides
the agent cancel control because that surface has no agent-cancel transport.

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
  independent send behavior, a localized accessible name, session isolation, and
  passthrough dismissal without agent cancellation.
- Task 02: mount `SessionCommands` and the Quick Chat source together and assert
  zero or one cancel entry and the exact callback/session target after tab
  switches, close/unmount, terminal/setup/idle/disconnected selection, backend
  pending rejection, and failed-request cleanup.

## E2E tests

Task 01 owns `cancel-turn-availability.spec.ts` and
`mobile-cancel-turn-availability.spec.ts` under `apps/web/e2e/tests/chat/`.
Cover task steering and background work with empty input/queue; click/tap cancel
and observe the selected session settle. Preserve direct submission. Use
existing generating-session and parked-background fixtures. Retain cancellation
reload checks and steering/queue delivery checks.

Task 02 owns `quick-chat-cancel-palette.spec.ts` and
`mobile-quick-chat-cancel-palette.spec.ts` in the same directory. The desktop
and phone suites now exercise negotiated Quick Chat steering with generating
activity, an empty editor and queue, then click/tap the visible composer cancel
control and wait for the exact Quick Chat session to settle. Desktop also covers
detached background work. The palette cases prove exact session targeting with
a running task beneath Quick Chat, and retain the existing phone palette entry.
All mobile files run in `mobile-chrome`; other files run in `chromium`.
Arm causal transport waits before actions and assert session outcomes afterward.

## Work orders

- [x] [Task 01: Restore shared cancellation availability](task-01-composer-availability.md) - complete
- [x] [Task 02: Scope Quick Chat palette cancellation](task-02-quick-chat-palette.md) - complete

Sequential execution; Task 02 depends on Task 01's eligibility wiring.

## Verification results

Planning validation on 2026-09-15:

- `python3 scripts/list-docs.py validate`: passed (272 decisions, 937 specifications).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check -- docs/specs docs/plans/cancel-turn-availability`: passed.

Implementation validation on 2026-09-15:

- RED regressions were observed before the production changes: direct working
  input produced `canCancelAgent=false`, the composer hook omitted `isWorking`,
  the toolbar had no cancel accessible name, and the task command remained
  registered while Quick Chat was open. The later review found that the
  underlying-command claim had only been covered by a builder test; no RED
  integration result is claimed for that earlier test.
- Review remediation added a passthrough callback regression and an actual
  registry integration matrix with both command sources mounted. It covers
  active A-to-B switching, close/unmount restoration, setup/terminal/idle and
  disconnected clarification suppression, backend-pending rejection, and
  failed-request cleanup.
- Affected frontend unit suite: 9 files, 128 tests passed, including the
  review remediation coverage for the changed composer, Quick Chat registry,
  spinner semantics, and eligibility paths.
- `pnpm run build:vite`: passed. Existing Vite chunk-size and dynamic-import
  warnings remain informational.
- `pnpm run typecheck`: passed.
- Targeted ESLint for all changed frontend and E2E files: passed.
- Targeted E2E sleep lint for changed E2E files: passed.
- `pnpm run i18n:check`: passed; the existing 200 orphan catalog warning remains.
- Chromium cancellation, queue, steering, and cancellation-progress coverage:
  28 tests passed.
- Chromium Quick Chat composer and palette coverage: 3 tests passed, including
  negotiated steering, detached background work, empty editor/queue evidence,
  exact Quick Chat settle, and underlying-task preservation.
- Mobile cancellation, reload progress, and Quick Chat composer/palette
  coverage: 4 tests passed, including touch-only composer cancellation with a
  44px control and exact Quick Chat settle.
- `python3 scripts/list-docs.py validate`: passed (272 decisions, 937 specifications).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.

Post-remediation managed E2E validation for PR #3705:

- Chromium: 5 tests passed, covering task steering/background cancellation and
  Quick Chat composer cancellation, background cancellation, and preservation
  of the underlying task turn.
- Mobile Chrome: 3 tests passed, covering task touch cancellation, Quick Chat
  touch composer cancellation, and the Quick Chat palette command.

Review fixup and delivery validation for PR #3705:

- PR [#3705](https://github.com/kdlbs/kandev/pull/3705) is open on
  `feature/investigate-issue-37-922` and closes issue #3700.
- Remediation commit `41882f3abe0011cf3112a014145496a760bb9cff` addresses all
  six inline review threads. Final test cleanup commit
  `96ff68d3f5b35704065e3aa43fefcf383f8a4e92` adds the enabled-state assertion
  for the underlying task cancel control.
- The required 15-minute post-creation wait was completed. Review fixup
  replies were added and all six threads are resolved on the final head.
- Fresh active-composer desktop and Pixel 5 screenshots are published from
  media commit `125ef91d16b292d103f60fba714c17929403df35`.
- The final desktop Quick Chat suite passed all 3 tests after the additional
  assertion. The local worktree is clean and the final branch head is pushed.
- Authenticated connector evidence reports no failed workflow runs on the
  final head. Backend Tests and E2E Tests remain queued or pending, while
  Frontend Tests and Preview Environment remain in progress. The managed
  `pr-await` check could not complete its final rollup because GitHub's status
  policy API rate limit was reached; remote CI is therefore recorded as
  pending rather than green.

The repo-wide `pnpm run lint:e2e-sleeps` audit still reports unrelated baseline
errors in other files, including missing rule definitions and existing
unsanctioned sleeps. The changed E2E files pass the same configuration in the
targeted check above.

## Risks

- Hook working state includes executor preparation; preserve session identity.
- Disconnected clarifications must retain their explicit suppression.
- Multiple palette sources can register identical IDs; priority is insufficient.
- Queue waits must use the explicit input-mode projection rather than visual
  busy state, because cancellation is also available during direct input.
- New mobile test filenames must use the mobile prefix to match the project.

## Documentation impact

Internal specifications and plans only in this turn. Existing cancellation
copy and transport remain unchanged. Public docs do not need speculative
instructions before implementation; implementation should recheck the public
interaction guide if its documented palette behavior needs updating.
