---
created: 2026-09-24
status: complete
requirements:
  - REQ-AGENTS-CURSOR-AUTH-001
  - REQ-AGENTS-CURSOR-AUTH-002
  - REQ-AGENTS-CURSOR-AUTH-003
system_design:
  - ../../specs/agents/system-design/cursor-mcp-oauth-bridge.md
legacy_specs: []
---

# Implementation plan: Cursor MCP OAuth bridge

## Overview

Implement the filesystem helper first, then the profile preference and settings control.
Connect both to launch preparation last.
All work orders run sequentially in the primary session.
This package does not authorize implementation or subagents.

Sources: [requirements](../../specs/agents/requirements/cursor-mcp-oauth-bridge.md) and
[system design](../../specs/agents/system-design/cursor-mcp-oauth-bridge.md).

## Scope

In scope: local Cursor ACP and CursorStrategy terminal launches, aggregation, atomic links, profile persistence, localized checkbox, and desktop/phone verification.
Out of scope: OAuth login, token refresh, remote credential transport, per-server account selection, and background synchronization.

## Technical approach

- Add `mcpconfig/cursor_auth_bridge.go` with pure slug derivation and composed filesystem operations.
- Add `cursor_mcp_auth_enabled` to the existing profile store and shared API contract. Omission defaults to true on creation.
- Carry the value through `AgentProfileInfo`, frontend normalization, draft state, and settings discovery.
- Add a checkbox in `ProfileFormFields` or a focused child component. Use the current effective strategy to determine visibility.
- Add lifecycle preparation before ACP and terminal process launch. Use resolved execution-profile state, not the stable Office identity.
- Update `docs/public/agents-and-profiles.md` with local eligibility, default, conflicts, disable timing, and preservation of regular auth files.

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

## Tests

Proposed test names are implementation targets, not existing evidence.

| Criteria | Evidence |
| --- | --- |
| 001.1, .2, .6; 003.1, .5, .6 | `cursor_auth_bridge_test.go`: `TestAggregateCursorMCPAuth` table cases |
| 001.4; 003.2, .3, .4 | Same file: `TestDeriveCursorProjectSlug`, `TestLinkCursorMCPAuth`, `TestCursorMCPAuthConcurrentPreparation` |
| 002.3, .4 | Same file: `TestDisableCursorMCPAuth`; lifecycle cleanup failure test |
| 002.1, .7 | Store migration/round-trip, controller create/patch/duplicate, DTO and settings catalog tests |
| 002.2, .5, .6 | Frontend form/state tests and both Playwright files below |
| 001.3, .5; 003.7 | `manager_project_mcp_test.go` and `manager_passthrough_mcpfiles_test.go`: eligibility, empty servers, errors, both launch modes |
| 001.1, .4 | `TestCursorMCPAuthWorktree`: temporary git worktree and synthetic home |

Each work order requires RED-GREEN-REFACTOR for changed logic.
Production helpers use Go only. Test fixture setup can invoke git to create a real temporary worktree.

## E2E tests

Add `apps/web/e2e/tests/settings/cursor-mcp-auth.spec.ts` for the `chromium` project.
Add `apps/web/e2e/tests/settings/mobile-cursor-mcp-auth.spec.ts` for `mobile-chrome`.
Use shared test helpers for disposable Cursor profile setup.

Prove default enabled, disable/save/reload, re-enable, discard, and hiding for non-Cursor profiles.
Also cover the custom terminal profile with strategy `cursor`.
Assert an accessible checkbox name and keyboard Space on desktop.
On phone, tap the label, measure its target, and assert no horizontal document overflow.
These flows cover AC-AGENTS-CURSOR-AUTH-002.1, .2, .5, and .6.
Use causal network/event waits and backend fixtures. Do not authenticate real MCP providers.

## Work orders

- [x] [Task 01: Safe Cursor auth helpers](task-01-auth-helpers.md)
- [x] [Task 02: Profile preference and settings checkbox](task-02-profile-preference.md)
- [x] [Task 03: Launch integration and delivery verification](task-03-launch-integration.md)

## Verification results

Design validation on 2026-09-24: catalog validation passed, all specification files passed lint, and 36 specification-linter tests passed.
Relative document links and whitespace checks passed for all six new documents.

Implementation completed on 2026-09-25. `make -C apps/backend build`, focused MCP bridge and lifecycle tests, lifecycle race tests, focused frontend tests (104 tests), TypeScript, i18n, desktop E2E (2 tests), mobile E2E (1 test), documentation validation, specification validation, and `git diff --check` passed.

The requested `make -C apps/backend test` target ran but did not pass. The process-probe package's two real-tree timing tests also fail in isolation. Common-config and launcher package tests pass in isolation after clearing the host-injected `KANDEV_INTERNAL_CONFIG_FILE` and `KANDEV_INTERNAL_CONFIG_HOME_FILE` values; those values made the broad run load `/root/.kandev/config.yaml` during tests that expect temporary configuration. The broad suite was not rerun during review follow-up.

Review follow-up passed the full focused MCP bridge and lifecycle suites, their race variants, the backend build, public-doc tests/validation, and whitespace/format checks. Terminal profile resolution now supplies the agent and preference from one successful lookup, and eligible local Cursor launches fail closed on missing profile data. Tests canonicalize workspace paths and cover a workspace under a symlinked parent. Public docs now distinguish symlink replacement while sharing is enabled from preservation of unrelated symlinks during opt-out cleanup.

## Risks

- Source modification time is deterministic but does not prove token validity.
- Different profiles can share one canonical workspace and therefore one auth link.
- Cursor can update the master or replace its symlink. Regular files remain protected.
- The bridge cannot coordinate token refresh with external processes.
- A non-default process HOME or remote executor cannot safely use the backend's host credentials.
- Filesystem rename behavior and symlink permission differ across operating systems.
- The supplied Cursor filesystem contract requires real-version compatibility evidence before claiming live OAuth success.
- No automatic backend restart or global runtime release flag is part of this request.
