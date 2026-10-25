---
status: draft
system: integrations
created: 2026-08-31
owners:
  - Kandev
---

# Pull request link copy actions Requirements

## Overview

The integration system owns the canonical links returned by external code-host
providers and the normalized feedback displayed in a change-request detail
surface. Users need to copy a pull request link or a link to one specific
comment without leaving Kandev. The shared UI system supplies the reusable
presentation, focus, and responsive interaction contract.

## Terminology

- **Change request:** A provider pull request, merge request, or equivalent
  reviewable code change.
- **Comment permalink:** The provider-supplied URL that opens the change
  request at one conversation or review comment.

## Requirements

### REQ-INTEGRATIONS-PR-LINK-COPY-ACTIONS-001: Pull request link copy actions

**Intent:** Users can share a linked pull request or a specific conversation or
review comment directly from its Kandev detail surface.

**User story:** As a reviewer, I want to copy the pull request or comment link
from Kandev, so that I can share the exact review context without searching for
it again on the provider site.

#### Acceptance criteria

- **AC-INTEGRATIONS-PR-LINK-COPY-ACTIONS-001.1:** When a GitHub pull-request
  detail surface has a non-blank canonical pull-request URL, its header shall
  expose an icon-only copy action near the pull-request identity, and activating
  it shall place that exact URL on the clipboard.
- **AC-INTEGRATIONS-PR-LINK-COPY-ACTIONS-001.2:** When a rendered GitHub
  conversation or review comment, including a reply, has a non-blank canonical
  comment permalink, its row shall expose a copy action, and activating it shall
  place that exact permalink on the clipboard.
- **AC-INTEGRATIONS-PR-LINK-COPY-ACTIONS-001.3:** After a successful copy, the
  activated action shall provide a localized tooltip and transient copied
  confirmation. A failed copy shall not report success, and the comment or
  pull-request content shall remain usable.
- **AC-INTEGRATIONS-PR-LINK-COPY-ACTIONS-001.4:** Copy actions shall remain
  available when the pull request is closed or merged; their availability shall
  not depend on an open-state action such as merge or approval.
- **AC-INTEGRATIONS-PR-LINK-COPY-ACTIONS-001.5:** On fine-pointer desktop
  surfaces, comment-row copy actions may be revealed on row hover but shall also
  be reachable through keyboard focus. On phone-sized surfaces, the actions
  shall not require hover and shall retain a touch target of at least 44 pixels
  in both dimensions. The shared detail surface shall keep its existing single
  scroll owner and shall not introduce horizontal page overflow.
- **AC-INTEGRATIONS-PR-LINK-COPY-ACTIONS-001.6:** When a provider feedback
  payload omits a comment permalink, Kandev shall render the existing comment
  content without offering a copy action for an empty or invented URL.

## Out of scope

- Copying a link as Markdown or copying a formatted title and URL pair.
- Adding a new persistence column or endpoint solely for comment permalinks.
- Delivering GitLab, Azure DevOps, or other provider-specific permalink mapping
  in this issue. The shared presentation can consume those providers later
  when their adapters expose the same normalized URL field.

## Implementation plan

[Implementation plan](../../../plans/pr-link-copy-actions/plan.md)
