---
created: 2026-10-20
status: implemented
requirements:
  - REQ-TASKS-LIST-ARCHIVE-FILTERS-001
  - REQ-TASKS-LIST-ARCHIVE-FILTERS-002
  - REQ-TASKS-LIST-ARCHIVE-FILTERS-003
  - REQ-TASKS-LIST-ARCHIVE-FILTERS-004
system_design:
  - ../../specs/tasks/system-design/task-list-archive-filters.md
legacy_specs: []
---

# Implementation plan: Task list archive access and filters

## Overview

Add global archive navigation and make List filters visible and predictable.
Deliver the archive destination and query integrity first. Then organize the
filter controls around that query model. Each work order includes its tests.
Execute sequentially in the primary session after an explicit implementation request.

The source contracts are the
[requirements](../../specs/tasks/requirements/task-list-archive-filters.md) and
[system design](../../specs/tasks/system-design/task-list-archive-filters.md).
The user requested implementation after the design-package handoff on 2026-10-20.
Production changes and focused tests are authorized. After manual validation,
the user requested a ready PR, then CI and review remediation.

## Planning assumptions

- Improve existing search, workflow, and repository filters. This package
  excludes additional task criteria.
- **Go to archived tasks** opens the whole current-workspace archive.
  Automatic navigation overrides do not change saved board scope.
- Manual workflow/repository edits retain existing portable behavior. Clear
  filters clears those choices while retaining archive mode and display preferences.

The implementation retains these package scope choices.

## Scope

### In scope

- A global archive command, explicit archive modes, and link restoration.
- One query model for request parameters, scope, paging, and response ownership.
- A desktop filter toolbar and phone filter drawer with visible applied scope.
- Targeted tests, all supported locale catalogs, and public how-to updates with implementation.

### Out of scope

Exclude new filter criteria, saved List views, task lifecycle changes, and
command-panel task-result ranking. Also exclude backend capabilities, database
migrations, new packages, and changes to board, Threads, sidebar, or plugin contracts.

## Technical approach

### Archive destination and query integrity

- Register `nav-archived-tasks` in `GlobalCommands`. Use
  `linkToArchivedTasks` helper in `lib/links.ts`. Keep it separate from the
  task mutation command and top-level destination manifest.
- Introduce `lib/tasks/tasks-list-query.ts` and
  `hooks/use-tasks-list-query-state.ts`. Distinguish omitted scope from explicit
  `all`. Keep URL restoration free of portable-setting writes.
- Replace `showArchived` throughout the Tasks path with an archive mode.
  Show its shared control above rows on both viewports. Remove the old phone
  archive checkbox while retaining View options sort/group fields.
- Use the existing `listTasksByWorkspace` archive flags and server totals.
  Gate boot rows in `TasksDataRoute`. Retain the existing `shouldSkipInitialTasksFetch`
  policy behind that eligibility gate.
  Use query identity plus request generation for publication. Reset page and
  filters together. Do not patch the obsolete async Tasks page instead of the SPA.
- Give List controls the effective query. Suppress the duplicate built-in scope
  fields in shared Tasks display surfaces. Preserve ordinary manual filter saves.

### Filter controls and feedback

- Add a feature toolbar through `TasksListControls` and `TasksPageContent`.
  `tasks-list-filters.tsx` and `tasks-list-filters-drawer.tsx` share
  query values and actions. Extract small components to meet frontend limits.
- Move List search into the toolbar. Suppress its old header and phone-toggle
  entry only for Tasks. Keep Board and Threads paths intact.
- Move workflow/repository controls out of Tasks display settings. Preserve
  plugin contributions, sort/group facets, row details, and other view options.
- Add applied indicators, targeted clear actions, stable read feedback, Retry,
  and last-page recovery after removals. Keep archive mode visible in all states.
- Complete translations and update `docs/public/tasks-and-workflows.md`.

Use existing `@kandev/ui`, `useResponsiveBreakpoint`, and sizing helpers.
Follow the mobile and polish contract in the system design. Do not add new
state storage, shared global CSS, animation systems, or lint exclusions.

## UI preview

The active task instructions require structured previews through Kandev rich
output. These labelled rows replace ASCII sketches. Order, grouping, state,
and scroll ownership are structural. Example names and spacing are illustrative.
All product text uses locale keys at implementation time.

