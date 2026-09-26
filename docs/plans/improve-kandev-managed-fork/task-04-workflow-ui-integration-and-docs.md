---
id: "04-workflow-ui-integration-and-docs"
title: "Workflow UI integration and docs"
status: complete
wave: 4
depends_on: ["03-runtime-credentials-and-fork-routing"]
plan: "plan.md"
spec: "../../specs/workspaces/requirements/improve-kandev.md"
---

# Task 04: Workflow, UI, Integration, and Docs

## Acceptance

- The managed PR workflow never runs `gh repo fork` or rewrites remotes. It pushes through the prepared
  upstream and creates the PR with explicit canonical repository, base, fork owner, and head branch. The
  executor-owned compatibility branch remains explicit and cannot be selected after managed failure.
- Desktop and mobile Improve Kandev dialogs block implementation reports for an unsupported managed
  automation configuration, translate stable backend reason codes into actionable copy, and keep Open
  issue usable.
- Focused integration proves creation, launch, push, resume, credential authorization, canonical origin,
  and PR targeting end to end. Public reference docs describe the behavior and limitation.

## Verification

```bash
cd apps/backend
rtk go test ./config/workflows ./internal/improvekandev ./internal/orchestrator/executor ./internal/backendapp -run 'Test.*(ImproveKandev|ContributionDestination|ManagedFork)'
cd apps && rtk pnpm --filter @kandev/web test -- improve-kandev
cd apps/web && rtk pnpm e2e:run improve-kandev.spec.ts
cd apps/web && rtk pnpm e2e:run --project mobile-chrome mobile-improve-kandev.spec.ts
rtk node --test scripts/validate-public-docs.test.mjs
rtk node scripts/validate-public-docs.mjs
```

## Files likely touched

- `apps/backend/config/workflows/improve-kandev.yml`
- `apps/backend/config/workflows/loader_test.go`
- focused backend integration test files
- `apps/web/lib/api/domains/improve-kandev-api.ts`
- `apps/web/components/improve-kandev-dialog-model.ts`
- `apps/web/components/improve-kandev-dialog-create.tsx`
- associated component/model tests and English/translated locale catalogs
- `apps/web/e2e/tests/improve-kandev.spec.ts`
- `apps/web/e2e/tests/mobile-improve-kandev.spec.ts`
- `docs/public/integrations.md`
- `docs/public/git-operations.md` if its ordinary-task push reference needs a destination exception

## Dependencies

Task 03's working destination remote, push target, and managed credential scopes.

## Parallelism

Sequential final integration. The prompt and UI must describe behavior already enforced by the backend.

## Inputs

- Spec: explicit managed PR target/head, guarded executor-owned compatibility, blocked managed
  configuration, issue-only bypass, and desktop/mobile parity.
- Mobile exemplar: the existing Improve Kandev dialog and mobile Improve Kandev Playwright flow. No new
  drawer, route, scroll owner, or touch interaction is introduced.
- Docs guidance: `integrations.md` and `git-operations.md` remain reference pages.

## Risks

- `gh pr create` must receive explicit target and head identity; canonical `origin` alone cannot identify
  an unpublished cross-fork head reliably.
- New user-facing copy must use i18n and must not contain an em dash.
- E2E must assert the user outcome through the dialog, not only the bootstrap JSON, and mobile must retain
  viewport containment and no document horizontal overflow.
- Run Playwright against a fresh production build through the managed runner and confirm both desktop and
  mobile projects discover tests.

## TDD sequence

1. Add failing loader, component/model, integration, desktop, and mobile assertions.
2. Rewrite the workflow prompt and expose the typed blocked state through shared UI logic.
3. Update public references and refactor repeated status rendering.
4. Run focused desktop/mobile suites, public-doc validation, and the plan's final audit.

## Output contract

Report workflow commands, desktop/mobile behavior, integration evidence, public docs classification, files
changed, red/green commands, remaining risks, divergence, and task/plan status updates.

## Completion

Completed 2026-08-13. The Improve Kandev prompt now uses the prepared managed route, preserves canonical
`origin`, blocks executor-owned recovery after managed preparation failure, and creates the PR with an
explicit canonical repository and fork-owner head. The UI exposes translated ready/creatable/blocked
states using a stable fork reason code and keeps issue reporting available. Desktop Playwright passed
15/15 tests, including the blocked managed state; mobile Playwright passed 1/1 with the existing 44px
touch target and overflow assertions.

Public documentation classification: reference updates only in `docs/public/integrations.md` and
`docs/public/git-operations.md`; no new page or navigation entry was required. The public-doc test suite
passed (60 tests), and the published-doc validator passed for 41 pages.
