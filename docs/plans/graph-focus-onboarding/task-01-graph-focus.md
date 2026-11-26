---
id: "01-graph-focus"
title: "Preserve the focused graph destination"
status: done
wave: 1
depends_on: []
plan: "plan.md"
acceptance_criteria:
  - AC-UI-FIRST-RUN-DIALOG-001.3
  - AC-UI-FIRST-RUN-DIALOG-001.6
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-009.4
requirements:
  - REQ-UI-FIRST-RUN-DIALOG-001
  - REQ-TASKS-TASK-RELATIONSHIP-GRAPH-009
system_design:
  - ../../specs/ui/system-design/first-run-dialog-availability.md
  - ../../specs/tasks/system-design/task-relationship-graph.md
---

# Preserve the focused graph destination

1. Extend `apps/web/app/page-client.test.tsx` with a marker-free larger viewport. Assert that a nonempty graph focus hides the dialog. Remove the focus on rerender and assert the dialog appears with the marker still absent.
2. Extend the existing sidebar action in `apps/web/e2e/tests/task/task-graph.spec.ts`. Remove the fixture's completion marker after the task page loads. Click Show in graph. Assert the node receives focus, the onboarding dialog stays hidden, and the marker stays absent.
3. Update the open prop in `apps/web/app/page-client.tsx` to read the current search parameters with the existing graph-focus constant from `apps/web/lib/links.ts`. Keep existing completion handlers and phone routing.

Run each command from the repository root. Run the PageClient test before and after the fix. Build the web bundle, backend binaries, and E2E fixtures before running Playwright with `--no-build`.

```sh
systemd-run --user --scope -p MemoryMax=8G pnpm --dir apps/web exec vitest run app/page-client.test.tsx
systemd-run --user --scope -p MemoryMax=8G pnpm --dir apps/web e2e:run --host --no-build --shards 1 --project chromium tests/task/task-graph.spec.ts
systemd-run --user --scope -p MemoryMax=8G pnpm --dir apps/web e2e:run --host --no-build --shards 1 --project mobile-chrome tests/task/mobile-task-graph.spec.ts tests/office/mobile-onboarding-dialog.spec.ts
```

## ASCII UI preview

UI-01. After Show in graph, the chosen node is focused and interactive.

```text
Desktop/tablet: [Sidebar] | [Board Graph: selected task]
Phone:          [Full-screen task graph sheet: selected task]
```

The surface and focus are required. Spacing is illustrative.

## Results

- The new PageClient case failed before the fix because the dialog opened. After the fix, all 23 tests passed.
- The managed Chromium graph suite passed all 7 tests. The mobile Chromium graph and onboarding suites passed all 4 tests.
- The desktop capture shows the selected graph node with the board available. The phone action still opens the graph sheet.
- The documentation catalog, specification lint, and PR documentation coverage checks passed.
- The host build required a separate 8 GB build of the shared send-now fixture with Go optimization disabled for the orchestrator package. The browser tests used the normal app builds and a clean environment.
