---
id: "03-management-experience"
title: "Package selection and responsive management"
status: completed
wave: 3
depends_on:
  - "02-policy-runtime-binding"
plan: "plan.md"
requirements:
  - REQ-AGENTS-BUILTIN-MCP-001
  - REQ-AGENTS-BUILTIN-MCP-002
  - REQ-AGENTS-BUILTIN-MCP-003
  - REQ-AGENTS-BUILTIN-MCP-004
acceptance_criteria:
  - AC-AGENTS-BUILTIN-MCP-001.1
  - AC-AGENTS-BUILTIN-MCP-001.2
  - AC-AGENTS-BUILTIN-MCP-001.3
  - AC-AGENTS-BUILTIN-MCP-001.4
  - AC-AGENTS-BUILTIN-MCP-002.1
  - AC-AGENTS-BUILTIN-MCP-002.3
  - AC-AGENTS-BUILTIN-MCP-002.4
  - AC-AGENTS-BUILTIN-MCP-002.5
  - AC-AGENTS-BUILTIN-MCP-003.1
  - AC-AGENTS-BUILTIN-MCP-003.3
  - AC-AGENTS-BUILTIN-MCP-003.4
  - AC-AGENTS-BUILTIN-MCP-003.5
  - AC-AGENTS-BUILTIN-MCP-004.1
  - AC-AGENTS-BUILTIN-MCP-004.2
  - AC-AGENTS-BUILTIN-MCP-004.3
  - AC-AGENTS-BUILTIN-MCP-004.4
  - AC-AGENTS-BUILTIN-MCP-004.5
system_design:
  - ../../specs/agents/system-design/builtin-mcp-management.md
---

# Task 03: Package selection and responsive management

## Summary

Expose built-in package policy and task choice through existing MCP management.
Complete Configuration opt-in, per-package session evidence, desktop/phone parity,
localization, dependent action guards, and public documentation.

## In scope

Typed API clients/domain hooks; Built-in package section and detail; workspace
limits/defaults with shared Save/Discard/Reset; task Inherit/On/Off selector in
Customize and authorized task creation; fixed configuration-chat explanation;
read-only/blocked/denied/conflict flows; per-package connection/evidence detail;
Walkthrough guard/queued intent; locales; focused unit/rendered/E2E tests; docs.

## Out of scope

Global-profile package picker, independent session override, general management
hub/navigation changes, inherited-server exclusions, assignment at registration,
new probes, automatic reconnect, or modifying sibling workspaces.

## Acceptance

1. UI-01/UI-02 make Configuration explicit for regular tasks and automatic in
   configuration chats, with human authority and shared-resource scope explained.
   Workspace blocks and Core reasons are visible. Save/discard/reset/conflict/
   denial retain correct drafts and task/workspace ownership.
2. Desired, bound, and observed packages stay separate across saves, reloads,
   execution/session changes, legacy agents, and failures. One package's health
   never appears as another's evidence. Walkthrough requests use bound eligibility;
   historical content remains readable. Collisions offer safe remediation.
3. Desktop and phone finish opt-in and pruning with equivalent capability,
   localized copy, one phone scroller, safe areas, correct targets, keyboard and
   focus return. Public docs explain names, defaults, authority, and exact timing.

## UI preview contract

