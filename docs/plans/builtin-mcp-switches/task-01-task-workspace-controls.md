---
id: "01-task-workspace-controls"
title: "Task and workspace controls"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-BUILTIN-MCP-005
acceptance_criteria:
  - AC-AGENTS-BUILTIN-MCP-005.1
  - AC-AGENTS-BUILTIN-MCP-005.2
  - AC-AGENTS-BUILTIN-MCP-005.3
  - AC-AGENTS-BUILTIN-MCP-005.4
  - AC-AGENTS-BUILTIN-MCP-005.5
system_design:
  - ../../specs/agents/system-design/builtin-mcp-management.md
---

# Task 01: Task and workspace controls

## Summary and scope

Implement this slice of the [switch plan](plan.md). Own the files listed below,
related locale keys, and the corresponding rendered regression tests.
Use TDD for changed logic and observable interaction changes. Pure markup needs no component unit test.
Exclude runtime, API, schema, provider changes, and automation inheritance.

## Acceptance

- Task switch edits and inheritance restoration persist with truthful source notes.
- Workspace switches retain all three policies and Configuration restrictions.
- Existing save/error states and desktop/phone flows pass targeted coverage.

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

Relevant views: UI-01 and UI-02. See the [combined preview](plan.md#ui-preview).

## Verification

From the repository root, after dependency bootstrap when needed:

```bash
(cd apps/web && pnpm exec vitest run components/mcp/builtin-package-choice.test.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/builtin-mcp-management.spec.ts tests/settings/builtin-mcp-configuration.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-builtin-mcp-management.spec.ts)
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

- `apps/web/components/mcp/builtin-package-detail.tsx`
- `apps/web/components/mcp/builtin-package-choice.ts`
- `apps/web/components/mcp/builtin-package-choice.test.ts`
- `apps/web/components/mcp/builtin-package-list.tsx`
- `apps/web/e2e/helpers/builtin-mcp-management.ts`
- `apps/web/e2e/helpers/builtin-mcp-save-feedback.ts`
- `apps/web/e2e/tests/settings/*builtin-mcp*.spec.ts`
- `apps/web/src/locales/*/settings.json`
- `docs/public/builtin-mcp-packages.md`

## Dependencies

None.

## Risks

Preserve persisted meaning, disabled states, save ownership, and viewport-independent drafts.
Do not reuse task inheritance controls in the automation editor.

## Parallelism

Sequential. No delegation authorized.

## Inputs

Read the linked owning requirement/design and plan. Reuse current controllers and E2E helpers.

## Results

Completed 2026-11-12.

- Unit red: three new cases failed. Green: all 8 cases passed.
- Browser red: the workspace flow failed because the old detail had no switch.
- Desktop: 11 scenarios passed initially. The new inheritance case expected a literal inherit value,
  but the backend canonically removes inherited entries. Corrected the assertion to require absence;
  the targeted rerun passed. All 12 task/workspace desktop scenarios now pass.
- Phone: all 11 scenarios passed, including Portuguese layout at 320/393/767/768/900px.
- Typecheck, lint, i18n:check, i18n:ratchet, catalog validation, specification lint, and diff checks passed.
- Public-document tests and validation passed.
- Actual UI captures are in artifacts/mcp-switches and were inspected with the image viewer.

Commands use the work-order verification block. On this Nix host, browser runs also require
LD_LIBRARY_PATH from installed Chromium libraries, KANDEV_SERVER_HOST=127.0.0.1, and removal
of inherited KANDEV_INTERNAL_CONFIG_FILE and KANDEV_INTERNAL_CONFIG_HOME_FILE variables.
These command-local overrides isolate the tests and do not change the running developer instance.
The first attempt hit inherited non-loopback binding; the next exposed missing libglib.
Both environment issues were resolved before the expected browser red result.
Subsequent unchanged builds use the runner's --no-build option.

## Direct-list correction, 2026-11-12

The user clarified that switches must work before entering details. Shared rows now separate
navigation from controls. Existing work orders cover this correction. Browser scenarios exercise
list toggles and captures, while preserving detail inspection and focus return.

Correction validation: 20 unique desktop and 16 phone scenarios passed. Two desktop New Task
checks initially measured padding on the navigation button. Their selector now measures the row
container, preserving the original padding assertion. Both reruns passed, as did a taller desktop
capture of task inheritance. All eight unit tests, typecheck, lint, translations, public docs,
specification validation, and diff checks passed. The list captures use the list-* filename prefix.
