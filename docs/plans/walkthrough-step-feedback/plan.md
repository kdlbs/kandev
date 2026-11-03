---
created: 2026-10-22
status: implemented
requirements:
  - REQ-UI-WALKTHROUGH-STEP-FEEDBACK-001
  - REQ-UI-WALKTHROUGH-STEP-FEEDBACK-002
  - REQ-UI-WALKTHROUGH-STEP-FEEDBACK-003
system_design:
  - ../../specs/ui/system-design/walkthrough-step-feedback.md
legacy_specs: []
---

# Implementation Plan: Walkthrough Step Feedback

## Overview

Show pending and submitted comments beneath their walkthrough step. Open the
composer only after Comment is activated. The user explicitly chose to retain
both pending and sent comments.

Implement retention and identity first. Then connect the step UI, localized
controls, and rendered checks. Both work orders run sequentially in the primary
conversation. The user authorized implementation on 2026-10-22, including all three UX recommendations.

UI owns the existing feedback interaction contract. This package extends that
owner without moving task, session, or delivery authority.

## Scope

### In scope

- Revision-safe matching and retained submitted history within current storage.
- Accepted delivery cleanup for Run, ordinary chat, and passthrough chat.
- On-demand composer, real cancellation, and a step-local comment list.
- Per-step drafts, comment counts, and a shared Kandev step picker.
- Desktop/phone interaction checks, translations, and user-guide updates.

### Out of scope

- Server-backed discussion, cross-session history, replies, and resolution.
- Recovery of comments already deleted by earlier software.
- Extra keyboard shortcuts and server-persisted unsaved drafts.
- Global style changes, agent prompt changes, and network idempotency redesign.

## Technical approach

Add a revision snapshot to `WalkthroughComment`. Add a pure selector in proposed
`apps/web/lib/walkthrough-comments.ts`. Keep comment history in the existing
`byId` and `bySession` records. Retain only walkthrough records when
`markCommentsSent` runs. Ensure retained sent rows cannot enter pending selectors.

Normal chat and passthrough chat currently delete walkthrough context after
success. Acknowledge the exact captured IDs instead. Explicit removal stays a
removal operation. Test asynchronous completion while another comment is added.

Extract a small feedback component from `WalkthroughStepInner`. Keep the
explanation and comments in one scroll owner, with disclosure and navigation
outside it. Reuse CommentForm without changing other consumers' behavior.

The same walkthrough ID survives replacement. New feedback must capture
`updated_at`. Legacy records require an exact snapshot match. Ambiguous rows
remain pending context but do not appear under a guessed step.

## UI preview

The task requests Kandev rich output instead of ASCII. These labelled views
define the same structural preview in text, with a rendered preview in the task.
Example comments and labels are illustrative. Ordering and scroll ownership
are requirements.

| View | Header | Single scrolling body | Fixed lower region |
| --- | --- | --- | --- |
| UI-01: Desktop, reading | Walkthrough, step picker with counts, Close | Title, file anchor, explanation, Pending comment, Submitted comment | Comment, Previous, Next |
| UI-02: Desktop, composing | Same header | Same explanation and comments | Input, Cancel / Add / Run, Previous / Next |
| UI-03: Phone, reading | Existing phone panel header | Title, file anchor, explanation, comments in one column | Comment and navigation, with safe-area clearance |
| UI-04: Phone, composing | Phone header with shared Kandev step picker | Body shrinks as input opens | Input and actions remain above navigation and keyboard |

UI-01/03 omit the comments heading when there are no comments. UI-02/04 show
disabled Add/Run for blank input. Missing session context keeps entered text
visible and reports an error. Run failure leaves one pending row and an error
notification. Cancel restores Comment focus. Step selection closes the composer but preserves its draft. Cancel also preserves drafts.

Entry point: the current walkthrough launcher. The nearest shipped mobile
surface is `WalkthroughFloatingWindow`. Reuse the fixed-controls/scroll-body
pattern from `MobilePickerSheet`, without adding a second overlay. The existing
640px presentation boundary stays. Phone sizing applies below 768px and for
coarse pointers. All new ordinary desktop buttons are 28px. Phone/coarse targets
are at least 44px. Use dynamic height, safe-area clearance, and contained scroll.

These views cover AC-UI-WALKTHROUGH-STEP-FEEDBACK-001.1 and
AC-UI-WALKTHROUGH-STEP-FEEDBACK-002.1 through .5. Component tests check behavior.
Playwright checks the actual desktop and phone geometry.

