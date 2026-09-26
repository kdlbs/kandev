---
id: "03-conflict-recovery-ui"
title: "Present and verify multi-window recovery"
status: done
wave: 3
depends_on:
  - "02-isolated-backend"
plan: "plan.md"
requirements:
  - REQ-DESKTOP-ISOLATED-STARTUP-001
  - REQ-DESKTOP-ISOLATED-STARTUP-002
  - REQ-DESKTOP-ISOLATED-STARTUP-003
  - REQ-DESKTOP-ISOLATED-STARTUP-004
acceptance_criteria:
  - AC-DESKTOP-ISOLATED-STARTUP-001.1
  - AC-DESKTOP-ISOLATED-STARTUP-001.2
  - AC-DESKTOP-ISOLATED-STARTUP-001.3
  - AC-DESKTOP-ISOLATED-STARTUP-001.4
  - AC-DESKTOP-ISOLATED-STARTUP-002.1
  - AC-DESKTOP-ISOLATED-STARTUP-002.2
  - AC-DESKTOP-ISOLATED-STARTUP-002.3
  - AC-DESKTOP-ISOLATED-STARTUP-002.4
  - AC-DESKTOP-ISOLATED-STARTUP-002.5
  - AC-DESKTOP-ISOLATED-STARTUP-003.1
  - AC-DESKTOP-ISOLATED-STARTUP-004.1
  - AC-DESKTOP-ISOLATED-STARTUP-004.2
system_design:
  - ../../specs/desktop/system-design/isolated-startup.md
---

# Task 03: Present and verify multi-window recovery

## Summary

Turn the raw startup error into a conflict launcher that can repeatedly open
temporary windows. Prove that two test windows own separate backends and that
closing one leaves the others running.

## In scope

- Render the one-instance-per-database rule, conflict path, optional owner
  details, then a bottom fallback with a separate-data and deletion warning,
  repeatable action, spawn feedback, and technical details.
- Render the test process's startup home path and clean-quit rule, and retain
  the allocated path on its failure panel.
- Use data-folder copy for a home lock with external SQLite or PostgreSQL;
  retain database copy for in-home SQLite and database-target conflicts.
- Replace the native loading ring and green-tinted startup background with
  Kandev's theme-matched background and nine-square grid spinner in both
  first-paint HTML and the loaded desktop stylesheet.
- Localize all new startup copy in the six supported catalogs, using OS
  language selection before the backend is available.
- Extend the fake-runtime smoke to launch two temporary windows, check
  independent homes, databases, ports, readiness, and isolated shutdown.
- Update the public desktop guide with temporary and reusable dev-home flows.

## Out of scope

- A phone web route, attachment to an existing backend, and deletion after
  uncertain shutdown.

## Acceptance

1. The normal conflict window matches `UI-01`: the database rule is the main
   message, while the isolated action and data-loss warning sit in a separate
   bottom section. It stays open after each action, producing `UI-02` and
   `UI-03` on successive clicks.
2. Each test window shows its home during startup and reaches only its owned
   backend after token-matched health and application readiness.
3. The public guide gives a copyable reusable-home `make desktop-dev` command
   and explains temporary data cleanup and multiple test windows.
4. In OS light and dark modes, both startup paints match Kandev's background
   and primary grid color. The grid becomes static for reduced motion and is
   absent on a failure or conflict screen.
5. Conflict copy identifies the locked resource for in-home SQLite, external
   SQLite, and PostgreSQL. A failed test launch displays and retains its home,
   including at a narrow window width.

## ASCII UI preview

