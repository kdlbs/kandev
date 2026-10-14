---
status: active
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
- **AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.5:** After approval, cancellation, rerun, or a head change, the next successful provider refresh shall reconcile the reason. Explicit refresh shall bypass cached Actions observations. Reloads shall retain the latest stored observation.
- **AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.6:** Missing checks or unstable mergeability alone shall not imply approval. Unavailable provider evidence shall remain distinct from an observed absence of approval gates.
- **AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.7:** When GitHub requires an action but the reason is ambiguous, Kandev shall show "Workflow needs attention" instead of an approval claim.
- **AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.8:** An approval reason shall replace unexplained "unstable" copy in the affected summary. Independent conflicts, reviews, queue state, and actual failures shall remain visible.

### REQ-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-002: Bounded Actions caching

- **AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-002.1:** Equivalent Actions reads shall share cached results and concurrent fetches within one credential scope, repository, and head SHA. Different credentials shall never share results.
- **AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-002.2:** Background observations shall expire after 30 seconds for empty, pending, running, or attention-required results. Nonempty completed results without attention requirements shall expire after 5 minutes, including success and failure.
- **AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-002.3:** A head change, credential change, or explicit refresh shall prevent reuse of the affected stale observation. Same-SHA reruns shall become visible after expiry and the next successful poll.
- **AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-002.4:** Provider errors shall not become cached absence. Cached observations shall not become merge eligibility evidence. Classification shall remain specific to each PR, including fork identity.

### REQ-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003: Task approval badge

**Intent:** Identify a workflow approval gate directly from a task's PR status icon.

**User story:** As a maintainer, I want an approval badge so I can find workflows that need my action.

#### Acceptance criteria

- **AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.1:** When an open linked PR has approval evidence for its current head, its task icon shall show an amber padlock badge. The badge shall appear before the user opens the status disclosure.
- **AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.2:** The badge shall occupy the upper-right warning position and retain the PR icon's status color. It shall use amber `#D97706` in light mode and `#FBBF24` in dark mode, with a background halo. Existing automation dots shall remain visible in their current positions.
- **AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.3:** When conflicts and approval coexist, the red conflict triangle shall occupy the shared warning position. The accessible name and status disclosure shall still identify both conditions.
- **AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.4:** Without prior eligible approval evidence, missing checks, pending checks, unavailable evidence, and ambiguous action-required evidence shall not produce the approval badge. The badge shall disappear after an authoritative observation clears approval, a head change, PR closure, PR merge, or unlinking.
- **AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.5:** A failed refresh shall retain a last-known approval badge only for the same open PR head. The detailed disclosure shall retain the existing stale-evidence explanation.
- **AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.6:** When a task has multiple linked PRs, approval on any open PR shall contribute to its badge. Conflicts on any open PR shall take visual priority. Details shall attribute each condition to its PR, and terminal siblings shall contribute neither condition.
- **AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.7:** Compact task rows, hydrated task rows, and rows restored after reload shall agree on approval visibility. Rendering a badge shall not add provider reads or session subscriptions.
- **AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.8:** The existing desktop hover/focus disclosure and phone tap disclosure shall explain approval with localized text. The icon's accessible name shall include the approval reason. Touch users shall reach the explanation through the existing PR drawer without hover, task navigation, or layout overflow caused by the badge.

## Out of scope

- Approving, rerunning, cancelling, or merging GitHub workflows or pull requests.
- Changing repository policy, credentials, or permissions automatically.
- Deployment environment approvals and provider-neutral automation redesign.
- Reclassifying existing third-party check conclusions without workflow evidence.

## Implementation plans

- [Workflow attention](../../../plans/github-workflow-attention/plan.md)

- [Polling efficiency](../../../plans/watch-task-cleanup/plan.md)
- [Task approval badge](../../../plans/github-workflow-approval-badge/plan.md)

- [Preserve PR details after approval clears](../../../plans/pr-task-disclosure-negative-projection/plan.md).