### UI-01: Command panel, archive navigation result

Entry: Cmd/Ctrl+K, then search for archived tasks. State: current workspace resolved.

| Order | Content | Behavior |
| --- | --- | --- |
| 1 | Existing command-panel search field | Retains command search and keyboard behavior |
| 2 | Navigation group: archive icon, Go to archived tasks | One command opens the explicit workspace archive |
| 3 | Other matching commands and task results | Existing ordering remains, including Archive task precedence |

No workspace: show the archive command disabled. The existing panel owns scrolling.
Coverage: `AC-TASKS-LIST-ARCHIVE-FILTERS-001.1` through `.4`.

### UI-02: Desktop List, Archived selected

Entry: archive command or explicit list link. State: query accepted.

| Region | Controls and content in order | Structure |
| --- | --- | --- |
| Shared header | Existing navigation, List context, view controls | Keep existing header composition |
| Toolbar row 1 | Active / **Archived** / All, Search tasks, result total | Archive selection stays visible. search grows |
| Toolbar row 2 | Workflow selector, Repository selector, Clear filters. Sort, Group | Scope first. display options grouped at the other end |
| Applied scope | Removable workflow and repository indicators | Wrap. omit All indicators |
| Results | Task rows, then pagination | One list-body scroll owner |

Ordinary desktop controls measure 28px at the standard root font. The second
row can wrap without clipping or moving display controls into scope fields.
Clear filters appears only when search or scope restricts the result.
Coverage: `AC-TASKS-LIST-ARCHIVE-FILTERS-002.1` through `.5`, `003.1`, `003.3`.

### UI-03: Phone List and Filters drawer

Entry: phone Tasks navigation or archive link. State: one selected workflow.

| Region | Controls and content in order | Structure |
| --- | --- | --- |
| Shared header | Workspace/List context, app navigation | Existing compact phone header |
| List controls | Active / **Archived** / All. Search tasks beside Filters (1) | Two rows. archive mode and search stay visible |
| Applied scope | Workflow: Development, remove action | Wrap. no horizontal chip scroller |
| Results | Single-column task rows, pagination | Main vertical list content |
| Open Filters drawer | Fixed Filters heading. Workflow, Repository, Clear filters. Done footer | Inset bottom drawer, one internal scroll body, safe-area footer |

Fields apply immediately. Done dismisses and restores focus to Filters.
Sort, Group, and row details remain in View options. Phone controls measure at
least 44px, including at narrow fine-pointer widths. Done uses nominal 48px sizing.
Coverage: `AC-TASKS-LIST-ARCHIVE-FILTERS-003.2`, `.3`, `.5`.

### UI-04: Result feedback with controls retained

| State | Result region | Available recovery |
| --- | --- | --- |
| Current read pending | Loading status. no previous-scope rows or current total | Controls remain usable |
| Archive empty with no list filters | No archived tasks | Other archive modes remain visible |
| Filtered result empty | No tasks match these filters | Clear filters, preserving archive mode |
| Current read failed | Error with stable Retry action | Retry the current query |
| Unavailable workflow/repository | Identify the unavailable field | Clear or replace it. do not broaden automatically |

Coverage: `AC-TASKS-LIST-ARCHIVE-FILTERS-003.4`, `004.2`, `004.5`.

## Tests

The following tests cover the implemented behavior. Behavioral changes use TDD.
Pure presentation is checked in the browser instead of adding markup-only unit tests.

### Archive destination and membership

| Acceptance criteria | Test evidence |
| --- | --- |
| `001.1`, `001.4` | `components/global-commands.test.tsx`: global command availability, active-workspace changes, disabled without a workspace |
| `001.2`, `001.3` | `components/global-commands.test.tsx`: current-workspace archive href. `hooks/use-tasks-list-query-state.test.tsx`: explicit All overrides saved scope without saves |
| `001.5` | `e2e/tests/task/task-list-archive-navigation.spec.ts`: selecting an archived row opens existing detail without session admission |
| `002.1`, `002.2` | `lib/tasks/tasks-list-query.test.ts`: three archive mappings. browser fixtures with matches beyond page one prove server totals and intersections |
| `002.3` | `hooks/use-tasks-list-query-state.test.tsx`: filters reset page atomically. `task-list.spec.ts`: page-size reset and full-set search totals |

