---
id: "01-quote-service-environment"
title: "Quote service environment values"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-CLI-NATIVE-KANDEV-CLI-001
acceptance_criteria:
  - AC-CLI-NATIVE-KANDEV-CLI-001.9
  - AC-CLI-NATIVE-KANDEV-CLI-001.10
  - AC-CLI-NATIVE-KANDEV-CLI-001.11
system_design:
  - ../../specs/cli/system-design/native-service-configuration.md
---

# Task 01: Quote Service Environment Values

## Summary

Separate systemd environment-value serialization from command-dollar escaping.
Independently authored renderer regressions must expose the dollar duplication
before the correction and protect command, PATH-specifier, and launchd behavior.

## Authorization and dependencies

None between work orders; this is the only work order. Do not execute until ROOT
reviews the design package and sends a later explicit implementation interrupt
with the sole global heavy lease. Remain in the same primary session/profile;
no delegates, tasks, sessions, or model switches. Design checks below are light;
product tests, lint, dependency installation, and hooks belong to the later phase.

## Owned files

- `apps/backend/internal/launcher/service.go`: environment quoting only.
- `apps/backend/internal/launcher/service_test.go`: correct the two erroneous
  environment expectations, including the misleading test name.
- `apps/backend/internal/launcher/service_environment_test.go`: new portable
  renderer regression and context controls.
- `docs/specs/cli/requirements/native-kandev-cli.md`: only AC 001.9-001.11 and
  their design link; retain the broader draft status and unrelated legacy text.
- `docs/specs/cli/system-design/native-service-configuration.md`: scoped design
  consistency and lifecycle after verification; no full CLI migration.
- `docs/plans/systemd-environment-literals/plan.md` and this file: progress and
  actual command/evidence results.
- `apps/web/e2e/tests/task/task-list.spec.ts`: later ROOT-authorized required-CI
  correction, limited to settling filtered page-one setup before pagination.

No other production or permanent test files are owned. No public documentation
change is planned; see the manifest's documentation judgment.

## In scope

1. Use `/tdd` and backend test guidance. Author fixture input and full expected
   assignment lines independently. Test user/system renderers with home, selected
   config, derived metadata, bundle, and accepted PATH-prefix dollars, including
   one `$`, adjacent `$$`, variable-shaped `$USER`/`${HOME}`, and a trailing `$`.
   Include plain paths and mixed spaces/double quotes/backslashes/percent. For
   `HomeDirFromConfig`, retain omission of `KANDEV_HOME_DIR` while selected config
   and derived metadata remain covered.
2. Assert literal percents are `%%` in ordinary assignments; trusted PATH `%h`
   remains single and active, beside dollar-bearing accepted prefixes. Retain
   prefix eligibility filtering, including rejected percent-bearing prefixes.
   Compare exact lines, not helper-derived expectations or a substitute parser.
3. Before production edits, run the named regression and record expected dollar
   failures plus passing controls. Correct the two old expectations explicitly
   against systemd semantics. Give environment quoting a separate context while
   retaining command escaping. Check exact `ExecStart=` output with special
   characters, unchanged `--headless`, and unchanged launchd/XML output.

## Out of scope

No schema/API/SDK/new flags/defaults, service identity/ownership/update policy,
lifecycle or service-manager changes, shell injection policy, template/script
expansion, dev launcher, passthrough, UTF-8 work, physical installed startup,
backend/agent/database launch, service installs/restarts, real configuration
changes, or persistent user units. No broad backend lint/test gate, new parser,
or test-only export. No new native-manager smoke without separate authorization.
Never access or replay ROOT's private `candidate_test.go` or `run.py`.

## Acceptance

1. Full-renderer dollar cases fail on the unmodified renderer for the qualified
   reason and pass after correction, alongside plain/ordinary character controls
   in both scopes (AC 001.9-001.10). Selected-config and metadata paths are covered
   even when configuration-owned home emission is omitted.
2. PATH retains exact accepted-prefix dollars and trusted `%h`; literal percent
   handling and prefix rejection remain intact (AC 001.11). `ExecStart=` and
   launchd controls match the existing command/XML contracts.
