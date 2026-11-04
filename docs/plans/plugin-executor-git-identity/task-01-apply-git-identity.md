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
same effective global Git configuration that the agent uses. The setup process
inherits the agent's `HOME` and Git configuration environment. It does not use
the credential-copy target home to select Git's configuration. Run the setup
through agentctl at fresh launch and retained reattach.

## In scope

- `executor_plugin_credentials.go`, `executor_plugin.go`,
  `executor_plugin_retained.go`, `executor_plugin_recovery.go`, and the
  plugin executor lifecycle tests.
- The requirement, system design and public docs updates.

## Out of scope

- A prepare-script mechanism for plugin executors.

## Acceptance

`go test -tags fts5 ./internal/agent/runtime/lifecycle` passes, and the Git
identity tests fail when the command stops setting either value. Lifecycle
tests cover fresh launch, retained attach, and a setup failure that does not
fail the launch.
