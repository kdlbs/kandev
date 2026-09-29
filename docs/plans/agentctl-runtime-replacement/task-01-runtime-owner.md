---
id: "01-runtime-owner"
title: "Introduce a generation-fenced runtime owner"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-AGENT-RUNTIME-AVAILABILITY-001
  - REQ-PLATFORM-AGENT-RUNTIME-AVAILABILITY-002
acceptance_criteria:
  - AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-001.2
  - AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-001.5
  - AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-002.1
  - AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-002.4
system_design:
  - ../../specs/platform/system-design/agent-runtime-availability.md
---

# Task 01: Introduce a generation-fenced runtime owner

## Summary

Introduce a generation-fenced runtime owner. Preserve original session authority and all prior durable-delivery fixes.

## In scope

Add the immutable runtime binding, operation lease, and snapshot revision model in internal/agent/runtime/agentctl.
Keep startup behavior working with one published binding. Do not activate replacement yet.
Make lease acquisition, retirement, and publication atomic; never hold locks through external work.
Keep credentials private. Couple cancellation with validation before applying results, including delayed HTTP success and old exit callbacks.
Define prepare/commit/abort binding integration, with rollback of candidate resources but no resurrection of retired bindings.

## Out of scope

Other work orders, unproven Git repairs, automatic external mutation retries, and unrelated executor changes.

## Acceptance

- The scoped outcome passes every named regression, including stale-owner and failure cases.
- No prompt, tool, or Git mutation is replayed by runtime replacement.
- Partial failure leaves accurate availability and admission fences; cleanup affects only owned resources.

## Tests

TestRuntimeBindingRejectsRetiredResults; TestRuntimeBindingPublishAtomic; TestRuntimeSnapshotRevisionOrdering.
Use barriers to pause a request, retire its epoch, publish another, and release the old result.
Test abort, duplicate publication, shutdown-versus-publication, and an accepted mutation whose response is lost.
Proposed tests belong beside their production owners. Verify the command selects them before recording success.

## Verification

Run each command from the repository root after implementation.

```bash
(cd apps/backend && go test -race ./internal/agent/runtime/agentctl ./internal/backendapp -count=1)
make -C apps/backend lint
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/agent/runtime/agentctl/availability.go`
- `apps/backend/internal/agent/runtime/agentctl/availability_test.go`
- `apps/backend/internal/backendapp/agentctl.go`

New runtime-owner/coordinator and regression files are expected beside these owners.

## Dependencies

None.

## Risks

Delayed callbacks and partial binding can affect a successor. Prove generation ownership before every state-changing result.

## Parallelism

`sequential`

## Inputs

- [Manifest](plan.md).
- [Requirements](../../specs/platform/requirements/agent-runtime-availability.md).
- [Design](../../specs/platform/system-design/agent-runtime-availability.md).
- [Decision](../../decisions/2026-09-27-agentctl-runtime-replacement.md).

## Results

Completed on 2026-09-27. Added a generation-fenced runtime owner with private binding credentials, cancellable operation leases, atomic prepare/commit/abort, stale exit/result rejection, and ordered revisioned availability snapshots. Backend startup publishes the authenticated fresh or adopted binding through the owner; system info and runtime snapshots share one boot ID. Intentional shutdown invalidates leases and owns binding cleanup without publishing an outage.

Validation passed:

- `(cd apps/backend && go test ./internal/agent/runtime/agentctl ./internal/backendapp ./internal/system/info -count=1)`
- `(cd apps/backend && go test -race ./internal/agent/runtime/agentctl ./internal/backendapp -count=1)`
- `(cd apps/backend && go test -race ./internal/agent/runtime/agentctl -run 'Availability|RuntimeBinding|RuntimeSnapshot|RuntimeOwner' -count=1)`
- `make -C apps/backend lint`
- `python3 scripts/list-docs.py validate`
- `python3 scripts/lint-spec-files.py --all`
- `git diff --check`

No runtime replacement coordinator or consumer migration is enabled yet; those remain in later work orders.
