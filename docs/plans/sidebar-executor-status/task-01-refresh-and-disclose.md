---
id: "01-refresh-and-disclose"
title: "Refresh and disclose executor status"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-TASK-STATUS-001
acceptance_criteria:
  - AC-EXECUTORS-TASK-STATUS-001.1
  - AC-EXECUTORS-TASK-STATUS-001.2
  - AC-EXECUTORS-TASK-STATUS-001.3
  - AC-EXECUTORS-TASK-STATUS-001.4
system_design:
  - ../../specs/executors/system-design/task-status-indicators.md
---

# Task 01: Refresh and disclose executor status

## Summary and scope

Implement the [plan](plan.md) as one sequential vertical repair. Own refresh
lifecycle, primitive composition, regression coverage and public documentation.
Exclude backend/API changes, executor lifecycle and unrelated tooltip consumers.

## Acceptance

1. Fake-clock tests first fail on the missing automatic refresh, then pass with
   one lifecycle per exact scope, including failures, visibility and cleanup.
2. Real-primitive regression first fails on missing trigger linkage; after the
   correction desktop details anchor to the glyph and mobile focus returns.
3. Desktop/mobile E2E prove fresh status without hover, visible details and
   touch containment without accidental task selection.

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

Full composition and evidence: [plan](plan.md#ascii-ui-preview).

## Verification

Run tests red before production edits, then all checks below after correction.
The new primitive test file is created by this work order.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run hooks/domains/session/remote-executor-status-transport.test.ts hooks/domains/session/remote-executor-status-resource.test.ts components/task/remote-cloud-tooltip.test.tsx components/task/remote-cloud-tooltip-primitives.test.tsx)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=3072 pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && PATH=/usr/local/go/bin:$PATH pnpm e2e:run --host --project=chromium tests/settings/kubernetes-task-environment.spec.ts -- --retries=0)
(cd apps/web && PATH=/usr/local/go/bin:$PATH pnpm e2e:run --host --no-build --project=mobile-chrome tests/settings/mobile-kubernetes-task-environment.spec.ts -- --retries=0)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/hooks/domains/session/remote-executor-status-resource.ts` and `.test.ts`
- `apps/web/hooks/domains/session/remote-executor-status-refresh.ts` (new scheduler)
- `apps/web/components/task/remote-cloud-tooltip.tsx` and `.test.tsx`
- `apps/web/components/task/remote-cloud-tooltip-primitives.test.tsx` (new)
- `apps/web/e2e/tests/settings/kubernetes-task-environment.spec.ts`
- `apps/web/e2e/tests/settings/mobile-kubernetes-task-environment.spec.ts`
- `docs/public/k8s.md` and `docs/public/executors.md`

## Dependencies

None. Install workspace dependencies before testing.

## Risks

Increased reads for many scopes; strict unsubscribe/visibility cleanup is required.
Do not replace real primitives with mocks in the composition regression.

## Parallelism

`sequential`

## Inputs

The requirement and design above, current shared-resource tests, task PR tooltip
interaction pattern, and mobile-menu-sheet layout exemplar.

## Results

Implementation and targeted verification complete.

- RED: no-interaction refresh, automatic retry, and visibility-return tests
  failed on stale/missing status. The real Tooltip regression failed because
  the DOM trigger had no `aria-describedby` linkage.
- GREEN: the listed focused Vitest command passes 29 tests in 4 files.
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

### PR review remediation

Greptile identified obsolete queued WebSocket reads during outages. The default
requester now treats a disconnected client as unavailable transport. The new
transport regression failed with four offline requests before the guard, then
passed with zero offline reads and a healthy result after reconnect. All 27
focused tests and changed-file ESLint pass. No UI, API or public-doc contract
changed; the system design records the transport guard. Remote CI/review
verification remains pending for the remediation commit.

CodeRabbit summary findings: unavailable transport now immediately publishes the
sanitized unavailable snapshot; primitive classes and touch handlers are
composed. The unavailable-state assertion failed before the fix. Actual-drawer
coverage now exercises click, Enter and Space, with no task selection. All 29
focused tests and changed-file lint pass; mobile E2E passes 1/1 without retries
after a fresh build. Desktop E2E also passes 1/1 without retries, and the
focused TypeScript check passes.
