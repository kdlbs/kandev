---
id: "06-e2e-mobile-qa"
title: "E2E, mobile QA, and final audit"
status: done
wave: 3
depends_on: ["02-workflow-drafts", "03-general-operational-drafts", "04-manual-editor-migration", "05-integration-settings"]
plan: "plan.md"
spec: "../../specs/ui/settings-manual-save.md"
---

# Task 06: E2E, Mobile QA, and Final Audit

## Acceptance

- Desktop E2E proves draft-before-Save, persistence-after-Save, dirty navigation, new workflow behavior, and retained immediate commands.
- Mobile E2E proves the fixed action is touch reachable, safe-area aware, non-overlapping, and does not introduce horizontal overflow at 390px.
- A final settings audit finds no persistence call reachable directly from a draftable control; full format, typecheck, test, and lint pass.

## Verification

```bash
cd apps/web && pnpm e2e:run tests/workflow/workflow-settings.spec.ts tests/settings/settings-manual-save.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome --no-build tests/workflow/mobile-workflow-settings.spec.ts tests/settings/mobile-general-settings.spec.ts
cd apps/web && pnpm e2e:run tests/settings/agent-profile-acp.spec.ts tests/settings/agent-profile-cli-flags.spec.ts tests/automations-settings.spec.ts tests/terminal/terminal-settings.spec.ts tests/integrations/jira-settings.spec.ts
GOCACHE=/tmp/kandev-go-cache make fmt
GOCACHE=/tmp/kandev-go-cache make typecheck
GOCACHE=/tmp/kandev-go-cache make test
GOCACHE=/tmp/kandev-go-cache GOLANGCI_LINT_CACHE=/tmp/kandev-golangci-cache make lint
```

## Files Likely Touched

- `apps/web/e2e/pages/workflow-settings-page.ts`
- `apps/web/e2e/tests/workflow/workflow-settings.spec.ts`
- `apps/web/e2e/tests/workflow/mobile-workflow-settings.spec.ts`
- `apps/web/e2e/tests/settings/settings-manual-save.spec.ts`
- `apps/web/e2e/tests/settings/mobile-general-settings.spec.ts`
- Existing affected settings/integration page objects and specs whose Save selectors changed
- `docs/specs/ui/settings-manual-save.md`
- `docs/plans/settings-manual-save/plan.md`

## Dependencies

Tasks 02-05.

## Inputs

- Every spec Scenario.
- Mobile parity and E2E skill guidance.
- Completed migration reports from Tasks 02-05.

## Output Contract

Report scenarios and viewport checks, screenshots inspected, final autosave audit results, exact commands/results, residual risks, and mark all task/plan statuses accurately.

## Result

- Added desktop coverage for workflow and general settings draft-before-Save, persistence-after-Save, and dirty-route navigation.
- Added mobile coverage for explicit workflow Save plus 390px action geometry, touch target size, last-control clearance, and horizontal overflow.
- Migrated affected workflow, automation, integration, prompt, repository, executor-profile, agent-profile, existing Sentry, and existing SSH selectors to the shared floating action.
- Audited the settings routes after the final migrations. Draftable configuration mutation sites are registered contributors; remaining local Create/Save controls are new-resource or dialog flows, and named operational/destructive actions remain immediate by design.
- Scoped E2E Prettier and ESLint pass. Focused coordinator, automation-toggle, Sentry, and SSH
  component tests pass.
- Rendered workflow and automation desktop coverage passes; the seed-protection workflow case
  also passed three consecutive stabilization runs.
- All 5 focused mobile settings tests pass at the mobile project viewport, including floating
  action geometry, 44px touch target, last-control clearance, and horizontal-overflow checks.
- All 5 Docker-backed SSH persistence tests pass, including draft-before-Save and
  persistence-after-Save for an existing executor.
- Full formatting, generated metadata, typecheck, backend tests, web tests, CLI tests (278),
  script tests, backend lint, web lint, harness lint, and `git diff --check` pass.
