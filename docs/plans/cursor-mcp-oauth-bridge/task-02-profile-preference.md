---
id: "02-profile-preference"
title: "Profile preference and settings checkbox"
status: done
wave: 2
depends_on:
  - "01-auth-helpers"
plan: "plan.md"
requirements:
  - REQ-AGENTS-CURSOR-AUTH-002
acceptance_criteria:
  - AC-AGENTS-CURSOR-AUTH-002.1
  - AC-AGENTS-CURSOR-AUTH-002.2
  - AC-AGENTS-CURSOR-AUTH-002.5
  - AC-AGENTS-CURSOR-AUTH-002.6
  - AC-AGENTS-CURSOR-AUTH-002.7
system_design:
  - ../../specs/agents/system-design/cursor-mcp-oauth-bridge.md
---

# Profile preference and settings checkbox

## Summary

Persist the default-enabled preference and expose one localized checkbox through the existing settings flow.

## Scope and owned files

- `apps/backend/internal/agent/settings/models/models.go`.
- `settings/store/sqlite.go` and migration/duplicate/round-trip tests.
- `settings/dto/profile_contract.go`, `dto.go`, controller CRUD and conversion files, and their tests.
- `apps/backend/internal/settingscatalog/defaults.go` and catalog contract tests.
- `apps/backend/internal/agent/runtime/lifecycle/types.go` and `profile_resolver.go` for the resolved preference.
- Web `lib/types/agent-profile.ts`, `lib/types/backend.ts`, `lib/api/domains/agent-profile-normalize.ts`, and profile mutation types.
- Web `components/settings/profile-form-fields.tsx`, `agent-profile-page.tsx`, `agent-profile-page-state.ts`, `agent-profile-dirty.ts`, and `agent-profile-reconciliation.ts`.
- Web `app/settings/agents/[agentId]/agent-save-helpers.ts` and creation flow defaults.
- Relevant tests and `src/locales/{en,pt-pt,zh-cn,zh-tw,zh-hk,ja}/agents.json`.
- New desktop/phone E2E files from the plan and their shared setup helper.

Follow existing field mappings into boot payloads, actions, export/import contracts, and event normalization where they explicitly enumerate profile fields.
Preserve false through every supported profile round trip.
Exclude launch filesystem mutations and changes to generic permission behavior.

## Implementation acceptance

1. Migration and all creation paths default to true. Explicit false survives update, duplication, reload, and unrelated edits.
2. Applicable profiles expose UI-01 with normal save/discard and conflict behavior. Other profiles do not expose the checkbox.
3. Desktop and phone tests prove persistence, accessibility, touch size, and localization completeness.

## ASCII UI preview

### UI-01: Cursor profile settings

Entry: Settings > Agents > Cursor profile. Shared desktop/phone composition, enabled state.

```text
Profile settings
  ...existing fields...
  [x] Share local Cursor MCP credentials
      Reuse MCP sign-ins from other local
      Cursor projects. Applies on the next launch.

Existing settings save controls:
  [Discard]  [Save changes]
```

Unchecked draft: `[ ]`, with the same save controls.
After save, unchecked persists across reload.
Loading and failed-save behavior use the existing settings surface.
The description of disable timing stays visible without hover.

Required structure: labeled checkbox, visible helper text, existing save/discard flow.
Phone: wrap text, make the label target at least 44px high, and retain the existing page scroll owner.
Desktop: retain current form density.
Copy and spacing are illustrative. Localization supplies final copy.
Maps to AC-AGENTS-CURSOR-AUTH-002.2, .5, and .6.

Full context: [plan preview](plan.md#ascii-ui-preview).

## TDD and verification

Write failing backend tests for omitted create, explicit false, omitted patch, migration, and duplicate.
Write frontend tests for normalization, dirty detection, patch conversion, and external update reconciliation.
Extend existing tests where the behavior already has a suite.

From `apps/backend`:

```bash
go test ./internal/agent/settings/... ./internal/settingscatalog/...
go test ./internal/agent/runtime/lifecycle/... -run TestProfileResolver
```

From `apps/web`:

```bash
pnpm exec vitest run components/settings/profile-form-fields.test.tsx components/settings/agent-profile-page-state.test.ts components/settings/agent-profile-reconciliation.test.ts lib/api/domains/agent-profile-normalize.test.ts
pnpm run typecheck
pnpm run i18n:zh-hant
pnpm run i18n:check
pnpm e2e:run --host --project chromium -- tests/settings/cursor-mcp-auth.spec.ts
pnpm e2e:run --host --project mobile-chrome -- tests/settings/mobile-cursor-mcp-auth.spec.ts
```

The managed E2E runner rebuilds production assets.
Use the standard fixture mock agents or an existing custom terminal fixture with Cursor strategy.
Keep the fixture independent of a real Cursor install.
Record screenshots or a rendered inspection for the phone checkbox.

## Dependencies and risks

Depends on Task 01 for agreed helper policy.
Omitted booleans and explicit false must remain distinct in create and patch requests.
Reconciliation and duplicated profiles can silently lose a field if only the main editor changes.

## Results

Implemented the profile preference through SQLite, DTO/controller contracts, runtime profile resolution, frontend drafts, save paths, localization, and the settings checkbox. The checkbox appears for Cursor ACP and custom Cursor-strategy agents, defaults on, and uses the normal reset/save and external-conflict flow.

Validation passed:

- `go test ./internal/agent/settings/... ./internal/settingscatalog/...`
- `go test ./internal/agent/runtime/lifecycle/... -run TestProfileResolver` (the pattern matches no tests; full lifecycle tests remain in Task 03)
- Focused Vitest suites for form rendering, reconciliation, normalization, page save, and agent save helpers.
- `pnpm run typecheck`
- `pnpm run i18n:check`
- Chromium settings E2E: 2 passed, including persistence and non-Cursor visibility.
- Mobile Chrome settings E2E: 1 passed, including touch-size and overflow checks.

`pnpm run i18n:zh-hant` is blocked by the existing simplified-string warning at `workflows:openAgentSettings`. The supported namespace-scoped generator wrote the `agents` Traditional Chinese catalogs from Simplified Chinese without reformatting other translated messages.
