---
id: "09-update-public-mcp-documentation"
title: "Update public MCP documentation"
status: done
wave: 6
depends_on:
  - "07-add-scoped-mcp-selectors"
plan: "plan.md"
requirements:
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-001
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-002
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-003
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-004
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-005
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-007
acceptance_criteria:
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-001.1
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-001.2
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-001.7
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-001.8
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.3
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.4
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.7
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.8
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.9
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.10
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.11
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-003.3
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-003.7
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-004.3
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-004.7
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-005.2
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-005.6
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-005.7
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-007.4
system_design:
  - ../../specs/agents/system-design/workspace-mcp-configuration.md
---

# Task 09: Update Public MCP Documentation

## Summary

Replace the raw profile JSON instructions with the workspace catalog and scope
selection workflow. Explain additive composition, Registry trust, session
application states, secret bindings, and attachment evidence limits.

## In scope

- Update the agent profile guide to use workspace-contextual selection and
  remove raw JSON setup instructions.
- Update the MCP guide with catalog creation, curated and public marketplace
  setup, custom definitions, secret references, and trust warnings.
- Explain the difference between a workspace definition, a remote connection,
  lazy managed-package materialization, and an existing executable.
- Document repository, task, and task-agent additions plus the additive union
  and inherited origins.
- Document active-turn restrictions, idle reconnect, next-start deferral,
  retry, provider limitations, and what a `session/load` fallback resets.
- State where a bound secret ends up: Kandev stores a reference, but delivery
  places the value inside the task executor as agent configuration or process
  arguments readable there.
- Update task/workflow, security, and feature-status pages when their current
  statements become incomplete or incorrect.
- Keep statements consistent with configured, delivered, and observed MCP
  attachment evidence.

## Out of scope

- Registry aggregator operator internals.
- Claims that a curated or public entry is security reviewed.
- Screenshots unless an existing page requires them for parity.

## Acceptance

- No public guide instructs users to paste raw MCP JSON into an agent profile.
- A user can follow the docs from workspace setup through scoped selection.
  The user can understand pending, deferred, and failed idle-session changes.
- Security guidance clearly separates registry metadata, installed
  configuration, workspace secrets, delivery, and observed attachment state.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
rg -n 'raw JSON|Profile MCP|mcp-config' docs/public
```

The final `rg` is a review aid. Any remaining matches must describe migration
or an intentional compatibility boundary, not the normal setup workflow.

## Files likely touched

- `docs/public/agents-and-profiles.md`
- `docs/public/automation-and-mcp.md`
- `docs/public/tasks-and-workflows.md`
- `docs/public/security.md`
- `docs/public/feature-status.md`
- `docs/public/README.md`, only if page summaries or navigation change

## Dependencies

- Task 07 finalizes user-visible names, locations, and applied-state behavior.

## Risks

- Existing MCP observability docs distinguish delivery from connection. Keep
  that limitation intact while adding configuration origins.
- Avoid documenting the public Registry as stable or vetted while it remains a
  preview discovery service.

## Parallelism

`parallel-safe` with Task 08 after Task 07 because it touches only public
documentation and documentation validators.

## Inputs

- Requirement sections 001 through 005 and 007.
- System-design sections `Marketplace installation`, `Effective-set
  resolution`, `Idle-session reconfiguration`, `Security`, and `Observability`.
- ADR-2026-07-30 and ADR-2026-09-01.
- Current public agent, MCP, task, security, and feature-status guides.

## Results

- Replaced raw profile JSON setup guidance with workspace catalog, marketplace, custom-mode, and scope-selection workflows.
- Documented additive composition, inherited origins, lazy package materialization, secret delivery boundaries, Registry trust limits, idle reconnect states, and provider fallback behavior.
- Updated agent, automation, security, and feature-status pages and validated all published links and coverage metadata.
- Verification passed: public-doc tests, public-doc validation, and specification lint.
