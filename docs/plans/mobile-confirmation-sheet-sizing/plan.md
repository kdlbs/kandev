---
created: 2026-09-10
status: complete
requirements:
  - REQ-UI-MOBILE-CONFIRMATION-001
  - REQ-UI-MOBILE-CONFIRMATION-002
  - REQ-UI-MOBILE-CONFIRMATION-003
system_design:
  - ../../specs/ui/system-design/mobile-action-confirmations.md
legacy_specs: []
---

# Implementation Plan: Compact Mobile Confirmation Sheets

## Overview

Correct the geometry of the already-delivered mobile confirmation surfaces.
First make hosted drawer confirmations content-sized without losing origin
state. Then align the two saved-query filter hosts that still enter from the
side. Execute both work orders sequentially after the design handoff.

UI owns this reusable geometry and interaction contract, not the task/view
mutation. The amended [requirements](../../specs/ui/requirements/mobile-action-confirmations.md)
and [design](../../specs/ui/system-design/mobile-action-confirmations.md) retain
the accepted [single-surface decision](../../decisions/2026-09-10-mobile-confirmation-surfaces.md).
No new ADR is needed: the modal/state ownership decision is unchanged.

The [original delivery](../mobile-action-confirmations/plan.md) remains a
completed historical record. This follow-up was implemented and verified after
the 2026-09-11 explicit request.

## Evidence and root cause

The user's Threads deletion screenshot shows a short decision in a tall editor
shell, with a large empty region between the explanation and actions. A phone
browser check of the corresponding sidebar-view flow at 393 by 851 CSS pixels
measured a 681-pixel sheet. The editor was correctly hidden/inert and Cancel
restored it. The isolated server's asset hash matched this worktree's build;
this is not stale delivery or a missed `SavedTaskViewDeleteConfirmation` route.

Two layout rules explain it: the filter/Threads/Tasks hosts set tall heights,
and `MobileConfirmationHostBody` keeps the invisible origin in the same grid
track as the confirmation. The old design explicitly preserved host height;
AC-UI-MOBILE-CONFIRMATION-002.4 only required standalone sheets to fit content.
The revised criterion applies to hosted bottom sheets too.

An inventory of every host also found right-hand `Sheet` components in the
GitHub and GitLab phone filters. They need a phone bottom host to meet the same
visual contract; other centered dialog hosts are intentional exceptions.

Smallest reproduction: `/threads` with two saved views, select the custom view,
open its view picker, open settings, tap Delete. The permanent RED regression
belongs in `e2e/tests/task/mobile-threads-view.spec.ts`, extending
`confirms deletion of a saved view inside the native drawer` with compact bounds
and copy-to-action spacing assertions.

## Scope

### In scope

- Compact same-host confirmation in Tasks, sidebar view settings, Threads,
  session and terminal pickers.
- Bottom-hosted saved-query filter/confirmation flows on phone GitHub/GitLab.
- Original size, draft, selection, scroll and focus restoration on Cancel/Back.
- Long-content containment, safe areas, touch targets and reduced-motion behavior.
- Targeted rendered regressions, relevant desktop/tablet checks and the existing
  phone how-to update alongside implementation.

### Out of scope

- Domain mutations, cleanup policy, permissions, persistence, recovery or Undo.
- Centered command/Quick Chat/workflow-sync hosts and retained full alerts.
- Changing desktop/tablet layout, general navigation, or global UI primitives.
- New animation libraries, snap detents, modal queues or plugin contracts.
- Publishing, committing, changing the seeded database or touching main :9998.

## Technical approach

`MobileConfirmationHost` gets an explicit drawer composition opt-in. While a
request is active, its content properties override tall origin dimensions with
intrinsic height and the same dynamic-viewport cap as standalone confirmation.
`MobileConfirmationHostBody` removes the hidden origin from active layout while
preserving its measured dimensions, mounted controls and nested scroll offsets.
Keep token lifetime, portal ownership, focus handling and callbacks unchanged.

The explicit opt-in prevents centered dialogs from accidentally shrinking.
Apply it to the actual drawer owners rather than per-action copies of sizing
logic. Convert the two phone integration filter roots to the existing Drawer
primitive in work order 02; do not use a drawer stacked over their side sheet.

### Mobile design contract

| Choice | Contract |
| --- | --- |
| Entry points and desktop outcome | The existing delete/archive/close controls remain discoverable; each action still targets the same record and uses existing desktop confirmation. |
| Exemplar | Standalone `MobileActionConfirmation` supplies compact inset geometry; `MobilePickerSheet` supplies bottom entry, safe areas and one internal scroll owner. |
| Hierarchy | Back where hosted, title/target, consequences/options, full-width primary action, Cancel. Short content has no flexible empty middle. |
| Surface rationale | A short temporary decision needs a content-sized bottom sheet; the long editor/list regains its normal height on return. |
| Geometry and state | Dynamic viewport cap, one active body scroller, 48px action targets, safe-area clearance; only presentation changes. Existing domain hooks own all data. |

