---
status: active
system: ci
created: 2026-11-09
owners:
  - kandev
---

# Claude initial review requirements

## Overview

A same-repository pull request can open with merge conflicts. GitHub does not
schedule its ordinary pull-request workflows until those conflicts are resolved.
The initial Claude review must remain available without automatic review rounds
on every subsequent push. CI owns this admission policy and its workflow events.

## Terminology

- **Initial attempt:** The first automatic invocation of the Claude review action
  for a pull request. An invocation counts even if it later fails or is cancelled.
- **Catch-up:** An initial attempt admitted after a push because the opening did
  not reach the review action.

## Requirements

### REQ-CI-CLAUDE-INITIAL-REVIEW-001: Initial automatic review admission

**Intent:** Start the missing initial review after conflict resolution without
spending tokens on automatic follow-up rounds.

#### Acceptance criteria

- **AC-CI-CLAUDE-INITIAL-REVIEW-001.1:** When a same-repository pull request opens
  without an initial attempt, its automatic review shall remain eligible through
  the existing opening path.
- **AC-CI-CLAUDE-INITIAL-REVIEW-001.2:** When a push resolves opening conflicts,
  the system shall admit the missing initial attempt without a maintainer
  comment or approval label.
- **AC-CI-CLAUDE-INITIAL-REVIEW-001.3:** When any retained run for the same pull
  request already invoked its automatic review, subsequent pushes shall not
  invoke another review. This includes attempts on older commits and attempts
  that failed or were cancelled. A skipped action is not an invocation.
- **AC-CI-CLAUDE-INITIAL-REVIEW-001.4:** Concurrent events for the same pull request
  shall not admit duplicate automatic attempts. A later push shall not cancel
  an active automatic attempt. An obsolete event shall not start a review after
  its head has changed.
- **AC-CI-CLAUDE-INITIAL-REVIEW-001.5:** When prior-attempt evidence is unavailable,
  incomplete, or ambiguous, admission shall fail visibly before invoking Claude.
  A normal skip shall identify its reason without implying a completed review.
- **AC-CI-CLAUDE-INITIAL-REVIEW-001.6:** The correction shall preserve fork approval,
  explicit manual reviews, action pinning, model selection, and existing
  authentication and publication identities.

## Exclusions

- Requiring an automatic review after each push, reopening, or ready transition.
- Automatic retries after an initial action invocation fails.
- Enforcing publication by the separate manual mention workflow.
- Treating an explicit workflow rerun as an ordinary automatic webhook event.
- Preserving deduplication after an administrator deliberately deletes review
  run history. Removed history cannot serve as retained evidence.
- Changing contributor authorization, provider credentials, or review findings.

## Implementation plans

- [Initial review catch-up](../../../plans/claude-initial-review-catch-up/plan.md)
