---
id: "01-phone-terminal-menu-keys"
title: "Expose phone terminal menu keys"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-CLI-MOBILE-PASSTHROUGH-NAV-001
acceptance_criteria:
  - AC-CLI-MOBILE-PASSTHROUGH-NAV-001.1
  - AC-CLI-MOBILE-PASSTHROUGH-NAV-001.2
  - AC-CLI-MOBILE-PASSTHROUGH-NAV-001.3
  - AC-CLI-MOBILE-PASSTHROUGH-NAV-001.4
  - AC-CLI-MOBILE-PASSTHROUGH-NAV-001.8
  - AC-CLI-MOBILE-PASSTHROUGH-NAV-001.9
  - AC-CLI-MOBILE-PASSTHROUGH-NAV-001.10
  - AC-CLI-MOBILE-PASSTHROUGH-NAV-001.11
system_design:
  - ../../specs/cli/system-design/mobile-passthrough-navigation.md
---

# Task 01: Expose phone terminal menu keys

## Summary

Put navigation and confirmation at the start of the shared phone key bar.
Show one horizontal swipe icon beside the strip. Send Enter to the selected
terminal through its existing input path.

## In scope

- Keep the shared order Esc, Up, Down, Left, Right, Enter, Tab, then Ctrl,
  Shift, Ctrl+C, Ctrl+D, followed by the remaining keys in their current order.
- Add `KEY_SEQUENCES.enter` for carriage return. Use the return glyph and a
  `task:keyEnter` accessible label resolved at render with `t()`. Add the
  label in all supported catalogs and generate the Traditional Chinese pair
  with `pnpm run i18n:zh-hant` from `apps/web`.
- Add one decorative horizontal swipe icon in a narrow gutter beside the
  native scroller. Keep all key targets at least 44 by 44 CSS pixels.
- Update the phone passthrough and Quick Terminal instructions in
  `docs/public/developer-tools.md` with Enter and the swipe cue.

## Out of scope

- Extra controls in desktop or tablet layouts.
- Changes to input routing, cursor modes, or terminal lifecycle.

## Acceptance

- At 375 × 812 CSS pixels, Up, Down, and Enter fit and respond before the
  row scrolls. The visible swipe cue sits beside the strip. Users can scroll
  to reach the rest of the keys.
- Enter sends one carriage return and confirms input in the visible agent PTY,
  task shell, and Quick Terminal. The focus target and modifier behavior use
  each host's existing path.
- The key bar remains inside its host and clears the software keyboard. Desktop
  passthrough uses native keyboard input at 768 CSS pixels and above.

## ASCII UI preview

