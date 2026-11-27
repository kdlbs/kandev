# Reproduce the journey investigation

The findings are in [evidence.md](evidence.md). The [implementation package](plan.md) tracks completed work orders and their verified changes.

The local synthetic database is `.kandev/diagnostics/journey-data-efficiency/seed.db`. Git ignores that directory. The database contains no copied production data.

## Prepare an isolated instance

Run these commands from the repository root:

```sh
scripts/dev-isolated --web --timeout 120
```

Use the database path and backend URL from the launcher output. Keep the printed teardown command.

```sh
python3 docs/plans/journey-data-efficiency/seed.py --database /tmp/kandev-iso-PORT-SUFFIX/home/.kandev/data/kandev.db --tasks 10
python3 docs/plans/journey-data-efficiency/measure_boot.py --base-url http://localhost:PORT > /tmp/boot-10.json
python3 docs/plans/journey-data-efficiency/seed.py --database /tmp/kandev-iso-PORT-SUFFIX/home/.kandev/data/kandev.db --tasks 1000
python3 docs/plans/journey-data-efficiency/measure_boot.py --base-url http://localhost:PORT > /tmp/boot-1000.json
python3 docs/plans/journey-data-efficiency/seed.py --database /tmp/kandev-iso-PORT-SUFFIX/home/.kandev/data/kandev.db --tasks 10000
python3 docs/plans/journey-data-efficiency/measure_boot.py --base-url http://localhost:PORT > /tmp/boot-10000.json
```

The seed script accepts only a `dev-isolated` database path. It rejects databases with non-fixture tasks and cannot shrink a fixture. Expansion preserves existing task placement. The 10,000-task comparison requires an existing 1,000-task fixture and adds the extra tasks to another workflow, so selected-board membership stays fixed. A fresh 1,000-task seed has 500 tasks on the selected board. The retained expanded fixture has 495.

The implementation comparison retained a second synthetic backup at `.kandev/diagnostics/journey-data-efficiency/seed-10000.db`; it contains 10,000 fixture tasks and passed SQLite integrity and foreign-key checks.

For production frontend captures, build the web assets:

```sh
pnpm --dir apps --filter @kandev/web build:vite
```

Restart only the owned isolated backend with its existing HOME, database, ports, and mock-provider environment. Omit `KANDEV_WEB_INTERNAL_URL`. Set `KANDEV_WEB_DIST_DIR` to the absolute `apps/web/dist` path. The backend then serves production assets. A Vite development capture does not establish production request counts.

## Capture browser demand

Open a named browser session through the pinned Playwright CLI. Install the recorder before capture. Navigate to the isolated backend before the capture script runs.

```sh
pnpm --dir apps exec playwright-cli -s=dbjourneys open about:blank
pnpm --dir apps exec playwright-cli -s=dbjourneys run-code "$(cat docs/plans/journey-data-efficiency/browser_install.js)"
pnpm --dir apps exec playwright-cli -s=dbjourneys goto http://localhost:PORT
pnpm --dir apps exec playwright-cli -s=dbjourneys run-code "$(cat docs/plans/journey-data-efficiency/browser_capture.js)" > /tmp/journey-browser.log
```

Root-owned Chromium needs a private CLI config with `browser.launchOptions.chromiumSandbox=false`. The investigation used a 1440 × 1000 context viewport. The four-second delay defines an observation window. It is not a readiness or latency assertion.

The output contains JSON after `### Result`. The recorder stores HTTP paths, status, header latency, and WebSocket action metadata. It does not store full WebSocket payloads. Resource entries include response-body sizes where the browser supplies them.

For navigation checks, select another agent tab. Then select another task from the sidebar. Verify that old subscriptions are released. Repeat the multi-session route at 390 × 844.

## Probe database contention

Navigate the owned browser to `about:blank` before the database probes. The first probe removes only fixture summaries to exercise read repair. It restores them through the homepage request.

```sh
python3 docs/plans/journey-data-efficiency/db_probe.py --database /tmp/kandev-iso-PORT-SUFFIX/home/.kandev/data/kandev.db --base-url http://localhost:PORT
python3 docs/plans/journey-data-efficiency/locktrace.py --database /tmp/kandev-iso-PORT-SUFFIX/home/.kandev/data/kandev.db --base-url http://localhost:PORT
```

The probes hold an external writer for a bounded interval on synthetic data. The stack probe requires the isolated debug endpoint. It writes `/tmp/kandev-journey-lock-stack.txt`. An interrupted audit probe can leave `journey_summary_*` diagnostic objects in the disposable database.

## Finish

Close only the owned browser session:

```sh
pnpm --dir apps exec playwright-cli -s=dbjourneys close
```

Run the exact teardown command from the isolated launcher. Use SQLite's backup API to retain the synthetic database before removing its temporary directory. Do not copy only the main database file while WAL writes are active.

## Validate the design package

Run from the repository root:

```sh
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node docs/plans/journey-data-efficiency/validate-package.cjs
git diff --check -- docs/plans/journey-data-efficiency docs/specs/platform
```

The coverage helper supplies a prospective production path to the repository validator. It checks the eight work orders without changing application files.