The default-English short Threads fixture should settle below 60% of the
configured Pixel 5 viewport, remain bottom-anchored, and leave at most 32 CSS
pixels between the final description content and the first action. These are
fixture assertions, not a fixed product height: translations, warnings and
long targets may need more space and internal scrolling.

## Tests

| Criteria | Evidence |
| --- | --- |
| 001.2, .4-.7; 003.1-.2, .5 | Existing `mobile-confirmation-host.test.tsx` and `mobile-action-confirmation.test.tsx`: same modal, mounted origin, focus, token invalidation, breakpoint cancel and callback lifetime. Add behavioral coverage for the drawer opt-in without using DOM-only tests as geometry evidence. |
| 002.3-.7 | Real browser bounds, footer spacing, hit tests, long content, dynamic viewport and reduced-motion cases in work order 01. |
| 001.8; 003.1, .6 | Real phone integration filter selection/default/deletion cases in work order 02. |

Abbreviated criteria use the `AC-UI-MOBILE-CONFIRMATION-` prefix. Full IDs and
exact commands are in each work order.

## E2E tests

| Flow | Project and test file |
| --- | --- |
| Screenshot regression and restored editor state | `mobile-chrome`: `tests/task/mobile-threads-view.spec.ts`, `tests/task/mobile-sidebar-views.spec.ts` |
| Compact Tasks confirmation, unchanged scroll and action target | `mobile-chrome`: `tests/task/mobile-action-confirmations.spec.ts`; replace its obsolete same-height assertion, retain the other safeguards |
| Picker content cannot force confirmation height | `mobile-chrome`: `tests/terminal/mobile-terminal-close.spec.ts`, `tests/session/mobile-session-deletion.spec.ts` |
| Centered form remains unchanged | `mobile-chrome`: workflow-sync case in `tests/settings/mobile-management-confirmations.spec.ts` |
| Saved-query filters enter from bottom, Cancel restores selection/scroll | `mobile-chrome`: `tests/github/mobile-github-sidebar.spec.ts`, saved-query case in `tests/gitlab/mobile-gitlab-parity.spec.ts` |
| Non-phone presentation remains unchanged | `chromium`: saved-view cases in `tests/task/sidebar-filter.spec.ts`, `tests/task/threads-view.spec.ts`, `tests/github/github-scope-bar.spec.ts`, `tests/gitlab/gitlab-issue-milestone-filter.spec.ts` |

Use fresh isolated E2E fixtures and a rebuilt frontend, not the user's seeded
test database. Settle finite animations before measuring; no fixed sleeps or
all-worker overrides. Inspect phone captures of Threads, Tasks and one provider
filter before refreshing the separately running test instance.

## Work orders

- [x] [Task 01: Compact hosted drawer geometry](task-01-compact-drawer-hosts.md)
- [x] [Task 02: Bottom-hosted saved-query filters](task-02-bottom-filter-hosts.md)

Order 02 depends on 01. Both are sequential; no delegation is authorized.

## Verification results

- `python3 scripts/lint-spec-files.test.py`: all 30 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `git diff --check`: passed.

The above document checks were run during the design turn. Work order 01 now
has passing rendered geometry, restoration, long-content and desktop checks;
its results are recorded in that work order. Both work orders are implemented.

Historical verification before PR integration with the newer base:

- Fresh `build:e2e`, typecheck, changed-file ESLint, Prettier and the i18n
  new-code ratchet passed. No user-facing copy or translation keys were added.
- All 20 host/content/request unit tests passed, including Cancel focus without
  scrolling and mounted-origin restoration in both drawer/dialog composition.
- The final targeted `mobile-chrome` batch passed all 16 cases without retries
  or skipped/flaky cases. It includes both providers, session deletion, centered
  workflow removal/retry, standalone and hosted archive at three viewport/locale
  combinations, sidebar views, Threads deletion, and terminal close.
- Earlier compatibility runs passed 12 desktop saved-view/scope cases and all
  three GitLab milestone/query cases. Four confirmation/persistence cases
  passed again after the final scroll-containment edit. That wrapper reported
  all four passes but exited 143; no fixture listener remained. The same four
  cases then passed with exit 0 through the repository's one-worker guarded
  `e2e:raw` runner. No test assertions were removed or retries added.
- The initial final batch's GitHub entry-timing failure and the subsequently
  discovered clipped Threads heading are documented in the work orders. They
  were remediated before the clean 16-case run; a passing box-size assertion
  alone is not accepted as visual evidence.

Inspected captures include Threads, Tasks, GitLab filters/confirmation, narrow
Portuguese, wide dark mode and landscape pseudo-locale. Final browser bounds:

| Surface | Viewport | Origin height | Confirmation height | Bottom edge |
| --- | --- | --- | --- | --- |
| Threads saved-view deletion | 393x640 | 512px | 350px | 640px |
| Tasks archive | 393x727 | 581.59px | 358px | 727px |
| GitLab saved-query deletion | 393x640 | 512px | 346px | 640px |

The Threads root now has zero scroll offset; the title and actions intersect
the viewport fully. Cancel preserves the invalid input, editor scroll and
original size. The short-content action gap is at most 32px. Long landscape
content scrolls inside the body with its footer reachable.

