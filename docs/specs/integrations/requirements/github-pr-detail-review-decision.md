---
status: active
system: integrations
created: 2026-10-27
owners:
  - kandev
---

# GitHub PR Detail Review Decision Requirements

## Overview

A user inspecting a linked GitHub pull request needs its detail header to
reflect the binding reviews in the feedback history, even when a reviewer adds
an informational review later. Integrations owns this provider-specific
read-only outcome. The existing review-request action and phone navigation are
owned by [GitHub PR review actions](../../ui/requirements/github-pr-review-actions.md).
This is a bounded corrective contract pair restoring existing binding-review
semantics: the existing action requirement covers requesting reviewers, not
the overall decision derived from review history.

## Requirements

### REQ-INTEGRATIONS-GITHUB-PR-DETAIL-DECISION-001: Binding review decision in detail

**Intent:** Show the existing GitHub binding-review summary in the detail header
without treating informational review history as a replacement decision.

#### Acceptance criteria

- **AC-INTEGRATIONS-GITHUB-PR-DETAIL-DECISION-001.1:** When a reviewer approves
  and subsequently submits a comment or pending review, the detail header
  shall continue to show the approval; the later review body shall remain
  visible in the review history.
- **AC-INTEGRATIONS-GITHUB-PR-DETAIL-DECISION-001.2:** When an active binding
  change request remains for any reviewer, the detail header shall show changes
  requested, including after that reviewer submits a neutral review. A later
  binding approval by the same reviewer replaces that reviewer's change request;
  a later binding change request replaces that reviewer's approval.
- **AC-INTEGRATIONS-GITHUB-PR-DETAIL-DECISION-001.3:** When at least one
  binding approval remains and no active binding change request remains, the
  detail shall show approved, including alongside neutral review history from
  another reviewer. Neutral or dismissed history alone shall not fabricate an
  approval. Existing no-review fallback, current requested-reviewer display,
  and re-request action behavior shall remain available.
- **AC-INTEGRATIONS-GITHUB-PR-DETAIL-DECISION-001.4:** Desktop detail and the
  existing phone Review destination shall show the same derived decision and
  retain the review content and existing interactions.

## Out of scope

- Review submissions, GitHub permissions, merge eligibility and authorization.
- Backend status, counts, issue flags, watchers, persistence, and other providers.
- Review-request registry, lifecycle, layout, labels, and navigation changes.

## Related documents

- [System design](../system-design/github-pr-detail-review-decision.md).
- [Implementation plan](../../../plans/github-pr-detail-binding-decision/plan.md).