3. The complete launcher package, canonical changed-Go complete-package lint,
   light documentation coverage/specification gates, and normal active hooks
   pass. Record exact results and joins. Update only this work order and manifest
   to done/implemented after checks; do not promote the broad legacy draft
   requirement as a side effect. The scoped design may become current once its
   covered criteria are verified.

## Verification

Run from the repository root. Recheck HEAD/main and source hashes before
implementation; if main changed, assess whether ROOT's qualification still
applies without replaying it unchanged. Confirm Go 1.26 before the later Go
commands. Run heavy commands serially under the lease; retain every native
session handle and join the actual process. Do not infer completion from a
duplicate run or exit status from a timeout wrapper alone.

First RED, after authoring the regression and before production edits:

```bash
(cd apps/backend && env GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 180s go test -race -trimpath -tags=fts5 -p=1 ./internal/launcher -run '^TestRenderSystemdUnitPreservesEnvironmentLiterals$' -count=1 -v)
```

After the fix, run the complete owning package once, then changed-Go lint:

```bash
(cd apps/backend && env GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 180s go test -race -trimpath -tags=fts5 -p=1 ./internal/launcher -count=1)
env GOMAXPROCS=2 GOMEMLIMIT=3GiB timeout --kill-after=10s 360s bash scripts/lint-go-changed apps/backend/internal/launcher/service.go apps/backend/internal/launcher/service_test.go apps/backend/internal/launcher/service_environment_test.go
```

The lint helper owns the internal five-minute complete-package scan; the outer
limit is 360 seconds with a ten-second kill grace. Do not replace it with broad
backend lint or escalate resource bounds on failure.

Light design/specification gates (also permitted during design). In this
workspace, Node is installed at
`/home/jcfs/.local/share/mise/installs/node/24.21.0/bin/node`; add that existing
bin directory to the invoking shell's PATH when `node` is absent. No install is
needed to run this preflight.

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/systemd-environment-literals
node <<'NODE'
const fs = require('node:fs');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const paths = [
  'docs/plans/systemd-environment-literals/plan.md',
  'docs/plans/systemd-environment-literals/task-01-quote-service-environment.md',
  'docs/specs/cli/requirements/native-kandev-cli.md',
  'docs/specs/cli/system-design/native-service-configuration.md',
];
const owned = [...paths,
  'apps/backend/internal/launcher/service.go',
  'apps/backend/internal/launcher/service_test.go',
  'apps/backend/internal/launcher/service_environment_test.go',
];
const list = args => execFileSync('git', args, { encoding: 'utf8' })
  .split('\n').filter(Boolean);
