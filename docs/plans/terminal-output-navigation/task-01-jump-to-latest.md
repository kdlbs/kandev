---
id: "01-jump-to-latest"
title: "Jump to latest terminal output"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-TERMINAL-OUTPUT-NAVIGATION-001
acceptance_criteria:
  - AC-UI-TERMINAL-OUTPUT-NAVIGATION-001.1
  - AC-UI-TERMINAL-OUTPUT-NAVIGATION-001.2
  - AC-UI-TERMINAL-OUTPUT-NAVIGATION-001.3
  - AC-UI-TERMINAL-OUTPUT-NAVIGATION-001.4
  - AC-UI-TERMINAL-OUTPUT-NAVIGATION-001.5
system_design:
  - ../../specs/ui/system-design/terminal-output-navigation.md
---

# Task 01: Jump to latest output

## Summary

Implement the scoped, accessible xterm action and preserve reading position as output streams. Demonstrate the actual desktop task shell, dev-server panel and phone task shell with isolated real fixtures.

## In scope

- Track the active normal xterm buffer through scroll, parsed writes, resize and buffer changes using the hosts' existing readiness, refs and event lifetimes; clean up subscriptions on replacement and unmount. Avoid speculative lifecycle infrastructure. Gate availability on task host, readiness, visibility and overlays. Scroll only on explicit activation.
- Wire desktop task shell, bottom terminal, task right-panel shell and dev-server hosts, and `MobileTerminalPane`; avoid enabling unrelated shared-renderer consumers. Add localized accessible copy in English, pt-pt, zh-cn, zh-hk, zh-tw, ja and ko; generate zh-hant catalogs via the existing script.
- Add narrow RED/GREEN coverage and real routed browser flows; update the integrated-terminal how-to in `docs/public/developer-tools.md` with the new action and scope.

## Out of scope

Agent TUI, Quick Terminal, login, new phone dev-server navigation, forced following, clearing, input bytes, connection, output retention and process lifecycle changes. No broad local QA, review, or verification gate.

## Acceptance

1. The action appears only above the latest normal-buffer output on eligible unobstructed usable hosts; click or keyboard activation reaches the bottom and hides it without sending input or unconditionally redirecting focus to the terminal. Alternate-buffer, readiness, overlays and excluded callers stay safe.
2. Appended output preserves a scrolled-up view subject to normal eviction, bottom follow remains normal, and current-instance subscriptions clean up on replacement and unmount.
3. Desktop task shell and dev-server and phone task shell pass real xterm tests, including phone 44px hitbox, containment, safe area, existing key row and stable document scroll; all locales and public how-to reflect the action.

## UI composition

