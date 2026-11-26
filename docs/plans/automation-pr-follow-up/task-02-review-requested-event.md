---
id: "02-review-requested-event"
title: "Fire when a review is requested"
status: done
wave: 2
depends_on:
  - "01-new-commits-event"
plan: "plan.md"
requirements:
  - REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-002
  - REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-003
  - REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-007
acceptance_criteria:
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-002.4
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-002.5
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-003.1
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-003.2
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-003.4
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-003.5
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-007.1
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-007.2
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-007.3
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-007.4
system_design:
  - ../../specs/office/system-design/automations-pr-follow-up.md
---

# Task 02: Fire when a review is requested

## Summary

Add the `review_requested` event. The poller compares each PR's requested
reviewers and teams with the stored observation, and fires once when a
matching reviewer is newly requested, including on a re-request. The editor
offers **Review requested** and a reviewers filter.

## In scope

- The `Reviewers` config field.
- Requested-set normalization: lowercased logins and `<owner>/<team-slug>`.
- The `review_requested` candidate in `chooseCandidate`, the precedence rule,
  the `requested` and `review_generation` updates, and the dedup key.
- Trigger data `requested_reviewers` and the `{{pr.requested_reviewers}}`
  placeholder.
- Editor: the **Review requested** checkbox and the reviewers input, with
  copy in all locales.
- Public docs: the event and the re-request use case.

## Out of scope

- The per-PR policy and later tasks.

## Acceptance

1. Evaluator integration tests prove these behaviors: a new request fires; a
   removal followed by a re-request fires again; several reviewers in one poll
   fire once; matching of team keys and case; the generation dedup key; and
   the baseline does not fire.
2. When `new_commits` and `review_requested` happen in the same poll, the
   result is one `new_commits` firing, and both transitions are stored as
   processed.
3. The editor's reviewers input parses on blur, as the authors input does, on
   desktop and phone. The E2E events scenario covers the checkbox and the
   input.

## ASCII UI preview

See [UI-01](plan.md#ui-01-pr-trigger-events-trigger-editor-github_pr-trigger).

```text
[x] Review requested (i)
Reviewers  [ kandev-bot, acme/reviewers ]
           Leave empty to react to any reviewer.
```

This covers AC-003.1, 003.2, 003.4, and 003.5.

## Verification

```bash
(cd apps/backend && go test ./internal/automation/ -run 'ReviewRequested|PRObservation|PRPlaceholders' -count=1)
(cd apps/backend && go test ./internal/github/ -run 'ListOpenPRs' -count=1)
(cd apps && pnpm --filter @kandev/web test -- components/automations/trigger-configs/github-pr-config.test.tsx)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/automations-pr-follow-up.spec.ts)
(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/mobile-automations-pr-follow-up.spec.ts)
```

## Files likely touched

- `apps/backend/internal/automation/evaluator_pr_observation.go`, `models.go`,
  `interpolator.go`, `trigger_registry.go`, `store_pr_observations.go`
- `apps/backend/internal/automation/evaluator_pr_review_requested_test.go`
  (new)
- `apps/web/components/automations/trigger-configs/github-pr-config.tsx` and
  its test, `apps/web/src/locales/*/automations.json`
- `apps/web/e2e/tests/automations-pr-follow-up.spec.ts`,
  `mobile-automations-pr-follow-up.spec.ts`
- `docs/public/automation-and-mcp.md`

## Dependencies

Task 01, which provides the listing with reviewers and the observation table.

## Risks

- The pulls listing returns `requested_teams` without an organization. Build
  team keys from the repository owner.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/office/requirements/automations-pr-follow-up.md)
  REQ-007, AC-002.4, and REQ-003.
- [System design](../../specs/office/system-design/automations-pr-follow-up.md):
  PR observation.

## Results

Done on 2026-11-24.

- `(cd apps/backend && go test -trimpath ./internal/automation/ -run 'ReviewRequested' -count=1)`: ok (new request, re-request, team and case matching, one firing per poll, precedence, quiet-period interplay).
- `(cd apps && pnpm --filter @kandev/web test -- components/automations/trigger-configs/github-pr-config.test.tsx)`: 12 passed.
- `(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/automations-pr-follow-up.spec.ts)`: 2 passed.
- `(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/mobile-automations-pr-follow-up.spec.ts)`: 1 passed.

Deviation: requested reviewers reuse the existing `PR.RequestedReviewers` field instead of new `RequestedReviewers`/`RequestedTeams` fields.
