---
id: "03-restore-coordinator"
title: "Unify harness restore orchestration"
status: complete
wave: 3
depends_on: ["02-continuation-snapshot"]
plan: "plan.md"
requirements:
  - REQ-AGENTS-HARNESS-SESSION-CONTINUITY-003
acceptance_criteria:
  - AC-AGENTS-HARNESS-SESSION-CONTINUITY-003.1
  - AC-AGENTS-HARNESS-SESSION-CONTINUITY-003.2
system_design:
  - ../../specs/agents/system-design/harness-session-continuity.md
---

# Task 03: Unify harness restore orchestration

## Summary

Route startup, explicit continuation, and workspace rebind through one configuration-preserving restore coordinator.

## In scope

- Replace divergent decisions in createOrLoadSession and createReboundACPSession with the typed policy.
- Capture immutable runtime configuration before native events. Reuse model, mode, and sorted-option restoration.
- Persist candidate native IDs before promotion. Preserve the prior committed token on creation or configuration failure.
- Own agent_restore_attempts_total, agent_restore_context_truncated_total, and agent_restore_recovery_required_total through one shared metrics contract.
- Check the persistent recovery block under the existing queue/session admission boundary.
- Associate the continuation snapshot with the first authorized submission. Treat pre-journal crash ambiguity as blocked.

## Out of scope

Directory capability matrix and new recovery presentation.

## Acceptance

- All restore entry points preserve model, permission mode, configuration, and current trusted MCP/system context.
- Concurrent recovery and interrupted creation cannot silently replace native identity or dispatch a prompt twice.
- Archive, prevent-auto-start, and queue ownership guards still apply.

## Verification

Run from the repository root. New test names describe required evidence, not existing passing tests.
Use TDD for implementation. Record the failing assertion before the implementation result.

```bash
(cd apps/backend && rtk go test -tags fts5 -race ./internal/agent/runtime/lifecycle ./internal/orchestrator/executor -count=1)
rtk make -C apps/backend lint
rtk python3 scripts/lint-spec-files.py --all
rtk git diff --check
```

Target evidence in `apps/backend/internal/agent/runtime/lifecycle/session_restore_coordinator_test.go`:

- `TestRestoreCoordinatorPreservesRuntimeConfiguration`: `AC-AGENTS-HARNESS-SESSION-CONTINUITY-003.1`.
- `TestRestoreCoordinatorPartialFailureBlocksDispatch`: `AC-AGENTS-HARNESS-SESSION-CONTINUITY-003.2`.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/session.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_workspace_rebind.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go`
- `apps/backend/internal/orchestrator/executor/executor_resume.go`
- `AGENTS.md` (agent_restore_* observability documentation).
- `apps/backend/internal/agent/runtime/lifecycle/session_restore_coordinator_test.go` (new tests or extensions).

## Dependencies

[Task 02](task-02-continuation-snapshot.md).

## Risks

- Native initialization can overwrite cached configuration. The capture must precede lifecycle mutation.

## Parallelism

`sequential`

The primary session owns integration. This work order does not authorize subagents.
Preserve existing user edits and unrelated changes.

## Inputs

- [Owned system design](../../specs/agents/system-design/harness-session-continuity.md).
- [Package manifest](plan.md), including shared regression gates and test prerequisites.
- Existing source and adjacent tests in the listed files.
- [Boundary decision](../../decisions/2026-09-10-durable-harness-session-boundaries.md).

## Results

The typed `RestoreCoordinator` owns normal native `session/load`, explicit continuation, and
workspace-rebind decisions while preserving the captured runtime configuration and committed
native identity. Production context continuation now initializes its candidate synchronously,
commits the generation before durable submission and prompt admission, and rolls back the
candidate when the generation CAS fails. Open recovery blocks move with that generation commit
in the same SQL transaction. Lifecycle/orchestrator race tests, lint, specification lint, and
diff checks pass. Live harness relocation behavior remains version-dependent and is covered by
the documented environment-dependent acceptance gate.
