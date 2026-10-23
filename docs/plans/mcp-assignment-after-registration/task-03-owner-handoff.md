---
id: "03-owner-handoff"
title: "Connect optional MCP assignment to existing owner editors"
status: pending
wave: 3
depends_on:
  - "02-registration-completion"
plan: "plan.md"
requirements:
  - REQ-AGENTS-MCP-ASSIGNMENT-001
  - REQ-AGENTS-MCP-ASSIGNMENT-002
  - REQ-AGENTS-MCP-ASSIGNMENT-003
  - REQ-AGENTS-MCP-ASSIGNMENT-004
acceptance_criteria:
  - AC-AGENTS-MCP-ASSIGNMENT-001.1
  - AC-AGENTS-MCP-ASSIGNMENT-001.2
  - AC-AGENTS-MCP-ASSIGNMENT-001.3
  - AC-AGENTS-MCP-ASSIGNMENT-001.4
  - AC-AGENTS-MCP-ASSIGNMENT-002.1
  - AC-AGENTS-MCP-ASSIGNMENT-002.2
  - AC-AGENTS-MCP-ASSIGNMENT-002.3
  - AC-AGENTS-MCP-ASSIGNMENT-002.4
  - AC-AGENTS-MCP-ASSIGNMENT-002.5
  - AC-AGENTS-MCP-ASSIGNMENT-002.6
  - AC-AGENTS-MCP-ASSIGNMENT-003.1
  - AC-AGENTS-MCP-ASSIGNMENT-003.2
  - AC-AGENTS-MCP-ASSIGNMENT-003.3
  - AC-AGENTS-MCP-ASSIGNMENT-003.4
  - AC-AGENTS-MCP-ASSIGNMENT-004.1
  - AC-AGENTS-MCP-ASSIGNMENT-004.2
  - AC-AGENTS-MCP-ASSIGNMENT-004.3
  - AC-AGENTS-MCP-ASSIGNMENT-004.4
system_design:
  - ../../specs/agents/system-design/workspace-mcp-assignment.md
---

# Task 03: Optional owner handoff

## Summary

Complete Use in on the registration receipt and existing catalog cards. Discover
authorized owners and hand off to existing editors through the management
sibling's committed navigation contract. Prove recovery and publish usage docs.

## In scope

- Read-only owner choice, scope consequences, existing-definition highlighting.
- Repository, workspace/global profile with explicit workspace, and exact
  existing task agent session; no default or implicit selection.
- Failure/eligibility/uncertain-save recovery and context generation guards.
- Desktop and phone composition, locales, focused E2E, and public docs.

## Out of scope

Management hub/Used in/global-profile implementation, session inherited
exclusions, built-in management, task-level or bulk assignment, owner creation,
endpoint probes, and runtime reconnection changes.

## Acceptance

1. All three owner paths use existing editors and their save contracts. Mere
   discovery/navigation changes no selection and starts no agent. Global
   profiles retain the explicit registering workspace; session links target
   the named existing sibling, not a default or newly ensured session.
2. Failed/uncertain assignment, conflict, disabled/removed targets, and access
   loss retain the saved definition and offer actionable recovery. Retry never
   calls create. Existing direct/inherited choices remain intact and no stale
   result crosses contexts. Provider timing and prior accepted setup remain intact.
3. Phone and desktop pass the plan's scenario matrix with measured geometry,
   keyboard/focus/resize and localized wrapping. Public docs describe actual
   shipped behavior, saved-only completion and partial-success recovery.

## UI preview

