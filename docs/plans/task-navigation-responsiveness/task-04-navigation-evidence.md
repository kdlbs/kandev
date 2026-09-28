---
id: "04-navigation-evidence"
title: "Verify navigation and investigate retained memory"
status: done
wave: 4
depends_on:
  - "01-overview-hydration"
  - "02-shared-session-reads"
  - "03-progressive-file-trees"
plan: "plan.md"
requirements:
  - REQ-UI-TASK-NAVIGATION-RESPONSIVENESS-001
acceptance_criteria:
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.1
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.2
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.3
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.4
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.5
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.6
system_design:
  - ../../specs/ui/system-design/task-navigation-responsiveness.md
---

# Task 04: Verify Navigation and Investigate Retained Memory

## Summary

Prove the repaired behavior through real desktop and phone navigation, then
compare isolated production-build timings and retained memory. Separate proven
improvements from the live instance questions that cannot yet be attributed.

## In scope

- Causal browser tests with request correlation and controlled response holds.
- Before/after measurement using disposable fixture data, including debug on/off.
- Browser/backend memory evidence, resource cleanup, and an honest residual-risk
  report. This is targeted verification for the diagnosed navigation issues.

## Out of scope

Broad QA, live-browser automation, reading live credentials, changing the user's
database/settings, stopping unknown processes, deployment, or unproven leak fixes.

## Acceptance

1. Desktop and phone regressions cover overview hydration, shared requests,
   progressive/warm Files, one failed branch/retry, and rapid task/workspace
   changes. Assertions are causal, not arbitrary waits. Existing geometry,
   touch actions, and workspace isolation still pass (`.1`-`.6`).
2. Record matched baseline/fixed production measurements, request counts, and
   separate usable-content/restoration/main-thread timings. Debug on/off is a
   controlled comparison. No subjective speed claim substitutes for evidence.
3. Record bounded cache behavior and comparable quiescent memory samples,
   identifying retained owners if growth reproduces. State limits when it does
   not reproduce or profiling is unavailable; do not label RSS alone a leak.
   Release all fixture-owned processes/files and update package results.

## Regression sequence

Add `task/task-navigation-responsiveness.spec.ts` and its `mobile-` counterpart.
Use `fixtures/test-base.ts`, existing task/page helpers, and real task selection.
Adapt `helpers/ws-response-hold.ts` into a targeted new
`helpers/navigation-response-hold.ts`, forwarding unrelated and newline-batched
frames unchanged. Unit-test frame splitting/correlation in
`helpers/navigation-response-hold.test.ts` if extracting nontrivial parsing.

Assert these cases with two tasks and multiple workflows:

- Hold one workflow's snapshot; available lanes render and filters remain scoped.
- Hold matching shell/commit/diff reads while multiple consumers mount; observe
  one per actual request key. Release, then invalidate and observe one refresh.
  Exclude periodic history reads and real subsequent git invalidations.
- Hold a restored child's response. Root/available sibling rows appear and open
  before release. Observe parent-first order and the active-owner concurrency
  limit without serializing independent branches.
- Return A-to-B-to-A with a retained tree. Hold its refresh and assert the correct
  A rows are usable; a deliberately late old response cannot replace them.
- Fail one expanded branch transiently, retain usable siblings and expansion
  intent, then retry successfully. Contrast an authoritatively deleted branch.
