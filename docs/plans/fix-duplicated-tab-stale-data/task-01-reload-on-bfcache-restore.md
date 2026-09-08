---
id: "01-reload-on-bfcache-restore"
title: "Reload on bfcache restore"
status: in-progress
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/fix-duplicated-tab-stale-data.md"
---

# Task 01: Reload on BFCache Restore

## Acceptance

- `apps/web/src/bfcache-restore-reload.ts` exports `installBfcacheRestoreReload`
  which reloads the page on a `pageshow` event when `event.persisted === true`
  only (never on the `back_forward` navigation type alone, which also covers
  cold history traversals and session-restored tabs), and returns an
  uninstall function.
- `apps/web/src/main.tsx` installs it at module scope next to
  `installVitePreloadRecovery()`, before React mounts.
- Unit tests cover: persisted restore reloads; persisted=false does not reload
  (fresh load, manual refresh, and cold `back_forward` traversal regression);
  an event with no persisted flag does not reload; uninstall removes the
  listener.
- E2E test dispatches a real persisted `pageshow` in the running app and
  asserts the page reloads (navigation type becomes `reload`); a normal load
  does not reload.
- No user-facing copy added (no i18n ratchet impact).
- MERGE BLOCKING: a real-Chrome native Duplicate-tab run is performed and
  recorded (see "Blocking verification" below). The automated suite proves
  the handler fires on `persisted === true`, which is the only event sequence
  consistent with the reported symptom (a cold duplicate load re-fetches the
  no-store boot payload and would show fresh data; only a frozen restore
  preserves the stale view, and frozen restores fire `pageshow` with
  `persisted === true`). The native-run record confirms that sequence on the
  user's Chrome before merge.

## Blocking verification (merge gate)

Run in the user's non-headless Chrome against a debug build of the affected
Kandev (debug mode sets `window.__KANDEV_DEBUG`, which arms the pre-reload
probe in `bfcache-restore-reload.ts`). The duplicated tab is a SEPARATE
DevTools target and the handler reloads synchronously during its initial
`pageshow`, so attaching DevTools afterward cannot observe the restore; the
probe persists the evidence to sessionStorage BEFORE the reload.

1. In the SOURCE tab's DevTools console, clear any inherited probe from an
   earlier run and store the attempt start time in sessionStorage:
   `sessionStorage.removeItem("kandev.bfcacheRestoreProbe"); sessionStorage.setItem("kandev.bfcacheProbeStart", String(Date.now()));`
   (sessionStorage survives reloads and Chrome Duplicate copies the source
   tab's storage — including this helper key — so the duplicated tab can
   compare timestamps even though console variables are target-local.)
2. Archive a task, then right-click the Kandev tab → **Duplicate**.
3. In the duplicated tab's DevTools console, read the probe and check it is
   newer than this attempt in one self-contained, null-safe expression
   (console bindings are not carried over from the source tab):
   `((JSON.parse(sessionStorage.getItem("kandev.bfcacheRestoreProbe") ?? "null")?.at ?? -1) >= Number(sessionStorage.getItem("kandev.bfcacheProbeStart")))`
   The probe records `{ persisted, navigationType, at }` captured pre-reload;
   the comparison must be true for the record to belong to this run (a stale
   probe inherited from an earlier restore must not be attributed to it). If
   no `pageshow` fired, there is no probe entry and the expression evaluates
   false instead of throwing, so the no-probe outcome in step 6 can be
   recorded.
4. Confirm the handler reloaded from inside the duplicated tab's console
   after the reload settles:
   `performance.getEntriesByType("navigation")[0]?.type` must be `"reload"`
   (a bfcache restore without the fix would report `"back_forward"`). The
   archived task must no longer be listed as active. (The duplicated tab is a
   separate DevTools target and reloads synchronously, so source-tab Network
   capture cannot observe its document request; the navigation-type and
   UI-state checks above are the executable proof.)
5. Expected with this fix: the probe shows `persisted: true`, the duplicated
   tab reloads, and the task is shown as archived.
