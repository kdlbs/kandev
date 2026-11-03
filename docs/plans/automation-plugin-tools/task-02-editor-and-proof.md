---
id: "02-editor-and-proof"
title: "Add editor controls and end-to-end proof"
status: in_progress
wave: 2
depends_on:
  - "01-selection-and-enforcement"
plan: "plan.md"
requirements:
  - REQ-OFFICE-AUTOMATION-PLUGIN-TOOLS-001
  - REQ-OFFICE-AUTOMATION-PLUGIN-TOOLS-002
  - REQ-OFFICE-AUTOMATION-PLUGIN-TOOLS-003
acceptance_criteria:
  - AC-OFFICE-AUTOMATION-PLUGIN-TOOLS-001.1
  - AC-OFFICE-AUTOMATION-PLUGIN-TOOLS-001.2
  - AC-OFFICE-AUTOMATION-PLUGIN-TOOLS-001.4
  - AC-OFFICE-AUTOMATION-PLUGIN-TOOLS-001.5
  - AC-OFFICE-AUTOMATION-PLUGIN-TOOLS-002.5
  - AC-OFFICE-AUTOMATION-PLUGIN-TOOLS-003.1
  - AC-OFFICE-AUTOMATION-PLUGIN-TOOLS-003.2
  - AC-OFFICE-AUTOMATION-PLUGIN-TOOLS-003.3
  - AC-OFFICE-AUTOMATION-PLUGIN-TOOLS-003.4
  - AC-OFFICE-AUTOMATION-PLUGIN-TOOLS-003.5
system_design:
  - ../../specs/office/system-design/automation-plugin-tools.md
---

# Task 02: Add editor controls and end-to-end proof

## Summary

Add optional tool checkboxes and review-required feedback to the existing automation editor.
Prove the complete hidden-run outcome on desktop and the configuration workflow on phones.
Publish user and author documentation with the implementation.

## In scope

- Typed options/save APIs, a domain hook, editor hydration, and independent selection dirty state.
- Valid, stale, unavailable, loading, empty, and error states.
- Existing Save/Discard ownership, preserved drafts, and no extra approval dialog.
- Native checkbox labels, read/write indications, phone targets, wrapping, and translated copy.
- Real packaged-plugin read and report-write calls from a hidden automation through the existing mock-agent MCP script.
- Configuration-change/upgrade invalidation, explicit reselection, and no visible generated task.
- Public automation guidance, feature status, and the plugin author's automation opt-in contract.

## Out of scope

- Another approval page, drawer, dialog, or polling service.
- Changes to managed-conversation or normal-task permission controls.
- Production plugin patches or claims that Kandev Security already supports the new surface.

## Acceptance

- The existing editor can select, save, reload, and clear exact tools on desktop and phone.
  Unrelated saves and failed options requests cannot approve, clear, or overwrite selections or later edits.
- A real hidden run performs the approved fixture read and report write.
  Run history contains the result, while Kanban and the sidebar contain no generated task.
- Plugin changes require explicit reselection, with no inherited access to added tools.
  Phone geometry, localization, and published guidance match the delivered behavior.

## ASCII UI preview

Use [UI-01 and UI-02 in the plan](plan.md#ascii-ui-preview).
The same inline composition serves desktop and phone:

```text
Plugin tools (optional)
Kandev Security                         Clear
[x] List findings                  Read
[ ] Complete report                Write

After a plugin configuration change:
Review required
Previously selected: List findings
[ ] List findings                  Read
[ ] Complete report                Write

Existing editor Save / Discard controls
```

For AC-003.1 through AC-003.5, preserve one editor scroll owner and shared draft state.
Descriptions wrap on phones, and labelled rows provide 44-pixel touch targets.
Fine-pointer desktop controls retain their existing density.
The preview adds no overlay or second approval action.

## Verification

In a fresh worktree, install dependencies once with `(cd apps && pnpm install --frozen-lockfile)`.
Run each command from the repository root:

```bash
(cd apps/web && pnpm exec vitest run components/automations/automation-plugin-tools.test.tsx components/automations/automation-payload.test.ts hooks/domains/settings/use-automation-plugin-tools.test.ts hooks/domains/settings/use-automations.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --host --project chromium e2e/tests/automations-plugin-tools.spec.ts)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome e2e/tests/settings/mobile-automation-plugin-tools.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Managed E2E builds the current backend, web assets, and plugin fixture.
Run desktop and phone sequentially with the configured resource limits.
Record a rendered phone capture and the actual target/overflow measurements.
Run changed-file web lint before completion.
New test files in this block are deliverables of this work order.

## Files likely touched

- `apps/web/lib/types/automation.ts` and `lib/api/domains/automation-api.ts`.
- New `apps/web/hooks/domains/settings/use-automation-plugin-tools.ts` and test.
- New `apps/web/components/automations/automation-plugin-tools.tsx` and test.
- `apps/web/components/automations/automation-editor.tsx`, `automation-editor-sections.tsx`, `config-section.tsx`, `automation-payload.ts`, and tests.
- `apps/web/hooks/domains/settings/use-automations.ts` and tests if response normalization changes.
- `apps/web/src/locales/{en,pseudo,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko}/automations.json`.
- New desktop/phone Playwright files named in Verification.
- `apps/web/e2e/tests/plugins/plugin-test-helpers.ts` and shared fixture helpers only where needed.
- `docs/public/automation-and-mcp.md`, `plugins-authoring.md`, and `feature-status.md`.
- This package's requirements, design, plan, and work-order results after implementation.

## Dependencies

Task 01 provides the options, selection, invalidation, MCP, and packaged-fixture contracts.
Use the existing `e2e:mcp:` script support in `apps/backend/cmd/mock-agent/script.go` without a new test bypass endpoint.

## Risks

- Rehydrating current policy values into a saved stale selection silently renews approval.
- Always sending `plugin_tools: []` from an unrelated Save removes access unexpectedly.
- Browser-only mocks cannot prove the in-session MCP and gRPC boundary.
- External plugin compatibility remains a separate author update.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/office/requirements/automation-plugin-tools.md).
- [System design](../../specs/office/system-design/automation-plugin-tools.md), Editor and mobile behavior.
- `apps/web/e2e/tests/settings/mobile-automations-settings.spec.ts` and `e2e/tests/plugins/plugin-test-helpers.ts`.
- `/mobile-parity`, `/e2e`, `/tdd`, and `/docs-maintainer`.

## Results

Implemented November 3, 2026. Managed Playwright execution remains pending.

- Added typed selection/option APIs, the domain options hook, grouped inline checkboxes, review-required/previously-selected feedback, clear, and visible loading/empty/error states.
- Refreshed options after a rejected save so a changed policy reaches review without discarding other dirty fields.
- Kept the existing editor Save/Discard ownership and omitted `plugin_tools` on unrelated saves; selection dirty state is independent of other form fields.
- Added 44-pixel phone rows, wrapping descriptions, read/write badges, and translated copy in all supported catalogs including pseudo.
- Added desktop hidden-run and phone Playwright specs covering default-empty access, selection, save/reload, invalidation, reselection, clear, touch targets, and overflow. Desktop interactions use click; the phone spec uses the shared fixture install/uninstall helpers.
- Updated public automation and plugin-author documentation.
- Targeted Vitest, TypeScript, ESLint, i18n check/ratchet, and docs lint passed. The managed Playwright suite was not executed here.
