---
id: "04-browser-evidence"
title: "Prove unloaded prompt availability in a browser"
status: done
wave: 4
depends_on:
  - "01-resolve-last-prompt-outside-window"
  - "02-load-unloaded-last-prompt"
  - "03-unloaded-prompt-edge"
plan: "plan.md"
requirements:
  - REQ-UI-PINNED-PROMPT-AVAILABILITY-001
acceptance_criteria:
  - AC-UI-PINNED-PROMPT-AVAILABILITY-001.1
  - AC-UI-PINNED-PROMPT-AVAILABILITY-001.2
  - AC-UI-PINNED-PROMPT-AVAILABILITY-001.3
  - AC-UI-PINNED-PROMPT-AVAILABILITY-001.7
  - AC-UI-PINNED-PROMPT-AVAILABILITY-001.8
system_design:
  - ../../specs/ui/system-design/pinned-prompt-availability.md
---

# Task 04: Prove unloaded prompt availability in a browser

## Summary

Add one desktop and one phone browser scenario in which the last prompt sits
outside the loaded window, proving the control stays available, the anchored bar
presents the prompt on desktop, and one activation loads and aligns the prompt.

## In scope

- One scenario in `apps/web/e2e/tests/chat/last-prompt-scroll.spec.ts`
  (`chromium`, desktop, anchored bar enabled).
- One scenario in `apps/web/e2e/tests/chat/mobile-last-prompt-scroll.spec.ts`
  (`mobile-chrome`, phone, no anchored bar).
- A reload of the task after seeding in both scenarios, so the transcript
  re-fetches its bounded newest window instead of keeping the live-seeded rows.
- Any seed knob the shared helper
  `apps/web/e2e/tests/chat/last-prompt-scroll-helpers.ts` needs to push the last
  prompt past the 100-row initial fetch window.
