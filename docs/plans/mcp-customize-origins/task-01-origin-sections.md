---
id: "01-origin-sections"
title: "Group MCP origins and enrich session choices"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-003
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-006
acceptance_criteria:
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-003.4
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-003.7
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-003.8
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.11
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.14
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.16
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.20
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.21
system_design:
  - ../../specs/agents/system-design/workspace-mcp-configuration.md
---

# Task 01: Group MCP origins and enrich session choices

## Summary

Replace Customize's flat inherited list and origin chips with populated source
sections, retaining one row per server and every contributor. Give session
choice rows the same description and definition metadata as inherited rows.
This is one presentation slice; do not modify selection or runtime ownership.

## In scope

TDD for origin grouping, shared definition detail rendering, session-only picker
composition, accessible rich checkbox rows, translated copy, existing journey
updates, desktop/phone evidence and public selection documentation. After checks,
publish fresh web assets to the guarded isolated preview if it remains running.

## Out of scope

Inherited disabling; controller, API, database or runtime changes; Connections
changes; new save controls; global profile routing; non-session selector redesign;
preview reseeding, main-instance mutation, delegation, commits or pushes.

## Acceptance

1. Populated-only task/repository/profile sections show every inherited server
   once with every contributor explicit, including multi-repository and
   multi-scope cases. Unknown names use safe scope labels; unattributed rows
   remain visible in a generic fallback. Existing order and read-only behavior
   survive application-state refreshes.
2. Additional choices show supplied description and truthful mode/transport/
   catalog-source labels, preserving alias, disabled status and pending actions.
   Metadata is passive; the single accessible checkbox target saves the same
   definition IDs and persists after reload. Inherited definitions are excluded.
3. Desktop and phone retain one scrolling Customize body, with long content
   wrapping, all final choices reachable and phone/coarse-pointer targets at
   least 44px. Existing refresh, rejected save, deferred apply and exact pending
   comparison journeys pass. Public docs and all shipped locales match.

## ASCII UI preview