UI-01 adds Use in beside Done on the saved receipt and to catalog cards.
UI-02 and UI-03 from [full previews](plan.md#ui-02-owner-choice-and-consequence-review):

| Region | Desktop dialog | Phone Drawer |
| --- | --- | --- |
| Header | Use Atlas docs in / Acme / Close | Fixed, contained close |
| Body | Owner type, then search/results, then consequences | One focused view per step; one scroll owner |
| Footer | Back / Open MCP settings | Same controls with safe-area and 44px targets |
| Failure | Saved remains visible; retry read or choose another owner | Same recovery without nested overlays |

Settings reveal the MCP section and can highlight the available definition
without changing its existing checkbox state or search. Session navigation opens
only the conversation; its hint waits until the user opens plug/Customize.
Existing settings Save changes or explicit session selection autosave owns commit. Maps to every criterion listed in this work order.

## Verification

Run TDD before implementation. From root, after work orders 01/02 and the
committed sibling prerequisite are integrated:

```bash
(cd apps/web && pnpm exec vitest run hooks/domains/workspace/use-mcp-registration-completion.test.ts hooks/domains/workspace/use-mcp-assignment-targets.test.ts components/mcp/mcp-assignment-navigation.test.ts hooks/domains/workspace/use-mcp-selection-editor.test.ts hooks/domains/workspace/use-mcp-selection-editor-context.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/mcp-assignment.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-mcp-assignment.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run targeted ESLint with all actual changed TS/TSX paths and record the expanded
command. Do not add a broad lint/test audit. Run E2E sequentially, with managed
fresh builds; inspect intended project discovery and one phone render. Tests
must assert persisted membership, no duplicate definition, and forbidden-request
absence, not only button visibility. Use disposable fixtures and mock providers.
No real-provider certification is claimed by these tests.

## Files likely touched

- `apps/web/components/settings/workspaces/mcp-settings.tsx` and completion component.
- New `mcp-assignment-chooser.tsx` and domain hook
  `hooks/domains/workspace/use-mcp-assignment-targets.ts`, with focused tests.
- `apps/web/components/mcp/mcp-assignment-navigation.test.ts` against the
  sibling's committed destination builder. Modify the builder only by the
  reviewed integration contract, not by copying a competing implementation.
- Existing repository/profile destination consumers and composer Customize
  focusing hook as needed; exact files follow the committed sibling contract.
- `apps/web/e2e/helpers/mcp-assignment.ts`, desktop/phone assignment specs and
  affected locale catalogs.
- `docs/public/agents-and-profiles.md`, `docs/public/automation-and-mcp.md`;
  the owning requirements/design and this work package for actual results.

## Dependencies

Work orders 01 and 02. Also management task
`000d477e-f792-4c9d-8503-50ed73bb9646`, planned
`docs/plans/mcp-management-clarity/task-02-management-navigation.md`.
Before coding, read its reviewed committed requirements/design/work order,
record the integrated SHA and actual route/owner-link symbols here, and reconcile
this package. If it is unavailable, stop this work order with the concrete
dependency. Do not implement the sibling feature or silently omit global profiles.

The management draft records `MCPOwnerDestination` and
`apps/web/lib/mcp/management-links.ts`. Consume its exact routes and required
identities from the [shared integration contract](../../specs/agents/system-design/workspace-mcp-assignment.md#management-sibling-integration).
Management owns base routes, validation, MCP section reveal and global-profile
context. This work order extends the builder with optional `definitionId` and
reserved `mcpServerId` through `URLSearchParams`, preserving query and hash.
Implement the focus-only parser and destination lifecycle here. It must validate
same-workspace availability in existing inventory, reject duplicate/malformed
hints, leave search and drafts unchanged, consume each hint once, and wait for
explicit user opening of session Customize. It must never open an overlay or
broaden inventory. Add parser/lifecycle tests plus desktop/phone coverage for
unknown, removed, disabled, cross-workspace and filtered-out definitions, delayed
loading, context changes, existing selected state, reopen, rerender and reload.
The catalog's `serverId`/`view=usage` route is optional recovery, not an assignment
destination. No definition hint is a selection command.

When exclusions is integrated, test that addition-only writes omit exclusions,
preserve an existing excluded ID, and use the same session revision. Do not
claim an excluded server is effectively included or restore it automatically.

## Risks

Two independent owner-link builders, cross-workspace global-profile defaults,
opening a removed session and triggering ensure, owner-list overfetch,
save-contributor draft replacement, locale conflicts, and catalog receipt
being mistaken for runtime success.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/workspace-mcp-assignment.md)
- [Owner handoff and sibling boundary](../../specs/agents/system-design/workspace-mcp-assignment.md#owner-handoff)
- [Parent testing guide](../workspace-mcp-configuration/testing-guide.md),
  interpreted as historical scope/evidence only.
- Existing picker, settings-save and composer E2E patterns; `/tdd`, `/e2e`,
  `/mobile-parity`, `better-ui`, and `/docs-maintainer`.

## Results

Pending. On 2026-10-23, management's plan remained at
`40b577c3-0d95-43b5-9a57-ae8af8ee149b` (2026-10-22 11:09), with design only
and no committed navigation prerequisite. Work orders 01 and 02 are complete.
No owner-handoff code or tests have been added. Resume after integrating the
committed management task 02 result and reconciling its exact typed contract.