## Tests

Requirement 003 is covered by draft-state unit tests and desktop/phone E2E for
step selection, count updates, draft restoration, and context isolation.

| Acceptance criteria | Evidence to add or extend |
| --- | --- |
| 001.1, 001.2 | `lib/walkthrough-comments.test.ts`: correct step, session, task, repository, revision, legacy ambiguity, ordered rows |
| 001.3, 001.5 | `lib/state/slices/comments/walkthrough-history.test.ts`: retention, mixed-source acknowledgement, hydration, cleanup, re-enqueue rejection |
| 001.3, 001.4 | Existing Run, pending selector, chat submission, and passthrough tests: direct/queued success, rejection, exact acknowledgement IDs, no second send |
| 002.1 through 002.3, 002.5 | `components/diff/walkthrough-step-feedback.test.tsx`: collapsed state, focus, Escape, successful save, missing context, step replacement |
| 002.3 | `components/diff/comment-form.test.tsx`: additive save-result behavior and unchanged void callbacks |

Criteria in this table use the prefix `AC-UI-WALKTHROUGH-STEP-FEEDBACK-`.
The work orders contain exact commands. New test paths above are proposed files.

## E2E tests

Extend `e2e/tests/review/walkthrough.spec.ts` in `chromium` and
`e2e/tests/review/mobile-walkthrough.spec.ts` in `mobile-chrome`.

1. Open a two-step walkthrough. Assert no composer. Add a comment to step one,
   move to step two, and return. Only step one shows it. Covers 001.1, 001.2,
   002.1, 002.2, and 002.3.
2. Submit through Run. Verify retained Submitted rows, updated counts, and reload
   persistence. Covers 001.3, 001.5, and 002.3. Queued delivery, rejection/retry,
   concurrent Run, and chat-context acknowledgement are covered at the existing
   hook/provider boundaries instead of duplicating transport fakes in Playwright.
3. Replace the walkthrough with the same ID and a new revision. Verify no
   old-step comments or counts remain. Covers 001.2.
4. Verify fixed navigation, long drafts, touch targets, focus return, safe-area
   clearance, and no document overflow. Test 639, 640, 767, and 768px widths
   with a fine pointer, plus the configured Pixel 5. Covers 002.2, 002.4,
   002.5, and 003.1 through .3.

Arm network or WS waits before actions. Do not use arbitrary sleeps. Capture
desktop and phone screenshots. Check keyboard-constrained phone height and
document any real software-keyboard behavior that automation cannot verify.

## Work orders

- [x] [Task 01: Retain and match walkthrough feedback](task-01-retain-feedback.md)
- [x] [Task 02: Display feedback and disclose the composer](task-02-step-feedback-ui.md)

Task 02 depends on Task 01. No delegation is authorized.

## Verification results

Design validation on 2026-10-22:

- `python3 scripts/list-docs.py validate`: passed, 384 decisions and 1618 specifications.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `git diff --check`: passed. All five new package files are present and untracked.
- Relative Markdown links across the package: passed.
- `.github/scripts/pr-docs.cjs` `validateCoverage`: passed for both work orders,
  using the current artifacts and a prospective walkthrough source change.
  This checks linked artifact coverage, not a published PR or production code.

Final implementation validation:

- Combined targeted Vitest: 12 files, 106 tests passed. After adding two
  submission concurrency/retry cases, the owning Run suite passed 29 tests.
  The final unique targeted inventory contains 108 tests.
- `NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck`: passed from
  `apps/web`. The default 4 GiB heap exhausted memory before this retry.
- ESLint across every changed/new TS/TSX file with `--max-warnings 0`: passed.
- `pnpm run i18n:check` and `pnpm run i18n:ratchet`: passed from `apps/web`.
- Managed desktop walkthrough suite: 10 passed. After adding resize and
  replacement-feedback assertions, the two affected desktop cases passed.
- Managed `mobile-chrome` walkthrough suite: 3 passed.
- Public-doc validator tests: 63 passed. Public-doc validation: 48 pages passed.
- Specification catalog, full specification lint, artifact coverage, and diff
  whitespace checks passed.

Browser commands used the managed runner and one worker. On this Nix host,
`/tmp/run-walkthrough-e2e.py` removed inherited `KANDEV_*` launcher settings,
set loopback binding, and supplied the installed Chromium library directories.
The original attempt failed localhost readiness because of inherited config.
The next attempt reached Chromium but lacked its shared libraries. Both setup
runs were stopped. The isolated run passed. Later `--no-build` runs reused the
same production build because only E2E tests changed.