See [plan composition](plan.md#ui-composition), AC-UI-TERMINAL-OUTPUT-NAVIGATION-001.1 through .5. Place the visible action inside the desktop terminal and phone task-shell bounds, above the existing phone key row and clear of the keyboard and safe area. Xterm owns scrolling; phone targets are at least 44px in both dimensions. Hide the action at bottom, in alternate buffer and behind search or status overlays. Support keyboard activation without unconditionally refocusing the terminal.

## Implementation and test instructions

Read the linked design, `apps/web/AGENTS.md`, `/tdd`, `/e2e` and `/mobile-parity` before implementation. First write `components/task/terminal-output-navigation.test.tsx` against the existing rendered `PassthroughTerminal` and `ShellTerminal` hosts, controlling their xterm state and existing dependencies without replacing the hosts. Set the normal viewport above bottom and assert the accessible action exists: its absence on the current host must be the observed RED failure. Missing imports, an unresolved new hook or other setup failures do not count. Then implement and record GREEN. Cover hidden at bottom; visible above; click and keyboard activation call the current terminal's scroll handler, hide the action and send no input; output while above preserves the viewport and remains available; normal follow at bottom; resize, search/status gating, alternate-buffer entry/exit, replacement and listener disposal. Assert focus is not unconditionally moved to the terminal or stolen from another control. Test scope gates including agent and utility surfaces. Use real xterm browser observations for continuity and actual event ordering.

For desktop, create an isolated task and shell with real backend, produce distinct numbered lines beyond one viewport, wheel up, append a distinct marker through a controlled real process, assert viewport/content continuity, click and observe newest marker and hidden control. Start a real isolated repository dev script with enough output and a controlled later marker, open the actual dev-server panel and repeat. Capture shell input or use an input sentinel to show the button causes no bytes. Clean up only fixture-owned process and repository settings. For phone, navigate to Terminal, use trusted touch on the actual xterm, then tap the real button and assert the same result, target dimensions, bounds, safe area, key row separation, and unchanged document scroll. Inspect the rendered phone viewport. Do not mock transport or mount an artificial routed surface.

## Likely owned files and dependencies

- `apps/web/components/task/passthrough-terminal.tsx`, `shell-terminal.tsx`, a shared viewport hook/button and focused tests under `components/task/`; task host callers `terminal-panel.tsx`, `bottom-terminal-panel.tsx`, `task-right-panel-tab-contents.tsx`, `mobile/mobile-terminal-pane.tsx` if explicit opt-in is needed.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko}/task.json`, generated Traditional Chinese, and `docs/public/developer-tools.md`.
- New desktop and mobile Playwright specs named below; reuse `e2e/fixtures/test-base.ts`, `mobile-terminal-helpers.ts`, `terminal-buffer-reader.ts`, and `session/dev-server-preview-toggle.spec.ts` patterns. Extend test-only observation only if necessary; do not fake xterm or the route.

## Verification

After ROOT grants HEAVY, use the pinned toolchain below. Install once in a fresh worktree with `pnpm --dir apps install --frozen-lockfile` if needed. Use unique task-owned log directories for each invocation; preserve all process handles and join them to actual exit. Run from repository root in Bash, sequentially. Run the first host test after authoring the test but before production changes, then run GREEN after implementation. The E2E CLI specifies one worker and zero retries; inspect the new specs and any reused `describe.configure` settings for nonzero overrides and remove or override those only if present. The existing dev-server example has a `retries: 1` describe override; do not copy it into a new spec.

```bash
export PATH="/home/jcfs/.local/share/mise/installs/pnpm/9.15.9:/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:$PATH"
export GOTOOLCHAIN=local
KANDEV_RUN_QUIET_DIR=.kandev-run-quiet/terminal-output-navigation/unit-red scripts/run-quiet frontend-test -- pnpm --dir apps/web exec vitest run --maxWorkers=1 --no-file-parallelism components/task/terminal-output-navigation.test.tsx
KANDEV_RUN_QUIET_DIR=.kandev-run-quiet/terminal-output-navigation/unit-green scripts/run-quiet frontend-test -- pnpm --dir apps/web exec vitest run --maxWorkers=1 --no-file-parallelism components/task/terminal-output-navigation.test.tsx
pnpm --dir apps/web run i18n:zh-hant
KANDEV_RUN_QUIET_DIR=.kandev-run-quiet/terminal-output-navigation/i18n scripts/run-quiet frontend-test -- pnpm --dir apps/web run i18n:check
mkdir -p .kandev-run-quiet/terminal-output-navigation/eslint-paths
git -C apps/web diff --relative --name-only -z --diff-filter=ACMR HEAD -- '*.ts' '*.tsx' > .kandev-run-quiet/terminal-output-navigation/eslint-paths/changed
git -C apps/web ls-files --others --exclude-standard -z -- '*.ts' '*.tsx' > .kandev-run-quiet/terminal-output-navigation/eslint-paths/untracked
mapfile -d '' -t web_lint_files < .kandev-run-quiet/terminal-output-navigation/eslint-paths/changed
mapfile -d '' -t -O "${#web_lint_files[@]}" web_lint_files < .kandev-run-quiet/terminal-output-navigation/eslint-paths/untracked
test "${#web_lint_files[@]}" -gt 0
printf '%s\n' "${web_lint_files[@]}"
KANDEV_RUN_QUIET_DIR=.kandev-run-quiet/terminal-output-navigation/eslint scripts/run-quiet frontend-test -- pnpm --dir apps/web exec eslint "${web_lint_files[@]}"
GOMAXPROCS=2 GOFLAGS=-p=2 KANDEV_RUN_QUIET_DIR=.kandev-run-quiet/terminal-output-navigation/desktop scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/terminal/terminal-jump-to-latest.spec.ts -- --workers=1 --retries=0
GOMAXPROCS=2 GOFLAGS=-p=2 KANDEV_RUN_QUIET_DIR=.kandev-run-quiet/terminal-output-navigation/phone scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/terminal/mobile-terminal-jump-to-latest.spec.ts -- --workers=1 --retries=0
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

The ESLint command selects actual changed and untracked web TypeScript paths, including any host callers and tests. Inspect the printed list before running ESLint; if it includes others' changes, remove those entries from the array and lint only this task's changed paths. Check each Git command succeeds before consuming its NUL-delimited output. Run full typecheck only if a narrow change leaves an unresolved type issue or CI requires it; do not replay the full i18n ratchet or spec-lint unit suite as extra pre-PR gates. Record exact commands, behavioral RED failure, GREEN results, browser counts, rendered phone inspection and limitations here. Normal hooks apply; no bypass. Return HEAVY before hosted validation.

## Results

- RED: the existing `ShellTerminal` host rendered but lacked the accessible button; `vitest run --maxWorkers=1 --no-file-parallelism components/task/terminal-output-navigation.test.tsx` exited 1 on that assertion. The final host test run passed 4 tests with the same worker options.
- Desktop shell: `e2e:run --host --shards 1 --project chromium tests/terminal/terminal-jump-to-latest.spec.ts -- --workers=1 --retries=0` passed the task-shell case in the rebuilt run; that run's dev-server case failed with the earlier timed fixture. The phone shell passed `e2e:raw --project=mobile-chrome tests/terminal/mobile-terminal-jump-to-latest.spec.ts --workers=1 --retries=0` (1 test).
- Desktop dev server: after the separate correlated diagnostic passed, `e2e:run --host --no-build --shards 1 --project chromium tests/terminal/terminal-jump-to-latest.spec.ts -- --grep 'dev-server output' --workers=1 --retries=0` passed (1 test). The no-build input/tool/artifact receipt matched. Its retained evidence records viewport 185 before and after the matched later output, viewport 190 after activation, and zero input frames before and after activation. The gated initial batch follows the actual snapshot; this does not establish the exact cause of earlier timed-fixture failures.
- Scoped ESLint passed for the changed web paths and again for the revised E2E files. `pnpm --dir apps/web run i18n:check` passed after regenerating the pseudo and Traditional Chinese catalogs. Desktop and phone captures from the final UI were inspected and their hashes verified.
- Public documentation validation passed with `node --test scripts/validate-public-docs.test.mjs` and `node scripts/validate-public-docs.mjs`.
- Command logs and the retained dev-server evidence are under `/home/jcfs/.kandev/loop-evidence/61dadc35-4a42-4581-8c94-62309c160fd9/child036/`. Normal commit hooks provide the catalog and specification validation results.
