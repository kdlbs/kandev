---
id: "01-mobile-shell-arrows"
title: "Restore mobile shell arrow navigation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-MOBILE-SHELL-ARROWS-001
acceptance_criteria:
  - AC-UI-MOBILE-SHELL-ARROWS-001.1
  - AC-UI-MOBILE-SHELL-ARROWS-001.2
  - AC-UI-MOBILE-SHELL-ARROWS-001.3
  - AC-UI-MOBILE-SHELL-ARROWS-001.4
  - AC-UI-MOBILE-SHELL-ARROWS-001.5
system_design:
  - ../../specs/ui/system-design/mobile-shell-arrow-navigation.md
---

# Task 01: Restore mobile shell arrow navigation

## Summary

Use the active dedicated shell xterm's live cursor mode for phone keybar arrows while keeping native input and existing routing intact. Independently prove the real user outcome against a PTY application before and after the change.

## In scope

- Implement the design's active sender/mode ownership and keybar-only arrow resolution for all four directions, preserving fallback and modifier semantics.
- Add focused unit/component checks, independent real PTY phone RED/GREEN, normal/native controls, rendered geometry and desktop native-keyboard evidence.
- Correct stale sender comments where touched and assess public docs; no new public copy is needed for the established arrow row.

## Out of scope

Agent and Quick Terminal behavior changes, new arrow modifiers, full-screen Home/End, backend APIs, layout and styling changes, new dependencies, and broad refactoring.

## Acceptance

- Each phone arrow is recognized once by a real curses application in application mode and by a real PTY consumer in normal mode; switching mode uses the new current mode. Establish defect-specific pre-fix RED after program readiness, and native ArrowUp positive control.
- The active terminal and session alone receive the key; stale release, unavailable sender, throwing sender, and latched/sticky modifiers preserve their existing semantics. Native xterm data, text, paste, supported Ctrl-letter and Shift-Tab remain unchanged.
- Phone controls remain tappable with at least 44 by 44 CSS-pixel hit areas and contained scrolling without document overflow; a desktop native-keyboard application-mode control succeeds.

## Verification

Run only after a later separate implementation instruction and an explicitly granted local heavy lease. Set PATH with `/home/jcfs/.local/share/mise/installs/pnpm/9.15.9`, `/home/jcfs/.local/share/mise/installs/node/24.21.0/bin`, and `/home/jcfs/.local/share/mise/installs/go/1.26.0/bin` in that order before the existing PATH. Install once from `apps/` with `pnpm install --frozen-lockfile` after the lease. Record all command handles, actual joins, exit codes, test counts, and any owned fixture cleanup. Do not run simultaneous heavy commands. Stop for resource/auth/unknown/out-of-scope/OOM or unchanged retry decisions.

Use these stable test title prefixes when authoring the tests: `application cursor keybar`, `normal cursor keybar`, and `desktop application cursor keyboard`. Give each application arrow and the native ArrowUp control its own case under the application prefix, so a failure cannot stop the remaining cases. Both existing mobile and desktop describes configure `retries: 1`: place new focused cases in independent or narrower describes explicitly configured with `retries: 0`, preserving the unrelated suite settings. Also pass `--retries=0` and verify the actual report shows zero retries; do not assume the CLI flag overrides a suite setting. The normal control must exercise the browser, real terminal and PTY consumer, not solely a synthetic byte-comparison stub. List each project first and require discovery of all four application cases, the native control, the normal control and the desktop control. Verify READY before judging expected vs observed key. Record the actual defect-specific failures and control PASS. Run the commands sequentially, preserve each native process handle, and record actual JOIN, exit, counts, and the unique log path. `run-e2e.sh` forwards arguments after its `--` to Playwright and supplies one worker.

From the repository root, create a task-owned unique log directory once, then list the intended projects and run the RED before any production change:

```bash
export KANDEV_RUN_QUIET_DIR="$(mktemp -d /tmp/kandev-child023-arrows.XXXXXXXX)"
(cd apps/web && pnpm e2e:raw --project=mobile-chrome tests/terminal/mobile-terminal-keybar.spec.ts --list --grep 'application cursor keybar|normal cursor keybar')
(cd apps/web && pnpm e2e:raw --project=chromium tests/terminal/terminal-keyboard.spec.ts --list --grep 'desktop application cursor keyboard')
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/terminal/mobile-terminal-keybar.spec.ts -- --grep 'application cursor keybar' --retries=0
```

After the fix, the managed GREEN runs rebuild production web/backend and use the same real consumers. Keep unit projects separate; require nonzero collected tests in each.

```bash
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/terminal/mobile-terminal-keybar.spec.ts -- --grep 'application cursor keybar|normal cursor keybar' --retries=0
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/terminal/terminal-keyboard.spec.ts -- --grep 'desktop application cursor keyboard' --retries=0
(cd apps/web && pnpm exec vitest run --project=node --maxWorkers=1 lib/terminal/key-sequences.test.ts lib/terminal/mobile-active-terminal.test.ts)
(cd apps/web && pnpm exec vitest run --project=browser --maxWorkers=1 lib/terminal/send-shell-input.test.ts components/task/mobile/mobile-terminal-keybar.test.tsx)
```

Before the staged-content ratchets, inspect the actual diff and index for user/peer edits. Stage all owned changed TS/TSX and helper files, and the four documents, using explicit paths only. The following is an anticipated path inventory, not authority to stage unchanged, nonexistent, or others' files: select only actual owned changed paths after inspection, add any newly authored owned helper by its exact path, and stage only owned hunks if a file contains others' edits. Confirm `git diff --cached --name-only` matches the reviewed ownership inventory. Then lint every staged owned changed web TS/TSX file and run both ratchets. Repeat staging and changed-input validation after any further edit.

