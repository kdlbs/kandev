---
created: 2026-09-04
status: completed
requirements:
  - ../../specs/ui/requirements/transcript-auto-scroll.md
  - ../../specs/ui/requirements/task-prompt-transcript-visibility.md
system_design:
  - ../../specs/ui/system-design/transcript-auto-scroll.md
  - ../../specs/ui/system-design/task-prompt-transcript-visibility.md
legacy_specs: []
---

# Implementation Plan: Transcript Task-switch Continuity

## Overview

Make an incoming task transcript appear at its intended reading position as
soon as cached rows are measurable. Keep that position correct when the latest
message window arrives, and prevent the older-history sentinel from turning a
stale pre-placement intersection into an unnecessary pagination request.

The confirmed failure has two parts. First, cached rows stay visible at the
browser-default top while the latest-window refresh blocks initial placement.
Second, releasing the placement block can replay an intersection recorded at
that temporary top without checking the sentinel's current geometry.

## Scope

### In scope

- Provisional task-entry placement from cached transcript rows.
- Final placement after the latest-window refresh settles.
- Enabled auto-scroll placement at the newest message.
- Disabled auto-scroll restoration from the incoming session's saved position.
- Placement-token ownership through both phases.
- Current-geometry validation before an observed sentinel intersection can
  start or transfer older-history pagination.
- Development diagnostics for placement phase, owner, token, and geometry.
- Focused unit, desktop browser, and mobile browser regression coverage.

### Out of scope

- Unread-divider behavior or its setting.
- The auto-scroll control, persistence format, or same-environment activation
  semantics.
- Backend message ordering, cursor contracts, or page size.
- Explicit transcript navigation and saved-layout behavior except for
  regressions caused by shared scroll ownership.
- Dockview architecture, mobile navigation, layout, or user-facing copy.

## Technical approach

### Split task-entry placement into provisional and final phases

Extend the initial-placement coordinator in
`apps/web/components/task/chat/message-list-native-scroll.ts` with
token-scoped provisional and final completion state. When cached rows exist,
the panel is measurable, and no unread-divider target owns entry, run a
provisional placement without clearing the environment-switch token:

- enabled auto-scroll selects the newest cached message;
- disabled auto-scroll restores the incoming session's saved position.

After the latest-window refresh settles, run the existing ownership decision
against the reconciled rows and conditionally clear the same token. Re-check
the live session, visibility, and competing scroll owners before both writes.
If no cached rows exist, keep the existing loading path and perform only the
final phase.

Keep `useSessionMessages` as the source of cached-history and refresh readiness.
Do not copy history state into a second store or replay the outgoing
transcript's absolute offset.

### Require live geometry before pagination replay

Update `useLazyLoadSentinel` so eligibility retries and stale-view handoffs
invoke `isCurrentGeometryEligible` when a consumer supplies it. A historical
`IntersectionObserver` entry remains a wake-up signal, not proof that the
sentinel is currently inside the preload region. Preserve existing behavior for
consumers that do not supply a current-geometry predicate.

The transcript already owns the scroll-root-aware geometry predicate. Keep the
placement token active through final placement so the sentinel cannot paginate
between the provisional and final phases. A transcript restored at its actual
top must still load older history.

### Add bounded diagnostics

Add development logging for provisional placement, final placement, request
completion, and pagination eligibility. Include the session identifier,
placement token, selected owner, and scroll geometry. Do not log message
content or saved offsets.

## Tests

| Acceptance criteria                              | Evidence                                                                                                                                       |
| ------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| `AC-UI-TRANSCRIPT-AUTO-SCROLL-001.11` and `.14`  | Coordinator tests hold the refresh pending while cached rows are measurable, then prove immediate enabled placement and final reconciliation.  |
| `AC-UI-TRANSCRIPT-AUTO-SCROLL-001.12` and `.14`  | Coordinator and browser tests prove disabled task entry uses the incoming session's saved position before and after refresh.                   |
| `AC-UI-TRANSCRIPT-AUTO-SCROLL-001.13`            | Sentinel tests prove eligibility release and stale-view transfer reject old intersections when current geometry is outside the preload region. |
| `AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.10` | Browser request tracing proves task entry does not request older pages before real upward navigation.                                          |
| `AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.14` | Existing and focused sentinel coverage proves a transcript currently restored at the top still paginates.                                      |

Use TDD. Add the pending-refresh and stale-intersection regressions first and
record their failures before changing production behavior.

## E2E tests

- Extend `apps/web/e2e/tests/chat/auto-scroll-toggle.spec.ts` with a cached
  environment-changing task switch. Hold the return task's latest-window
  `message.list` response, prove the transcript is already at the newest cached
  message, release the response, and prove it remains at the final newest
  message.
- Capture outgoing `message.list` requests and prove no request with an older
  `before` cursor occurs during entry or block release.
- Cover disabled auto-scroll by proving the incoming task's saved reading
  position is used while refresh is held and after it settles.
- Extend `apps/web/e2e/tests/chat/mobile-auto-scroll-toggle.spec.ts` with the
  same cached-entry contract through the current mobile task switcher.
- Reuse the current WebSocket routing helpers. Add a narrowly scoped response
  hold helper only if desktop and mobile need the same control.

Use causal waits for the held response and rendered geometry. Do not replace
the regression with a long eventual-bottom poll.

## Mobile design contract

- **Desktop outcome:** the existing Dockview session panel places cached rows
  immediately and reconciles them after refresh.
- **Mobile entry:** the existing task switcher opens the full-height task Chat
  tab; no new control or navigation surface is introduced.
- **Exemplar:** `TaskLayout` remains the responsive composition owner.
- **Scroll ownership:** the transcript remains the only vertical scroll region;
  the header and composer keep their current safe-area behavior.
- **Shared behavior:** cached placement, refresh reconciliation, auto-scroll
  preference, and sentinel geometry checks use the same coordinator rules.
  Mobile does not consume Dockview's environment-switch token.
- **Mobile proof:** mobile-chrome verifies immediate cached placement, final
  reconciliation, disabled position restoration, and no document overflow.

## Work orders

- [x] [Task 01: Stabilize task-switch transcript entry](task-01-stabilize-task-switch-transcript.md)

The work order is sequential because placement ownership and pagination
eligibility share one transcript lifecycle.

## Results

Implemented provisional cached-history placement and final refresh
reconciliation for environment-changing task switches. Enabled transcripts now
open at the newest cached message; disabled transcripts restore the incoming
session's saved reader position. The placement token remains active until the
final phase, and an unread-divider target keeps priority.

Eligibility retries and stale-view handoffs now require the sentinel's current
geometry when the consumer supplies that predicate. The desktop browser
regression holds the latest-window response and proves both placement modes plus
the absence of older-page requests. The mobile regression proves the same user
value through the task-switcher sheet and checks document width.

Verification passed: 142 focused unit tests, targeted ESLint, TypeScript
typecheck, one Chromium E2E, one mobile-chrome E2E, specification lint, Prettier,
and diff checks.

## Risks

- Clearing the placement token after provisional placement can reopen the
  original pagination race. Only final placement can clear it.
- A provisional write must not override unread-divider or explicit-navigation
  ownership.
- Cached row heights can change after refresh. Final placement must recompute
  against current DOM geometry instead of preserving a stale pixel delta.
- Applying the geometry predicate to every sentinel consumer could change
  Prompt History behavior. Keep it optional and test the supplied-predicate
  path independently.
- Browser tests can pass while still showing a top flash if they only assert an
  eventual position. Hold the refresh response and assert before release.