The first resize test expected the panel to remain open across layout remounts.
Source inspection confirmed existing remounts close it. The test now reopens
through the launcher and proves draft retention, as the requirement specifies.

Desktop reading/composing and phone composing screenshots were inspected.
Evidence is in `/tmp/walkthrough-feedback-evidence/`. The phone screenshot
shows the intended single-column panel and visible actions. Native operating
system keyboard behavior was not verified. Narrow and reduced-height browser
viewports were verified. No commit, push, or PR was requested.

## Related delivery record

[The earlier feedback-controls plan](../walkthrough-feedback-controls/plan.md)
records the completed removal of an inert Cancel button. Keep its historical
results. The new composer gives Cancel a real temporary state to dismiss.
The previous requirement is deprecated. This package's requirement is active,
its design is current, and public walkthrough instructions are updated.

## Risks

- A retained sent row can re-enter chat context unless every pending path excludes it.
- A broad success cleanup can remove comments added while a request is active.
- Legacy rows lack revision evidence. Exact snapshot matching is conservative.
- History remains local to a browser tab. Users must not infer shared discussion storage.
- Unsaved drafts are page-lifetime state and do not survive reload.

## Accepted UX additions

The user authorized these additions and their implementation:

1. Preserve an unsaved draft per step while navigating. This prevents accidental loss.
2. Add a comment count to each step so users can find feedback quickly.
3. Add a compact step picker with titles for longer walkthroughs.

## Implementation review refinements (2026-10-23)

The user authorized content-driven walkthrough height and the shared Kandev
step dropdown during review of the running test instance. Extend this existing
work order with AC-UI-WALKTHROUGH-STEP-FEEDBACK-002.6 and
AC-UI-WALKTHROUGH-STEP-FEEDBACK-003.4. Verify short/long content, opening/cancelling
the composer, desktop keyboard selection, and phone touch/viewport constraints.
Investigate the reported white editor in both light and dark themes before
changing editor code. Keep the isolated test URL available with the new build.

Validation complete:

- RED: the old native picker failed the keyboard disclosure test. The old
  fixed-height card failed the short-content geometry assertion.
- GREEN: all seven targeted component tests passed. Typecheck (8 GiB heap),
  changed-file ESLint, formatting, i18n ratchet, specification lint, artifact
  coverage, and public-doc validation passed.
- Desktop: nine tests passed in the full suite. Its old fixed-navigation-position
  assertion conflicted with the approved content-sizing change. After updating
  that assertion, the step-navigation and expanded drag/containment cases passed.
  The feedback case then passed with long-comment scrolling, keyboard open/Escape,
  shared-menu selection, and existing draft/reload/resize coverage.
- A temporary selection-focus assertion was removed after source inspection:
  selecting a newly opened file uses the existing editor-focus behavior. The
  regression now checks restored trigger focus after Escape, which does not
  navigate to a file.
- Isolated browser checks at port 37665 verified the rebuilt controls. Both
  initial dark/light themes and live theme switching produced matching editor
  backgrounds: dark rgb(20, 20, 20), light rgb(255, 255, 255). No mismatch between
  the app theme and editor theme was reproduced. No editor theme code changed.
- Phone: the two unaffected suite cases passed. The feedback case passed after
  the touch-size assertion waited for the shared dropdown's opening animation
  to finish. It verifies 44px option targets, content sizing, long-comment
  scrolling, draft retention, and submitted history.
- Desktop dark/light screenshots and the phone reading/composing screenshots
  were inspected. Native operating-system keyboard behavior remains unverified.

The isolated test UI was refreshed without restarting the backend or changing
its database. Its URL and guarded shutdown command remain valid. The main
instance on port 9998 was not modified.


## Mobile step selection repair (2026-10-23)

The user reported repeated taps on iPhone Safari. The shared Select closes on a
window resize. A WebKit reproduction changed viewport height between opening
and tapping an option; the menu detached before selection. The same regression
failed in the managed mobile Chrome suite. This confirms a reproducible failure
path, rather than proving the exact sequence on the user's physical device.

Phones and coarse pointers now use the existing MobilePickerSheet with Command
rows. Desktop retains the shared Select. The sheet has 44px targets, one scroll
region, localized labels/counts, current-step indication, and one-tap selection.
Keyboard focus enters the list on every opening, including reopening during the
closing animation. Escape restores the trigger. Tests now use actual touch taps
for phone selection rather than mouse clicks.

