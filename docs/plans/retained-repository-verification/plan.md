---
created: 2026-11-06
status: implemented
requirements:
  - REQ-EXECUTORS-REMOTE-DOCKER-001
system_design:
  - ../../specs/executors/system-design/remote-docker-executor.md
legacy_specs: []
---

# Implementation Plan: Retained Repository Verification

## Overview

One work order restores the existing remote-Docker resume guarantee for
unpublished commits in multiple repositories. The executor system owns the
retained compute and its recovery; task worktree ownership remains unchanged.

## Scope

Port existing branch/origin verification for reused remote Docker only.
Exclude deployments, pilot rebase conflict resolution, credentials, journal
changes, UI, new public APIs and other executor providers.

## Root cause and reproduction

The current launch path materializes repositories even after attaching existing
compute. Ordinary `matchingCheckoutCommit` requires HEAD to equal the remote
tracking commit. A temporary real-Git experiment shows that the correct feature
branch with one unpublished commit is refused without changing HEAD.

## Technical approach

Extract launch reconciliation into `retained_launch_workspace.go`. Select
verification using `RuntimeRemoteDocker` and `ReusedWorkspace`. Port the existing
read-only branch/origin probe with unique completion marker, bounded context
and exact owned-process cleanup. Never fall back to materialization on failure.
Keep `stopLaunchRuntimeOnRollback` and ordinary collision checks unchanged.

| Provenance | Behavior | Evidence |
| --- | --- | --- |
| Remote Docker, reused | Verify all repositories, accept unpublished HEAD | Real Git plus launch HTTP fixture |
| Remote Docker, fresh | Existing materialization | Fresh control |
| Other provider/no reuse | Existing behavior | Provider/provenance controls |
| Missing/mismatched/cancelled | Abort, preserve runtime and source | Negative/cleanup checks |

## Tests and end-to-end evidence

Criterion `AC-EXECUTORS-REMOTE-DOCKER-001.12` covers the real Git regression
for an unpublished commit and the authenticated agentctl launch fixture.
Other tests cover branch and origin failures, missing sources, cancellation,
probe-output lifetime, and fake-Docker rollback. No paid agent, browser change,
or live remote-Docker test ran.

## Work orders

- [x] [Task 01: Verify retained repositories](task-01-verify-retained-repositories.md)

## Verification results

The authenticated launch regression failed before the change with the actual
materialization conflict for an unpublished commit. It now passes while
preserving refs, dirty files and unrelated processes. Fresh/provider controls,
origin/branch/missing-source negatives, process-result lifetime and existing
rollback cases pass. The full lifecycle package passed in 187.592 seconds;
the focused race suite passed in 15.238 seconds. Documentation catalog and
full specification validation also pass. No live executor deployment was used.

## Risks

Process expiry can hide results. Verification must never change refs or
materialize missing repositories. Scope must not weaken ordinary collision checks.