const tracked = list(['diff', '--name-only', 'HEAD', '--', ...owned]);
const untracked = list(['ls-files', '--others', '--exclude-standard', '--', ...owned]);
const actualPaths = [...new Set([...tracked, ...untracked])];
const fileContents = Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, 'utf8')]));
const evaluate = filenames => validateCoverage({
  changedFiles: filenames.map(filename => ({ filename,
    status: untracked.includes(filename) ? 'added' : 'modified' })),
  fileContents,
});
const actual = evaluate(actualPaths);
console.log(JSON.stringify({ scope: 'actual owned changed paths', ...actual }, null, 2));
const hasGo = actualPaths.some(p => p.endsWith('.go'));
if (!actual.ok || actual.errors.length || (hasGo && actual.status !== 'covered')) {
  process.exit(1);
}
if (!hasGo) {
  const simulated = evaluate([...actualPaths, 'apps/backend/internal/launcher/service.go']);
  console.log(JSON.stringify({ scope: 'SIMULATED planned Go scope; design links only',
    ...simulated }, null, 2));
  if (!simulated.ok || simulated.errors.length || simulated.status !== 'covered') {
    process.exit(1);
  }
}
NODE
```

This preflight reports actual owned changes, including untracked design,
work-order, and Go test files. During design, a legitimate Markdown-only
`exempt` result passes; the separately labelled simulated Go input validates
design links only. After implementation, the actual Go-containing scope must
return `covered` with no errors. Simulated coverage never proves actual source
coverage. Neither result is a hosted PR status or a product test result.

Before normal active hooks in the later delivery phase, install worktree
dependencies from `apps/` with `pnpm install --frozen-lockfile` once only if
missing. Use the available Node 24.21/pnpm 9.15.9; no install during design.
Load `/commit`, `/push`, and `/pr` at their authorized phases. Keep published
head frozen except actual required correctness/CI fixes; do not amend, bypass,
delete branches, wipe caches, kill foreign processes, rebase for drift alone,
or run synthetic merged tests. No optional bot/style cleanup pushes.

After the ready PR, return the global heavy lease with actual command joins and
current known-own absence, then directly start one attached original 90-minute
observer with GNU 91-minute limit, ten-second kill grace, 60-second cadence and
all-terminal reporting in the same turn, as authorized by ROOT. No manual full
bot reviews on each SHA. ROOT owns the later static compatibility check and a
separate serial expected-head normal squash grant; do not merge without it.

## Risks and inputs

Inputs: paired requirement/design; `service.go`/`service_test.go` patterns; the
manifest's qualified evidence; authoritative systemd Environment semantics.
The primary risks are conflating command/environment escaping and confusing
trusted PATH specifiers with literal percent paths. External manager evidence
proves fixture delivery only; portable output controls make no installed-startup
or database-damage claim.

## Parallelism

`sequential`

## Results

Implemented after ROOT's later explicit release. Independent renderer RED
joined exit 1 with 20 dollar failures and 8 passing plain/ordinary controls;
pre-fix context RED joined exit 1 with a PATH dollar failure and passing
ExecStart/launchd controls. Complete owning launcher GREEN joined exit 0
(18.678s); canonical changed-Go complete-package lint joined exit 0 (`0 issues`).
Exact commands, handles, resource bounds, and the corrected transient overlap
are recorded in the manifest.

Catalog validation, all-spec lint, and diff check passed. Actual implementation
coverage includes all seven owned changed paths, including untracked Go test,
design and work order: `covered`, no errors. One missing-dependency frozen
install joined exit 0 with existing pnpm 9.15.9/Node 24.21.0. Normal active-hook
and publication receipts are recorded in the delivery session. The unchanged
linter's already-run 36-test suite remains historical evidence only.

The broad CLI requirement remains draft; only this scoped design becomes
current. No native service changes, Kandev startup, or private fixture replay.

### Required-CI correction

ROOT subsequently authorized one test-only correction after E2E shard 24 failed
the pagination case on its original attempt and three hosted retries. The trace
shows Next requested unfiltered page two before the 300ms search debounce settled;
filtered page two and then page one followed. The test and implicated frontend
files were byte-identical to main. This is independent of service serialization.

The existing test now registers a successful filtered page-one response wait
before filling search, then awaits that response before its existing full page,
row, total, and page-size assertions. No sleep, timeout increase, retry change,
assertion weakening, or production pagination change. Test-only synchronization
does not change mobile behavior or require a new mobile case.

Hosted assertion and trace provide RED. The canonical guarded host runner passed
exactly this one Chromium case with one worker and zero retries (13.2s; native
16517 joined exit 0, group absent). An initial anchored selection built fresh
artifacts but selected zero tests and exited 1; the corrected exact-one discovery
and successful run reused those artifacts through `--no-build`. Scoped ESLint
and Prettier checks joined exit 0. The app tsconfig excludes E2E, so its typecheck
would not validate this spec. The already-green launcher suite was not replayed.

ROOT later released actual conflict integration with main `9c24ab73ec`, whose
independent pagination wait provides the same synchronization. Ordinary merge
returned the expected conflict exit 1 for this test alone. Resolution selected
the exact upstream test, preserving all assertions and unrelated upstream work.
Because main changed production backend/web files, one fresh canonical guarded
build preceded the focused case: one Chromium test, one worker, zero retries,
7.3s, native 53861 joined exit 0 with its owned group absent. The test now matches
main, so the final PR difference returns to the seven service/design/plan files.
No launcher-suite or native-service replay was needed. Hosted checks on the
integrated published head remain a delivery requirement.