UI-02: Desktop excerpt, plug > Customize. See the [full plan](plan.md#ascii-ui-preview).

```text
Application status / pending changes
SELECTED FOR THIS TASK [1]
  Review notes
  Description / passive definition metadata
FROM REPOSITORIES [1]
  Atlas documentation
  Description / passive definition metadata
  Repository: Atlas API
  Also selected by
    Agent profile: Mock - Atlas tools
FROM AGENT PROFILE [1]
  Search tools
  Description / passive definition metadata
  Agent profile: Mock - Atlas tools
ADDITIONAL SERVERS
[Search MCP servers]
[ ] Issue tracker
    issue-tracker
    Find and update project issues.
    [Remote endpoint] [Streamable HTTP] [Custom]
```

UI-03: Phone excerpt; fixed title/views, one existing body scroller.

```text
MCP servers                         [Close]
Connections | Customize
-------------------------------------------
Application status / pending changes
FROM REPOSITORIES [1]
Atlas documentation
Shared guides for the infrastructure team.
[Remote endpoint] [Server-sent events]
[Custom]
Repository: Atlas API
Also selected by
  Agent profile: Mock - Atlas tools
ADDITIONAL SERVERS
[Search MCP servers]
[ ] Issue tracker
    issue-tracker
    Find and update project issues.
    [Remote endpoint] [Streamable HTTP]
    [Custom]
```

UI-04 edge states are in the plan: empty origin sections omitted; disabled
choices preserved but unavailable; blank descriptions omitted; unknown sources
visible without invented attribution. AC-006.20/21 govern the section structure
and richer rows; AC-006.11/14/16 preserve refresh, scrolling and pending truth.
Spacing and example copy are illustrative. Keep the explorer drawer geometry,
safe areas and scroll owner; do not add a drawer or per-section scrolling.

## Verification

Run from the repository root. Write the new projection suite first and observe
RED, then implement and run these focused checks. Existing installed workspace
dependencies may be reused; a new worktree needs `pnpm install --frozen-lockfile`
from `apps/` before any package checks.

```bash
(cd apps/web && corepack pnpm@9.15.9 exec vitest run components/mcp/mcp-origin-groups.test.ts components/mcp/mcp-definition-badges.test.ts components/mcp/mcp-origin-labels.test.ts hooks/domains/workspace/use-mcp-selection-editor.test.ts components/task/chat/mcp-explorer/mcp-selection-changes.test.ts)
(cd apps/web && corepack pnpm@9.15.9 run typecheck)
(cd apps/web && corepack pnpm@9.15.9 exec eslint components/mcp/mcp-selection-picker.tsx components/mcp/mcp-inherited-server-details.tsx components/mcp/mcp-origin-groups.ts components/mcp/mcp-origin-sections.tsx components/mcp/mcp-definition-details.tsx e2e/helpers/mcp-inheritance-badges.ts e2e/helpers/mcp-customization-hierarchy.ts e2e/helpers/mcp-customization-scrolling.ts e2e/helpers/mcp-status-clarity.ts e2e/helpers/mcp-origin-sections.ts e2e/tests/task/task-mcp-selection.spec.ts e2e/tests/task/mobile-task-mcp-selection.spec.ts)
(cd apps/web && corepack pnpm@9.15.9 exec prettier --check components/mcp/mcp-selection-picker.tsx components/mcp/mcp-inherited-server-details.tsx components/mcp/mcp-origin-groups.ts components/mcp/mcp-origin-groups.test.ts components/mcp/mcp-origin-sections.tsx components/mcp/mcp-definition-details.tsx e2e/helpers/mcp-inheritance-badges.ts e2e/helpers/mcp-customization-hierarchy.ts e2e/helpers/mcp-customization-scrolling.ts e2e/helpers/mcp-status-clarity.ts e2e/helpers/mcp-origin-sections.ts e2e/tests/task/task-mcp-selection.spec.ts e2e/tests/task/mobile-task-mcp-selection.spec.ts)
(cd apps/web && corepack pnpm@9.15.9 run i18n:check && corepack pnpm@9.15.9 run i18n:ratchet)
GOCACHE=/tmp/kandev-pr3293-test-runtime/pending-go-cache make build-backend
make build-web PNPM="corepack pnpm@9.15.9"
GOCACHE=/tmp/kandev-pr3293-test-runtime/pending-go-cache make -C apps/backend e2e-plugin-package
(cd apps/web && corepack pnpm@9.15.9 e2e:run --host --no-build --project chromium e2e/tests/task/task-mcp-selection.spec.ts e2e/tests/chat/mcp-pending-changes.spec.ts e2e/tests/chat/mcp-status.spec.ts)
(cd apps/web && corepack pnpm@9.15.9 e2e:run --host --no-build --project mobile-chrome e2e/tests/task/mobile-task-mcp-selection.spec.ts e2e/tests/chat/mobile-mcp-pending-changes.spec.ts e2e/tests/chat/mobile-mcp-status.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs/agents docs/plans/mcp-customize-origins apps/web docs/public
```

If extracting additional presentation files, include them and their tests in
the exact check commands before marking done. Locale generation can change
embedded backend locale timestamps, hence native build before managed E2E.
Run suites sequentially. Inspect desktop/phone light/dark captures; include
320px and narrow fine-pointer phone geometry in the shared journey helper.
Run both public-doc validators through `/docs-maintainer` after updating copy:
`node --test scripts/validate-public-docs.test.mjs` and
`node scripts/validate-public-docs.mjs`.

## Files likely touched

- `apps/web/components/mcp/mcp-origin-groups.ts` and its new `.test.ts`.
- `apps/web/components/mcp/mcp-definition-details.tsx` and `mcp-origin-sections.tsx` (new presentational components).
- `apps/web/components/mcp/mcp-selection-picker.tsx` and `mcp-inherited-server-details.tsx`.
- `apps/web/e2e/helpers/mcp-inheritance-badges.ts`, `mcp-customization-hierarchy.ts`
  and the two task selection suites above. Pending-change helper assertions may
  need updating if they name the old inherited heading.
- `apps/web/src/locales/*/settings.json` plus normal pseudo/Traditional Chinese outputs.
- `docs/public/automation-and-mcp.md`, scope selection and composer MCP sections.

## Dependencies

None. Existing catalog, additive inheritance, Customize controller and pending
comparison are already implemented. No new persistent Kandev task is required.

## Risks

Grouping is display placement, not override semantics. Generic selector changes
must not grow Advanced settings. Richer rows can regress the repaired scroll
body; check final-row reachability, not only surface visibility. Missing names
must not reveal raw IDs or owners from another workspace.

## Parallelism

`sequential`

## Inputs

- [Workspace MCP requirements](../../specs/agents/requirements/workspace-mcp-configuration.md), especially 003.4/7/8 and 006.11/14/16/20/21.
- [Workspace MCP design](../../specs/agents/system-design/workspace-mcp-configuration.md#customize-origin-sections).
- [Plan](plan.md), including full previews and companion delivery records.
- Existing origin formatter/metadata mapping and shared selection journey helpers.
- Scoped `apps/web/AGENTS.md`, `/tdd`, `/e2e`, `/mobile-parity` and `/docs-maintainer`.

## Results

Completed on 2026-10-14. Every Verification command passed; source/component
extractions were added to scoped lint/format checks. Vitest: 54 checks across
five suites, with the corrected grouping test subsequently rerun (13 passed).
Typecheck, scoped ESLint/Prettier, i18n check/ratchet, native/web/plugin builds,
catalog/spec lint and whitespace passed. Public-doc validators: 62 tests,
47 pages. Documentation-reference preflight: covered. Desktop E2E: 13 passed;
phone E2E: 10 passed. These are the final full file runs after fixture repairs,
not stale results from an earlier attempt.

Six read-only preview modes passed in light/dark themes, including 320px touch
and 767px fine-pointer phones. Screenshots inspected; final choice reachable,
metadata wrapped/passive, inherited rows unique, empty groups absent and one
scroller retained. Preview assets deployed at :48595 without saved-selection
writes, process restarts or main-instance changes. New requirements remain in
the existing active pair, and the design now describes the delivered UI.

See the [plan results](plan.md#verification-results) for exact logs, fixture
corrections and deployment evidence. No commit, push or delegated work.
