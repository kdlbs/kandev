---
id: "01-auth-warning"
title: "Authentication warning presentation"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-MCP-PREP-002
  - REQ-AGENTS-MCP-PREP-003
acceptance_criteria:
  - AC-AGENTS-MCP-PREP-002.6
  - AC-AGENTS-MCP-PREP-002.7
  - AC-AGENTS-MCP-PREP-003.5
system_design:
  - ../../specs/agents/system-design/agent-mcp-preparation.md
---

# Task 01: Authentication warning presentation

## Summary and scope

Derive a shared warning severity for exact authentication-required imported MCP
verification rows. Apply it to row icon, aggregate status and recovery text while
preserving readiness and existing Authenticate/Retry behavior. Exclude all other
failure-code policy changes and backend wire/schema changes.

## Acceptance

- Warning-only completion is amber with actionable text, including old hydrated failed rows.
- A real failed row or overall failed preparation remains red; mixed servers are counted correctly.
- Authentication and retry remain available on both viewports; successful current-attempt retry clears only its warning.

## Files likely touched

- `apps/web/components/session/prepare-progress.tsx` and adjacent status/component tests
- `apps/web/components/task/agent-mcp-prepare-actions.tsx` and its tests
- New shared classifier beside these components with focused unit tests if needed
- `apps/web/src/locales/` only if existing copy cannot express the warning
- New `apps/web/e2e/tests/chat/setup-recovery.spec.ts` and `mobile-setup-recovery.spec.ts`

## Verification

Run the regression tests red before implementation, then green. From repo root:

```bash
(cd apps/web && pnpm exec vitest run components/session/prepare-progress-status.test.ts components/session/prepare-progress.test.tsx components/task/agent-mcp-prepare-actions.test.tsx)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/setup-recovery.spec.ts -- --grep 'authentication warning')
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-setup-recovery.spec.ts -- --grep 'authentication warning')
(cd apps/web && pnpm run i18n:check)
```

Add any new classifier test file to the exact Vitest command before recording results.

## Risks

Do not mark failed readiness completed or suppress recovery controls. Match exact
kind/reason codes, never an English error substring. Preserve stale-attempt fences.

## ASCII UI preview

See the [combined preview](plan.md#ascii-ui-preview).

### UI-01: Preparation authentication warning

Entry: task or Quick Chat conversation, completed environment preparation.
Current: red "Environment setup finished with errors", red verification cross.
Proposed shared desktop/phone content:

```text
[v] Environment setup finished with warnings
    [!] Verify connection: plugin-atlassian-atlassian
        Authentication is required.
        [Authenticate] [Retry connection]
```

The triangle and warning message are amber; a real fatal failure stays red.
On phones actions wrap into reachable 44px targets in the existing scroll region.
Maps to AC-AGENTS-MCP-PREP-002.6/002.7 and AC-AGENTS-MCP-PREP-003.5.


## Dependencies

None.

## Parallelism

`sequential`

## Inputs

Read the linked requirement/design amendment, scoped AGENTS.md, existing tests,
and the evidence in plan.md. Follow /tdd, /mobile-parity and /e2e during implementation.

## Results

- Implemented `isAgentMcpAuthWarning` classifier in `apps/web/lib/prepare/agent-mcp-warning.ts`.
- Updated `apps/web/components/session/prepare-progress.tsx` to derive `"completed_with_warnings"` for auth-required imported MCP verification failures, render amber warning icon and auto-expand details.
- Updated `apps/web/components/task/agent-mcp-prepare-actions.tsx` to render auth warning message with amber styling (`text-amber-500`).
- Added unit tests in `prepare-progress-status.test.ts`, `prepare-progress.test.tsx`, `agent-mcp-prepare-actions.test.tsx`, and `agent-mcp-warning.test.ts` (all 24 tests passed).
- Added desktop and mobile E2E tests in `setup-recovery.spec.ts` and `mobile-setup-recovery.spec.ts` (passed).
- Verified `i18n:check` and `typecheck` passed cleanly.

## Review validation follow-up

Review corrections and focused unit checks are complete; see the
[local review record](plan.md#local-review-and-corrections-2026-10-02) for the exact
scope, regressions, results, and Chromium sandbox blocker. Earlier E2E counts
above are historical implementation results, not validation of the revised
working tree. Rendered verification subsequently passed on desktop and phone;
see the plan publication-validation follow-up for commands and fixture corrections.
