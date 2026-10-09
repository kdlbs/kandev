---
status: active
system: office
created: 2026-09-19
owners:
  - Kandev
---

# Configuration Chat Automation Creation Requirements

## Overview

Configuration chat can create workspace automations through its MCP surface.
Office owns this capability because it owns automation configuration and firing;
MCP supplies another client of that contract.

Existing automation target behavior remains authoritative in
[Automation target modes](automation-target-modes.md). Existing configuration
settings tools continue to discover and edit saved automations.

## Requirements

### REQ-OFFICE-CONFIG-AUTOMATION-001: Create an automation from configuration chat

**Intent:** Let a user describe an automation in configuration chat and have the
assistant save it without opening the automation editor.

#### Acceptance criteria

- **AC-OFFICE-CONFIG-AUTOMATION-001.1:** A configuration-chat session shall
  discover `create_automation_kandev` with a documented input schema. Task,
  title-pending, Office, automation-run, and external MCP catalogs shall not
  gain this tool as part of this change.
- **AC-OFFICE-CONFIG-AUTOMATION-001.2:** A valid creation request shall return
  the saved automation ID, effective configuration, and saved triggers. The
  automation shall be available through existing automation and settings reads.
  Supported creation fields, trigger types, defaults, and one-time webhook
  secret output shall match the existing automation creation contract.
- **AC-OFFICE-CONFIG-AUTOMATION-001.3:** Requests shall respect the caller's
  existing workspace access and workflow/repository ownership checks. Invalid
  configuration or unauthorized targets shall return an error without creating
  an automation. Automation-run callers shall remain unable to invoke this
  mutation through a raw MCP action.
- **AC-OFFICE-CONFIG-AUTOMATION-001.4:** Configuration-chat instructions shall
  explain discovery of required resource IDs, scheduled-trigger configuration,
  and that creation saves an enabled automation with caller-selected trigger
  enablement. Creation shall not independently issue a manual run, although an
  enabled trigger can subsequently fire through normal scheduling.

## Out of scope

New automation edit/delete/run tools, trigger semantics, scheduler changes,
new rendered controls, runtime flags, and changes to external MCP availability.
No new promise of creation idempotency or transactionality is introduced.
