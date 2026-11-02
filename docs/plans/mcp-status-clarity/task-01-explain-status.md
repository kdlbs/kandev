---
id: "01-explain-status"
title: "Explain saved changes and current evidence"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-MCP-SESSION-OBSERVABILITY-001
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-006
acceptance_criteria:
  - AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.13
  - AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.14
  - AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.15
  - AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.16
  - AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.17
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.10
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.11
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.13
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.14
system_design:
  - ../../specs/platform/system-design/mcp-session-observability.md
  - ../../specs/agents/system-design/workspace-mcp-configuration.md
---

# Task 01: Explain saved changes and current evidence

## Summary

Use one shared presenter to turn delivery evidence into understandable status
and reasons. Separate the saved-change notice from current agent rows in the
existing hover/dialog/drawer, without changing state authority or timing.

## In scope

- Inventory actual reason-code producers and add localized safe mappings.
- Status explanations, saved/current grouping, and accessible phone parity.
- Distinct Customize groups and restrained icon/color application summaries.
- TDD for mixed evidence and every application-state notice.

## Out of scope

Provider capabilities, health probes, persistence changes, missing-command
resolution, fake evidence, and another transcript bar.

## Acceptance

1. Regression tests fail on the absent exclusion reason and ambiguous saved/current
   grouping before the minimal correction, then pass using one shared presenter.
2. Pending/deferred/failed/applied copy agrees with saved application state;
   Delivered and Active remain distinct. Active ignores an obsolete filter reason.
3. Unknown reasons, long names and translations wrap; the original controller,
   polling/focus/search guards and phone drawer composition remain intact.
   Customize groups have semantic headings and stronger separation, with less
   per-row decoration. Color is supplemental to consequence text and icons.

## ASCII UI preview

UI-02 from [the combined preview](plan.md#ascii-ui-preview):

```text
Saved changes: Saved, not applied yet.
Current agent
  kandev         Active          Tools loaded.
  issue-tracker  Not available   HTTP not supported by this agent.
  atlas-docs     Delivered       Connection not verified.
```

UI-03 phone: tapping the plug opens the existing full-height Connections/Customize
drawer; the same saved-change grouping and row reasons are visible without
hover. Fixed header, one scrolling body, safe area and 44px phone targets.
AC-001.13 through .16 govern these views.

UI-06 from [the full Customize preview](plan.md#ascii-ui-preview):

```text
(check) Setup up to date
        Your saved setup has been applied to this agent.

[ FROM TASK SETUP ]
  Atlas shared documentation
    Profile: Atlas tools / Repository: Atlas API

ADDITIONAL SERVERS
[Search servers]
[x] Atlas review notes
```

UI-07 phone uses wrapped origin lines and the same groups in the one scroll
body, with a fixed drawer header/view selector. Muted check/green, clock/amber,
next-start/blue and alert/red summaries explain now/next; color alone conveys
nothing. Applied does not claim all tools work. Platform .17 and agents .14
govern these additions. Inherited rows stay read-only pending a separate local
exclusion contract; no fake working toggle.

## Verification

Run from repository root. Use the package manager pinned in apps/package.json.

```bash
(cd apps/web && corepack pnpm@9.15.9 exec vitest run components/task/chat/mcp-explorer/mcp-explorer-view-model.test.ts components/task/chat/mcp-explorer/mcp-selection-presentation.test.ts components/task/chat/mcp-explorer/mcp-server-explorer.test.tsx)
(cd apps/web && corepack pnpm@9.15.9 run typecheck)
(cd apps/web && corepack pnpm@9.15.9 exec eslint components/task/chat/mcp-explorer components/task/mcp-session-selector.tsx components/mcp/mcp-selection-picker.tsx)
(cd apps/web && corepack pnpm@9.15.9 run i18n:check)
(cd apps/web && corepack pnpm@9.15.9 run i18n:ratchet)
git diff --check
```

After adding translated copy, use `i18n:zh-hant` and the existing pseudo generator for
generated catalogs. Do not accept unrelated normalization churn.

## Files likely touched

- `apps/web/components/task/chat/mcp-explorer/mcp-explorer-view-model.ts` and test.
- `mcp-status-presentation.tsx`, `mcp-server-detail.tsx`, `mcp-server-list.tsx` in that directory.
- `mcp-selection-presentation.ts`, explorer application notice, and component tests.
- Relevant `apps/web/src/locales/*/task.json` and `settings.json` catalogs.
- `apps/web/components/task/mcp-session-selector.tsx` and the narrow session
  presentation path in `apps/web/components/mcp/mcp-selection-picker.tsx`.

## Dependencies

None. Read the existing completed composer and forwarding repair packages;
preserve their controller and authentication behavior.

## Risks

Stale reason codes on stronger evidence; unsupported reason-code assumptions;
copy accidentally promising a verified third-party connection.

## Parallelism

`sequential`

## Inputs

- Platform requirement .13 through .17 and paired plain-language presentation design.
- Agents responsive controls .9 through .14 and composer selection design.
- Live evidence and compatibility matrix in the parent plan.

## Results

Completed the parent plan's inherited metadata badge refinement. Ownership:
new definition-label helper/tests and inherited details renderer under
`components/mcp/`, plus the existing session presentation path. Generic picker
callers keep their original plain origin labels. No new copy keys are required.
Verification includes definition-badge and origin-label unit tests, typecheck,
scoped lint and translation ratchet, followed by Task 02 browser coverage.
Final metadata/origin tests: 27 passed. Web build, typecheck, warning-free lint,
formatting, i18n ratchet and whitespace passed. The public how-to now describes
configuration badges separately from connection evidence.

Completed the parent plan's dialog hierarchy revision. Applied external
section headings, bounded/inset server lists, compact Applied summary, dialog
title/tab hierarchy and wrapped metadata without changing status meanings or
selection state ownership. Final focused unit checks: 40/40; typecheck,
warning-free scoped ESLint, formatting and web build passed. No new copy.

Implemented on 2026-10-07. Behavioral RED reproduced missing transport
explanations and saved/current grouping; GREEN passed. A further RED protected
sanitized failure details while stale Active transport reasons remain ignored.
Final focused Vitest command: 3 files, 40 tests passed. Typecheck, targeted
ESLint (including session body/picker), i18n completeness and new-code ratchet,
and whitespace checks passed. All seven shipped locales updated; generated
Traditional Chinese and pseudo catalogs retained only related changes.

Shared explanation presenter is used by hover, server list and detail. Shared
application notice supplies semantic icons, restrained accents and now/next
copy. Customize groups are scoped through the session presentation option;
generic picker callers, polling, save/retry ownership and state authority remain
unchanged. Desktop and phone browser verification continues in Task 02.

Completed the parent plan's Customize scroll/source-heading repair. The
coarse-pointer drawer has a definite height and inherited rows have an explicit
translated source subheading. Tablet RED and final desktop/tablet/phone long-list
journeys are documented in the parent plan. Source badges retain resolved names
and every contributing scope; saved selections and process identities were
preserved through frontend-only deployment on 48595.


## Proposed origin-section continuation (2026-10-14)

The [Customize origin sections package](../mcp-customize-origins/plan.md)
proposes replacing the inherited origin-chip presentation and enriching session
choice rows. This record's delivered status, historical previews and checks
remain unchanged. The continuation is not implemented; its work order owns the
new composition and regression evidence. Application and pending-change
semantics from this delivery remain in force.
