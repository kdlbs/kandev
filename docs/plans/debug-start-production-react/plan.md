---
created: 2026-11-04
status: implemented
requirements:
  - REQ-PLATFORM-SOURCE-FRONTEND-BUILD-MODE-001
system_design:
  - ../../specs/platform/system-design/source-frontend-build-mode.md
legacy_specs: []
---

# Fix plan: Production React during debug source startup

## Overview

[Issue #5050](https://github.com/kdlbs/kandev/issues/5050) reports development React in a debug source build with inherited `NODE_ENV=development`.
First correct the compilation environment and add a focused build-contract regression.
Then verify the served debug build, document the behavior, and repeat the memory comparison.

This package was implemented on 2026-11-04.
Task 01 pins the compilation environment and guards the build contracts.
Task 02 verifies the served debug build and records the bounded memory observation.

## Evidence and root cause

Investigation used repository commit `81cb56a3550f7fb7fe5c5b4a36a2305882984500`.
The canonical issue had no comments or image attachments at investigation time.
GitHub assignment to `carlosflorencio` succeeded on 2026-11-04.

The root Makefile clears `VITE_KANDEV_API_PORT` and `VITE_KANDEV_DEBUG` for web compilation but leaves `NODE_ENV` inherited.
`start-debug` delegates to `start DEBUG=1`, which uses `build-web-quiet`.
The native Windows frontend command has the same omission.

A temporary pnpm recorder executed the unchanged `build-web`, `build-web-quiet`, and `build-web-e2e` recipes.
Every command received `NODE_ENV=development` while both Vite values were empty.
The recorder performed no installation or application build.

A temporary fixture then used the installed Vite 8.0.16 and React 19.2.3 packages.
With inherited development, Vite bundled React development warnings.
With production, the same fixture omitted those warnings.
A separate configuration probe returned `mode=production` and `isProduction=false` under inherited development.
These observations establish the compilation defect independently of the reported renderer crashes.
All temporary fixtures were removed.

The smallest read-only command trace is:

```bash
NODE_ENV=development make -n build-web build-web-quiet build-web-e2e
```

The planned failing assertion is `check_root_web_build_environment`: expected `NODE_ENV=production`, observed `development` at the pnpm command boundary.
Task 01 must record this behavioral RED before changing the Makefile.

## Requirement conformance and assumptions

Platform owns source startup and diagnostic isolation.
The adjacent UI and Release system boundaries do not own this source-start contract.
Existing diagnostic logging and debug-profile specifications do not define React compilation mode.
The new [requirement](../../specs/platform/requirements/source-frontend-build-mode.md) fills that gap without changing the existing runtime contracts.

The issue settles the intended mode, diagnostic behavior, installation scope, Windows parity, and development-mode compatibility.
No material product choice remains unresolved.
The exact contribution of React timing entries to total renderer memory remains an evidence question.

## Scope

### In scope

- Build-local production environment selection in the four root Makefile frontend commands.
- A regression through actual POSIX recipe execution and Windows recipe-generation assertions.
- A served debug source-build check with production React and working Kandev diagnostics.
- A controlled memory comparison and accurate documentation of its limits.

### Out of scope

- React upgrades, profiling features, console-retention changes, or performance-entry clearing.
- Global `NODE_ENV` changes, package-script changes, and dependency-installation policy changes.
- Vite dev-server environment normalization under an inherited production value.
- Repairing every source of browser memory growth or the quiet build's existing error suppression.
- Production code, permanent tests, commits, pushes, or PR creation during this design turn.

## Technical approach

Add `NODE_ENV=production` beside the existing Vite environment assignments in `build-web`, `build-web-quiet`, and `build-web-e2e`.
Add `set "NODE_ENV=production"` to the native Windows frontend build command.
Keep the assignment after dependency installation and local to the build recipe.
Do not add an abstraction for four short environment prefixes.

Extend `scripts/check-make-shells`; its existing CI entry point already covers repository Make contracts.
Add the temporary recorder and Windows root-recipe assertions there.
Keep production frontend and Go launcher code unchanged unless a focused diagnostic regression proves an additional defect.
If that happens, revise this package before expanding the repair.

Task 02 adds one focused browser integration script under `apps/web/scripts`.
It launches real debug source startup and verifies the frontend assets served by Go.
The script also supports the bounded memory observation described in its work order.
No CI soak job is part of this repair.

## Tests

| Acceptance criteria | Evidence |
| --- | --- |
| `.1`, `.3`, `.4` | `scripts/check-make-shells`: `check_root_web_build_environment`, install isolation, Windows generation, and alias forwarding |
| `.5` | The same recorder checks development commands and parent-environment preservation. Existing launcher web-child tests remain applicable. |
| `.2` | Existing `TestResolveLogThresholdsByMode`, `TestBackendEnvCarriesIndependentThresholds`, and `TestBootPayloadScriptSetsDebugGlobalBeforeBootPayload` |
| `.2`, `.6` | Existing `lib/debug/log.test.ts` and `src/boot-payload.test.ts`, plus served-build evidence |

The abbreviated criteria above belong to `AC-PLATFORM-SOURCE-FRONTEND-BUILD-MODE-001`.

## E2E tests

`apps/web/scripts/debug-start-production.test.mjs` uses Node's test runner and the installed Playwright Chromium browser.
Its shared observation and process-cleanup helpers live under `apps/web/scripts/lib/`.
It covers `.1`, `.2`, and `.6` through the actual `make start-debug` path.
The check is excluded from Vitest and exposed as `pnpm --filter @kandev/web test:debug-start`.
It remains a dedicated manual target because it builds the backend and launches Chromium; it is not part of the default unit-test or CI job.
This source-build integration check does not use the standard E2E fixture, which launches a different backend path.
It owns disposable data and processes and runs headless.

No rendered UI changes are proposed.
The build-mode observation is independent of viewport, so no separate mobile scenario or ASCII preview is necessary.

## Work orders

- [x] [Task 01: Pin production frontend compilation and guard build contracts](task-01-pin-build-environment.md)
- [x] [Task 02: Verify served diagnostics and repeat the memory comparison](task-02-verify-debug-source-build.md)

Execute Task 01 before Task 02.
Both tasks are sequential and do not authorize delegation.

## Verification commands

Task 01 owns:

```bash
make -C apps/backend check-make-shells
(cd apps/backend && go test -trimpath -tags fts5 ./internal/launcher ./internal/webapp -run '^(TestResolveLogThresholdsByMode|TestBackendEnvCarriesIndependentThresholds|TestBootPayloadScriptSetsDebugGlobalBeforeBootPayload)$' -count=1)
```

Task 02 owns:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm run test lib/debug/log.test.ts src/boot-payload.test.ts)
(cd apps/web && NODE_ENV=development node --test scripts/debug-start-production.test.mjs)
(cd apps/web && NODE_ENV=development KANDEV_DEBUG_START_SOAK_MINUTES=35 node --test scripts/debug-start-production.test.mjs)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

