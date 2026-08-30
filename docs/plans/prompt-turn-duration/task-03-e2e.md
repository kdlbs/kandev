---
id: "03-e2e"
title: "E2E hover duration spec"
status: done
wave: 3
depends_on: ["02-message-actions-ui"]
plan: "plan.md"
spec: "../../specs/ui/requirements/prompt-turn-duration.md"
---

# Task 03: E2E hover duration spec

- **Acceptance:**
  1. New Playwright spec `apps/web/e2e/tests/task/prompt-turn-duration.spec.ts` verifies: in a settled task session, the persisted user prompt's row is located within the ACTIVE chat, the action row (parent of `message-turn-duration`, obtained e.g. via `duration.locator("xpath=..")`) has computed `opacity: 0` before hover and `opacity: 1` after hover — asserted with `toHaveCSS` (it auto-retries through the `transition-opacity` transition; `toBeVisible`/`not.toBeVisible` are NOT usable because Playwright ignores `opacity: 0`) — and `message-turn-duration`'s text matches `DURATION_SHAPE` (`/^\d+s$|^\d+m \d+s$|^\d+h \d+m \d+s$/`).
  HOVER TARGET (verified against the code): `#msg-<id>` is the FULL-WIDTH outer wrapper (`message-list-native.tsx` `MessageRow`); the `group` class lives on a descendant (`chat-message.tsx` — the `max-w-[85%] ... overflow-hidden group` div) with the right-aligned `user-message-bubble`. Playwright `hover()` centers the pointer on the target, so hovering `#msg-<id>` can land OUTSIDE the bubble and never trigger `group-hover` (opacity stays 0). The hover target MUST be an element inside the `.group` container: `row.getByTestId("user-message-bubble")` (stable testid) or `row.locator(".group").first()` — not the outer `#msg-<id>` itself.
  2. The spec follows `prompt-history-panel.spec.ts` conventions: `createTaskWithAgent` + `DONE_STATES` settle poll, `SessionPage` helpers. BEFORE navigating, capture the seeded prompt's persisted message id by `expect.poll`-ing `apiClient.listSessionMessages(sessionId)` until the seeded user prompt appears (pre-navigation timing from `mobile-prompt-history-panel.spec.ts` — which reads messages ONCE after the settle poll; the explicit `expect.poll` is an ADDED robustness requirement, because a terminal session state can precede message observability and a single read could strand the test with no id). Scope every transcript locator to `session.activeChat()` — `#msg-<id>` exists on every mounted `MessageRow`, and portal-mounted inactive chat panels duplicate rows, so use ``const chat = session.activeChat(); const row = chat.locator(`#msg-${messageId}`)`` and `await expect(row).toHaveCount(1)`, mirroring `prompt-history-panel.spec.ts`. Duration assertion is shape-only (wall clock is uncontrolled), with the same comment explaining why.
  3. A mobile spec `apps/web/e2e/tests/task/mobile-prompt-turn-duration.spec.ts` (mobile-chrome project, Pixel 5; modeled on `mobile-prompt-history-panel.spec.ts`) asserts: at mobile width the settled prompt's `message-turn-duration` renders inside the ACTIVE chat AND the action row (the duration's parent) has computed `opacity: 1` (`toHaveCSS` — below `sm` the row is always visible, base `opacity-100`; Playwright does NOT treat opacity as non-visible, so presence alone proves nothing). It also asserts the duration is not clipped and stays on one line: the duration span has computed `white-space: nowrap`, the ACTION ROW has `scrollWidth <= clientWidth`, and the duration's bounding rect stays within the action row's rect. The outer `#msg-<id>` row check (`scrollWidth <= clientWidth`) stays only as an ADDITIONAL assertion — the user-message bubble and its wrappers are `overflow-hidden` (`chat-message.tsx`), so inner clipping can happen without the outer row overflowing. NOTE: the E2E wall-clock duration is uncontrolled and normally sub-minute, so the mobile spec can only assert the no-wrap STYLING — the deterministic multi-unit (`5m 23s`) no-wrap/geometry proof lives in task-02's component test.
  3a. The mobile spec also resizes the touch-emulated page to 700px wide. At this coarse-pointer width, which is above the CSS `sm` breakpoint, it asserts that the duration's action row remains opaque without hover.
  4. The desktop spec also proves keyboard reveal INDEPENDENTLY of hover: after the hover assertions, move the mouse to a point OUTSIDE the user row (e.g. `page.mouse.move(0, 0)`), assert the action row's computed `opacity` returns to `0` (proving the pointer is no longer over the row), THEN focus an existing action button inside the user row (e.g. the copy button, or Tab into the row) and assert the duration parent's computed `opacity` is `1` (`toHaveCSS`) with `message-turn-duration` present — the row reveals via `focus-within:opacity-100` (`message-actions.tsx`); without the pointer-reset, a focused-while-hovered button would pass purely from `group-hover` and a focus-within regression would go unnoticed.
