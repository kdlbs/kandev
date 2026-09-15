---
status: implemented
requirements:
  - REQ-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001
system_design:
  - ../../specs/integrations/system-design/task-change-link-mcp.md
created: 2026-09-14
---

# Implementation Plan: Manage Task Pull Requests and Merge Requests

## Overview

Expose one provider-neutral MCP link contract,
`link_task_pr_kandev` / `unlink_task_pr_kandev` / `replace_task_pr_kandev`,
that manages an existing task's GitHub pull request and GitLab merge request
associations. The backend routes all three operations through one coordinator
so validation, workspace authorization, and store dispatch stay identical for
both providers, and each mutation returns the resulting canonical linked set.

## Delivery packages

- [Task 01: Provider-neutral task change-request link
  coordinator](task-01-coordinator.md)
  - Backend coordinator, MCP handler and server registration, provider service
    entry points, and GitLab deletion-event publication. This package is the
    only work order: the merged diff is backend-only and public documentation
    was part of the same delivery.

## Requirements and design

- Requirements: `docs/specs/integrations/requirements/task-change-link-mcp.md`.
- System design: `docs/specs/integrations/system-design/task-change-link-mcp.md`.

## Risks

- Fork and canonical repositories can share a number; identity is anchored to
  the canonical repository identity, not the number.
- Legacy repositories may have an empty provider host; link operations fail
  closed instead of guessing `github.com`.
- Replacement failure must not strand two active associations; compensation is
  reported when it also fails.

## Verification strategy

Focused Go suites for the coordinator, MCP handler/server, GitHub, and GitLab
packages, plus race-enabled focused tests for identity, authorization,
idempotency, rollback, detach, and projection behavior. Public documentation
validation via `Validate public docs`.
