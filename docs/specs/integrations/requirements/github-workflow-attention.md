---
status: draft
system: integrations
created: 2026-09-10
owners:
  - kandev
---

# GitHub Workflow Attention Requirements

## Overview

Users need to distinguish workflows that require approval from tests that failed.
The integration system owns this provider state and its task presentation.
The shared [task summary](../../ui/requirements/pr-task-status-summary.md) owns disclosure layout and interaction.

## Requirements

### REQ-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001: Workflow attention

**Intent:** Show why CI cannot proceed, including workflows with no check results.

#### Acceptance criteria

- **AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.1:** When a current-head workflow requires maintainer approval, Kandev shall show that reason even when no checks exist.
- **AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.2:** Approval-only workflows shall not count as failed, passed, or running checks. They shall not trigger CI repair rounds or enable automatic merging.
- **AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.3:** Desktop task summaries and the existing phone PR drawer shall show the same localized reason. Detailed surfaces shall link to GitHub.
- **AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.4:** When approval and failed checks coexist, Kandev shall show both. Each linked PR shall retain its own status.
- **AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.5:** After approval, cancellation, rerun, or a head change, the next successful refresh shall reconcile the reason. Reloads shall retain the latest stored observation.
- **AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.6:** Missing checks or unstable mergeability alone shall not imply approval. Unavailable provider evidence shall remain distinct from an observed absence of approval gates.
- **AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.7:** When GitHub requires an action but the reason is ambiguous, Kandev shall show "Workflow needs attention" instead of an approval claim.
- **AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.8:** An approval reason shall replace unexplained "unstable" copy in the affected summary. Independent conflicts, reviews, queue state, and actual failures shall remain visible.

## Out of scope

- Approving, rerunning, cancelling, or merging GitHub workflows or pull requests.
- Changing repository policy, credentials, or permissions automatically.
- Deployment environment approvals and provider-neutral automation redesign.
- Reclassifying existing third-party check conclusions without workflow evidence.

## Implementation plans

- [Workflow attention](../../../plans/github-workflow-attention/plan.md)
