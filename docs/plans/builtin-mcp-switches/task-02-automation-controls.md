---
id: "02-automation-controls"
title: "Explicit automation controls"
status: done
wave: 2
depends_on: ["01-task-workspace-controls"]
plan: "plan.md"
requirements:
  - REQ-OFFICE-AUTOMATION-MCP-004
acceptance_criteria:
  - AC-OFFICE-AUTOMATION-MCP-004.11
system_design:
  - ../../specs/office/system-design/automation-mcp-selection.md
---

# Task 02: Explicit automation controls

## Summary and scope

Implement this slice of the [switch plan](plan.md). Own the files listed below,
related locale keys, and the corresponding rendered regression tests.
Use TDD for changed logic and observable interaction changes. Pure markup needs no component unit test.
Exclude runtime, API, schema, provider changes, and automation inheritance.

## Acceptance

- Automation switches preserve explicit membership and expose no inheritance control.
- Core, target restrictions, save/discard, and untouched defaults remain correct.
- Desktop and phone rendered checks pass with localized accessible controls.

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

Relevant views: UI-03. See the [combined preview](plan.md#ui-preview).

## Verification

From the repository root, after dependency bootstrap when needed:

```bash
(cd apps/web && pnpm e2e:run --project chromium tests/automations-mcp-selection.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-automations-mcp-selection.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Regenerate Traditional Chinese with pnpm run i18n:zh-hant from apps/web after Chinese key edits.
Run new assertions before implementation and record the expected failing behavior.
Measure rendered geometry and compare structure against the preview. Do not infer visual success from unit tests.

## Files likely touched

- `apps/web/components/automations/automation-builtin-packages.tsx`
- `apps/web/e2e/helpers/automation-mcp.ts`
- `apps/web/e2e/tests/automations-mcp-selection.spec.ts`
- `apps/web/e2e/tests/settings/mobile-automations-mcp-selection.spec.ts`
- `apps/web/src/locales/*/automations.json`
- `docs/public/automation-and-mcp.md`

## Dependencies

Task 01. Reuse the established switch treatment and shared translations.

## Risks

Preserve persisted meaning, disabled states, save ownership, and viewport-independent drafts.
Do not reuse task inheritance controls in the automation editor.

## Parallelism

Sequential. No delegation authorized.

## Inputs

Read the linked owning requirement/design and plan. Reuse current controllers and E2E helpers.

## Results

Completed 2026-11-12.

- Browser red failed because the previous automation detail had no switch.
- Desktop: all 8 automation scenarios passed.
- Phone: all 5 scenarios passed, including a narrow fine-pointer viewport and keyboard activation.
- Tests cover explicit package membership, absence of inheritance actions, blocked selection removal,
  persistence, conflict recovery, private-run tool invocation, and retained process bindings.
- Final typecheck, full web lint, translation integrity, and the new-copy ratchet passed.
- Actual desktop/phone captures are in artifacts/mcp-switches. Image previews were inspected and published through FilePreview.
- Public docs tests/validation, catalog validation, specification lint, and diff checks passed.

Use the documented command-local Nix browser libraries and isolated configuration overrides from Task 01.
The mobile run used --no-build against the same freshly built assets as the passing desktop run.

## Direct-list correction, 2026-11-12

The user clarified that switches must work before entering details. Shared rows now separate
navigation from controls. Existing work orders cover this correction. Browser scenarios exercise
list toggles and captures, while preserving detail inspection and focus return.

Correction validation: 20 unique desktop and 16 phone scenarios passed. Two desktop New Task
checks initially measured padding on the navigation button. Their selector now measures the row
container, preserving the original padding assertion. Both reruns passed, as did a taller desktop
capture of task inheritance. All eight unit tests, typecheck, lint, translations, public docs,
specification validation, and diff checks passed. The list captures use the list-* filename prefix.


## PR review remediation, 2026-11-13

Link blocked automation switches to the visible policy reason in both list and detail views.
Update the public package-selection steps to use the list switches. Existing package access
and save semantics remain unchanged, so no requirement or design contract changes are needed.

Validation: the shared 43-test unit/component run includes both automation surfaces. The
`removes blocked and inherited direct MCP choices without losing inheritance` browser scenario
passed in Chromium and mobile-chrome with accessible-description assertions. Typecheck and
public documentation validation passed. Remote checks remain pending for the remediation head.
