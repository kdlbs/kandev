---
id: "03-neutral-evidence"
title: "Neutral MCP delivery and unobserved evidence"
status: done
wave: 3
depends_on:
  - "02-management-navigation"
plan: "plan.md"
requirements:
  - REQ-AGENTS-MCP-MANAGEMENT-003
  - REQ-AGENTS-MCP-MANAGEMENT-004
  - REQ-PLATFORM-MCP-SESSION-OBSERVABILITY-001
acceptance_criteria:
  - AC-AGENTS-MCP-MANAGEMENT-003.1
  - AC-AGENTS-MCP-MANAGEMENT-003.2
  - AC-AGENTS-MCP-MANAGEMENT-003.3
  - AC-AGENTS-MCP-MANAGEMENT-004.2
  - AC-AGENTS-MCP-MANAGEMENT-004.3
  - AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.14
  - AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.15
  - AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.16
  - AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.17
system_design:
  - ../../specs/agents/system-design/workspace-mcp-management.md
  - ../../specs/platform/system-design/mcp-session-observability.md
---

# Task 03: Neutral evidence

## Summary

Present delivered and unobserved MCP rows neutrally while preserving stronger
session evidence. Keep saved-selection state independent in every presentation.
Reconcile Platform and public documentation with the resulting behavior.

## In scope

- `buildMcpExplorerServers` configured-only fallback correction and regressions.
- Shared neutral delivery/unknown presentation in list, detail, and tooltip.
  Preserve observed active/connected/failure/filter meanings and tool inspection.
- Simultaneous pending/deferred/failed application and neutral connection rows.
  Preserve current responsive geometry and existing status controller ownership.
- Amend Platform's current delivered-in-amber wording and clarify existing
  AC-001.16 without claiming recovery of the missing archived patch. Update
  public explanations and parent deferred-scope notes by intent.

## Out of scope

New status producers, probes, endpoint tests, runtime reconnects, provider
changes, automatic retries, session exclusions, built-in management, or new
connection-health persistence.

## Acceptance

1. Configured-only rows remain unknown even beside observed servers. Current
   server-specific history wins and older evidence never promotes a new attempt.
2. Delivered/unknown stays neutral across tooltip/list/detail. Pending markers
   derive only from saved selection state. Active and explicit failure evidence
   retain their meaning and localized explanations.
3. Focused desktop/mobile tests and documentation checks pass. The final phone
   row remains reachable, touch title/close geometry stays intact, and there is
   no new save button, conversation bar, probe, or runtime call.

## UI preview

