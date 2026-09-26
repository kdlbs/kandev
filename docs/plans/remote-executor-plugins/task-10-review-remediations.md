---
id: "10-review-remediations"
title: "Close review findings for authentication, retention and recovery"
status: done
wave: 10
depends_on:
  - "02-authenticated-transport"
  - "03-profile-admission"
  - "04-provision-bootstrap"
  - "05-recovery-cleanup"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-PLUGIN-001
  - REQ-EXECUTORS-PLUGIN-003
  - REQ-EXECUTORS-PLUGIN-004
acceptance_criteria:
  - AC-EXECUTORS-PLUGIN-001.2
  - AC-EXECUTORS-PLUGIN-003.1
  - AC-EXECUTORS-PLUGIN-004.1
  - AC-EXECUTORS-PLUGIN-004.3
  - AC-EXECUTORS-PLUGIN-004.4
system_design:
  - ../../specs/executors/system-design/remote-executor-plugins.md
---

# Task 10: Close review findings for authentication, retention and recovery

## Summary

Fix five implementation review findings in the remote executor plugin lifecycle and preserve the original
plugin provider architecture and durable ownership rules.

## Scope

- Use one concurrency-safe bootstrap token source for HTTP, WebSocket and `Client.AuthToken()`.
- Retain replaced provider credentials while inventory can reference them; block clear or profile deletion
  while a retained environment depends on the profile.
- Reopen stopped plugin sessions by attaching the exact recorded resource and retaining its runtime
  identity, auth and conversation state. Never provision implicitly on reuse.
- Recover every durable nonterminal bootstrap phase. Attach with persisted auth; otherwise perform
  idempotent cleanup and retain unknown cleanup outcomes.
- Compare inventory checkpoints with the caller-observed envelope revision, enforce phase transitions,
  and merge only the plugin envelope into the latest metadata.
- Preserve documented legacy `local_pc` routing while unknown executor types fail closed.

## Acceptance

- A real bootstrap handshake is followed by an authenticated WebSocket upgrade.
- Rotation keeps old inventory references recoverable; clear/delete is guarded until retained inventory is
  settled and historical credentials are cleaned only after profile deletion.
- Manager stop/reopen attaches the same resource and does not provision or destroy it; conversation state
  and authenticated runtime identity remain intact.
- Restart recovery covers `artifact_staging`, `bootstrapping`, `provisioned`, and `ready` before and after
  handshake-token persistence. Unconfirmed cleanup remains retryable.
- A stale inspection checkpoint cannot overwrite cleanup's `absent` state or unrelated metadata.

## Verification

Run from the repository root unless a command says otherwise. Set `TMPDIR` and `GOTMPDIR` to a writable
workspace-backed directory when `/tmp` is full.

```bash
(cd apps/backend && go test ./internal/agent/runtime/agentctl ./internal/gateway/websocket ./internal/agent/executor ./internal/task/models ./internal/task/service ./internal/agent/runtime/lifecycle ./internal/task/repository/sqlite ./internal/plugins/... -count=1)
(cd apps/backend && go test -race ./internal/agent/runtime/agentctl -run '^TestPluginExecutorEndpointBootstrapHandshake$' -count=1)
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle -run 'TestPluginExecutor|TestManagerReopensStoppedPluginExecutorByAttachingRetainedEnvironment' -count=1)
(cd apps/backend && go test -race ./internal/task/service -run '^TestPluginExecutorProfileSecrets$' -count=1)
(cd apps/backend && go test -race ./internal/task/repository/sqlite -run '^TestPluginExecutorInventoryCAS$' -count=1)
(cd apps/backend && go test -race ./internal/task/repository/sqlite -run '^TestPluginExecutorPostgres' -count=1)
make -C apps/backend build
```

Required regressions:

- `TestPluginExecutorEndpointBootstrapHandshake`
- `TestPluginExecutorProfileSecrets`
- `TestManagerReopensStoppedPluginExecutorByAttachingRetainedEnvironment`
- `TestPluginExecutorPreHandshakeRecoveryCleansKnownResources`
- `TestPluginExecutorPreHandshakeUnknownDestroyRetainsCleanupInventory`
- `TestPluginExecutorPostHandshakeRecoveryAttachesEveryCheckpointPhase`
- `TestPluginExecutorStaleInspectionCannotResurrectCleanedInventory`
- `TestPluginExecutorInventoryCAS`
- `TestRunOwnerLaunchCreatesWorkspaceWithoutTaskMarker` and executor mapping tests for `local_pc` and unknown types

PostgreSQL verification requires the disposable test database selected by `KANDEV_TEST_POSTGRES_DSN`.

## Results

Completed all five review remediations and preserved the legacy `local_pc` executor alias while unknown
executor values fail closed.

Validation passed:

- Full affected Go package suites passed after the lifecycle sanitizer assertion was aligned with the
  established `[path-redacted]` output: agentctl, gateway WebSockets, task service, task models, executor
  mapping, lifecycle, SQLite repository, and all plugin packages.
- Race tests for the authenticated post-bootstrap WebSocket handshake, retained-session manager reopen
  and plugin recovery, profile secret rotation/clear/delete, SQLite inventory CAS, gateway transports,
  fixture provider, and plugin packages.
- PostgreSQL inventory CAS and cleanup-claim tests passed under the race detector against the disposable
  local PostgreSQL 16 instance.
- Root `make build` passed, including the web production build and all backend/runtime binaries.
- Fresh PR capture runs passed: desktop `remote-executor-pr-capture.spec.ts` and phone
  `mobile-remote-executor-pr-capture.spec.ts` each passed under the managed E2E runner; all four
  synthetic-data screenshots were manifest-checked and compressed.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.test.py` (36 passed),
  `python3 scripts/lint-spec-files.py --all`, and `git diff --check` passed after the documentation edits.
- Changed-package Go lint (`bash scripts/lint-go-changed`) passed with zero findings after simplifying
  the remote executor lifecycle and inventory paths.

The lifecycle test previously asserted for an obsolete `***` sanitizer marker. Its expectation now matches
the existing `[path-redacted]` contract; runtime behavior is unchanged.

## Dependencies

[Task 02](task-02-authenticated-transport.md), [Task 03](task-03-profile-admission.md),
[Task 04](task-04-provision-bootstrap.md), and [Task 05](task-05-recovery-cleanup.md).

## Parallelism

`sequential`
