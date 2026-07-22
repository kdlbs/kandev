---
id: "08-e2e-docs-verification"
title: "GitLab parity E2E and documentation"
status: done
wave: 5
depends_on: ["02-watcher-dispatch", "03-task-mr-linking-launch", "04-reviewers-subscriptions", "05-watch-settings-ui", "06-mr-review-ui", "07-mr-creation-runtime"]
plan: "plan.md"
spec: "../../specs/gitlab-integration/spec.md"
---

# Task 08: GitLab Parity E2E And Documentation

## Acceptance

- The mock/reset harness deterministically supports two workspace connections,
  watch matches, MR feedback/actions, reviewers, subscriptions, links, and MR
  creation without state leaking between Playwright workers/tests.
- Desktop and mobile Playwright flows prove workspace isolation, watch dispatch,
  quick launch/link/unlink, review/actions/subscriptions, and automatic MR link
  after creation, including layout/overflow assertions.
- Public docs describe the shipped workspace-scoped behavior and limitations,
  and full format/typecheck/test/lint plus docs validation pass.

## Verification

```bash
rtk make fmt
rtk make typecheck
rtk make test
rtk make lint
cd apps/web && rtk pnpm e2e:run tests/gitlab/gitlab-parity.spec.ts tests/gitlab/gitlab-watches.spec.ts tests/gitlab/gitlab-mr-creation.spec.ts
cd apps/web && rtk pnpm e2e:run --no-build --project mobile-chrome tests/gitlab/mobile-gitlab-parity.spec.ts
rtk node --test scripts/validate-public-docs.test.mjs
rtk node scripts/validate-public-docs.mjs
```

## Files Likely Touched

- `apps/backend/internal/gitlab/mock_client.go`
- `apps/backend/internal/gitlab/mock_controller.go`
- `apps/backend/internal/gitlab/mock_client_test.go`
- `apps/backend/internal/backendapp/e2e_reset.go`
- `apps/backend/internal/backendapp/e2e_reset_test.go` (new)
- `profiles.yaml`
- `apps/web/e2e/helpers/api-client.ts`
- `apps/web/e2e/pages/gitlab-page.ts` (new)
- `apps/web/e2e/pages/gitlab-settings-page.ts` (new)
- `apps/web/e2e/tests/gitlab/gitlab-parity.spec.ts` (new)
- `apps/web/e2e/tests/gitlab/gitlab-watches.spec.ts` (new)
- `apps/web/e2e/tests/gitlab/gitlab-mr-creation.spec.ts` (new)
- `apps/web/e2e/tests/gitlab/mobile-gitlab-parity.spec.ts` (new)
- `docs/public/integrations.md`
- `docs/public/feature-status.md`
- `docs/public/extending-kandev.md`
- `docs/public/websocket-api.md`
- `docs/specs/gitlab-integration/spec.md`
- `docs/specs/INDEX.md`
- `docs/plans/gitlab-integration-parity/plan.md`
- `docs/plans/gitlab-integration-parity/task-*.md`

## Dependencies

All production tasks must be integrated. This task may fix test-harness and
integration wiring defects, but it must return feature logic regressions to the
owning task rather than silently weakening assertions.

## Inputs

- All spec scenarios and plan E2E section.
- Skills: `/e2e`, `/mobile-parity`, `/qa`, `/docs-maintainer`, `/verify`.
- E2E patterns: GitHub watch reset, PR action/task indicator, Jira/Linear
  settings, and existing task PR detail/create tests.
- Constraint: assert user-visible UI outcomes; use API only to seed
  preconditions. Verify persistence with reload and cross-workspace isolation
  with distinct hosts/accounts.

## Output Contract

Report scenario coverage by file/project, screenshots or bounding-box evidence
for desktop/mobile, mock/reset changes, public docs updated, exact verification
results, files changed, blockers, flaky/residual risks, and any spec corrections.
Mark this task and plan `done` only after every required command passes; change
the spec to `shipped` only when all acceptance scenarios are satisfied.
