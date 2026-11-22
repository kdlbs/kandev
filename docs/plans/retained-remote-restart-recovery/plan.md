---
created: 2026-11-21
status: complete
requirements:
  - REQ-EXECUTORS-REMOTE-DOCKER-001
  - REQ-EXECUTORS-PLUGIN-004
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-001
system_design:
  - ../../specs/executors/system-design/remote-docker-executor.md
  - ../../specs/executors/system-design/remote-executor-plugins.md
  - ../../specs/platform/system-design/durable-restart-reconciliation.md
  - ../../specs/platform/system-design/durable-agent-reattachment.md
legacy_specs: []
---

# Fix plan for retained remote executor recovery

## What fails

A remote workspace can keep its files while the agent inside it stops. After a
backend restart, Kandev guards the session until it knows what happened to the
previous agent. `proveRecoveredRemoteInstance` accepts a running agent but
rejects an authenticated `stopped` response. The retry holds the same guard,
so it blocks the normal Resume that would start the agent in the same workspace.
The plugin report describes this loop after its idle environment suspends and
the provider later comes online. The same live-agent check rejects stopped
Remote Docker agents.
[Issue #5415](https://github.com/kdlbs/kandev/issues/5415)

Remote Docker has another failure path. Startup can force-stop a failed
session that lacks a usable resume token. That operation can remove the
container and its unpushed work. Startup also invokes force-stop for completed,
cancelled, or missing sessions. For plugin executors, the startup stop does
not destroy the workspace. The generic prune can discard the record needed
to reconnect to it.
[Issue #5414](https://github.com/kdlbs/kandev/issues/5414)

## Fix

1. Check remote workspace ownership before startup cleanup can stop or prune
   it. The resume token describes the conversation. Reset, archive, and delete
   own removal of an established workspace.
2. Handle an authenticated stopped agent in the existing recovery path. Replay
   retained output, then mark the recorded execution stopped and return an
   active session to waiting. If there is no recorded outcome, close the old
   turn as interrupted and keep the uncertainty for normal Resume.

The existing periodic loop retries when the plugin becomes available.
Resume starts the agent in the same workspace and restores the native
conversation when a usable token exists. Settlement leaves queued work in place.

| Agent and delivery evidence | Behavior |
| --- | --- |
| The exact agent is running | Adopt it and replay by the existing rules. |
| The exact agent stopped and the journal matches | Project retained output, settle the stopped execution, and let Resume apply its normal checks. |
| Ownership or journal evidence conflicts or is incomplete | Keep the applicable guard and retry or show the existing recovery reason. |

The reported ACP restore error and missing SSH target remain unverified.
Both need the reporter's runtime and SSH evidence and are outside this package.

## Test evidence

Temporary diagnostics ran against `d7d38f8e6f32dd4ceadb7c294416ba9f3ece70af`.

- `TestDiagnosticRemoteRestartStoppedAgent` exercised the real proof with an authenticated HTTP fixture.
  Both `plugin_remote` and `remote_docker` failed at the live association check.
  The four selected controls for live attachment, retained plugin reuse, and confirmed absence passed.
- `TestDiagnosticRemoteRestartPreservesWorkspace` called startup reconciliation with SQLite and a recording agent manager.
  Startup requested `Force: true` for a failed Remote Docker session with a matching ready environment.
  All 16 existing `TestReconcileSessionsOnStartup` subtests passed.

The work orders name the permanent regressions, their acceptance IDs, and their exact red and green commands.

## End-to-end verification

- Use the Playwright `containers` project for the failed-session startup case
  and the stopped-agent case. Check the original container and unpushed
  commit after restart and again after Resume.
- Use the backend plugin adapter fixture in `TestPluginExecutorRestartRecovery`
  for the unavailable-provider retry. Keep
  `TestManagerReopensStoppedPluginExecutorByAttachingRetainedEnvironment`
  as the control for reusing the plugin resource.

## Work orders

- [x] [01 Preserve retained remote workspaces](task-01-preserve-remote-docker-workspace.md)
- [x] [02 Settle the guard for an exact stopped agent](task-02-settle-stopped-agent.md)

## Verification results

The specification catalog, specification lint, public documentation tests and
validation, changed-file Go lint, SQL guard, persistence conformance suite,
backend build targets, and `git diff --check` passed. The targeted
race-enabled lifecycle, orchestrator, and SQLite regressions passed. The
Remote Docker container E2E selection passed all four cases: a tokenless failed
session retaining its workspace, a tokenless failed session releasing its
guard after the exact recorded container exits, stopped-agent tail replay and
Resume, and the same-session Resume control. The backend and E2E fixtures were
rebuilt before the final container run.

The PR fixup head 4920de0e591ad17d87d13f8ea279a6a1f729229c was validated on
its exact merge with main. Its focused merged-tree regressions and PostgreSQL
owner-change test passed. The E2E shard that failed once also failed on main;
the retries-disabled synthetic-merge replay and the single CI rerun both
passed. At fixup head 4920de0, all checks were terminal with none failed or
pending, and no review threads remained unresolved.

## Risks

- A plugin can save a newer checkpoint during attachment. Check that the row
  still describes the authenticated resource and use its freshly read revision.
- Stop, cleanup, or a successor can win a race with recovery. Use the same owner
  checks as confirmed process absence.
