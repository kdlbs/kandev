---
id: "01-capability-negotiation-workspace"
title: "Capability negotiation and workspace descriptor (R1)"
status: pending
wave: 1
depends_on:
  - "00"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-NATIVE-WORKER-001
  - REQ-EXECUTORS-NATIVE-WORKER-002
acceptance_criteria:
  - AC-EXECUTORS-NATIVE-WORKER-001.1
  - AC-EXECUTORS-NATIVE-WORKER-001.2
  - AC-EXECUTORS-NATIVE-WORKER-001.3
  - AC-EXECUTORS-NATIVE-WORKER-001.4
  - AC-EXECUTORS-NATIVE-WORKER-001.5
  - AC-EXECUTORS-NATIVE-WORKER-001.6
  - AC-EXECUTORS-NATIVE-WORKER-001.7
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

# Task 01: Capability negotiation and workspace descriptor (R1)

## Summary

Add the manifest keys and host capability set, the capability snapshot in inventory, and the workspace path and identity on the descriptor with platform-aware validation and immutability.

## Size

M: ~450 source, ~700 test lines.

## In scope

- Add the manifest keys and host capability set, the capability snapshot in inventory, and the workspace path and identity on the descriptor with platform-aware validation and immutability.

## Out of scope

- Transport, repository intent, drain, checkpoints beyond the descriptor.

## Acceptance

- Manifest rejects unknown, duplicate, and unsatisfiable capabilities before activation; a non-declaring provider is byte-for-byte unchanged.
- The recorded workspace descriptor drives instance creation and is immutable per generation.

## Failing-first tests

- Manifest validator table test for each capability rule (fails: field absent).
- Workspace path table per platform: relative, root, UNC, NUL, dot segments, over-length, platform mismatch.
- Attach returns a different path or identity: `conflict`, no inventory change, session not started.
- Undeclared field in a provider response: `conflict` and the created resource is cleaned up.
- Fixture conformance test passes unmodified.
- No filesystem call on the recorded path (a test path that exists only on the backend host is never touched).

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -race -trimpath ./internal/plugins/... ./pkg/pluginsdk ./cmd/plugin-fixture -count=1)
```

```bash
go test -race -trimpath ./internal/agent/runtime/lifecycle -count=1
```

## Likely files and risks

- `apps/backend/proto/kandev/plugin/v1/plugin.proto`
- `apps/backend/internal/plugins/manifest/executor_provider.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_plugin.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_plugin_validation.go`
- Risk: concurrency on both dialects; add the PostgreSQL race run before marking done.
