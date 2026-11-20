---
created: 2026-11-11
status: implemented
requirements:
  - REQ-AGENTS-BUILTIN-MCP-005
  - REQ-OFFICE-AUTOMATION-MCP-004
system_design:
  - ../../specs/agents/system-design/builtin-mcp-management.md
  - ../../specs/office/system-design/automation-mcp-selection.md
legacy_specs: []
---

# Built-in MCP switch controls

## Overview and confirmed scope

Replace built-in package dropdowns across workspace settings, New Task, task Customize,
and automation editors. The user confirmed explicit automation On/Off on 2026-11-11.
Task inheritance remains live. Workspace permission and default inclusion remain distinct.
Agents owns package policy/preferences. Office owns automation selections and templates.
No ownership change or new ADR is needed. Existing automation authority remains explicit.

The user authorized this package on 2026-11-12 and requested image file previews.
Implementation runs in the primary conversation. No delegation is authorized.

## Scope

Include shared task/workspace details, automation details, source labels, localized copy,
unit and rendered desktop/phone evidence, and public instructions.
Exclude automation inheritance, new persistence, session-specific package overrides,
third-party server controls, runtime delivery changes, and automatic restarts.

## Technical approach

1. Extend the existing builtin-package-choice view model and tests for inherited defaults,
   explicit overrides, restrictions, and workspace draft transitions. Replace PackageChoice
   dropdowns in builtin-package-detail.tsx. Reuse @kandev/ui/switch and existing save owners.
2. Replace AutomationPackageChoice's dropdown with an explicit switch. Preserve membership
   writes, task-mode restrictions, Core, and untouched-selection omission.

The existing helper currently checks saved workspace blocks before workspace draft values.
Tests must expose that stale-block behavior before changing workspace switch derivation.
A restored inherited value is dirty if the saved choice was explicit, even with equal booleans.
A blocked saved task override remains identifiable and resettable. Do not silently normalize it.

No provider or transport path changes. Current accepted bindings remain authoritative for
running agents. The controls project desired state and retain next-process notices.

## UI preview

These structural descriptions replace ASCII previews under the task's rich-output instruction.
Copy is illustrative and must use localization keys.

**UI-01, Task list:** Each row shows package name/description and a separate On/Off switch.
Source and Use inherited value appear below. Package names open optional details.
Core shows Required. Blocked and ineligible packages preserve their reasons and restrictions.
New Task and Customize share this direct-list interaction. Toggling never opens details.

**UI-02, Workspace list:** Each row shows package identity and permission/default switches.
Package names open optional details. Configuration remains explicit-only, with no default switch.

**UI-03, Automation list:** Each optional row shows a package name/description and an On/Off switch.
There is no inheritance action. Core remains required. Package details open separately.

Desktop uses inline package rows with optional details. Phone uses the current settings page,
New Task drawer, or MCP explorer drawer. Stack source and reset below the switch.
Use one scroll owner, fixed existing navigation/save actions, and safe-area clearance.
The existing creation drawer and automation full-page editor are the mobile exemplars.
Touch targets measure at least 44px; ordinary desktop actions remain 28px.
Test keyboard focus return, 320/393/767px phones, the 768px boundary, and 900px coarse touch.
No hover-only help, nested drawer, new animation, or automatic save is introduced.

## Tests and E2E coverage

Task 01 extends builtin-package-choice.test.ts with restored equal-value inheritance,
workspace draft unblocking, inherited On/Off, explicit overrides, required/ineligible packages,
and blocked saved overrides. These protect AC-AGENTS-BUILTIN-MCP-005.1 through .5.
Existing builtin-mcp-management helpers/specs own rendered task/workspace coverage.
Exercise Save/reload, discard, reset, inherited workspace changes, Configuration limits,
failed/conflicting writes, keyboard use, and touch geometry. Assert saved preferences through API.

Task 02 extends automation-mcp helpers and existing desktop/mobile automation selection specs.
Cover explicit toggling, no inheritance control, Core, blocked normal templates,
selected blocked removal, save/discard/reload, and untouched target defaults.
These protect AC-OFFICE-AUTOMATION-MCP-004.11 plus existing save/accessibility contracts.
Update old dropdown-based selectors in every affected helper, not only new scenarios.

## Work orders

- [x] [01: Task and workspace controls](task-01-task-workspace-controls.md)
- [x] [02: Explicit automation controls](task-02-automation-controls.md)

Execute sequentially because shared locales, UI, and documentation overlap.
Install dependencies once from apps with pnpm install --frozen-lockfile if this worktree lacks them.
Run the exact targeted commands in each work order. No broad review or provider test sweep is added.

## Documentation and existing packages

Update docs/public/builtin-mcp-packages.md and docs/public/automation-and-mcp.md during implementation.
Keep completed builtin-mcp-management and automation-mcp-selection plans as historical records.
Their dropdown tests are updated through this package; their historical result counts remain unchanged.
The existing owning requirements/designs link this follow-up. No system boundary changes.

## Verification results

Implementation completed 2026-11-12.

- 8 focused unit tests passed after the expected red failures.
- 36 browser scenarios passed: 12 task/workspace desktop, 11 task/workspace phone,
  8 automation desktop, and 5 automation phone. The canonical inheritance assertion
  was corrected and rerun on desktop. Details are in each work order.
- Final typecheck, full web lint, translation integrity, and new-copy ratchet passed.
- Public documentation tests and validation passed.
- Specification catalog validation, full specification lint, and diff checks passed.
- Design-phase specification-linter tests passed: 36 tests.
- Actual screenshots in artifacts/mcp-switches were inspected and shown through image FilePreview.
- No runtime, API, database, grant, or automatic-restart behavior changed.
- Changes remain uncommitted. No delegation was used.

## Risks

- Confusing blocked with default-off can broaden task access.
- Inferring inheritance from equal booleans can erase an explicit override.
- Reading saved policy during a workspace draft change can leave a switch visually stuck.
- Shared controls must preserve automation explicit membership and all save ownership.

## Direct-list correction, 2026-11-12

The user clarified that switches must work before entering details. Shared rows now separate
navigation from controls. Existing work orders cover this correction. Browser scenarios exercise
list toggles and captures, while preserving detail inspection and focus return.

Correction validation: 20 unique desktop and 16 phone scenarios passed. Two desktop New Task
checks initially measured padding on the navigation button. Their selector now measures the row
container, preserving the original padding assertion. Both reruns passed, as did a taller desktop
capture of task inheritance. All eight unit tests, typecheck, lint, translations, public docs,
specification validation, and diff checks passed. The list captures use the list-* filename prefix.

## Automation run summary removal, 2026-11-12

Removed the duplicate MCP access summary above automation run conversations at the user's request.
The existing MCP plug viewer and automation editor package controls retain their behavior.
Deleted the unused summary component, evidence helper, helper tests, and translation keys.
Updated the owning Office requirement, design, and public guide.

Validation: 25 automation detail tests passed. The private-run browser scenario passed with
summary-absence checks on desktop and a phone viewport. Typecheck, web lint, translation checks,
public documentation validation, and specification validation passed. The updated screenshot is
artifacts/mcp-switches/automation-run-without-summary.png. Changes remain uncommitted.