```bash
git add -- docs/specs/ui/requirements/mobile-shell-arrow-navigation.md docs/specs/ui/system-design/mobile-shell-arrow-navigation.md docs/plans/mobile-shell-application-cursors/plan.md docs/plans/mobile-shell-application-cursors/task-01-mobile-shell-arrows.md apps/web/lib/terminal/key-sequences.ts apps/web/lib/terminal/key-sequences.test.ts apps/web/lib/terminal/mobile-active-terminal.ts apps/web/lib/terminal/mobile-active-terminal.test.ts apps/web/lib/terminal/send-shell-input.ts apps/web/lib/terminal/send-shell-input.test.ts apps/web/components/task/mobile/mobile-terminal-keybar.tsx apps/web/components/task/mobile/mobile-terminal-keybar-helpers.tsx apps/web/components/task/mobile/mobile-terminal-keybar.test.tsx apps/web/components/task/mobile/mobile-terminal-pane.tsx apps/web/e2e/tests/terminal/mobile-terminal-keybar.spec.ts apps/web/e2e/tests/terminal/terminal-keyboard.spec.ts apps/web/e2e/tests/terminal/mobile-terminal-helpers.ts apps/web/e2e/tests/terminal/terminal-test-helpers.ts
git diff --cached --name-only
```

After confirming all staged web paths are owned, this command builds an ESLint array from the staged changed TS/TSX paths, including helpers, without pulling in unrelated terminal tests. Run these gates from the repository root:

```bash
bash <<'BASH'
web_files=()
while IFS= read -r -d '' path; do
  case "$path" in
    *.ts|*.tsx) web_files+=("${path#apps/web/}") ;;
  esac
done < <(git diff --cached --name-only -z --diff-filter=ACMR -- apps/web)
(( ${#web_files[@]} > 0 )) || exit 1
(cd apps/web && pnpm exec eslint --max-warnings 0 "${web_files[@]}")
BASH
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm run e2e:sleep-ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans/mobile-shell-application-cursors
git diff --cached --check
git status --short -- docs/plans/mobile-shell-application-cursors
```

Canonical web typecheck requires an explicit memory budget based on fresh available memory and the prior 4 GiB OOM / 6 GiB success evidence; obtain the approved cap before running it. Record the exact command, budget, actual result, and any blocker. Do not blindly replay an OOM. Run the repository's PR-documentation coverage preflight when available. A physical-device OS-keyboard check is not implied by mobile emulation.

## Files likely touched

- `apps/web/lib/terminal/key-sequences.ts` and its test; `mobile-active-terminal.ts` and its test; `send-shell-input.ts` and its test.
- `apps/web/components/task/mobile/mobile-terminal-keybar.tsx`, `mobile-terminal-keybar-helpers.tsx`, `mobile-terminal-keybar.test.tsx`, and `mobile-terminal-pane.tsx` as needed.
- `apps/web/e2e/tests/terminal/mobile-terminal-keybar.spec.ts`, `terminal-keyboard.spec.ts`, and shared test helpers only when warranted.
- This package and the linked requirement/design pair, with statuses and actual results updated at completion.

## Dependencies

None. ROOT must fully read the four artifacts and issue a later separate implementation instruction; the local heavy lease must be granted before install or heavy checks.

## Risks

Mode and sender can become detached during terminal switching or reconnect. Rewriting generic `onData` would corrupt native keyboard or pasted escape bytes. A raw-frame-only assertion is not a user-outcome oracle.

## Parallelism

Sequential.

## Inputs

- [Requirements](../../specs/ui/requirements/mobile-shell-arrow-navigation.md) and [system design](../../specs/ui/system-design/mobile-shell-arrow-navigation.md).
- Existing phone and desktop terminal tests and helpers; public mobile terminal documentation.

## Results

- Fresh managed phone RED: four separate curses consumers reached READY and reported `^[` rather than `KEY_UP`, `KEY_DOWN`, `KEY_LEFT`, or `KEY_RIGHT`; native ArrowUp control passed (five cases, one worker, zero retries).
- Fresh managed phone GREEN using the listed mobile command: 6 passed, including all four application arrows, native control, normal shell editing after application mode, and phone hit-target/overflow assertions. Fresh managed desktop command: 1 passed. Both used one worker and zero retries.
- An existing history test expected CSI without establishing cursor mode. A separate real-shell diagnostic observed `ESC O A` and confirmed that Up recalled and ran the modified command; the corrected test checks that user outcome instead of assuming a mode. Its focused fresh managed run passed one case with one worker and zero retries.
- CI's interactive `/bin/sh` lacked the line editing assumed by the two shell-outcome cases. Both now start an explicit interactive Bash with deterministic readline and history settings. A focused managed run entering `/bin/sh` first verified Bash readiness, all four normal CSI arrows, the curses control, and both real shell outcomes: 2 passed with one worker and zero retries.
- Listed node Vitest command: 2 files / 7 tests passed. Listed browser Vitest command: 2 files / 38 tests passed. All changed web inputs passed scoped ESLint. Canonical `NODE_OPTIONS=--max-old-space-size=6144 pnpm run typecheck` passed.
- Staged `pnpm run i18n:ratchet` and `pnpm run e2e:sleep-ratchet` passed. `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.test.py` (36 tests), `python3 scripts/lint-spec-files.py --all`, staged diff check, and the `.github/scripts/pr-docs.cjs` `validateCoverage` preflight passed.
- The actual CI runtime digest `sha256:4850eaf9061f48d915dd007dbe4a0a017ed3d583f9db83f8d93b40a7303f128a` passed a read-only Python curses and xterm terminfo probe. A disposable managed phone capture showed `KEY_UP`; its temporary spec was removed. Existing public docs already describe the arrow controls.
