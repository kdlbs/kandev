---
created: 2026-09-11
status: complete
requirements:
  - REQ-INTEGRATIONS-GITHUB-MOBILE-001
  - REQ-UI-MOBILE-TASK-VIEWS-001
system_design:
  - ../../specs/integrations/system-design/github-dashboard-mobile.md
  - ../../specs/ui/system-design/mobile-task-view-access.md
legacy_specs: []
---
# GitHub mobile parity implementation plan

## Overview

Implements the assessment accepted by the user's explicit implementation request. The earlier Kandev task plan contains the source evidence; these work orders record execution. Integrations owns GitHub query/results behavior; UI owns the independently reusable task-view navigation.

## Scope and technical approach

Restore query load/save feedback, replace the GitHub hamburger with a Views drawer, expose existing task-sidebar views through shared navigation, improve touch rows, and replace phone pagination. Reuse existing state and primitives; no backend schema or provider API changes. Each work order owns its regression tests and locale changes.

## ASCII UI preview

Structural requirements are control order, separate collections, fixed drawer header/footer, internal scrolling, and conditional touch geometry. Labels/spacing are illustrative; product copy is localized.

Desktop retains its scope bar:
```text
[PRs | Issues] [Preset pills] [Saved queries v]
Title/count  [Repository] [Query] [Refresh]
Results
Count                    [Previous] [1 2 ...] [Next]
```

### UI-01: Saved-query recovery

```text
Loading views...
Unable to load views  [Retry]
Save query: [Name] [Repository]
[Cancel] [Save]  (pending prevents duplicates)
```

### UI-02: Mobile Views picker

```text
GitHub
[Views: Review requested v]
[Repository]
[Query]
Results 75           Updated just now [Refresh]
Drawer: Views                  [Done]
[Pull requests] [Issues]
  Inbox / Created / Saved (scroll)
[Save current query] (fixed footer)
```

### UI-03: Shared task-view entry

```text
App menu -> [Task views]
Task drawer: [Saved view v] [Filters]
Matching task -> /t/id
Browser Back -> /github
```

### UI-04: Touchable result actions

```text
Wrapped PR/issue title          [Task]
owner/repository#123
[Linked task   Workflow step]
```

### UI-05: Compact phone pagination

```text
Results (scroll)
101-125 of 1000+
[Previous] [Page 5 of 40 v] [Next]
Drawer: Choose results page       [Done]
  Page 4 of 40
  Page 5 of 40 (selected)
  Page 6 of 40
(one scrolling page list)
```

## Work orders

- [x] [Saved-query recovery](task-01-query-recovery.md) (complete)
- [x] [Mobile Views picker](task-02-view-picker.md) (complete)
- [x] [Shared task-view entry](task-03-task-views.md) (complete)
- [x] [Touchable result actions](task-04-result-actions.md) (complete)
- [x] [Compact phone pagination](task-05-pagination.md) (complete)
- [x] [Screenshot feedback refinements](task-06-toolbar-page-picker.md) (complete)

Sequential delivery: 01 -> 02 -> 03 -> 04 -> 05 -> 06. No delegated implementation. User-requested cross-repository planning tasks are tracked separately in the [integration follow-up inventory](integration-followups.md); they do not expand this implementation's scope.

## Tests and E2E

AC .1 uses saved-query hook/action regressions and mobile save/load recovery.
AC .2/.5 use mobile-github-sidebar.spec.ts and github-scope-bar.spec.ts.
The UI criteria use mobile-task-view-access.spec.ts and existing task-sheet navigation tests.
AC .3/.4 use mobile-github-results.spec.ts and mobile-issue-list-task-indicator.spec.ts.
Use configured Pixel 5 and desktop projects; assert actual hitboxes, no clipping/overflow, long names, and 767/768px transitions.

## Verification

Bootstrap completed: `(cd apps && pnpm install --frozen-lockfile)`.

Run each work order's commands from apps/web. Final shared checks:

