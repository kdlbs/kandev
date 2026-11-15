---
created: 2026-11-05
status: implemented
requirements:
  - REQ-OFFICE-AUTOMATION-RUNS-001
system_design:
  - ../../specs/office/system-design/automation-latest-run-indicator.md
legacy_specs: []
---

# Implementation Plan: Automation Latest-Run Indicator

## Overview

Fix issue [#5081](https://github.com/kdlbs/kandev/issues/5081) in one sequential
end-to-end work order. Keep current activity semantics separate from newest-run
outcome, then consume one presentation derivation in desktop navigation, the
agenda, and the active phone navigation path. No backend changes are required.

The [requirement](../../specs/office/requirements/automation-runs.md) remains
REQ-OFFICE-AUTOMATION-RUNS-001; the new scope is AC-001.19 through .22, with
.1/.9 preserving consistency and live transitions. The
[scoped design](../../specs/office/system-design/automation-latest-run-indicator.md)
is authoritative for this extension.

## Settled decisions and ownership

- User chose paused-gray priority, all three surfaces, and visible non-success
  text. Paused can therefore show a gray dot and Failed text together.
- Preserve running priority and existing never-run behavior, including the
  green idle dot. Never-run must not acquire a success label or fabricated age.
- Non-success means failed, retry_scheduling_failed, cancelled, skipped,
  archived, or scheduled_retry. Do not present skipped as a failure.
- Office owns newest-run selection and navigation status; no UI-system pair or
  ADR is needed. No unresolved product choice blocks this package.

## Scope

### In scope

- Shared outcome-aware presentation using authoritative per-automation summary.
- Sidebar color and non-success text before the age, agenda color and status,
  and active saved-layout phone row color/status.
- Running/paused precedence, assistive text, localization, long-label wrapping,
  latest-success recovery, and current refresh regression controls.
- Update the inactive phone fallback to share the same row presentation without
  changing routing or refresh ownership.
- Targeted component/browser evidence and one public-doc paragraph on status.

### Out of scope

- Backend/schema/API/event changes, status inference, retry policy, retention,
  feed aggregation, new pollers, navigation redesign, and saved preference writes.
- Changing custom pinned automation shortcuts or aggregate activity indicators.
- Adding last-run ages to phone navigation or the agenda.
- Generic QA/review audits, commits, pushes, PR creation, or delegation.

## Technical approach

1. Add the small planned pure helper `automationRunPresentation` in
   `apps/web/components/runs/automation-run-presentation.ts`. Input is existing
   `AutomationActivityState` plus newest `AutomationRun`, not workspace history.
   Reuse `statusDotClass`, `statusLabelKey`, and `isOpenRun` in `run-status.ts`.
   Keep `automationState`, `AutomationRow.state`, `buildAutomationRows`, and
   `buildAgenda` semantics unchanged. Priority is running > paused > last status
   > no-run fallback; keep newest-status metadata independent of color priority.
2. Update `AutomationRowLink` in `automations-section.tsx` and `AgendaRow` in
   `runs-list-page.tsx`. Preserve the spinner, same-run age tick, links, ordering,
   and next-firing reason. Use a localized status-plus-age interpolation frame.
3. Extend `ShortcutActivity` in `use-shortcut-activity.ts` with newest-run data
   from its already matched summary, only when activity is settled and known.
   Preserve Unknown/error/loading and `hasRunningActivity` behavior. No extra
   summary subscription or effect is needed.
4. Replace builtin `SavedMobileAutomationRows` rendering of generic ShortcutRows
   with a small planned `MobileAutomationRow` in
   `components/navigation/mobile-automation-row.tsx`; retain catalog entry label,
   href, availability and identity. Use the same row in the no-children fallback
   in `mobile-automations-section.tsx`. Do not modify generic shortcut activity
   colors/labels or change their consumers. Preserve accessible name/description
   separation, row tap, touch target and drawer dismissal.
5. Reuse run/activity translation keys and add the composite frame to all seven
   languages; regenerate Traditional Chinese and pseudo locales using existing
   scripts. Long labels can wrap without widening the rail/document.
6. After runtime proof, update the existing Read the output section of
   `docs/public/automation-and-mcp.md` with indicator precedence and the paused
   exception. It is an explanation paragraph; no new public page, screenshot,
   command, or API documentation is needed. Root README and screenshot catalog
   do not promise outcome colors and remain unchanged.

No shared provider/transport compatibility matrix is needed: one unchanged
WebSocket summary contract serves all three surfaces.

## ASCII UI preview

### UI-01: Desktop sidebar, expanded Automations

Entry: existing sidebar disclosure. Before is source-verified, not a new capture.

```text
Before:  [green dot] PR tagging                         4m ago
After:   [red dot]   PR tagging                 Failed, 4m ago
         [green dot] Dependency audit                   2m ago
         [spinner]   Nightly drift                      1m ago
         [gray dot]  Paused monitor             Failed, 8m ago
         [green dot] Never-run check
```

Dot colors are annotations, not literal UI text. Running overrides outcome;
paused remains muted. Ordinary rows remain compact. Long non-success metadata
may wrap within the row rather than disappearing or extending beyond the rail.
Row click opens the same automation history. Success keeps age-only metadata.

### UI-02: Agenda row on /automations

```text
[red dot] ~09:00 tomorrow | PR tagging | Failed
[gray dot] Switched off  | Monitor    | Failed
```

Keep next-firing/reason as the primary datum and preserve ordering. No last-run
age column. Status is separate from why an automation will or will not fire.
On a narrow viewport, metadata can wrap below the name without changing order.

### UI-03: Phone navigation drawer, expanded Automations

```text
+---------------------------------------+
| Existing navigation header             |
| ...                                    |
| v Automations                          |
| [red dot] PR tagging          Failed   |
| [green dot] Dependency audit Succeeded  |
| [blue dot] Nightly drift     Running   |
| [gray dot] Paused monitor              |
|                         Paused, Failed |
| Set up an automation                   |
| Open automations                       |
+---------------------------------------+
```

Retain the existing inset drawer, fixed header, safe area and single nav
scroller. Each row is one at-least-44-px touch target: tap navigates and dismisses
the drawer. Phone status is visible without hover; no age is added. Names may
truncate, but non-success status wraps rather than being clipped. The existing
mobile menu and MobileAutomationsSection are the content-only exemplar.

Structural requirements: activity/outcome separation, metadata order, same-run
status/age, preserved primary navigation, readable failure status and no
horizontal page overflow. Exact spacing, names and relative ages are illustrative.
Map UI-01/UI-02 to AC-001.19 through .21; UI-03 to AC-001.20 through .22.
The focused Chromium and mobile-chrome checks below prove these views.

## Tests

Planned new test names are implementation targets, not claims that tests exist.
Use behavioral Red-Green coverage; do not add tests that merely copy palette maps.

| AC suffix (REQ-OFFICE-AUTOMATION-RUNS-001) | File / planned behavior cases |
| --- | --- |
| .19, .20 | `components/runs/automation-run-presentation.test.ts`: `failure is not idle success`; `running wins over a newer skipped firing`; `disabled automation and disabled schedule retain paused color`; `a newer success clears failure`; `never-run has no outcome metadata` |
| .19, .21 | `components/runs/runs-list-page.test.tsx`: `agenda outcome does not replace next-firing reason` |
| .9, .19-.21 | `components/app-sidebar/sections/automations-section.test.tsx`: `live failure becomes red with outcome before age`; `newer success clears failure text without remount`; existing running, age tick and visibility controls |
| .20, .22 | `hooks/domains/sidebar/use-shortcut-activity.test.ts`: `failure survives the active phone projection without becoming running`; `unavailable activity does not expose an outcome` |
| .21, .22 | `components/navigation/mobile-automation-row.test.tsx`: `paused failure keeps both labels`; `running and newest skip remain distinct`; localized long label rendering and navigation |
| .22 | `components/navigation/app-nav-sheet.test.tsx`: `saved-layout automation row exposes latest failure`; exercise actual saved-layout composition and activity projection, not only fallback. Existing `mobile-automations-section.test.tsx` protects fallback and disclosure behavior. |

Retain existing summary ordering/workspace isolation and next-firing tests.
Do not re-pin an obsolete Idle assertion after a successful run: assert the
consumer-visible successful outcome instead. Runtime smoke/E2E is still required.

## E2E tests

- Extend `apps/web/e2e/tests/automations-sidebar.spec.ts`, project `chromium`,
  with `latest failed run is red and recovers after a newer success`.
  Seed an automation and failed run with existing `seedAutomation` and
  `seedAutomationRun`. Open the actual desktop section; assert red indicator,
  Failed before the age, and the same newest run's red Failed status in details.
  Confirm the agenda follows the same semantics while preserving next-firing
  data. Back on the mounted sidebar, seed a newer success; wait for a
  payload-matched `automation.summaries` response containing its exact run ID,
  then assert green and age-only metadata without reload/remount. Cover paused
  failure and existing running cases as controls. Maps .9 and .19-.21.
- Extend `apps/web/e2e/tests/layout/mobile-navigation-hierarchy.spec.ts`, project
  `mobile-chrome`, with `phone automation rows expose latest failure and keep
  row-tap navigation`. Use the active saved-layout route. Open app navigation,
  expand Automations, assert red Failed, then observe the accepted newer success
  while the same drawer remains open. Tap the same row; assert its history URL
  and drawer dismissal. Include paused-gray/Failed, a long localized status,
  actual row height >=44 px and zero document horizontal overflow. Maps .20-.22.
- Arm `watchWs` before stimulus and match run identity/status in response payloads;
  request completion alone is not evidence of new data. Seed subsequent runs
  with strictly newer creation times; do not assume random ID order breaks a
  timestamp tie in insertion order. If fixture timing cannot prove same-run
  terminal transition, keep that deterministic transition in the component test
  and prove the newest-run replacement in the browser; do not add a production
  fixture API or mutate the browser's store to pass.
- Capture and inspect UI-01 and UI-03 screenshots using existing `prCapture`;
  inspect UI-02 as rendered. Record any structural mismatch in work-order Results.
  The managed runner builds fresh Vite/backend assets and uses isolated data.

## Work orders

- [x] [Task 01: Deliver latest-run indicators across automation navigation](task-01-latest-run-indicator.md)

Wave 1 only, no dependencies, sequential in the primary session. Layers form one
observable outcome; splitting them would leave inconsistent intermediate surfaces.
No implementation or subagents are authorized by this design package.

## Verification

Run the full block from the repository root after explicit implementation
request. Bootstrap once, not before every check. Run desktop and phone E2E
sequentially; never pass worker-budget overrides.

```bash
# From the repository root. Install once if this worktree is not bootstrapped.
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/runs/automation-run-presentation.test.ts components/runs/automation-rows.test.ts components/runs/run-status.test.ts components/runs/runs-list-page.test.tsx components/app-sidebar/sections/automations-section.test.tsx hooks/domains/sidebar/use-shortcut-activity.test.ts components/navigation/mobile-automation-row.test.tsx components/navigation/mobile-automations-section.test.tsx components/navigation/app-nav-sheet.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings components/runs/automation-run-presentation.ts components/runs/automation-run-presentation.test.ts components/runs/runs-list-page.tsx components/runs/runs-list-page.test.tsx components/app-sidebar/sections/automations-section.tsx components/app-sidebar/sections/automations-section.test.tsx hooks/domains/sidebar/use-shortcut-activity.ts hooks/domains/sidebar/use-shortcut-activity.test.ts components/navigation/mobile-automation-row.tsx components/navigation/mobile-automation-row.test.tsx components/navigation/mobile-automations-section.tsx components/navigation/mobile-automations-section.test.tsx components/navigation/mobile-sidebar-layout-navigation.tsx components/navigation/app-nav-sheet.test.tsx)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/automations-sidebar.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/layout/mobile-navigation-hierarchy.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check -- apps/web docs/specs/office docs/plans/automation-latest-run-indicator docs/public/automation-and-mcp.md
```

Also rerun the PR-documentation `validateCoverage` preflight with the work order,
plan, owning requirement, scoped design and proposed/changed runtime paths.
Record the exact invocation and result. All new `*.test.ts(x)` files above must
exist by then; existing test files and scripts were located during planning.

## Verification results

Task 01 is done. The bounded implementation and exact verification gates are
recorded in [Task 01 Results](task-01-latest-run-indicator.md#results).
The broad migrated automation-runs requirement and base design remain draft.

### Design-package preflight (2026-11-05)

- `python3 scripts/list-docs.py validate`: 431 decisions and 1,921 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `git diff --check -- docs/specs docs/plans/automation-latest-run-indicator`: passed.
- `python3 scripts/list-docs.py specs --system office --text automation-latest-run-indicator --format paths`:
  discovers the owning requirement, base design and scoped indicator design.
- The exact local `validateCoverage` invocation in Task 01: `covered`, no errors;
  the work order, requirement, design and plan references are accepted together
  with the seven proposed runtime paths. This is planning preflight, not runtime
  or remote PR evidence.
- Local-link check: 25 document links resolve; all nine existing targeted test
  files resolve. Planned new helper/phone-row source and tests are marked new.
- `git status --short -- docs/specs/office docs/plans/automation-latest-run-indicator`:
  two modified specs, one new scoped design and the new plan directory, unstaged.

### Implementation delivery (2026-11-05)

- Red-first regressions demonstrated the failure before production changes.
- Final focused unit/component checks: 148 passed across 9 files.
- Native web typecheck, exact scoped lint, and complete-locale/pseudo checks passed.
- Fresh managed Chromium: 6 passed; fresh managed mobile-chrome: 5 passed.
  Projects ran sequentially with retries disabled. UI-01/UI-02/UI-03 captures
  were inspected and their paths are recorded in the work order.
- Public automation status explanation updated after runtime proof; public-doc
  validators passed (63 tests, 55 pages).
- The full owning web suite and extra repository-wide Make command were not green
  in unchanged areas. Their exact counts, failures and evidence are explicitly
  recorded in Task 01; this plan does not claim an all-repository green audit.

## Related delivery records

The completed [sidebar running indicator](../automation-sidebar-running-indicator/plan.md)
and [definition refresh](../sidebar-automation-definition-refresh/plan.md) remain
historical; their refresh ownership is preserved and completed tasks are not
reopened. [Run-status refresh](../automation-run-status-refresh/plan.md) owns the
detail page refresh gate and is unchanged. No companion task scope or recorded
historical result is superseded; this work adds outcome presentation only.

## Risks

- Paused deliberately masks failure color, as requested; Failed text preserves
  the outcome. Never-run enabled rows deliberately remain green idle.
- Retried/skipped rows can be newest even when another run is open; conflating
  activity with outcome would remove running aggregation or mislabel the age.
- The phone fallback is not the active route. Browser and composition tests must
  exercise saved-layout rows, and adding newest-run data must not alter pins.
- Long translations must wrap instead of clipping the automation name/status or
  creating document overflow; verify actual desktop and phone rendering.
- Existing polling freshness and request failure behavior remain; this is not
  an instantaneous push update or a health model redesign.

## Opus implementation review remediation

Round 1 found two medium issues, both fixed in the parent session. The agenda
name now retains a nonzero minimum width so non-success metadata wraps rather
than hiding the name. The phone E2E file includes scheduled-retry/cancelled
geometry at 360, 393, 767 and 768 px. Unrelated generated zh-hk/zh-tw Plugin-tools
copy was restored to HEAD, leaving only the new status/age frame.

Fresh full phone E2E: 6 passed with retries disabled; agenda unit tests: 13 passed;
scoped lint and i18n validation passed. A disposable built-app browser smoke
confirmed readable widths and no overflow. Exact commands, RED/GREEN logs and
capture are recorded in [Task 01 Results](task-01-latest-run-indicator.md#opus-implementation-review-remediation-round-1).
The optional label-consolidation suggestion was deferred because it identifies
no current behavioral defect. Review-loop progress is tracked in the Kandev task
plan; the earlier three clean Sol rounds were design-only.
