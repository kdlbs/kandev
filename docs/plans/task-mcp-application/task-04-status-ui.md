---
id: "04-status-ui"
title: "Expose application status and recovery"
status: done
wave: 6
depends_on:
  - 03-provider-acceptance
plan: "plan.md"
requirements:
  - REQ-AGENTS-MCP-APPLY-001
  - REQ-AGENTS-MCP-APPLY-003
acceptance_criteria:
  - AC-AGENTS-MCP-APPLY-001.6
  - AC-AGENTS-MCP-APPLY-003.4
  - AC-AGENTS-MCP-APPLY-003.5
system_design:
  - ../../specs/agents/system-design/task-mcp-application.md
---

# Task 04: Expose application status and recovery

## Summary

Expose saved selection and per-session application status in the current editor and Connections panel.
Keep save failure distinct from runtime application failure.

## In scope

Wire projection, reload recovery, Retry deduplication, localized status, and shared desktop/phone state.
Update `docs/public/builtin-mcp-packages.md` timing and recovery instructions when behavior ships.
Reconcile the existing management criteria and timing language with the implemented exception.
Use binding/process generation to reject stale status even when the execution ID remains the same.
The native conformance and helper lifecycle tests prove terminal and preview continuity through success and provider-start failure. Desktop and phone E2E prove status, recovery, and current-catalog presentation.
Do not disable workspace controls while provider replacement is active.

## Out of scope

New settings navigation, new overlays, and automatic dismissal or focus movement.

## Acceptance

1. Save errors retain dirty drafts. Application errors retain saved choices and offer one authorized Retry.
2. Per-session mixed results survive reload. Old observations cannot erase pending or failed state.
3. Desktop and phone satisfy UI-01, localization, focus, hitbox, scroll, and viewport continuity checks.

## UI-01: Saved task package status

Entry: Task Customize > MCP servers > Built-in packages. Also show status in Connections.
The user requested Kandev rich output instead of ASCII previews. This table records the same structural contract.
The conversation preview uses `show_rich_output_kandev`. Spacing and example copy are illustrative.

| Region | Desktop panel | Phone existing full-height editor |
| --- | --- | --- |
| Fixed header | Built-in packages, Close | Back, Built-in packages, Close |
| Content scroller | Package switches, inheritance, details | Same controls, one detail level at a time |
| Status after save | Saved. Waiting for the current turn to finish. | Same wrapping text below the tabs |
| Session detail | Selected session, saved selection, verified current catalog | Same selected session, one detail level, no horizontal table |
| Actions | Existing Save / Reset when dirty; Retry in the status region | Same controls, 44px minimum touch targets |

Provider-start failure leaves terminals and previews available. Show the old-active notice only if the old provider is live and its scope permits effects.
State variants: Unsaved changes; Saving; Saved and pending; Saved and applying; Applied; Saved but application failed.
A save error retains Save/Discard and the draft. An application error shows Retry separately from Save.
Unsupported providers show the next-supported-start reason. Running sessions show the current-turn reason.
All status text uses a polite live region. No state change moves focus or closes the editor.
Required geometry follows existing package management. Do not add another overlay.
Coverage: AC-AGENTS-MCP-APPLY-003.4 and AC-AGENTS-MCP-APPLY-003.5.

Full preview: [UI-01](plan.md#ui-01-saved-task-package-status).

## Verification

Use TDD for changed logic. Run commands from the repository root. New test paths are targets created by this work order.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/task/chat/mcp-explorer/builtin-mcp-view-model.test.ts)
(cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/settings/task-mcp-application.spec.ts)
(cd apps/web && pnpm e2e:run --host --no-build --shards 1 --project mobile-chrome tests/settings/mobile-task-mcp-application.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/components/task/chat/mcp-explorer/builtin-session-evidence.tsx`
- `apps/web/components/task/chat/mcp-explorer/builtin-package-save.tsx`
- `apps/web/components/task/chat/mcp-explorer/builtin-mcp-view-model.test.ts`
- `apps/web/lib/types/builtin-mcp.ts`
- `apps/web/src/locales/`
- `apps/web/e2e/tests/settings/task-mcp-application.spec.ts (new)`
- `apps/web/e2e/tests/settings/mobile-task-mcp-application.spec.ts (new)`
- `docs/public/builtin-mcp-packages.md`

## Dependencies

03-provider-acceptance

## Risks

Do not clear saved status when application fails. UI-only mocks cannot prove process continuity.

## Parallelism

Sequential. No delegation is authorized.

## Inputs

- [Requirements](../../specs/agents/requirements/task-mcp-application.md).
- [System design](../../specs/agents/system-design/task-mcp-application.md).
- [Plan and test mapping](plan.md).
- Scoped backend/web guidance and existing builtin MCP tests.

## Results

The existing task MCP explorer now separates saved request state from accepted tools. Desktop keeps its dialog and split view. Phone and coarse-pointer layouts keep their full-height drawer and one-level navigation. The shared status region wraps above the content scroller. Retry keeps the standard control sizing, including a 44px touch minimum. No new overlay or focus movement is introduced.

Open pending applications refresh without overlapping interval requests. Terminal failures and uncertain final acceptance stop polling until explicit user action. Owner fences reject late responses after task/session/backend changes. Mounted Retry tests cover one request, errors, busy cleanup, and a changed session.

Connections uses the accepted current provider catalog. Removed packages disappear. Verified tool discovery uses the existing Active status and loaded-tools description. Closed provider effects do not display historical tools as active. Applied requires complete current evidence, or a verified unchanged selection. English and six translated locales include the state and recovery copy. The pseudo catalog is regenerated.

Public docs updated: `docs/public/builtin-mcp-packages.md` (how-to) and the timing link in `docs/public/agents-and-profiles.md` (reference). Final browser and validation results are recorded in the plan.

All 86 focused explorer/session tests pass. The 24 existing settings/walkthrough consumer tests pass. Desktop and phone Playwright checks pass after the final catalog-status correction. Full web lint, typecheck, localization, and copy checks pass. The full frontend suite completes with 3381 passing files and three failures in one unchanged Docker-dependent Git fixture; the host has no Docker executable. Baseline details and exact totals are recorded in the plan.
