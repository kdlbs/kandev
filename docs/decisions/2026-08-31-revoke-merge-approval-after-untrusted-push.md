# ADR-2026-08-31-revoke-merge-approval-after-untrusted-push: Revoke merge approval after an untrusted pull-request push

**Status:** accepted
**Date:** 2026-08-31
**Area:** workflow, security

## Context

The `ready-to-merge` label is used by a coordinator as the maintainer's
approval to repair CI, enqueue, or automatically merge a pull request. If a
user without write access pushes a new commit after that approval, the label
and any active merge operation describe an older revision. Leaving them active
can allow a changed external contribution to proceed without a fresh
maintainer review.

The workflow must also handle an external pull request that a maintainer
updates. That push is trusted for this purpose, even though the pull request
author may not have repository access.

## Decision

Add a base-controlled `pull_request_target` workflow for `synchronize` events.
Use `github.event.sender` as the pusher identity and resolve the sender's
current repository permission through GitHub's collaborator permission API.
Only `write` and `admin` are trusted. Treat missing, unknown, or failed
permission results as untrusted.

For an untrusted event whose label snapshot contains the exact
`ready-to-merge` label, remove that label. Then read current GitHub pull-request
state and independently disable an active auto-merge request and dequeue an
active merge-queue entry. Do not check out or execute pull-request content.
Give the job only `pull-requests: write`, pin all external actions by commit
SHA, and serialize runs per pull request without canceling older runs.

## Consequences

- A contributor push requires a fresh `ready-to-merge` decision before the
  coordinator can continue.
- A maintainer can update an external contributor's branch without resetting
  the approval.
- Existing auto-merge and merge-queue state is actively withdrawn instead of
  relying on the coordinator to notice the label change.
- A transient permission lookup failure can conservatively revoke approval from
  a maintainer push. The workflow exposes that failure so the result can be
  investigated.
- The workflow has a pull-request write token and must remain free of checkout,
  arbitrary scripts, and contributor-controlled inputs that could execute.

## Alternatives Considered

- **Use the pull-request author's `author_association` or head repository:**
  Rejected because the requested exemption follows the person who pushed the
  new commit, and those fields do not provide the current effective repository
  permission for that person.
- **Remove only the label:** Rejected because a coordinator may already have
  enabled auto-merge or placed the pull request in the merge queue.
- **Use a normal `pull_request` workflow:** Rejected because fork events do not
  provide the trusted write-capable token needed to mutate the base pull
  request.
- **Maintain a static maintainer allowlist:** Rejected because repository,
  team, organization, and enterprise grants can change independently of a
  workflow file.