Validation:

- Three picker component tests pass: desktop keyboard selection, phone selection,
  and coarse-pointer selection after resize.
- The permanent touch/resize regression failed before the change and passes with
  the sheet, including immediate reopening and arrow-key selection.
- The desktop feedback E2E passes with the existing Select.
- WebKit iPhone emulation selected seven successive steps on the first tap across
  viewport-height changes. A separate check passed Escape focus restoration
  and keyboard selection. Physical iPhone hardware remains unverified.
- The isolated test instance keeps its existing URL, data, and shutdown command.
  Its frontend assets are refreshed in place. Port 9998 remains untouched.

Final repair checks passed: all four managed mobile walkthrough tests, all three
picker component tests, changed-file ESLint, formatting, typecheck (8 GiB heap),
i18n ratchet, specification validation, artifact coverage, and whitespace checks.
The final served build also passed WebKit keyboard selection, Escape focus
restoration, and seven consecutive one-tap selections across height changes.
WebKit emitted existing HTTP clipboard/editor cancellation errors during file
reveal; these did not prevent the picker checks. No physical-device claim is made.


## Desktop shared-picker follow-up (2026-10-23)

The user reported an intermittent double-opening effect in Firefox and asked
whether the desktop picker reused an existing dropdown. It already used
`@kandev/ui/select`, but supplied popper position, alignment, collision padding,
and trigger-width overrides. Remove these overrides and use the shared Select's
default item-aligned behavior, matching `TypedSortPicker` in task filters. Keep
the viewport width cap. The mobile picker sheet is unchanged.

Initial Chromium, WebKit, and Firefox traces each recorded one opening animation.
The exact intermittent effect was not reproduced, so this is a verified alignment
with the existing component pattern, not a confirmed root-cause claim.

The new build passed the desktop feedback E2E and all three picker component
tests. Firefox selected six successive steps, kept the menu within the viewport,
and passed keyboard opening and Escape focus restoration. The Firefox screenshot
was inspected. Changed-file ESLint, formatting, specification catalog/lint, and
whitespace checks passed. The isolated test frontend was updated in place.

The mobile touch/resize regression also passed on the new build, including
immediate reopening and keyboard selection.


## Immediate mobile touch repair (2026-10-23)

The user reported repeated taps for both opening and selection after the first
mobile repair. Reusing the old settled-target tests was insufficient. A WebKit
reproduction with raw touchscreen coordinates selected the wrong step during
sheet motion. Its event trace showed the closing sheet receiving a pointer-down
intended for the underlying trigger. The new managed mobile test also failed
on all three attempts before this repair, leaving the previous or wrong step.

Keep the shared MobilePickerSheet and disable its entrance/exit animation for
walkthrough selection. The optional DrawerContent control applies to both content
and backdrop, so closing cannot leave a tap-blocking overlay. Other consumers
keep the shared default animation. This is a frequent-selection surface, so
stationary targets take precedence over entrance motion. Touch sizes, focus,
keyboard selection, one scroll region, and viewport-height stability remain.

The permanent regression uses raw touchscreen taps without Playwright's stable
layout wait or target retries. It repeatedly opens, selects, and immediately
reopens. Its primary owner is `mobile-walkthrough.spec.ts` under AC-003.4.

Validation complete: the final mobile suite passed all five tests, including the
new immediate-tap regression and the existing viewport-height/keyboard checks.
The same WebKit script that failed before the change passed 24 consecutive
first-tap selections on the updated Tailscale test instance. All three picker
component tests, frontend and UI-package ESLint, formatting, typecheck (8 GiB
heap), i18n ratchet, specification checks, artifact coverage, and whitespace
checks passed. The isolated frontend was refreshed without changing its data
or restarting the backend. Physical iPhone confirmation remains with the user.

Physical iPhone repair is implemented under task 02 and AC-003.4. The captured
failed touches reach the controls but lack compatibility click events. The work
order specifies direct touch activation with cancellation and duplicate-click
protection. The shared bottom-sheet composition stays unchanged.


Final tap repair checks passed: 10 component tests, six mobile E2E tests, and
48 WebKit first-tap selections split between normal and withheld click events.
The new browser regression failed against the old build and passed against the
final build. Typecheck, changed-file lint, formatting, and i18n ratchet passed.
The isolated test frontend is updated. Physical iPhone validation remains open.
Temporary opt-in capture is retained only in that disposable instance until the
user retests. Task 02 records the trace, cancellation rules, and cleanup scope.


