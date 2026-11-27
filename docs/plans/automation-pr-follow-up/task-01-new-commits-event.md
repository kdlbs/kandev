---
id: "01-new-commits-event"
title: "Fire on new PR commits"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-001
  - REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-002
  - REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-003
acceptance_criteria:
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-001.1
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-001.2
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-001.3
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-001.4
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-001.5
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-001.6
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-001.7
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-001.8
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-001.9
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-001.10
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-002.1
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-002.2
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-002.3
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-002.5
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-003.1
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-003.2
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-003.3
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-003.5
system_design:
  - ../../specs/office/system-design/automations-pr-follow-up.md
---

# Task 01: Fire on new PR commits

## Summary

Add the `new_commits` event and the shared PR listing and observation layer.
The poller lists open PRs through the REST pulls endpoint and applies every
filter. It records baselines and fires once per settled head, honoring an
optional quiet period. The editor offers **New commits** and the quiet-period
input.

## In scope

- `github.Client.ListOpenPRs` (PAT, `gh`, mock, and noop) with pagination and
  `complete`. The `PR` fields `Labels`, `RequestedReviewers`, and
  `RequestedTeams`. `Service.ListAutomationOpenPRs` and `GetAutomationPR`.
- The evaluator rewrite with `chooseCandidate` and `updateObservation`,
  covering the `opened` and `new_commits` candidates and the one-per-PR rule.
  Task 02 adds the `review_requested` candidate.
- The `automation_pr_observations` table and store. The requested-reviewers
  column is created now and written from task 02.
- `QuietPeriodMinutes` config and validation.
- `FireResult.Deferred` handled as "no change". Task 03 is what sets it.
- Trigger data keys `head_sha`, `previous_head_sha`, `event`, and
  `pr_subject`, with the matching placeholders.
- Editor: the **New commits** checkbox and the quiet-period input, with copy
  in all locales.
- Public docs in `docs/public/automation-and-mcp.md`: PR events, the quiet
  period, placeholders, and the filter correction.

## Out of scope

- `review_requested` (task 02), the per-PR policy (task 03), and later tasks.

## Acceptance

1. Integration tests with real in-memory stores, `MockClient`, the real
   evaluator, and `Service.FireTrigger` prove AC-001.1–001.10. That includes
   the quiet period with an injected clock and the unchanged `opened` behavior.
2. All filters apply to listed PRs. The placeholders render. PAT and `gh`
   pagination tests pass.
3. The editor toggles `new_commits` and edits `quiet_period_minutes`
   (0–1440) on desktop and phone. The E2E events scenario passes on `chromium`
   and `mobile-chrome`.

## ASCII UI preview

See [UI-01](plan.md#ui-01-pr-trigger-events-trigger-editor-github_pr-trigger).

```text
[x] Opened (i)   [x] New commits (i)
Wait until pushes stop for  [ 0 ] minutes
```

This covers AC-003.1–003.3 and AC-003.5.

## Verification

```bash
(cd apps/backend && go test ./internal/github/ -run 'ListOpenPRs' -count=1)
(cd apps/backend && go test ./internal/automation/ -run 'PRTrigger|NewCommits|PRFilters|PRPlaceholders|GitHubPRPlaceholders|PRObservation|QuietPeriod' -count=1)
(cd apps/backend && go test ./internal/automation/ -run 'TestGitHubEvaluator|TestPREvents' -count=1)
(cd apps && pnpm --filter @kandev/web test -- components/automations/trigger-configs/github-pr-config.test.tsx)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/automations-pr-follow-up.spec.ts)
(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/mobile-automations-pr-follow-up.spec.ts)
```

## Files likely touched

- `apps/backend/internal/github/client.go`, `pat_client.go`, `gh_client.go`,
  `mock_client.go`, `noop_client.go`, `models.go`, `service_reviews.go`, and
  `pat_client_open_prs_test.go` (new), `gh_client_reads_test.go`
- `apps/backend/internal/automation/evaluator.go`,
  `evaluator_pr_observation.go` (new), `store_pr_observations.go` (new),
  `store.go` (schema list), `models.go`, `interpolator.go`,
  `trigger_registry.go`, `service.go` (`FireResult.Deferred`, config
  validation)
- `apps/backend/internal/automation/evaluator_pr_new_commits_test.go` (new),
  `store_pr_observations_test.go` (new), `interpolator_test.go`,
  `trigger_registry_test.go`, `evaluator_pr_events_test.go` (fixture move)
- `apps/web/components/automations/trigger-configs/github-pr-config.tsx` and
  its test, `apps/web/src/locales/*/automations.json`
- `apps/web/e2e/tests/automations-pr-follow-up.spec.ts`,
  `mobile-automations-pr-follow-up.spec.ts` (new)
- `docs/public/automation-and-mcp.md`

## Dependencies

None.

## Risks

- Move the `evaluator_pr_events_test.go` fixtures from search to
  `ListOpenPRs` without weakening their assertions.
- `gh api --paginate` concatenates JSON arrays. Parse every page.
- The i18n generators can touch unrelated catalogs. Revert those changes.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/office/requirements/automations-pr-follow-up.md)
  REQ-001–003.
- [System design](../../specs/office/system-design/automations-pr-follow-up.md):
  PR listing, Trigger config, PR observation, and Evaluator control flow.
- Patterns: `evaluator_pr_events_test.go`, `ListAuthoredPRs`.

## Results

Done on 2026-11-24.

- `(cd apps/backend && go test -trimpath ./internal/github/ -run 'ListOpenPRs' -count=1)`: ok (PAT pagination, partial listing, first-page error, `gh pr list` conversion).
- `(cd apps/backend && go test -trimpath ./internal/automation/ -run 'NewCommits|QuietPeriod|FiringAdvances|OpenedAndNewCommits|PRFiltersApply|PRPlaceholders|GitHubPRPlaceholders|PRObservationStore|TestGitHubPR' -count=1)`: ok.
- `(cd apps && pnpm --filter @kandev/web test -- components/automations/trigger-configs/github-pr-config.test.tsx)`: 12 passed.
- `(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/automations-pr-follow-up.spec.ts)`: 2 passed.
- `(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/mobile-automations-pr-follow-up.spec.ts)`: 1 passed.

Deviations: `gh` uses `gh pr list --limit 1000` instead of `gh api --paginate`; `NoopClient.ListOpenPRs` returns `ErrNoClient`. The unused search-based `github.Service.ListAutomationPRs` was removed. Placeholder descriptions come from the backend registry, like the existing `pr.*` keys; AC-002.5 was reworded to match. The design was updated for all three.