Use [UI-01 and UI-02](plan.md#ui-preview-contract). The explicit task instruction
requires native rich-output previews rather than ASCII.

UI-01 desktop: Built-in package list > inline package detail; purpose/owner,
registered tools/contexts, workspace policy choices, fixed required contexts,
next-start notice, shared Save/Discard. Configuration offers Blocked or Allow
explicit task selection; its configuration-chat default remains visible. Phone:
list replaced by full-height detail, fixed navigation/save, one scrolling body.

UI-02 task: Customize > Built-in packages, with Core Required and each optional
package Inherit/On/Off plus effective value/origin. Configuration initially reads
Inherit (Off); choosing On shows authority inline before Save. Connections >
package shows desired selection, bound manifest, pending state, and actual
observations separately. Desktop master/detail; phone list > detail > tool in one
drawer with Back/Close. Conflict keeps draft, denial stays unsaved, read-only
shows values/reasons. Map to AC-001.1/.2/.3/.4 and AC-004.1 through .5.

## Verification

Run from repository root; install once before pnpm checks in this fresh worktree.
Create named tests via TDD. Managed E2E builds use isolated fixtures, sequential
runs, and no protected runtime/preview ports.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run hooks/domains/workspace/use-builtin-mcp-management.test.ts hooks/domains/session/use-builtin-mcp-session.test.ts hooks/domains/session/use-request-changes-walkthrough.test.ts components/task-create-dialog-helpers.test.ts components/task/chat/mcp-explorer/builtin-mcp-view-model.test.ts components/task/chat/mcp-explorer/mcp-explorer-view-model.test.ts lib/state/slices/session-runtime/builtin-mcp-attachment.test.ts lib/ws/handlers/session-mcp-status.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/builtin-mcp-management.spec.ts tests/settings/builtin-mcp-configuration.spec.ts -- --retries=0)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-builtin-mcp-management.spec.ts -- --retries=0)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run scoped eslint on actual changed TS/TSX paths and any changed tests not listed.
Inspect light/dark, long locales, 320/393/767px phone, 768px boundary, 900px touch,
and desktop. Record screenshots and measured hitboxes/overflow against UI-01/02.
No full E2E audit. Check actual MCP tool catalogs/dispatch through controlled real
built-in requests; mocked healthy rows are presentation-only evidence.

## Files likely touched

- `apps/web/components/settings/workspaces/mcp-settings.tsx`, new detail and tests;
  `hooks/domains/workspace/use-builtin-mcp-policy.ts`; typed API clients/types.
- `apps/web/components/task/chat/mcp-explorer/` selector, detail, navigation and
  view-model; `hooks/domains/task/use-builtin-mcp-preferences.ts`; task creation
  selection/save integration; session snapshot projection and domain hooks.
- `apps/web/hooks/domains/session/use-request-changes-walkthrough.ts`, Changes,
  mobile Changes, Review callers, and typed queued intent payload.
- `apps/web/e2e/tests/settings/{builtin-mcp-management,builtin-mcp-configuration,mobile-builtin-mcp-management}.spec.ts`,
  fixture helpers and `apps/web/src/locales/`.
- `docs/public/agents-and-profiles.md` and `docs/public/automation-and-mcp.md`;
  affected README task-agent MCP and screenshots. Reconcile active workspace MCP,
  rich-output, discovery, and configuration requirements/design wording where
  it assumes unconditional availability or a single injected server.

## Dependencies

Task 02. The UI only exposes selection once runtime and dispatch enforce it.

## Inputs

Full design, parent testing guide, shipped settings save contributor and explorer,
mobile-parity language/sizing references. Use `/tdd`, `/e2e`, `/mobile-parity`,
`/better-ui`, and `/docs-maintainer`. Keep one draft/controller across viewports.

## Risks

Task settings affect future starts of all its normal sessions; a session editor
must not label the scope session-only. Optional server entries are not catalog
records or healthy merely because selected. Qualified names and legacy reports
need clear labels. All seven locales are required; generate Traditional Chinese.

## Parallelism

`sequential`

## Results

Completed. Workspace policy, task customization and
initial creation choices, package evidence, Walkthrough guards, localization, and
public docs are implemented. Tests use domain hooks and pure view models plus
focused browser journeys; React component unit tests were replaced with rendered
browser checks under the current repository testing convention. Actual commands
and results are in [implementation-results.md](implementation-results.md).

Final results: 102 focused frontend tests pass, with full typecheck, zero-warning
changed-file ESLint, seven-language/i18n checks, documentation validation, and
focused browser journeys. Desktop Configuration chat catalog and task creation
consent pass real package calls. Phone creation, opt-in/pruning, workspace policy,
and Portuguese dark/light layout at 320/393/767/768/900 px pass. Existing desktop
and phone MCP explorer tests pass. Six fresh desktop/phone screenshots cover
workspace policy, task choices, and initial consent. Disposable capture specs were
removed after the successful managed run; assets remain ignored.
