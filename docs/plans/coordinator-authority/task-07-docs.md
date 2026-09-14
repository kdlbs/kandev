---
id: "07-docs"
title: "Public docs: coordinator grants operator guide"
status: done
wave: 7
depends_on:
  - "05-operator-api"
plan: "plan.md"
requirements:
  - REQ-TASKS-COORDINATOR-AUTHORITY-001
  - REQ-TASKS-COORDINATOR-AUTHORITY-002
acceptance_criteria:
  - AC-TASKS-COORDINATOR-AUTHORITY-001.3
  - AC-TASKS-COORDINATOR-AUTHORITY-002.2
system_design:
  - ../../specs/tasks/system-design/coordinator-task-authority.md
---

# Task 07 — Public Docs: Coordinator Grants Operator Guide

## Owner

Docs

## Predecessors

05 (Operator API)

## Description

Write operator-facing documentation for coordinator grants:

- What capabilities grant and don't grant
- How to grant, inspect, and revoke
- Audit trail explanation
- Threat model (grant == operator API access)
- Recovery path (revoke → prior behavior on next call)
- Default (no grants, flag off → identical to today)
