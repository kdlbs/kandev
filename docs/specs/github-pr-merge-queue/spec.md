---
status: building
created: 2026-08-17
owner: Kandev
---

# GitHub PR Merge Queue

## Why

Users can merge an eligible GitHub pull request from Kandev, but repositories
that require GitHub's merge queue leave the same pull request looking blocked
and force the user to leave Kandev. The merge action should respect the
repository's GitHub rules and complete through the appropriate direct or queued
path.

## What

- An open, approved pull request with successful required checks exposes its
  merge action in the existing GitHub PR detail and compact status surfaces,
  including when GitHub reports it blocked only because the base branch uses a
  merge queue.
- Activating the action asks GitHub to choose the appropriate merge behavior:
  merge immediately where permitted or add the pull request to the configured
  merge queue.
- A direct merge reports that the pull request merged. An accepted queued merge
  reports that the pull request was added to the merge queue and prevents
  repeated submission while local PR state refreshes.
- The action remains unavailable for drafts, conflicts, changes requested,
  failed or incomplete required checks, and unmet required reviews.
- GitHub remains authoritative for final eligibility. A rejected request leaves
  the action retryable and shows GitHub's error without claiming that the pull
  request merged or entered the queue.
- Desktop and mobile use the existing PR detail behavior. On phones, the action
  remains available through the full-height Review surface with a touch-sized
  target and no horizontal document overflow.

## API Surface

Kandev retains the existing endpoint and extends its success response:

```http
PUT /api/v1/github/prs/:owner/:repo/:number/merge?workspace_id=:workspaceId
Content-Type: application/json

{"merge_method":"squash"}
```

- `merge_method` remains optional and accepts `merge`, `squash`, or `rebase`.
- Kandev submits the request through GitHub's asynchronous merge API with
  `merge_action=default`, allowing GitHub to select direct merge or merge queue.
- Success returns `200` with one of:

```json
{"status":"merged"}
{"status":"queued"}
```

- A `pending` response includes a UUID. Kandev polls that request, including
  the UUID returned by an existing-request `409`, until GitHub reports
  `merged`, `enqueued`, or `failed`.
- Only `enqueued` maps to `queued`. An already-merged pull request maps to
  `merged`, while `failed` remains an error that the user can retry.

## Permissions

The action uses the active workspace's personal-write GitHub routing and
requires the same GitHub content/pull-request permissions as the provider's
merge API. Kandev does not bypass repository rules or elevate the user.

## Failure Modes

- Missing GitHub credentials or required permissions return a non-success
  response, surface a useful error, and leave the action retryable.
- GitHub validation, readiness, rate-limit, or transport failures do not change
  local PR state and do not show a success notification.
- An unrecognized successful provider status fails closed rather than claiming
  that the pull request merged or entered the queue.
- After an accepted request, Kandev invalidates cached PR feedback/status and
  refreshes the linked pull request; the GitHub poller remains authoritative
  for its eventual merged state.

## Scenarios

- **GIVEN** an approved open PR with successful required checks on a branch
  without a required merge queue, **WHEN** the user activates the merge action,
  **THEN** GitHub merges it and Kandev reports that the PR merged.
- **GIVEN** an approved open PR with successful required checks on a branch
  that requires a merge queue, **WHEN** the user activates the merge action,
  **THEN** GitHub accepts it into the queue and Kandev reports that the PR was
  added to the merge queue.
- **GIVEN** a PR already in the merge queue, **WHEN** the merge request is
  repeated, **THEN** Kandev treats GitHub's idempotent response as queued and
  does not report a failure.
- **GIVEN** a draft, conflicted PR, failed checks, changes requested, or missing
  required approvals, **WHEN** the PR surface renders, **THEN** no merge or
  queue action is available.
- **GIVEN** GitHub rejects an otherwise eligible merge request, **WHEN** the
  action completes, **THEN** Kandev shows the provider error and leaves the
  action available for retry.
- **GIVEN** a phone-sized task view with an eligible queue-required PR, **WHEN**
  the user opens Review and activates the action, **THEN** the queued outcome is
  visible, the action is touch-usable, and the document has no horizontal
  overflow.

## Out of Scope

- Displaying queue position, estimated merge time, or the full merge queue.
- Removing a pull request from the merge queue.
- Selecting between direct merge and merge queue when GitHub policy permits
  both; GitHub's `default` behavior remains authoritative.
- Changing Kandev's independent CI auto-merge automation setting.
- GitLab merge-request behavior.

## Implementation Plan

[Implementation plan](../../plans/github-pr-merge-queue/plan.md)
