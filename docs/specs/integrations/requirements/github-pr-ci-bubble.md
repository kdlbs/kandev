---
status: active
system: integrations
created: 2026-11-10
owners:
  - kandev
---

# GitHub PR CI Bubble Requirements

## Overview

Draft pull requests use a muted status icon. Users need to see CI problems and waits without opening the status details.
Successful CI must not add a green bubble. Skipped checks must not produce a false failure, progress indicator, or success claim.

The integrations system owns this capability because GitHub check and workflow evidence define the displayed state.
The existing [task status disclosure](../../ui/requirements/pr-task-status-summary.md) owns the shared disclosure interaction.

This requirement extends [workflow attention](github-workflow-attention.md), including its task approval badge.
The November 11 amendment restores the shared conflict/CI position and conflict priority requested by the user.
The amendment is implemented in the [corner-badge fix](../../../plans/pr-conflict-corner-badge/plan.md).
The earlier CI-bubble delivery remains implemented. This amendment changes badge presentation, not CI evidence or classification.
Workflow evidence eligibility, stale-observation handling, and automation protections remain applicable.

## Terminology

- **CI bubble:** A small upper-right status marker on a task's PR status glyph.
- **Skipped check:** A completed check whose conclusion is `skipped`.
- **All skipped:** A nonempty detailed check observation containing only completed skipped checks.
- **Workflow approval:** Approval needed to run a GitHub Actions workflow. This differs from approval of the pull request review.

## Requirements

### REQ-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001: CI visibility on task PR indicators

**Intent:** Show actionable CI states while keeping successful and skipped-only CI visually quiet.

#### Acceptance criteria

- **AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.1:** For an open draft PR, the main icon shall remain muted. Its corner badge shall use the same conflict/CI priority as other PR glyphs.
- **AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.2:** Without a merge conflict, CI failures shall show a red cross bubble. Queued or running CI shall show an amber progress ring.
- **AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.3:** Without a merge conflict, confirmed workflow approval shall show an amber lock, including approval gates with no started checks. Other confirmed workflow attention shall show an amber exclamation mark.
- **AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.4:** Successful CI shall add no CI bubble, including results that combine passed and completed skipped checks. An independent conflict badge shall remain eligible.
- **AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.5:** All-skipped results shall add no CI bubble. Available detailed observations shall identify all-skipped results without calling them passed or unstarted.
- **AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.6:** Completed skipped checks shall contribute neither failures nor pending work. Available skipped counts shall remain separate from passed counts in the details.
- **AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.7:** Without a merge conflict, the bubble shall prioritize failure, workflow approval, other CI attention, then queued or running work. Details shall retain each independent condition.
- **AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.8:** A task with multiple linked PRs shall aggregate CI over open PRs only. Details shall attribute conditions to their PRs.
- **AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.9:** Merge conflicts shall use a small red triangle in the upper-right bubble of the PR glyph. Conflicts shall take priority over every CI marker, without a trailing triangle or extra inline width.
- **AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.10:** Existing auto-fix and auto-merge dots shall remain distinguishable in their current corners. When conflicts hide a CI marker, accessible names and status details shall retain both conditions.
- **AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.11:** Sidebar rows, task lists, cards, task pickers, subtask indicators, the task top bar, and the composer CI control shall use consistent CI classification.
- **AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.12:** Compact rows shall apply conflict/CI badge priority before hover or tap, including after reload. Rendering shall not add provider requests, polling, or session subscriptions.
- **AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.13:** Authoritative updates shall clear obsolete bubbles after CI resolution, head changes, closure, merge, or unlinking. Older compact or full observations shall not restore an obsolete bubble.
- **AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.14:** Missing checks, unavailable evidence, review requests, and mergeability alone shall not imply failed CI, running CI, or workflow approval. Bubble absence shall not imply that CI passed.
- **AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.15:** Desktop hover and keyboard focus shall explain the state. Phone tap shall use the existing PR drawer without task navigation or an extra badge target.
- **AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.16:** The indicator and details shall use localized text and accessible names. Light, dark, phone, and reduced-motion views shall preserve the status meaning without clipping or horizontal overflow.
- **AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.17:** Display aggregation and skipped metadata shall not authorize merging or change CI repair eligibility. Cancelled, neutral, stale, and superseded checks shall retain their existing action semantics.
- **AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.18:** After authoritative conflict resolution, the corner shall immediately show the currently eligible CI marker, or no marker for `none`. This presentation change shall require no additional provider request.

## Out of scope

- GitHub approval, rerun, cancellation, or merge actions.
- Provider polling, caching, credentials, or check-selection redesign.
- GitLab, plugin-provider indicators, and PR navigation icons that do not represent task status.
- Replacing the existing PR disclosures or changing task navigation.
- Changing the main PR status-color precedence, review rules, or merge-queue behavior.

## Implementation plan

- [GitHub PR CI bubble](../../../plans/github-pr-ci-bubble/plan.md)
- [Restore the PR conflict corner badge](../../../plans/pr-conflict-corner-badge/plan.md)