[UI-03](plan.md#ui-03-connection-evidence-and-saved-changes) owns the full preview.
Use structured control order instead of ASCII per the task's output instruction.

Desktop summary: Saved changes with clock and timing -> Current agent -> named
rows with separate evidence. Connections retains list -> server -> tool detail.
Phone: fixed centered title/views -> one body scroller with the same meanings.
Example: Waiting to apply can appear above a neutral Delivered, connection
unverified row and a green Active row. Applying selections clears the clock,
not the evidence qualification. Unknown configuration stays neutral.

Apply AC-003.1 through .3, AC-004.2/.3, and Platform AC-001.14 through .17.
The separation and geometry are required. Sample wording is illustrative.

## Verification

Use `/tdd` for the inference and presentation regressions. Reuse the existing
E2E fixtures and status helpers. Managed runs rebuild their isolated runtime.
Execute from repository root, sequentially after task 02.

```bash
(cd apps/web && pnpm exec vitest run components/task/chat/mcp-explorer/mcp-explorer-view-model.test.ts components/task/chat/mcp-explorer/mcp-status-presentation.test.tsx components/task/chat/mcp-explorer/mcp-server-explorer.test.tsx components/task/chat/mcp-explorer/mcp-selection-presentation.test.ts lib/ws/handlers/session-mcp-status.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:pseudo)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm exec eslint components/task/chat/mcp-explorer/mcp-explorer-view-model.ts components/task/chat/mcp-explorer/mcp-status-presentation.tsx components/task/chat/mcp-explorer/mcp-server-list.tsx components/task/chat/mcp-explorer/mcp-server-detail.tsx)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/mcp-status.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-mcp-status.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Cover delivered/unknown plus every selection application state, old execution
history, observed Active and failure, known/unknown tool catalog, reload, and
context switching. Preserve existing geometry cases at 320/393/767px and touch
tablet. Inspect one desktop and phone render in light/dark and pseudo locale.
Mock evidence is not proof that a third-party endpoint connected.

Before delivery, run PR documentation coverage preflight on the actual changed
work orders and referenced documents through `.github/scripts/pr-docs.cjs`.
Record results in all work orders and the manifest. Promote requirement/design
statuses only after the implementation matches the accepted contract and checks
pass. Do not alter historical test receipts in `merge-readiness.md`.

## Files likely touched

- `apps/web/components/task/chat/mcp-explorer/mcp-explorer-view-model.ts` and
  `.test.ts`, `mcp-status-presentation.tsx` and `.test.tsx` (new presentation tests).
- Same directory: `mcp-server-list.tsx`, `mcp-server-detail.tsx`,
  `mcp-indicator.tsx` only if needed for common presentation wiring.
- `apps/web/e2e/helpers/mcp-status-clarity.ts`, existing desktop/mobile
  `e2e/tests/chat/*mcp-status.spec.ts`, and shipped locales if copy changes.
- `docs/specs/platform/requirements/mcp-session-observability.md` and
  `docs/specs/platform/system-design/mcp-session-observability.md`.
- `docs/public/automation-and-mcp.md`, `docs/public/agents-and-profiles.md`.
- `docs/plans/workspace-mcp-configuration/testing-guide.md` deferred-UI note,
  plus this package's statuses/results. Preserve the historical ledger.

## Dependencies

Task 02. The sequential order is the requested delivery sequence, not a claim
that evidence rendering depends on the new usage endpoint.

## Risks

Neutral colors can conceal explicit failures if applied to every non-active
state. Match exact delivery/unobserved cases. Platform currently specifies
amber for delivered evidence. Amend that source with the UI to avoid two
conflicting authorities. Do not broaden “unobserved” to connected-without-tools.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/workspace-mcp-management.md).
- [Evidence design](../../specs/agents/system-design/workspace-mcp-management.md#evidence-presentation).
- [Platform evidence contract](../../specs/platform/system-design/mcp-session-observability.md).
- Existing status clarity tests, parent testing guide, and `/docs-maintainer`.

## Results

Implementation and verification started on 2026-10-25 after tasks 01 and 02.
The original design checkpoint did not run runtime or browser checks.

- RED: the partial-observation and prior-attempt tests exposed inferred delivery;
  the shared presentation test exposed amber delivery. All three failed for
  the expected behavior before the production changes.
- GREEN: all 45 focused tests passed across the explorer view model, status
  presentation, explorer behavior, selection markers, and WebSocket reducer.
  A final shared-presentation check passed after extracting the neutral class.
- Full web typecheck, affected ESLint, localization completeness, public docs
  validation (63 tests and 54 pages), catalog validation (403 decisions and
  1734 specifications), specification lint (including 36 linter tests), and
  actual-change documentation coverage passed.
- Existing localized copy is reused; no locale key/value changes were needed.
  The generated catalogs from task 02 remain in sync, verified by i18n:check.
- Managed existing explorer scenarios passed: 3 desktop/touch-tablet and 4
  phone. Coverage includes tooltip/list/detail neutral delivery, current Active
  evidence, filtering, pending/deferred/failed/applied states, reload, tools,
  scrolling, and 320/393/767px plus 900px touch geometry.
- Inspected desktop and 393px phone screenshots in light, dark, and pseudo
  locale. Pending notices remain independent; labels wrap within the existing
  surface. Captures contain synthetic data and are uploaded to draft PR #4759.
  These checks do not establish live third-party endpoint connectivity.

Only current configured-only fallback and the shared neutral dot styling change
production behavior. Existing Active, Connected, Failed, filtered reasons,
selection timing, and runtime producers retain their previous contracts.
The missing historical Platform patch was not recovered; current requirements
and design were reconciled by intent. The parent testing guide now records the
implemented global-profile picker; merge-readiness history is unchanged.
