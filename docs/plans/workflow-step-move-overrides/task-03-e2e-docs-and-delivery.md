---
id: "03-e2e-docs-and-delivery"
title: "Move override E2E and docs"
status: reopened
wave: 3
depends_on:
  - "01-backend-move-contract-and-entry"
  - "02-frontend-move-options"
plan: "plan.md"
spec: "../../specs/workflow-step-move-overrides/spec.md"
---

# Task 03: Move override E2E and docs

## Scope

Prove the feature through desktop and mobile workflows, then update public workflow and agent communication documentation for the new one-shot contract.

## Likely files

- apps/web/e2e/tests/workflow/workflow-step-move-overrides.spec.ts.
- apps/web/e2e/tests/workflow/mobile-workflow-step-move-overrides.spec.ts.
- Existing workflow page objects or helpers only when a focused helper is needed.
- docs/public/tasks-and-workflows.md.
- docs/public/agent-communication.md.
- docs/public/websocket-api.md.
- apps/backend/config/prompts/config-context.md if Task 01 leaves agent-tool wording incomplete.
- docs/specs/INDEX.md and docs/decisions/INDEX.md.

## Acceptance

- Desktop E2E opens target options, submits a one-shot profile/reset/instructions override, and verifies the task reaches the target without mutating workflow defaults.
- Mobile E2E reaches the same options through touch, verifies the Drawer controls and submission, and asserts no horizontal overflow.
- An active-session or deferred move scenario verifies that the override survives the turn boundary and is delivered once when the target starts.
- A WIP-queued move scenario verifies that all override fields survive queue promotion and are delivered once after capacity opens.
- Public docs explain direct moves, one-shot overrides, target-step defaults, the MCP compatibility alias, and desktop/mobile affordances without promising PR draft/readiness support.
- Public documentation validators and focused E2E commands pass with retries disabled for the focused specs.

## Verification

    cd apps/web && pnpm e2e:run --project chromium tests/workflow/workflow-step-move-overrides.spec.ts --retries=0
    cd apps/web && pnpm e2e:run --project mobile-chrome tests/workflow/mobile-workflow-step-move-overrides.spec.ts --retries=0

    node --test scripts/validate-public-docs.test.mjs
    node scripts/validate-public-docs.mjs
    git diff --check

## Handoff

Public documentation is updated and five desktop plus four mobile browser scenarios cover immediate, active-turn deferred, restart-recovered, and WIP-promoted moves with all four overrides and exactly-once assertions. Playwright discovers the scenarios with retries disabled, but the managed runner cannot start agentctl because this sandbox rejects its listener with `EPERM` before any test body runs. Keep this work order open until both focused browser commands execute in an environment that permits loopback listeners; PR readiness remains out of scope.
