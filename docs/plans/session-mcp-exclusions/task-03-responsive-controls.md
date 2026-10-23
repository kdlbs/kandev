---
id: "03-responsive-controls"
title: "Expose responsive exclusion controls"
status: done
wave: 3
depends_on:
  - "02-application-recovery"
plan: "plan.md"
requirements:
  - REQ-AGENTS-SESSION-MCP-EXCLUSIONS-001
  - REQ-AGENTS-SESSION-MCP-EXCLUSIONS-002
  - REQ-AGENTS-SESSION-MCP-EXCLUSIONS-003
acceptance_criteria:
  - AC-AGENTS-SESSION-MCP-EXCLUSIONS-001.1
  - AC-AGENTS-SESSION-MCP-EXCLUSIONS-001.2
  - AC-AGENTS-SESSION-MCP-EXCLUSIONS-001.3
  - AC-AGENTS-SESSION-MCP-EXCLUSIONS-001.4
  - AC-AGENTS-SESSION-MCP-EXCLUSIONS-001.5
  - AC-AGENTS-SESSION-MCP-EXCLUSIONS-001.6
  - AC-AGENTS-SESSION-MCP-EXCLUSIONS-001.7
  - AC-AGENTS-SESSION-MCP-EXCLUSIONS-002.4
  - AC-AGENTS-SESSION-MCP-EXCLUSIONS-002.5
  - AC-AGENTS-SESSION-MCP-EXCLUSIONS-003.1
  - AC-AGENTS-SESSION-MCP-EXCLUSIONS-003.2
  - AC-AGENTS-SESSION-MCP-EXCLUSIONS-003.3
  - AC-AGENTS-SESSION-MCP-EXCLUSIONS-003.4
  - AC-AGENTS-SESSION-MCP-EXCLUSIONS-003.5
system_design:
  - ../../specs/agents/system-design/session-mcp-exclusions.md
---

# Task 03: Expose responsive exclusion controls

## Summary

Expose session-only exclusion and restoration through existing Customize.
Ship desktop/phone evidence and update the public selection instructions.

## In scope

- Typed session payloads, committed-state controller, revision conflicts, stale comparison guards.
- Origin-row actions, dormant/disabled explanations, pending comparison, and built-in limit copy.
- Desktop dialog, phone/tablet drawer, keyboard/touch parity, and seven-locale copy.
- Focused E2E journeys, public how-to update, and parent contract reconciliation.

## Out of scope

Registration assignment, usage navigation, global-profile editor, built-in tool
management, and exclusion controls in task/session creation or upstream editors.

## Acceptance

1. Exclude/restore changes only the existing session and retains every origin.
   Failed or stale writes never appear saved. Pending state never claims health.
2. Desktop, phone, narrow fine-pointer, and coarse-pointer tablet paths preserve
   one controller and one scroller with measured targets and reachable final rows.
3. Focused unit/E2E/i18n checks pass, public instructions match behavior, and
   requirement/design activation accurately reflects completed work.

## ASCII UI preview