Disposable captures and the JSON result report are in
`/tmp/kandev-compact-captures.SMEGuq/`; they are not committed product assets.
Public docs updated: `docs/public/mobile-remote-access.md`, an existing how-to,
now describes compact hosted steps, restoration and bottom provider filters.
Specification lint, its 30 tests, public-doc validator tests, all 46 published
page checks and `git diff --check` passed.

The existing isolated instance remains at
`https://koi.taile29c7d.ts.net:48649`. Its served assets match the final local
build (`index-9P1CcRyb.js`, `index-DfhIhLPE.css`). Test 48429 and main 9998 both
returned readiness 200. No seed reset, backend restart, Tailscale route change,
commit or push was performed. The ownership-checked shutdown command remains:

```bash
bash /tmp/kandev-mobile-test.MPZpbZ/stop.sh
```

Known pre-existing limitation outside this correction: a general Tasks ellipsis
CSS rule hides the entry at 640-767px with a coarse pointer. Wide hosted tests
open in portrait and then rotate; they do not claim to fix that entry rule.

## PR integration verification

The base integration preserves the newer GitHub Views picker, fixed save action
and focus-release handoff; Threads listing chrome and sync errors; shared
control sizing; and picker close-focus callbacks. The extracted task-action
dialog owner now supplies the task ID required by mobile detach confirmation.
The design and mobile how-to name the current provider owners.

Post-integration checks:

- All 277 tests in the 37 changed unit-test files passed. A separate 31-file
  host/GitHub/Threads/menu run passed 253 tests (overlapping coverage), and the
  newly extracted task-action dialog owner's nine tests passed.
- `pnpm run typecheck`, resolved-file ESLint with zero warnings, Prettier,
  `pnpm run i18n:check`, architecture and harness lint passed.
- Harness validator: 19 tests passed; specification validator: 36 tests passed;
  all specs and 46 public docs pages validated; 62 public-doc validator tests passed.
- Fresh managed backend, web and packaged-fixture builds passed. The full mobile
  GitHub sidebar, action-confirmations and secrets specs passed all 16 tests.
- A second one-worker, no-retry batch passed eight selected mobile cases in
  Threads, sidebar views, GitLab, sessions, terminals and management confirmations.
- Four desktop GitHub scope/saved-query and sidebar/Threads deletion checks
  passed without retries against the same build.

The integration commit was pushed and its remote checks were inspected. Static
and frontend checks passed; four E2E shards exposed five stale mobile test
expectations and three intermittent assertion/fixture interactions. The
dependent E2E aggregate failures came from those shards.

## PR CI remediation

This remediation is test-only. It changes no application behavior, public
contract, copy, or screenshots, so the requirements, system design and public
how-to need no further change.

- Archive and plugin-uninstall regressions now require compact bottom sheets,
  48px confirmation actions and restored origin state, rather than inline rows.
- Quick Chat enters through the current mobile menu. Sentry's ordinary row
  controls use the shared 44px minimum; confirmation actions still require 48px.
- Mobile CI retry bounds settle finite animations and allow only 0.001px of
  floating-point error around the existing 44px target requirement.
- File-tree readiness checks a visibly collapsed parent folder rather than a
  virtualized root file pushed offscreen by preceding LSP fixtures.
- Copy-files progress accepts an unrelated remote-sync warning but still requires
  the exact two-file count and a successfully completed copy step.

Local evidence before the remediation commit:

- The five failing mobile scenarios reproduced with retries disabled, then
  passed after the assertion/entry updates.
- Running the LSP file-intelligence spec before file-tree search reproduced the
  virtualized-row failure. The corrected ordered desktop run passed all 18
  cases, including both copy-files progress scenarios, without retries:
  `pnpm e2e:run --no-build --project chromium tests/lsp/lsp-file-intelligence.spec.ts tests/task/file-tree-search.spec.ts tests/session/copyfiles-progress.spec.ts -- --retries=0`.
- All 29 cases in the five affected mobile specs passed without retries:
  `pnpm e2e:run --no-build --project mobile-chrome tests/task/mobile-sidebar-task-actions.spec.ts tests/task/mobile-content-confirmations.spec.ts tests/plugins/mobile-plugin-settings-row.spec.ts tests/integrations/mobile-integration-remove-confirmations.spec.ts tests/pr/mobile-ci-automation-options.spec.ts -- --retries=0`.
- Typecheck, seven-file ESLint with zero warnings, Prettier,
  `python3 scripts/lint-spec-files.py --all` and `git diff --check` passed.

No timeouts were increased, retries added or production behavior changed.
Earlier exact-head CI/review evidence becomes historical after the remediation
push. Final remote CI, delayed review comments and mergeability must be checked
on that new head before delivery; they are not claimed complete here.

## Risks

- Hiding an origin with zero or smaller dimensions can silently reset scroll.
- A shared host change can shrink centered dialogs unless explicitly scoped.
- Drawer transforms, focus return and resize can compete; preserve Vaul's
  ownership and test cancellation during transitions.
- Normalizing provider filters must not select a view or change its default
  while confirming deletion; preserve their current close timing.
