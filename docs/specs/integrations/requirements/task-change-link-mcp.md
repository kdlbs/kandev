---
status: active
system: integrations
created: 2026-09-14
owners:
  - kandev
---

# Manage Task Pull Requests and Merge Requests Requirements

## Overview

Agents coordinating task delivery need to attach, swap, or remove the external
change request (a GitHub pull request or a GitLab merge request) that carries
the task's delivery after a task already exists. The MCP surface exposes three
provider-neutral operations so an agent can state intent without learning
provider-specific store mechanics, and every mutation answers with the
resulting canonical linked set so the caller can verify immediately.

This requirement is owned by the integration system because the contract
spans both provider stacks and one shared task-facing tool surface, while the
task system owns the task identity and the workspace system owns repository
attachment.

## Terminology

- **Change request (CR):** A GitHub pull request or a GitLab merge request.
- **Canonical repository identity:** The persisted repository identity
  (`repository_id`, resolved through the task's workspace) that distinguishes
  a fork from the canonical repository even when both contain the same
  change-request number.
- **Active association:** The stored link between the task and one
  provider-specific change request; historical receipts (terminal transcripts,
  conversation history) are unaffected by unlinking.

## Requirements

### REQ-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001: Manage task change requests through MCP

**Intent:** Give agents safe, verifiable control over the external delivery
record of an existing task.

**User story:** As an agent coordinating a task, I want to link, unlink, or
replace a GitHub pull request or GitLab merge request on that task through
MCP, so that the task's delivery trail points at the real change request
without me editing task records by hand.

#### Acceptance criteria

- **AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001.1:** When a mutation request is
  authorized for the calling task's workspace and supplies the task ID,
  provider (`github` or `gitlab`), canonical repository identity, and the
  change-request number, the system applies the link, unlink, or replacement
  and returns the resulting canonical linked set in the same response.
  A bare number without repository identity is rejected, and same-number
  change requests in a fork and a canonical repository remain distinct
  associations.
- **AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001.2:** When a caller requests an
  operation for a task outside its reachable workspace set, the system
  rejects the request without mutating any store.
- **AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001.3:** When a link targets an
  already-active association, the system succeeds without duplicating it;
  when an unlink targets an absent association, the system succeeds.
  Unlink removes only automation and settings scoped to that exact
  provider, repository, and number, and leaves other linked change requests
  and task metadata untouched.
- **AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001.4:** When a replacement's new
  target cannot be established, the previous association remains active; when
  the established new association also cannot be removed during rollback, the
  response reports both failures so the caller can reconcile the partial
  state.
- **AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001.5:** When a repository cannot be
  resolved to a verified provider host (including a persisted identity with
  no provider host), link operations fail closed instead of assuming a
  default provider host.
- **AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001.6:** When a mutation succeeds,
  query surfaces and connected clients observe the new linked state after a
  restart, and unlinking changes only the active association, never
  conversation or terminal-receipt history.
