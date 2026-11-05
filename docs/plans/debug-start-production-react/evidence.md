# Production React during debug source startup evidence

Observation date: 2026-11-04 to 2026-11-05. Issue: [#5050](https://github.com/kdlbs/kandev/issues/5050).
Source revision for the corrected build: `81cb56a3550f7fb7fe5c5b4a36a2305882984500` plus the
uncommitted build-environment and verification changes described in this package.
Baseline revision: `81cb56a3550f7fb7fe5c5b4a36a2305882984500` in a detached temporary worktree.

## Environment

- Host: Linux 7.0.14-4-pve x86_64. The reporting host in the issue used macOS and Chrome; this
  observation therefore limits causal claims to this Linux host.
- Node.js: `v24.16.0`; pnpm: `9.15.9`.
- Chromium: Playwright `149.0.7827.55`, headless.
- React and React DOM: `19.2.3`; Vite: `8.0.16`.
- Both runs inherited `NODE_ENV=development` from the invoking shell.

## Procedure

`apps/web/scripts/debug-start-production.test.mjs` owns a temporary `KANDEV_HOME_DIR`, a free
loopback `KANDEV_BACKEND_PORT`, the launcher process tree, and the browser context. It seeds an
inherited hostile PostgreSQL configuration and environment that point at an outside sentinel
database, then pins a private configuration file, `database.driver=sqlite`, and a database path
under the owned home. It removes `apps/web/dist` before startup so the quiet build recipe cannot
serve stale assets, then launches the actual `make start-debug` path with
`KANDEV_SERVER_HOST=127.0.0.1` and `KANDEV_NO_BROWSER=1`. It clears inherited source selectors,
web-dist overrides, and diagnostic threshold overrides for the child only.

The check asserts the launcher's resolved database path from its startup output and requires the
owned SQLite file before any API write. After the task workload it re-reads the outside sentinel
and fails if the hostile database changed.

The check is excluded from Vitest and exposed as `pnpm --filter @kandev/web test:debug-start`.
It remains a dedicated manual target because it builds the backend and launches Chromium; it is
not part of the default unit-test or CI job.

The browser installs the React DevTools observation hook before application scripts execute and
requires an injected `react-dom` renderer. The check also fetches every JavaScript asset referenced
by the served shell, compares each SHA-256 digest with the fresh build, and scans the served bytes
for React development-warning sentinels.

The disposable dataset is one default workspace, one Development workflow, and one task titled
`5050 memory probe`. The required workload stays inside the SPA: open Home, open the task route
by its created ID, return Home, open Settings, toggle the theme and require the label to change,
then return Home. Every step asserts its route or control state. Missing controls, failed clicks,
or a document reload fail the check, and the report records the successful workload round count
and the document load count.

Samples record `performance.getEntriesByType("measure")` counts and dominant names, Chromium
JavaScript heap usage from `Runtime.getHeapUsage`, and the summed RSS of renderer processes
reported by `SystemInfo.getProcessInfo` and read through `ps`. Entries are never cleared between
samples.

Commands:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && NODE_ENV=development node --test scripts/debug-start-production.test.mjs)
(cd apps/web && NODE_ENV=development KANDEV_DEBUG_START_SOAK_MINUTES=35 node --test scripts/debug-start-production.test.mjs)
baseline_dir="$(mktemp -d "${TMPDIR:-/tmp}/kandev-5050-baseline.XXXXXX")"
git worktree add --detach "$baseline_dir" 81cb56a3550f7fb7fe5c5b4a36a2305882984500
(cd "$baseline_dir/apps" && pnpm install --frozen-lockfile)
(cd apps/web && NODE_ENV=development KANDEV_DEBUG_START_REPO="$baseline_dir" KANDEV_DEBUG_START_EXPECT_REACT=development node --test scripts/debug-start-production.test.mjs)
(cd apps/web && NODE_ENV=development KANDEV_DEBUG_START_REPO="$baseline_dir" KANDEV_DEBUG_START_EXPECT_REACT=development KANDEV_DEBUG_START_SOAK_MINUTES=35 node --test scripts/debug-start-production.test.mjs)
git worktree remove --force "$baseline_dir"
```

## Short smoke observations

| Measurement | Corrected build | Baseline build |
| --- | --- | --- |
| `react-dom` renderer package | `19.2.3` | `19.2.3` |
| Renderer `bundleType` | `0` (production) | `1` (development) |
| Served assets with development warning sentinels | none | 3 (`react`, `settings-floating-save`, `table-resize`) |
| Counted performance measures, start to end | `0` to `0` | `6,947` to `33,356` |
| JavaScript heap used, start to end | `66,292,396` to `132,208,880` bytes | `75,671,452` to `136,908,120` bytes |
| Renderer process RSS, start to end | `365,621,248` to `484,605,952` bytes | `408,076,288` to `546,693,120` bytes |
| Required workload rounds | 2 | 2 |
| Document loads during observation | 0 | 0 |

The short runs lasted about 36 seconds and 40 seconds. The baseline workload created measures
continuously even though its JavaScript heap did not grow monotonically during the observed
interval. That makes the measure count a stronger signal than a short heap or process-memory
delta. Both runs completed the required Home, task route, Settings, and theme-toggle sequence
without a document reload.

Both builds satisfied the diagnostic assertions: `window.__KANDEV_DEBUG === true`, a namespaced
Kandev `console.debug` message, an accessible `/debug/pprof/` index, a debug event in
`<home>/logs/backend-logs.log`, and `KANDEV_DEBUG_AGENT_MESSAGES=true` and
`KANDEV_DEBUG_PPROF_ENABLED=true` in `<home>/supervisor/launch.json`.

## Bounded soak observation

Both replacement runs use `KANDEV_DEBUG_START_SOAK_MINUTES=35`, the mandatory workload, the same
observation hook, and fresh browser contexts against separate disposable homes. Every workload
round completes the asserted Home, task route, Settings, and theme-toggle sequence.

| Measurement | Corrected build | Baseline build |
| --- | --- | --- |
| Required workload rounds | 387 | 351 |
| Document loads during observation | 0 | 0 |
| Counted performance measures | `0` at every sample | `6,928` to `4,378,212` |
| JavaScript heap used | start `60,774,560`, end `84,724,548` bytes | start `62,798,752`, end `234,619,332` bytes |
| JavaScript heap range | `60,774,560` to `125,374,232` bytes | `62,798,752` to `322,892,048` bytes |
| Renderer process RSS | start `379,092,992`, end `663,306,240` bytes | start `411,025,408`, end `6,291,087,360` bytes |
| Renderer process RSS range | `379,092,992` to `676,634,624` bytes | `411,025,408` to `6,291,087,360` bytes |

The corrected test process ran for 35.62 minutes; its 37 samples span 35.01 minutes. It served
production React, kept the measure count at zero, completed 387 required workload rounds without a
document reload, and retained every diagnostic assertion. The baseline process ran for 35.60
minutes; its 37 samples span 35.03 minutes and completed 351 required workload rounds. It served
development React, retained the development sentinels, accumulated 4,378,212 measures, and reached
6.29 GB renderer RSS. The dominant baseline measure names were React component integration
measures such as `ForwardRef`, `Primitive.div`, `PopperProvider`, `Popper`, and `PopperAnchor`.

The corrected build still grew its renderer RSS by `284,213,248` bytes over the observation.
This repair removes the unintended React measure instrumentation, but it does not establish a
bound on renderer memory or prove that renderer crashes stop. The issue's 34-minute heap
observation is reproduced as a measure-count difference; the remaining process-memory growth is
recorded without expanding this patch.

## Unavailable measurements

- Heap-snapshot `PerformanceMeasure` counts and retained `UserTiming` bytes were not collected.
  The report includes explicit unavailable entries for both and does not infer them from live
  heap usage.
- The reporting macOS and Chrome environment is unavailable. Results apply to the Linux host above.
- Native Windows `cmd.exe` execution was not observed. The native recipe is covered by dry-run
  generation assertions, not by a Windows launcher run.
- Renderer crash attribution is unavailable. The issue's crash dumps and raw heap snapshots were
  not present in this workspace.

## Limitations

- The short smoke runs cannot establish a memory bound or prove that renderer crashes stop.
- JavaScript heap usage, renderer process RSS, and performance measure counts are separate
  measurements and can move in different directions.
- A 35-minute soak remains shorter than long-running desktop sessions and does not cover every
  navigation path, plugin, or agent workload.
- The baseline and corrected runs use identical hooks, workload, and dataset, but they still run
  on one host at different times.
