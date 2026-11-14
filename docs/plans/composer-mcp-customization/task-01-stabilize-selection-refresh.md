---
id: "01-stabilize-selection-refresh"
title: "Keep MCP choices stable during refresh"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-003
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-005
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-006
acceptance_criteria:
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-003.8
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-005.1
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-005.5
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.5
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.11
system_design:
  - ../../specs/agents/system-design/workspace-mcp-configuration.md
---

# Task 01: Stable MCP Selection Refresh

## Summary

Stop pending-idle polling from unmounting loaded choices. Distinguish initial
load from refresh and preserve context/save request authority. Deliver this
behavior through the existing bar first so it passes independently of Task 02.

## In scope

The editor's loading/refreshing snapshot lifecycle, context masking, catalog
context reset, and a pending-refresh browser regression. Retain loaded state on
refresh failure with an error surface, and treat successful empty state as ready.

## Out of scope

Moving the entry point, backend lifecycle changes, new caches, global store
redesign, network cadence tuning, and proxy changes.

## Acceptance

1. A deferred reload after successful load keeps selection/origins visible and
   does not set initial loading. Failed refresh retains them and can recover;
   loaded empty IDs/catalog remain ready.
2. Session/workspace changes immediately mask prior context; late old loads and
   saves cannot restore it. Saving during a refresh cannot regress IDs/revisions
   or leave load/saving flags stuck.
3. Through at least three held pending refreshes, the same focused checkbox and
   search input stay mounted, query and scroll stay unchanged, and only the
   initial fetch can show blocking loading.

## Regression sequence

First add a deferred-promise editor test named `retains loaded selection while
a background refresh is unresolved`. Existing `setLoading(true)` must fail it.
Add empty, refresh failure/recovery, inherited-scope/context switch, and
save-overlapping-poll cases. Audit `useMCPWorkspaceDefinitions` for old snapshots
on workspace switch and use a dedicated hook test for the equivalent boundary.

In both task MCP suites, hold selection GET responses at the real API-client
boundary, then return pending three times before terminal state. Capture an
element handle for a checkbox and focused query; assert it stays connected and
focused while each response is held. A timeout snapshot alone can miss flashes.
Distinguish the periodic GET from a selection PUT and count calls per scope;
no save occurs merely because an overlay opens.

## ASCII UI preview

UI-04 from [the combined preview](plan.md#ui-04-refresh-and-error):

```text
First load:       Loading MCP servers...
Later refresh:   [Search: issue........]  keeps focus/query
                 [ ] Issue tracker      remains mounted
Refresh failed:  Selection error
                 [Search: issue........]
                 [ ] Issue tracker      saved state retained
```

Use the current desktop disclosure and phone sheet in this work order. AC-006.11
requires retained rows during refresh; exact notice copy/spacing is illustrative.
Task 02 later embeds the same body in the composer overlay.

## Files likely touched

- `apps/web/hooks/domains/workspace/use-mcp-selection-editor.ts` and its test.
- `apps/web/hooks/domains/workspace/use-mcp-workspace-settings.ts` and a new hook test.
- `apps/web/components/task/task-session-mcp-settings.tsx` and `mcp-session-selector.tsx`.
- `apps/web/e2e/tests/task/task-mcp-selection.spec.ts` and `mobile-task-mcp-selection.spec.ts`.
- Existing catalogs if a distinct refresh-error label is needed.

## Verification

From repository root, run the browser projects sequentially with one worker:

```bash
(cd apps/web && pnpm exec vitest run hooks/domains/workspace/use-mcp-selection-editor.test.ts hooks/domains/workspace/use-mcp-workspace-settings.test.ts components/mcp/mcp-origin-labels.test.ts lib/api/client.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint hooks/domains/workspace/use-mcp-selection-editor.ts hooks/domains/workspace/use-mcp-selection-editor.test.ts hooks/domains/workspace/use-mcp-workspace-settings.ts hooks/domains/workspace/use-mcp-workspace-settings.test.ts hooks/domains/session/use-task-session-mcp-selection.ts components/task/mcp-session-selector.tsx e2e/tests/task/task-mcp-selection.spec.ts e2e/tests/task/mobile-task-mcp-selection.spec.ts --max-warnings 0)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --host --project chromium -- tests/task/task-mcp-selection.spec.ts --retries=0)
(cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome -- tests/task/mobile-task-mcp-selection.spec.ts --retries=0)
git diff --check
```

If new localized copy is introduced, populate all shipped catalogs and run
`i18n:zh-hant` and `i18n:pseudo` before those checks. Add any extra changed files
to targeted lint. Skip-build is valid only without intervening production edits.

## Dependencies and risks

No preceding work order. Preserve existing handled-error semantics and request
guards; using inventory length as readiness mishandles successful empty results.
Sequential work; no delegation.

## Inputs and results

Owning AC-006.11, the design's Composer selection ownership and refresh section,
and the current pending polling reproduction. Task 01 implemented: RED reproduced 5 loading/context failures; 39 unit tests, 4 desktop and 5 phone browser cases passed. Typecheck and localization gates passed. The final extraction keeps the editor within the function-size limit. These regressions were rerun after Task 02 and passed within the final 102-unit, 8-desktop/tablet, and 6-phone checks. See Task 02 for the final deployment evidence; the owned :48593 instance now serves the composer implementation.
