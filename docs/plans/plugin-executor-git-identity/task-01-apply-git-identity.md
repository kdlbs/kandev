---
id: "01-apply-git-identity"
title: "Apply the profile Git identity on plugin executors"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-PLUGIN-002
acceptance_criteria:
  - AC-EXECUTORS-PLUGIN-002.8
system_design:
  - ../../specs/executors/system-design/remote-executor-plugins.md
---

# Task 01: Apply the profile Git identity on plugin executors

## Summary

After the agent credential upload, set the profile's Git name and email in the
environment's global Git configuration through an agentctl process, at launch
and at retained reattach.

## In scope

- `executor_plugin_credentials.go`, `executor_plugin.go`,
  `executor_plugin_retained.go`, and `executor_plugin_git_identity_test.go`.
- The requirement, system design and public docs updates.

## Out of scope

- A prepare-script mechanism for plugin executors.

## Acceptance

`go test -tags fts5 ./internal/agent/runtime/lifecycle` passes, and the Git
identity tests fail when the command stops setting either value.