### Filtering and presentation

| Acceptance criteria | Test evidence |
| --- | --- |
| `002.4`, `002.5` | `hooks/use-tasks-list-query-state.test.tsx`: clear/remove semantics, preserved display fields, manual portable saves versus restoration |
| `003.1`, `003.3` | `task-list-filters.spec.ts`, `mobile-task-list-filters.spec.ts`, and `kanban-display-dropdown.test.tsx`: effective scope, indicators, count, and duplicate-control removal |
| `003.2`, `003.5` | Phone browser scenarios plus desktop keyboard flow and real geometry measurements |
| `003.4` | `app/tasks/tasks-page-client.query.test.tsx`: pending/error/current-query Retry. Query-status tests and browser tests cover unavailable/empty feedback |

### Navigation and concurrency

| Acceptance criteria | Test evidence |
| --- | --- |
| `004.1` | `hooks/use-tasks-list-query-state.test.tsx`: history reconciliation and query-only navigation. browser reload/history scenarios |
| `004.2` | `src/spa-routes.tasks-preferences.test.tsx`, `app/tasks/tasks-page-fetch-policy.test.ts`, and deferred page-client tests: incompatible boot data and late response suppression |
| `004.3` | Query-state and deferred data-hook tests: retained mode when List stays open, cleared scope, preserved Home navigation, no previous-workspace rows |
| `004.4` | Data-hook tests: retained refresh callbacks use current identity, Retry settles, and final-page recovery chooses the remaining page. Existing mutation and pull-to-refresh callers share that callback |
| `004.5` | Query parser tests, metadata-read failure/deferred readiness tests, and browser unavailable-scope and current-read failure scenarios |

Existing `lib/api/domains/kanban-api.test.ts` covers `only_archived` serialization.
Retain that coverage rather than duplicating its implementation in a new mock.
Retain `homepage-commands`, list row, workflow-step grouping, and command archive
tests to protect adjacent behaviors affected by control relocation. Keep the existing
archived-conversation navigation tests unchanged; the new List scenarios use separate files.

## E2E tests

| File and project | Scenario | Criteria |
| --- | --- | --- |
| New `task/task-list-archive-navigation.spec.ts`, `chromium` | Open archive from task, list, and settings. Check workspace, saved filters, Archived selection, reload/history, and read-only row navigation | `001.1` through `.5`, `002.1`, `004.1`, `004.2` |
| Existing `task/command-palette-archive.spec.ts`, `chromium` | Existing Archive task action keeps precedence and confirmation | `001.1` compatibility |
| Existing `task/task-list-filters.spec.ts`, `chromium` | Combine archive/search/workflow/repository. Check clear/remove, saves, feedback, unavailable scope, and breakpoint geometry. The existing task-list pagination test checks a match beyond page one | `002.1` through `.5`, `003.1`, `003.3`, `003.4`, `004.2` through `.5` |
| New `task/mobile-task-list-archive-navigation.spec.ts`, `mobile-chrome` | Open List through existing View options and tap Archived. Also open the palette with Control+K and tap the archive command. Check reload and row navigation | `001.1` through `.5`, `002.1`, `004.1` |
| New `task/mobile-task-list-filters.spec.ts`, `mobile-chrome` | Permanent search, immediate drawer filtering, clear/remove, Done focus return, empty/error/Retry, containment, one scroll body, and touch geometry | `002.2` through `.5`, `003.2` through `.5`, `004.2` through `.5` |

Use canonical Pixel 5 settings in `mobile-chrome`. In the desktop filter spec,
also check 767px and 768px with its fine pointer. Include one coarse-pointer
tablet check in the phone filter spec without changing the configured device.
Measure hitboxes and overflow after finite overlay animations settle.
Inspect desktop and phone screenshots during these focused runs.

Seed through isolated API fixtures. Preserve and restore changed user settings.
Use causal HTTP/WS waits armed before actions and deferred responses for races.
Do not add fixed sleeps, all-worker overrides, or tests against a developer instance.

## Work orders

- [x] [Task 01: Add archive navigation and query integrity](task-01-archive-navigation.md)
- [x] [Task 02: Organize desktop and phone list filters](task-02-filter-controls.md)

Dependency: Task 01 precedes Task 02. Execute sequentially.

