---
id: "06-build-workspace-mcp-settings"
title: "Build workspace MCP settings"
status: done
wave: 3
depends_on:
  - "02-integrate-public-mcp-registry"
plan: "plan.md"
requirements:
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-001
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-002
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-006
acceptance_criteria:
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-001.1
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-001.2
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-001.4
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-001.5
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-001.7
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-001.8
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.1
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.2
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.3
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.4
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.6
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.7
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.8
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.9
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.10
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.11
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.1
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.3
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.4
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.5
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.6
system_design:
  - ../../specs/agents/system-design/workspace-mcp-configuration.md
---

# Task 06: Build Workspace MCP Settings

## Summary

Add the dedicated workspace MCP settings destination with configured and
marketplace experiences. Deliver a desktop composition and a native phone flow
that share typed state without sharing an unsuitable layout.

## In scope

- Add `MCP servers` to the workspace settings tab manifest and shell. Name and
  describe it so it is not confused with the existing External MCP page, which
  exposes Kandev's own MCP server to outside clients.
- Add typed catalog, marketplace, refresh, install, disable, and delete API
  clients.
- Build configured-server list, custom definition form, marketplace search,
  source/status labels, review, setup, stale/degraded state, and impact
  confirmation.
- Present remote, managed npm, and existing executable as distinct setup modes.
  Explain lazy materialization and unsupported Registry package types.
- Use a desktop list or split view and a phone single-column list.
- Use a direct full-height phone route or surface for setup, with safe-area
  padding and one scroll owner.
- Add keyboard labels, visible focus, 44-pixel controls, loading, empty, error,
  stale, and conflict states.
- Add all locale keys. Generate both Traditional Chinese catalogs from the
  Simplified Chinese source.

## Out of scope

- Profile, repository, task, or session selection controls.
- Runtime delivery and applied-state presentation.
- Playwright journey coverage, which Task 08 owns.

## Acceptance

- An authorized user can complete custom and marketplace setup, edit or disable
  a definition, and confirm an impacted delete without raw JSON.
- Setup states whether Kandev installs nothing, materializes an exact npm
  package on first use, or expects an executable in the task executor.
- Phone setup uses a full-height, single-scroll composition with no horizontal
  overflow and no compressed desktop pane.
- Marketplace trust, stale, degraded, deprecated, deleted, and conflict states
  remain localized and accessible.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps/web && pnpm exec vitest run lib/api/domains/mcp-api.test.ts components/settings/workspaces/mcp-settings.test.tsx components/settings/workspaces/mcp-marketplace.test.tsx
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run i18n:check
```

Write API and component interaction tests first. The initial tests must fail
because no workspace MCP route or typed client exists.

## Files likely touched

- `apps/web/lib/settings/workspace-settings-tabs.ts`
- `apps/web/components/settings/workspaces/workspace-settings-shell.tsx`
- `apps/web/app/settings/workspace/[id]/mcp/page.tsx`
- `apps/web/components/settings/workspaces/mcp-settings.tsx`
- `apps/web/components/settings/workspaces/mcp-settings.test.tsx`
- `apps/web/components/settings/workspaces/mcp-marketplace.tsx`
- `apps/web/components/settings/workspaces/mcp-marketplace.test.tsx`
- `apps/web/components/settings/workspaces/mcp-definition-form.tsx`
- `apps/web/lib/api/domains/mcp-api.ts`
- `apps/web/lib/api/domains/mcp-api.test.ts`
- `apps/web/lib/types/http-mcp.ts`
- `apps/web/src/locales/en/settings.json`
- `apps/web/src/locales/pt-pt/settings.json`
- `apps/web/src/locales/zh-cn/settings.json`
- `apps/web/src/locales/zh-hk/settings.json`
- `apps/web/src/locales/zh-tw/settings.json`

## Dependencies

- Task 02 supplies catalog, marketplace, refresh, and review-install APIs.

## Risks

- The settings shell already owns mobile scrolling and tab navigation. New
  screens must not introduce a competing page scroll owner.
- Registry descriptions are untrusted text. Do not render arbitrary HTML.
- Deep setup forms need route-aware cancel and back behavior on phones.

## Parallelism

`sequential`

## Inputs

- Requirement sections 001, 002, and 006.
- System-design sections `Frontend surfaces`, `Marketplace installation`, and
  `Responsive behavior`.
- Existing workspace settings shell and mobile full-height form patterns.
- Mobile parity skill constraints and repository i18n rules.

## Results

- Added the dedicated workspace MCP servers route, configured catalog view, custom setup form, marketplace search/review/install flow, and guarded deletion impact UI.
- Added remote, managed npm, and existing executable setup modes with lazy-materialization and unsupported-choice explanations.
- Added loading, stale/degraded, empty, conflict, failure, and mobile full-height setup states with localized copy.
- Verification passed through web lint/typecheck, localization gates, public-doc validation, and desktop/mobile settings E2E coverage.