UI-01. Phone task Chat, connected passthrough with panels closed.
See the [full phone and desktop previews](plan.md#ascii-ui-preview).

```text
Before | [Ctrl] [Shift] [^C] [^D] [Esc] [Tab] [Up] [Do|
After  | [Esc] [Up] [Down] [Left] [Right] [Enter] [Tab] | <-> |
```

The row scrolls inside the viewport beside the fixed swipe icon. Enter's
visible cap uses the return glyph. AC-CLI-MOBILE-PASSTHROUGH-NAV-001.9 and
.10 cover the initial view and swipe cue. Criterion .11 covers the shared bar.

## Verification

Start with these regression edits and run the relevant tests before changing
the shared key bar.

1. In `mobile-terminal-keybar.test.tsx`, add
   `sends carriage return once to the selected terminal on Enter tap`.
   Exercise the default sender and the inline override. The old bar fails
   at the Enter lookup.
2. In `mobile-passthrough-navigation.spec.ts`, extend
   `phone passthrough keys deliver once to the agent without a task-shell request`.
   Set the viewport to 375 × 812 CSS pixels before opening the task. Check
   zero scroll offset and full containment of Up, Down, and Enter before
   any tap. Measure the three targets against the scroll strip. The old
   bar fails on Down and lacks Enter.
3. In the same E2E, use the on-screen Enter to submit the existing raw input.
   Check one byte 13 on the visible session's agent socket and the mock
   agent's response. Keep the checks for shell traffic, user messages, and
   xterm focus. Verify that the swipe cue is visible, a trailing key is
   reachable, and the strip stays inside the document.
4. Use key bar Enter for one existing shell command in each of the task-shell
   and Quick Terminal specs. Verify command output through the existing
   terminal assertions. Keep the native input, modifier, keyboard clearance,
   and desktop breakpoint coverage.

Run from the repository root. A fresh worktree needs
`(cd apps && pnpm install --frozen-lockfile)` before the first pnpm command.

```bash
(cd apps/web && pnpm exec vitest run \
  components/task/mobile/mobile-terminal-keybar.test.tsx \
  components/task/mobile/mobile-agent-terminal.test.tsx \
  lib/terminal/send-pty-input.test.ts)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome \
  e2e/tests/cli-mode/mobile-passthrough-navigation.spec.ts \
  e2e/tests/terminal/mobile-terminal-keybar.spec.ts \
  e2e/tests/terminal/mobile-quick-terminal.spec.ts -- --retries=0)
(cd apps/web && pnpm e2e:run --host --project chromium \
  e2e/tests/cli-mode/passthrough-toolbar.spec.ts -- --retries=0 --grep \
  'narrow fine-pointer layouts retain touch-sized keys|toolbar renders for passthrough sessions')
(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings \
  components/task/mobile/mobile-terminal-keybar.tsx \
  components/task/mobile/mobile-terminal-keybar-helpers.tsx \
  components/task/mobile/mobile-terminal-keybar.test.tsx \
  lib/terminal/key-sequences.ts \
  e2e/tests/cli-mode/mobile-passthrough-navigation.spec.ts \
  e2e/tests/terminal/mobile-terminal-keybar.spec.ts \
  e2e/tests/terminal/mobile-quick-terminal.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Inspect the initial phone view and the keyboard-clearance capture from the
mobile suite. Record physical iOS and Android focus checks when available.

## Files likely touched

- `apps/web/components/task/mobile/mobile-terminal-keybar.tsx`
- `apps/web/components/task/mobile/mobile-terminal-keybar-helpers.tsx`
- `apps/web/lib/terminal/key-sequences.ts`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko}/task.json`
- `apps/web/components/task/mobile/mobile-terminal-keybar.test.tsx`
- `apps/web/e2e/tests/cli-mode/mobile-passthrough-navigation.spec.ts`
- `apps/web/e2e/tests/terminal/mobile-terminal-keybar.spec.ts`
- `apps/web/e2e/tests/terminal/mobile-quick-terminal.spec.ts`
- `docs/public/developer-tools.md`

## Dependencies

None.

## Risks

- The inline agent and Quick Terminal bars have their own senders. Route
  Enter through the same key-tap path as Tab and Esc.
- The icon uses some of the first view's width. Check the three menu keys
  against the strip before the browser scrolls on tap.

## Parallelism

Sequential.

## Inputs

- [Phone navigation requirements](../../specs/cli/requirements/mobile-passthrough-navigation.md).
- [Shared key bar design](../../specs/cli/system-design/mobile-passthrough-navigation.md#shared-key-bar).
- [Issue #5423](https://github.com/kdlbs/kandev/issues/5423).

## Results

- Added the shared priority order, localized Enter key, fixed horizontal swipe
  cue, and public guidance for passthrough and Quick Terminal.
- `pnpm exec vitest run components/task/mobile/mobile-terminal-keybar.test.tsx components/task/mobile/mobile-agent-terminal.test.tsx lib/terminal/send-pty-input.test.ts` passed all 42 tests.
- The planned mobile Playwright command passed all 28 tests. It verified initial
  375 × 812 containment, Enter delivery to the agent PTY, task-shell and Quick
  Terminal submission, keyboard clearance, and reset behavior.
- The planned desktop passthrough command passed both compatibility tests,
  including the 767/768px boundary.
- Review follow-up keeps `px-2 py-1.5` inside the overflow strip. The 375px
  Playwright check confirms 58px bar height, all seven first-view keys, and
  horizontal edge clearance for the first and last keys.
- Focused Oxlint, web typecheck, i18n check, public-doc validation, spec
  validation/lint, and `git diff --check` passed. The managed E2E runner built
  the backend and web frontend successfully.
- Inspected the captured phone and keyboard-open views. Physical iOS and
  Android smoke checks were unavailable in this environment.