## Verification

Each work order contains its exact product checks. Before the first package
command in a fresh worktree, run once from the repository root:

```bash
(cd apps && pnpm install --frozen-lockfile)
```

Design-package gates, from the repository root:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans/task-list-archive-filters
git status --short -- docs/specs/tasks docs/plans/task-list-archive-filters
```

Run the local documentation-coverage preflight from the repository root:

```bash
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const directory = 'docs/plans/task-list-archive-filters/';
const workOrders = ['task-01-archive-navigation.md', 'task-02-filter-controls.md']
  .map(name => directory + name);
const paths = [...workOrders, directory + 'plan.md',
  'docs/specs/tasks/requirements/task-list-archive-filters.md',
  'docs/specs/tasks/system-design/task-list-archive-filters.md'];
const fileContents = Object.fromEntries(paths.map(path => [path, fs.readFileSync(path, 'utf8')]));
const changedFiles = [{filename: 'apps/web/components/global-commands.tsx', status: 'modified'},
  ...workOrders.map(filename => ({filename, status: 'added', additions: 1, changes: 1}))];
const result = validateCoverage({changedFiles, fileContents});
console.log(JSON.stringify({ok: result.ok, status: result.status, errors: result.errors}));
if (!result.ok || result.status !== 'covered') process.exitCode = 1;
NODE
```

The representative implementation path makes this preflight check coverage
instead of accepting a documentation-only exemption. It performs no GitHub writes.

## Verification results

Design-package checks passed on 2026-10-20:

- Catalog validation: 384 decisions and 1,618 specification files validated.
- Specification linter regression suite: 36 tests passed.
- Full specification lint: passed.
- Local PR-documentation preflight: `covered`, both work orders accepted, zero errors.
- Diff whitespace, new-file whitespace, linked design paths, and untracked-file inventory: passed.

Implementation checks passed on 2026-10-20. Both work orders are complete.
Requirements are `active`; the design is `current`.

### Feature validation before base integration

- Focused Vitest command: 13 suites, 99 tests passed. See the work-order commands.
  Log: `/tmp/kandev-archive-unit-final.log`.
- Typecheck passed with `NODE_OPTIONS=--max-old-space-size=12288`. The default
  heap was too small for this worktree. Log: `/tmp/kandev-archive-typecheck-final.log`.
- ESLint passed for all 34 changed TypeScript files with `--max-warnings 0`.
  The final two browser-test edits also passed focused lint.
- `i18n:check` and `i18n:ratchet` passed. All supported locales are complete.
  Traditional Chinese and pseudo catalogs were generated with the repository scripts.
- Desktop: all 25 unique scenarios passed. The fresh-build run passed its first
  22 cases, then found a singular/plural typo in the pagination assertion.
  After correcting that assertion, the pagination and two remaining preference
  cases passed. Logs: `/tmp/kandev-run.e2e.b3oe4w3z.log` and
  `/tmp/kandev-run.e2e.CPruagbn.log`.
- Phone: all 9 unique scenarios passed. The final run passed navigation, plugin,
  drawer, Retry, and search cases. The existing display-menu test needed to
  dismiss its open overlay before tapping the relocated archive control.
  Its corrected file and the remaining display cases passed. Logs:
  `/tmp/kandev-run.e2e.jYeiyABf.log` and `/tmp/kandev-run.e2e.Y215WW8n.log`.
- Public documentation validator tests: 63 passed. All 48 published pages validated.
- Desktop and phone screenshots were inspected. The toolbar matches UI-02.
  The inset drawer matches UI-03, with contained scrolling, visible labels,
  safe-area clearance, and focus return. Fine-pointer checks at 767px/768px
  measured 44px/28px targets. A coarse-pointer tablet retained 44px controls.
  Artifacts: `apps/web/test-results/task-list-desktop-filters.png` and
  `apps/web/test-results/task-list-phone-filters.png`.

The isolated browser runs used `--host` and one worker. The local wrapper
`python3 /tmp/kandev-archive-test-env.py` supplies loopback binding and the Nix
library paths needed by cached Chromium. It changes no repository source.
The desktop run rebuilt production assets after the final production edit.
Later focused runs used `--no-build` against those same assets.

The final focused desktop command was:

```bash
python3 /tmp/kandev-archive-test-env.py scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --project chromium tests/task/task-list.spec.ts tests/task/task-listing-view-preferences.spec.ts --grep 'pagination shows totals|Task listing display preferences' --max-failures=1
```

The final focused phone command was:

```bash
python3 /tmp/kandev-archive-test-env.py scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome tests/task/mobile-task-list-search.spec.ts tests/task/mobile-task-listing-display.spec.ts --max-failures=1
```

Behavioral RED evidence includes absent archive navigation, delayed-query
publication, retained refresh callbacks, last-page recovery, workspace loss,
and preserved Home navigation. The phone browser checks also caught an
untranslated Retry label and inline empty-state layout. Both are corrected.
Pure presentation uses rendered browser checks rather than markup-only unit tests.

Shared workspace navigation keeps its established Home destination. List
normalization runs only when a workspace change leaves the Tasks route open.
This compatibility refinement is recorded in criterion `004.3` and its hook test.

The user approved manual testing and requested a ready PR. The isolated manual
test instance was stopped before publication. No backend, package, persistent
platform task, or delegated-agent change was needed.

## PR integration validation

Main changed plugin facet preferences, SPA routing, and shared search coverage
before publication. Integration retains the new facet preference types, the
integrated-control route, archive boot eligibility, and current-query ownership.
Plugin facet choices remain in URL/preferences and use a built-in API sort.
List keeps one 300ms request debounce; clearing admits the empty query immediately.
Inherited search and facet tests now use the visible List controls.

After the first integration, 153 focused unit tests passed across 20 suites. Typecheck
and ESLint for all 40 changed TypeScript files passed. Fresh production assets
passed all 32 desktop and 11 phone browser cases, including plugin preferences,
queued-search cancellation, stale replies, archive navigation, filters, and history.
The failed preliminary browser run used a native-select API on a Radix trigger;
that test now uses the established selection helper.

Evidence is under `/tmp/kandev-archive-pr-delivery-1n02tpi8/`: `merged-unit-final.log`,
`merged-typecheck-final-2.log`, `merged-eslint-final.log`,
`kandev-run.e2e.us9VZMmr.log`, and `kandev-run.e2e.BbcpuZI9.log`.
A later main update extended task deletion. Integration preserves the shared
`DeleteTaskParams` through the List layers and retains cleanup feedback.
The refreshed unit run passed 158 tests in 21 suites; typecheck and affected-file
ESLint passed. Fresh-build desktop and phone deletion smoke checks also passed.
Evidence: `final-unit.log`, `final-typecheck.log`, and `kandev-run.e2e.Rzl7M8cY.log`
and `kandev-run.e2e.pNNtIdjU.log` in the same delivery directory.

The manual instance remains stopped. PR publication and remote CI are separate
from these completed local checks.

## Risks

- Archive and ordinary task actions share the term `archive`. Preserve the
  existing action's ranking and test the new navigation command by its label.
- Global saved-scope values can contradict route overrides unless every List
  selector receives the effective query. Automatic entry must perform no save.
- Go boot rows use saved active scope. Reusing them for archive links would
  flash incorrect tasks or suppress the required first read.
- Shared header changes can affect Board, Threads, mobile View options, and
  plugin contributions. Limit exclusions to Tasks and run affected regressions.
- The shared workspace picker opens Home. List normalization must run only
  while the current route remains Tasks, so it cannot replace that navigation.

## PR review remediation

The route now validates the URL workspace before mounting List. Unknown
workspaces offer Home. Failed workspace lookups offer Retry and Home without
exposing internal errors. Browser history retains each link's explicit filters.
Tasks boot metadata expires when leaving Tasks or changing workspace. SPA
metadata readiness follows successful collection reads, including empty lists.

Applied filter labels use complete translated messages with a name placeholder.
The public phone guide limits the search toggle to Kanban and states that List
search stays visible.

Validation before the remediation commit:

- The workspace and metadata regressions failed against the original behavior.
- All 172 unit tests passed in 22 focused suites. The final route and loading
  rerun passed all 26 tests. Typecheck and changed-file lint passed.
- All 16 focused desktop browser scenarios passed. The five initial phone
  scenarios passed, including cross-workspace history, Retry, unknown-workspace
  Home recovery, 44px phone actions, and filter-drawer interactions.
- Localization checks, the new-copy ratchet, the specification catalog,
  specification lint, and the public documentation validator passed.

Logs are in `/tmp/kandev-archive-pr-delivery-1n02tpi8`. The desktop browser log is
`kandev-run.e2e.7U9t9I8J.log`; the initial phone log is
`kandev-run.e2e.T4VD6W6L.log`.

### Independent filter metadata readiness

A later regression exposed that combining workflow and repository readiness
could keep a valid workflow loading after an unused repository read failed.
The route now tracks each collection separately. The filter client also waits
for the current repository read instead of treating cached metadata as complete.

Three unit regressions and the real-browser unused-repository case failed
before this refinement. Validation passed 175 tests in 22 unit suites,
17 desktop browser scenarios, 14 phone browser scenarios, typecheck, and
changed-file lint without warnings. The browser logs are
`kandev-run.e2e.GC5ujdUu.log` and `kandev-run.e2e.ez4PLAOe.log` in the same
artifact directory.

The SPA route now keeps its loading state until metadata reads settle. It
mounts List after that point so a valid filter does not briefly receive an
unavailable label. A deferred-read unit regression and a browser regression
failed before the guard. The browser test holds the actual workflow API request
and checks loading, absent filter controls, then restored rows and history.

The final loading-state refinement passed all 175 unit tests, 17 desktop
scenarios, 14 phone scenarios, typecheck, and changed-file lint without warnings.
Its logs are `review-loading-all-unit.log`, `kandev-run.e2e.14kwCIII.log`, and
`kandev-run.e2e.Kos825mR.log` in the same artifact directory.

## CI remediation

CI exposed a workflow fallback regression and six stale UI assertions across
five E2E shards. List now uses the active workflow resolved for the workspace
instead of reapplying a rejected saved workflow. Explicit URL filters retain
their validation, and navigation does not save the resolved fallback.

Phone search-toggle checks now select Kanban, while List search stays visible.
The filter accessibility case uses List's dedicated Filters drawer and checks
labels, 44px targets, focus return, and the absence of duplicate View options
filters. Startup and Home tests expect the filtered empty message when the
seeded workflow default is selected.

Local reproductions used the same base and PR source as the failed CI run,
with one browser worker and retries disabled. A component regression failed
before the workflow correction. After the fix, all 38 targeted unit tests in
five suites and all 13 desktop browser scenarios passed. Typecheck and
changed-file ESLint passed without warnings. All 31 phone scenarios passed
with retries disabled.

Evidence is in `/tmp/kandev-archive-pr-delivery-1n02tpi8`, including
`ci-remediation-evidence.json`, `ci-workflow-green-unit.log`, and
`kandev-run.e2e.RcLoOzl1.log`. The phone log is `kandev-run.e2e.7UmKsM2V.log`.
Exact-head CI and review validation remains
pending until the remediation is pushed and its new checks finish.

Run the unit checks from `apps/web`:

```bash
VITEST_MAX_WORKERS=2 pnpm exec vitest run \
  app/tasks/tasks-page-client.facet-selection.test.tsx \
  app/tasks/tasks-page-client.mr-hydration.test.tsx \
  hooks/use-tasks-list-query-state.test.tsx \
  lib/tasks/tasks-list-query.test.ts \
  lib/kanban/resolve-workflow.test.ts
```

Run the browser checks from the repository root. The local runtime wrapper
and the exact isolated merge are recorded in the evidence file.

```bash
pnpm --dir apps/web e2e:run --host --project chromium \
  tests/settings/startup-page.spec.ts \
  tests/task/task-list-workspace.spec.ts \
  tests/task/task-list-archive-navigation.spec.ts \
  --max-failures=1 --retries=0
pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome \
  tests/layout/mobile-unified-navigation.spec.ts \
  tests/kanban/mobile-kanban-topbar.spec.ts \
  tests/kanban/mobile-display-settings-groups.spec.ts \
  tests/settings/mobile-startup-page.spec.ts \
  tests/chat/mobile-quick-chat-entry.spec.ts \
  tests/task/mobile-task-list-workspace.spec.ts \
  tests/task/mobile-task-list-filters.spec.ts \
  tests/task/mobile-task-list-archive-navigation.spec.ts \
  --max-failures=1 --retries=0
```