The two browser commands run sequentially.
The script requires a fresh source build and must not reuse stale embedded assets.

Package checks:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs/platform docs/plans/debug-start-production-react
git status --short -- docs/specs/platform docs/plans/debug-start-production-react
```

The local PR-documentation coverage preflight uses the repository validator API.
The Makefile change supplies the coverage trigger:

```bash
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const paths = [
  'docs/specs/platform/requirements/source-frontend-build-mode.md',
  'docs/specs/platform/system-design/source-frontend-build-mode.md',
  'docs/plans/debug-start-production-react/plan.md',
  'docs/plans/debug-start-production-react/task-01-pin-build-environment.md',
  'docs/plans/debug-start-production-react/task-02-verify-debug-source-build.md',
  'docs/plans/debug-start-production-react/evidence.md',
];
const result = validateCoverage({
  changedFiles: [
    { filename: 'Makefile', status: 'modified' },
    ...paths.map(filename => ({ filename, status: 'added' })),
  ],
  fileContents: Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, 'utf8')])),
});
console.log(JSON.stringify(result, null, 2));
if (!result.ok) process.exitCode = 1;
NODE
```

## Verification results

Design-package checks passed on 2026-11-04:

- `python3 scripts/list-docs.py validate`: 431 decisions and 1,918 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- PR-documentation coverage preflight: both work orders covered, with no reference errors.
- Specification and plan whitespace checks passed.

Implementation checks passed on 2026-11-04 and 2026-11-05:

- `make -C apps/backend check-make-shells`: the new root web build assertions failed before the
  Makefile change and passed after it, including installation isolation, development compatibility,
  parent-environment preservation, and Windows recipe generation.
- The three launcher/webapp diagnostic tests passed.
- `pnpm install --frozen-lockfile` and the web unit tests `lib/debug/log.test.ts` and
  `src/boot-payload.test.ts` passed.
- The corrected debug source start served production React 19.2.3, pinned a private SQLite database
  over hostile inherited PostgreSQL configuration, matched all 86 served JavaScript assets to the
  fresh build, omitted React development-warning sentinels, kept the measure count at zero, and
  retained the Kandev diagnostics. The 35-minute observation completed 387 required workload
  rounds without a document reload.
- The matched baseline checkout served development React, retained the development sentinels, and
  during its 35-minute observation accumulated 4,378,212 measures and 6.29 GB renderer RSS. Both
  runs used the required workload and the same observation hooks. See [evidence](evidence.md).
- The observation workload required each Home, task-route, Settings, and theme-toggle step and
  asserted its resulting route or control state. Cleanup drained the owned make, launcher, and
  backend process groups with portable POSIX process-table identity checks.
- Public docs validation and the PR-documentation coverage preflight passed.
- Native Windows `cmd.exe` execution and the reporting macOS/Chrome memory behavior remain
  unverified; see the evidence limitations.

The corrected 35-minute observation still recorded renderer memory growth, so this package does
not claim that every renderer crash or all browser memory growth is resolved.

## Risks

- Native Windows recipe generation can be verified on Linux. Actual cmd.exe startup still needs a Windows-capable environment.
- The quiet web recipe can return success after a failed build. The served-build check must reject stale asset identities.
- Browser memory depends on the host, Chrome version, workload, and observation duration.
  A short smoke test cannot establish that renderer crashes are resolved.
- The reporting host's raw heap snapshots and crash dumps are unavailable here.
  A new controlled comparison must state any environment differences.