- **Verification:** (run from the repo root; the backend builds work from any directory via `-C`, then `cd` into `apps/web`)
  ```sh
  make -C apps/backend build && make -C apps/backend e2e-plugin-package && cd apps/web && pnpm run build:e2e && pnpm e2e:raw tests/task/prompt-turn-duration.spec.ts --project=chromium && pnpm e2e:raw tests/task/mobile-prompt-turn-duration.spec.ts --project=mobile-chrome && pnpm run e2e:sleep-ratchet
  ```
  (E2E runs the PREBUILT backend and web bundle: `fixtures/backend.ts` spawns `apps/backend/bin/kandev`, `build:e2e` only rebuilds the Vite bundle, and `global-setup.ts` fails fast when a backend artifact is missing or older than any file under `apps/backend` — so `make -C apps/backend build` + `make -C apps/backend e2e-plugin-package` are MANDATORY prerequisites per `apps/web/e2e/README.md`, alongside `pnpm run build:e2e`. Playwright `testDir` is `e2e/tests`; `--project=chromium` is the desktop project, `--project=mobile-chrome` is Pixel 5. Each worker bootstraps backend+frontend, so allow several minutes. `e2e:sleep-ratchet` must pass on both new specs — no new sleeps.)
- **Files likely touched:**
  - `apps/web/e2e/tests/task/prompt-turn-duration.spec.ts` (new, desktop)
  - `apps/web/e2e/tests/task/mobile-prompt-turn-duration.spec.ts` (new, mobile)
  - Possibly `apps/web/e2e/pages/session-page.ts` if a row-hover helper is needed (prefer inline locators to avoid touching shared pages)
- **Dependencies:** task 02 (UI must render the duration).
- **Parallelism:** sequential.
- **Inputs:**
  - Spec: `docs/specs/ui/requirements/prompt-turn-duration.md` — golden-path scenario (hover reveal).
  - Plan: `docs/plans/prompt-turn-duration/plan.md` — E2E Tests section.
  - Pattern: `apps/web/e2e/tests/task/prompt-history-panel.spec.ts` (seeding, settle polling, sentinel message-id capture, `session.activeChat()` scoping with `toHaveCount(1)`, `DURATION_SHAPE` regex + comment); `activeChat()` is `page.locator("[data-testid='session-chat']:visible").first()` (`apps/web/e2e/pages/session-page.ts`) because background dockview chat panels stay mounted.
  - Mobile: `apps/web/e2e/tests/task/mobile-prompt-history-panel.spec.ts` (mobile-chrome project, Pixel 5, per `apps/web/e2e/playwright.config.ts`); `pnpm run build:e2e` before `pnpm e2e:raw` (per `apps/web/e2e/README.md`).
  - Row locator: the transcript's stable outer row is `#msg-<key>` (`apps/web/components/task/chat/message-list-native.tsx`, `MessageRow`) — always scoped under `session.activeChat()`, never `page`. There is NO `chat-message` test id/class to select — do not invent one. The duration is inside the `MessageActions` row of the user message; the fine-pointer reveal is `opacity`-based and coarse pointers use `opacity-100` at every width, so assertions use computed opacity, not visibility.
- **Output contract:** summary; files changed; test run result; blockers/risks; update task status and `plan.md` checkbox.

## Results

- TDD red: `cd apps/web && pnpm e2e:raw tests/task/prompt-turn-duration.spec.ts --project=chromium` failed as expected when the outer `#msg-<id>` hover target did not activate the descendant `.group` state; the three retry traces are in the Playwright test-results directory.
- Verification: `make fmt`, `make -C apps/backend build`, `make -C apps/backend e2e-plugin-package`, and `cd apps/web && pnpm run build:e2e` passed.
- Verification: `cd apps/web && pnpm e2e:raw tests/task/prompt-turn-duration.spec.ts --project=chromium` passed: 1 test.
- Verification: `cd apps/web && pnpm e2e:raw tests/task/mobile-prompt-turn-duration.spec.ts --project=mobile-chrome` passed: 1 test.
- Verification: `cd apps/web && pnpm run e2e:sleep-ratchet` passed.
- Generated artifacts: production web bundle, backend binaries, plugin E2E package, and failed retry traces. Build outputs are required E2E artifacts; failed traces are retained for the recorded red run.
- Fixup verification: `make -C apps/backend build`, `make -C apps/backend e2e-plugin-package`, and `cd apps/web && pnpm run build:e2e` — passed. The desktop duration E2E and the Pixel 5 mobile duration E2E passed; the mobile spec also verified a 700px coarse-pointer viewport. `pnpm run e2e:sleep-ratchet` passed.
