---
id: "02-step-feedback-ui"
title: "Display feedback and disclose the composer"
status: done
wave: 2
depends_on:
  - "01-retain-feedback"
plan: "plan.md"
requirements:
  - REQ-UI-WALKTHROUGH-STEP-FEEDBACK-001
  - REQ-UI-WALKTHROUGH-STEP-FEEDBACK-002
  - REQ-UI-WALKTHROUGH-STEP-FEEDBACK-003
acceptance_criteria:
  - AC-UI-WALKTHROUGH-STEP-FEEDBACK-001.1
  - AC-UI-WALKTHROUGH-STEP-FEEDBACK-001.2
  - AC-UI-WALKTHROUGH-STEP-FEEDBACK-001.3
  - AC-UI-WALKTHROUGH-STEP-FEEDBACK-001.4
  - AC-UI-WALKTHROUGH-STEP-FEEDBACK-001.5
  - AC-UI-WALKTHROUGH-STEP-FEEDBACK-002.1
  - AC-UI-WALKTHROUGH-STEP-FEEDBACK-002.2
  - AC-UI-WALKTHROUGH-STEP-FEEDBACK-002.3
  - AC-UI-WALKTHROUGH-STEP-FEEDBACK-002.4
  - AC-UI-WALKTHROUGH-STEP-FEEDBACK-002.5
  - AC-UI-WALKTHROUGH-STEP-FEEDBACK-002.6
  - AC-UI-WALKTHROUGH-STEP-FEEDBACK-003.1
  - AC-UI-WALKTHROUGH-STEP-FEEDBACK-003.2
  - AC-UI-WALKTHROUGH-STEP-FEEDBACK-003.3
  - AC-UI-WALKTHROUGH-STEP-FEEDBACK-003.4
system_design:
  - ../../specs/ui/system-design/walkthrough-step-feedback.md
---

# Task 02: Display Feedback and Disclose the Composer

## Summary

Render the selected step's comment history below its explanation. Replace the
always-visible composer with a Comment button on desktop and phones.

## In scope

- Shared step feedback component, session hydration, disclosure, and focus.
- Page-lifetime drafts, saved comment counts, and a shared Kandev step picker.
- Safe save outcome handling and source-specific Submitted status.
- Phone target sizes, scroll containment, translations, and browser evidence.
- User-guide update and specification lifecycle reconciliation.

## Out of scope

Server-persisted drafts, threaded discussions, global UI polish,
and backend changes.

## Acceptance

1. Pending and submitted comments render only in the matching step. Empty steps
   have no comment-list chrome. Direct and queued delivery retain history.
2. Comment alone opens the composer. Cancel/Escape restore focus. Add and Run
   save exactly one row. Missing context preserves entered text.
3. Desktop and phone flows pass the plan's E2E matrix. Public instructions and
   specification statuses match the implemented behavior.

## UI preview

See the [combined preview](plan.md#ui-preview). The task requires structured
previews instead of ASCII.

| View | Scrolling content | Fixed controls |
| --- | --- | --- |
| UI-01: Desktop reading | Explanation, Pending row, Submitted row | Comment, Previous, Next |
| UI-02: Desktop composing | Same comment history | Input, Cancel/Add/Run, navigation |
| UI-03: Phone reading | One column in the existing walkthrough panel | Touch-sized Comment and navigation |
| UI-04: Phone composing | Body shrinks within the dynamic viewport | Input/actions and navigation clear keyboard and safe area |

Map these views to requirement 001.1 and requirement 002.1 through .5. Empty
input disables submission. Missing session keeps the draft visible. A delivery
failure shows one pending row and an error. Test focus return and both the 640px
presentation boundary and 768px phone sizing boundary.

## Verification

Run from the repository root after Task 01. Use TDD for the new tests. The managed
E2E runners rebuild the application. Run desktop and mobile commands sequentially.

```bash
(cd apps/web && pnpm exec vitest run components/diff/walkthrough-step-card.test.ts components/diff/walkthrough-step-feedback.test.tsx components/diff/comment-form.test.tsx lib/walkthrough-feedback-state.test.ts lib/walkthrough-comments.test.ts lib/state/slices/comments/walkthrough-history.test.ts hooks/domains/comments/use-run-comment.test.ts components/task/chat/chat-input-area-context-submission.test.tsx components/task/passthrough-chat-composer.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/diff/walkthrough-step-card.tsx components/diff/walkthrough-step-feedback.tsx components/diff/comment-form.tsx lib/walkthrough-comments.ts lib/state/slices/comments/types.ts lib/state/slices/comments/comments-store.ts hooks/domains/comments/use-pending-comments.ts hooks/domains/comments/use-run-comment.ts components/task/chat/chat-input-area.tsx components/task/passthrough-chat-composer.tsx)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/review/walkthrough.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/review/mobile-walkthrough.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Use `pnpm run i18n:zh-hant` from `apps/web` when generating Traditional Chinese
catalog changes. Record exact changed-file lint coverage if extraction creates
additional source files. Capture screenshots for UI-01 through UI-04, including
long comments and a constrained phone viewport. Report native keyboard behavior
as unverified if browser automation cannot reproduce it.

## Files likely touched

- `apps/web/components/diff/walkthrough-step-card.tsx`.
- Proposed `apps/web/components/diff/walkthrough-step-feedback.tsx` and `.test.tsx`.
- `apps/web/components/diff/comment-form.tsx` and proposed `.test.tsx`.
- `apps/web/e2e/tests/review/{walkthrough,mobile-walkthrough}.spec.ts`.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko}/diff.json` and generated pseudo catalog if required.
- `docs/public/sessions-and-review.md`.
- This plan, work orders, paired specs, and the prior feedback-controls requirement status.

The primary conversation owns these changes. Preserve Task 01's comment metadata
and delivery fixes. Do not rewrite the earlier completed plan's historical results.

## Dependencies

Task 01 must pass before starting this task.

## Risks

Shared CommentForm consumers expect void callbacks. Additive save-result behavior
must preserve their success and cancellation behavior. A comment list must not
push navigation offscreen or create a second vertical scrollbar.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/walkthrough-step-feedback.md).
- [Design](../../specs/ui/system-design/walkthrough-step-feedback.md).
- Existing walkthrough desktop/mobile E2E and `MobilePickerSheet` geometry.
- `/mobile-parity`, `/better-ui`, `/tdd`, `/e2e`, and `/docs-maintainer`.

