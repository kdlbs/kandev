---
id: "04-browser-proof-and-docs"
title: "Prove browser behavior and document cloning"
status: done
wave: 4
depends_on:
  - 03-responsive-clone-flow
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-CLONE-001
  - REQ-WORKSPACES-CLONE-002
  - REQ-WORKSPACES-CLONE-003
acceptance_criteria:
  - AC-WORKSPACES-CLONE-001.1
  - AC-WORKSPACES-CLONE-001.2
  - AC-WORKSPACES-CLONE-001.3
  - AC-WORKSPACES-CLONE-001.4
  - AC-WORKSPACES-CLONE-001.5
  - AC-WORKSPACES-CLONE-001.6
  - AC-WORKSPACES-CLONE-001.7
  - AC-WORKSPACES-CLONE-002.4
  - AC-WORKSPACES-CLONE-003.1
  - AC-WORKSPACES-CLONE-003.2
  - AC-WORKSPACES-CLONE-003.3
  - AC-WORKSPACES-CLONE-003.4
  - AC-WORKSPACES-CLONE-003.5
system_design:
  - ../../specs/workspaces/system-design/clone-workspace.md
---

# Task 04: Prove browser behavior and document cloning

## Summary

Prove the complete desktop and phone clone flow through the production API and
UI. Publish concise instructions with actual copy boundaries only after the
feature works, then update this package's execution and specification statuses.

## In scope

- Apply `/e2e` and `/mobile-parity`; seed disposable source configuration through
  existing API fixtures, then clone through the UI without mocking success.
- Verify copied repository/workflow settings and GitHub PR/issue defaults via
  settings/dashboard DOM, reload persistence, empty history, and source isolation.
- Add phone coverage for the real drawer, cancel/focus, draft retention, pending
  double-tap guard, failure retry, long content, 44px targets and no page overflow.
- Arm HTTP/WS causal waits before actions. Use actual backend clone response to
  correlate the new ID. Keep shared fixture mutation cleanup explicit.
- Use full-store backend tests from Task 02 for security/rollback; do not expose
  tokens to browser fixtures or add artificial API-failure mocks as encryption proof.
- Apply `/docs-maintainer`: update the workspace portion of
  `docs/public/tasks-and-workflows.md`, and search root README/screenshots for
  conflicting claims. Keep walkthrough copy as a short how-to within that page.
- Record each task-defined command/result and compare final behavior with all
  requirement IDs before promoting paired drafts and marking the plan implemented.

## Out of scope

Broad local QA/review/verify passes, new screenshots or videos, performance
benchmarks, commits, pushes, PR creation, and unrelated E2E cleanup outside user-authorized PR CI remediation.

## Acceptance

1. Desktop and phone tests complete cloning with real persistence, show copied
   query defaults for both kinds after entry/reload, and prove no source mutation
   or task/session history copying.
2. Rendered phone checks match UI-02/UI-03, preserve form state across viewport
   changes, meet touch/containment/focus requirements, and handle failure retry.
   Unsupported sources have no executable clone action.
3. Public instructions describe actual includes/exclusions, credential reuse
   and personal sign-in, while all work orders record exact results and the
   requirement/design statuses accurately describe implementation.

## ASCII UI preview

