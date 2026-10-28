---
status: active
system: executors
created: 2026-10-26
owners:
  - kandev
---

# Task MCP executor discovery

## Overview

Task agents need to discover custom executor IDs before selecting compute for
child tasks. The executor system owns this catalogue and its profile visibility.
[Issue #4708](https://github.com/kdlbs/kandev/issues/4708) identifies the missing
task-mode entry point.

## Requirements

### REQ-EXECUTORS-TASK-DISCOVERY-001: Discoverable executor catalogue

**Intent:** Agents can select an authorized executor without an ID supplied by a human.

#### Acceptance criteria

- **AC-EXECUTORS-TASK-DISCOVERY-001.1:** Task mode shall expose
  `list_executors_kandev` without required arguments. The result shall include
  every visible executor, including custom IDs, and its visible profiles.
- **AC-EXECUTORS-TASK-DISCOVERY-001.2:** Listings shall enforce the existing
  executor and profile ownership rules for the authenticated caller. A mixed
  catalogue shall retain shared and caller-owned resources and exclude foreign
  owned resources, including foreign profiles on visible executors.
- **AC-EXECUTORS-TASK-DISCOVERY-001.3:** A discovered profile ID shall be usable
  as `executor_profile_id` in authorized task creation. Explicit selections
  shall be checked against current ownership during creation, and launch
  admission shall recheck current eligibility. Discovery shall not grant launch
  authority after ownership changes.
- **AC-EXECUTORS-TASK-DISCOVERY-001.4:** The existing
  `list_executor_profiles_kandev(executor_id)` contract shall remain available.
  Its guidance shall point to executor discovery instead of implying a fixed
  inventory of built-in IDs.
- **AC-EXECUTORS-TASK-DISCOVERY-001.5:** Task mode shall gain no executor or
  executor-profile mutation tools. Other MCP surfaces shall retain their
  existing tool membership and response contracts.
- **AC-EXECUTORS-TASK-DISCOVERY-001.6:** Empty catalogues shall return an empty
  executor collection and zero total. Listing errors shall return errors without
  a successful partial catalogue or a fallback to another identity.

## Related contracts

[Per-user ownership](per-user-ownership.md) remains authoritative for visibility
and admission, including manager access and authentication-disabled behavior.

## Out of scope

New executor types, profile mutations, new permission scopes, runtime launches
caused by discovery, UI changes, and removal of existing tools.

## Implementation plans

[Task MCP executor discovery](../../../plans/task-mcp-executor-discovery/plan.md).