```bash
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/github/github-scope-bar.spec.ts tests/github/issue-list-task-indicator.spec.ts tests/github/pr-list-task-indicator.spec.ts tests/github/pr-action-create-task-dialog.spec.ts)
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Run focused eslint on changed source/tests. The managed E2E runner builds fresh assets and isolates mock data; desktop and phone runs stay sequential.

## Risks and exclusions

Slow settings responses must not override local mutation or a new workspace. Shared task navigation must preserve browser history. Shared rows must retain other providers' desktop behavior. No new saved-query rename/edit feature, Office redesign, commits, push, or PR is included.

## Verification results

Completed 2026-09-11.

- 230 unit tests passed in 30 focused files covering the GitHub dashboard, shared integration rows/menus, navigation, and mobile task switching. The 22 navigation tests were rerun successfully after the orientation fix.
- 23 distinct Playwright scenarios passed: 12 GitHub/mobile scenarios, 7 desktop GitHub regressions, and 4 shared-navigation/task-view editing regressions. The final task-navigation run also verifies preserving a typed task draft when rotating from 393px to 851px.
- TypeScript typecheck, focused ESLint, translation completeness/pseudo synchronization and new-code ratchet, specification lint (including 36 validator tests), public-doc validators (46 pages), and whitespace checks passed.
- Production frontend assets built successfully with the existing Vite chunk warnings. No dependencies or backend contracts changed.
- Public how-to documentation updated: `docs/public/integrations.md`.

### Test environment and observed failures

Initial worktree dependencies were installed with the frozen lockfile. The backend build required `GOCACHE=/tmp/kandev-github-mobile-go-cache` because the default cache is read-only. The sandbox also blocks local listening sockets; isolated E2E runs used approved local-server access. Subsequent frontend-only runs rebuilt with `pnpm run build:e2e` and reused the unchanged backend through `pnpm e2e:run --host --no-build`.

Red regressions reproduced premature kind-switch dismissal, failed-save form closure, duplicate submission, missing loading/error/retry state, missing Task views navigation, a 24px issue task action, absent phone page selection, and loss of a child task draft on rotation. All are covered by passing regressions. A long-list geometry assertion now waits for the drawer's final position before measuring it; an old unconditional-truncation assertion was updated for the intended desktop-only behavior.

Implementation remains uncommitted. No push or PR was requested.

### Screenshot feedback refinements

Completed 2026-09-11 after the user's isolated-instance test. The long Views trigger now stays on one line, Results and updated/refresh metadata have their own row below the query, and the page chooser opens an in-app drawer instead of an operating-system select. The desktop pager remains unchanged.

- Six focused unit tests and 15 browser regressions passed: ten mobile GitHub, four desktop GitHub, and one shared GitLab mobile browse/review flow.
- Final typecheck, zero-warning focused ESLint, i18n completeness and new-code ratchet, specification lint with 36 validator tests, public-doc checks, and whitespace checks passed. Existing unused-locale-key notices and Vite chunk warnings remain non-blocking.
- Browser verification through the actual Tailscale test URL confirmed the long trigger is 44px high at both 320px and 393px, Results 75 and Updated just now occupy the status row, all three page choices work, focus returns, and there is no horizontal overflow. Light-theme captures were inspected.
- Updated only frontend assets in `/tmp/kandev-mobile-parity-test-vUutse/web-dist`, preserving old hashed assets and backing up the prior index. Backend PID 3052545 stayed running; neither the database nor in-memory mock provider data was reset. The main runtime on :9998 was not modified.

The disposable environment was stopped at the user's request on 2026-09-11. Its former URL was `http://100.105.155.17:48761/github`; direct Tailscale IP access had replaced the earlier Serve HTTPS route without restarting the backend. The owned direct-IP listener, backend, and agentctl were stopped through the guarded shutdown script. Ports 48761, 48762, and 48763 were verified closed. Data, logs, screenshots, and ownership metadata remain in `/tmp/kandev-mobile-parity-test-vUutse`; main :9998 and the integration follow-up tasks were untouched. Four user-requested repo-specific planning tasks are recorded in [integration follow-ups](integration-followups.md).

### Open PR delivery validation

Completed 2026-09-11 in the inherited workspace and branch. The user's explicit Open PR request supersedes the earlier phase's commit, push, and PR exclusions. Delivery retains all six accepted work orders, shared navigation/integration changes, specifications, and public documentation. Neither the stopped demo environment nor the personal instance was restarted.

The delivery regression run exposed a Views-to-save focus race. A failing browser regression reproduced the overlap; the save form now opens after the drawer's close/focus callback and returns focus to its initiating control on cancellation. Pending opening work is cancelled on unmount. The page-navigation regression now waits for finite drawer animations before measuring hitboxes and uses a bounded test-provider response while retaining the 1,050-result total and 1,000-result navigation cap.

Fresh task-scoped validation:

- 325 unit tests passed in 51 focused files. After the focus remediation, 29 focused unit tests passed again in four files.
- All 22 distinct browser regressions passed against the final production code across the final runs: 14 GitHub phone scenarios, seven GitHub desktop scenarios, and one shared GitLab phone scenario. The two results scenarios were rerun successfully after the test-provider correction; this is not a claim that the earlier combined run had no failures.
- Typecheck, zero-warning ESLint on changed source/tests, translation completeness and staged new-code ratchet, specification validation, public-doc validation, and whitespace checks passed. Existing unused-key notices and Vite chunk warnings remain non-blocking.
- E2E assets were rebuilt after the last production edit. All browser runs used the managed host runner, isolated mocks, one worker, and sequential suites; backend binaries were unchanged.
- Five fresh synthetic-data screenshots cover the phone dashboard, Views drawer, page chooser, Task views drawer, and desktop dashboard. The temporary capture spec is excluded from the delivery diff; compressed image assets are published separately from the feature branch.