Use the combined [UI preview](plan.md#ascii-ui-preview). Browser proof covers
001.7 and 003.1-.5, including the following structural checkpoints:

```text
UI-01 desktop | dialog then target overview
| From: source | Name [copy name] | Summary/exclusions |
|                                [Cancel] [Clone]    |
              -> cloned workspace settings overview

UI-02 phone | inset drawer, one internal scroll owner
    +-----------------------------------+
    | Clone workspace                   |
    | From: source                      |
    | Name [copy name               ]   |
    | Copy summary/exclusions (scroll)  |
    | [Cancel]              [Clone]     |
    +-----------------------------------+

UI-03 shared form | retry after definite failure
| Name [unchanged draft] | Error | [Cancel] [Clone] |
```

Check actual control bounding boxes, safe-area padding, focused cancellation,
and scroll owner. ASCII spacing is illustrative; labels are localized.

## Verification

Run from the repository root. Managed E2E builds fresh web/backend assets and
uses one worker per shard; run projects sequentially and confirm test discovery.

```bash
(cd apps/web && pnpm e2e:run --project chromium tests/settings/workspace-clone.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-workspace-clone.spec.ts)
(cd apps/web && pnpm exec eslint e2e/tests/settings/workspace-clone.spec.ts e2e/tests/settings/mobile-workspace-clone.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Invoke `.github/scripts/pr-docs.cjs`'s exported `validateCoverage` with the
actual changed-file list and the plan/work-order/requirement/design contents
to check local delivery traceability without publishing a GitHub status.
If a product failure requires a fix, rerun its affected task-defined check;
do not substitute a broad suite for the focused evidence.

## Files likely touched

- `apps/web/e2e/tests/settings/workspace-clone.spec.ts` (new).
- `apps/web/e2e/tests/settings/mobile-workspace-clone.spec.ts` (new).
- `apps/web/e2e/tests/session/transient-turn-runtime-continuity.spec.ts` and
  `apps/web/e2e/tests/task/mobile-task-switch-efficiency.spec.ts`,
  `apps/web/e2e/tests/settings/profile-capability-discovery.spec.ts` and
  `apps/web/e2e/tests/settings/mobile-profile-capability-discovery.spec.ts` and
  `apps/web/e2e/tests/task/mobile-directory-browser-hidden-folders.spec.ts`
  for user-authorized PR CI test repairs.
- `apps/web/e2e/helpers/workspace-clone.ts` (new only if shared setup warrants it).
- `apps/web/e2e/helpers/api-client.ts` only for necessary reusable fixture methods.
- `docs/public/tasks-and-workflows.md`.
- `docs/plans/clone-workspace/plan.md` and work-order Results/status fields.
- `docs/specs/workspaces/requirements/clone-workspace.md` and
  `docs/specs/workspaces/system-design/clone-workspace.md` lifecycle fields.

## Dependencies

Tasks 01-03 complete. Test design follows the packet from the first TDD pass;
this work order supplies final production-build browser evidence, not a broad audit.

## Risks

GitHub default queries differ from saved default views; seed and verify both.
Phone/project selection can silently omit tests. `e2eReset` does not restore
every shared configuration row, so prefer disposable workspaces and cleanup.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/workspaces/requirements/clone-workspace.md).
- [Design](../../specs/workspaces/system-design/clone-workspace.md).
- Workspace settings switcher E2E pattern, GitHub settings/default-query fixtures,
  causal wait helpers, and mobile drawer geometry tests.

## Results

Final production-build browser checks passed on 2026-10-08 (Europe/Lisbon):

- Desktop: 2/2 passed in 15.9s, log `/tmp/kandev-run.e2e.q9GAq44M.log`.
- Phone: 2/2 passed in 16.9s, log `/tmp/kandev-run.e2e.5jZD6k00.log`.
- Both flows create through the real API, read back copied repositories/workflows,
  preserve GitHub PR/issue defaults across reload, prove empty task history and
  source isolation, and leave the globally active workspace unchanged.
- Desktop covers cancel/focus and draft retention across phone/desktop resize.
  Phone covers inset-sheet geometry, 44px controls, no horizontal overflow,
  explicit secret/sign-in exclusions, blank-name blocking, cancel/focus, one
  pending POST and recovery after a definite failure.

After a reported machine interruption, browser projects ran sequentially in host
mode under a 3 GB memory cap, no swap, a two-core CPU quota and one worker. The
cause of the interruption was not established. Fresh web/backend/plugin artifacts
were built within the same resource limits before the final runs. Commands from
the repository root (with the installed Go/Node/pnpm on `PATH`):

```bash
systemd-run --user --scope --quiet --collect -p MemoryMax=3G -p MemorySwapMax=0 -p CPUQuota=200% pnpm --dir apps/web e2e:run --host --no-build --project chromium tests/settings/workspace-clone.spec.ts
systemd-run --user --scope --quiet --collect -p MemoryMax=3G -p MemorySwapMax=0 -p CPUQuota=200% pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome tests/settings/mobile-workspace-clone.spec.ts
```

Public clone instructions now describe the shipped includes/exclusions,
credential reuse, unchanged active selection and ambiguous-request recovery.
Public-document validation passed (62 tests and 47 pages); catalog validation,
36 specification-linter tests and all-file specification lint passed. Actual
changed-file documentation coverage accepts all four work orders with no errors.
Tracked and untracked whitespace checks passed. Requirements are now `active`,
the design `current`, and the plan `implemented`. Changes were uncommitted at
the implementation checkpoint; the subsequent user continuation starts commit
delivery with normal hooks.

### Prior compact-icon verification

The user-requested card refinement replaces the labeled button with a ghost
copy icon and localized tooltip. Phone placement moves into the title row.
After rebuilding the web assets, the same bounded, sequential host commands
above passed again:

- Desktop: 2/2 passed in 12.2s, log `/tmp/kandev-run.e2e.uEuTZz4V.log`.
  Assertions cover the 28px square action, tooltip, 44px size below the 768px
  breakpoint and 28px size at/above it, alongside real cloning and resize.
- Phone: 2/2 passed in 16.9s, log `/tmp/kandev-run.e2e.JfM4PPSE.log`.
  Assertions cover a square target of at least 44px aligned with the title,
  plus the existing real clone, sheet containment, cancellation and retry flow.

The focused desktop size regression failed against the old wide button before
the implementation changed. The public procedure and durable UI previews now
identify the compact copy icon. Screenshot publication remains a delivery step
outside this implementation work order.

### Workspace actions menu and page entry

After a fresh managed production build, the bounded host commands above passed:

- Desktop: 3/3 passed in 20.1s, log `/tmp/kandev-run.e2e.Qtne9tI6.log`.
  The 2048px list check keeps the menu beside the workspace name. Keyboard
  activation, breakpoint sizes, shared draft and cancel/focus return pass. A
  new page-header flow clones the viewed workspace from Repositories and
  verifies the saved configuration and query defaults through real persistence.
- Phone: 3/3 passed in 28.6s, log `/tmp/kandev-run.e2e.vWfqRQoO.log`.
  The Active header fits at 320px. List/page actions and menu rows retain 44px
  targets; menus and sheets stay contained. The new Workflows page-header flow
  clones the viewed workspace and verifies persistence and query defaults.

The page-action regression failed because the trigger was absent before the
implementation. Initial browser runs exposed test-fixture assumptions: the
shared verifier expects a fixed clone name, and target geometry must be measured
after finite opening animations. The final runs use that fixture name and wait
for animation completion without adding timed sleeps. No application change
was needed for those assertion corrections.

The shared clone hook/form moved to domain hook/component owners. 33 targeted
unit tests, scoped lint, TypeScript and all i18n gates passed. Durable previews
and public instructions cover both menu entry points and saved setup semantics.
Public-page, catalog, specification lint, changed-file coverage and whitespace
validation passed. The plan is implemented and all work orders remain done.

Final visual inspection caught a desktop menu label wrapping at the shared
trigger width. The menu now fits its label within viewport bounds. The
desktop regression fails against the former wrapping row and passes with
the final content width; the latest desktop and phone results above include
this adjustment. Fresh screenshots are recaptured after the final commit.


### PR CI browser-test repairs

The user requested CI and review remediation for PR #4311. At head
`9adb6da3b300a93d998877a33f0d87e10d927562`, E2E run `37713048747`
failed shard 10/14; report and aggregate gates failed because of that shard.
The desktop completed-tools continuity test required the transient retry card
after reload/second-viewer navigation, although the five-second continuation
had already completed. Local reproduction at exact CI merge
`0696a3a4f3e5724d1411a98dff8235de42a31b98` also rejected the valid
"Waiting to reconnect" phase because it expected "Continuing".

The test now verifies history preservation initially and completed continuation
in both viewers. Exact runtime/process/ACP session, prompt-count, completed
side-effect and execution-ID checks remain. No production logic or timeout
changed. RED log: `/tmp/kandev-run.e2e.klRp67mG.log`; GREEN: four repetitions,
retries disabled, 1.2m,
`/tmp/clone-fixup-e2e-local/kandev-run.e2e.QzGRfErS.log`.

The same shard recorded one flaky mobile PR drawer close. Its original failed
attempt tapped Close during the entrance animation: visibility assertion began
at 1791428011335 and Close tap at 1791428011370. The test uses the existing
finite-animation helper before the native Close tap; the drawer-removal
assertion and timeout remain unchanged. Before this change, the exact case
passed four local repetitions, and the full spec passed twelve repetitions
under a one-core CPU cap. The corrected mobile case passed the full shard replay; final CI remains pending.

All browser reproductions use disposable fixture data and fresh production
assets in an owned detached checkout, one worker, CI mode, no retries,
a 4 GB memory limit, no swap and bounded CPU. Source/product behavior remains
unchanged; these repairs remove assumptions about transient UI timing.


The shard replay also exposed a profile-discovery assertion capturing the
raw `mock-fast` label before model names hydrated, then rejecting `Mock Fast`.
`ModelPicker` intentionally displays a configured ID until its catalog entry
arrives, while the fixture keeps the same selected model. Desktop success/
failure update tests and the corresponding phone draft helper now await the
fixture's resolved `Mock Fast` label before capturing their before-update value.
Selection-preservation and persisted-model assertions are unchanged. The
original failed shard replay is the RED evidence; focused GREEN is recorded below.


The same replay also measured the phone hidden-folder touch control before
its popover's scale animation settled (1.718px difference from the expected height).
The first case now uses the same existing finite-animation wait as the second
case before measuring. The 44px target, original 1px tolerance and native
touch/selectability assertions remain unchanged. Focused GREEN is recorded below.


Shard replay completed with retries disabled: 245 passed, one skipped and the
two additional profile-label/popover-geometry failures in 36.8m, log
`/tmp/clone-fixup-e2e-local/kandev-run.e2e.fiwfzzAw.log`. The original
continuity and PR-drawer failures both passed in the exact shard file order.
The manifest retained its 91 files, both projects, 1266 catalog units and
assignment/order; only candidate source hashes/checksum were refreshed and
validated before the managed runner. RED contexts/blob remain in the owned
`/tmp/clone-fixup-shard10-red-artifacts` directory.

After adding the label/geometry readiness guards, all three affected specs
passed four repetitions across their desktop/phone projects: 40/40, retries
disabled, 3.5m, log
`/tmp/clone-fixup-e2e-local/kandev-run.e2e.tKjoUxXG.log`. All five edited
browser specs pass scoped ESLint and Prettier; web TypeScript passes. Catalog validation (362
decisions/1438 specifications), specification lint, actual changed-file
coverage and whitespace checks pass. Final remote CI remains pending.

Commands from the disposable exact-CI checkout, with installed Go/Node/pnpm
on PATH and CI=true, GOMAXPROCS=2, GOMEMLIMIT=1GiB, GOGC=30 and
NODE_OPTIONS=--max-old-space-size=3072:

```bash
systemd-run --user --scope --quiet --collect -p MemoryMax=4G -p MemorySwapMax=0 -p CPUQuota=200% scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --project chromium -- e2e/tests/session/transient-turn-runtime-continuity.spec.ts --grep 'completed tools continue once' --retries=0 --repeat-each=4
systemd-run --user --scope --quiet --collect -p MemoryMax=4G -p MemorySwapMax=0 -p CPUQuota=200% scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --project chromium -- --project=chromium --project=mobile-chrome e2e/tests/settings/profile-capability-discovery.spec.ts e2e/tests/settings/mobile-profile-capability-discovery.spec.ts e2e/tests/task/mobile-directory-browser-hidden-folders.spec.ts --retries=0 --repeat-each=4
```

The full shard replay first validated the refreshed downloaded manifest with
`readManifest`/`resolveShard` from `run-planned-shard.ts`, then passed all
shard-10 files to the same guarded managed runner with both projects and
`--retries=0`. Subsequent changes only add readiness guards to the three
locally failing specs; the passing original cases need no additional rerun.

## Additional shard-5 CI-remediation proof

The exact current-head shard replay passed the reported managed-relocation
case but exposed two further failures (243 passed, two skipped, two failed in
51.4m, retries disabled). `executor-profile-routing.spec.ts` globally matched
both the expanded sidebar profile link and its card. Reuse the exact
`executor-profile-card-<id>` locator for the legacy executor entry point; retain
all canonical-path, editor, persisted-save and leave-confirmation assertions.
The runtime idle-cancellation failure is owned by the existing platform work
orders under `docs/plans/transient-turn-runtime-continuity/`; their active
requirements and historical results are preserved. Focused managed Playwright GREEN: 9/9
tests passed in 9.8m, three repetitions
per affected case with retries disabled after a fresh production build. Log:
`/tmp/clone-fixup-e2e-local/kandev-run.e2e.AN57mwam.log`. Scoped lint and current-base
composition passed as recorded below.
Final current-head delivery remains pending externally.

Final local checks: scoped Go lint reported zero issues; all three browser
specs passed ESLint and Prettier. A conflict-free merge with the latest main
passed 124 clone/routing/editor tests across 12 suites, 24 archived-sidebar
freshness tests, and web TypeScript. The original 13-suite command supplied
an incorrect sidebar path and ran only 12 suites; the sidebar suite then ran
separately at `lib/sidebar/sidebar-archived-update-freshness.test.ts`. Logs:
`/tmp/kandev-run.vitest.21UlgosQ.log`,
`/tmp/kandev-run.vitest.p4wCbkEc.log`,
`/tmp/kandev-run.typecheck.2e9Djqxo.log`.
Actual changed-file work-order coverage passed with all unchanged referenced
platform requirement/design inputs loaded. Full catalog/specification and
whitespace checks passed. The primary session records exact merge/head IDs,
normal hook receipts and fresh remote CI/review results in the external task
plan, avoiding a documentation-only push that would invalidate those results.
