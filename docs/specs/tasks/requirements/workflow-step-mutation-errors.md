---
status: active
system: tasks
created: 2026-11-13
owners:
  - kandev
---

# Workflow step mutation error requirements

## Overview

Workflow authors need error responses that identify the rejected operation correctly.
A dependency failure must not tell them that an existing step disappeared.
The task system owns this contract because it owns workflow definitions and their mutation APIs.

## Requirements

### REQ-TASKS-WORKFLOW-STEP-MUTATION-ERRORS-001: Accurate mutation error responses

**Intent:** Distinguish missing workflow resources, policy rejections, and unexpected dependency failures without disclosing private data.

#### Acceptance criteria

- **AC-TASKS-WORKFLOW-STEP-MUTATION-ERRORS-001.1:** A missing step shall produce HTTP 404 with `{"error":"Step not found"}`.
  A concealed workflow-resource access rejection shall retain that same response on step mutation endpoints.
  Neither response shall disclose resource ownership or existence.
- **AC-TASKS-WORKFLOW-STEP-MUTATION-ERRORS-001.2:** An unexpected dependency or storage failure shall produce HTTP 500 with `{"error":"Failed to save step"}`.
  The words `not found` in an error message shall not establish that a step is missing.
  This rule shall also apply to nested errors and differently capitalized messages.
- **AC-TASKS-WORKFLOW-STEP-MUTATION-ERRORS-001.3:** A recognized missing template, workflow, or workspace shall retain HTTP 404 with a resource-specific message.
  The messages shall be `Template not found`, `Workflow not found`, and `Workspace not found`, respectively.
  Concealed access rejections shall retain criterion `.1` instead.
- **AC-TASKS-WORKFLOW-STEP-MUTATION-ERRORS-001.4:** Existing policy and validation rejections shall retain their status and safe response fields.
  These responses include read-only conflicts (409), executor mutation denial (403), workspace default executor unavailability (409), and recognized validation errors (400).
  Workspace default executor unavailability shall retain its `workspace_default_executor_unavailable` error code.
- **AC-TASKS-WORKFLOW-STEP-MUTATION-ERRORS-001.5:** Additional error context shall preserve a recognized error classification.
  Unexpected errors shall retain a generic response, while backend diagnostic logs shall retain their cause.
  Responses shall not expose raw dependency errors, private resource identifiers, credentials, or database details.
- **AC-TASKS-WORKFLOW-STEP-MUTATION-ERRORS-001.6:** These classifications shall apply to step creation, update, deletion, reordering, and template application over HTTP.
  A dependency rejection before persistence shall leave stored steps unchanged and publish no successful step mutation event.

## Compatibility

Unexpected dependency errors that previously matched `not found` change from a misleading 404 to the existing generic 500 response.
Missing templates retain 404, but their message changes from `Step not found` to `Template not found`.
Recognized workflow and workspace misses also receive messages that name the correct resource.

## Related contracts

- [Authentication](../../auth/requirements/auth.md), especially `AC-AUTH-AUTH-001.7`, owns workspace privacy and concealed cross-user access.
- [Executor ownership](../../executors/requirements/per-user-ownership.md) owns executor admission and retained binding eligibility.
- [Workflow step ordering](workflow-step-ordering.md) owns reorder membership and transaction safety.

## Out of scope

- Changes to authorization decisions, retained executor eligibility, or resource deletion policy.
- A general error envelope, new error codes, or raw error passthrough.
- Changes to WebSocket/MCP classification, browser layout, localization catalogs, or workflow persistence semantics.

## Implementation plans

- [Workflow step mutation errors](../../../plans/workflow-step-mutation-errors/plan.md)
