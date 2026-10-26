---
id: "01-keep-launch-profiles"
title: "Keep plugin task launch profiles"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-PLUGINS-001
acceptance_criteria:
  - AC-PLUGINS-PLUGINS-001.15
system_design:
  - ../../specs/plugins/system-design/plugins-02.md
---

# Task 01: Keep plugin task launch profiles

## Summary

Store a plugin task's launch agent and executor profiles in the task metadata,
and validate them whenever a plugin gives them, so a task started later by a
workflow step launches with them.

## In scope

- `backendapp/services.go` and `adapters_plugin_writer_test.go`.
- `plugins/host_write.go` and `host_write_launch_test.go`.
- The requirement and system design updates.

## Out of scope

- Keeping `launch.prompt` for a later start.

## Acceptance

`go test ./internal/plugins ./internal/backendapp` passes, and the metadata
tests fail when `pluginTaskMetadata` stops writing either profile.
