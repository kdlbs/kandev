---
created: 2026-10-29
status: complete
requirements:
  - REQ-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001
  - REQ-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-002
  - REQ-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003
  - REQ-INTEGRATIONS-GITHUB-PR-POLLING-002
  - REQ-INTEGRATIONS-GITHUB-RATE-002
system_design:
  - ../../specs/integrations/system-design/github-workflow-attention.md
  - ../../specs/integrations/system-design/github-pr-discovery-health.md
  - ../../specs/integrations/system-design/github-rate-limit-coordination.md
legacy_specs: []
---

# Implementation Plan: Workflow approval freshness

## Overview

Make the sidebar's workflow-approval badge follow GitHub on the one-minute PR
monitoring cadence. First stop unavailable job evidence from becoming a generic
"needs attention" claim. Then move monitored workflow evidence from a per-head
REST fan-out to a dedicated GraphQL phase. Finally resume that phase through the
existing PR-monitor continuation when local admission defers it.

## Evidence and assumption check

The user reported fork PRs whose sidebar icon showed pending CI (yellow) while
GitHub displayed "8 workflows awaiting approval" (PR #4940). The investigation
used read-only state from the running instance on 2026-10-29.

- GitHub listed 8 `pull_request` runs for #4940's head from fork
  `dreampiler/kandev`, each `completed` / `action_required`, with an empty
  `pull_requests` association and zero jobs. The classifier maps that shape to
  `approval_required`.
- `github_task_prs.workflow_attention` for #4940, #4889, and #4960 held
  `{"state":"unknown","stale":true,"runs":[]}`. #4950, #4952, and #4956 held
  `action_required`; GitHub showed 9 to 12 jobless fork approval gates for each.
  `statussummary.workflowApprovalRequired` accepts only `approval_required`, so
  none of those tasks showed the badge.
- Backend logs held about 2,370 `workflow attention read unavailable` entries
  that day, roughly 21 per one-minute poll tick. All were local admission
  deferrals: 2,342 `background_in_flight` and 24 `interactive_priority`. None
  was a GitHub error.
- `/debug/vars` recorded 16,503 Core `background_in_flight` and 4,309 Core
  `background_pacing` deferrals over 13.2 hours of uptime. GitHub reported only
  122 Core requests used in the current hour, so provider quota is not the limit.
- Numbered watches were all checked within the last minute, so status other
  than workflow attention stays on the one-minute cadence.

Root cause: the poller requests non-blocking background admission
(`poller.go:117`, since #3143). `enrichBatchedWorkflowAttention` starts up to
four concurrent `ListWorkflowRuns` reads, then per-run job reads.
`RateAdmission.tryAcquireBackground` admits one background request per
principal and resource, then paces the next by at least one second. The other
requests fail at once with `AdmissionDeferredError`. The enrichment records a
failed run read as `unknown`. A failed job read makes the classifier choose the
generic `action_required` reason. Nothing resumes that work, so a PR can wait
many ticks for one admitted read. #4920 added continuation for the numbered and
searching GraphQL phases only.

Probes on 2026-10-29 settled the evidence source:

| Probe | Result |
| --- | --- |
| Production 50-PR status batch, unchanged | Cost 3, no errors |
| Same batch plus `checkSuites { workflowRun }` | HTTP 502 or truncated body |
| 20 PRs, `checkSuites` with `workflowRun` and `checkRuns` | 360 to 628 "Resource limits exceeded" errors |
| Dedicated 20- and 30-PR `checkSuites { workflowRun }` query | Cost 1, no errors, three runs each |
| `nodes(ids:)` with LATEST `checkRuns` totalCount for 10 suites | Cost 1, all zero for a waiting fork PR |

Confirmed intent: the badge tells the maintainer whether CI needs approval,
refreshed about every minute. Integrations owns the provider evidence and its
storage. The task-summary projection and icon rendering keep their contracts.
No UI, copy, or schema change is needed.

## Requirement conformance

The implementation violates active criteria:

- AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.1 and -003.1: approval evidence
  exists on GitHub but is not shown.
- AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.6: unavailable job evidence
  becomes a generic attention claim.
- AC-INTEGRATIONS-GITHUB-PR-POLLING-002.5 and -002.6: local deferral of Actions
  reads discards pending work instead of resuming at the admission boundary.

The requirement gained two criteria that make the repaired behavior testable:

- AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.9: one-minute cadence under a
  shared principal, resumption at the boundary, and no stale marking.
- AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.10: unavailable job evidence
  claims neither approval nor generic attention.

The workflow-attention system design gained a
[monitored workflow phase](../../specs/integrations/system-design/github-workflow-attention.md#monitored-workflow-phase).
The discovery design's continuation section now names that phase.
RATE-002.2, .3, and .7 stay unchanged: interactive priority and pacing remain.
No new ADR is required; the accepted rate-coordination decision still governs.

## Scope

### In scope

- Mark an observation `unknown` when any needed job read fails (REST and GraphQL).
- Add the dedicated suites query, the suite-ID job-count query, and their decoders.
- Run a third, head-guarded workflow phase after status apply in the batched path.
- Remove the concurrent REST enrichment from the batched GraphQL path.
- Serial REST fallback for unsupported GraphQL or an invalid suites query.
- Record deferred workflow work in the PR-monitor pending inventory and resume it alone.
- Update `apps/backend/internal/github/AGENTS.md` for the third phase.

### Out of scope

- UI, copy, locale, projection, or wire-field changes. The badge already
  renders from `workflow_approval_required`.
- Changing admission, pacing, reserve, or interactive priority.
- Changing the 15/30-minute idle cadence for branches without a PR. That is
  AC-INTEGRATIONS-GITHUB-PR-POLLING-001.1 behavior, confirmed in this investigation.
- Paging check suites beyond 100 per head, approving workflows, or GitHub App
  permission changes.
- Live-instance repair, restarts, deployment, commit, push, or PR creation.

## Technical approach

Task 01 changes `classifyWorkflowAttentionWithJobs` in `workflow_attention.go`.
A job-read error for any selected `action_required` run returns `unknown` for
the head, so `resolveTaskPRWorkflowAttention` preserves same-head evidence as stale.

Task 02 adds `graphql_workflow_suites.go` for the two queries and their
conversion to `WorkflowRun`. Add `service_pr_workflow_phase.go` for the phase
runner, the `batched-workflow:` singleflight and progress key, and the
head-guarded store write. Statuses from `fetchBatchedNumberedPhase` and
`fetchBatchedSearchingPhase` stop calling `enrichBatchedWorkflowAttention`, so
their `WorkflowAttentionPopulated` stays false and storage keeps the prior
observation. `prWatchSyncOutcome` gains unfinished workflow work with its typed
deferral. The REST collector stays for single-PR reads and serves the serial fallback.

Task 03 teaches `poller_pr_continuation.go` a workflow-phase marker on pending
targets. A due wake for such a target calls only the workflow phase. Status
phases are neither re-fetched nor republished. Pruning, credential fences,
and stop semantics stay unchanged.

### Request budget per tick, one principal with 30 open PRs

| Path | Core REST | GraphQL |
| --- | --- | --- |
| Current | 30 run reads plus 1 job read per action-required run, mostly rejected | status chunks |
| Planned | 0 | status chunks + 2 suite queries + at most 1 job-count query |

## Tests

| Acceptance | Regression evidence |
| --- | --- |
| WORKFLOW-ATTENTION-001.6, .10 | `TestWorkflowAttention_UnavailableJobEvidenceIsUnknown` in `workflow_attention_test.go` (Task 01) |
| WORKFLOW-ATTENTION-001.1, .9; 003.1 | `TestWorkflowPhase_ClassifiesManyHeadsWithoutRESTActions` in `service_pr_workflow_phase_test.go` (Task 02) |
| WORKFLOW-ATTENTION-001.9; POLLING-002.5 | `TestWorkflowPhase_DeferralLeavesStoredObservationAndReturnsPending` (Task 02) |
| WORKFLOW-ATTENTION-001.5, .6 | `TestWorkflowPhase_HeadChangeAndPageOverflowAreUnknown` (Task 02) |
| WORKFLOW-ATTENTION-001.9; POLLING-002.6, .7; RATE-002.3 | `TestPRMonitorWorkflowAttention_ApprovalVisibleWithinOneMinuteUnderSharedAdmission` in `poller_workflow_attention_test.go` (Task 03) |
| WORKFLOW-ATTENTION-003.7 | Same test, asserting task status summaries through the real projector |

The Task 03 regression reproduces the reported defect end to end. It uses
30 fork PRs on one principal, real `RateAdmission` with non-blocking poller
admission, a fake GraphQL executor, SQLite, and `testing/synctest`. Each PR
has a head waiting for approval, and the test asserts every task summary shows
approval before the next inventory tick. Each work order records its own RED
failure before its production edits:

- Task 01: on current code the classifier returns `action_required`.
- Task 02: on current code the service issues REST Actions calls per head and
  leaves most rows `unknown` under real admission.
- Task 03: on Task 02 code the workflow phase defers behind the status batch's
  GraphQL pacing and is not resumed before the next tick.

## E2E tests

The E2E mock GitHub client has no GraphQL executor. Browser runs therefore
exercise the REST collector, and the existing specs remain the rendered
evidence for the unchanged badge:

- Desktop: `apps/web/e2e/tests/pr/pr-status-badge.spec.ts`, case
  "explains a jobless fork workflow approval and recovers after refresh".
- Phone: `apps/web/e2e/tests/pr/mobile-pr-sidebar-automation-indicators.spec.ts`.

Task 03 reruns both. The GraphQL phase is covered by the backend tests above.
No ASCII preview is required because the rendered UI does not change.

## Work orders

- [x] [Task 01: Treat unavailable job evidence as unknown](task-01-job-evidence-unknown.md)
- [x] [Task 02: Read monitored workflow evidence through GraphQL](task-02-graphql-workflow-phase.md)
- [x] [Task 03: Resume deferred workflow evidence](task-03-workflow-phase-continuation.md)

Dependency order: 01, then 02, then 03. No delegation is authorized.
Each work order lists exact RED and GREEN commands.

## Risks

- GitHub GraphQL limits can change. Mitigation: a dedicated query, a 20-PR chunk,
  no nested `checkRuns` in step 1, and REST fallback on an invalid query.
- Suites lack run attempts and head-repository fields. Mitigation: they are read
  from the PR's own head commit; identity comes from the PR, and attempt is stored as 0.
  Nothing outside the attention payload consumes the attempt.
- A shared head SHA across two PRs maps the same suites to both. REST matching
  already accepted unassociated fork runs by head SHA, branch, and repository.
- GitHub Enterprise Server schema differences fall back to serial REST.

## Results

Implemented all three work orders. Job evidence is now unavailable rather than
generic attention, monitored Actions evidence comes from the dedicated GraphQL
suite and job-count queries, and deferred workflow work resumes through the
existing PR-monitor continuation. Review follow-ups keep resolved siblings on
partial GraphQL errors, associate converted suites with their PR so non-
`pull_request` events still match, retain rate-limit retry boundaries, and
reject empty-head workflow writes. Fixup evidence adds per-caller application
of shared evidence, retained REST PR identity and job deferrals, immediate
workflow refresh for completed status phases, and an independent workflow
retry boundary. The full `internal/github` package, targeted
race coverage, the summary projector package, the status-summary adapter tests,
and both Playwright regressions passed.

## Companion records

The workflow-attention, approval-badge, negative-projection, watch-task-cleanup,
and sidebar-PR-freshness packages keep their recorded scopes and results.
This package owns only the freshness repair.
