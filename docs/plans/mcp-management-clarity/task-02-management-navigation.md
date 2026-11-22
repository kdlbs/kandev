---
id: "02-management-navigation"
title: "Named management navigation and global-profile workspace picker"
status: done
wave: 2
depends_on:
  - "01-usage-projection"
plan: "plan.md"
requirements:
  - REQ-AGENTS-MCP-MANAGEMENT-001
  - REQ-AGENTS-MCP-MANAGEMENT-002
  - REQ-AGENTS-MCP-MANAGEMENT-004
acceptance_criteria:
  - AC-AGENTS-MCP-MANAGEMENT-001.1
  - AC-AGENTS-MCP-MANAGEMENT-001.2
  - AC-AGENTS-MCP-MANAGEMENT-001.3
  - AC-AGENTS-MCP-MANAGEMENT-001.4
  - AC-AGENTS-MCP-MANAGEMENT-002.1
  - AC-AGENTS-MCP-MANAGEMENT-002.2
  - AC-AGENTS-MCP-MANAGEMENT-002.3
  - AC-AGENTS-MCP-MANAGEMENT-002.4
  - AC-AGENTS-MCP-MANAGEMENT-002.5
  - AC-AGENTS-MCP-MANAGEMENT-004.1
  - AC-AGENTS-MCP-MANAGEMENT-004.2
  - AC-AGENTS-MCP-MANAGEMENT-004.3
system_design:
  - ../../specs/agents/system-design/workspace-mcp-management.md
---

# Task 02: Management navigation

## Summary

Make catalog usage actionable through existing owner destinations. Expose
workspace-contextual catalog selection for saved global profiles without
losing drafts, changing save ownership, or removing legacy recovery.

## In scope

