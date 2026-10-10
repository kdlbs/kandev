---
status: current
system: platform
requirements:
  - REQ-PLATFORM-GIT-COMMIT-PUSHED-EVIDENCE-001
created: 2026-10-03
owners:
  - kandev
---

# Git commit pushed evidence design

## Boundary and existing contracts

`GitOperator.GetLog` in `apps/backend/internal/agentctl/server/process/git_log.go`
produces local commit metadata for `GET /api/v1/git/log`. Its `GitCommitInfo.Pushed`
already promises upstream reachability. Platform owns that shared evidence,
rather than task-delivery accounting or file-status classification.

Preserve the [contribution contract](../../tasks/requirements/remote-contribution-tasks.md),
[provider enrichment decision](../../../decisions/2026-08-13-provider-history-changes-enrichment.md),
[workspace base contract](../../workspaces/requirements/workspace-base-branch-propagation.md),
[merge-detail contract](../../ui/requirements/merge-commit-details.md), and
[file-navigation design](../../ui/system-design/commit-file-navigation.md).
They retain their source, selection, interaction, and mutation boundaries.
No new ADR is needed: this restores an existing field invariant inside the
existing batched lookup rather than establishing a new architectural constraint.

## Requirement mapping

| Criteria | Design section |
| --- | --- |
| AC-PLATFORM-GIT-COMMIT-PUSHED-EVIDENCE-001.1, .2 | Faithful bounded traversal |
| AC-PLATFORM-GIT-COMMIT-PUSHED-EVIDENCE-001.3 | Failure behavior |
| AC-PLATFORM-GIT-COMMIT-PUSHED-EVIDENCE-001.4, .5 | Routing and consumers |
| AC-PLATFORM-GIT-COMMIT-PUSHED-EVIDENCE-001.6 | Execution and resource boundaries |

## Confirmed mismatch

For a nonempty base, `GetLog` selects `git log --first-parent <base>..HEAD`.
`markPushedCommits` instead selects `git rev-list -nN HEAD ^<upstreamSHA>`
across all positive parents, where N is the returned row count. A newer local
side tip can consume a slot and omit an older local first-parent commit.
The current complement operation then falsely sets that omitted row true.

The retained actual-operator proof returned only feature and merge rows, while
the evidence lookup returned merge and side tip. A real push made both returned
rows true as the positive control. Membership and stats are not the defect.

## Faithful bounded traversal

Pass the existing `baseCommit` string into `markPushedCommits` as the small local
representation of traversal context. Do not introduce a general graph model.
Keep `GetLog` selection, format, shortstat parsing, and limits unchanged.

Anchor the optional lookup to `commits[0].CommitSHA`, the already returned
positive tip, rather than rereading moving `HEAD`. With these unfiltered log
modes a nonempty result begins at its positive tip. This costs no additional
Git command and prevents a newly advanced HEAD from consuming the evidence cap.

Resolve upstream exactly as today, then issue one capped command:

| Returned log mode | Optional unpushed lookup |
| --- | --- |
| Nonempty base | `git rev-list -nN --first-parent <base>..<returnedTipSHA> ^<upstreamSHA>` |
| No base | `git rev-list -nN <returnedTipSHA> ^<upstreamSHA>` |

Here N is `len(commits)`, not the requested API limit. A range read currently
ignores the limit; this repair does not add a range cap. A recent read retains
its explicit limit or the operator's default 50. API limits and aggregate
truncation remain independently owned by the handler.

The positive traversal must match the original graph mode and range. Negative
base and upstream exclusions must retain full ancestry. Git documents
[`--first-parent` and `--exclude-first-parent-only`](https://git-scm.com/docs/git-rev-list)
as separate inclusion and exclusion controls. Do not add the latter: a commit
reachable through an upstream merge's side parent is still published.

For the same tip and range, upstream exclusion removes published rows and
their published ancestry from the original traversal. An excluded ancestor
cannot expose a local-only ancestor, since all its ancestors are published.
The remaining local rows preserve the selection's traversal order; at most N
local rows can occur within its first N returned rows. Therefore the first N
local SHAs cover every returned local SHA. On the range path, matching the
first-parent traversal prevents unrelated side rows from consuming those slots.
On the recent path, both traversals continue following all parents.

Parse that successful result into the existing local SHA set and apply its
complement only to the returned rows. No per-row ancestry process, enlarged cap,
uncapped walk, or provider read is needed. A cap bounds emitted evidence, not
every internal Git object visit; existing admission and timeouts bound execution.

## Failure behavior

An empty list skips enrichment. Missing upstream, failed ref resolution, empty
resolved upstream, cancelled context, or failed rev-list leaves every affected
row false. Do not partially apply evidence from a failed command or change the
successful list result into a failure. Resolve the tracking ref locally; this
field says nothing about changes on the server since the last tracking update.

## Routing and consumers

`Server.runGitLogForRepo` gets `Manager.GitOperatorFor(repo)` and resolves each
existing comparison before invoking `GetLog`. `collectLogForRepo` retains
per-repository bases. `mergeGitLogResults` stamps `repository_name`, sorts,
and truncates already enriched rows without recomputing pushed evidence.
Leave these paths and the transport schema unchanged.

`mergeCommits` in `apps/web/components/task/changes-panel-helpers.ts` uses local
`pushed=true` as evidence even without a PR; provider SHA matching remains its
independent overlay. Existing `changes-panel.test.ts` covers no-PR mixed values,
PR-SHA mismatch, provider-only rows, and ordering. `changes-panel-remote.test.ts`
covers repository identity and divergent provenance. No frontend edit is needed.
Do not reinterpret false as a veto of independently confirmed provider evidence.

## Execution and resource boundaries

All commands remain under `runGitCommand`, argument validation, filtered executor
environment, interactive Git admission, the existing post-admission timeout,
and caller cancellation. Successful nonempty enrichment remains two ref reads
and one rev-list, independent of row count. Preserve the
[managed Git execution design](git-subprocess-execution.md).
No network access, checkout/index write, configuration, persistence, telemetry,
retry, or authorization path is added.

## Verification and surface assessment

Permanent process tests use real local/bare repositories, deterministic author
and committer dates, and actual `NewGitOperator.GetLog`. Include the retained
merged-side regression, a real push positive control, mixed first-parent rows,
limited full-graph side rows, upstream second-parent reachability, missing
upstream, and failed/cancelled evidence. Pin all relevant fixture dates,
including base and merge; `runGit` strips every `GIT_*`, so set fixture dates
after filtering in a test-local subprocess environment. Register teardown
immediately and do not run these environment-sensitive fixtures in parallel.

A registered-router test uses two disposable repository contexts containing
the same commit graph but different tracked upstream tips. Selected reads prove
opposite pushed values for the same SHA; aggregate reads preserve both contexts
and identities. Snapshot stable refs, HEAD, status, and index around read calls.
HTTP producer evidence is the end-to-end boundary for this backend correction;
do not substitute canned JSON or mocked Git/coordinator results.

Mobile parity: this is shared source-data correction only. It changes no rendered
layout, copy, touch behavior, scrolling, navigation, or breakpoint behavior.
The existing consumer unit evidence and real HTTP producer test suffice; no
new browser/E2E/build or ASCII preview is required. If a pure consumer regression
becomes necessary, the skill's state/data exception applies with the same note.

Public docs: the existing Git operations how-to explains Changes and separates
inspection from publication. This restores its intended behavior and changes
no user instruction, command, API shape, or terminology. Update internal specs
and delivery records only; do not add Git traversal implementation trivia to
public guidance.

See the [sequential repair package](../../../plans/git-commit-pushed-evidence/plan.md).
