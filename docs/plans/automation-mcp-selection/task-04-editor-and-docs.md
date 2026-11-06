---
id: "04-editor-and-docs"
title: "Add editor, run evidence, and documentation"
status: done
wave: 4
depends_on:
  - "03-workspace-delivery"
plan: "plan.md"
requirements:
  - REQ-OFFICE-AUTOMATION-MCP-001
  - REQ-OFFICE-AUTOMATION-MCP-002
  - REQ-OFFICE-AUTOMATION-MCP-003
  - REQ-OFFICE-AUTOMATION-MCP-004
acceptance_criteria:
  - AC-OFFICE-AUTOMATION-MCP-001.1
  - AC-OFFICE-AUTOMATION-MCP-001.2
  - AC-OFFICE-AUTOMATION-MCP-001.3
  - AC-OFFICE-AUTOMATION-MCP-001.4
  - AC-OFFICE-AUTOMATION-MCP-001.5
  - AC-OFFICE-AUTOMATION-MCP-001.6
  - AC-OFFICE-AUTOMATION-MCP-002.1
  - AC-OFFICE-AUTOMATION-MCP-002.2
  - AC-OFFICE-AUTOMATION-MCP-002.3
  - AC-OFFICE-AUTOMATION-MCP-002.4
  - AC-OFFICE-AUTOMATION-MCP-002.5
  - AC-OFFICE-AUTOMATION-MCP-002.6
  - AC-OFFICE-AUTOMATION-MCP-003.1
  - AC-OFFICE-AUTOMATION-MCP-003.2
  - AC-OFFICE-AUTOMATION-MCP-003.3
  - AC-OFFICE-AUTOMATION-MCP-003.4
  - AC-OFFICE-AUTOMATION-MCP-003.5
  - AC-OFFICE-AUTOMATION-MCP-004.1
  - AC-OFFICE-AUTOMATION-MCP-004.2
  - AC-OFFICE-AUTOMATION-MCP-004.3
  - AC-OFFICE-AUTOMATION-MCP-004.4
  - AC-OFFICE-AUTOMATION-MCP-004.5
system_design:
  - ../../specs/office/system-design/automation-mcp-selection.md
---

# Task 04: Add editor, run evidence, and documentation

## Summary

Add MCP access to the existing automation editor and show truthful run evidence.
Deliver the same configuration and recovery behavior on desktop and phone.

## In scope

Domain API/hooks/types, editor sections and payload state, shared Save/Discard, stale-response handling,
run evidence, all locales, desktop/mobile E2E, public docs, and final specification lifecycle reconciliation.

## Out of scope

New automation pages, general MCP navigation changes, automatic restart controls, and provider connectivity claims.

## Acceptance

- All optional packages and workspace additions save/reload correctly. Plugin approvals retain their separate behavior and native save boundary.
- UI-01/02/03 pass desktop/phone interactions, target measurements, overflow, focus, viewport draft retention, conflict, and unavailable-state checks.
- Docs explain scope, Configuration authority, inheritance, timing, export, and legacy helpers. Promote specs only after all work-order checks pass.

## UI preview

The structured preview in the task conversation uses these stable view labels.
These descriptions are the durable layout contract. They replace ASCII sketches as requested by the task's rich-output instructions.

**UI-01: Desktop editor, explicit private selection.** Entry: workspace Automations > create/edit.
The existing Then section precedes MCP access. Its groups appear in this order:

1. Built-in packages: Core (Required), Configuration, Rich output, Walkthroughs, Diagnostics.
2. Workspace MCP servers: search, inherited rows with source labels, then additional selectable definitions.
3. Plugin tools: existing grouped selection and approval state.

Package rows use the shared task/workspace list, with status badges and chevrons. Select a row to open its inline detail and On/Off selector. Core remains Required. Configuration's explanation stays visible below the package picker. The private timing notice says changes apply at the next agent process start.
Existing Save changes and Discard controls remain in the settings save surface.
Sample package choices are illustrative. Group order, required Core, authority notice, and shared save ownership are required.

