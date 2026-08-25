---
id: "04-e2e-and-locales"
title: "E2E and locale parity"
status: done
wave: 4
depends_on: ["03-transcript-navigation"]
plan: "plan.md"
spec: "../../specs/ui/requirements/prompt-history-panel.md"
---

# Task 04: E2E and locale parity

## Acceptance

- A desktop E2E spec proves the user flow end to end, in an explicit sequence because opening the history panel activates it (and hides the chat composer — `SessionPage.activeChat()`/`sendMessage` only work while a chat panel is visible): (1) seed the task (description = first prompt); (2) open Prompt history via the "+" menu and MEASURE the `prompt-history-panel` root (`clientWidth` → chars per line ≈ width / 8 px; `clientHeight` → required lines for the 40 % cap at the rendered line height) and generate the payload at ≥ 2× the required lines — a space-separated sentence (repeated words, so it wraps and CANNOT stay on one line) with a unique sentinel token, not a fixed 2,000-character claim; (3) activate the session chat tab (`SessionPage.clickSessionChatTab()`) and send that prompt via `SessionPage.sendMessage`; (4) wait for the turn to settle, then poll `apiClient.listSessionMessages(sessionId)` until a user-authored message containing the unique sentinel exists and capture its `id` (`sendMessage` returns void and the page object exposes no message id — this API poll is the only implementable source of the `#msg-<id>` target); (5) re-activate the Prompt history tab and assert both prompts render newest-first with durations; (6) expand the long prompt and assert a capped, internally scrollable, wrapping box (`scrollHeight > clientHeight`, computed `max-height` ≈ 40 % of the `prompt-history-panel` root's `clientHeight`); (7) TRANSCRIPT-JUMP WITH EXPLICIT PANEL TRANSITIONS — opening history hides the chat and `SessionPage.activeChat()` only resolves a VISIBLE chat, so the jump cannot run while history is active: activate the session chat tab, scroll its `.chat-message-list` viewport away (e.g. to the top), RE-ACTIVATE the Prompt history tab, click the row's arrow, WAIT FOR THE SCROLL TO SETTLE (`scrollIntoView({ behavior: "smooth" })` animates — poll the viewport's `scrollTop` until it stops changing across consecutive samples, mirroring `apps/web/e2e/tests/chat/last-prompt-scroll.spec.ts`), then assert the sentinel-token prompt is visible AND TOP-ALIGNED in the now-active chat container — scope via `SessionPage.activeChat()` (`[data-testid='session-chat']:visible` first-match; portal-mounted session-chat panels duplicate `#msg-<id>` rows), then locate `#msg-${capturedId}` (from step 4) and the `.chat-message-list` viewport within that container; assert TOP ALIGNMENT per the spec, accounting for the transcript's dynamic scroll-margin: `MessageRow` carries `scroll-mt-[calc(4rem+env(safe-area-inset-top))] sm:scroll-mt-[var(--anchored-bar-h,0px)]` (`message-list-native.tsx`), and scrolling away from the last prompt can show the anchored bar, which sets a nonzero `--anchored-bar-h` — correct `scrollIntoView({ block: "start" })` lands the target at viewport top PLUS the computed scroll-margin, not exactly at it. Measure the target row's computed `scroll-margin-top` and assert `row.boundingBox().y − viewport.boundingBox().y` equals that offset within a small documented tolerance (a few px); intersection alone can pass a wrong or partial scroll; exactly one target row. A bare `getByText(sentinel)` or an unscoped `#msg-<id>` locator is insufficient.
- Locale catalogs are consistent: English keys exist, pseudo is regenerated, `i18n:check` passes, and `i18n:ratchet` reports no new hardcoded literals in the changed files.

## Verification

```bash
cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet
make -C ../.. build-web
cd apps/web && pnpm e2e:raw e2e/tests/task/prompt-history-panel.spec.ts --project=chromium
```

(The `make` target lives in the repository-root Makefile only; `apps/web` has no Makefile, so invoke it as `make -C ../.. build-web` from `apps/web` — `../..` is the repo root. The E2E runs against the production Vite build served by the Go backend — build first, or `make -C ../.. test-e2e` which rebuilds both. See `apps/web/e2e/README.md`. The desktop Playwright project is named `chromium`, not `desktop`; `mobile-*.spec.ts` files are picked up by `mobile-chrome`.)

## Files likely touched

- `apps/web/e2e/tests/task/prompt-history-panel.spec.ts` (new)
- `apps/web/e2e/pages/session-page.ts` (only if a helper is missing; `sendMessage`/`clickSessionChatTab`/`activeChat` should already exist — reuse them)
- `apps/web/components/task/prompt-history-panel-content.tsx` (ONLY if a stable test ID is missing — the root/row/expand/expanded-box/arrow IDs are assigned in Task 02 and consumed here)
- Advisory `pt-pt`/`zh-cn` copies of the new keys in `apps/web/src/locales/` (English keys + pseudo regeneration are SOLELY Task 02's, which already runs `i18n:pseudo && i18n:check`; Task 04 only adds English-fallback or translated advisory entries, and its `i18n:check`/`i18n:ratchet` here are repo-wide guards)

## Dependencies

Tasks 01–03 (panel exists end to end).

## Parallelism

Sequential.

## Inputs

- Spec: all scenarios; `Out of scope` (recorded phone product decision mapping the existing `apps/web/e2e/tests/chat/mobile-last-prompt-scroll.spec.ts` as the mobile coverage of the transcript fallback the arrow reuses — only the desktop panel is new UI, so no new `mobile-*.spec.ts`).
- Plan: `E2E Tests`, `Frontend > 4. i18n`.
- Patterns: `apps/web/e2e/tests/settings/todo-list-panel.spec.ts` — seed via `apiClient.createTaskWithAgent` (the description IS the first user prompt), poll `DONE_STATES`, `SessionPage.waitForLoad()/waitForDockviewReady()`, `dockview-add-panel-btn` + `add-panel-prompt-history-item` selectors, `.dv-default-tab` tab matching.

## Risks

- Mock-agent `e2e:message(...)` emits an AGENT text update (`apps/backend/cmd/mock-agent/script.go`), not a user prompt — it cannot seed a second prompt row. The second prompt must be sent through the UI (`SessionPage.sendMessage`) and its turn must settle (reuse the `DONE_STATES` poll against the session state) before asserting two rows.
- Duration rows are NEWEST-FIRST, so `prompt-history-duration-0` is the newly sent long prompt, and the elapsed wall time between seeding and sending is uncontrolled (the seeded prompt's duration is NOT deterministically `0s`) — do NOT assert exact `0s` in the E2E. Instead: locate each row by known content (the sentinel token for the long prompt, the seeded description text for the other) and assert that row's `prompt-history-duration-<index>` element matches the `formatPromptDuration` shape (`^\d+s$|^\d+m \d+s$|^\d+h \d+m \d+s$`); the exact-`0s` boundary stays in fixed-time unit/component coverage, where timestamps are controlled.
- The transcript scroll assertion needs a deterministic target: scroll the transcript away (e.g. to the top) first, then assert the sentinel-token prompt row is in view after the arrow click.
- Internal-scroll assertions must prove overflow, not just styling: assert `scrollHeight > clientHeight` on the expanded box and a computed `max-height` matching 40 % of the `data-testid="prompt-history-panel"` root's `clientHeight`, with stable test ids on the row, expand button, expanded box, AND the panel root. Without the root id the measurement can accidentally target a row or ancestor and couple the test to dockview markup.
- The long prompt MUST be a space-separated sentence: an unbroken string has no wrap opportunities and stays on one horizontally-overflowing line under `whitespace-normal`, so `scrollHeight > clientHeight` would not hold. A fixed length is not a guarantee either: at a wide panel with a tall viewport, even 2,000 characters can wrap to fewer lines than 40 % of the panel height — derive the payload from the measured panel geometry (chars per line × required lines, with a 2× margin) and keep the explicit overflow assertion.
- The transcript-jump assertion must be scoped to the ACTIVE chat container (`SessionPage.activeChat()`), then `#msg-<id>` + `.chat-message-list` within it: portal-mounted inactive chat panels duplicate the rows, so an unscoped locator can strict-match or pass on the wrong instance.
- The spec requires TOP alignment, not mere visibility: assert `#msg-<id>`'s top minus the viewport's top equals the row's computed `scroll-margin-top` within a small documented tolerance. The row's margin is dynamic (`sm:scroll-mt-[var(--anchored-bar-h,0px)]`), so an equality-to-viewport-top check would reject correct behavior whenever the anchored bar is visible; the computed-margin comparison holds for any anchored-bar state.
- The alignment measurement MUST happen after a scroll-settle poll: `scrollIntoView` is smooth-animated, so sampling bounding boxes immediately after the click observes an intermediate position. Poll `scrollTop` until stable (the `last-prompt-scroll.spec.ts` pattern), then measure.
- The step order is load-bearing: the history panel must be opened and measured BEFORE the prompt is sent, then the chat tab must be re-activated for the composer; the panel is then re-activated for assertions. Skipping either activation leaves a hidden composer (send waits/fails) or the wrong active surface for the geometry.
- Step 7's transitions are mandatory: with history active there is NO visible chat (so `activeChat()` cannot resolve and there is nothing to scroll), and with chat active the history row is not clickable — the sequence activate-chat → scroll-away → re-activate-history → click-arrow → assert-active-chat is what makes the jump assertion runnable and deterministic.
- Keep the sentinel token in the sent prompt text unique to that prompt so the transcript-jump locator cannot match the seeded description. The jump assertion must be scoped to the transcript viewport (`#msg-<id>` bounding rect inside `.chat-message-list`): the sentinel also renders in the prompt-history row, so a global visibility check passes even if the transcript never scrolls.

## Output contract

Summary, files changed, exact commands and results (E2E run output or exact blocker), blockers/risks, then mark this task `done` and update its checkbox in `plan.md`.
