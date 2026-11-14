---
id: "01-stable-status-frame"
title: "Stabilize status frames and PR actions"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-DISCLOSURE-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-PR-DISCLOSURE-001.1
  - AC-INTEGRATIONS-GITHUB-PR-DISCLOSURE-001.2
  - AC-INTEGRATIONS-GITHUB-PR-DISCLOSURE-001.3
  - AC-INTEGRATIONS-GITHUB-PR-DISCLOSURE-001.4
  - AC-INTEGRATIONS-GITHUB-PR-DISCLOSURE-001.5
  - AC-INTEGRATIONS-GITHUB-PR-DISCLOSURE-001.6
system_design:
  - ../../specs/integrations/system-design/github-pr-status-disclosure.md
---

# Task 01: Stabilize Status Frames and PR Actions

## Summary

Make the PR link stationary through sequential status updates. Deliver the
stable desktop frame and existing mobile drawer as one tested presentation change.

## In scope

- Fixed frame, header and footer, one inner scroller, and labelled PR action.
- First-load placeholders, cached refresh continuity, localization, and focus.
- Single/multiple PRs in the topbar, chat chip, and touch drawer.

## Out of scope

Provider logic, cache changes, polling, sidebar summary redesign, and publication.

## Acceptance

1. All six referenced criteria pass. Delayed updates do not move the frame or
   link by more than one CSS pixel, and clicks at the recorded position succeed.
2. Phone drawers and narrow fine-pointer surfaces keep actions reachable,
   contain overflow, and preserve keyboard and touch access.
3. Existing PR controls and shared consumers retain behavior. Copy passes all
   seven locale checks, including generated Traditional Chinese catalogs.

## UI preview