**UI-02: Phone editor, same selection.** Use the existing full-page automation editor and inline groups.
The hierarchy matches UI-01. One page scroll owner contains the sections, with the existing save surface clear of safe areas.
Use wrapped labels and at least 44px phone/coarse-pointer targets. Preserve one draft during desktop/phone transitions.
A long workspace catalog uses the existing bounded searchable list. Do not introduce nested dialogs or a new management page.

**UI-03: Recovery and run evidence.** Loading preserves saved choices. An options error offers Retry and retains unavailable selected rows.
A stale save retains the draft and offers Reload choices before an explicit retry.
A saved package change on a reused process shows Desired and Current process separately, plus next-process timing.
Missing historical evidence says Unknown. An On badge never implies Connected.

UI-01/02 map to AC-OFFICE-AUTOMATION-MCP-004.1, .2, .4, and .5.
UI-03 maps to AC-OFFICE-AUTOMATION-MCP-003.1, .4, .5 and AC-OFFICE-AUTOMATION-MCP-004.2, .3.

Full combined preview: [plan](plan.md#ui-preview).

## Verification

Run from the repository root. Use `/tdd` for meaningful logic tests.
New test paths named by this work order must exist before its verification completes.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/automations/automation-payload.test.ts components/automations/automation-plugin-tools.test.tsx hooks/domains/settings/use-automation-mcp-options.test.ts hooks/domains/settings/use-automation-plugin-tools.test.ts lib/state/slices/session-runtime/automation-mcp-validation.test.ts lib/state/slices/session-runtime/builtin-mcp-attachment.test.ts components/mcp/mcp-origin-labels.test.ts components/mcp/mcp-origin-groups.test.ts components/runs/automation-mcp-evidence.test.ts components/runs/automation-detail-page.test.tsx)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
(cd apps && pnpm --filter @kandev/web lint)
(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/automations-mcp-selection.spec.ts tests/automations-plugin-tools.spec.ts tests/automations-target-modes.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-automations-mcp-selection.spec.ts tests/settings/mobile-automation-plugin-tools.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/components/automations/automation-editor.tsx`
- `apps/web/components/automations/automation-editor-save.ts`
- `apps/web/components/automations/automation-editor-save.test.ts`
- `apps/web/components/automations/automation-payload.ts`
- `apps/web/components/automations/automation-mcp-access.tsx (new)`
- `apps/web/hooks/domains/settings/use-automation-mcp-options.ts (new)`
- `apps/web/lib/api/domains/automation-api.ts`
- `apps/web/lib/state/slices/automations/types.ts`
- `apps/web/components/runs/automation-detail-page.tsx`
- `apps/web/components/runs/automation-mcp-evidence.tsx (new)`
- `apps/web/src/locales/`
- `apps/web/e2e/tests/automations-mcp-selection.spec.ts (new)`
- `apps/web/e2e/tests/settings/mobile-automations-mcp-selection.spec.ts (new)`
- `docs/public/automation-and-mcp.md`
- `docs/public/builtin-mcp-packages.md`
- `docs/specs/office/requirements/automation-mcp-selection.md`
- `docs/specs/office/system-design/automation-mcp-selection.md`
- `docs/specs/office/requirements/automation-continuity.md`
- `docs/specs/agents/requirements/builtin-mcp-management.md`
- `docs/specs/agents/system-design/builtin-mcp-management.md`

## Dependencies

03-workspace-delivery.

## Risks

An unrelated save must not approve loaded choices. Do not display desired choices as accepted historical run evidence.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/office/requirements/automation-mcp-selection.md).
- [System design](../../specs/office/system-design/automation-mcp-selection.md).
- [Authority decision](../../decisions/2026-11-04-automation-mcp-selection.md).
- Existing plugin-tool save tests, built-in package binding tests, and workspace resolver tests.

## Results

The editor, stale-save reconciliation, run evidence, locales, and public docs are implemented.

- The implementation baseline passed 80 tests in eight files. The expanded command above passed 116 tests in ten files during PR remediation. Rendering is covered through focused desktop/phone browser flows. State and request behavior use pure-model and domain-hook tests.
- Typecheck passed with an 8 GiB Node heap. Full web lint, translation checks, and new-copy ratchet passed.
- Four new desktop scenarios passed. The prior combined run also passed the existing plugin-tool and both target-mode scenarios.
- Three phone scenarios passed, including the existing plugin flow. New checks cover Save/Reset, all optional packages, viewport draft retention and overflow at 320/393/767/768px, 44px rows, and a narrow fine pointer with keyboard selection.
- Desktop and phone captures were inspected. No actionable UI-polish findings remained. There is no new motion to verify.
- Public docs updated: `docs/public/automation-and-mcp.md` (how-to) and `docs/public/builtin-mcp-packages.md` (reference). The root README and screenshot catalog were checked. No new overview or media change was needed.
- Public validators passed 63 tests and validated 55 pages. Catalog/spec lint and actual-diff work-order coverage passed.
- Detailed commands, environment adjustments, final lint, and limits are recorded in [the plan](plan.md#verification-results).

### PR remediation

PR remediation covers canonical save values, retained in-flight edits, task-mode round trips, stale callbacks, unsaved recovery, named automation origins, removable blocked/direct inherited choices, corrected translations, and disclosure-scoped evidence reads. The plan records the expanded tests and browser results.

The final review batch passed 144 focused web cases and ten desktop plus five phone browser scenarios. The browser checks include the deletion warning, new-process attachment limit, and retained automation references. Save-stage regressions distinguish repository, creation, update, and trigger errors. Hook regressions cover committed ownership, stale callbacks, and recovery after a failed reload.

### Package picker consistency refinement

- Reuse shared package rows, detail headers, and tool information across workspace, task, and automation editors.
- Keep the phone entry point and page scroll owner. Open package details inline, use touch-sized controls, and return focus on Back to packages.
- Preserve explicit automation On/Off grants, required Core, normal-task workspace blocks, and shared Save/Reset ownership.
- Verify touch selection, keyboard navigation, registered tools, reload, conflict recovery, blocked-choice removal, and responsive draft retention in the existing desktop/phone suites.
- Validation: the keyboard detail test failed before implementation. After implementation, 18 desktop and 15 phone E2E cases passed with retries disabled. These include the existing task/workspace package picker suites. The 17 focused unit cases, web typecheck, changed-file lint, translation integrity/ratchet, public-doc checks, and specification checks passed.

### Follow-up review and CI checks

- The saved automation baseline now advances after its update succeeds, even if a later trigger write fails. A hook integration regression covers Reset, pending-trigger preservation, and a subsequent MCP edit with the accepted revision.
- The narrow fine-pointer scenario uses the desktop page fixture at phone width and verifies the actual pointer media query. The previous fixture forced Pixel 5 touch options.
- The SSH keepalive stop check is shared at the loss-notification boundary. Deterministic stopped/running cases for probe errors and deadlines replace a scheduler-dependent coincidence test that failed in CI. Focused SSH transport tests passed with the race detector.
- Validation: 39 focused hook/payload tests, eight desktop automation cases, five phone cases, and SSH keepalive/transport tests with the race detector passed. Both partial-save and pointer regressions failed before their fixes. The stopped-watchdog boundary cases failed before centralizing the guard. Changed-Go lint, changed-web lint, typecheck, and specification checks passed.

### Repository retry follow-up

- Accepted repository registrations now reach the current form before trigger writes. Matching rows retain branch edits, replacements, removals, and ordering changes made during the automation update.
- Three regressions failed before the fix: reuse of a registered repository after a trigger failure, preservation of a new branch, and preservation of a replacement repository. All 42 focused save, trigger, and payload cases passed afterward.
- A synchronized loop regression queues probe errors and pending stops before loop entry. It complements the direct notification-boundary cases and passes with the SSH transport tests under the race detector.
- The loss-notification and deadline helpers now document their existing invariants.
- Final local checks passed: 44 focused cases, eight desktop scenarios, five phone scenarios, typecheck, changed-web lint, changed-Go lint, and SSH transport tests with the race detector. Removal, addition, and row ordering are covered alongside branch edits and replacements.