- Phone: use the actual task chooser and Files navigation, open a file, reach a
  visible secondary action, and check touch-target/viewport containment and no
  horizontal overflow. Compare UI-01/UI-02 in the [plan](plan.md#ascii-ui-preview).

Run the new failing scenarios against an isolated baseline checkout before
accepting them as regressions, copying only owned test fixtures as needed.
Retain that checkout only for the comparison and remove only owned resources.
Do not revert the repaired working tree or touch the live instance.

## Profiling procedure

Create opt-in `task/task-navigation-profile.spec.ts`, gated by
`KANDEV_NAVIGATION_PROFILE=1` (test-only). Use the normal managed fixture and
attach JSON measurements/traces to test output. No production timing threshold
or public configuration key is introduced.

Seed 32 tasks and a representative active task with 73 changed files, 21 expanded
folders (including nested and independent branches), and at least 600 tree rows.
Include a substantial conversation using supported fixture helpers, recording
the actual message count. Avoid copying the user's database. Warm task content,
then run ten A/B cycles (20 selections) with identical data on baseline and fixed
production builds. Record hardware/load, revision, build, browser, and mode.

For each selection record input-to-selected-task, input-to-first-usable-content,
all-expanded-folders completion, action/key request counts, and long tasks/main
thread time. DOM-ready/paint proxies must be labeled accurately. Measure normal
runs without CPU-profiler startup in the timed window; capture a separate trace
to attribute hot work. Keep debug logging on/off comparisons separate from
baseline/fixed comparisons, using the fixture's `backend.useEnv()` and cleanup.

Use fixture-owned `backend.pid()` and API origin for memory samples. In the
debug-enabled isolated case, collect authenticated pprof/expvar through the
test-owned auth context if available; never extract the live instance's token.
Compare quiescent Go heap samples at the same forced-GC policy before and after
repeated cycles, separately recording RSS. Browser retained heap/GC sampling
uses the isolated Chromium context and must not be included in timing windows.
Exercise more than four visited environments and 32 inactive read keys to test
new cache budgets; report retained object owners and released listeners/timers.

If the original approximately 3.8GiB backend RSS does not reproduce, record
that limitation and the isolated evidence. Do not close it as a fixed leak.
If a separate retaining path does reproduce, state the root cause and update
its owning design/work order before a production correction. A surviving
Firefox-only symptom needs a focused isolated Firefox trace; Chromium results
alone do not establish the original browser's performance.

## Verification

From the repository root, run these sequentially after the plan's bootstrap:

```bash
(cd apps/web && pnpm exec vitest run e2e/helpers/navigation-response-hold.test.ts)
(cd apps/web && pnpm exec eslint e2e/helpers/navigation-response-hold.ts e2e/helpers/navigation-response-hold.test.ts e2e/tests/task/task-navigation-responsiveness.spec.ts e2e/tests/task/mobile-task-navigation-responsiveness.spec.ts e2e/tests/task/task-navigation-profile.spec.ts)
(cd apps/web && pnpm run lint:e2e-sleeps -- e2e/helpers/navigation-response-hold.ts e2e/tests/task/task-navigation-responsiveness.spec.ts e2e/tests/task/mobile-task-navigation-responsiveness.spec.ts e2e/tests/task/task-navigation-profile.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --project chromium tests/task/task-navigation-responsiveness.spec.ts tests/task/workspace-switch-sidebar-isolation.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-task-navigation-responsiveness.spec.ts tests/task/mobile-workspace-switch-sidebar-isolation.spec.ts)
(cd apps/web && KANDEV_NAVIGATION_PROFILE=1 pnpm e2e:run --project chromium tests/task/task-navigation-profile.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the profile command on the isolated baseline and fixed checkouts with the
same owned test harness; record exact revisions and discovered test counts.
Do not overlap managed suites or add worker overrides. The frame-parser test
path is part of this work order, not an existing test claimed as already run.

## Files likely touched

- `apps/web/e2e/helpers/navigation-response-hold.ts` and `.test.ts` (new)
- `apps/web/e2e/tests/task/task-navigation-responsiveness.spec.ts` (new)
- `apps/web/e2e/tests/task/mobile-task-navigation-responsiveness.spec.ts` (new)
- `apps/web/e2e/tests/task/task-navigation-profile.spec.ts` (new, opt-in)
- `apps/web/e2e/tests/task/task-navigation-helpers.ts` (new)
- `apps/web/e2e/tests/task/task-navigation-profile-helpers.ts` (new)
- `docs/public/tasks-and-workflows.md` for progressive Files and Retry guidance
- These work orders and `plan.md` for final evidence/status
- Diagnostic artifacts under ignored `.kandev/diagnostics/`, excluding secrets

## Dependencies

Tasks 01-03. All deterministic corrections must be present for fixed-build
measurement; the isolated baseline remains available for RED/comparison.

## Risks

Synthetic data cannot reproduce every historical instance condition. Profiling
and debug logging change measured work, so label modes and separate traces from
timing runs. Account for legitimate refreshes and uncancelable obsolete requests
instead of enforcing a misleading global request-count target.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/task-navigation-responsiveness.md)
- [Validation design](../../specs/ui/system-design/task-navigation-responsiveness.md#observability-and-validation)
- `e2e/helpers/ws-response-hold.ts` and `e2e/helpers/causal-waits.ts`
- Existing file-tree lazy-load, virtualization, and workspace-isolation tests
- `.kandev/diagnostics/responsiveness-report.md` for measured baseline and limits

## Results

- Behavioral RED on the isolated original build: Files rows remain hidden behind
  a held child response; mounted panels issue 3 shell, 4 commit, and 3 diff reads
  for identical keys instead of one. Overview's missing-cache TypeError is
  reproduced by the real-store hook tests. The browser boot prehydrates its
  snapshots, so the overview E2E covers live workflow filtering, not that RED
  race; the earlier delayed-snapshot fixture was replaced after its setup
  assertion showed that no snapshot request was being held.
- Fixed browser coverage passes for progressive root/sibling visibility, failed
  branch retry, warm return while refresh is held, opening the retained file,
  shared reads, workflow filtering, and desktop workspace isolation. Desktop
  lazy-load/virtualization contributes five additional passing cases. All five
  final phone cases passed: new navigation/retry/open-file, large-tree window,
  visible folder actions, 44px coarse-pointer targets, and workspace isolation.
  There are 16 distinct passing desktop/phone behavior cases in total.
- Final production-build profile cases passed: baseline and fixed with debug off
  and on, 20 selections each. Each uses 32 tasks, 21 folders, 630 base tree files,
  73 changed files in each active worktree, and 205 messages per conversation.
  Debug-off median usable-file DOM time fell from 3,741 ms to 1,991 ms; all-folder
  restoration fell from 3,759 ms to 2,125 ms. Debug-on usable DOM was 3,727 ms
  versus 1,676 ms. URL selection did not improve consistently. These are driver/DOM proxies,
  not physical-input/compositor paint measurements.
- Warm navigation made 900 baseline versus 700 fixed requests across 20
  selections in either mode (45 versus 35 per selection). All 440 tree reads
  remained. The removed reads were 60 shell, 80 commit, and 60 diff snapshots;
  invalidation and cold shared-read behavior are covered independently.
- Debug-off long-task duration fell from 46.20 s to 28.91 s across 20 selections;
  CDP main-thread task time fell from 65.41 s to 42.84 s. Separate CPU profiles
  were captured outside timed windows. Minified rendering/translation/layout
  work remains; no new source-level rendering defect is claimed from those
  profiles. The debug-mode comparison does not support a large consistent
  logging penalty or a live-settings change.
- In isolated debug-on runs, forced-GC Go HeapAlloc was 14.68 → 14.95 MiB on
  baseline and 14.83 → 15.21 MiB on fixed. Browser used heap grew by a few MiB
  in both builds. Backend RSS ranged around 866-1,147 MiB across runs, without
  reproducing the live approximately 3.8 GiB observation. Sampled pprof changes
  include small SQL reflection caches and regex buffers; this is not a demonstrated leak.
  Debug-off profiler routes return HTML and are recorded as unavailable.
- Cache tests exercise five tree identities and 33 inactive read keys. They
  verify retention budgets, preservation of active requests, scope retirement,
  and detached retry cleanup. This does not establish bounds for all existing
  app storage. Live history and Firefox remain outside the synthetic evidence.
- Raw JSON, CPU/Go heap profiles, exact machine/build metadata, and detailed
  methodology remain in ignored `.kandev/diagnostics/navigation-profile/` and
  `.kandev/diagnostics/navigation-implementation-evidence.md`. No production
  database, credentials, settings, or live processes were changed.
- Targeted 95-test regression run plus seven corrected Changes badge integration
  tests and two environment-rebinding regressions, TypeScript, changed-file
  ESLint/Prettier, i18n checks, public-doc validation, and actual PR documentation coverage all
  passed. Playwright discovery found 3,597 specs with no discovery errors.
  The owned baseline checkout was removed after copying its evidence, and no
  managed fixture runtime processes remain after the final runs. The full
  frontend command completed with 20,365 passed, four skipped, and seven failures
  confined to the obsolete Changes badge mock. All seven pass after migration
  to the real store. Two subsequent ownership regressions also pass, along with
  all 51 affected coordinator/hook/handler tests. Browser and profile runs were
  refreshed for the final environment-mapping guard: all six desktop cases,
  both affected phone cases, and both fixed profiling modes pass. No production
  changes followed these runs. Typecheck, changed-file lint/format checks,
  spec validation, and whitespace checks pass on the final changes.

The complete package sweep was not repeated after the focused fixture and
ownership corrections; the affected tests, static checks, and browser/profile
runs above were rerun on the final implementation.
