---
id: "03-commit-source-e2e"
title: "Commit-source E2E coverage"
status: done
wave: 3
depends_on: ["01-github-commit-detail", "02-source-aware-commit-ui"]
plan: "plan.md"
spec: "../../specs/ui/requirements/pr-only-commit-details.md"
---

# Task 03: Commit-source E2E coverage

- **Acceptance:** Deterministic GitHub mock support can seed an individual
  commit detail independently of the local worktree; desktop E2E proves a
  PR-only commit omits false zero stats and local mutation actions and renders
  the remote header/patch; mobile E2E proves the same remote patch opens in the
  existing full-height sheet, has no local action, closes normally, and causes
  no horizontal overflow; the scenario cannot pass by serving a local
  `session.commit_diff` response for the remote SHA.
- **Verification:** Follow strict TDD. Add mock-controller and Playwright
  regressions first; record the backend/mock failure and browser failure against
  the pre-task state. After the minimal mock and test wiring, run
  `cd apps/backend && go test -run 'Test.*Mock.*Commit' ./internal/github`,
  `cd apps/web && pnpm e2e:run tests/git/git-changes-panel.spec.ts -- --grep "PR-only commit"`,
  `cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-changes-panel.spec.ts -- --grep "PR-only commit"`,
  and `git diff --check`.
- **Files likely touched:**
  `apps/backend/internal/github/mock_client.go`;
  `apps/backend/internal/github/mock_controller.go`;
  their focused tests;
  `apps/web/e2e/helpers/api-client.ts`;
  `apps/web/e2e/tests/git/git-changes-panel.spec.ts`; and
  `apps/web/e2e/tests/task/mobile-changes-panel.spec.ts`.
- **Dependencies:** Tasks 01 and 02 complete with stable backend and frontend
  source-routing contracts.
- **Parallelism:** sequential — mock seeding must match the settled response
  contract, and the desktop/mobile tests jointly validate one integrated source
  route.
- **Inputs:** repair spec regression scenarios; Task 01 individual-commit mock
  model; Task 02 source-aware targets; existing `mockGitHubAddPRCommits`,
  Git Changes panel fixtures, and mobile Changes panel full-height drawer tests.
- **Output contract:** Report the deterministic stale-worktree fixture, how the
  test prevents local fallback, exact files changed, backend and desktop/mobile
  RED/GREEN results, cleanup state, any quarantined instability, and
  synchronized task/plan status.

## Results

- GREEN: added deterministic mock individual-commit seeding through the
  existing mock GitHub controller and API helper. Desktop and mobile fixtures
  use PR-only SHAs absent from the stale local worktree, so the rendered
  metadata and patch require the GitHub detail response.
- Desktop verification passed with
  `pnpm e2e:run tests/git/git-changes-panel.spec.ts -- --grep "PR-only commit"`.
- Mobile verification passed with
  `pnpm e2e:run --project mobile-chrome tests/task/mobile-changes-panel.spec.ts -- --grep "PR-only commit"`.
  The first mobile attempt only exposed a strict-locator ambiguity in the test
  assertion; narrowing it to the sheet passed the same scenario.
- No local mutation action appeared, the remote patch marker rendered, the
  mobile sheet closed normally, and the horizontal-overflow check passed.
