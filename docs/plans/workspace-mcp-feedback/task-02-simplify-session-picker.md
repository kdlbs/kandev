---
id: "02-simplify-session-picker"
title: "Simplify session MCP selection"
status: done
wave: 2
depends_on:
  - "01-repair-rejection-feedback"
plan: "plan.md"
requirements:
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-003
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-006
acceptance_criteria:
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-003.7
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-003.8
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.3
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.4
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.5
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.6
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.9
system_design:
  - ../../specs/agents/system-design/workspace-mcp-configuration.md
---

# Task 02: Simplify Session MCP Selection

## Summary and root cause

Remove the redundant session disclosure and replace UUID origin labels with
contextual names. `MCPSessionSelector` wraps a picker that owns its own trigger;
`originLabel` concatenates a scope label and `owner_id`. Correct their shared
presentation without changing saved selections or effective-set composition.

## Scope and files

- `apps/web/components/mcp/mcp-selection-picker.tsx`: expose the existing
  selection content independently from its trigger; reuse it rather than
  duplicating inherited rows, search, or checkbox behavior.
- `apps/web/components/task/mcp-session-selector.tsx`: one desktop disclosure
  renders that content inline. One phone trigger opens a Drawer directly, with
  a fixed title and one internal scroll owner for application state, error,
  search, inherited origins, and additions. Use existing mobile picker patterns.
- `apps/web/components/task/task-session-mcp-settings.tsx` and shared picker
  consumers: pass contextual name data or share a narrow store projection.
  `agentProfiles.items` provides profile `label`; repositories are keyed by
  workspace and provide `name`. Task/session records supply their names when
  available. Do not add broad fetches or change origin IDs/API persistence.
- Add `apps/web/components/mcp/mcp-origin-labels.ts` and its unit test for
  readable presentation with workspace isolation and localized scope-only
  fallback. Keep translation calls at render time.
- Update `apps/web/e2e/tests/task/task-mcp-selection.spec.ts` and
  `apps/web/e2e/tests/task/mobile-task-mcp-selection.spec.ts` for one action,
  actual profile/repository names, safe fallback, and preserved application state.
- Localize new copy in every shipped locale. Check public MCP documentation
  under `docs/public/` for instructions about session access and update only
  instructions/screenshots made inaccurate by this change.

Inherited entries remain non-removable summaries. Profile/repository/task-create
picker entry points retain their established contextual composition. No new
scope precedence, API fields, marketplace behavior, or built-in tool pruning.

## Acceptance

1. The first session disclosure shows choices on desktop and opens the phone
   sheet directly. There is no second `MCP servers` disclosure. Desired/applied,
   pending, deferred, failed, and retry states remain accessible.
2. Known inherited origins show profile/repository/task/session names in the
   correct workspace. Unknown/deleted or unhydrated names show localized scope
   labels. Internal UUIDs and other-workspace names never appear as fallback.
3. Phone content has one scroll owner, safe-area spacing, 44-pixel targets,
   predictable dismissal/focus return, and no horizontal overflow. Search
   survives closing/reopening the mounted picker; saved selections survive
   desktop-phone-desktop task-layout remounts. Transient search resets on a
   full layout remount. Error behavior from Task 01 remains intact, and shared
   reload recovery closes the phone sheet to expose the recovery control.

## Regression sequence

Write origin-label tests first: known profile and repository, duplicate names
with different IDs, global profile in workspace context, task/session name,
missing record, and an origin from another workspace. Missing-name and isolation
cases must never interpolate the raw identifier.

Change the existing desktop journey `composes profile, repository, task, and
idle-session additions with origins` to expect fixture names instead of UUIDs
and to select after the first session disclosure. Change the phone journey
`opens the idle session MCP sheet and reports applied state` to expect the
sheet after the first tap. Both must fail for the current nested structure
before implementation. Extend the phone fixture with inherited entries so
names are proven there too.

Retain the task-create Advanced settings test, its inherited summaries, and
the rejection/recovery cases from Task 01. Assert viewport containment, one
sheet scroll owner, keyboard search/toggle, application notice, and preserved
saved selection across viewport changes and search across reopening. A
class-name or snapshot assertion alone is
insufficient.

## ASCII UI preview

UI-01 from [the plan](plan.md#ui-01-session-mcp-additions-expanded-desktop-and-phone).
Before, desktop:

```text
[Session MCP additions]
  [MCP servers]
    Profile: <UUID>
```

After, desktop first click:

```text
[Session MCP additions | 0 selected | v]
  State: Applied / Pending / Deferred / Failed [Retry]
  Inherited: Team documentation
             Profile: MCP sandbox agent
             Repository tools
             Repository: MCP sandbox repository
  [Search MCP servers......................]
  [ ] Issue tracker
  [ ] Session extras
```

After, phone first tap:

```text
+-----------------------------------------+
| Session MCP additions            [Close]|  fixed title
|-----------------------------------------|
| State and error                         |  one scroll body
| Inherited summaries with names          |
| Search and session choices              |
+-----------------------------------------+
```

Structure is required; spacing and labels are illustrative. Use existing UI
tokens and translated copy. Do not shrink the desktop disclosure into a phone
accordion. Dismissal returns focus to the original session summary.

## Exact verification commands

From `apps/web`, sequentially:

```bash
pnpm exec vitest run components/mcp/mcp-origin-labels.test.ts lib/api/client.test.ts hooks/domains/workspace/use-mcp-selection-editor.test.ts
pnpm run typecheck
pnpm run i18n:zh-hant
pnpm run i18n:check
pnpm run i18n:ratchet
pnpm e2e:run --host --project chromium -- tests/task/task-mcp-selection.spec.ts
pnpm e2e:run --host --project mobile-chrome -- tests/task/mobile-task-mcp-selection.spec.ts
```

Run targeted ESLint on changed TS/TSX files. The guarded E2E runner rebuilds
production web/backend artifacts; do not skip the build after changes. Rebuild
the isolated test instance and visually check both compositions before reporting
delivery. Keep the main instance at :9998 unchanged.

If public docs change, run their documented validation commands. Update this
package's results and the original package's follow-up link without replacing
historical test counts with new counts.

## Dependencies and results

Task 01 provides correct failure ownership and authoritative selection state.
Sequential work in the primary session; no delegation. Complete: 15 resolver
tests cover names, workspace isolation, missing records, duplicate names, and
localized fallback. The final combined unit run passed 32 tests. Desktop browser
tests passed 3 cases and phone tests passed 4, including one-action entry,
profile/repository/task names, keyboard selection, retained query on reopening,
saved selection after reload/viewport changes, phone containment, focus return,
and ordinary/stale error ownership. The stale recovery case initially exposed
an open sheet covering the global alert; it now closes on handled errors.

Typecheck, targeted lint, localization gates, and public/specification validators
passed. Public session instructions and forwarding troubleshooting were updated.
The tested artifacts are deployed at :48593 with the existing isolated database.
Fresh desktop/phone rendering and forwarded rejection were verified; the latter
now preserves selection and shows only the local error. Main :9998 was untouched.
See the plan's final results for counts and the remaining forwarding limitation.
