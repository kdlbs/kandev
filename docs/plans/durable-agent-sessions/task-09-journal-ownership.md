---
id: "09-journal-ownership"
title: "Bind journals to retained native storage"
status: complete
wave: 9
depends_on: ["08-agentctl-journal"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-002
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-002.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-002.2
system_design:
  - ../../specs/platform/system-design/durable-agent-delivery.md
---

# Task 09: Bind journals to retained native storage

## Summary

Add the shared journal-location contract and native-host storage ownership.

## In scope

- Define the retained-storage capability and stable owner-scoped journal root.
- Wire native-host agentctl replacement to its retained Kandev data directory.
- Register the typed storage-maintenance owner, quota visibility, safe cleanup, and retired-identity retention.
- Keep providers without explicit support non-durable until their dedicated work order passes.

## Out of scope

Docker, SSH, Kubernetes, and network replay.

## Acceptance

- Native agentctl replacement preserves the journal independently from its worktree and harness home.
- Cleanup preserves live or unacknowledged data. Explicit deletion reports lost delivery recovery.
- Unsupported providers cannot advertise v1.

## Verification

Run from the repository root. New test names describe required evidence, not existing passing tests.
Use TDD for implementation. Record the failing assertion before the implementation result.

```bash
(cd apps/backend && rtk go test -tags fts5 -race ./internal/agent/runtime/lifecycle ./internal/agentctl/journal ./internal/system/storage/... -count=1)
rtk make -C apps/backend lint
rtk python3 scripts/lint-spec-files.py --all
rtk git diff --check
```

Target evidence in `apps/backend/internal/agent/runtime/lifecycle/executor_journal_lifetime_test.go`:

- `TestNativeJournalSurvivesAgentctlReplacement`: `AC-PLATFORM-DURABLE-AGENT-DELIVERY-002.1`.
- `TestNativeJournalLossAndCleanupSafety`: `AC-PLATFORM-DURABLE-AGENT-DELIVERY-002.2`.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/executor_backend.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_standalone.go`
- `apps/backend/internal/agent/runtime/agentctl/launcher/`
- `apps/backend/internal/system/storage/`
- `apps/backend/internal/agentctl/journal/` (new in Task 08).
- `apps/backend/internal/agent/runtime/lifecycle/executor_journal_lifetime_test.go` (new tests or extensions).

## Dependencies

[Task 08](task-08-agentctl-journal.md).

## Risks

- The shared location contract must not derive identity from a task title, process ID, or worktree path.

## Parallelism

`sequential`

The primary session owns integration. This work order does not authorize subagents.
Preserve existing user edits and unrelated changes.

## Inputs

- [Owned system design](../../specs/platform/system-design/durable-agent-delivery.md).
- [Package manifest](plan.md), including shared regression gates and test prerequisites.
- Existing source and adjacent tests in the listed files.
- [Boundary decision](../../decisions/2026-09-10-durable-harness-session-boundaries.md).

## Results

Implemented retained journal location resolution and fail-closed startup/admission behavior. Native executor lifetime tests pass; retained remote/container live replacement checks remain environment-dependent.
