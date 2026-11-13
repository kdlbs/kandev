# ADR-2026-11-02-automation-plugin-tool-selection: Approve exact plugin tools in the automation editor

**Status:** accepted
**Date:** 2026-11-02
**Area:** backend, frontend, protocol, workflow

## Context

Private automations currently exclude plugin tools under the [automation authority decision](2026-08-22-user-configured-automation-continuity.md).
[Discussion #4997](https://github.com/kdlbs/kandev/discussions/4997) requests findings reads and report submission without visible Kanban tasks.
Its comment asks for exact tool approval that expires after configuration changes.
The user requests a small solution.

## Decision

This extension keeps the fixed core automation surface.
The existing native automation editor saves an optional list of exact plugin tools.
Saving a changed selection is the approval action.
No separate approval service or dialog is introduced.

Each saved plugin selection names its installation and one host-owned policy version.
Installation updates and admitted configuration saves replace that version before changed behavior becomes available.
This includes secret-only saves and same-version package replacement.
Unchanged process restarts do not replace it.

The backend checks the current selection for discovery and each invocation.
Plugins must explicitly declare the automation surface.
Core administrative restrictions and existing Host API capability checks remain authoritative.
Tool annotations describe behavior and do not approve access.

This decision extends points 6 and 8 of the earlier authority decision only for explicit plugin selections.
It does not introduce arbitrary selection of core tools.
The [requirements](../specs/office/requirements/automation-plugin-tools.md) and [design](../specs/office/system-design/automation-plugin-tools.md) own the proposed contract.
The earlier decision remains the implemented behavior until this package is delivered.

## Consequences

- One automation field and one plugin policy value represent the approval.
- Plugin and secret configuration values never enter the selection or an approval digest.
- A configuration save can require tool review even when the submitted values are unchanged.
  This conservative rule avoids secret comparisons and another change-detection system.
- Failed updates can invalidate approval without changing effective configuration.
  They must never expose changed configuration under an old approval value.
- Existing plugin capability approval records and managed-conversation policies remain separate.
- Third-party plugins need a manifest and handler update before they support automation tools.

## Alternatives Considered

- **Expose all installed plugin tools.** This grants future tools without an explicit selection and breaks the requested boundary.
- **Add an approval ledger and workflow.** Existing editor authorization is enough for this bounded feature.
- **Reuse a managed-conversation profile unchanged.** That profile changes the MCP surface and has a different lifecycle.
- **Approve only read-only annotations.** Annotations are plugin metadata, and this rule prevents the requested report submission.
- **Hash packages, declarations, and secret configuration into each grant.** One persisted policy value invalidates the same access with fewer moving parts.
