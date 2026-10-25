---
id: "01-phone-navigation"
title: "Add phone passthrough navigation"
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
  - AC-CLI-MOBILE-PASSTHROUGH-NAV-001.5
  - AC-CLI-MOBILE-PASSTHROUGH-NAV-001.6
  - AC-CLI-MOBILE-PASSTHROUGH-NAV-001.7
  - AC-CLI-MOBILE-PASSTHROUGH-NAV-001.8
system_design:
  - ../../specs/cli/system-design/mobile-passthrough-navigation.md
---

# Task 01: Add Phone Passthrough Navigation

## Summary

Reuse terminal keys on phone passthrough Chat with local agent-socket targeting.
Implement the complete vertical slice using TDD, including mobile geometry,
composer coexistence, lifecycle cleanup, and public how-to guidance.

## In scope

- Shared `sendPtyInput` relocation with behavior preserved; owned phone wrapper.
- Toolbar visibility and mobile-layout keyboard clearance; localized controls.
- Unit/component and mobile/desktop E2E coverage mapped by the plan.

## Out of scope

- Backend routes, persistence, tablet controls, and Quick Terminal floating-bar repair.

## Acceptance

1. Bar taps and typed modifier chords send exactly once to the selected agent;
   connecting/reconnecting/old-close races never send to a shell or another session.
2. UI-01/UI-02 phone layout is touch-reachable, reserves keyboard space, preserves
   composer drafts, and clears modifier/listener ownership on hide or departure.
3. UI-03 and existing task-shell/host-PTY controls retain their behavior; all
   targeted checks pass and captures/smoke limitations are recorded.

## ASCII UI preview