Use [UI-01 and UI-02](plan.md#ui-preview) for all six criteria.
Desktop: fixed PR tabs and header, scrolling details, fixed refresh row.
Phone: fixed drawer heading and PR action, scrolling details, fixed safe-area footer.
Initial loading, cached refresh, empty results, and errors keep the same geometry.

## Implementation sequence

1. Read the paired requirement/design and scoped frontend guidance. Install
   dependencies if this worktree lacks them: `(cd apps && pnpm install --frozen-lockfile)`.
2. Add the controlled-response geometry regression. Run it against current
   production code and confirm failure from moved link coordinates. Do not use
   a locator click that automatically follows the moved target as the sole proof.
3. Implement the region split and scoped stable sizing. Keep existing PR
   resource hooks. Add component coverage for state transitions and URL identity.
4. Extend phone coverage, run the commands below, and record actual outcomes.
   Promote the paired requirement/design to active/current only after all checks pass.

## Verification

Run from the repository root. Execute browser runs sequentially.
Set `KANDEV_SERVER_HOST=127.0.0.1` when the local configuration binds another
interface. On NixOS, expose the installed Chromium shared libraries through
`LD_LIBRARY_PATH`. These are process-local test settings, not product changes.
The existing checks suite owns state-transition coverage, so no separate
markup-only layout test is necessary.

```bash
(cd apps/web && pnpm exec vitest run components/github/pr-ci-popover.checks.test.tsx components/github/pr-ci-popover.footer.test.tsx components/github/pr-ci-popover.automation.test.tsx components/github/pr-status-refresh-routes.test.tsx components/github/pr-status-chip.test.tsx components/integrations/integration-change-request-status.test.tsx components/integrations/registered-change-request-status.test.tsx)
(cd apps/web && pnpm e2e:run --project chromium tests/pr/pr-status-popover-stability.spec.ts tests/pr/pr-topbar-popover.spec.ts tests/pr/pr-multi-popover.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/pr/mobile-pr-ci-chip.spec.ts)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/github/pr-ci-popover.tsx components/github/pr-status-chip.tsx components/github/pr-topbar-button.tsx components/github/multi-pr-ci-popover.tsx components/integrations/change-request-ci-anatomy.tsx components/integrations/change-request-status-chrome.tsx components/github/pr-ci-popover.checks.test.tsx components/github/pr-status-chip.test.tsx e2e/tests/pr/pr-status-popover-stability.spec.ts e2e/tests/pr/mobile-pr-ci-chip.spec.ts e2e/tests/pr/pr-topbar-popover.spec.ts)
(cd apps/web && pnpm run i18n:check)
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

If additional shared rendering or helper files change, include them and their
existing tests in the exact command record before marking this work order done.
Compare rendered desktop and phone screenshots with the region order in the preview.

## Files likely touched

- `apps/web/components/github/{pr-ci-popover,pr-status-chip,pr-topbar-button,multi-pr-ci-popover}.tsx`
- `apps/web/components/integrations/{change-request-ci-anatomy,change-request-status-chrome}.tsx`
- `apps/web/components/github/pr-ci-popover.checks.test.tsx` and `pr-status-chip.test.tsx`
- `apps/web/e2e/tests/pr/pr-status-popover-stability.spec.ts` (new)
- `apps/web/e2e/tests/pr/mobile-pr-ci-chip.spec.ts`
- `apps/web/src/locales/*/github.json` and generated pseudo catalog if new copy is required

- `docs/public/sessions-and-review.md`

## Dependencies

None. The paired design must be read before implementation.

## Risks

Shared provider defaults, nested mobile scrolling, collision resizing, and
test fixtures whose background feedback warmers complete before the first hover.
Install route controls before navigation and release each response explicitly.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/integrations/requirements/github-pr-status-disclosure.md)
- [Design](../../specs/integrations/system-design/github-pr-status-disclosure.md)
- Existing `pr-topbar-popover.spec.ts`, `pr-multi-popover.spec.ts`, and
  `mobile-pr-ci-chip.spec.ts` fixture and interaction patterns.

## Results

Completed on 2026-11-02.

- RED: the original chip link moved 104 CSS pixels after feedback arrived.
  The label regression also failed before the new Open PR action existed.
- The seven-suite Vitest command above passed: 88 tests. This includes the
  unchanged/default shared provider surfaces and the GitHub opt-in layout.
- Desktop managed E2E passed: 22 tests across the three specified files.
  After adding separately gated summary/feedback refresh and late-author
  coverage, the stability file passed again: 5 tests. These are a subset of
  the 22, not additional distinct scenarios.
- Phone managed E2E passed: 10 tests. The new case checks loading geometry,
  actual touch dimensions, one scroll owner, and opening at the original target.
- Typecheck passed with `NODE_OPTIONS=--max-old-space-size=8192`. The first
  attempt exhausted Node's default 4 GiB heap before completion.
- Targeted ESLint passed with no warnings after extracting the component prop
  types. Localization checks passed for all seven languages and pseudo.
- `node scripts/validate-public-docs.mjs` passed: 54 published pages.
- Specification/catalog, documentation coverage, and whitespace gates passed.
- Desktop loading/loaded screenshots and the phone drawer screenshot were
  inspected. Both match the fixed header/body/footer composition in UI-01/UI-02.

The local host runs used `--host`; subsequent browser runs used `--no-build`
after the successful managed build. NixOS required installed Chromium library
paths and an explicit loopback server bind in the test process environment.
The product configuration was not changed. All browser runs used one worker
and `--retries=0`. No subagents, commit, push, or PR were used.

### Horizontal overflow follow-up

The user accepted the existing layout and requested removal of horizontal
scrolling. Keep the header, footer, drawer height, controls, and vertical
scrolling behavior. Reserve room for switch touch targets inside the details
scroller and fit or wrap phone PR tabs. Extend the existing desktop stability,
phone stability, and phone unlink cases to measure internal horizontal overflow.

RED on 2026-11-03: the phone details body exceeded its client width by 7 CSS
pixels. The two-PR tab row exceeded its client width by 9 CSS pixels at 320px.
The new browser assertions failed on these exact differences.

GREEN: all 10 phone tests and all 5 desktop stability cases passed. After
preserving the PR number separately from the truncated repository name, the
two affected phone cases passed again. The automation suite passed all 16
tests, including tab accessibility and selection. Targeted ESLint, formatting,
specification validation, and whitespace checks passed. The existing layout
and controls remain; no mobile redesign was applied.

### PR review validation

Review follow-up extends the existing phone cases at 320px: Portuguese
refresh and updated-time text must fit the reserved footer, and a long
repository name must retain a reachable PR number, tab selection, and unlink
controls. Both focused cases passed with retries disabled. Targeted ESLint
passed. These checks preserve the accepted presentation; no product behavior
or public documentation change is required.

The fixed desktop height is deliberate, including short and empty results,
as specified in the stable-frame design. Keyboard transfer is covered by the
desktop stability scenarios: tabbing from the link into the scroll body keeps
the disclosure open after the pointer leaves. The shared hover lifecycle
cancels close timers on focus entry and rechecks focus presence before closing.


The next review batch added a 320x480 phone case with twelve linked PRs.
RED: wrapped tabs placed the PR action outside the viewport. GREEN: bound
selection to 8rem or 20dvh with its own vertical scroll region; all 11 phone
cases passed. Matching the visible link label and preserving the text space
between repository and PR number also passed all 20 affected component tests.
The component documentation comment now remains attached to the component.
The plan summary includes both follow-up verification rounds. No ASCII preview
was added because this session requires structured previews; UI-01/UI-02 retain
the approved region tables. Fresh PR screenshots are captured after the fix.

CodeRabbit's summary suggestion is covered by an explicit first-load status
assertion in the existing checks suite. All four checks tests passed.

CodeRabbit's final review identified live tab-count changes as another source
of motion. The short-phone case now opens with two linked PRs, records the
action position, and associates ten more PRs while the drawer remains open.
RED: the action moved 37 CSS pixels. GREEN: the selector reserves the smaller
of 8rem and 20dvh, with rows aligned at the top and excess rows scrolling
inside it. All 11 phone cases passed after this change. Targeted ESLint and
specification validation passed. This preserves the existing frame and action
contract; no new public copy or provider behavior is introduced.

The next CI round exposed two stale assertions in
`mobile-ci-automation-options.spec.ts`: they still measured the outer drawer
wrapper instead of the details scroller. Both failed locally with retries
disabled before updating the selector. The review also identified clipping at
240px viewport height. The compact composition uses full height, hides the
decorative handle, preserves the accessible title, and reserves room for the
close button. The existing phone cases verify single and multiple PRs, the
full action bounds, hit testing, and dismissal at 320x240. Live association
updates now assert both horizontal and vertical action coordinates.

All 17 cases in `mobile-pr-ci-chip.spec.ts` and
`mobile-ci-automation-options.spec.ts` passed. The two compact cases passed
again with footer bounds and at least 44px of scrolling details space.
Shared component tests passed:
seven suites, 88 tests. No localization or public terminology changed.

The queue-follow-up CI case failed on both attempts because its mock agent
completed the active tool before the delay intended to let CI queue a message.
After integrating current main, the existing delay now precedes tool completion.
The focused browser case passed with retries disabled, and the mock-agent
package passed with the race detector. Production queue logic is unchanged.

A further review found insufficient details space just above 320px height.
The existing single-PR and multi-PR cases reproduced that failure. Compact
composition now applies through 480px, with boundary checks at heights 240,
321, 400, 480, and 481. Both horizontal and vertical link bounds must fit
inside the drawer body; the footer and at least 44px of scrolling details
remain visible. The two cases passed after extending the breakpoint.

The queue fixture now accepts an optional release marker through its existing
friendly command. The browser releases active work only after durable queue
admission and the queue chip are confirmed. The existing file-gate helper
provides a bounded wait. A fake-clock unit test proves the follow-up remains
active beyond the queue-mode wait and finishes after release. The browser case
passes without retries, and its explicit retry override was removed. The
marker is also released on test failure. No runtime product contract changed.

Final focused verification passed: 17 phone cases, five desktop stability
cases, the gated queue-delivery case, seven unit suites (88 tests), mock-agent
race tests, typecheck, targeted lint, and specification validation.

The next review identified cutout insets for compact Close positioning.
Browser CDP overrides with nonzero top, right, bottom, and left insets reproduced
the failure in both compact cases. Close and content padding now respect those
insets; both cases and all 17 affected phone tests passed. Five desktop cases
and the gated queue case also passed. The mock unit test now requires a completed
tool update as well as finished text. Removing tool completion made it fail.
Release failure intentionally retains the active tool so a broken fixture cannot
deliver queued work and conceal the failure. A new fake-clock case asserts that
behavior and its visible error; mock-agent race tests passed.

The ambient-environment CI job reached Go's default ten-minute package budget
while a one-second subtest was running. The same package passed on the prior
head in 464 seconds; the local full run completed in 488 seconds, with separate
Git-shim assertion failures on NixOS. The GitLab-facing ambient command now uses
an explicit 25-minute aggregate budget, matching existing backend race-job
budgets. Per-test waits and assertions are unchanged. The exact timeout-site
test passed locally with the race detector.

The final review requested two coverage additions. Compact link bounds now
respect each viewport inset, and the footer bottom must stay above the bottom
inset. The two compact cases passed. A separate fake-clock case leaves the
release marker absent beyond two minutes, then requires the deadline error
and an active tool. It releases the marker during cleanup so a future missing
deadline cannot strand the test bubble. Mock-agent race tests passed. These
are test-only changes; rendered screenshots remain valid.
