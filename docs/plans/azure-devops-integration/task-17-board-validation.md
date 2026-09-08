---
id: "17-board-validation"
title: "Board E2E, docs, and verification"
status: done
wave: 10
depends_on: ["16-board-ui"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/azure-devops-integration.md"
---

# Task 17: Board E2E, Docs, And Verification

## Acceptance

- Desktop Playwright proves default board load, card edit, cross-column drag,
  and provider-backed state after reload; mobile Playwright proves focused
  column navigation, core-field editing, explicit move, editor containment,
  44px controls, and no document horizontal overflow.
- Azure settings and public integration docs require Work Items Read & write
  plus Code Read, explain read-only legacy PAT behavior, and describe Board
  mode's supported edits and explicit non-goals.
- The requested repository format, typecheck, test, and lint gates pass after
  focused backend/frontend/E2E checks, or an exact external blocker is
  recorded.

## Verification

- `rtk pnpm e2e:run --host tests/integrations/azure-devops.spec.ts -- --project=chromium` from `apps/web`.
- `rtk pnpm e2e:run --host tests/integrations/mobile-azure-devops.spec.ts -- --project=mobile-chrome` from `apps/web`.
- `rtk node --test scripts/validate-public-docs.test.mjs` from the repository root.
- `rtk node scripts/validate-public-docs.mjs` from the repository root.
- `rtk make fmt` from the repository root.
- `rtk make typecheck test lint` from the repository root.

## Files Likely Touched

- `apps/backend/internal/azuredevops/mock_client.go`
- `apps/backend/internal/azuredevops/mock_client_test.go`
- `apps/web/e2e/helpers/api-client.ts`
- `apps/web/e2e/tests/integrations/azure-devops.spec.ts`
- `apps/web/e2e/tests/integrations/mobile-azure-devops.spec.ts`
- `apps/web/components/azure-devops/azure-devops-settings.tsx`
- `apps/web/components/azure-devops/azure-devops-settings.test.tsx`
- `docs/public/integrations.md`

## Dependencies

Task 16 and all backend dependencies.

## Parallelism

Sequential. E2E and documentation assert the final integrated API/UI contract.

## Inputs

- All Board-mode scenarios in the spec.
- Required workflows: `.agents/skills/e2e/SKILL.md`,
  `.agents/skills/mobile-parity/SKILL.md`, and
  `.agents/skills/docs-maintainer/SKILL.md`.
- Existing Azure desktop/mobile integration specs and mock seed API.

## Risks

- Run Playwright through the managed runner so both Go and Vite production
  artifacts are rebuilt; `--no-build` would risk stale board code.
- Seed and assert Azure-backed mock state rather than only optimistic DOM state.

## Output Contract

Report Playwright and broad verification results, rendered mobile inspection,
public docs changes, files changed, blockers and residual risks, then set this
task and its plan checkbox to done.