UI-01/UI-02/UI-03 excerpt; see [full previews](plan.md#ascii-ui-preview).

```text
Phone connected      Phone composer open   Desktop / tablet
[Agent output    ]   [Agent output     ]   [Native terminal   ]
[Ctrl ... arrows>]   [Composer + Send  ]   [Optional composer ]
[Status row      ]   [Status row       ]   [Status row        ]
[Task navigation ]   [Task navigation  ]
```

The phone bar is inline and internally scrollable, absent when panels are open
or the socket is unavailable. Output owns flexible vertical scrolling. The
layout clears keyboard/safe areas; desktop gets no bar. These structural choices
map to 001.1, 001.4-.8. Spacing and abbreviated copy are illustrative.

## Verification

From repository root, use independent working directories. In a fresh worktree,
first run `(cd apps && pnpm install --frozen-lockfile)` once. Write meaningful
RED coverage before production changes. Run each E2E project sequentially using
the managed runner and verify discovery counts. Extend every changed suite's
command if additional tests are introduced.

```bash
(cd apps/web && pnpm exec vitest run lib/terminal/send-pty-input.test.ts components/task/mobile/mobile-agent-terminal.test.tsx components/task/passthrough-toolbar.test.tsx components/task/mobile/mobile-terminal-keybar.test.tsx components/task/mobile/session-mobile-layout.test.tsx components/settings/pty-terminal-view.test.tsx components/quick-chat/quick-terminal-tab-view.test.tsx)
(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/cli-mode/mobile-passthrough-navigation.spec.ts e2e/tests/cli-mode/mobile-passthrough-composer.spec.ts e2e/tests/terminal/mobile-passthrough-rendering.spec.ts e2e/tests/terminal/mobile-terminal-keybar.spec.ts e2e/tests/terminal/mobile-quick-terminal.spec.ts -- --retries=0)
(cd apps/web && pnpm e2e:run --project chromium e2e/tests/cli-mode/passthrough-toolbar.spec.ts e2e/tests/terminal/quick-terminal.spec.ts -- --retries=0)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint lib/terminal/send-pty-input.ts components/task/mobile/mobile-agent-terminal.tsx components/task/passthrough-toolbar.tsx components/task/mobile/session-mobile-layout.tsx components/settings/pty-terminal-view.tsx e2e/tests/cli-mode/mobile-passthrough-navigation.spec.ts e2e/tests/cli-mode/passthrough-toolbar.spec.ts)
(cd apps/web && pnpm run i18n:check)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the local `validateCoverage` preflight exported by
`.github/scripts/pr-docs.cjs` with changed paths and full referenced document
contents. Record actual results plus inspected phone captures; real iPhone
Safari/Android Chrome keyboard checks must be recorded or explicitly unavailable.

## Files likely touched

- `apps/web/components/task/mobile/mobile-agent-terminal.tsx` and colocated tests.
- `apps/web/components/task/passthrough-toolbar.tsx` and tests.
- `apps/web/components/task/mobile/session-mobile-layout.tsx` and tests.
- `apps/web/lib/terminal/send-pty-input.ts` and test (moved from settings).
- `apps/web/components/settings/pty-terminal-view.tsx`; associated host-PTY tests.
- `apps/web/e2e/tests/cli-mode/mobile-passthrough-navigation.spec.ts` (new),
  `passthrough-toolbar.spec.ts`; nearest page objects/helpers if shared.
- `docs/public/developer-tools.md`; paired specification lifecycle on completion.

## Dependencies

None. The user confirmed Quick Terminal work was already implemented and deleted
its task; preserve those existing controls.

## Risks

See [plan risks](plan.md#risks). Do not use global xterm focus or shell fallback.
Keep callback identity stable; guard socket close cleanup by socket identity.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/cli/requirements/mobile-passthrough-navigation.md).
- [System design](../../specs/cli/system-design/mobile-passthrough-navigation.md).
- Existing mobile terminal pane, key bar, PTY input helper, and mobile composer tests.
- Root/scoped AGENTS.md; `/tdd`, `/mobile-parity`, `/e2e`, and `/docs-maintainer`.

## Results

Completed locally on 2026-10-20. The current implementation uses the existing
`AgentTerminalInput`, with local socket ownership and inline controls. The shared
PTY helper retains its host-terminal behavior. No new user-facing labels were added.

The unit/component command passed all 118 tests. The mobile browser command
passed all 27 tests. The desktop browser command passed all 12 tests. Both browser
runs disabled retries. Typecheck, changed-file lint, formatting, localization,
specification validation, public documentation checks, and local documentation
coverage passed. The new regression tests failed against main before production
changes and passed with the implementation.

Connected, simulated-keyboard, and composer phone captures were inspected.
Physical iPhone Safari and Android Chrome keyboard testing was unavailable.
The initial checkpoint did not publish changes. The user subsequently authorized
a branch push and isolated test instance, followed by PR #4625 and review fixup.

See [recovery evidence](recovery.md) for source task/session IDs, approval evidence,
artifact paths, test logs, and the limits of the restored source snapshots.

### PR review fixup

The phone agent releases shared modifiers while Quick Chat is open. Its input
callback reads only committed control visibility, including during suspended
React transitions. Unit regressions failed before each fix and passed afterward
(120 tests across the seven-file command above). The new browser regression
failed before the overlay ownership fix and verifies Ctrl on the host PTY after
a background agent reconnect.

The complete mobile command passed all 28 tests with retries disabled.
Typecheck, changed-file ESLint, formatting, specification catalog/lint, and diff
whitespace checks passed. The desktop path is unchanged by these fixes; its
12-test result above remains the prior verification baseline.

Final remote CI and review state will be verified after pushing.

### Claude review suggestion

Inlined the one-use terminal JSX factory. The existing composer hook now groups
composer state, focus, and shortcut handling. Rendered structure and input
behavior are unchanged; no additional behavior test is needed. The existing
120 unit tests, typecheck, and changed-file lint passed after this refactor.

The affected mobile composer/navigation suites passed all 10 tests with retries
disabled. An earlier run hit a local backend port collision before the failing
test body; rerunning on verified free `E2E_PORT_OFFSET=29` passed. No production
change was made for that fixture-startup failure.
The desktop command also passed all 12 tests with retries disabled.

### Frontend CI manifest correction

Frontend shard 1 failed the existing project-selection contract because the
reviewed browser manifest retained the moved `pty-terminal-input.test.ts` path.
The contract reproduced locally before the correction. The manifest now lists
`lib/terminal/send-pty-input.test.ts` in the same browser project. This changes
only the file identity, not the test setup or discovery contract.

The corrected project-selection contract and seven terminal suites passed all
124 tests across eight files. The moved input test retained its browser setup.
