---
id: "11-end-to-end-session-gaps"
title: "Close the gaps that stop a plugin session end to end"
status: in_progress
wave: 11
depends_on:
  - "04-provision-bootstrap"
  - "05-recovery-cleanup"
  - "07-profile-ui"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-PLUGIN-002
  - REQ-EXECUTORS-PLUGIN-004
  - REQ-EXECUTORS-PLUGIN-007
acceptance_criteria:
  - AC-EXECUTORS-PLUGIN-002.1
  - AC-EXECUTORS-PLUGIN-002.2
  - AC-EXECUTORS-PLUGIN-002.4
  - AC-EXECUTORS-PLUGIN-004.2
  - AC-EXECUTORS-PLUGIN-007.1
system_design:
  - ../../specs/executors/system-design/remote-executor-plugins.md
---

# Task 11: Close the gaps that stop a plugin session end to end

## Summary

A provider plugin session driven from the UI stops at five points (issue #4098): the launch has no
public Kandev API URL, the task dialog offers no agents because plugin profiles cannot carry agent
credentials, credential files are never copied, the repository is never cloned, and a rolled-back
launch keeps its environment.

## Scope

- Inject `<githubCredentialBroker.publicBaseUrl>/api/v1` as the plugin launch API URL.
- Accept, validate, and hide from providers the Kandev-owned credential keys on plugin profiles;
  render the remote credentials card on the plugin profile page.
- Upload selected credential files and bundles through agentctl processes after instance readiness.
- Materialize the primary repository for `plugin_remote` through the existing agentctl API.
- Destroy the environment when a launch is rolled back, before its inventory is released.

## Validation

- `go test ./internal/agent/runtime/... ./internal/task/service/ ./internal/backendapp/`
- `pnpm run typecheck`; plugin credential, plugin profile, and executor compatibility vitest suites.
- A live provider session launched from the UI cloned its repository, used a copied CLI login, and
  answered; forced launch failures destroyed their environments.

## Results

Implemented as specified. One of three forced launch failures still left its environment running in
the live run; issue #4098 tracks the unexplained case.
