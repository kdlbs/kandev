# Threads Grid Clarification Overflow: Diagnostic Evidence

Recorded 2026-09-13 in task `f8e3db36-7fe5-492a-b9da-7cf72d3b5077`, session
`0c4b868a-694a-4de2-8d5b-48d554076e23`. This is diagnosis and a browser-only
causal probe, not an implemented or verified production fix.

## User report and base

> when i'm on grid view i can't seem to scroll down to see all the options on a clarification (on my laptop at 90% zoom. I think we need to make it scrollable in that situation.

Clean starting branch: `feature/fix-grid-clarificati-zmg`. A fresh
`git fetch origin main` returned `d62daa9d70437e1a46f7dd31f6138f90ebd54b3f`,
matching HEAD and origin/main. `git merge-base --is-ancestor` confirmed that
`f718c50666eb7175e39087294afabc70ac39f3a7` is included.
`gh pr view 3626 --repo kdlbs/kandev --json state,mergedAt,mergeCommit,baseRefName`
confirmed MERGED, main, that same merge commit, and `2026-09-13T12:13:10Z`.
No obsolete parent branch was checked out or written to.

## Smallest faithful reproduction

1. Use the standard disposable E2E backend/worktree fixture. Create two tasks
   with live primary mock-agent sessions, one using `/e2e:clarification-multi`
   and its neighbor using `/e2e:clarification`. Wait for WAITING_FOR_INPUT.
2. Save a test-owned Threads view with Grid and auto-hide false or true. Open
   `/threads` focused on the multi-question session. Two tasks are essential:
   a single admitted task fills the available height by design.
3. Use a 1366x768 content viewport at 100%, or the same browser window at real
   90% zoom. Place the pointer inside the visible part of the question and
   scroll down. The inner content reaches its end while the outer footer
   stays at scrollTop 0. SQLite (last option) and Next remain clipped.
4. Do not use a locator click/focus/scrollIntoView to reveal the option. Native
   focus can scroll the outer footer and would conceal the wheel regression.

The existing three-question fixture is already long enough: context, question,
three described options, custom input, and navigation total about 475 CSS px.
No fake question DOM or modified backend protocol was needed.

## Native zoom

Browser: bundled headless Chromium `149.0.7827.55`, Linux. A temporary persistent
profile and local MV3 extension with only `tabs` permission called
`chrome.tabs.setZoom(tabId, 0.9)` and verified `chrome.tabs.getZoom(tabId) === 0.9`.
The browser window used `--window-size=1366,855` and no Playwright viewport or
device-scale emulation. At 100%, the content area was 1366x768; at 90%,
`innerWidth/innerHeight` were 1517x853, devicePixelRatio was
0.8999999761581421, and visualViewport.scale stayed 1. At 100%, DPR and visual
scale were both 1. CSS zoom was not applied.

The setup used `chromium.launchPersistentContext(ownedProfile, { channel:
"chromium", headless: true, viewport: null, deviceScaleFactor: undefined,
args: ["--disable-extensions-except=<owned-extension>",
"--load-extension=<owned-extension>", "--window-size=1366,855"] })`.
Explicitly clearing deviceScaleFactor matters inside Playwright Test because
its desktop project otherwise supplies a default. The extension has a minimal
background service worker; obtain that worker through `context.serviceWorkers`
and invoke the tab zoom API there. Close the context in `finally`.

This follows the primary [Playwright extension setup](https://playwright.dev/docs/chrome-extensions)
and [Chrome tabs zoom API](https://developer.chrome.com/docs/extensions/reference/api/tabs#method-setZoom).
The completed probe verifies native browser zoom, not CSS zoom, a larger
emulated viewport, deviceScaleFactor, or pinch zoom. Wheel events hit the actual
clarification at both zooms; at 90% a requested delta of 900 was reported as
about 1000 CSS px. This is desktop automation, not physical laptop/trackpad
hardware evidence. No exact user device/browser information was needed to
establish a faithful reproduction.

## Measured geometry

All numbers below are CSS px, rounded to two decimals. The raw
[geometry record](evidence/geometry.json) retains full precision, before/after
ancestor styles, wheel targets and keyboard focus trails.

| Zoom | Auto-hide | Tile height | Footer height | Footer content height | Question viewport height | Inner content height | Inner scrollTop after wheel | Footer scrollTop after wheel |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 100% | Off | 346 | 198.50 | 541 | 384 | 475 | 90 | 0 |
| 100% | On | 346 | 198.50 | 573 | 384 | 475 | 90 | 0 |
| 90% | Off | 388.66 | 240.87 | 584 | 426.67 | 475 | 48.89 | 0 |
| 90% | On | 388.66 | 240.87 | 616 | 426.67 | 475 | 48.89 | 0 |

At 90%, the tile bottom was y=440.66 and the footer ended at y=439.55.
After three downward wheel events over the question, SQLite began at y=461.39
and Next at y=588.07. Both center hit-tests failed. The header Submit had
scrolled above the visible footer (y=160.95), while the outer footer remained
unmoved. At 100%, the final option began at y=419.75 below a y=397 footer end.
The native transcript stayed 80px high in all four cases.

![Real 90% zoom: final option clipped after wheel scrolling](https://raw.githubusercontent.com/kdlbs/kandev/e0c9e9c77703f7a2efc2729f5c26fb883c29be0a/diagnosis-grid-90-before.png)

These are Playwright screenshots taken at native zoom; use the raw DOM metrics
for CSS geometry, not the exported bitmap dimensions as physical screen size.

## Root cause and scope

The chain is:

```text
ThreadColumn: fixed grid tile, min-height 0, overflow hidden
  TaskChatPanel: full-height flex column
    transcript: independent native scroll owner
    ComposerFooterAllocation: max-height calc(100% - 80px), overflow-y auto,
                              overscroll contain
      ClarificationPanelSection
        overlay container: max-height 50vh, overflow hidden, overscroll contain
          clarification-scroll-region: flex child, min-height 0, overflow-y auto
            question, options, custom input, Next/Back and header Submit
      ChatFooter / ChatInputArea / composer disclosure region
```

`ClarificationPanelSection`'s clipped intermediate container blocks scroll
chaining at the end of the inner question scroll range. Its 50vh viewport is
larger than the tile's footer, so the inner scroller alone cannot expose the
bottom of its own viewport inside the outer clipping boundary. The footer has
ample remaining scroll range, but wheel input never reaches it.

Pending state is correct: the real selected session owns the clarification;
`ChatInputArea` reports required activity and disclosure remains expanded.
The question is outside the animated composer wrapper. Auto-hide changes
routine footer content height but not this failure. No new pending-state or
disclosure controller behavior is needed.

Fourteen native Tab steps reached the last option, custom input, Next, and
composer. Focus scrolled the outer footer to 306px at 100% and 327.78px at
90%, confirming a usable programmatic scroll range. This records navigation
only; full keyboard answering/submission remains an implementation acceptance
check. Pointer-based test helpers that implicitly focus/scroll can mask the bug.

## Browser-only causal probe

After preserving all four failing baseline measurements, change only
`clarification-overlay-container.style.overscrollBehaviorY = "auto"` in the
isolated browser at 90%, auto-hide on. Reset the scrollers to their baseline,
then use small wheel events, center hit-tests, and real mouse clicks.

- The footer moved to 66.67px; SQLite and then Rust became clickable.
- The final question's Bare metal option was clickable with footer scrollTop
  115.56px. Scrolling back exposed header Submit, which passed the hit-test.
- Submit completed the real pending question exchange. The overlay cleared,
  and the mock agent received `db=q1_opt3`, `language=q2_opt3`,
  `deploy=q3_opt2`. No full-task navigation or direct submission API was used.

The [causal probe record](evidence/scroll-chain-probe.json) contains the click
coordinates, ancestor geometry, pending-clear observation, and agent receipt.
The probe proves the scroll-chain cause and feasibility of a narrow correction;
it is not production GREEN or a complete final containment/keyboard/mobile audit.

![Real submission after the temporary DOM-only change](https://raw.githubusercontent.com/kdlbs/kandev/e0c9e9c77703f7a2efc2729f5c26fb883c29be0a/diagnosis-grid-90-dom-probe.png)

## Coverage gap

The current desktop `only the selected session's required question forces its
composer open` scenario admits one task (full height even with Grid selected),
switches sessions and checks required disclosure; it does not answer a long
question. It passed on the affected main. The existing short-phone scenario
taps the first PostgreSQL option in a single question. Neither proves wheel/
touch access to a final option and carousel action in a bounded short tile.

## Validation

| Command/check | Exact result |
| --- | --- |
| `cd apps && pnpm install --frozen-lockfile` | Passed, 903 packages; frozen lockfile unchanged |
| Managed current-assets baseline command below | Passed: backend, Vite E2E assets, fixture plugin built; 1 selected-session test passed (11.5s test body) |
| Temporary diagnostic discovery | 1 test in 1 file |
| First native-zoom diagnostic attempt | 1 setup failure: inherited deviceScaleFactor incompatible with null viewport; no app behavior tested |
| Corrected temporary diagnostic command below | 1 instrumentation test passed (35.3s test body; 46.6s total), four baseline failures recorded and one successful DOM-only causal probe; workers 1, retries 0, strict WS |
| Screenshots | Before/after inspected; data and actions match the measured root cause |
| Production fix and full affected desktop/mobile acceptance | Not run; pending explicit implementation request |

Exact commands, from `apps/web`:

```bash
pnpm e2e:run --host --shards 1 --project chromium tests/task/threads-composer-disclosure.spec.ts --grep 'only the selected session' --workers=1 --retries=0
pnpm e2e:raw --project=chromium e2e/tests/task/grid-clarification-native-zoom-diagnostic.spec.ts --list --workers=1 --retries=0
pnpm e2e:run --host --no-build --shards 1 --project chromium tests/task/grid-clarification-native-zoom-diagnostic.spec.ts --workers=1 --retries=0
```

The last two commands used a temporary diagnostic file, removed at the handoff.
They document executed evidence, not a permanent suite command. Permanent
regression test paths/commands are defined in Task 01.

The initial sandboxed build failed to write the shared Go cache. The authorized
retry completed outside that filesystem restriction. Native Chromium's first
standalone probe also needed the authorized socket-capable execution context.
Neither environment setup failure is counted as behavioral RED. The default
managed build compiled all remote helpers on the cold worktree; the work order
uses the existing lean host build plus fresh web assets for these focused paths.

## Isolation and cleanup

The successful diagnostic used backend PID `3185664`, port `18100`, and owned
root `/tmp/kandev-e2e-0-28DMRc`. Its recorded fallback teardown command was:

```bash
scripts/kandev-kill 18100 --yes
```

That command was valid only while this fixture owned port 18100. The normal
fixture teardown already removed that root and stopped that PID; both absence
checks passed. Do not run a historical port command against a future occupant.
Persistent diagnostic browsers closed via `context.close()`. Temporary test
files and only this task's disposable browser profiles/extensions are removed
before the handoff. The synthetic measurements/screenshots above are retained.
The baseline fixture `/tmp/kandev-e2e-0-s4S96m` and setup-failure fixture
`/tmp/kandev-e2e-0-hDHSWa` are likewise test-owned and automatically torn down.

Developer :9998 and parent playground :48431 were not used or modified. No
parent branch/PR writes, merge/queue changes, or task/session/agent spawning
occurred. Public documentation needs no change for this diagnosis/plan, and
the implementation restores the behavior already described in the existing
Threads guide.
