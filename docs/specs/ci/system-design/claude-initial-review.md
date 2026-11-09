---
status: current
system: ci
requirements:
  - REQ-CI-CLAUDE-INITIAL-REVIEW-001
---

# Claude initial review design

## Ownership and boundaries

CI owns workflow admission, event filtering, and concurrency. The
[integration requirement](../../integrations/requirements/claude-fork-review-allowlist.md)
retains fork author authorization and explicit mention behavior. The existing
[workflow input design](privileged-workflow-inputs.md) retains trust boundaries.
This design does not move a review to another trigger or token identity.

| Requirement | Design sections |
| --- | --- |
| REQ-CI-CLAUDE-INITIAL-REVIEW-001 | Events and serialization; Admission evidence; Failure and reporting; Verification |

## Events and serialization

In `.github/workflows/claude-code-review.yml`, extend `pull_request` activity
types from `opened` to `opened, synchronize`. Preserve the same-repository job
gate. Keep `pull_request_target: [opened, labeled]`, fork authorization, and the
manual `.github/workflows/claude.yml` unchanged. A fork push can create an empty
pull-request run, but cannot invoke either review job through the new path.

Keep the existing event-and-PR concurrency group. Set cancellation false for
the `pull_request` event so a newer push cannot terminate the admitted review.
Retain the current cancellation behavior for target-triggered fork runs. GitHub
can replace a pending run with a newer pending event. The surviving event must
repeat admission after the active run releases the group.

Keep the current same-repository checkout and App-token review action. Add a
read-only admission step before the action, plus `actions: read` for its history
queries. Do not grant the model any additional tools or permissions. The review
action runs only when the admission step explicitly returns `should_review=true`.
Bind that action to the event head with `CLAUDE_REVIEW_HEAD_SHA`; before
reviewing or posting, it rechecks the current PR head and stops if it differs.

## Admission evidence

Add `.github/scripts/claude-initial-review.cjs`, loaded by a pinned
`actions/github-script` step. Its proposed `shouldStartInitialReview` function
receives the Octokit client and validated repository, PR, event-head, and run IDs.
It returns an admission decision and a bounded reason.

1. Read the current PR, and re-read it immediately before a positive decision.
   Skip a closed PR or an event whose head no longer matches.
2. List runs of `claude-code-review.yml` with event `pull_request`, starting at
   the PR creation time. Request PR associations explicitly. Do not restrict
   history to the current commit SHA.
3. Select runs associated with this PR number. Exclude the current run ID.
   That exclusion preserves explicit GitHub workflow reruns. Do not restrict
   matching history to one branch name.
4. Read every page of jobs for each selected run, with `filter: all` so a later
   skipped rerun cannot hide an earlier invocation.
5. Inspect job `claude-review-same-repo` and its existing action step,
   `Run Claude Code Review (App token, inline comments)`. A non-skipped step
   that started consumes the initial attempt, regardless of its final outcome.
   GitHub reports a skipped step as completed with conclusion `skipped`, so
   skip the conclusion before inspecting timestamps. A terminal job that never
   reached this step does not consume an attempt.
6. Admit only when **all** other associated retained runs contain no prior
   invocation and every required history page was read successfully. Compare
   the collected record count with the declared total; a short or empty final
   page is incomplete evidence.
7. A pending run normally waits behind the current run and repeats admission
   when the concurrency group releases. A pending rerun is not ignored until
   its completed attempts have been inspected for a prior invocation.

A skipped target workflow, a different PR, or a run that stopped before the
review action is not prior review evidence. A mixed history with skipped runs
and one started action denies admission. Active or malformed evidence that
cannot distinguish these cases fails closed.

Bound job pagination to 20 pages of 100 records. The workflow-runs endpoint
exposes at most 1,000 results, so bound it to 10 pages and fail when the declared
total exceeds 1,000. If another page exists at the bound, fail rather than
treating partial history as empty. Propagate API and schema errors before the
model step. Use API numeric IDs for matching. Do not evaluate PR metadata as
JavaScript or shell code.

## Persistence and compatibility

Retained GitHub Actions history supplies the attempt record. No labels, marker
comments, repository variables, or application database state are added. Runs
from the previous workflow already expose the same job and action-step names.
Preserve those names so an existing successful review suppresses catch-up.

History deletion by an administrator can remove that record. This policy does
not promise deduplication after deliberate history deletion. Existing histories
that cannot be read fail closed instead of causing another model invocation.

An action invocation that fails during installation still consumes the attempt.
Maintainers use the explicit review entry point after a failed initial attempt.
An explicitly rerun workflow is a human action outside automatic deduplication.

## Failure and reporting

Log only the PR number, relevant run IDs, decision, and reason. Distinguish
`no_prior_attempt`, `prior_attempt_started`, `stale_head`, and `closed_pr`.
History errors fail the job with a bounded diagnostic. A normal duplicate skip
does not assert that the earlier review succeeded or published findings.

The manual action can currently finish successfully without publication. Its
custom prompt selects agent mode, and the pinned SDK accepts a successful
result even when it reports tool denials. That independent publishing defect
does not change admission evidence: an invocation still consumes one attempt.

## Alternatives

Adding `synchronize` without admission causes repeated paid reviews. Moving
same-repository reviews to `pull_request_target` changes their token and author
identity. A sticky label or marker comment adds mutable PR state and a separate
claim lifecycle. Retained run evidence preserves the existing review paths and
their single-attempt policy with one read-only gate.

## Verification

Use Node's built-in test runner for the real admission module with injected
Octokit responses. Cover cross-head history, all attempts, multiple pages,
skipped jobs, mixed histories, API errors, ambiguous evidence, the 1,000-result
workflow-run API limit, and final head/state revalidation. Execute the
opening-conflict, resolution-push, and later-push sequence with a stub model
invocation and a shared run history. Assert exactly one invocation.

Extend the existing Python workflow contract for event types, action-step
gating, same-repository serialization, admission arguments, event-head binding,
and unchanged fork/manual routes. Register the new Node suite in
`.github/workflows/lint-action-pinning.yml`.
