---
id: "07-add-scoped-mcp-selectors"
title: "Add scoped MCP selectors"
status: done
wave: 5
depends_on:
  - "03-migrate-scoped-mcp-selections"
  - "05-apply-idle-session-mcp-changes"
  - "06-build-workspace-mcp-settings"
plan: "plan.md"
requirements:
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-003
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-005
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-006
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-007
acceptance_criteria:
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-003.1
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-003.2
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-003.4
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-003.7
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-005.1
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-005.2
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-005.5
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-005.6
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-005.7
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.2
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.3
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.4
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.5
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.6
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.7
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.8
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-007.4
system_design:
  - ../../specs/agents/system-design/workspace-mcp-configuration.md
---

# Task 07: Add Scoped MCP Selectors

## Summary

Replace the raw profile MCP editor with one reusable, origin-aware selection
control. Use it in profile, repository, task creation, new task agent, and idle
session flows with distinct desktop and phone compositions.

## In scope

- Add typed selection and effective-summary API clients.
- Build `MCPSelectionPicker` with separate inherited and current-scope groups,
  origin labels, search, disabled definitions, and empty states.
- For global profiles, require a workspace context before selection. Use the
  owning workspace directly for workspace profiles.
- Replace `profile-mcp-config-card.tsx` after migration fallback is available.
  Retire only that card. Leave `profile-edit/mcp-policy-card.tsx`,
  `mcp-task-agent-profile-default-settings.tsx`, and `custom-tui-mcp-card.tsx`
  in place; the policy card configures the transport filtering that runs after
  scope composition.
- Add repository settings selection.
- Put repository selection in a collapsed-by-default section. Show the selected
  count in the closed disclosure header.
- Add optional task selection state to task-create dialog state, payload
  builders, and submission.
- Keep the task MCP selector inside the existing collapsed Advanced settings
  section. Do not show it in the primary task form.
- Add session additions to new task agent creation.
- Add idle-session selection with pending, applied, deferred, and failed
  revision states plus retry.
- Use a mobile bottom sheet or direct route with 44-pixel rows and one scroll
  owner. Preserve the existing task-create full-height mobile composition.
- Localize every new state in all shipped locales.

## Out of scope

- Catalog and marketplace editing.
- Backend selection, resolver, or reconnect behavior.
- Playwright journeys, which Task 08 owns.

## Acceptance

- Every scope can add enabled workspace definitions and shows inherited origins
  without implying that inherited items can be removed locally.
- Repository and task selectors consume no expanded form space until the user
  opens their disclosure section.
- Global profile edits cannot save until the workspace context is explicit.
- Desktop and phone session controls display desired and applied state and keep
  active-turn, retry, and next-start behavior understandable and accessible.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps/web && pnpm exec vitest run components/mcp/mcp-selection-picker.test.tsx app/settings/agents/[agentId]/profile-mcp-config-card.test.tsx components/settings/repository-card.test.tsx components/task-create-dialog-helpers.test.ts components/task-create-dialog-state.test.ts components/task/mcp-session-selector.test.tsx
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run i18n:check
```

Write picker and payload tests first. The first RED run must show missing
workspace context, lost task-create IDs, and incorrect inherited checkbox
semantics.

## Files likely touched

- `apps/web/components/mcp/mcp-selection-picker.tsx`
- `apps/web/components/mcp/mcp-selection-picker.test.tsx`
- `apps/web/lib/api/domains/mcp-api.ts`
- `apps/web/lib/types/http-mcp.ts`
- `apps/web/app/settings/agents/[agentId]/profile-mcp-config-card.tsx`
- `apps/web/app/settings/agents/[agentId]/profile-mcp-config-card.test.tsx`
- `apps/web/components/settings/repository-card.tsx`
- `apps/web/components/settings/repository-card.test.tsx`
- `apps/web/components/task-create-dialog.tsx`
- `apps/web/components/task-create-dialog-state.ts`
- `apps/web/components/task-create-dialog-helpers.ts`
- `apps/web/components/task/mcp-session-selector.tsx`
- `apps/web/components/task/new-session-dialog.tsx`
- `apps/web/lib/types/http-agents.ts`
- `apps/web/src/locales/en/agents.json`
- `apps/web/src/locales/en/task.json`
- `apps/web/src/locales/en/workspaces.json`
- `apps/web/src/locales/pt-pt/`
- `apps/web/src/locales/zh-cn/`
- `apps/web/src/locales/zh-hk/`
- `apps/web/src/locales/zh-tw/`

## Dependencies

- Task 03 supplies selection APIs and task/session creation fields.
- Task 05 supplies desired/applied session state and retry behavior.
- Task 06 supplies shared catalog types, clients, and workspace settings links.

## Risks

- The current profile card owns raw JSON validation. Retire only that UI path,
  not the legacy fallback required by unimported workspaces.
- Task create state is split across helpers and components. Ensure every submit
  transport maps the same IDs.
- A checked inherited row implies subtraction. Render it as inherited
  summary, not as a current-scope checkbox.

## Parallelism

`sequential`

## Inputs

- Requirement sections 003, 005, 006, and 007.
- System-design sections `Frontend surfaces`, `Idle-session reconfiguration`,
  and `Responsive behavior`.
- Existing `MobilePickerSheet`, task-create mobile dialog, repository card, and
  profile settings patterns.

## Results

- Added the reusable origin-aware MCP selection picker with inherited summaries, current-scope additions, search, disabled rows, and empty states.
- Integrated workspace-contextual profile selection, collapsed repository selection, task-create Advanced selection, new-session selection, and idle-session selection.
- Added explicit workspace propagation through desktop task panels and native mobile bottom-sheet interactions.
- Added localized applied, pending, deferred, and failed session notices with retry behavior.
- Verification passed through focused web tests, lint/typecheck, localization gates, and desktop/mobile task E2E coverage.
