---
id: "09-shared-environments"
title: "Workspace reuse and shared environments (M1)"
status: pending
wave: 6
depends_on:
  - "08"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-PLUGIN-SHARED-001
acceptance_criteria:
  - AC-EXECUTORS-PLUGIN-SHARED-001.1
  - AC-EXECUTORS-PLUGIN-SHARED-001.2
  - AC-EXECUTORS-PLUGIN-SHARED-001.3
  - AC-EXECUTORS-PLUGIN-SHARED-001.4
  - AC-EXECUTORS-PLUGIN-SHARED-001.5
  - AC-EXECUTORS-PLUGIN-SHARED-001.6
  - AC-EXECUTORS-PLUGIN-SHARED-001.7
  - AC-EXECUTORS-PLUGIN-SHARED-001.8
  - AC-EXECUTORS-PLUGIN-SHARED-001.9
  - AC-EXECUTORS-PLUGIN-SHARED-001.10
  - AC-EXECUTORS-PLUGIN-SHARED-001.11
  - AC-EXECUTORS-PLUGIN-SHARED-001.12
  - AC-EXECUTORS-PLUGIN-SHARED-001.13
  - AC-EXECUTORS-PLUGIN-SHARED-001.14
  - AC-EXECUTORS-PLUGIN-SHARED-001.15
  - AC-EXECUTORS-PLUGIN-SHARED-001.16
system_design:
  - ../../specs/executors/system-design/remote-executor-plugin-shared-admission-placement.md
---

# Task 09: Workspace reuse and shared environments (M1)

## Summary

Add owner sets, attach and release with share epochs, last-owner destroy, per-owner instances, and the provider fence, behind `workspace_sharing`.

## Size

L: ~500 source, ~900 test lines.

## In scope

- Holder row carries the owner set; every other owner has its own row with its own instance port; attach is one transaction across the holder and the owner row, and a non-last release is write one, the instance stop, then write two (`owner_phase` separate from the environment `phase`; settlement deletes every row after a confirmed destroy); releases come only from archive, delete, reset, and failed launch, never from Stop; a release loser retries up to three times.
- Owner set and `share_epoch` in the envelope; `attachSharedOwner` and `releaseOwner` as single revision-checked writes.
- Replace the unconditional rejection in `validateLaunch` with the capability-gated path; keep the rejection (with `missing_capability`) otherwise.
- `sharing_supported` and `max_owners` in profile validation; `owner_count` in destroy.
- `checkSharedEnvironmentDelete` in the task delete transaction (`shared_environment_in_use`); `owner_releasing` refusal; recovery of `attaching` owners by matching control-client instances on `SessionID`; skipped `closing` candidates (AC 001.5, 001.6, 001.14 to 001.16).

## Out of scope

- Isolation between owners of one environment; cross-principal sharing (refused).

## Acceptance

- Without the capability both reuse and shared workspace stay rejected.
- Two concurrent releases produce one destroy; attach racing the last release resolves to exactly one outcome.

## Failing-first tests

- Stop on a shared owner does not release it. Releasing an owner not in the set is a no-op. The 17th attach is `capacity_refused`.
- Two owners release concurrently (sqlite and PostgreSQL multi-connection `-race`; fixture over the process boundary).
- Attach races the last release: attach wins or gets `environment_closing` and tries the next candidate, then provisions anew; a reuse-required request fails with `environment_closing`.
- Crash after write one of a non-last release: owner out of the set and `releasing`, sweep repeats the stop and write two, reservation released once; failed stop keeps the reservation and the holder `phase` unchanged; a relaunch of that session is `owner_releasing` with no insert.
- Last release when the holder's owner left earlier: `closing` on the re-keyed holder row, one settlement deletes every row and releases each reservation once.
- Delete of the holder's task with a foreign owner attached is `shared_environment_in_use` (concurrent attach and delete exclude each other, sqlite and PostgreSQL race); archive of the holder's task is allowed and the claim is taken against it.
- Recovery of an `attaching` owner whose instance exists stops it by `SessionID` match before releasing.
- Stale `share_epoch` rejected by the fixture; destroy with owners remaining refused by the host.
- Attach from another principal, workspace, profile, identity, or repository set refused with a named reason; cap exceeded is `capacity_refused`.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -race -trimpath ./internal/plugins/... ./cmd/plugin-fixture -count=1)
```

```bash
(cd apps/backend && go test -race -trimpath ./internal/agent/runtime/lifecycle -count=1)
```

```bash
(cd apps/backend && go test -race -trimpath ./internal/task/repository/sqlite -run Executor -count=1)
```

## Likely files and risks

- `apps/backend/internal/agent/runtime/lifecycle/executor_plugin.go`
- `apps/backend/internal/task/repository/sqlite/executor_plugin_inventory.go`
- Risk: concurrency on both dialects; add the PostgreSQL race run (`KANDEV_TEST_POSTGRES_DSN`) before marking done.
