---
id: "01-maintenance-json"
title: "Present maintenance JSON"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-MAINTENANCE-JSON-001
acceptance_criteria:
  - AC-PLATFORM-MAINTENANCE-JSON-001.1
  - AC-PLATFORM-MAINTENANCE-JSON-001.2
  - AC-PLATFORM-MAINTENANCE-JSON-001.3
  - AC-PLATFORM-MAINTENANCE-JSON-001.4
  - AC-PLATFORM-MAINTENANCE-JSON-001.5
system_design:
  - ../../specs/platform/system-design/maintenance-json.md
---

# Task 01: Present maintenance JSON

## Summary

Add `--json` to the native maintenance CLI and map its existing outcome to the
reviewed public contract. Preserve guardrails and report committed progress on
post-commit errors; document script usage.

## In scope

- TDD the real entrypoint with isolated disposable SQLite, exact output,
  stderr and exit checks; assert JSON decoder EOF and no stdout pollution.
- Cover default text, dry-run nonmutation, candidate groups, samples and
  unavailable schema; execute backup and lock guard; compaction; deterministic
  error after committed work; and writer failure.
- Add explicit CLI projection and flag/help; update the public reference.

## Out of scope

Domain SQL, policy, lock/backup/compaction redesign, new queries/metrics,
web UI, browser tests, actual user data and new dependencies.

## Acceptance

- All fields, omission rules and semantics match the design and documented
  schema; default text and existing exit/guard behavior are preserved.
- Executed failure emits truthful committed outcome and stderr diagnostic with
  exit 1; write failure exits nonzero without a false success.
- Focused TDD and the checks below pass and actual results are recorded here.

## Verification

Run sequentially only after the separate implementation and HEAVY grant. From
repo root, with unique external log directories per command; keep every native
process handle and observe its actual exit. Install workspace dependencies once
before hooks in this fresh worktree, using pnpm's install root (not `/bin`):

```bash
export PATH="/home/jcfs/.local/share/mise/installs/pnpm/9.15.9:/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:$PATH"
export GOTOOLCHAIN=local
(cd apps && KANDEV_RUN_QUIET_DIR="$(mktemp -d /tmp/kandev-039-install.XXXXXXXX)" ../scripts/run-quiet install -- pnpm install --frozen-lockfile)
(cd apps/backend && KANDEV_RUN_QUIET_DIR="$(mktemp -d /tmp/kandev-039-go-test.XXXXXXXX)" GOMAXPROCS=2 GOFLAGS=-p=2 GOMEMLIMIT=512MiB ../../scripts/run-quiet backend-test -- go test -race -trimpath -tags fts5 -p=1 -count=1 ./internal/launcher -run 'Test(ParseMaintenance|RunMaintenance|Maintenance|PrintMaintenance)')
KANDEV_RUN_QUIET_DIR="$(mktemp -d /tmp/kandev-039-go-lint.XXXXXXXX)" GOMAXPROCS=2 GOFLAGS=-p=2 GOMEMLIMIT=3GiB scripts/run-quiet go-lint -- bash scripts/lint-go-changed apps/backend/internal/launcher/maintenance_database.go apps/backend/internal/launcher/maintenance_database_json.go apps/backend/internal/launcher/maintenance_database_json_test.go apps/backend/internal/launcher/maintenance_database_sigpipe_unix_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check -- docs/specs/platform docs/plans/maintenance-json docs/public/cli.md apps/backend/internal/launcher
git status --short -- docs/plans/maintenance-json
```

Run the focused test in red first after writing meaningful tests, then green
with the same command after implementation. If additional Go files change,
include every changed Go file in the lint invocation. Do not use `--summary`
for these non-E2E tags; no full backend or web suite is required. Name new tests with the `TestRunMaintenance`, `TestMaintenance`, or
`TestPrintMaintenance` prefix so the focused command includes them. Run the
repository PR documentation coverage preflight below against the actual final
changed paths and record its result. Review the exact `jq` example against output.

Run this coverage preflight from the repository root after implementation:

```bash
node <<'NODE'
const fs = require('node:fs');
const assert = require('node:assert/strict');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const paths = [
  'docs/specs/platform/requirements/maintenance-json.md',
  'docs/specs/platform/system-design/maintenance-json.md',
  'docs/plans/maintenance-json/plan.md',
  'docs/plans/maintenance-json/task-01-maintenance-json.md',
];
const fileContents = Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, 'utf8')]));
const tracked = execFileSync('git', ['diff', '--name-only', '-z', 'HEAD'], {encoding: 'utf8'}).split('\0').filter(Boolean);
const untracked = execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z'], {encoding: 'utf8'}).split('\0').filter(Boolean);
const changedFiles = [...tracked.map(filename => ({filename, status: 'modified'})), ...untracked.map(filename => ({filename, status: 'added'}))];
const result = validateCoverage({changedFiles, fileContents});
console.log(JSON.stringify(result));
assert.equal(result.ok, true);
assert.deepEqual(result.errors, []);
NODE
```

## Files likely touched

- `apps/backend/internal/launcher/maintenance_database.go`
- `apps/backend/internal/launcher/maintenance_database_json.go`
- `apps/backend/internal/launcher/maintenance_database_json_test.go`
- `apps/backend/internal/launcher/maintenance_database_sigpipe_unix_test.go`
- `apps/backend/internal/launcher/maintenance_database_test.go` (existing fixtures and text regressions)
- `docs/public/cli.md`
- This plan and work order for final status and results.

## Dependencies

None, after ROOT's full package review and a separate implementation release.

## Risks

Write errors may leave truncated output; post-commit failure and bounded partial
success have different meanings. Tests must never use a real installation.

## Parallelism

sequential

## Inputs

- `REQ-PLATFORM-MAINTENANCE-JSON-001` and the linked system design.
- Existing `maintenance_database.go`, adjacent tests and maintenance outcome.

## Results

ROOT accepted the design package. Real entrypoint RED: `--json` was rejected
with exit 2. After implementation, the focused race test passed; a separate
short-write test failed before the writer fix and the final focused command
passed again. Changed-Go lint passed after correcting diagnostic write checks.
The documented jq query returned `1` from a disposable fixture output.
`list-docs validate`, specification lint, public-doc validator tests (63/63),
public-doc validation (54 pages), and `git diff --check` passed. No actual user
database or runtime was used. Hosted checks and reviews are pending.

CodeRabbit SIGPIPE correction: the Unix subprocess test first confirmed its
control child died from SIGPIPE and observed maintenance terminate by SIGPIPE
without a diagnostic. With the existing platform helper invoked for JSON after
help and usage handling, the closed-pipe test and focused launcher race suite
passed; changed-Go lint passed. No domain behavior changed.
