---
id: "01-preserve-external-plugin-task-priority"
title: "Preserve external-plugin task priority"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-PLUGINS-001
acceptance_criteria:
  - AC-PLUGINS-PLUGINS-001.7
system_design:
  - ../../specs/plugins/system-design/plugins-02.md
---

# Task 01: Preserve External-Plugin Task Priority

## Summary

Add provider-neutral regression coverage that proves an authorized external
plugin can persist `priority: high` through the Host task-write path. See
[plan.md](plan.md).

## In scope

- The plugin fixture package manifest and task-write probe.
- The fixture's private priority webhook.
- External-process Host tests with isolated SQLite immediate readback.

## Out of scope

- Production priority policy, task priority values, and user-interface changes.
- Provider-specific plugin behavior or a new public RPC.

## Acceptance

- An external plugin with declared task read/write capabilities creates a task
  with `priority: high`, and immediate task-service readback remains `high`.
- Updating that task through the same Host boundary persists `priority: high`.
- Omitted priority retains the task-service `medium` default, and invalid
  create or update priority is rejected.
- Host task calls without a declared read or write capability return
  `PermissionDenied`, as required by AC-PLUGINS-PLUGINS-001.7.

## Verification

Run from `apps/backend`:

```bash
go test -race -count=1 -run 'TestPluginHost_ExternalProcessPersistsPriorityThroughTaskService|TestPluginPriorityWirePayload' -v ./internal/plugins
go test -count=1 -v ./cmd/plugin-fixture
go vet ./cmd/plugin-fixture ./internal/plugins
go build ./cmd/plugin-fixture
```

## Files covered

- `apps/backend/cmd/plugin-fixture/fixture-package/manifest.yaml`
- `apps/backend/cmd/plugin-fixture/fixture_package_test.go`
- `apps/backend/cmd/plugin-fixture/plugin.go`
- `apps/backend/internal/plugins/host_write_external_process_test.go`

## Dependencies

The existing Host `api_write:tasks` implementation and task service remain the
implementation boundary.

## Results

The fixture declares the necessary task capabilities and exposes a private test
webhook. The external-process regression exercises the real go-plugin broker,
plugin Host, task service, and isolated SQLite readback. It proves create and
update retain `high`, omitted create defaults to `medium`, and invalid values
are rejected. Existing Host capability tests cover AC-PLUGINS-PLUGINS-001.7;
the priority persistence outcomes are this work order's local acceptance.
