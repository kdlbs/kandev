---
created: 2026-09-07
status: done
requirements:
  - REQ-TASKS-CROSS-WORKSPACE-TRANSFER-001
  - REQ-TASKS-CROSS-WORKSPACE-TRANSFER-002
  - REQ-TASKS-CROSS-WORKSPACE-TRANSFER-003
system_design:
  - ../../specs/tasks/system-design/cross-workspace-task-transfer.md
---

# Implementation plan: Audited cross-workspace task transfers

## Overview

Deliver one least-privilege capability that atomically transfers an existing
task identity across same-owner workspaces into an equivalent lane, preserving
sessions, plans, relations, repositories, and history, with an idempotent
receipt and a redacted audit trail for every attempt.

The work is one vertical slice: schema and repository transaction first, then
service authorization, then the MCP surface, then broadcast reconciliation,
then documentation. Each layer's tests build on the one below it.

## Scope

- Additive task-store schema for the transfer ledger (receipts and audit rows).
- One repository transaction that locks the source, binds every placement
  predicate, resolves the destination lane, validates lane equivalence, workspace
  boundary, relations, and capacity, and persists the transfer with its receipt.
- Service boundary authorization with server-attested actor resolution.
- The `transfer_task_kandev` MCP tool on configuration, external, and
  server-attested Office surfaces.
- Post-commit cache, event, and WIP reconciliation that survives caller
  cancellation and runs exactly once.
- Public automation documentation for the transfer contract.

## Out of scope

- Task recreation, copying, or splitting; cross-owner movement; mapping
  workspace-owned labels, groups, or projects; any frontend surface; and any
  change to same-workspace move behavior.

## Waves and work orders

- [Task 01 — Cross-workspace task transfer](task-01-cross-workspace-task-transfer.md)

## Risks

- A half-transferred task would corrupt live sessions and durable history: the
  transaction is all-or-nothing and every predicate fails closed.
- A leaking error would expose task existence: the stable conflict vocabulary
  never distinguishes denial causes to unattested callers.
- Reconciliation drift after cancellation: post-commit work is idempotent and
  exactly-once under replay.

## Verification strategy

Focused repository, service, handler, and server suites cover the transfer
contract and its rejections; race builds cover concurrent retry; the SQL guard
and schema conformance suites cover the additive migration; the full backend
build and public-docs validation cover integration.

## Acceptance

Delivery is complete when the transfer behaves per
[the requirements](../../specs/tasks/requirements/cross-workspace-task-transfer.md)
and the verification commands in the work order pass.