See the [full preview](plan.md#ascii-ui-preview). These excerpts cover
`AC-DESKTOP-ISOLATED-STARTUP-001.1` through `.4` and
`AC-DESKTOP-ISOLATED-STARTUP-002.1` through `.4`, plus
`AC-DESKTOP-ISOLATED-STARTUP-004.1` and `.2`.

`UI-00: Theme-matched loading, before and after bundled CSS`

```text
  LIGHT: white                       DARK: #181818
  +------------------------+         +------------------------+
  |      [ ][ ][ ]         |         |      [ ][ ][ ]         |
  |      [ ][ ][ ]         |         |      [ ][ ][ ]         |
  |      [ ][ ][ ]         |         |      [ ][ ][ ]         |
  |    Starting backend    |         |    Starting backend    |
  +------------------------+         +------------------------+
       purple grid                       purple grid
```

`UI-01: Normal desktop window, confirmed data conflict`

```text
+--------------------------------------------------------------------+
| (red)(yellow)(green)                                                |
| Only one Kandev instance can use this database at a time.         |
| Another instance is already using your main Kandev data.          |
| Database: /Users/.../.kandev/data/kandev.db                        |
| Running process: 6804 (.../bin/kandev)                             |
| [ Technical details v ]                                            |
|--------------------------------------------------------------------|
| Start a temporary instance                                         |
| Warning: This starts with empty data, separate from your main    |
| instance. You will lose anything created there when its window   |
| closes normally.                                                  |
| [ Start isolated temporary instance ]                              |
+--------------------------------------------------------------------+
```

`UI-02: First test window opens while the launcher stays available`

```text
  CONFLICT LAUNCHER                     TEST WINDOW A
  +----------------------------+        +----------------------------+
  | Main database is in use    |        | Temporary test instance    |
  | [ Start isolated instance ]|        | Starting backend...        |
  +----------------------------+        | Data: /tmp/kandev-...-A    |
                                      +----------------------------+
```

`UI-03: Second click adds another test window`

```text
  CONFLICT LAUNCHER       TEST WINDOW A       TEST WINDOW B
  +------------------+    +---------------+   +---------------+
  | [ Start another ]|    | Data A        |   | Data B        |
  +------------------+    | Backend A     |   | Backend B     |
                          +---------------+   +---------------+
```

Copy and spacing are illustrative. The Tauri window has a 960px minimum
width, and this native startup page has no phone route. On a short desktop
window, the status panel owns scrolling and the action remains keyboard and
pointer accessible. For a home-only conflict without default SQLite, replace
the database line with the exclusive data-folder rule. Mobile web E2E cannot
exercise this native launch action.
The macOS circles are native controls placed by Task 04; they do not add a
separate title strip or web buttons.
The grid stops moving under `prefers-reduced-motion`, while startup status text
remains accessible. A failure or conflict screen does not show a loading grid.

## Verification

```bash
(cd apps && pnpm --filter @kandev/desktop typecheck)
(cd apps && pnpm --filter @kandev/desktop e2e)
node --test apps/desktop/e2e/desktop-launch-smoke.test.mjs
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

The rendered startup check covers light, dark, and reduced-motion first paint,
resource-specific conflict copy, retained failure path, narrow-window
containment, nine grid cells, and hiding the grid after failure. Conflict and
failure panels announce terminal state assertively; launch-action errors are
assertive while starting feedback remains polite. Traditional Chinese locale
tags map to the matching Hong Kong or Taiwan catalog, with a Traditional
Chinese fallback for other `zh-Hant` tags.

## Files likely touched

- `apps/desktop/index.html`
- `apps/desktop/src/main.ts`
- `apps/desktop/src/styles.css`
- `apps/desktop/src/locales/**`
- `apps/desktop/package.json`
- `apps/pnpm-lock.yaml`
- `apps/desktop/e2e/desktop-launch-smoke.mjs`
- `apps/desktop/e2e/desktop-launch-smoke.test.mjs`
- `docs/public/desktop-app.md`

## Dependencies

Task 02's typed conflict status and native process-spawn command.

## Risks

The Tauri startup page cannot read the backend's saved locale before startup,
so OS-language fallback must be deterministic. The fake-runtime smoke needs a
supported way to drive native startup controls and observe two independent
GUI processes without a timed blind click.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/desktop/requirements/isolated-startup.md),
  [design](../../specs/desktop/system-design/isolated-startup.md),
  [decision](../../decisions/2026-09-25-temporary-desktop-test-processes.md),
  and existing startup-page markup and fake-runtime smoke.

## Results

Passed the Linux desktop release build and two-window smoke through
`TMPDIR=/root/.cache/kandev-desktop-task-tmp pnpm --filter @kandev/desktop
e2e`, desktop typecheck, and all 13 desktop-smoke tests. Rendered startup
coverage now checks home conflicts with external SQLite and PostgreSQL against
the localized data-folder rule, preserves the database rule for database
targets, and displays the failed test home's path at a 390px viewport. Public
docs validation passed (62 validator tests and 47 published pages). Native
macOS titlebar behavior is tracked in Task 04.

The review follow-up adds assertive terminal announcements and assertive
temporary-launch errors, plus Traditional Chinese script-tag selection. The
smoke tests also verify non-OK retry backoff. The shared Home confirmation
control keeps its accessible name stable and exposes a separate polite status
message while saving; the existing mobile discovery flow retains the same
button and touch target.
