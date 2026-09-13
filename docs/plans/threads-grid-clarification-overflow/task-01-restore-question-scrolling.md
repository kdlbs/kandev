---
id: "01-restore-question-scrolling"
title: "Restore required-question scrolling"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-THREADS-DECK-004
  - REQ-UI-THREADS-DECK-005
acceptance_criteria:
  - AC-UI-THREADS-DECK-004.1
  - AC-UI-THREADS-DECK-004.6
  - AC-UI-THREADS-DECK-004.7
  - AC-UI-THREADS-DECK-005.1
  - AC-UI-THREADS-DECK-005.3
  - AC-UI-THREADS-DECK-005.5
  - AC-UI-THREADS-DECK-005.6
  - AC-UI-THREADS-DECK-005.7
  - AC-UI-THREADS-DECK-005.8
  - AC-UI-THREADS-DECK-005.9
  - AC-UI-THREADS-DECK-005.11
system_design:
  - ../../specs/ui/system-design/threads-conversation-deck.md
  - ../../specs/ui/system-design/threads-saved-views.md
---

# Task 01: Restore Required-Question Scrolling

## Summary

Allow the existing clarification scroller to chain vertical input through its
clipped wrapper into the bounded Threads footer. Preserve all other question,
composer, transcript, layout, and submission behavior. The user explicitly
authorized implementation on 2026-09-13 after the design handoff.

## In scope

- Use `/tdd` and `/e2e`: add a rendered behavioral RED before production edits,
  then the smallest Threads-scoped vertical containment correction.
- In `ClarificationPanelSection`, identify Threads by the existing disclosure
  context's presence, including `enabled: false`. Allow vertical chaining on
  `clarification-overlay-container`; retain its horizontal behavior and the
  outer `ComposerFooterAllocation` bound/containment.
- Add the desktop/phone cases below beside the existing disclosure scenarios;
  keep all current test coverage. Record fresh commands, geometry, screenshots,
  and results here, and synchronize `plan.md` at completion.

## Out of scope

New scroll owners/handlers, tile sizing or fallback changes, moving/sticking
question actions, composer/disclosure state changes, new backend fixtures or
protocols, translation changes without new copy, normal-chat containment
changes, public-doc redesign, publishing to the parent PR, and delegation.

## Acceptance

1. With two admitted tasks at a 1366x768 content viewport and both auto-hide
   settings, Grid remains Grid and wheel input over the question reaches the
   final option and Next. Every target is fully within the footer's visible
   bounds, its center passes `elementFromPoint`, and real activation succeeds.
   Answer all three questions and submit; the pending overlay clears and the
   mock agent receives the exact selected answers. Repeat at real 90% browser
   zoom, recording the API zoom value and effective CSS viewport.
2. Keyboard-only Tab/Shift+Tab and Enter/Space can select and submit the same
   bundle; Next/Back navigation is exercised without implicitly answering.
   Neighbor tile boxes and board scroll offsets stay unchanged, the transcript
   retains its existing follow/history/frozen policy and 80px floor, and the
   existing CI, disclosure, normal task chat, and Columns scenarios still pass.
3. Pixel 5 and short 393x500 phone views retain one active conversation and
   an inline visible composer. Real vertical touch input reaches final options
   and Submit; tap completes the pending request. Verify actual hit geometry,
   option/primary-action touch targets, safe-area/viewport clearance, and no
   document horizontal overflow. Retain the existing coarse-pointer tablet
   and saved-preference assertions, plus the 767/768px composition boundary.

## ASCII UI preview

