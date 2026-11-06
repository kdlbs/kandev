---
created: 2026-10-12
status: implemented
requirements:
  - REQ-PLUGINS-PLUGINS-001
system_design:
  - ../../specs/plugins/system-design/plugins-02.md
legacy_specs: []
---

# Implementation Plan: Plugin Host Task Priority Persistence

## Overview

Preserve an explicitly supplied task priority when an external plugin creates or
updates a task through the capability-gated Host API. The regression package
exercises the real plugin broker, Host writer, task service, and isolated SQLite
readback so the public contract is verified across its transport boundary.

## Scope

In scope:

- The fixture manifest's task-read and task-write capability declarations.
- The fixture's priority webhook and probe behavior used only by the integration
  contract test.
- External-process assertions for create and update with `priority: high`,
  omitted-priority defaulting to `medium`, and invalid-priority rejection.

Out of scope:

- New task priority values, UI priority controls, task ordering, or provider-specific
  behavior.
- Changes to the public plugin protocol or task-service defaulting rules.

## Technical approach

Keep the existing Host writer and task service as the behavior boundary. Extend
the fixture plugin only with a private test webhook that invokes the SDK task
write API, then use the real external plugin process to issue requests and read
the stored task back through the task service. The test proves `high` reaches
persistence for both create and update without weakening omitted or invalid input
behavior.

## Tests

| Acceptance criterion | Evidence |
| --- | --- |
| AC-PLUGINS-PLUGINS-001.7 (capability access) | The fixture declares `api_read: tasks` and `api_write: tasks`; existing Host tests verify undeclared access returns `PermissionDenied`. |
| Task 01 priority persistence | The external-process test proves authorized Host task writes preserve `high`, omitted priority defaults to `medium`, and rejected values leave stored tasks unchanged. |

## Work orders

- [x] [Task 01: Preserve external-plugin task priority](task-01-preserve-external-plugin-task-priority.md)

## Verification results

At successor commit `104434ef7aaf533a2a9320908d43af16cbb9229c`:

- `go test -race -count=1 -run 'TestPluginHost_ExternalProcessPersistsPriorityThroughTaskService|TestPluginPriorityWirePayload' -v ./internal/plugins` passed.
- `go test -count=1 -v ./cmd/plugin-fixture` passed.
- `go vet ./cmd/plugin-fixture ./internal/plugins` passed.
- `go build ./cmd/plugin-fixture` passed.