Physical iPhone validation (2026-10-24): the user confirmed that tapping now
works. The final phone trace also shows the shared drawer at the viewport
bottom. Temporary capture scripts were removed from the served page and the
owned collector was stopped. Saved evidence remains outside the repository.
The test app remains available. The user later clarified that the refinement
applies to the step selector, as recorded below.


## Mobile selector presentation (2026-10-24)

Status: complete. The user confirmed that the step selector should look and
move like the session and terminal bottom sheets. Keep the main walkthrough
panel unchanged. Restore the shared drawer motion and remove the now-unused
animation opt-out from MobilePickerSheet and DrawerContent. Reuse the sheet's
header, handle, backdrop, safe-area padding, and single scroll region.

Inside the sheet, use transparent list chrome and session-style active borders.
Each row shows its number, title, and a separate muted comment count. Preserve
localized accessible option labels, keyboard selection, and direct-touch handling.
The trigger uses the same downward chevron as other mobile pickers.

The existing no-motion raw-tap test becomes a consecutive first-tap test. It
waits for the requested sheet animation to finish before locating visible rows,
and for dismissal before tapping the revealed trigger. It does not retry taps.
The withheld-click regression and gesture-cancellation tests remain. Verify
standard motion, reduced-motion preferences, bottom containment, list styling,
keyboard behavior, dismissal, reopening, and a rendered phone screenshot.
No new translation keys are needed.


Validation: the motion assertion failed against the previous static build. The
updated implementation passes all six managed mobile E2E tests and ten picker
component tests. The mobile suite covers normal motion, reduced motion for both
content and backdrop, viewport changes, keyboard selection, and consecutive
single taps with and without browser click events. WebKit passed the rendered
sheet checks and 24 single-tap selections with browser clicks withheld. The
phone screenshot was inspected. Changed-file ESLint, formatting, typecheck,
i18n ratchet, spec validation/lint, artifact coverage, and whitespace checks
passed. The isolated Tailscale test instance was refreshed in place. The main
instance on port 9998 was not changed.


## Previous and Next touch repair (2026-10-24)

Status: complete. This extends the authorized walkthrough implementation.
The footer navigation still uses click-only handlers. A completed touch without
its compatibility click cannot change the step. Reuse the selector's validated
`useWalkthroughTap` helper for both navigation buttons. Keep the existing footer,
44px phone targets, safe-area clearance, one content scroller, labels, and shared
step state. Desktop mouse and keyboard activation retain their normal handlers.
Disabled first/last controls must not install touch activation handlers.

The mobile exemplar is the repaired walkthrough picker: one completed touch
performs one action, and its later browser click cannot repeat that action.
No layout or global button changes are required. The current phone and desktop
previews remain applicable. This restores the reachable navigation contract in
AC-UI-WALKTHROUGH-NAVIGATION-LAYOUT-001.7 and touch controls in
AC-UI-WALKTHROUGH-STEP-FEEDBACK-002.4.

Regression owner: the two `navigates Previous and Next with single taps` cases
in `apps/web/e2e/tests/review/mobile-walkthrough.spec.ts`. Traverse all steps in
both directions with normal clicks and with clicks withheld, check 44px targets,
disabled boundaries, Enter, and Space. Use the existing picker component tests
for gesture cancellation and compatibility-click suppression. Verify the same
navigation on the isolated Tailscale app using WebKit.

Files: walkthrough-step-card.tsx, use-walkthrough-tap.ts (ownership comment),
mobile-walkthrough.spec.ts, and this existing design package. Run the managed
mobile walkthrough suite, picker component tests, changed-file ESLint,
formatting, frontend typecheck, i18n ratchet, and specification validation/lint.
Refresh only the existing isolated frontend after a successful build.


Repair evidence: before the change, normal navigation passed but the withheld
click case stayed on Step 1 instead of Step 2 on every test attempt. After the
change, both navigation regressions pass, including synthetic pointer events
inside disabled buttons and Enter/Space activation. The ten existing picker
component tests pass. WebKit passes sixteen first-tap step transitions across
normal and withheld-click runs, plus disabled-boundary and keyboard checks.
Changed-file ESLint, formatting, frontend typecheck, i18n ratchet, specification
validation/lint, artifact coverage, and whitespace checks pass. The isolated
Tailscale frontend was refreshed without restarting or changing port 9998.