## Results

Implemented the step comment list, on-demand composer, page-lifetime drafts,
comment counts, and shared Kandev step picker. All seven real locales and pseudo are
updated. Public instructions and specification lifecycle are synchronized.

The combined targeted test run passed 106 tests across 12 files. The Run suite
then passed 29 tests after two additional concurrency/retry cases. Typecheck
passed with `NODE_OPTIONS=--max-old-space-size=8192`. Changed/new-file ESLint
passed with zero warnings. Both i18n checks passed.

The managed desktop suite passed 10 tests. The final two affected cases passed
after adding resize and replacement-feedback assertions. The mobile suite
passed 3 tests. See the plan for exact environment adjustments and command results.

All 63 public-doc validator tests passed. Public documentation validation passed
for 48 pages. Specification catalog/lint and whitespace checks passed. Desktop
and phone screenshots match UI-01 through UI-04. Native OS keyboard behavior
remains unverified, while narrow/reduced-height viewport behavior passed.

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

## Physical iPhone tap repair (2026-10-24)

Status: implemented and checked locally. Physical iPhone confirmation is pending.
AC-003.4 remains the applicable requirement. The physical
Safari trace contains 14 touch down/up pairs but only six click events. Failed
taps hit connected, stationary controls with pointer events enabled. The picker
currently activates only through click, so those completed touches do nothing.
The trace does not establish why Safari omits its compatibility click.

Keep the shared sheet and Command rows. Add a scoped touch-tap handler for its
trigger and rows. Activate on release only for a primary touch that remains
within the control, moves at most 8 CSS pixels, and lasts less than 500 ms.
Cancel scrolling, canceled pointers, multi-touch, and long presses. Suppress a
subsequent compatibility click, including one retargeted after the row unmounts.
Release that guard on the next pointer gesture or unmount. Mouse and keyboard
keep their existing handlers. No global component behavior changes.

Regression owner: walkthrough-step-picker.test.tsx exercises the recorded
pointer sequence without a click, at-most-once activation, gesture cancellation,
and input-mode changes. Mobile E2E exercises the real sheet with compatibility
clicks withheld, plus the existing raw-tap and viewport tests. Run focused
Vitest, mobile walkthrough E2E, changed-file lint, typecheck, and spec checks.


Final repair validation: all 10 picker component tests passed. The new browser
case failed against the old build on all three attempts because the sheet did
not open without click events (`/tmp/kandev-run.e2e.56kghjbU.log`). After the
final touch guard change, all six mobile walkthrough E2E tests passed
(`/tmp/kandev-run.e2e.4yxtqPFU.log`). The served build passed 24 raw WebKit tap
selections with normal clicks and 24 with clicks withheld. Changed-file ESLint,
formatting, typecheck, and i18n ratchet passed. The browser checks emulate the
missing-click event sequence; they do not establish a physical-device fix.

The isolated frontend was refreshed at the existing Tailscale URL. Its backend
and data remain in place. Temporary v3 capture remains opt-in for the physical
retest, so another failure can be inspected without another capture setup.
The guarded shutdown command also stops the collector. Remove that collector
and the injected script after physical validation. No diagnostic code enters
the repository and no commit or PR was created.


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

## CI interaction-fixture ordering (2026-10-24)

CI run 40112617636 failed three first-attempt browser cases; each passed its retry.
All backend checks passed. The browser failures are the failure evidence: the
original stale-activity case passed three local repetitions, and the two original
mobile cases passed three repetitions each.

- The stale-activity case now holds the real focus-refresh request while it seeds
  and verifies the missed-event projection. It releases that request and verifies
  the authoritative response clears the indicator. An unrelated refresh cannot
  erase the projection between setup and its assertion. Removing the suite-local
  retry override lets the runner's `--retries 0` apply to this case.
- The mobile workflow helper waits for the shared drawer's finite animations before
  its single tap. Visibility alone does not establish animation completion.
- The question helper keeps its native touch-scroll events and waits for both
  scroll owners to remain still across animation frames. Its single locator tap
  checks actionability and computes the current center after the recorded hit test.
  Raw rapid touch-move frames could leave inertial scrolling active, and a stored
  coordinate could become stale before activation. Native scrolling, hit testing,
  and complete answer-receipt assertions remain.

The stale-activity repair passed five fresh runs with retries disabled:
`pnpm --dir apps/web e2e:run --no-build --project chromium tests/session/session-resume.spec.ts --grep 'clears stale background activity' --retries 0 --repeat-each 5`.
The final question helper passed six cases (both phone sizes, three repetitions):
`pnpm --dir apps/web e2e:run --no-build --project mobile-chrome tests/task/mobile-threads-composer-disclosure.spec.ts --grep 'long required question' --retries 0 --repeat-each 3`.
The drawer readiness check passed three palette workflow cases in the mobile
probe. All these cases passed on their first attempt. Changed-file ESLint,
Prettier, documentation catalog validation, and specification lint passed.
These are test-fixture changes; no rendered UI or product interaction behavior changes.
