---
id: "02-workspace-descriptor"
title: "Workspace descriptor (R1)"
status: pending
wave: 2
depends_on:
  - "01"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-NATIVE-WORKER-002
acceptance_criteria:
  - AC-EXECUTORS-NATIVE-WORKER-002.1
  - AC-EXECUTORS-NATIVE-WORKER-002.2
  - AC-EXECUTORS-NATIVE-WORKER-002.3
  - AC-EXECUTORS-NATIVE-WORKER-002.4
  - AC-EXECUTORS-NATIVE-WORKER-002.5
  - AC-EXECUTORS-NATIVE-WORKER-002.6
  - AC-EXECUTORS-NATIVE-WORKER-002.7
system_design:
  - ../../specs/executors/system-design/remote-executor-plugin-native-workers.md
---

# Task 02: Workspace descriptor (R1)

## Summary

Add the workspace path and identity on the descriptor with platform-aware validation and immutability, gated by the `workspace_descriptor` capability from task 01.

## Size

M: ~450 source, ~700 test lines.

## In scope

- Add the workspace path and identity on the descriptor with platform-aware validation and immutability, gated by the `workspace_descriptor` capability from task 01.

## Out of scope

- The registry and negotiation (task 01), transport, repository intent, drain, checkpoints.

## Acceptance

- A provider not declaring `workspace_descriptor` is byte-for-byte unchanged.
- The recorded workspace descriptor drives instance creation and is immutable per generation.

## Failing-first tests

- Two environments of one installation record one normalized path (POSIX trailing and repeated separators; Windows case and separator forms): the second commit gets `conflict` and its resource is cleaned (sqlite and PostgreSQL race).
- Workspace path table per platform: relative, root, UNC, NUL, dot segments, over-length, platform mismatch.
- Attach returns a different path or identity: `conflict`, no inventory change, session not started.
- Descriptor field in a response from a provider that did not declare the capability: `conflict` and cleanup.
- Fixture conformance test passes unmodified.
- No filesystem call on the recorded path (a test path that exists only on the backend host is never touched).

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -race -trimpath ./internal/plugins/... ./pkg/pluginsdk ./cmd/plugin-fixture -count=1)
```

```bash
(cd apps/backend && go test -race -trimpath ./internal/agent/runtime/lifecycle -count=1)
```

## Likely files and risks

- `apps/backend/proto/kandev/plugin/v1/plugin.proto`
- `apps/backend/internal/plugins/manifest/executor_provider.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_plugin.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_plugin_validation.go`
- Risk: concurrency on both dialects; add the PostgreSQL race run before marking done.
