---
created: 2026-11-11
status: complete
requirements:
  - REQ-UI-QUICK-TERMINAL-001
system_design:
  - ../../specs/ui/system-design/quick-terminal.md
  - ../../specs/ui/system-design/quick-terminal-render-lifecycle.md
legacy_specs: []
---

# Fix plan: Quick Terminal render continuity

## Overview

Fix [issue #5272](https://github.com/kdlbs/kandev/issues/5272) by preserving the selected terminal across ordinary Quick Chat renders.
The startup adapter is stable, and disposed stream callbacks are fenced.
Desktop and phone browser tests prove command entry and resource continuity.

GitHub assignment to `carlosflorencio` was applied and read back on 2026-11-11.

## Evidence and confirmed root cause

Investigation used checkout `fa0c48b88e77`.
Fetched `origin/main` was `4f736702fe0ce37a04aa87017a0c3983c744d26f`.
The three relevant production files and existing Quick Terminal requirement/design matched that upstream head.
No existing PR mentioned #5272 at investigation time.
The canonical issue and its comment contained no image attachments.

The source confirms this chain:

1. `QuickChatTerminalContent` creates a new per-tab status callback on each render.
2. `QuickTerminalTabView.startSession` depends on that callback.
3. `PtyTerminalView` includes `startSession` in its mount-effect dependencies.
4. A callback replacement closes the socket, disposes xterm, and starts another mount.
5. Startup notifications persist terminal state and cause further parent renders.
6. Closing a CONNECTING socket can emit an unguarded `onerror`, which persists another state update.

`useQuickChatStore` also subscribes to task-session and profile collections.
Unrelated agent activity can therefore start the same cycle.
The backend's stable client key can reuse a PTY, but it cannot preserve a repeatedly disposed browser terminal.

### Temporary reproduction

A temporary Vitest harness rendered the real Quick Terminal adapter and shared PTY view.
It reused existing xterm, WebSocket, resize, and API mock patterns.
It did not run a browser or connect to a real PTY.

| Scenario | Observed result |
| --- | --- |
| Five parent renders with fresh inline status callbacks | Six xterm instances, six sockets, six host-shell starts, five socket closes |
| Same renders with one stable callback | One xterm, one socket, one start |
| State-persisting parent and CONNECTING-close errors | More than four starts before the test's 16-notification safety cap |
| Error and binary output after actual cleanup | An error notification and a write to disposed xterm |
| Desired continuity assertion after one callback-only render | Expected one terminal, received two |

The four reproduction checks and 18 existing component tests passed together: three files, 22 tests.
A separate expected-behavior run failed at the resource-count assertion: one failed test, four skipped.
The temporary file `components/settings/issue-5272.repro.test.tsx` was then removed.
No production or permanent test file changed during investigation.

Smallest reproduction: render one fresh Quick Terminal, allow startup to settle, then replace only its parent notification callback.
The permanent regression is `retains one terminal and stream across inline callback rerenders` in the integration suite named below.

## Requirements and assumptions

UI owns this repair because it concerns browser presentation lifetime within the existing shared-tab interaction contract.
Backend descriptor and process ownership remain with their current services.
The adjacent Platform shared-PTY design concerns Unix I/O closure and does not own this React resource lifetime.

Existing AC-UI-QUICK-TERMINAL-001.8, .13, and .14 constrain isolation, phone controls, and startup reconciliation.
The requirement lacked explicit acceptance criteria for ordinary-render continuity and stale stream events.
The package adds .21 and .22 to the existing requirement, with no separate repair requirement.
The focused lifecycle design supplements the existing design, which was already near its 32 KiB limit.

Confirmed scope: investigate, assign a sufficiently clear issue, and produce the fix package.
Verified scope: a stable adapter plus stream cancellation addresses both observed causes.
No unresolved product choice blocks the package.
Existing descriptor and tab-order ADRs remain authoritative, so this repair needs no new ADR.

## Scope

### In scope

- Keep Quick Terminal startup independent of notification-handler identity.
- Use the latest notification handler after an asynchronous start.
- Ignore every event from a disposed stream generation, including a delayed initial-input send.
- Preserve late-start identity publication, explicit close, detach, login cleanup, and StrictMode behavior.
- Add component regressions and desktop/phone command-entry coverage.

### Out of scope

- [Issue #5273](https://github.com/kdlbs/kandev/issues/5273): terminal keepalive and proxy-related WebSocket failures.
- Reconnect policy, descriptor schema, backend process lifetime, or rolling-buffer retention.
- Removing duplicate startup notifications or redesigning status persistence.
- Modal subscription optimization, global handler memoization, or task `PassthroughTerminal` changes.
- Changing shared `startSession` or `initialInput` reset semantics.
- New controls, localized copy, settings, or navigation.

## Technical approach

### Stable startup adapter

In `components/quick-chat/quick-terminal-tab-view.tsx`, use `onStateChangeRef.current` inside `startSession`.
Keep that callback stable with an empty dependency list.
Retain its existing startup notification and stable per-tab client identity.
`PtyTerminalView.reportRef` already supplies the latest handler for later lifecycle reports.

The shared mount effect retains `startSession` and `initialInput` dependencies.
Login callers can change agent or command parameters through these inputs.
Stabilizing the affected adapter fixes the demonstrated regression without changing that existing contract.

### Stream generation guard

In `components/settings/pty-terminal-view.tsx`, pass the mount cancellation token into the stream callbacks.
Check it before error reporting, output writes, exit handling, resize disarming, or initial-input sends.
Set cancellation before socket close and xterm disposal.
Keep guards effective for callbacks already queued or captured before cleanup.
Use a small callback context object rather than exceeding the five-parameter limit.

Keep the existing pending-start and generation ownership rules.
In particular, retain `reports a detached late start so the tab retains its session identity`.
A blanket suppression of `args.report` after cleanup breaks that contract.

The [lifecycle compatibility matrix](../../specs/ui/system-design/quick-terminal-render-lifecycle.md#lifecycle-compatibility) defines affected consumers and controls.

## ASCII UI preview

### UI-01: Desktop, selected running terminal

Entry: the existing Quick Terminal launcher or shared-dialog terminal tab.
The correction preserves the current composition.

```text
Before, after an unrelated update:
[ Quick Chat                              Close ]
[ + ] [ Terminal 1* ]
[ terminal clears, reconnects, replays output     ]
[ connection error can flash                     ]

After, across the same update:
[ Quick Chat                              Close ]
[ + ] [ Terminal 1* ]
[ previous output                               ]
[ $ partly typed command_                       ]
```

The header and tab strip remain fixed.
The selected xterm remains the vertical scroll owner and retains input focus.
Real connection errors still use the existing localized status presentation.
Maps to .21 and .22, with sibling isolation under .8.

### UI-02: Phone, selected running terminal

Entry: app menu, then Quick terminal.
This retains the existing full-height dialog rather than introducing another surface.

```text
+--------------------------------+
| Quick Chat               Close  |
| + | Terminal 1* | other tabs -> |
| previous terminal output       |
| $ partly typed command_        |
|                                |
| Ctrl Shift ^C ^D Esc Tab ... -> |
+--------------------------------+
| on-screen keyboard             |
+--------------------------------+
```

The tab strip and shortcut row retain their own horizontal overflow.
Terminal output owns the remaining vertical region.
Existing visual-viewport offsets and safe-area padding keep controls above the keyboard.
Shortcut targets retain at least 44 CSS pixels and preserve input focus.
Maps to .13 and .15, with stale-event protection under .16.
Structure and resource continuity are required. Text and spacing are illustrative.

## Tests

All paths below are relative to `apps/web`.

| Acceptance | Test file and named case | Evidence |
| --- | --- | --- |
| .15 | New `components/quick-chat/quick-terminal-render-lifecycle.test.tsx`: `retains one terminal and stream across inline callback rerenders` | Real adapter/view, five separate renders, one construction/start/stream, no disposal |
| .15, .16 | Same file: `does not loop when persisted state rerenders the parent and a connecting socket errors` | State persistence and CONNECTING-close errors stay bounded to one mount |
| .15 | `components/quick-chat/quick-terminal-tab-view.test.tsx`: `reports a deferred start through the latest status handler` | Handler replacement does not capture stale state |
| .16 | `components/settings/pty-terminal-view.test.tsx`: `ignores disposed stream events after cleanup and replacement` | Captured error/close/output/exit/open callbacks cannot affect either generation |
| .16 | Same file: `preserves live stream error and exit handling` | Live errors, exit notifications, and resize disarming still work |
| .8, .13, .14 | Existing wrapper, shared PTY, and agent-login suites | Detach/stop, late identity, explicit cancellation, StrictMode, input, modifiers, and missing-session behavior |

The existing wrapper suite mocks `PtyTerminalView`.
Keep it and add a separate integration suite that imports both real components.
Otherwise the regression test cannot observe the original dependency chain.

## E2E tests

New desktop file: `e2e/tests/terminal/quick-terminal-render-continuity.spec.ts`, project `chromium`.
New phone file: `e2e/tests/terminal/mobile-quick-terminal-render-continuity.spec.ts`, project `mobile-chrome`.
Shared scenario helpers belong in `quick-terminal-render-continuity-helpers.ts` beside those files.

Arm start-request, descriptor-write, and terminal-stream observers before opening the terminal.
Use the existing `__KANDEV_E2E_STORE__` bridge to publish five immutable `taskSessions.items` replacements.
Allow each update to render separately rather than batching them into one evaluation.
Do not add a production hook or require a real running agent.

Type a split `printf` command across these updates and require its assembled output marker.
The echoed command must not contain that complete marker.
Assert the original `.xterm` node, focused helper textarea, retained output, and unchanged session identity.
Require one initial host-shell start and one terminal stream, with no render-driven request growth.
Use the causal-wait helpers and a documented negative-assertion `dwell` only where a quiet observation window is necessary.

The phone scenario enters through the app menu with touch.
It retains the shortcut row and terminal focus, checks viewport containment, and captures a screenshot.
Existing desktop and phone terminal files remain in the targeted runs for detach, reload, close, and sibling controls.

## Related delivery records

| Package | Recorded status at investigation | Treatment |
| --- | --- | --- |
| [Original terminal package](../quick-terminal/plan.md) | completed | Preserve its detach, explicit-close, and shared-dialog scenarios |
| [Refresh persistence](../quick-terminal-refresh-persistence/plan.md) | complete | Preserve reattachment and missing-session coverage |
| [Durable lifecycle/menu](../quick-terminal-durable-lifecycle/plan.md) | draft | No timeout, shutdown, or menu work in this package |
| [Tab organization](../quick-chat-tab-order/plan.md) | completed | No ordering or rename changes |
| [Phone controls](../quick-terminal-mobile-controls/plan.md) | done | Retain modifier, focus, keyboard-clearance, and sibling checks |

These records are context, not new implementation dependencies.
Historical result counts remain historical. Task 02 records the actual final counts for this repair.

## Work orders

- [x] [Task 01: Preserve terminal resources and fence stale events](task-01-preserve-terminal-lifecycle.md) (done)
- [x] [Task 02: Prove desktop and phone input continuity](task-02-prove-render-continuity.md) (done)

Execute Task 01, then Task 02, sequentially in the primary conversation.

## Verification results

Investigation results are recorded above.
Implementation and verification results are recorded in both work orders and this manifest.
The design-package validation below is historical handoff evidence from 2026-11-11.

Design-package validation must include:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs/ui docs/plans/quick-terminal-render-continuity
git status --short -- docs/specs/ui docs/plans/quick-terminal-render-continuity
```

Design-package results at handoff on 2026-11-11:

- Catalog validation passed: 449 decisions and 1985 specifications.
- Specification linter tests passed: 36 tests.
- Full specification lint passed, including both Quick Terminal designs.
- Documentation coverage passed for both work orders against the two planned production paths.
- Diff whitespace and untracked-file inventory checks passed.
- Production changes, permanent regressions, browser runs, and screenshots were pending at handoff.

Implementation results on 2026-11-11:

- The Quick Terminal startup adapter remains stable across notification-handler replacements and forwards deferred startup state to the latest handler.
- The shared PTY stream ignores callbacks from a disposed mount before reporting, writing output, handling exits, disarming resize, or sending delayed initial input. Detached late-start identity publication remains covered.
- Review follow-up: the stale-stream regression rerenders the same `PtyTerminalView` instance with a changed `startSession` callback. Removing only the old `onclose` cancellation guard makes the successor resize assertion fail.
- Review follow-up: the desktop E2E captures the prior sidebar preferences and restores them in a nested `finally`, including when terminal cleanup throws.
- Before the PR merge, four focused component suites passed 29 tests, including the retained existing lifecycle and agent-login controls.
- The updated PTY suite passed 16/16 tests after the same-instance race regression was added; the component-test count remains 29.
- Focused Oxlint, web typecheck, i18n check, and i18n ratchet passed.
- Fresh-build desktop E2E passed 3/3: the continuity regression and both existing Quick Terminal desktop cases.
- Fresh-build phone E2E passed 3/3: the continuity regression and both existing mobile Quick Terminal cases.
- Desktop and phone discovery each found all three targeted tests. The phone screenshot was captured and inspected at `apps/web/e2e/test-results/terminal-mobile-quick-term-714ab-s-through-unrelated-renders-mobile-chrome/quick-terminal-render-continuity.png`; it shows retained command output and the shortcut row without an overlay.
- PR screenshot capture passed both continuity projects in one managed run and produced fresh desktop and phone entries in the ignored `.pr-assets/manifest.json`. Both screenshots were inspected, their terminal prompt paths were reduced to `/tmp`, and the PNGs were compressed for upload.
- Catalog validation, the 36 specification-linter tests, full specification lint, documentation coverage, and diff checks passed.

The managed browser runs built the backend fixture and fresh pseudo-locale Vite assets before each project. Vite emitted existing chunking and import warnings; both builds completed successfully.

PR fixup results on 2026-11-12:

- Merged current `main` commit `dab0d02e` to resolve the PR conflict. The main branch already owns recovery criteria .15 through .20; continuity remains the same behavior and is recorded as .21 and .22. Work-order and design references now point to those IDs.
- The sidebar restoration and same-component stale-handler findings were already covered on the PR head. The continuity spec restores preferences from a nested `finally`, and the PTY regression rerenders one mounted component, fires the old handlers, and checks replacement resize.
- Fixed the valid Greptile/CodeRabbit screenshot timing finding by reading `endTime` from `getComputedTiming()`.
- Four focused component suites passed 32/32 tests after the merge.
- The fresh-build desktop and phone continuity E2E tests each passed 1/1 with retries disabled.
- Web typecheck, focused Oxlint and E2E sleep lint, catalog validation, all 36 specification-linter tests, full specification lint, and diff checks passed after the merge.
- The post-merge catalog validated 450 decisions and 1993 specifications. Documentation coverage passed for both work orders.

For documentation coverage, run this repository validator from the root.
It includes the two planned production paths so the validator checks references rather than returning the documentation-only exemption.
These paths are validation inputs, not evidence of production edits.

```bash
node <<'NODE'
const fs = require('node:fs');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const paths = [
  'docs/specs/ui/requirements/quick-terminal.md',
  'docs/specs/ui/system-design/quick-terminal.md',
  'docs/specs/ui/system-design/quick-terminal-render-lifecycle.md',
  'docs/plans/quick-terminal-render-continuity/plan.md',
  'docs/plans/quick-terminal-render-continuity/task-01-preserve-terminal-lifecycle.md',
  'docs/plans/quick-terminal-render-continuity/task-02-prove-render-continuity.md',
];
const changed = new Set([
  ...execFileSync('git', ['diff', '--name-only', 'HEAD'], { encoding: 'utf8' }).trim().split('\n'),
  ...execFileSync('git', ['ls-files', '--others', '--exclude-standard'], { encoding: 'utf8' }).trim().split('\n'),
].filter(Boolean));
changed.add('apps/web/components/quick-chat/quick-terminal-tab-view.tsx');
changed.add('apps/web/components/settings/pty-terminal-view.tsx');
const result = validateCoverage({
  changedFiles: [...changed].map(filename => ({ filename, status: fs.existsSync(filename) ? 'modified' : 'removed' })),
  fileContents: Object.fromEntries(paths.map(path => [path, fs.readFileSync(path, 'utf8')])),
});
console.log(JSON.stringify(result, null, 2));
if (!result.ok || result.status !== 'covered' || result.workOrders.length !== 2) process.exitCode = 1;
NODE
```

## Documentation impact

Internal specifications and delivery records change.
The existing public Quick Terminal guidance retains the same controls and lifecycle.
No public-page, screenshot-catalog, or translation change is required for this repair.

## Risks

- Broad cancellation can erase a late-start identity needed for explicit close. Retain that existing regression.
- CONNECTING-close errors differ across browsers. Component tests model the reported behavior without claiming a Safari browser run.
- StrictMode replay can create deliberate development mounts. Assert one resource for ordinary renders after setup settles.
- Initial-input and live-error controls must remain effective after stream guards are added.
- WebSocket proxy failures from #5273 can still occur after this repair.
