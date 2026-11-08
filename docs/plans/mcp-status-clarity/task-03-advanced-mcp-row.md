---
id: "03-advanced-mcp-row"
title: "Match MCP selection to Advanced settings"
status: done
wave: 3
depends_on:
  - "02-prove-and-deliver"
plan: "plan.md"
requirements:
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-006
acceptance_criteria:
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.4
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.6
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.8
system_design:
  - ../../specs/agents/system-design/workspace-mcp-configuration.md
---

# Task 03: Match MCP selection to Advanced settings

## Scope and ownership

Task-create Advanced MCP row and its localized help, existing touch-aware help
presenter reuse, desktop/phone task-selection E2E and owned preview assets.
Preserve domain hooks, draft IDs, inherited union and all other picker callers.
Use the parent plan's Advanced settings ASCII preview and mobile contract.

## Acceptance

1. Collapsed Advanced still hides MCP controls. Expanded MCP label, info and
   compact count follow dependency/priority density, without a full-width card
   or permanent explanatory paragraph. Loading uses the same disabled control.
2. Desktop popover and touch drawer reuse selection content. Search, keyboard
   activation, multi-selection, dismissal/focus return and draft retention work.
3. UI task submission persists selected additions; inherited server selections
   remain in their original scopes. Phone controls measure at least 44px,
   desktop selector 28px, and the surface fits without horizontal overflow.

## Verification

From apps/web, use corepack pnpm@9.15.9: typecheck, scoped ESLint, i18n:check,
i18n:ratchet and existing Advanced/priority unit tests. Build the web first.
Run task/task-mcp-selection.spec.ts focused on the new Advanced row journey in
chromium, then task/mobile-task-mcp-selection.spec.ts focused on task additions
in mobile-chrome. Verify read-only loading and light/dark preview layouts,
then deploy only web assets while comparing all four selection scopes and
main/preview process identities. Run docs catalog/lint/whitespace checks.

## Risks and parallelism

Nested overlays must use the parent dialog portal container and preserve
focus/dismissal. Other Advanced controls must retain their current behavior.
Sequential in the primary session; no delegation or commit/push.

## Results (2026-10-09)

Desktop behavioral RED measured the old full-width MCP trigger at 846px against
Priority's 128px. The task-only row now uses the same 128px control width and
28px desktop / 44px phone height. Existing SettingsInfo handles tooltip/touch
help; shared selection content remains responsible for inherited summaries and
task additions. The picker labels additions explicitly rather than This scope.

Final focused desktop and phone task-creation journeys passed, 1/1 each. They
cover help, keyboard or touch selection, focus return, draft retention, and
persisted task additions without changing inherited profile selections. The
existing Advanced/priority unit suites passed all nine tests. Typecheck, scoped
ESLint, formatting, full localization checks, new-copy ratchet, production web
build, whitespace, docs catalog and all-spec lint passed. Public docs validation
passed 62 tests and 47 pages.

Read-only live rehearsals inspected desktop/phone light and dark layouts.
Holding the catalog request proved Loading stays disabled at 128x28. The
desktop popover fits at 384x448; the phone drawer fits a 390x844 viewport with
no horizontal overflow and a 44px close target. Its body scrolls through all
nine choices while the header remains fixed. An initial screenshot was taken
during the drawer animation; captures after settling verify the final layout.

Frontend assets were deployed to the owned 48595 preview, retaining old hashed
assets. Saved selections in all four scopes and main/preview process identities
match before deployment and after rehearsals. No backend restart or preview
data mutation. Evidence under /tmp/kandev-pr3293-test-runtime uses the
advanced-row prefix: desktop/phone-e2e.log, phone-inspect.log,
final-rehearsal.log, deployment.json, post-rehearsal.json,
desktop-options-final.png, phone-options-{top,bottom}.png and check/build logs.
