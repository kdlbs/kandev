# ADR-2026-11-04-automation-mcp-selection: Explicit automation MCP selection

**Status:** accepted (implemented)
**Date:** 2026-11-04
**Area:** backend, frontend, protocol

## Context

Legacy private automations use a fixed native catalog. Approved plugin tools extend that catalog through a separate live policy.
The user requests built-in package and workspace MCP selection and explicitly permits every built-in package, including Configuration.
Before this change, the package resolver excluded Automation, and the dispatcher denied configuration and authoring actions.
Changing only the editor or discovery would leave selected tools unusable.

## Decision

The automation owns a human-saved selection. Agents owns package descriptors and delivery machinery.
Private automations can select all optional packages. Core retains its automation-specific required catalog.
An explicit Configuration choice grants authorized configuration operations without changing `SurfaceAutomation` or its trusted principal.
Resource permissions and prevention of agent-controlled grants remain mandatory.

Reuse the existing package manifest, routes, dispatch checks, workspace resolver, and execution evidence.
Bind automation choices at a fresh process start. Preserve the accepted choice through same-process continuation and recovery.
Do not restart an active run after Save. Keep plugin approvals on their existing live admission contract.

Workspace server choices are additive. They do not silently remove repository/profile inheritance.
Normal-task automations apply templates at task creation, then leave task-owned customization authoritative.
Managed conversations retain destination-owned tool policy.
Portable exports omit access selections. Imported automations require explicit native selection.

This decision replaces the rejection of per-automation package selection in
[the continuity decision](2026-08-22-user-configured-automation-continuity.md).
It extends [native package selection](2026-10-22-builtin-mcp-tool-policy.md) to private automations.
Those changes become runtime behavior only when this feature's work orders pass.
The remaining coordination and plugin restrictions stay in force.

## Consequences

Configuration access is visible and explicit. A selected package must work through both discovery and direct dispatch.
Core cannot be disabled, and package selection does not authorize arbitrary tool names or arbitrary resource access.
A saved removal does not revoke an already running process. The editor must state the next-process boundary.
An operator who needs immediate cessation can stop the run through the existing control.

All configuration writers must reject grant fields, including generic settings writers and automation creation/update tools.
Selected third-party servers can perform operations under their own credentials. Kandev's native dispatcher does not sandbox those services.

## Alternatives considered

- Keep private runs fixed and require visible tasks. Rejected because the user requests package selection in automations.
- Enable all packages automatically. Rejected because existing automations must not gain Configuration authority without a native save.
- Change the principal to Configuration. Rejected because that would discard automation ownership and coordination restrictions.
- Mutate active package registrations after Save. Rejected because current package delivery binds one manifest per process.
- Copy plugin approval rules onto native packages. Rejected because native package membership and plugin installation approvals have different owners and lifecycles.

## Related artifacts

- [Requirements](../specs/office/requirements/automation-mcp-selection.md)
- [System design](../specs/office/system-design/automation-mcp-selection.md)
- [Implementation plan](../plans/automation-mcp-selection/plan.md)