- One-sentence update in `docs/public/tasks-and-workflows.md` ("Navigate long chat
  transcripts"). Write it so both fail-before phrases below are present verbatim,
  for example: "The control also appears when your latest prompt is no longer part
  of the loaded transcript window, as after a long agent run; selecting it loads
  that part of the transcript and aligns the prompt at the top." The section's
  existing
  statements (appearance once the prompt has left the viewport, direction, hide
  behavior, per-action settings, the anchored bar's desktop-only scope, and the
  phone composition) stay as they are, and no screenshot changes.

Which part of `AC-UI-PINNED-PROMPT-AVAILABILITY-001.7` these passes prove: the
rendered-row clause only. The reloaded transcript holds no prompt row while the
prompt is resolved and the control is offered, so no resolution path inserted one.
The request and pagination-cursor clauses (no older or complete history load, no
`hasMore`/`oldestCursor` change as a side effect of resolving) are proved by Task
01's component and store suites, which assert exactly that; this work order does
not restate them as browser evidence.

## Out of scope

- Changing the production behavior these scenarios verify.
- Other transcript navigation, pagination, or pinned-prompt rendering scenarios.
- The containers, routing, auth, and kubernetes projects.
- Restructuring or rewriting the public page beyond that one sentence.
- A failure-then-retry browser scenario. The transcript has no projection-retry
  surface by design (AC .6 accepts the connected-state dead end, and this package
  adds no new user-facing copy), so there is no affordance for such a scenario to
  exercise; the read's retry behavior is component evidence in Task 01.

## Acceptance

- Desktop: after seeding `trailingFillerCount: 120` (the helper's default 50 stays
  inside the initial 100-row fetch) and reloading the task
  (`testPage.reload()`, `waitForLoad`, `waitForChatIdle`), the transcript renders
  no prompt row, the control in the chat status bar is visible with the upward
  direction, the anchored bar shows the prompt's stored text, one activation of
  the bar's own button renders the prompt row at its scroll-margin-adjusted start
  position relative to the transcript scrollport, and a second reload repeats the
  unloaded state so the status-bar control's activation lands the same way, all
  within the tolerance used by
  `apps/web/e2e/tests/task/prompt-history-panel.spec.ts`. The two passes are
  required because the first successful jump makes the prompt visible, which closes
  the bar and hides the status-bar control, so neither affordance can be exercised
  twice in one state. Both affordances call the panel's single
  `scrollToLastPrompt` handler, which is where the local target is created, so the
  two passes exercise the same mechanism. Assertions are web-first
  and retrying (`toBeVisible`, `toHaveAttribute("data-state", "open")`,
  `toHaveCount`), and locators are scoped: the control through
  `chat.getByTestId("chat-status-bar")`, the bar's own button through
  `anchored-last-prompt-bar`, because both surfaces carry the same test id while
  the bar is enabled. The prompt row itself is located through its transcript row
  (`chat.locator("[id^='msg-']")`) so the filter cannot match the anchored bar's
  own copy of the same text, which is the duplicate on screen; the sibling
  scenarios in that file keep their existing `getByText(marker, { exact: false }).first()`
  form unchanged. The upward
  direction is read from the button's icon class, and the bar's own scroll button
  is asserted in addition to the status-bar control.
- Phone: the scenario enables `show_anchored_prompt_bar` before seeding and
  restores it in `afterEach`, exactly as the existing mobile last-prompt scenario
  does, so the "no anchored bar" assertion proves the mobile composition rather
  than a disabled setting. The same seeded and reloaded session then shows no
  anchored bar, the control
  is visible, and the persisted prompt row is absent before activation (resolve
  `<id>` as below, then assert the scoped `#msg-<id>` row has count 0), so the
  scenario cannot pass through the already-loaded path. One activation then reaches
  the prompt row at the same
  scroll-margin-adjusted start position the desktop pass asserts. The row lookup
  follows `apps/web/e2e/tests/task/mobile-prompt-history-panel.spec.ts`:
  resolve the row through `new SessionPage(testPage).activeChat().locator("#msg-<id>")`,
  then wait for it to be attached. The measurement follows the desktop
  `apps/web/e2e/tests/task/prompt-history-panel.spec.ts`: settle causally first —
  poll the closest `.chat-message-list` `scrollTop` until it is unchanged across
  two consecutive reads, because an immediate measurement can observe a
  transient position while the transcript's verifier force-lands a smooth scroll
  over a bounded frame window — and then assert
  `Math.abs(rowTop - listTop - scrollMarginTop) <= 2` against that element. (The
  mobile spec's own measurement reads the row offset twice one animation frame
  apart and asserts a range rather than that formula; this scenario asserts the
  desktop formula so both surfaces prove the same landed position.) The scenario
  obtains `<id>` by listing the session's
  persisted messages after seeding and selecting the user message whose content is
  `LAST_PROMPT_MARKER` (the seed helper exposes only the session id today). The
  in-flight `transcript-jump-loading` portion of
  `AC-UI-PINNED-PROMPT-AVAILABILITY-001.2` is evidenced by the component test
  rather than by holding the phone's around response.
- Both scenarios fail against the pre-change behavior because no last-prompt
  affordance is present once the reloaded window holds no user row; without the
  reload they would pass before the change, since the live-seeded rows keep the
  prompt in the cache.
- Each desktop pass also asserts that the newest seeded row is still present after
  the jump, so a merge that replaced the newest window instead of unioning it cannot
  pass. The locator is the scoped transcript row for `filler message <trailingFillerCount>`
  (the helper writes the trailing fillers with exactly that content via
  `seedAgentMessages`, and a plain `message` row is never folded into a
  `turn_group`), asserted with a row count of 1; the equivalent `#msg-<id>` row from
  the persisted-message listing below is also acceptable. The 120-row seed stays below
  the span at which a middle region would remain unloaded (the retained newest window
  plus one around window), so that case has no browser host by construction: `.7`'s
  non-contiguous clause belongs to `load-message-window.test.ts`, which asserts the
  gap directly against the loader.
- The browser passes evidence `AC-UI-PINNED-PROMPT-AVAILABILITY-001.7`'s
  rendered-row clause and nothing more: the reloaded transcript holds no prompt row,
  which is what a resolution path that inserted one would violate. No browser
  assertion observes request or pagination state, so `.7`'s no-history-load and
  cursor clauses stay with Task 01's component and store evidence.
- Public docs: `docs/public/tasks-and-workflows.md` describes availability for the
  known prompt outside the loaded window in the same PR. Two fail-before commands
  in Verification assert both phrases of the new sentence, so the docs work cannot
  pass by leaving the page unchanged; a third command guards the section heading
  against a rewrite, and the copy-agnostic generic validators run alongside them.

## ASCII UI preview

Both scenarios verify the views in
[plan.md](plan.md#ui-01-desktop-transcript-last-prompt-outside-the-loaded-window):
`UI-01` for desktop and `UI-02` for phone. No new view is introduced by this
work order.

`UI-01: Desktop transcript, last prompt outside the loaded window` (post-change
state, both passes in sequence; ACs `AC-UI-PINNED-PROMPT-AVAILABILITY-001.1`,
`.2`, `.3`, `.7`):

```text
+---------------------+--------------------------------------------+----------+
| sidebar             | [chat] [Prompt history] [Changes]          | right    |  fixed
|                     +--------------------------------------------+          |
|                     | [^] LAST-PROMPT-MARKER-9F2Q please handle  | <- asserted
|                     |     this scrolled-past regression ... [v]  |    bar state
|                     +--------------------------------------------+          |
|                     |  ^ last prompt row                         |  scrolling
|                     |    (asserted at scroll-margin-top)         |  region
|                     +--------------------------------------------+          |
|                     |  [ composer ]  [^ Scroll to last prompt]   | <- asserted
+---------------------+--------------------------------------------+----------+
```

`UI-02: Phone transcript, last prompt outside the loaded window` (post-change
state, single composition; ACs `AC-UI-PINNED-PROMPT-AVAILABILITY-001.1`, `.2`,
`.8`):

```text
+----------------------------------+
| < Task            [ status bar ] |  fixed: asserted bar absence here
+----------------------------------+
| tool call: read file ...         |  scrolling: .chat-message-list
| filler message 120 ...           |  (the settle poll reads this element)
| (no user prompt row is loaded)   |
+----------------------------------+
| (no anchored bar on phone)       |  fixed: composer row
| [ composer ]   [^ last prompt]   |  <- asserted control, then landed row
+----------------------------------+
```

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
make -C apps/backend build
(cd apps/web && pnpm run build:e2e)
make -C apps/backend e2e-plugin-ui
make -C apps/backend e2e-plugin-package
(cd apps/web && pnpm e2e:raw tests/chat/last-prompt-scroll.spec.ts --project=chromium)
(cd apps/web && pnpm e2e:raw tests/chat/mobile-last-prompt-scroll.spec.ts --project=mobile-chrome)
rg -n "Scroll to last prompt" docs/public docs/specs docs/decisions
# Fail-before docs assertions: the two phrase checks fail on the unchanged page and
# pass only once the required sentence is present (wording must match the sentence
# this work order adds). The heading check is not fail-before evidence: it guards
# against a rewrite that drops the section the sentence belongs to.
rg -q "no longer part of the loaded transcript window" docs/public/tasks-and-workflows.md
rg -q "loads that part of the transcript and aligns the prompt at the top" docs/public/tasks-and-workflows.md
rg -q "^### Navigate long chat transcripts$" docs/public/tasks-and-workflows.md
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

The generic public-doc validators stay copy-agnostic: they check publishing, links,
and structure, not this feature's wording, so they cannot prove the sentence
exists. No permanent test file is added for one sentence, because a copy assertion
in `scripts/validate-public-docs.test.mjs` would pin prose that a later editor may
legitimately reword; the two phrase assertions above are this work order's
fail-before docs verification and the heading check is its rewrite guard.

## Files likely touched

- `apps/web/e2e/tests/chat/last-prompt-scroll.spec.ts`
- `apps/web/e2e/tests/chat/mobile-last-prompt-scroll.spec.ts`
- `apps/web/e2e/tests/chat/last-prompt-scroll-helpers.ts`
- `docs/public/tasks-and-workflows.md` (one sentence in "Navigate long chat
  transcripts")

## Dependencies

- Tasks 01 to 03 supply the behavior under test.

## Risks

- Seeding rows beyond the initial fetch window is what creates the defect state,
  but only after a reload: seeded rows arrive over the live socket and stay in
  `messages.bySession`, so without `testPage.reload()` the prompt remains cached
  and rendered. With the helper's default trailing count the prompt also stays
  inside the 100-row window, so the count must be an explicit literal at or above
  the window size.
- The mock agent must stay idle while rows are seeded, or live rows can push the
  window during assertion.
- Desktop assertions must enable `show_anchored_prompt_bar` and restore it in
  `afterEach`, as the existing scenarios in that file do.
- Alignment must be asserted against the row's computed `scroll-margin-top`
  (`calc(4rem + safe-area-inset-top)` on phone, `var(--anchored-bar-h)` on
  desktop), not against zero, and the phone pass must assert it too: a phone
  implementation that lands the row merely somewhere in view would otherwise pass
  while violating `AC-UI-PINNED-PROMPT-AVAILABILITY-001.2`.
- The public-doc sentence must stay additive: the section's existing statements
  (appearance once the prompt has left the viewport, direction, hide behavior,
  per-action settings, the desktop-only bar) remain true after this change, so
  rewording them would misdescribe today's behavior. `docs/public/tasks-and-workflows.md`
  is in the same PR as the behavior change, and both public-doc validators run.

## Parallelism

`sequential`

## Inputs

- `apps/web/e2e/tests/chat/last-prompt-scroll.spec.ts`,
  `apps/web/e2e/tests/chat/mobile-last-prompt-scroll.spec.ts`,
  `apps/web/e2e/tests/chat/last-prompt-scroll-helpers.ts`.
- `apps/web/e2e/README.md` for prerequisites, projects, and commands.
- `apps/web/e2e/tests/chat/inactive-session-transcript-reconciliation.spec.ts`
  for seeding history beyond the newest page.

## Results

Verified: desktop prompt E2E passed 12/12, phone prompt E2E passed 2/2,
capture scenarios passed 1/1 per viewport, and public-doc validation passed
(62 tests, 47 published pages).