- Used in detail, paged usage hook, validated query entry and owner destinations.
  Reuse the existing catalog route and typed helper proposed by the design.
  Implement the exact [shared owner-navigation contract](../../specs/agents/system-design/workspace-mcp-management.md#shared-owner-navigation-contract),
  including validated `workspaceId`/`repositoryId`, `#mcp` section entry, and
  exact-session admission. Test that each route opens without selection writes.
- Profile capability routing, workspace choice, shared navigation guard,
  context-safe save acceptance, and authoritative baseline handling.
- Safe profile import-status projection and explicit authorized legacy recovery.
  This read does not import or mutate legacy state. Preserve org/global versus
  workspace selection permissions.
- Phone composition, keyboard/focus behavior, seven shipped locale catalogs and
  generated pseudo/Traditional Chinese outputs. Public how-to updates belong
  with the behavior in `agents-and-profiles.md` and `automation-and-mcp.md`.

## Out of scope

Registration assignment, new hubs, task/session creation, session exclusions,
global configuration authorization changes, resolver/import policy changes,
selection CAS redesign, built-in controls, and evidence styling (task 03).

## Acceptance

1. Used in reaches the exact authorized owner on desktop and phone, including
   profile workspace and task session context. Missing/denied targets never
   navigate to a guessed fallback. Empty/error/partial states remain distinct.
2. Global profile workspace A/B changes use shared Save/discard/stay. Failed
   saves and obsolete completions cannot clear drafts, cross contexts, or hide
   active legacy fallback. Other settings contributors retain their ownership.
3. Focused tests prove responsive composition, touch geometry, keyboard focus,
   reload/deep-link recovery, localization, and unchanged additive selection.

## UI preview

See [UI-01 and UI-02](plan.md#ui-preview). Structured control order follows the
task's rich-output instruction instead of ASCII.

UI-01 desktop: configured card -> Used in -> titled dialog -> scope headings ->
named owner links -> continuation/retry. Phone: fixed Back/title -> one full-height
scroll body of full-width owner rows. No nested editor or assignment controls.

UI-02 desktop and phone: MCP heading -> Workspace -> scope sentence -> inline
search and checkbox list. Reuse the repository settings list treatment; the
settings page owns scrolling. No nested selector opener/drawer or catalog link.
Shared Save changes remains outside the list. Dirty switching uses
the existing save/discard/stay guard. Initial empty, load error, denied context,
legacy pending, and failed save states remain explicit.

Apply AC-001.1 through .4, AC-002.1 through .5, and AC-004.1 through .3.
Spacing and sample labels are illustrative. Save ownership, grouping, context,
and one-scroller geometry are required.

## Verification

Use `/tdd`, `/mobile-parity`, and `/e2e`. Run new unit/browser regressions red
before implementation. Dependencies are installed by task 01. Commands start
from repository root and the managed runner rebuilds isolated fixtures.

```bash
(cd apps/backend && go test -trimpath ./internal/agent/settings/handlers -run 'Test.*(ProfileMCP|MCPSelection)' -count=1)
(cd apps/backend && go test -trimpath ./internal/agent/mcpconfig -run 'Test.*(Legacy|Selection)' -count=1)
(cd apps/web && pnpm exec vitest run 'app/settings/agents/[agentId]/profile-mcp-config-card.test.tsx' 'app/settings/agents/[agentId]/profile-mcp-selection-card.test.tsx' components/settings/workspaces/mcp-usage.test.tsx lib/mcp/management-links.test.ts hooks/domains/workspace/use-mcp-usage.test.ts hooks/domains/workspace/use-mcp-selection-editor-context.test.ts components/settings/settings-save-provider.test.tsx src/settings-routes.workspace-data.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:pseudo)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm exec eslint 'app/settings/agents/[agentId]/profile-mcp-config-card.tsx' 'app/settings/agents/[agentId]/profile-mcp-selection-card.tsx' components/settings/workspaces/mcp-settings.tsx components/settings/workspaces/mcp-usage.tsx lib/mcp/management-links.ts hooks/domains/workspace/use-mcp-usage.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/mcp-management.spec.ts tests/settings/workspace-mcp-configuration.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-mcp-management.spec.ts tests/settings/mobile-workspace-mcp-configuration.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run `bash scripts/lint-go-changed` with the actual changed handler/service Go
files after the safe import-status read is implemented. Include new tests for
pending, unknown, complete, denied, and read failure. Do not broaden backend
test scope into runtime packages that this work order does not change.

Browser scenarios cover 320/393/767px phone, 768px desktop, and 900px touch
tablet. Assert actual 44px targets and 28px ordinary desktop controls, contained
scrolling, final-row reachability, no horizontal overflow, translated long
labels, and desktop-phone-desktop draft survival. Capture one desktop and phone
render for the UI comparison. Do not claim mock fixtures verify connectivity.

## Files likely touched

- `apps/web/components/settings/workspaces/mcp-settings.tsx`, `mcp-usage.tsx`
  and `mcp-usage.test.tsx` (new detail presenter/tests).
- `apps/web/lib/mcp/management-links.ts` and `.test.ts` (new),
  `hooks/domains/workspace/use-mcp-usage.ts` and `.test.ts` (new).
- `apps/web/app/settings/agents/[agentId]/profile-mcp-config-card.tsx`,
  `profile-mcp-selection-card.tsx`, and their tests.
- `apps/web/src/settings-routes.workspace-data.tsx`, its tests, profile route
  adapter, and existing repository editor for validated deep-link intent.
- `apps/backend/internal/agent/settings/handlers/mcp_selection_handlers.go`
  and focused tests, with read-only dependency wiring if needed.
- `apps/web/lib/types/http-mcp.ts`, shipped locale files,
  `e2e/tests/settings/{mcp-management,mobile-mcp-management}.spec.ts` (new).
- `docs/public/agents-and-profiles.md`, `docs/public/automation-and-mcp.md`.

## Dependencies

Task 01 and a later explicit implementation request. Re-read registration
sibling scope before changing shared catalog UI. Do not edit its workspace.

## Risks

Late reads, partial shared saves, and route changes can clear a draft unless
identity and revision ownership are explicit. Selection permission differs
from global profile mutation permission. Unknown import readiness must not
hide legacy configuration or change runtime fallback.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/workspace-mcp-management.md).
- [Design](../../specs/agents/system-design/workspace-mcp-management.md), profile,
  destination, responsive, and privacy sections.
- Current `SettingsSaveProvider`, `requestNavigation`, repository selector,
  `ProfileMCPSelectionCard`, and curated mobile exemplars named in the design.
- Root/web/backend guidance and `/docs-maintainer` public-page conventions.

## Results

Implementation completed on 2026-10-25. The original design checkpoint did not
include implementation verification; the following checks ran on this work.

- RED/GREEN covered safe import-status projection, owner-link validation,
  paged usage races, context-owned settings drafts, stored agent-ID profile
  routing, exact-session admission, and focus return from Used in.
- Focused backend profile/selection/legacy tests and affected Go lint passed.
  Frontend helper, hook, settings-save, route, and legacy-recovery regressions
  passed. Full web typecheck passed with an 8 GiB Node heap.
- Affected ESLint/Prettier, seven-language completeness, pseudo/Traditional
  Chinese generation, and the new-copy ratchet passed.
- Managed isolated desktop management and existing selection tests: 9 passed.
  Phone management and existing selection tests: 7 passed. Final management
  scenarios add sparse paging, stale/denied recovery, reload, and invalid hints.
  Desktop: 3 passed. Final phone checkpoint: 3 passed.
- Geometry checks cover 320/393/767px, desktop resizing, and 900px touch;
  full-height usage scrolling, final-row reachability, focus return, 44px touch
  controls, and retained drafts passed. Desktop and phone captures use synthetic
  data; these fixtures do not verify third-party connectivity.
- Public documentation validators (63 tests and 54 pages), specification catalog,
  specification lint, documentation coverage, and whitespace checks passed.

Pure presenter behavior is tested through the actual browser flows rather than
new markup-only component tests. Shared draft logic has focused hook tests.
Assignment owns the optional `mcpServerId` focus hint; this work provides typed
owner routes, validated workspace/repository context, section entry, and exact
session navigation without selection writes.


### Workspace-profile admission correction, 2026-10-25

Assignment integration exposed a remaining admission gap: `GET /agents`
filters workspace profiles, so matching a stored agent ID did not admit a
workspace profile absent from that inventory. The earlier results prove global
profile navigation, not this missing case. Reopened this work order to add the
metadata contract in the design's workspace-profile destination admission
section and mount the existing selector under the exact authorized owner route.

Validate metadata paging, global/exact-workspace inclusion, foreign/deleted
exclusion, disabled-owner identity, sanitized failures, fail-closed dependencies,
read-only configuration handling, response-context races, and no global-store
pollution. Extend the existing desktop/phone management scenarios with a real
workspace-owned profile that is absent from GET /agents, Used in navigation,
manual Save changes, reload and cross-workspace rejection. Retain the existing
global workspace-switch regression. No assignment-specific code is included.

Correction validation completed, 2026-10-25:

- RED/GREEN covered metadata reads, canonical identity, and route admission.
  Backend race tests passed with SQLite and isolated PostgreSQL 17, including
  paging, authorization, safe errors and configuration-free projection.
- Twelve frontend client/profile hook tests passed, including stale-context
  rejection and unchanged global profile behavior. Full web typecheck, affected
  Go/ESLint checks and internationalization validation passed.
- Four desktop and four phone management scenarios passed. They include the
  workspace-owned profile route, manual Save changes, reload, foreign-workspace
  rejection, and existing global-profile switching guards. An initial desktop
  cleanup failure was corrected before the final passing run.
- Catalog/specification lint, work-order coverage and public documentation
  checks passed. Desktop and phone screenshots were inspected with synthetic
  data. These tests do not verify third-party MCP connectivity.

The shared conditional-write path and assignment-owned focus hint are unchanged.
The metadata API, route admission and ownership rationale are recorded in the
existing system design; no separate architectural decision is needed.

### User-tested profile presentation cleanup, 2026-10-25

The user requested removal of the catalog shortcut and duplicate profile picker
chrome after testing the isolated instance. Keep the existing shared list
inline on desktop and phone. Existing workspace-switch guards, save ownership,
legacy recovery and assignment focus hints retain their contracts. Update the
existing global/workspace profile browser flows to exercise directly visible
choices, a single search/description, and absence of the catalog shortcut.

Cleanup validation: the new direct-visibility assertion failed before the UI
change. Final focused global/workspace profile scenarios passed on desktop
(2) and phone (2), including manual save/reload and workspace switch guards.
Full web typecheck, affected ESLint, public docs (63 tests, 54 pages),
specification lint/catalog, work-order coverage and whitespace checks passed.
No new user-facing strings were introduced. Screenshots were inspected, and
the isolated Tailscale instance received the updated web assets with its data
and backend preserved.

### PR review recovery corrections

Reconcile Claude and Greptile findings: distinguish session-list read failures
from confirmed missing owners; preserve existing route recovery for ordinary
errors; keep stale usage links disabled through refresh retry; retain valid
prior pages after continuation failure; preserve paging above responsive
composition; add pointer styling and scope/owner row keys; show legacy recovery
only after accepted selection evidence; separate retryable owner read failures
from deterministic admission failures. Route/hook RED/GREEN and desktop/phone
regressions cover these corrections. Also synchronize usage/draft refs after
commit so abandoned renders cannot change save ownership, and preserve cursor
encoding without requiring `URLSearchParams.size`. Correct the parent's obsolete
global-profile testing note.

RED reproduced the route/usage/owner failures, suspended-render races and cursor
loss without `URLSearchParams.size`. GREEN: 54 focused tests across nine files
passed. Five desktop management scenarios passed before the final nonvisual
ref/transport corrections. Affected ESLint and the normal Go lint wrapper passed.
Final committed-tree desktop/phone runs and current-head remote CI/review state
are required before publication and merge; their exact receipts are external.

The raw-error logging suggestions are not applied: owner readers and database
errors can contain private metadata, and the design forbids logging names,
cursors or response bodies. Generic safe logs and HTTP request metadata remain.
No raw diagnostic text is added to either logging or client responses.