Final managed mobile walkthrough suite: all eight tests passed (1.6 minutes).

PR preparation: 118 focused tests and all 10 desktop walkthrough E2E tests
passed. Full localization checks passed. The user-requested isolated instance
on port 37665 was stopped before publication; port 9998 remains active.


## PR landscape navigation review (2026-10-24)

Status: complete. Greptile identified a valid clipped-footer case at 667x375.
The 640px desktop height override applies before the 768px phone control sizes
end, leaving insufficient height for the non-shrinking composer and navigation.
The regression opens a long draft and checks actual hit testing for Next at
639px, 640px, 667px, and 767px widths with 375px viewport height, then checks
forward/back navigation and draft retention. Use the full available phone
height below 768px while keeping the approved card placement and shared state.

The system design also retained a stale fixed-card instruction. Remove that
instruction in favor of the existing content-driven height contract. This
implements comments 4367476248 and 4367476252 under the existing AC-002.4 and
AC-002.6; no new product requirement is required. Reuse the current phone
composition preview, one scrolling body, 44px targets, and safe-area clearance.
Run the mobile suite, focused picker tests, typecheck/lint, and spec checks.
Recapture mobile PR screenshots after the final fix.


CodeRabbit aggregate review 5638682499 also identified the stale height text,
a stale render-time sent-status guard, and a dense public-doc paragraph. The
guard now reads the current comment store at submission time, with a regression
that changes acknowledgement after render and proves pending delivery remains
reachable. Replace the old promise-only acknowledgement test with this store-guard
regression. Final comments-domain results are recorded below. Public draft retention and saved
comment limits are separated. No blanket docstring expansion is planned:
repository guidance asks comments to document invariants, and no repository CI
policy requires an 80% function-docstring threshold.

Landscape validation: the initial rebuilt height-cap-only change still clipped
navigation. A disposable browser probe proved the cause: scroll-region padding
and the composer's minimum sizes. Put padding inside the scrolling body, allow
the open composer to shrink its input to 44px, and keep its action row fixed.
The final mobile suite passes all nine tests, including 375px and 320px heights,
actual button hit testing, navigation, and retained drafts. The single-scroll
owner and the existing 640px floating placement boundary remain.

Final review checks: all 114 comments-domain tests pass. The rebuilt mobile
suite passes nine tests; the three affected desktop navigation, dragging, and
feedback tests pass. Typecheck, changed-file lint, localization ratchet, and
specification checks pass.


## Workspace-adoption CI assertion repair (2026-10-24)

CI run 40103966553, shard 19, failed the authenticated workspace-adoption test
because it read the fixture's original file before the asynchronous mock turn
wrote SECOND_MODIFICATION. The scenario delays the write by 50ms, and file
existence is not evidence of that write. The same original assertion passed
three isolated local repetitions, so the CI artifact is the failure evidence.
The correction polls for the intended file contents instead of polling only
existence. No product behavior, adoption contract, or backend code changes.

The feature branch was rebased onto authoritative main commit
0b81ae48d4e2c358f80ac489e39388dfd8d7e6ab to change this existing upstream test.
The remote PR head before the rebase was bde20fa0f45ef328fe0a763ead949fb43baca2ce;
push uses that exact lease. Validate the corrected authenticated test with
`pnpm --dir apps/web e2e:run --project chromium tests/session/workspace-adoption.spec.ts --grep 'auth true' --retries 0 --repeat-each 3`, then run the rebuilt
walkthrough feedback and mobile suites before final delivery.

The corrected workspace-adoption assertion passes three fresh authenticated
runs with retries disabled. On the updated branch, all 158 focused tests,
typecheck, changed-file ESLint, and specification validation/lint pass.


## Composer minimum-space refinement (2026-10-24)

Final screenshot inspection found that Next remained clickable while the
composer action row overlapped navigation. The strengthened landscape test
checks both row order and textarea/action separation, in addition to hit testing
and viewport bounds. It fails against 0c585f37951. Reserve 8.125rem for the open
composer: a 44px input, a 44px action row, padding, gap, and borders. A disposable
browser probe verifies both rows fit at 375px and 320px heights with this floor.
Keep all previous content-driven height, draft, and input-scroll behavior.

The strengthened regression and all nine mobile tests plus the affected
desktop feedback test pass (10 tests). Composer tests, changed-file lint,
formatting, and specification checks also pass.
