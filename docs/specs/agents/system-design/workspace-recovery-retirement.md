---
status: current
system: agents
created: 2026-11-10
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-005
owners:
  - Kandev
---

# Workspace recovery during runtime retirement

## Context and requirement mapping

This extension of [session recovery](session-recovery-failures.md) implements
AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-005.1 and .4 at the cached execution
boundary. The existing design retains admission, terminal-state, provider
identity, cleanup, and UI ownership. No API or persistence schema changes.

## Cached workspace access

`GetOrEnsureExecution` and `EnsureWorkspaceExecutionForSession` join the existing
session operation gate before inspecting cached state. Inside that operation,
lookup acquires the cached execution's `remoteInstanceLifecycleMu` and rechecks
that the same execution is still registered. Runtime retirement owns that lock
while stopping the controller and removing its entry.

If retirement removed the execution, lookup continues through normal
workspace-only admission and creation. Otherwise, recheck cached admission
before returning it. Neither cache fast path may bypass the session operation
or the runtime owner. Concurrent workspace requests retain existing
singleflight deduplication. Do not extend readiness timeouts or retry prompts.

## Verification and delivery

Lifecycle regressions hold the session cleanup operation or runtime retirement
lock while a cached execution remains registered. Lookup must wait and then
inspect current registration instead of returning that execution.
The existing mobile and desktop session settings recovery scenarios exercise
failed startup, retained workspace restoration, and provider continuity.

[Implementation and results](../../../plans/workspace-recovery-retirement/task-01-wait-for-runtime-retirement.md).