6. If the duplicated tab fires NO `pageshow` (no probe entry) or shows a
   probe with `persisted: false` while still displaying stale data, the
   frozen-restore assumption is wrong for that Chrome version: record the
   observed sequence (including WebSocket close code/timing, if any), then
   design a fallback from that evidence — see the WS-close caveat below. Do
   not add a fallback without this evidence.

### WS-close fallback caveat (only if step 6 triggers)

The navigation type `back_forward` is fixed for the document lifetime, and
`WebSocketClient.handleDisconnect` observes every later unintentional
disconnect, so reloading on "unexpected WS close while the navigation type is
`back_forward`" would also fire after a cold history/session restore followed
by an ordinary network loss — recreating the round-1 false-positive class.
Any WS-based fallback must be bounded to the restore/bootstrap context (e.g.
only within a short window after the restore, or only when the sessionStorage
probe from this handler is present) and must include a regression test that a
routine disconnect after a cold `back_forward` load does NOT reload.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run src/bfcache-restore-reload.test.ts
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
cd apps/web && pnpm e2e:run tests/layout/bfcache-restore-reload.spec.ts
```

Manual, on a real (non-headless) Chrome: archive a task, right-click the
Kandev tab → Duplicate; the duplicated tab must reload and show the task as
archived. Record the outcome in Results.

## Files Likely Touched

- `apps/web/src/bfcache-restore-reload.ts` (new)
- `apps/web/src/bfcache-restore-reload.test.ts` (new)
- `apps/web/src/main.tsx` (wire install call)
- `apps/web/e2e/tests/layout/bfcache-restore-reload.spec.ts` (new)

## Dependencies

None.

## Parallelism

Sequential. Single task; no parallel candidates.

## Inputs

- Repair spec: `docs/specs/ui/requirements/fix-duplicated-tab-stale-data.md`.
- Existing pattern: `apps/web/src/vite-preload-recovery.ts` and its test
  (injected `target`/`reload`, defensive storage reads).
- Bootstrap entry: `apps/web/src/main.tsx` (`installVitePreloadRecovery()`
  call site).

## Risks

- Chrome duplicate-tab restore event delivery may vary by version. Detection
  is `pageshow.persisted === true` only; a state-clone restore that fires no
  `pageshow` at all is not detectable by this handler. If a real-Chrome
  duplicate test shows no reload, follow up with the WS-close fallback
  described in the plan (reload on unexpected WS close when the navigation
  type is `back_forward`), gated on that evidence; do not add it
  speculatively.
- happy-dom lacks `PageTransitionEvent`; tests define the `persisted` flag on
  a plain `Event`. The cold-`back_forward` regression is covered twice: the
  injected-seam case (harness `navigationType: "back_forward"` through the
  injected reader) and the default-reader case (module installed without the
  injected reader while global `performance` reports `back_forward`), so a
  reintroduced navigation-type fallback fails either way.
- E2E must use the synthetic persisted-`pageshow` signal (see plan): with an
  open WebSocket, current Chrome does not bfcache `no-store` pages on
  back/forward, so `page.goBack()` would pass trivially. The reload assertion
  retries with `expect(...).toPass({ timeout: 15_000 })` (catching the
  destroyed execution context mid-navigation); no sleeps.

## Output Contract

Report the root-cause evidence summary, files changed, exact tests run
(unit, typecheck, lint, E2E), the real-Chrome duplicate-verification outcome,
task and plan status updates, and any follow-up decision on the WS-close
fallback.

## Results

- Added `apps/web/src/bfcache-restore-reload.ts`: a `pageshow` handler that
  reloads the page when `event.persisted === true`, with an uninstall
  function (pattern: `vite-preload-recovery.ts`).
- Wired `installBfcacheRestoreReload()` into `apps/web/src/main.tsx` at module
  scope next to `installVitePreloadRecovery()`, before React mounts.
- Added `apps/web/src/bfcache-restore-reload.test.ts` (13 tests) and
  `apps/web/e2e/tests/layout/bfcache-restore-reload.spec.ts`.
- Checks passed:
  - `cd apps && pnpm --filter @kandev/web test -- --run src/bfcache-restore-reload.test.ts`
    (13/13 tests, current HEAD).
  - `cd apps/web && pnpm run typecheck` (clean).
  - `cd apps/web && pnpm exec eslint src/bfcache-restore-reload.ts
    src/bfcache-restore-reload.test.ts src/main.tsx
    e2e/tests/layout/bfcache-restore-reload.spec.ts` (clean).
  - `cd apps/web && pnpm e2e:run tests/layout/bfcache-restore-reload.spec.ts`
    (1/1; the assertion arms a `framenavigated` causal wait before
    dispatching the restore signal, then asserts the reload navigation type
    with the default timeout — the earlier `expect(...).toPass({ timeout:
    15_000 })` effect-polling budget was removed per the E2E causal-wait
    convention).

### Adversarial review round 1 (50-luna-review-fix)

- Finding (P2, accepted): the initial version reloaded on the
  `back_forward` navigation type in addition to `persisted === true`, but
  `back_forward` identifies every history traversal, not only frozen
  restores. A cold back/forward load or session-restored tab (both load
  fresh) was therefore reloaded a second time, discarding restored scroll/UI
  state on the common click-out-and-back flow. Frozen restores always fire
  `pageshow` with `persisted === true`, so the fallback added false positives
  without covering any additional restore path.
- Resolution: detection is now `persisted === true` only; the navigation-type
  fallback and its tests were removed; a regression test asserts no reload on
  a cold `back_forward` traversal. Spec, plan, and this file updated to match.

- Real-Chrome duplicate-tab verification (non-headless, user's environment)
  remains outstanding: archive a task, right-click the tab → Duplicate, expect
  the duplicated tab to reload and show the task as archived. If a Chrome
  version restores without firing `pageshow` at all (no persisted signal),
  add the plan-documented WS-close fallback gated on that evidence; none was
  needed for the signals observed here.

### Adversarial review round 2 (50-luna-review-fix)

- Finding 1 (minor, accepted): the cold-`back_forward` regression test only
  dispatched `persisted=false` without exposing a `back_forward` navigation
  entry, so it was behaviorally identical to the fresh-load test and a
  reintroduced navigation-type fallback would stay green under happy-dom (no
  matching navigation entry). Fixed: the test now stubs
  `performance.getEntriesByType("navigation")` to return a `back_forward`
  entry before dispatching `persisted=false`, and asserts no reload.
- Finding 2 (nit, accepted): the plan and task still described the removed
  navigation-type reader and defensive Navigation Timing reads. Fixed: both
  documents now describe only injected `target`/`reload` and the
  plain-`Event` persisted test, retaining the separately gated WS-close
  fallback.
- No production-code correctness issue found. The real-Chrome duplicate-tab
  run remains the outstanding evidence gate for the WS-close fallback
  decision.

### Adversarial review round 3 (50-luna-review-fix)

- Finding (nit, accepted): the durable plan/task record said the unit
  environment is jsdom and the E2E uses `expect.poll` with default timeouts,
  but `apps/web/vitest.config.ts` configures happy-dom and the implemented
  E2E uses `expect(...).toPass({ timeout: 15_000 })` (the reload teardown
  destroys the evaluate context, so the retry swallows those errors; 15s
  covers the reload plus a fresh app boot). Fixed: all jsdom references in
  the plan package now name happy-dom, and the E2E wait strategy is
  documented as `toPass`/15s.
- No production correctness issue found (persisted-only semantics, bootstrap
  timing/StrictMode, listener lifecycle, reload-path interaction, option/cast
  safety, cold-`back_forward` stub, E2E positive/negative causality).

### Adversarial review round 4 (50-luna-review-fix)

- Finding (major, partially accepted): native Chrome Duplicate calls
  `contents->Clone()` + `CopyStateFrom(..., true)` + `LoadIfNecessary()`, and
  the cited Chromium discussion establishes only a `back_forward` navigation
  type, not `persisted=true`; the E2E synthesizes the signal, so all
  automated checks could pass while the reported path remains unfixed.
- Response: the premise that native Duplicate is a cold "navigated clone" is
  inconsistent with the reported symptom. A cold duplicate load re-fetches
  the no-store boot payload (and the tasks/kanban fetches are `no-store`),
  so it would show FRESH data; the user's stale view can only come from a
  frozen restore (JS heap preserved), and frozen restores fire `pageshow`
  with `persisted === true`. `persisted === true` is therefore the correct
  signal for the observed bug, and the handler cannot fire on a cold load.
  Accepted part: the native-Duplicate event sequence on the user's Chrome
  version is unverified, and a Chrome build could in principle clone without
  firing `pageshow` at all. That verification is now a MERGE BLOCKING gate
  with an instrumentation checklist (pageshow.persisted, navigation type,
  boot-payload requests); if the observed sequence contradicts the
  assumption, the WS-close fallback is implemented gated on that evidence.
- The real-Chrome duplicate-tab run remains the outstanding evidence gate
  and now blocks merge.

### Adversarial review round 5 (50-luna-review-fix)

- Finding 1 (major, accepted): the task/plan were marked done/complete while
  the merge gate was outstanding. Fixed: both are now `in-progress`; they are
  flipped to done/complete only after the native run is recorded.
- Finding 2 (major, accepted): the gate checklist could not capture the
  evidence — `window.__KANDEV_DEBUG` is only a boolean, the duplicated tab is
  a separate DevTools target, and the synchronous reload destroys the
  pre-reload event/navigation data. Fixed: the module now writes a
  debug-gated probe (`kandev.bfcacheRestoreProbe`) to sessionStorage BEFORE
  reloading, recording `{ persisted: true, navigationType, at }`; the
  checklist reads the probe post-duplicate and corroborates with a fresh
  boot-payload request. Probe covered by 3 new unit tests (records in debug,
  silent in non-debug, never blocks the reload).
- Finding 3 (minor, accepted): the WS-close fallback rule was not
  restore-specific — `back_forward` is fixed for the document lifetime and
  `handleDisconnect` observes every later unintentional disconnect, which
  would recreate the round-1 false-positive class after a cold restore plus
  ordinary network loss. Fixed: the fallback is now explicitly caveated (must
  be bounded to the restore/bootstrap context, e.g. a short post-restore
  window or the presence of this handler's sessionStorage probe, and must
  include a routine-disconnect-after-cold-back_forward regression test) and
  is only designed after the step-5 evidence.
- Finding 4 (nit, accepted): the plan claimed no `pageshow` handlers existed
  anywhere, but `useForegroundRefresh` already registers one. Fixed: the
  claim now says no handler inspects `pageshow.persisted`.

### Adversarial review round 6 (50-luna-review-fix)

- Finding 1 (major, accepted): the probe could not capture the falsifying
  `persisted=false` case — the handler returned before recording and the
  payload hard-coded `persisted: true`. Fixed: the handler now records the
  ACTUAL `persisted` value for duplicate/history candidates (`persisted ===
  true`, or any `back_forward`-typed document) before any reload decision,
  and the record survives the reload in sessionStorage; a
  persisted=false/back_forward record-without-reload unit test was added.
- Finding 2 (minor, accepted): `isDebug()` ran outside the best-effort
  guard, so a throwing predicate aborted the handler before `reload()`.
  Fixed: the predicate is inside the diagnostic try/catch; a throwing-`isDebug`
  test proves the reload still runs.
- Finding 3 (minor, accepted): the gate read the fixed sessionStorage key
  without clearing it, and Chrome Duplicate copies the source tab's storage,
  so a stale probe could be misattributed. Fixed: the checklist now clears
  the key and records `t0` before Duplicate, and requires `at >= t0`.
- Finding 4 (minor, accepted): Results recorded 6/6 for a file now containing
  12 tests. Fixed: re-ran the focused unit command on current HEAD (12/12)
  and updated the record.
- Finding 5 (nit, accepted): the spec said the fix writes no client storage,
  but debug restores write the sessionStorage probe. Fixed: Persistence
  guarantees now document the debug-only diagnostic key, fields, lifetime,
  cleanup, and best-effort failure behavior.

### Adversarial review round 7 (50-luna-review-fix)

- Finding 1 (minor, accepted): the cold-`back_forward` regression test
  stubbed global `performance`, but the harness always injects the
  navigation-type reader, so the stub never reached the exercised code path
  and a reintroduced fallback reading the injected option would pass. Fixed:
  the test now passes `navigationType: "back_forward"` through the harness's
  injected reader (the actual seam the module uses) and the ineffective
  global stub and its cleanup were removed.
- Finding 2 (minor, accepted): the gate's Network-capture step could not
  observe the duplicated tab — it is a separate DevTools target that reloads
  synchronously, so source-tab DevTools never records its document request.
  Fixed: step 4 now uses executable post-reload proof from the duplicated
  tab's own console — `performance.getEntriesByType("navigation")[0]?.type
  === "reload"` — plus the archived-task UI check.
- No production-handler correctness defect found.

### Adversarial review round 8 (50-luna-review-fix)
- Finding 1 (minor, accepted): the cold-`back_forward` regression only
  covered a fallback using the injected navigation reader; a fallback calling
  the module's default `readNavigationType` (global performance) would stay
  green. Fixed: the harness gained `injectNavigationType: false`, and a
  companion test installs without the injected reader while stubbing global
  `performance` to report `back_forward` (with `performance.now` for
  happy-dom); the Risk text now describes both paths.
- Finding 2 (minor, accepted): step 1 declared `const t0` in the source tab's
  console, which is target-local and unavailable in the duplicated tab.
  Fixed: the start time is stored in sessionStorage
  (`kandev.bfcacheProbeStart`, copied to the duplicate) and the duplicated
  tab compares with a self-contained null-safe one-liner
  (`((JSON.parse(sessionStorage.getItem("kandev.bfcacheRestoreProbe") ?? "null")?.at ?? -1) >= Number(sessionStorage.getItem("kandev.bfcacheProbeStart")))`),
  after a follow-up review caught that the first revision referenced an
  unbound `at`, and a later review caught that the expression threw on an
  absent probe.
- Finding 3 (nit, accepted): the spec's client-storage contract only
  mentioned writes before reload, omitting the `persisted === false`
  `back_forward` diagnostic writes. Fixed: the contract now states the probe
  covers frozen restores AND persisted-false candidates, with only
  `persisted === true` causing a reload.
- No production-handler correctness defect found.

### PR review fixup (CodeRabbit / Claude / Greptile, PR #2717)

- CodeRabbit P2 (plan.md:108): the Wave-1 checkbox marked Task 01 complete
  while the native gate is open. Fixed: unchecked; it stays `[ ]` until the
  native result is recorded.
- CodeRabbit P2 (e2e:30): the reload assertion polled the navigation effect
  with a hand-picked 15s budget. Fixed: the E2E now arms a
  `framenavigated` causal wait before dispatching the restore signal and
  asserts the reload navigation type with the default timeout.
- CodeRabbit + Claude (unit tests): "fresh load" and "manual refresh" tests
  exercised identical paths. Fixed: they now pass
  `navigationType: "navigate"` / `"reload"` through the harness, guarding a
  reintroduced fallback keyed on either type.
- CodeRabbit (task-01:61): the gate's probe expression threw on an absent
  probe. Fixed: both step 3 and the round-8 record now use a null-safe
  expression that evaluates false (no-probe outcome recordable) instead of
  throwing.
- CodeRabbit (plan/spec): the spec claimed native Duplicate "still delivers
  `persisted`" — unverified. Fixed: the failure-mode bullet and the
  Duplicate-tab scenario now mark that outcome pending until the native gate
  is recorded; LanguageTool wording nit ("in the meantime") fixed.
- Claude suggestion: spec status `draft` → `building` (INDEX updated).
- Not actionable here: the merge-blocking native-Chrome Duplicate-tab
  verification requires the user's non-headless Chrome (right-click →
  Duplicate cannot be driven via CDP/Playwright); it remains the documented
  gate and stays open in the task/plan records.
