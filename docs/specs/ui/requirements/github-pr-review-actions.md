---
status: active
system: ui
created: 2026-07-23
owners:
  - Kandev
---
# GitHub PR Review Actions Requirements

## Overview

Pull-request authors can see dismissed GitHub reviews in Kandev, but must leave Kandev to ask that reviewer to review again. Re-requesting a review should be available where the dismissed review is already visible.

## Requirements

### REQ-UI-GITHUB-PR-REVIEW-ACTIONS-001: GitHub PR Review Actions

**Intent:** Pull-request authors can see dismissed GitHub reviews in Kandev, but must leave Kandev to ask that reviewer to review again. Re-requesting a review should be available where the dismissed review is already visible.

#### Acceptance criteria

- **AC-UI-GITHUB-PR-REVIEW-ACTIONS-001.1:** An open GitHub pull request shows a **Re-request review** action on each reviewer's latest review when that review is `DISMISSED`.
- **AC-UI-GITHUB-PR-REVIEW-ACTIONS-001.2:** The action identifies the reviewer in its accessible name and uses the dismissed review's author login as the new requested reviewer.
- **AC-UI-GITHUB-PR-REVIEW-ACTIONS-001.3:** A reviewer already present in GitHub's current `requested_reviewers` list is shown as pending and cannot be requested again.
- **AC-UI-GITHUB-PR-REVIEW-ACTIONS-001.4:** A current request takes precedence over dismissed review history: after a successful re-request, the reviewer is shown once as pending even though the old dismissed review remains in GitHub's review history.
- **AC-UI-GITHUB-PR-REVIEW-ACTIONS-001.5:** While a request is in flight, repeated submission for that reviewer is blocked.
- **AC-UI-GITHUB-PR-REVIEW-ACTIONS-001.6:** Success refreshes pull-request feedback and shows a success notification. Failure leaves the action available and shows the returned error.
- **AC-UI-GITHUB-PR-REVIEW-ACTIONS-001.7:** Desktop and tablet use the existing GitHub PR detail panel. On phones, a task with a linked GitHub PR exposes the existing **Review** bottom-navigation destination and renders the same PR detail behavior in a full-height, single-scroll surface.
- **AC-UI-GITHUB-PR-REVIEW-ACTIONS-001.8:** When a task has multiple linked GitHub PRs, the phone Review surface uses the shared task PR selection so the user can reach the dismissed review on the intended PR.

## Migrated source detail

## Why

Pull-request authors can see dismissed GitHub reviews in Kandev, but must leave
Kandev to ask that reviewer to review again. Re-requesting a review should be
available where the dismissed review is already visible.

## What

- An open GitHub pull request shows a **Re-request review** action on each
  reviewer's latest review when that review is `DISMISSED`.
- The action identifies the reviewer in its accessible name and uses the
  dismissed review's author login as the new requested reviewer.
- A reviewer already present in GitHub's current `requested_reviewers` list is
  shown as pending and cannot be requested again.
- A current request takes precedence over dismissed review history: after a
  successful re-request, the reviewer is shown once as pending even though the
  old dismissed review remains in GitHub's review history.
- While a request is in flight, repeated submission for that reviewer is
  blocked.
- Success refreshes pull-request feedback and shows a success notification.
  Failure leaves the action available and shows the returned error.
- Desktop and tablet use the existing GitHub PR detail panel. On phones, a task
  with a linked GitHub PR exposes the existing **Review** bottom-navigation
  destination and renders the same PR detail behavior in a full-height,
  single-scroll surface.
- When a task has multiple linked GitHub PRs, the phone Review surface uses the
  shared task PR selection so the user can reach the dismissed review on the
  intended PR.

## API surface

Kandev exposes:

```http
POST /api/v1/github/prs/:owner/:repo/:number/requested-reviewers
Content-Type: application/json

{"reviewers":["octocat"]}
```

- `reviewers` contains one or more non-empty GitHub user logins.
- Success returns `200` with `{"requested": true}`.
- Invalid PR numbers or an empty/invalid reviewer list return `400`.
- The backend requests the reviewers through GitHub's pull-request review
  request API and invalidates cached feedback/status for that PR before the UI
  refreshes.

## Permissions

GitHub remains the authorization authority. The action uses the active
workspace's GitHub credentials and requires GitHub pull-request write
permission. Kandev does not elevate or emulate that permission.

## Failure modes

- Missing GitHub configuration fails without changing local state and surfaces
  a useful error.
- GitHub permission, eligibility, validation, rate-limit, or transport failures
  return a non-success response; the UI shows the error and permits retry.
- A failed request does not mark the reviewer pending.
- A successful request must not be hidden by stale PR feedback; affected
  feedback and status cache entries are evicted before refresh.

## Scenarios

- **GIVEN** an open PR whose latest review from `octocat` is `DISMISSED` and
  `octocat` is not currently requested, **WHEN** the user activates
  **Re-request review from octocat**, **THEN** GitHub receives a review request,
  Kandev shows success, and refreshed feedback shows `octocat` pending.
- **GIVEN** a dismissed review whose author is already in
  `requested_reviewers`, **WHEN** the reviews section renders, **THEN** the
  author appears once as pending and no re-request action is available.
- **GIVEN** a review whose latest state is `APPROVED`,
  `CHANGES_REQUESTED`, `COMMENTED`, or `PENDING`, **WHEN** the reviews section
  renders, **THEN** no re-request action is shown for that review.
- **GIVEN** a closed or merged PR with a dismissed review, **WHEN** its detail
  panel renders, **THEN** no re-request action is available.
- **GIVEN** GitHub rejects a re-request, **WHEN** the action completes,
  **THEN** Kandev shows the error, does not show the reviewer as pending, and
  leaves the action available for retry.
- **GIVEN** a phone-sized task view with a linked GitHub PR and dismissed
  review, **WHEN** the user opens **Review** and re-requests that reviewer,
  **THEN** the same pending outcome is visible without horizontal document
  overflow and all action targets remain touch-usable.

## Out of scope

- Choosing arbitrary new reviewers or teams.
- Dismissing GitHub reviews.
- Changing GitLab merge-request reviewer behavior.
- Re-requesting reviews on closed or merged pull requests.

## Implementation plan

[Implementation plan](../../../plans/github-pr-review-rerequest/plan.md)
