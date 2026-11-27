---
created: 2026-11-08
status: complete
requirements:
  - REQ-AGENTS-SHARED-PROFILE-SECRET-001
  - REQ-AGENTS-SHARED-PROFILE-SECRET-002
  - REQ-EXECUTORS-PREPARED-LAUNCH-ACTOR-001
system_design:
  - ../../specs/agents/system-design/shared-profile-secrets.md
  - ../../specs/executors/system-design/prepared-launch-actor.md
legacy_specs: []
---

# Implementation plan: Shared profile account boundaries

## Overview

This plan delivers two fixes:

- [#5167](https://github.com/kdlbs/kandev/issues/5167): shared agent profiles
  whose global secret bindings resolved only for the secret owner.
- The [#5181](https://github.com/kdlbs/kandev/issues/5181) acceptance: a
  prepared-workspace start that could reach the runtime unscoped.

Both are account boundaries on the same launch. The first adds an explicit,
owner-granted shared use. The second refuses a launch that has no identity to
run as. They are implemented as two independent work orders against base
`52309e613fc9a76d38a9f891b74b4d8b32bff265`.

The contracts are defined in:

- [Shared agent profile secrets requirements](../../specs/agents/requirements/shared-profile-secrets.md)
  and [system design](../../specs/agents/system-design/shared-profile-secrets.md).
- [Prepared launch actor requirements](../../specs/executors/requirements/prepared-launch-actor.md)
  and [system design](../../specs/executors/system-design/prepared-launch-actor.md).

## Scope

### In scope

- A per-binding owner grant on agent-profile secret bindings. It is set and
  revoked only by the secret owner through the profile API, and re-verified on
  every save.
- Launch-time grant resolution. It requires all of the following:
  - an authenticated starter;
  - an active grantor in the same organization;
  - a global secret that the grantor still owns.
- Actionable launch errors that never contain secret values or identifiers.
- A required launch actor for `startAgentOnPreparedWorkspace`.

### Out of scope

The exclusions in both requirement documents apply. In particular, this work
does not add:

- a web UI control;
- grants on executor profiles, duplicated profiles, or nested agent creation;
- grants for the discovery probe, GitHub bridge, or coordinator containment
  checks;
- workspace-secret or cross-organization grants;
- inferred admin or global ownership;
- per-account profile copies;
- the cold `ResumeSession` fix, which is in draft PR
  [#5176](https://github.com/kdlbs/kandev/pull/5176).

## Work orders

| ID | Work order | Wave | Depends on | Status |
| --- | --- | --- | --- | --- |
| 01 | [Prepared launch actor](task-01-prepared-launch-actor.md) | 1 | none | done |
| 02 | [Shared profile secret grants](task-02-shared-profile-secret-grants.md) | 1 | none | done |

## Tests

- Unit and integration tests with synthetic accounts, synthetic secret
  values, and disposable SQLite stores under `t.TempDir()`.
- No live install, real credentials, or PostgreSQL fixture is used.
- Negative controls run on an unmodified export of the base commit.

## Verification results

All commands ran from `apps/backend` with Go `-trimpath` and bounded
timeouts.

| Gate | Result |
| --- | --- |
| Focused `-race` run of the 33 new top-level tests (six packages) | pass |
| `go build ./...` | exit 0 |
| `go vet` of the nine changed packages | exit 0 |
| `golangci-lint` v2.9.0, `--new-from-rev` base, nine changed packages | 0 issues |
| `gofmt -l` on changed Go files | clean |

Package `-race` runs:

- **Pass:** `secrets`, `agent/settings/*`, and `orchestrator`.
- **Fail on both base and fix:**
  - `agent/runtime/lifecycle`, `task/service`, and `backendapp`. The fix
    failure set is a strict subset of the base failure set (77 against 81).
  - `orchestrator/executor`, with the same six failures on base and fix.
- **Cause:** every shared failure is the host refusing Git maintenance
  (`git worktree remove` / `pack-refs`: "Shared Aeries Git maintenance is
  host-owned"). That guard was not bypassed.
- **Base-only failures:** four Kubernetes worker-preparation tests read
  `k8s/worker-images/full/prepare.sh`, which the base source export lacked.

Negative controls on the unmodified base:

- **#5181:**
  - Three cases fail on base and pass with the fix:
    `TestPreparedSessionAdmission_UnattributedSessionFailsClosed`,
    `_UnavailableDurableActorFailsClosed`, and
    `_ConcurrentUnattributedStartsStayClosed`.
  - The other eight cases already pass on base, which shows that current main
    already enforces those boundaries:
    - caller precedence;
    - no fallback for an unauthorized caller;
    - owner and collaborator actors;
    - auth disabled;
    - private profile;
    - cancellation;
    - a single concurrent launch.
- **#5167:** A temporary base reproduction resolved the owner's synthetic
  binding but failed the same-organization collaborator with an unactionable
  `env key "SHARED_TOKEN" unavailable`. Base has no grant field. The temporary
  test was removed after the run.

Mutation control on the fix:

- In a disposable source export, the executor-profile path was changed to
  resolve `profile.EnvVars` without `withoutSecretShares`. With that change,
  `TestExecutorProfileBindingsNeverHonorAgentProfileShares` failed because the
  collaborator resolved the shared value. Restoring the file made the test
  pass again. The candidate checkout was not modified.

Known limits:

- Only `startAgentOnPreparedWorkspace` requires a launch actor. Other prepared
  and internal starts keep their existing contracts, and some bookkeeping runs
  before admission. See
  [Scope and limits](../../specs/executors/system-design/prepared-launch-actor.md#scope-and-limits).

## Risks

- **Disclosure.** A grant discloses the value to every authorized starter's
  agent process. That is intended, but owners must understand it.
- **Retention on edit.** A UI save that omits `shared` keeps grants on
  unchanged bindings. A collaborator can keep a grant but not create or move
  one.
- **Different refusal.** An internal prepared start whose recorded actor was
  later deleted is now refused where it previously ran unscoped. Operators
  must start it as an authorized user.