Commands (frontend commands run from `apps/web`):

```bash
pnpm exec vitest run components/github/my-github components/integrations components/navigation components/task/mobile/session-task-switcher-sheet
pnpm exec vitest run components/github/my-github/save-preset-dialog.test.tsx components/github/my-github/use-saved-preset-actions.test.ts components/github/my-github/use-saved-presets-recovery.test.ts components/github/my-github/results-pagination.test.tsx
pnpm run typecheck
pnpm run i18n:check
pnpm run i18n:ratchet
pnpm run build:e2e
pnpm e2e:run --host --no-build --shards 1 --project mobile-chrome tests/github/mobile-github-results.spec.ts tests/github/mobile-github-view-recovery.spec.ts tests/github/mobile-github-sidebar.spec.ts tests/github/mobile-task-view-access.spec.ts tests/github/mobile-issue-list-task-indicator.spec.ts -- --retries 0
pnpm e2e:run --host --no-build --shards 1 --project mobile-chrome tests/github/mobile-github-results.spec.ts -- --retries 0
pnpm e2e:run --host --no-build --shards 1 --project chromium tests/github/github-scope-bar.spec.ts tests/github/issue-list-task-indicator.spec.ts tests/github/pr-list-task-indicator.spec.ts tests/github/pr-action-create-task-dialog.spec.ts -- --retries 0
pnpm e2e:run --host --no-build --shards 1 --project mobile-chrome tests/gitlab/mobile-gitlab-parity.spec.ts -- --grep 'browses, quick launches' --retries 0
```

Screenshot runs additionally enabled `CAPTURE_PR_ASSETS=1` and included a disposable `mobile-pr-delivery-capture.spec.ts`; its capture scenario passed. Repository-root specification and public-doc commands are listed in Verification above. Publication, CI, and automated-review state are tracked by the delivery task and PR; an open PR does not satisfy sibling work's merged-host dependency gate.

### PR fixup results (2026-09-12)

The later explicit user authorization permits reconciling main into PR #3614, fixing CI/review findings, and pushing this branch. It does not authorize merging the PR, queueing it, or restarting the stopped demo/personal instance.

Reconciled base `20efe4855a8d88c6f2a8a220be9abc03ca3a0d50` with PR head `6b22494e142754c8cccae2128450a2cc878f3177`. The task-switcher conflict retains main's extracted dialogs and move options together with optional-provider handling, focus return, and rotation-safe child drafts. The UI index retains both independent specification links.

All five inline findings and two additional CodeRabbit aggregate findings have scoped fixes: cached saved-query visibility; page-drawer focus fallback; stable Save label/status; desktop menu-to-save handoff; first-render workspace snapshot isolation; stale A-to-B-to-A save rejection; localized result count/plural/order. New regressions failed before their fixes. The desktop keyboard scenario passed before remediation, but the callback-order regression proved Save opened before menu focus restoration; the explicit deferred handoff and unmount cancellation now pass.

Fresh local validation:

- 338 unit tests passed in 51 scoped files using the original delivery Vitest command above.
- 17 phone scenarios passed using the original mobile command plus `tests/workflow/mobile-workflow-step-move-overrides.spec.ts`, with retries disabled. This includes page-count shrink recovery and all three move-option scenarios.
- 13 desktop scenarios passed using the original desktop command plus `tests/github/github-scope-bar-focus.spec.ts` and `tests/workflow/workflow-step-move-overrides.spec.ts`. The shared GitLab mobile browse/review scenario passed separately. All 31 final browser scenarios passed without retries; suites used one worker and did not overlap.
- Typecheck, zero-warning changed-file ESLint, i18n completeness/new-code ratchet, specification lint, and public-doc validators passed. Public how-to instructions remain accurate; these corrections clarify existing recovery, accessibility, and localization behavior rather than add a new user workflow.
- Harness validation passed: 19 harness-validator tests, all 196 harness files, 36 specification-validator tests, and the harness pre-commit hook. No manual harness changes were introduced.
- An initial mobile direct-move test observed stale running UI after an early idle-placeholder check. Three isolated runs and two complete original-order runs passed. The move fixture now waits for the real mock session to finish before navigation, then checks UI readiness; the final phone run passes without retries. This is a fixture readiness correction, not a task lifecycle change.
- Locale generation touched backend catalog timestamps without changing their contents. The managed freshness guard rejected stale artifacts; rebuilding backend and frontend resolved that guard before the successful phone run. Existing Vite chunk and unused-key notices remain non-blocking.

Fresh screenshot publication, normal-hook commit/push, current-head CI, review dispositions, and the minimum five-minute feedback hold remain in progress. Exact publication receipts and mutable PR state are tracked in the Kandev delivery task plan.
