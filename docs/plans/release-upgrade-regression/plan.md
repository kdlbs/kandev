---
created: 2026-10-20
status: implemented
requirements:
  - REQ-PLATFORM-UPGRADE-REGRESSION-001
  - REQ-PLATFORM-UPGRADE-REGRESSION-002
  - REQ-PLATFORM-UPGRADE-REGRESSION-003
system_design:
  - ../../specs/platform/system-design/release-upgrade-regression.md
legacy_specs: []
---

# Implementation plan: Release upgrade regression

## Overview

Add a baseline-first, two-binary upgrade rehearsal alongside existing frozen
SQL migration tests. First establish trustworthy release acquisition and a
baseline-only seed, then assert preservation/replay, then exercise task runtime
and browser flows, and finally wire the CI-only runner and main-only workflow.
Implementation and CI acceptance are complete. The implementation is test/CI infrastructure; product runtime code is unchanged.

## Scope

### In scope

- Latest published Stable to exact main SHA in CI, with explicit Stable replay.
- CI only on main pushes, nightly schedule and manual dispatch on main.
- Real released backend and matching host agentctl; baseline-created data.
- Small representative SQLite and PostgreSQL 16 installations.
- Deterministic mock agent, runtime/task lifecycle, immutable data oracle,
  SQLite backup, second boot, desktop and phone browser smoke.
- CI-only execution, dispatch, phase logs and failure artifacts.

### Out of scope

- Real external providers, all remote executors, Office autonomous lifecycle,
  desktop installer upgrades, rollback, performance qualification, and product
  fixes discovered while building the suite.
- Replacement or automatic regeneration of v0.93.0 conformance fixtures.
- Product UI changes. Browser tests use current shipped interfaces; no UI
  preview or new mobile composition is required.

## Technical approach

Use the [paired design](../../specs/platform/system-design/release-upgrade-regression.md)
for process sequencing and oracles. Do not use ordinary `backendFixture` initial
startup, since it runs the candidate before seeding. Keep baseline adapters
small and explicit. Run production profiles with logged deterministic overrides;
verify mock protocol support before seeding. Old and new helpers must belong to
their respective runtime bundles.

Dedicated `upgrade-desktop` and `upgrade-mobile` projects select only
`tests/upgrade/` specs; exclude these from ordinary chromium/mobile-chrome
projects. The new fixture consumes the checked manifest and selected engine.
Fail when the manifest or required engine is missing, rather than skip.

| Surface | Identity/transport | Intended evidence | Unsupported case |
| --- | --- | --- | --- |
| Stable backend + agentctl | Published archive and SHA256 | API seed and real deterministic turn | Fail named preflight capability |
| SQLite | Disposable persisted home | Preservation, integrity, backup, lifecycle, replay | Fail |
| PostgreSQL 16 | Owned isolated database | Same preservation/lifecycle/replay assertions | Fail if absent |
| Candidate desktop/phone | Go-served Vite build + HTTP/WS | History and follow-up on SQLite | Fail selected browser run |
| Real providers/remote executors | Not selected | No claim | Explicit exclusion |

## Tests

Implemented tests and evidence:

- `scripts/upgrade/prepare-release.test.mjs`: Stable selection, archive digest,
  path validation, immutable manifest, download failure. Covers 001.1/001.4.
- `e2e/fixtures/upgrade-backend.test.ts`: baseline-first spawn order, owned
  cleanup on failure, helper isolation and required-engine checks. Covers
  001.2/001.3/001.4.
- `e2e/helpers/upgrade-oracle.test.ts`: detects missing/changed/duplicate records
  and unexpected migration transformations. Covers 002.1/002.2 and 003.5.
- `tests/upgrade/release-upgrade.spec.ts`: named `baseline seed provenance`,
  `preserves release data and replays startup`, and `executes old and new task
  lifecycles after upgrade`. Covers all 002 and 003.1/003.2/003.3/003.5.
- `scripts/upgrade/run.test.mjs`: validates scenario completeness, engine
  failure propagation, shared baseline manifest, and cleanup. Covers 001.4/003.5.
- `scripts/upgrade/workflow-contract.test.mjs`: main-only triggers/job guards
  and candidate commit pinning. Covers 001.5.

IDs above abbreviate the matching `AC-PLATFORM-UPGRADE-REGRESSION-*` prefix.
Use negative mutations of an oracle input to demonstrate sensitivity; a suite
that passes only healthy data is insufficient evidence of useful assertions.

## E2E tests

- `release-upgrade.spec.ts`, `upgrade-desktop`: SQLite and PostgreSQL API/runtime
  lifecycle; SQLite desktop history and follow-up. AC 003.1 through 003.5.
