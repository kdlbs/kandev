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
system_design:
  - ../../specs/executors/system-design/remote-executor-plugin-shared-admission-placement.md
---

# Task 09: Workspace reuse and shared environments (M1)

## Summary

Add owner sets, attach and release with share epochs, last-owner destroy, per-owner instances, and the provider fence, behind `workspace_sharing`.

## Size

L: ~500 source, ~900 test lines.

## In scope

- Holder row carries the owner set; every other owner has its own row with its own instance port; attach and release are single transactions across the holder and the owner row; releases come only from archive, delete, reset, and failed launch, never from Stop; a release loser retries up to three times.
- Owner set and `share_epoch` in the envelope; `attachSharedOwner` and `releaseOwner` as single revision-checked writes.
- Replace the unconditional rejection in `validateLaunch` with the capability-gated path; keep the rejection (with `missing_capability`) otherwise.
- `sharing_supported` and `max_owners` in profile validation; `owner_count` in destroy.

## Out of scope

- Isolation between owners of one environment; cross-principal sharing (refused).

## Acceptance

- Without the capability both reuse and shared workspace stay rejected.
- Two concurrent releases produce one destroy; attach racing the last release resolves to exactly one outcome.

## Failing-first tests

- Stop on a shared owner does not release it. Releasing an owner not in the set is a no-op. The 17th attach is `capacity_refused`.
- Two owners release concurrently (sqlite and PostgreSQL multi-connection `-race`; fixture over the process boundary).
- Attach races the last release: attach wins or gets `environment_closing` and provisions anew.
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
