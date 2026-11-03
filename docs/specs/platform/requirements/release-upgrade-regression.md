---
status: active
system: platform
created: 2026-10-20
owners:
  - kandev
---

# Release upgrade regression requirements

## Overview

Maintainers need executable evidence that an installation created with the
latest published Stable release remains usable after upgrading to a candidate
revision of main. Platform owns this cross-system startup and compatibility
verification contract. Task and agent systems continue to own lifecycle
semantics; this suite exercises those contracts without changing them.

This complements the frozen store-fixture guarantees in
[required persisted store parity](postgres-domain-store-parity.md), especially
`REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-006`. Those fixtures remain valuable
historical migration coverage and are not replaced by this rehearsal.

## Terminology

- **Baseline:** Latest published, non-draft, non-prerelease Stable SemVer release,
  resolved once per run, or an explicitly selected published Stable tag.
- **Candidate:** Exact main commit under test in GitHub Actions.
- **Seed:** Synthetic data created and exercised by the baseline application.
- **Rehearsal:** Baseline seed, upgrade, candidate assertions, second candidate
  boot, and repeat assertions on the same isolated installation.

## Requirements

### REQ-PLATFORM-UPGRADE-REGRESSION-001: Reproducible release rehearsal

**Intent:** A passing result identifies the real upgrade path it exercised.

#### Acceptance criteria

- **AC-PLATFORM-UPGRADE-REGRESSION-001.1:** Each run shall identify the baseline
  tag and source commit, downloaded artifact digest, candidate commit and binary
  digest, database engine/version, seed version, and effective test configuration.
- **AC-PLATFORM-UPGRADE-REGRESSION-001.2:** The baseline shall create the
  installation and seed before any candidate application initializes its storage.
- **AC-PLATFORM-UPGRADE-REGRESSION-001.3:** Each rehearsal shall use disposable
  storage, repository, runtime processes, and synthetic identity without reading
  or modifying a developer installation or using paid provider credentials.
- **AC-PLATFORM-UPGRADE-REGRESSION-001.4:** Missing artifacts, invalid checksums,
  unsupported baseline capabilities, unavailable required databases, and missing
  required scenarios shall fail the requested run with a named cause. A skipped
  scenario shall not count as upgrade acceptance.
- **AC-PLATFORM-UPGRADE-REGRESSION-001.5:** CI shall run only on main, through
  pushes to main, a nightly schedule, or manual dispatch on main. Pull requests
  and other branches shall not launch the rehearsal. Each run shall record the
  exact tested main commit.

- **AC-PLATFORM-UPGRADE-REGRESSION-001.6:** Release acquisition, the upgrade
  runner and direct upgrade browser entry points shall refuse execution outside
  GitHub Actions on main before starting builds, runtime processes or database
  services. The suite shall have no local execution override.

### REQ-PLATFORM-UPGRADE-REGRESSION-002: Persistent state preservation

**Intent:** Successful startup is accompanied by preserved, readable data.

#### Acceptance criteria

- **AC-PLATFORM-UPGRADE-REGRESSION-002.1:** After upgrade, seeded workspaces,
  repositories, workflows and steps, agent/executor profiles, user preferences,
  tasks, sessions, plans, and conversation history shall retain their expected
  identities, relationships, and semantic content. Explicitly expected migration
  transformations shall be asserted rather than ignored.
- **AC-PLATFORM-UPGRADE-REGRESSION-002.2:** After upgrade and after a second
  candidate boot, the installation shall become ready, database integrity and
  relationship checks shall pass, and immutable seed records shall remain
  unchanged with no duplicate records introduced by replay.
- **AC-PLATFORM-UPGRADE-REGRESSION-002.3:** The rehearsal shall pass independently
  on SQLite and PostgreSQL and shall report separate results for each engine.
- **AC-PLATFORM-UPGRADE-REGRESSION-002.4:** When the binary version changes,
  SQLite upgrade backup evidence shall
  demonstrate that the pre-migration snapshot is readable and preserves the
  baseline seed. PostgreSQL results shall not claim automatic SQLite backup
  coverage.

### REQ-PLATFORM-UPGRADE-REGRESSION-003: Task lifecycle after upgrade

**Intent:** Old data remains actionable and new work can still execute.

#### Acceptance criteria

- **AC-PLATFORM-UPGRADE-REGRESSION-003.1:** Seeded pending, waiting, completed,
  archived, and interrupted tasks shall remain inspectable after upgrade.
  Passive history reads shall not create a session, execution, or duplicate turn.
- **AC-PLATFORM-UPGRADE-REGRESSION-003.2:** On the candidate, a seeded pending
  task shall start; a waiting task shall accept a follow-up; a completed task
  shall resume its selected conversation; and an interrupted task shall recover
  through its supported user action. Each action shall produce an observable
  deterministic agent response with the expected task/session identity and
  without duplicated historical messages.
- **AC-PLATFORM-UPGRADE-REGRESSION-003.3:** A candidate shall create and execute a
  new task, stop and resume work, move a task through seeded workflow steps,
  complete work, archive it, and unarchive it. Resulting state shall survive the
  second candidate boot.
- **AC-PLATFORM-UPGRADE-REGRESSION-003.4:** Desktop and phone browser checks
  against the candidate shall open seeded history and send a follow-up through
  their existing task interfaces, rendering the expected response. Backend
  lifecycle checks shall run on both database engines.
- **AC-PLATFORM-UPGRADE-REGRESSION-003.5:** A failed rehearsal shall retain phase
  diagnostics and expected/actual state for the failing scenario. Automated
  results shall distinguish infrastructure failure, preservation failure, and
  lifecycle failure without retrying away a deterministic regression.

## Out of scope

- A guarantee that every feature, executor, real agent provider, plugin, or
  third-party integration works after upgrade.
- Desktop installer/package-manager upgrades, remote executor survival, hot
  upgrades with retained processes, arbitrary older release paths, downgrade,
  and catastrophic interruption during a migration.
- Large-database performance qualification. Small representative seeds prove
  correctness; existing storage benchmarks retain performance ownership.
- New product UI, changed task recovery semantics, or automatic production data
  repair. Regressions discovered by the suite need their own focused fixes.

## Implementation plans

- [Release upgrade regression](../../../plans/release-upgrade-regression/plan.md)