UI-01/02 excerpts. Full structure and material states: [plan preview](plan.md#ascii-ui-preview).
Applies to AC-003.1 through .5 and AC-002.4/.5.

```text
UI-01 Desktop dialog:
  MCP servers [Close] / [Connections] [Customize]
  Pending: Remove Atlas Docs after this turn
  Selected for this task
  Atlas Docs                  [Restore inheritance]
  Also selected by: app, api, Review profile
  Saved: excluded. Still in last accepted setup.
  Additional servers / [Search] / choices

UI-02 Phone drawer:
  MCP servers [Close]
  [Connections] [Customize]             <- fixed
  Pending removal after this turn       <- one scroller
  Atlas Docs / all contributor names
  Saved: excluded
  Still in last accepted setup
  [Restore inheritance                ] <- 44px target
  Additional servers / search / choices
  Safe-area clearance

UI-03 Variants:
  Disabled: Restore does not enable this definition.
  Dormant: No longer inherited. [Restore inheritance]
  Failed: Last accepted setup retained. [Retry]
  Unknown: Exact changes cannot be compared yet.
  Rejected: Could not save. Authoritative choices remain.
```

Preserve shared tokens and compact desktop density. Status changes require
static text. No new animation or confirmation overlay is needed.

## Verification

Run from repository root. Install dependencies once before the first pnpm command.
Use current managed builds, then reuse them for the following phone run only
when no code changed. The E2E commands must run sequentially.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run hooks/domains/workspace/use-mcp-selection-editor.test.ts hooks/domains/workspace/use-mcp-selection-editor-context.test.ts hooks/domains/session/use-task-session-mcp-selection.test.ts components/mcp/mcp-selection-picker.test.tsx components/task/chat/mcp-explorer/mcp-selection-changes.test.ts components/task/chat/mcp-explorer/mcp-session-exclusions.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --host --project chromium tests/chat/mcp-session-exclusions.spec.ts tests/chat/mcp-pending-changes.spec.ts tests/task/task-mcp-selection.spec.ts)
(cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome tests/chat/mobile-mcp-session-exclusions.spec.ts tests/chat/mobile-mcp-pending-changes.spec.ts tests/task/mobile-task-mcp-selection.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Also run changed-file ESLint with actual edited TS/TSX paths. Generate Traditional
Chinese with `pnpm run i18n:zh-hant` and pseudo output with the repository generator
before the i18n gates. Record browser geometry and focus results against UI-01/02/03.
Run all modified test suites if implementation requires additional adjacent tests.

## Files likely touched

- `apps/web/lib/types/http-mcp.ts`, `lib/api/domains/mcp-api.ts`, and a focused API test if payload logic changes.
- `apps/web/hooks/domains/workspace/use-mcp-selection-editor.ts` and `hooks/domains/session/use-task-session-mcp-selection.ts`, with their tests.
- `apps/web/components/task/mcp-session-selector.tsx`, `components/mcp/mcp-origin-sections.tsx`, and `components/mcp/mcp-selection-picker.tsx`.
- `apps/web/components/task/chat/mcp-explorer/` comparison, notice, views, and new `mcp-session-exclusions.test.tsx`.
- `apps/web/src/locales/*/{settings,task}.json`, new exclusion E2E specs/shared helper, and affected MCP neighbor tests.
- `docs/public/agents-and-profiles.md`, affected parent requirement/design/ADR text, and this package's statuses/results.

## Dependencies

Task 02, which includes the committed assignment work order 01 prerequisite.
Reuse that package's session revision types and Reload choices conflict handling;
extend it with E without adding a competing controller. Reconcile copy by intent.

## Risks

`MCPSessionSelectionBody` also serves creation. Require existing-session capability
before exposing exclusions. Ordinary 403 and revision 409 are local save errors,
not shared restart recovery. A narrow viewport with a fine pointer still needs
phone geometry. Do not count status fixtures as runtime proof.

## Parallelism

`sequential`

## Inputs

- [Design](../../specs/agents/system-design/session-mcp-exclusions.md): API, editor, responsive contract.
- [Parent testing guide](../workspace-mcp-configuration/testing-guide.md), especially sections 6 through 8, as scenario context only.
- `/tdd`, `/e2e`, `/mobile-parity`, `/better-ui`, and `/docs-maintainer` during implementation.

## Results

Completed on 2026-10-23. Existing-session rows expose committed exclude/restore
state, preserve contributors, and keep disabled/dormant exclusions reversible.
Creation and upstream editors remain additive. The shared revision controller
handles conflicts and lost-response reloads without automatic replay.

RED evidence: action/restore interaction tests failed before controls existed;
the reason-code test rejected the missing localized explanation. A separate
regression showed that a rejected addition draft hid the saved pending comparison.
The fix passes committed positive IDs to comparison while retaining the draft.
Exclusion actions deliberately keep no unsaved checkbox state: after Reload
choices, the user repeats the explicit action. The design records this refinement.

Frontend focused tests passed (59 cases across eight suites, including the new
comparison regression); changed-file lint passed with zero warnings, typecheck
passed, and all locale checks/ratchet passed. Traditional Chinese and pseudo
keys were generated; unrelated generated changes were discarded.

Managed browser results: 13 desktop/neighbor tests and 9 phone/neighbor tests
passed, sequentially. The exclusion journeys cover running-turn pending state,
409/reload, reopen, inherited restore, disabled dormant restore, keyboard/touch,
scrolling to final choices, fixed tabs, focus return, and no horizontal overflow.
320px fine-pointer and 820px coarse-pointer cases passed; touch targets met 44px.
Initial fixture-name reuse was corrected. Two neighboring helper expectations
were reconciled with assignment WO01's retained addition drafts and Reload choices.

Public how-to updated: `docs/public/agents-and-profiles.md`. Parent requirements,
configuration/pending designs, and the composition ADR record the session-only
exception. Public docs validators, catalog/spec lint, and whitespace checks pass.
These tests prove Kandev's mock-provider and UI contracts, not live-provider
connectivity or revocation of independently configured tools.
