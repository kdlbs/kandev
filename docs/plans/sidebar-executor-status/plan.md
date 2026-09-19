---
created: 2026-09-19
status: complete
requirements:
  - REQ-EXECUTORS-TASK-STATUS-001
system_design:
  - ../../specs/executors/system-design/task-status-indicators.md
legacy_specs:
  - ../../specs/kubernetes-executor/spec.md
---

# Implementation Plan: Sidebar executor status

## Overview and evidence

Source tracing establishes two defects: `useRemoteExecutorStatus` loads on mount
or identity change only, and the resource has no timer or visibility listener.
Hover/focus explicitly refreshes, explaining a mounted icon staying stale until
interaction. `StatusTrigger` destructures a fixed prop list and drops the Radix
DOM ref and injected properties. The component tests mock Tooltip/Drawer as
fragments, so they do not verify anchoring or real trigger linkage. This is a
confirmed composition defect; its exact rendered symptom still requires E2E.
At design time frontend dependencies were absent. Implementation installed them
and verified the real primitive linkage and browser behavior below.

The earlier completed
[preview repair](../kubernetes-executor-task-status-preview-repair/plan.md)
introduced eager hydration and explicitly excluded periodic polling. This package
supersedes that exclusion for mounted consumers in visible documents and repairs
primitive composition. Its historical test results are not evidence for this fix.

## Scope and technical approach

One sequential vertical work order repairs the shared resource lifecycle and
trigger composition, adds real-primitive and lifecycle regressions, and verifies
desktop/mobile flows. Reuse existing summaries, localized labels, exact-scope
requests, and safe error projection. No new API, executor lifecycle behavior,
settings UI or unrelated tooltip refactor is in scope.

## ASCII UI preview

UI-01: Sidebar indicator, desktop hover/focus (AC .1/.3).

```text
Task title [executor glyph]
              +---------------------------+
              | Pod / executor name       |
              | State       Running       |
              | Restarts    0             |
              | Workspace   Persistent    |
              | Last check  just now      |
              +---------------------------+
```

UI-02: Phone task-list indicator, tapped (AC .4).

```text
Task list remains behind drawer
+--------------------------------+
| Executor name              [X] | fixed header
| State / restarts / workspace   | scrolling body
| Created / last check           |
| Error or updating, when needed |
+--------------------------------+
             safe area
```

The anchored desktop surface and short touch drawer are required. Labels and
spacing are illustrative and localized. Loading retains facts; failures use a
text row. Preserve dense desktop row height and a non-overlapping 44 px touch
hit area. No hover is required for ongoing status updates.

## Tests

AC .1/.2: extend `remote-executor-status-resource.test.ts` with fake-clock cases
for no-interaction refresh, sharing, failed reads, absent transport, visibility,
unmount and obsolete scope settlement. Assert request counts as well as outcomes.
AC .3/.4: new `remote-cloud-tooltip-primitives.test.tsx` uses actual TooltipProvider,
Tooltip and Drawer; asserts trigger aria linkage, focus opening, Escape, content
and focus return. Existing mocked tests remain useful for summary projections.

## E2E tests

Extend existing `settings/kubernetes-task-environment.spec.ts` and
`settings/mobile-kubernetes-task-environment.spec.ts`. Change the exact-session
response while mounted, advance controlled time, await the causal request, and
assert tone before pointer interaction. Desktop hover/focus must show a panel
anchored near the icon, inside the viewport. Mobile taps must open the drawer,
show the same facts and preserve task selection on dismissal. Verify hit area,
adjacent controls, internal scroll, safe area and no horizontal overflow.

## Work orders

- [x] [Task 01: Refresh and disclose executor status](task-01-refresh-and-disclose.md)

## Verification commands

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run hooks/domains/session/remote-executor-status-resource.test.ts components/task/remote-cloud-tooltip.test.tsx components/task/remote-cloud-tooltip-primitives.test.tsx)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=3072 pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && PATH=/usr/local/go/bin:$PATH pnpm e2e:run --host --project=chromium tests/settings/kubernetes-task-environment.spec.ts -- --retries=0)
(cd apps/web && PATH=/usr/local/go/bin:$PATH pnpm e2e:run --host --no-build --project=mobile-chrome tests/settings/mobile-kubernetes-task-environment.spec.ts -- --retries=0)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Verification results

Implementation and targeted verification complete.

- RED: no-interaction refresh, automatic retry, and visibility-return tests
  failed on stale/missing status. The real Tooltip regression failed because
  the DOM trigger had no `aria-describedby` linkage.
- GREEN: the listed focused Vitest command passes 26 tests in 3 files.
- Changed-file ESLint with `--max-warnings=0` passes.
- Fresh managed production build passes. Desktop Chromium E2E: 1/1, 4.3 seconds;
  mobile Chrome E2E: 1/1, 5.7 seconds; both with `--retries=0`. These verify
  automatic updates before interaction, anchored desktop details, touch drawer
  facts, no task selection, focus return, and horizontal containment.
- `pnpm run i18n:check` passes. Public-doc validation passes for 47 pages.
  Catalog validation, specification lint, and `git diff --check` pass.
- Full `pnpm run typecheck` could not complete: Node exhausted both its default
  2 GB heap and a 3 GB heap in this 4 GB environment. This remains an explicit
  full-project validation limitation, not a passing check.
- Focused typecheck passes with the repository tsconfig, changed production/test
  roots, actual transitive dependencies, web ambient declaration files, and
  explicit Node/React ambient types. It used `/tmp/executor-status-tsconfig.json`
  with incremental checking disabled and a 3 GB heap. No compiler errors were
  suppressed. This does not replace full-project typecheck evidence.
- Ready PR delivery was explicitly requested after user testing. Full-project
  typecheck remains an explicit validation limitation for CI.
  Managed E2E instances and the isolated user preview were stopped; the main
  instance was untouched. Generated typecheck crash dumps were removed.

## Risks

- Polling scales with distinct mounted scopes; share requests, stop on hidden
  documents/unmount, and preserve existing cache bounds.
- Real primitives may expose additional focus/portal issues hidden by mocks.
- Browser throttling can delay timers; returning to visibility must recover.

## Public documentation

At implementation, update `docs/public/k8s.md` and `docs/public/executors.md`
with automatic refresh while visible. Preserve their how-to/reference structure.
No public claim changes during this design-only turn.
