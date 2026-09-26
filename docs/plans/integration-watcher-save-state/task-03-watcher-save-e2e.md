---
id: "03-watcher-save-e2e"
title: "Prove pause and resume without refresh"
status: done
wave: 3
depends_on:
  - "01-github-review-watch-response"
  - "02-shared-watcher-saved-baseline"
plan: "plan.md"
spec: "../../specs/ui/requirements/settings-manual-save.md"
---

# Task 03: Prove pause and resume without refresh

## Acceptance

- The GitHub workspace settings E2E pauses and resumes a review watch through
  the floating Save action without reloading between either saved state.
- The test asserts the status, action availability, and persisted API value.
- The existing mobile GitLab pause scenario remains green against the shared
  hook with its touch and viewport assertions.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps/web && pnpm e2e:run tests/integrations/github-workspace-settings.spec.ts -- --grep "keeps review watch pause and resume visible after save"
cd apps/web && pnpm e2e:run tests/gitlab/mobile-gitlab-parity.spec.ts -- --project=mobile-chrome --grep "watch controls remain touch sized and persist a pause"
```

## Files likely touched

- `apps/web/e2e/tests/integrations/github-workspace-settings.spec.ts`
- `apps/web/e2e/tests/gitlab/mobile-gitlab-parity.spec.ts`

## Dependencies

Tasks 01 and 02.

## Parallelism

Sequential.

## Inputs

- `docs/specs/ui/requirements/settings-manual-save.md`
- `docs/plans/integration-watcher-save-state/plan.md`
- `apps/web/e2e/helpers/api-client.ts`
- Existing GitLab watcher desktop/mobile E2E patterns

## Risks

Use stable role/test-id selectors and the managed runner's production rebuild.
Do not add timeouts to hide a missed state transition.

## Output contract

Report the red and green command results, files changed, blockers, risks, and
update this task plus `plan.md` status in the primary conversation.
