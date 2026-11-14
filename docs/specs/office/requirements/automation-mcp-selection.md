---
status: active
system: office
created: 2026-11-04
owners:
  - kandev
---

# Automation MCP selection

## Outcome and ownership

An automation author can select built-in packages and workspace MCP servers before a run starts.
Private automations can use every built-in package, including Configuration, without creating visible Kanban tasks.
Office owns the automation configuration and its application to runs.
Agents continues to own package membership, workspace definitions, provider delivery, and accepted execution evidence.

The user explicitly confirmed on 2026-11-04: “We should allow selecting any package.”
This includes Configuration. Core remains required because it supplies run coordination.
The [decision](../../../decisions/2026-11-04-automation-mcp-selection.md) records the change to fixed automation authority.

## Evidence and terms

[Discussion #4997](https://github.com/kdlbs/kandev/discussions/4997) reports unavailable plugin tools in private automations.
The existing [plugin-tool contract](automation-plugin-tools.md) now addresses that report.
This feature adds built-in packages and workspace MCP definitions beside that existing selection.
[Issue #3805](https://github.com/kdlbs/kandev/issues/3805) concerns missing provider tools in ordinary tasks.
Its closing comment identifies a released fix. It is related evidence, not proof of this automation defect.

- **Private run:** An automation using the hidden `automation_run` target.
- **Normal task:** A visible task created by the `normal_task` target.
- **Selection:** Optional built-in packages plus additional workspace MCP definitions saved on an automation.
- **Accepted selection:** The selection accepted for a particular agent process. Saving desired choices does not change this evidence.

## Requirements

### REQ-OFFICE-AUTOMATION-MCP-001: Configure tools before execution

**Intent:** Give authors explicit control without changing existing automations automatically.

- **AC-OFFICE-AUTOMATION-MCP-001.1:** Private automation editors shall offer Configuration, Rich output, Walkthroughs, and Diagnostics. Core shall appear as required.
- **AC-OFFICE-AUTOMATION-MCP-001.2:** Editors shall offer enabled MCP definitions from the automation's workspace. Selected definitions shall add to existing repository/profile selections without duplicates.
  Inherited definitions shall show their source. Clearing additions shall not claim to remove inherited servers.
- **AC-OFFICE-AUTOMATION-MCP-001.3:** New private automations shall initially use their existing Core capabilities, with optional packages off and no additional definitions.
  Existing automations without a saved selection shall preserve current delivery, including inherited servers and approved plugin tools.
- **AC-OFFICE-AUTOMATION-MCP-001.4:** Omitted selection updates shall preserve saved choices. Explicit empty additions shall clear only direct additions.
  Invalid, foreign, disabled, unknown, duplicate, or stale selection writes shall reject without partial changes. Mode changes that clear a selection shall also require its editor revision.
- **AC-OFFICE-AUTOMATION-MCP-001.5:** Native automation configuration shall support normal-task templates using the same package and server controls.
  Normal tasks shall retain regular-task package limits and defaults. Managed-conversation automations shall retain destination-owned tool policy.
- **AC-OFFICE-AUTOMATION-MCP-001.6:** Portable export shall omit selections, authorization revisions, and accepted execution state.
  Imports shall start with existing target-mode defaults and require a native save to add access.

### REQ-OFFICE-AUTOMATION-MCP-002: Enforce selected native authority

**Intent:** Make selected packages usable while retaining authenticated ownership and run identity.

- **AC-OFFICE-AUTOMATION-MCP-002.1:** An explicitly selected private-run package shall expose its applicable tools and permit their authorized invocation.
  Configuration shall permit configuration operations. Rich output and Walkthroughs shall support authoring for the run. Diagnostics shall support the run's diagnostic operation.
- **AC-OFFICE-AUTOMATION-MCP-002.2:** Private runs shall retain their automation identity, workspace boundary, hidden history, completion behavior, and coordination restrictions.
  Selecting a package shall not change the run into a configuration chat or ordinary task.
- **AC-OFFICE-AUTOMATION-MCP-002.3:** Only authorized native configuration saves shall grant package access or additional MCP definitions to automations.
  Agent calls, plugin schedules, forged arguments, and retry data shall not create or increase these selections.
- **AC-OFFICE-AUTOMATION-MCP-002.4:** Configuration shall obey existing resource authorization, including shared-resource permissions.
  It shall not change built-in grants, automation MCP selections, or plugin approvals through direct or indirect configuration writes.
- **AC-OFFICE-AUTOMATION-MCP-002.5:** Discovery, direct calls, alternate routes, and generated instructions shall agree on accepted package availability.
  Unselected tools shall fail before effects. Each canonical tool shall appear at most once.
  Existing plugin selection checks shall remain independent and enforce their current live admission rules.
- **AC-OFFICE-AUTOMATION-MCP-002.6:** Required Core coordination shall preserve self-target restrictions and workspace isolation.
  Package-local authoring shall use the current hidden task. It shall not grant self-messaging, self-spawning, or task-local user questions.

### REQ-OFFICE-AUTOMATION-MCP-003: Apply selections predictably

**Intent:** Preserve working runs and show when changed choices take effect.

- **AC-OFFICE-AUTOMATION-MCP-003.1:** Saving private-run choices shall not start, stop, restart, or reconnect an agent.
  The choices shall apply at the next normal process start. Another turn in a reused process shall keep its accepted selection.
- **AC-OFFICE-AUTOMATION-MCP-003.2:** Fresh private-run starts, replacement processes, and retry starts shall resolve current saved choices.
  Retained processes, continuation, reconnect, and backend adoption shall preserve accepted choices.
  Historical retry snapshots shall not restore removed grants for a fresh process.
- **AC-OFFICE-AUTOMATION-MCP-003.3:** Normal-task templates shall apply at task creation before the first start.
  Reusing an existing normal task shall preserve that task's subsequent human edits. Automation edits shall affect newly created tasks.
- **AC-OFFICE-AUTOMATION-MCP-003.4:** Selection changes during launch shall not relabel an earlier accepted launch.
  Invalid explicit selection state or unsupported package enforcement shall prevent a fresh start with an actionable error.
  Retained legacy processes shall preserve their current tools and show unknown historical selection evidence.
- **AC-OFFICE-AUTOMATION-MCP-003.5:** Workspace definitions shall retain existing transport filtering, secret handling, name-collision behavior, and delivery errors.
  Missing or disabled definitions shall show an unavailable decision, never an unrelated replacement.
  Selected or delivered shall not imply connected. Existing inherited-server updates retain their established application rules.

### REQ-OFFICE-AUTOMATION-MCP-004: Edit and inspect on desktop and phone

**Intent:** Make selection, timing, and recovery understandable in the existing editor.

- **AC-OFFICE-AUTOMATION-MCP-004.1:** The editor shall group Built-in packages, Workspace MCP servers, and existing Plugin tools under MCP access.
  Configuration shall explain its settings mutation authority before Save. Each group shall keep its distinct save and application semantics visible.
- **AC-OFFICE-AUTOMATION-MCP-004.2:** The existing Save changes and Discard actions shall own the draft.
  Loading, unavailable options, unrelated saves, failed saves, and stale responses shall not erase or silently approve selections.
  Conflicts shall retain the draft and require explicit reconciliation after reload.
- **AC-OFFICE-AUTOMATION-MCP-004.3:** Automation run details shall use the existing MCP plug viewer for MCP inspection and shall not add a separate MCP access summary above the conversation.
  Historical runs shall not display today's choices as their accepted selection.
- **AC-OFFICE-AUTOMATION-MCP-004.4:** Phone and desktop shall support the same selection and recovery actions.

- **AC-OFFICE-AUTOMATION-MCP-004.5:** New copy and accessible labels shall use all shipped locale catalogs.
  Reasons shall remain readable without hover, color, or motion alone.
- **AC-OFFICE-AUTOMATION-MCP-004.6:** Phone and coarse-pointer targets shall measure at least 44px.
- **AC-OFFICE-AUTOMATION-MCP-004.7:** Ordinary desktop controls shall retain 28px sizing.
- **AC-OFFICE-AUTOMATION-MCP-004.8:** Long names shall wrap without horizontal page overflow.
- **AC-OFFICE-AUTOMATION-MCP-004.9:** Keyboard focus shall survive viewport changes.
- **AC-OFFICE-AUTOMATION-MCP-004.10:** Drafts shall survive viewport changes.

- **AC-OFFICE-AUTOMATION-MCP-004.11:** Optional built-in packages shall show explicit On/Off switches directly in the package list, without an inheritance action. Toggling shall not open details. Existing target-mode defaults and saved selections shall remain unchanged. Core shall remain required. Workspace restrictions shall still prevent enabling blocked packages in normal-task templates.

## Exclusions

No arbitrary built-in tool allowlists, new package identities, provider integrations, endpoint probes, or automatic process restarts.
This work does not change plugin approval, managed-conversation policy, native questions, or Core completion rules.
External MCP services retain their own authorization. Package selection does not grant third-party credentials.

## Implementation plans

- [Automation MCP selection](../../../plans/automation-mcp-selection/plan.md)

The completed [switch controls plan](../../../plans/builtin-mcp-switches/plan.md) owns AC-OFFICE-AUTOMATION-MCP-004.11.