UI-01 excerpt from [the combined preview](plan.md#ascii-ui-preview), covering
`005.5`, `005.6`, `004.1`:

```text
+----------------------------+
| Grid tile A         [Open] |
| transcript (80px minimum)  |
| question / options         |
| inner scroll -> footer     |
| [last option]       [Next] |
+----------------------------+
| Grid tile B: same bounds   |
+----------------------------+
```

Keep both tile bounds fixed. The header Submit remains in its existing place;
scroll back to it for final submission. UI-02 phone keeps the existing
composition (`004.6`, `005.7`, `005.8`):

```text
[Threads / View] [1/2] [Menu]
[Task A v]            [Open]
[one transcript            ]
[inline question / actions ]
[normal composer           ]
[safe-area clearance       ]
```

Spacing is illustrative. Do not add a drawer, sticky action row, or new phone
navigation. Retain the existing local question scroller and its outer footer
boundary; remove the input trap between them.

## TDD and browser mechanics

Add a test named `scrolls long required questions through the Grid footer`
parameterized by auto-hide true/false and native zoom 1/0.9. Use the existing
`/e2e:clarification-multi` scenario through `createTaskWithAgent`, wait for real
`WAITING_FOR_INPUT`, and create a second admitted task. Keep its native pending
request and sender; do not replace the question with DOM-only options or a
summary-store fake. The one-task layout is full height and misses this bug.

Use a scoped helper in `threads-clarification-helpers.ts` for shared seeding,
geometry/gesture operations and the native-zoom setup when needed. For native
zoom, create an owned temporary Chromium persistent context with `channel:
"chromium"`, `headless: true`, `viewport: null`, and `deviceScaleFactor:
undefined` to override the project's inherited emulation. Create an ephemeral
MV3 extension with only `tabs` permission, call `chrome.tabs.setZoom`, verify
`getZoom`, and record `innerWidth`, `innerHeight`, devicePixelRatio and
visualViewport.scale. Use `--window-size=1366,855` for the verified 1366x768
baseline content area. This produced 1517x853 at 90% on this host. Reuse the
same isolated fixture backend; close the browser and delete only its temporary
profile/extension in `finally`. No shared Playwright config or dependencies
need changing. The [evidence recipe](evidence.md#native-zoom) records the API
and setup details. If a runner lacks regular Chromium, report that limitation
and retain the 100% behavioral gate; never silently call an approximation real
zoom or skip all scroll coverage.

The RED assertion must fail because the outer footer stays at scrollTop 0 and
the final target remains clipped after genuine wheel events over the question.
Use small bounded wheel deltas and recompute the target's full containment,
not only its center; a working scroll chain can overshoot with a large delta.
Do not use `scrollIntoView`, direct scrollTop assignment, `.focus()`, or an
auto-scrolling locator `.click()` to make an unreachable option pass. Read
geometry and use `page.mouse` before activation. After proven containment and
hit-testing, activate the option and assert the actual carousel/backend result.
Exercise Next and Back before completing all answers, then scroll to the
existing header Submit. Wait for the agent's answer receipt/pending-clear
state, not only disappearance caused by navigation.

Add `answers a long required question with keyboard navigation` and
`keeps long required answers usable in Columns` in the desktop file. Native
keyboard focus may scroll the footer, which is existing useful behavior;
verify selection and submission, not just focusability. Preserve all current
CI, history/bottom/frozen-scroll, draft, cancellation, and session-ownership
scenarios. For the new gesture case record sibling boxes and board offsets
before/after question scroll and submission. Preserve reader anchoring checks
with a transcript taller than the viewport.

Add `scrolls and submits a long required question by touch` in the existing
mobile disclosure file. Use the mobile-chrome project's Pixel 5 device; do not
replace it with desktop dimensions/deviceScaleFactor. Use real CDP touch
start/move/end for vertical scrolling and `.tap()` for actions. Follow the
cleanup pattern in `mobile-threads-swipe-helpers.ts`; always end the gesture
and detach CDP in `finally`. Cover canonical/short phone sizes, 767/768px
composition, and the existing 900px coarse-pointer tablet path without
changing saved Grid/auto-hide preferences. Assert Submit/option targets are
at least 44px, full containment, center hit-test, single detail, and successful
submission. Native hardware keyboard/safe-area behavior remains a separately
labelled limitation of emulation.

## Verification

From the repository root. This workspace already ran the frozen install;
a fresh worktree must first run `(cd apps && pnpm install --frozen-lockfile)`.
Run one command at a time. No retries, additional workers, or overlapping full
suites. Build after production changes; reuse only those just-built assets.
The lean host target suffices for these local/worktree executor tests and
avoids rebuilding unrelated remote-platform helpers on a cold machine.

```bash
(cd apps/web && pnpm exec vitest run components/task/chat/clarification-panel-section.test.tsx hooks/use-resizable-clarification-overlay.test.ts components/task/chat/composer-disclosure.test.tsx components/task/chat/use-composer-disclosure.test.ts components/task/chat/transcript-viewport-resize.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/task/chat/clarification-panel-section.tsx e2e/tests/task/threads-composer-disclosure.spec.ts e2e/tests/task/mobile-threads-composer-disclosure.spec.ts e2e/tests/task/threads-clarification-helpers.ts)
(cd apps/web && pnpm run i18n:ratchet)
make -C apps/backend build-dev e2e-plugin-package
make build-web-e2e
(cd apps/web && pnpm e2e:raw --project=chromium e2e/tests/task/threads-composer-disclosure.spec.ts e2e/tests/chat/clarification-resize.spec.ts --list --workers=1 --retries=0)
(cd apps/web && pnpm e2e:run --host --no-build --shards 1 --project chromium tests/task/threads-composer-disclosure.spec.ts tests/chat/clarification-resize.spec.ts --workers=1 --retries=0)
(cd apps/web && pnpm e2e:raw --project=mobile-chrome e2e/tests/task/mobile-threads-composer-disclosure.spec.ts --list --workers=1 --retries=0)
(cd apps/web && pnpm e2e:run --host --no-build --shards 1 --project mobile-chrome tests/task/mobile-threads-composer-disclosure.spec.ts --workers=1 --retries=0)
git diff --check
git status --short -- docs/plans/threads-grid-clarification-overflow
```

Before the production correction, run the new desktop regression against the
current built assets using the same managed command with `--grep 'scrolls long
required questions through the Grid footer'`. Record its behavioral RED, then
rebuild and run the complete command block for GREEN. List/discovery counts,
actual zoom/viewport, geometry, input mode, and all failed setup attempts belong
in Results. Do not claim the planning-stage DOM probe as GREEN.

## Files likely touched

- `apps/web/components/task/chat/clarification-panel-section.tsx`.
- `apps/web/e2e/tests/task/threads-composer-disclosure.spec.ts`.
- `apps/web/e2e/tests/task/mobile-threads-composer-disclosure.spec.ts`.
- New `apps/web/e2e/tests/task/threads-clarification-helpers.ts`, using existing
  `threads-presentation-helpers.ts`, clarification/session API helpers and
  native mobile gesture conventions.
- Existing clarification panel unit test only if needed to cover an added
  behavioral seam; browser tests own the layout regression.
- This plan, work order, and their evidence/results artifacts.

No changes to `TaskChatPanel`, `ComposerFooterAllocation`, disclosure CSS,
transcript scroll owners, backend, locale catalogs, or shared configuration
are expected. Reassess the plan if evidence requires any of those boundaries
to change materially.

## Dependencies

None. Verified base: `d62daa9d70437e1a46f7dd31f6138f90ebd54b3f`; parent PR
#3626 is merged at `f718c50666eb7175e39087294afabc70ac39f3a7`. Reconcile current
main and preserve concurrent edits before implementation/delivery. Work only
in this task workspace and its branch.

## Risks

A context `enabled` check would miss auto-hide off and phones. Global overscroll
changes could alter normal task chat. Focus/locator auto-scrolling can mask RED.
Native zoom has fractional CSS bounds and inherits Playwright context defaults;
keep that setup explicit. Never touch developer :9998 or parent playground
:48431; fixture teardown owns only its isolated backend/data/browser.

## Parallelism

`sequential`; no delegated agents or platform task/session creation authorized.

## Inputs

- [Plan and mobile contract](plan.md).
- [Measured reproduction and causal probe](evidence.md).
- Existing deck requirements: `004` layout and `005` composer criteria above.
- Deck system design: Composer disclosure, Composer geometry, Responsive behavior.
- Saved-view system design: Presentation preferences (preserve only).
- Reviewed Threads layouts Tasks 02, 03, 06 and current scoped AGENTS.md.

## Results

Implementation and task-defined verification are complete. Refreshed main is
`4e4b29b29e7680c0ed6c5de003a8dfe7e8b2fb94`; its one intervening Office fix has no
overlap with this work. The unpushed documentation commit was rebased onto that
main before edits. Planning evidence remains in `evidence.md`.

### Change and conformance

`ClarificationPanelSection` now overrides only `overscrollBehaviorY` to `auto`
when `useComposerDisclosureContext()` is non-null. This covers auto-hide on,
auto-hide off, Columns and touch Threads; normal task/Quick Chat keep their
existing containment. No height limit, resize behavior, disclosure state,
transcript owner, question action, or phone composition changed.

Permanent coverage adds four native-zoom Grid cases, one keyboard case, one
Columns case and two phone touch cases. Existing scenarios remain intact.
The normal-chat resize spec's local `retries: 1` override was removed so the
explicit zero-retry run applies to all affected cases.

`AC-UI-THREADS-DECK-005.5` and `005.6` are restored. All mapped preservation
criteria use the existing deck requirements and design; no new AC or contract
was introduced. Internal docs are updated; public docs need no change because
the existing independent-scroll and in-Threads answer behavior is restored.

### Validation receipt

The frozen install from diagnosis remained valid: the intervening main commit
changed neither package manifest nor lockfile. Commands ran from the locations
specified in Verification. Every browser run was headless, strict WS, one
worker and retries zero; no browser suites overlapped.

| Check | Result |
| --- | --- |
| Five named unit files | 5 files, 46 tests passed (17.23s) |
| `pnpm run typecheck` | Passed |
| Named scoped ESLint command, also including `e2e/tests/chat/clarification-resize.spec.ts` | Passed; rerun on the final changed test code |
| `pnpm run i18n:ratchet` | Passed, one modified production file clean; 644 guard entries intact |
| `make -C apps/backend build-dev e2e-plugin-package` | Passed on refreshed main |
| `make build-web-e2e` | Passed; Vite completed in 9.13s after the final production edit |
| Desktop discovery | 17 tests in 2 files |
| Desktop behavioral RED, Grid grep | All 4 failed at full-containment/hit assertions after actual wheel input |
| First full desktop GREEN attempt | 13 passed, 4 failed (4.7m); all question submissions succeeded, but the added post-submit history assertion contradicted native work-start auto-follow |
| Six-case desktop correction/capture run | 4 passed, 2 failed (1.7m); synthetic scrollTop after disabling auto-scroll did not update the native frozen offset |
| Final four-case Grid rerun | 4 passed (1.2m); uses real transcript wheel input, preserves history before Submit, verifies follow when enabled and the frozen offset when disabled |
| Mobile discovery | 5 tests in 1 file |
| Documentation catalog/spec lint and diff checks | Passed: 267 decisions, 873 specifications; all spec files clean |
| Full mobile command with `CAPTURE_PR_ASSETS=1` | 5 passed (1.2m), including both new touch cases |

The full desktop command is the one listed in Verification. Its 11 unchanged
compatibility cases all passed. The two new keyboard/Columns cases passed in
both the full run and the six-case rerun. Only the four edited Grid cases
needed the last rerun. Thus all 17 affected desktop cases pass on the final
production implementation, with every changed regression run after its last
edit. Exact supplemental commands, from `apps/web`:

```bash
pnpm e2e:run --host --no-build --shards 1 --project chromium tests/task/threads-composer-disclosure.spec.ts --grep 'scrolls long required questions through the Grid footer' --workers=1 --retries=0
CAPTURE_PR_ASSETS=1 pnpm e2e:run --host --no-build --shards 1 --project chromium tests/task/threads-composer-disclosure.spec.ts --grep 'scrolls long required questions|answers a long required question|keeps long required answers' --workers=1 --retries=0
CAPTURE_PR_ASSETS=1 pnpm e2e:run --host --no-build --shards 1 --project chromium tests/task/threads-composer-disclosure.spec.ts --grep 'scrolls long required questions' --workers=1 --retries=0
CAPTURE_PR_ASSETS=1 pnpm e2e:run --host --no-build --shards 1 --project mobile-chrome tests/task/mobile-threads-composer-disclosure.spec.ts --workers=1 --retries=0
```

### Final behavior and geometry

The [raw geometry](evidence/implementation-geometry.json) records each actual
activation. Chromium `149.0.7827.55` used a 1366x768 content viewport at native
zoom 1 and 1517x853 at native zoom 0.9, with visualViewport scale 1. Native tab
zoom was verified through `chrome.tabs.getZoom`; device scale and CSS zoom
were not substitutes.

The Grid footer remains 198.50 CSS px at 100% and 240.87 CSS px at 90%; the
transcript retains its 80px floor. Every final option, Next, Back and Submit
had its full box inside all clipping ancestors and passed center hit-testing
before mouse activation. The agent received exactly `db=q1_opt3`,
`language=q2_opt3`, `deploy=q3_opt2`; the pending overlay cleared. Tab,
Shift+Tab, Enter and Space completed the same bundle in the keyboard case.
Neighbor boxes and deck scroll offsets remained unchanged.

Pixel 5 (393x851) and short 393x500 phone emulation completed that same exchange
with CDP touch start/move/end and real taps. Option rows measured 56.5px high,
Submit 44px; full containment and real hits passed. One active conversation,
the visible composer, viewport clearance, no document horizontal overflow,
the 767/768px transition and saved Grid/auto-hide preferences passed. Existing
900px coarse-pointer tablet, draft, model, attachment and cancellation cases
also passed. Screenshots were inspected against UI-01/UI-02.

Phone verification uses emulation's safe-area values and a resized viewport;
it does not simulate an OS keyboard, physical display cutout, Safari, or a
hardware trackpad. No failure remains in the exercised browser paths.

### Isolation and teardown

The final Grid run owned `/tmp/kandev-e2e-0-JmMopy`; the phone run owned
`/tmp/kandev-e2e-0-CdFlQo`. Both used the worker's isolated port 18100 and were
fully removed by fixture teardown; absence checks passed. While such a fixture
owns that port, the exact fallback is `scripts/kandev-kill 18100 --yes`.
Do not apply that historical command to a future occupant. Temporary
`threads-question-zoom-*` profiles/extensions close in `finally`; none remain.
Developer :9998 and parent playground :48431 were never touched.

Screenshots are retained on an orphan media commit, rather than as binaries in
the fix branch. Final rendered examples: [Grid at 90%](https://raw.githubusercontent.com/kdlbs/kandev/e0c9e9c77703f7a2efc2729f5c26fb883c29be0a/grid-90-last-option.png), [Grid Submit](https://raw.githubusercontent.com/kdlbs/kandev/e0c9e9c77703f7a2efc2729f5c26fb883c29be0a/grid-100-submit.png), [short phone](https://raw.githubusercontent.com/kdlbs/kandev/e0c9e9c77703f7a2efc2729f5c26fb883c29be0a/phone-short-last-option.png), [phone Submit](https://raw.githubusercontent.com/kdlbs/kandev/e0c9e9c77703f7a2efc2729f5c26fb883c29be0a/phone-submit.png).

### PR review correction

PR #3655's documentation finding corrected the parent package's stale
"pending results" reference to "completed results", matching this plan's
implemented status. The historical parent evidence remains intact. This
documentation-only correction passed catalog validation, specification lint
and diff checks; production code, tests and screenshots are unchanged.