- `mobile-release-upgrade.spec.ts`, `upgrade-mobile`: separate SQLite rehearsal
  followed by phone history and follow-up. AC 003.4.
- Preserve existing `TestPreviousStableUpgrade` on SQLite/PostgreSQL as the
  frozen historical store gate. It does not substitute for the new live gate.

One worker per shard, separate isolated installation per project/engine. Run
only the main-only `Release upgrade regression` GitHub Actions workflow. Its
`baseline` input accepts `latest` or an exact Stable tag. Acquisition, the runner,
the managed E2E entry point and direct Playwright startup reject local execution.
There is no local override. All verification commands in work orders are CI job
commands, not local reproduction instructions.

## Work orders

- [x] [Task 01: Baseline-first release fixture](task-01-baseline-fixture.md)
- [x] [Task 02: Preservation and replay oracles](task-02-preservation-replay.md)
- [x] [Task 03: Runtime and browser lifecycle](task-03-task-lifecycle.md)
- [x] [Task 04: CI-only runner and workflow](task-04-runner-ci.md)

Execute sequentially; no delegation is requested or authorized.

## Verification results

CI acceptance passed on 2026-10-20 in [run 39510140130](https://github.com/kdlbs/kandev/actions/runs/39510140130).
Published Stable `v0.97.0` was compared with candidate `43e1a82ceb639d083d31d5af3b586ee05173dc34`:
SQLite desktop 3/3, PostgreSQL desktop 3/3, and SQLite phone 1/1 scenarios passed,
with no skipped or retried scenario. The harness contracts (10 Node tests and
6 fixture/oracle tests), changed-file lint, action pinning tests (9), shell syntax,
and specification/catalog checks passed. All three evidence uploads succeeded.

The user authorized this pre-merge branch validation. A temporary exact-branch
exception enabled it; the final policy-only follow-up restores main-only workflow,
runner and fixture guards and rejects this branch in the guard contract. That
follow-up is not claimed as an additional upgrade rehearsal. No upgrade workloads
ran locally after the user's CI-only instruction. Product runtime code is unchanged.

## Risks

- The latest published release can change APIs or mock protocol. A preflight
  capability failure must be diagnosed, not weakened to a skip or repaired with
  candidate-written seed data.
- Interrupted recovery and completed resume have distinct semantics. Preserve
  observed identities and assert supported actions rather than forcing a
  particular state merely to make the suite pass.
- A deterministic provider proves orchestration, not real provider resumption.
- Small synthetic seeds cannot establish arbitrary production database health.
- Release download availability is an infrastructure dependency. Report it
  separately while keeping the requested gate failed.
- Main-only CI discovers regressions after merge. Nightly runs also detect a
  changed Stable baseline even when no new main commit is pushed.

## Run summary follow-up

Each Actions job now publishes an always-run Markdown summary with the verified
Stable tag, source SHA and archive digest, candidate SHA, workflow identity,
database/browser project, scenario results and durations, and evidence artifact
name. Acquisition lists the planned seven scenarios. Failed setup, absent reports,
skips, duplicate results and retries remain explicit; summary formatting cannot
turn a failed runner into acceptance. Formatter/output and workflow-wiring tests
are included in CI. User-requested demonstration run
[39612973568](https://github.com/kdlbs/kandev/actions/runs/39612973568)
compared `v0.97.0` with `e42dbdce9ae23f19128ddacb6863466201740687`:
all seven scenarios and summary tests passed, and all four summary steps
succeeded. GitHub rejected the first attempt before starting jobs because status
functions are not available in step env expressions; `job.status` corrected the
workflow and is covered by its contract test. The temporary exact-branch
exception is removed in the final follow-up; main-only execution is restored.
No upgrade workloads or formatter tests ran locally.

## PR review remediation

Review remediation strengthens post-action message preservation and requested lifecycle outcomes, isolates upgrade projects from default Playwright discovery, bounds PostgreSQL readiness, and removes the nonexistent single-shard log reference. Prior exact-head CI evidence is historical; these changes require a fresh CI upgrade rehearsal and PR checks. No local workload or test execution is permitted.

Remediation verification passed in [CI run 39686419086](https://github.com/kdlbs/kandev/actions/runs/39686419086)
at candidate `59bfe1d15b23eed7e25763b8cb0b3a60120fb9ce`: all seven upgrade
scenarios, 15 Node contracts and 10 fixture/config/oracle tests passed. The
worker-selection regression found in the first remediation run is covered by
an inherited-environment config test. Main-only guards are restored in the
policy/documentation follow-up; this is not an additional upgrade rehearsal.
PR checks and review/merge gates remain external and pending at commit time.
