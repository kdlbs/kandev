---
id: "13-enhancement-validation"
title: "Cross-provider E2E, security review, and documentation"
status: done
wave: 7
depends_on: ["08-availability-and-identity", "09-azure-presets", "12-unified-repository-picker"]
plan: "plan.md"
spec: "../../specs/azure-devops-integration/spec.md"
---

# Task 13: Cross-Provider E2E, Security Review, And Documentation

## Acceptance

- Focused desktop/mobile E2E proves immediate integration navigation, Azure presets, and task creation from each configured VCS provider.
- PAT setup documentation includes the organization deep link and exact Work Items Read plus Code Read scopes.
- Public integration/task-creation documentation explains discovery versus clone/push credentials.
- Full format, typecheck, test, lint, security, and simplification passes succeed or blockers are documented.

## Verification

- Repository standard format, backend test/lint, web test/typecheck/lint, docs validation, and focused Playwright projects.
- Manual secret-leak scan of logs, DTOs, persisted models, and executor metadata.

## Files Likely Touched

- `apps/web/e2e/tests/integrations/`
- `apps/web/e2e/tests/task/`
- `docs/public/integrations.md`
- Relevant mock controllers and test fixtures.

## Output Contract

Report all verification results, residual risks, documentation changes, and update this task plus its plan checkbox.
