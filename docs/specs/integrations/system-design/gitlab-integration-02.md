---
status: draft
system: integrations
requirements:
  - REQ-INTEGRATIONS-GITLAB-INTEGRATION-001
created: 2026-05-04
updated: 2026-10-22
owners:
  - tbd
---
# GitLab Integration System Design Part 2

## Purpose and boundaries

This design preserves the technical source detail for `REQ-INTEGRATIONS-GITLAB-INTEGRATION-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-INTEGRATIONS-GITLAB-INTEGRATION-001` | [Migrated source detail](#migrated-source-detail) |
| `REQ-INTEGRATIONS-GITLAB-INTEGRATION-001` (AC .11-.15) | [Discussion reply draft settlement](#discussion-reply-draft-settlement) |
| `REQ-INTEGRATIONS-GITLAB-INTEGRATION-001` (AC .20-.23) | [Watch dialog ownership](#watch-dialog-ownership) |
| `REQ-INTEGRATIONS-GITLAB-INTEGRATION-001` (AC .16-.19) | [Pipeline job summary classification](#pipeline-job-summary-classification) |
| `REQ-INTEGRATIONS-GITLAB-INTEGRATION-001` (AC .24-.27) | [Merge API source-head compatibility](#merge-api-source-head-compatibility) |
| `REQ-INTEGRATIONS-GITLAB-INTEGRATION-001` (AC .28-.30) | [Default merge squash choice](#default-merge-squash-choice) |


## Merge API source-head compatibility

For AC .24-.27, `Service.mergeMRWithClient` continues validating project merge
methods and calls the selected client; interactive workspace/expected-host
routing and `MergeMRForAutomation` retain their existing connection selection.
`PATClient.MergeMR` reads the single MR endpoint using the same client, full
encoded target project path and IID immediately before the merge PUT. It uses
the response's top-level `sha`, documented as the source branch head, as the
merge body's `sha`. A nonempty, non-whitespace source SHA is required; an error,
missing or whitespace-only value prevents the PUT. Do not fall back to diff,
base, pipeline, target branch, cached task or UI values. Keep the returned
value unchanged for GitLab to validate; do not infer a reviewed-head guarantee.

`convertRawMR` currently prefers `diff_refs.head_sha` for the general MR
projection. That diff-version value can lag the top-level head, so do not use
`GetMR`'s projected `HeadSHA` for this write and do not change the projection's
semantics as part of this repair. Use a small provider-local raw response read;
no `Client` interface or UI request change is needed. A fork still addresses
the MR in its target project, while the returned top-level SHA identifies the
source head. The provider checks the SHA atomically during its merge. Propagate
its error, including 409, without a second read or merge retry after the PUT;
a future independent action remains subject to existing automation scheduling.

`GLabClient` embeds `PATClient` and inherits this path. Service and automation
callers therefore get the same guard without altering mock or noop contracts.
Keep all existing merge request fields and error wrapping, credential selection,
method and squash policy, and no requested source-branch removal. The extra
scoped read is mandatory even for older hosts, and its failure is fail-closed.
The separate default-choice correction below clarifies the preserved squash
policy without changing this guard or the other AC .24-.27 invariants.

GitLab's [single MR](https://docs.gitlab.com/api/merge_requests/#get-single-merge-request)
and [merge API](https://docs.gitlab.com/api/merge_requests/#merge-a-merge-request)
document the source-head field, conditional merge `sha` and mismatch response;
its [merge implementation](https://raw.githubusercontent.com/gitlabhq/gitlabhq/master/lib/api/merge_requests.rb)
shows the parameter passed to merge. The scoped registered-route and provider
contract tests in the [work order](../../../plans/gitlab-required-merge-sha/task-01-bind-merge-head.md)
prove the actual boundary with disposable servers. This is not a live GitLab
mutation or a new public UI contract.

## Default merge squash choice

For AC .28-.30, `Service.mergeMRWithClient` must distinguish project
capability from caller intent: an empty method leaves the existing squash bool
false; explicit `squash` still validates `AllowSquash` and sets it true.
The PAT request's existing `omitempty` omits false, so GitLab retains the MR
choice and applies project policy. `default_off` and `default_on` both permit
squash, but neither permission implies the user's current choice. Required
and prohibited policies remain GitLab's decision. Other explicit method checks
and unknown-method errors remain unchanged. `GLabClient` embeds the PAT client.
No client interface, policy metadata, frontend option, or new setting is needed.

The current desktop `MRStatusChipPopover` and phone `MRStatusChipDrawer` both
render `MRCIPopover` and `MRMergeButton`; the MR detail confirmation also calls
the existing `mergeMR` helper. Both pass `squash: false`, which the helper
encodes as an omitted method in the existing workspace/expected-host request.
The controller resolves the workspace client and automation uses the same
service resolution with an empty method. The change is backend data behavior:
no layout, touch interaction, navigation, or copy changes. This source trace
does not establish rendered browser behavior; focused real-route and provider
HTTP tests, rather than new mobile browser coverage, exercise the shared path.

Independently authored disposable HTTP provider tests must observe the actual
merge body and model GitLab's MR-choice and project-policy resolution. Cover
both unchecked default policies, selected default-on, required and never,
explicit squash and invalid methods, plus existing identity, head, race,
message and source-branch guards. Exercise workspace action and automation
wiring. The fixture models provider behavior and is not a live GitLab test.
See the [work order](../../../plans/gitlab-squash-choice/task-01-respect-squash-choice.md).
GitLab documents the [merge API](https://docs.gitlab.com/api/merge_requests/)
and [squash policies](https://docs.gitlab.com/user/project/merge_requests/squash_and_merge/);
its [merge parameter implementation](https://gitlab.com/gitlab-org/gitlab/-/raw/master/lib/api/merge_requests.rb)
provides the omission behavior modeled by the fixture.

## Watch dialog ownership

AC .20-.23 use the existing `GitLabWatchSettings` actions, review/issue wrappers,
shared `GitLabWatchDialog`, domain hooks, API and store. The shared dialog owns
only draft, inline error, busy state and dismissal. Domain actions continue
publishing accepted writes and their truthful parent feedback after dismissal.

Render one form lifetime only while open, keyed by workspace, kind and selected
watch ID (including create mode). Initialize local form state once from the
selected watch/defaults. Same-ID object replacement is data hydration, not a new
opening. Closing and reopening unmounts and creates a fresh lifetime even when
all identity values are equal; keys alone do not represent an opening counter.
Use a local ref activated in layout-effect setup and revoked in cleanup, plus
synchronous revocation when the user dismisses. StrictMode setup restores the
ref after its simulated cleanup. Do not mutate ownership during render.

Capture the existing normalized payload and callbacks at Save. Await the real
create/update unconditionally. Gate only dialog-local success dismissal, catch
inline error and finally busy release by that form's liveness. This keeps old
requests authoritative for their original accepted writes without granting them
ownership of a newer form. A new form has independent busy state. No shared
revision framework, API changes, cache rewrites or ADR are necessary: the local
component boundary preserves the complete ownership rule.

The existing watch dialog is the nearest shipped phone surface. Preserve its
single vertical scroller, dynamic viewport bounds, stacked phone fields/actions
and desktop paired fields. Entry is the workspace GitLab settings New watch or
row Edit action on its existing phone card. The focused edit surface
supports a multi-field task rather than adding a picker drawer. Desktop and
native phone taps exercise the same domain actions; existing primitives own
safe-area/touch sizing. No geometry or navigation redesign is included.

Verification uses real parent/settings/provider/domain/API/store integration
with controlled fetch settlement; only the unrelated Monaco renderer may be
stubbed in jsdom while its loader exports and ScriptEditor wrapper remain real.
Keep ordinary owned success/failure controls, old success and rejection,
create/update and review/issue, same/different reopening, scope replacement,
unmount/StrictMode, hydration, retry, raw focus/caret and newer busy ownership.
Production-build desktop and mobile E2E hold only the causal HTTP response
through a disposable mock-GitLab backend, preserving actual persistence.
See the [one work order](../../../plans/gitlab-watch-dialog-ownership/task-01-preserve-current-watch.md).

## Pipeline job summary classification

The [optional-manual plan](../../../plans/gitlab-optional-manual-ci/plan.md)
records one locally implemented and verified pass. This section clarifies the existing
Integrations-owned provider projection; it introduces no separate UI contract
or architecture decision. Hosted CI, review and merge remain pending. This
classification section describes the implemented behavior; unrelated migrated
sections and the overall split design retain their existing draft lifecycle.

### Provider evidence and classification boundary

[GitLab job control](https://docs.gitlab.com/ci/jobs/job_control/#types-of-manual-jobs)
defines optional manual jobs by `allow_failure=true` and states that they do not
contribute to overall pipeline status. The
[YAML reference](https://docs.gitlab.com/ci/yaml/#when) treats unstarted manual
jobs as skipped/successful for pipeline progression. Kandev's existing pass
bucket is an acceptance summary, already including skipped and allowed-failure
jobs, rather than an execution verdict.

`apps/backend/internal/gitlab/models_pipeline_jobs.go` owns
`pipelineJobBucket` and `summarizePipelineJobs`. Add only the `manual` case:
accepted when `AllowFailure` is true, otherwise in progress. Mirror that exact
predicate in `bucketJob` in `apps/web/lib/gitlab/pipeline-buckets.ts`. Do not
accept arbitrary nonterminal jobs merely because `allow_failure` is true.

| Raw job status | `allow_failure=false` | `allow_failure=true` |
| --- | --- | --- |
| `success`, `skipped` | passed | passed |
| `manual` | in_progress | passed |
| `failed`, `canceled` | failed | passed |
| `running`, `pending`, `created`, `scheduled`, unknown | in_progress | in_progress |

`summarizePipelineJobs` retains `len(jobs)` as total and the same failing-job
selection. `bucketJobCounts` retains one bucket per job; `groupJobsByStage`
retains stage order, membership, empty-stage fallback and priority
`failed > in_progress > passed`. The stage's click target remains the first
linked job matching its derived bucket. No job object is rewritten.

### API, cached and first-party display paths

`PATClient.ListPipelineJobs` decodes `rawPipelineJob` into `PipelineJob`,
including raw `Status`, `AllowFailure` and `WebURL`, across existing pages.
`PATClient.GetMRStatus` calls `populatePipelineJobCounts`; `GetMRFeedback`
calls the same summarizer and includes raw jobs on the latest pipeline.
Their counts therefore agree without changes to endpoints or schemas.
`GLabClient` embeds `PATClient`, so authenticated CLI connections use this
same REST implementation; environment-token connections also resolve a PAT
client. `MockClient` already uses the backend summarizer.

`Service.syncTaskMRWithClient` and task linking project the status counts into
`TaskMR.PipelineJobsTotal` and `PipelineJobsPass`; existing store upserts and
HTTP/WS task-MR projections carry `pipeline_jobs_total` and
`pipeline_jobs_pass`. Existing cached rows become correct on their next
ordinary sync. Do not migrate rows, force refreshes, or change polling.
`Controller.httpGetMRFeedback` returns live feedback at
`GET /api/v1/gitlab/mrs/feedback` with the existing workspace/project/IID/host
identity and permission checks.

The real `MRCIPopover` owns its existing gated `useMRFeedback` read. When the
latest pipeline contains jobs it uses `bucketJobCounts` for its pass-rate bar
and `groupJobsByStage` for stage counts, icons and links. With no live job list,
including before feedback or on an unavailable initial read, it uses the
`TaskMR` aggregate counts and renders no invented stage rows. Live job data
continues to take precedence over aggregate counts. Other feedback consumers
continue to receive backend `jobs_passing`/`jobs_total` summaries.

`latestPipelineStatusAndFailingJobs` continues to return the provider's raw
pipeline status and the summarizer's failing jobs. Both blocking and optional
manual jobs were already absent from that failing set; accepting the optional
one does not change auto-fix inputs. Merge readiness and automation continue
to use existing raw pipeline and MR gates, as documented by the
[GitLab lifecycle ADR](../../../decisions/2026-08-01-gitlab-mr-lifecycle-notifications.md).
No automation or readiness code changes are planned.

### Shared desktop/phone outcome and evidence

The fine-pointer `MRStatusChipPopover` and coarse-pointer
`MRStatusChipDrawer` render the same `MRCIPopover` body. The existing drawer
is the nearest shipped exemplar for the mobile skill's hover-to-drawer
pattern. Shared normalization changes counts and existing status icons only;
composition, scrolling, controls, navigation, breakpoints and copy remain as
shipped. The mobile skill's pure state/data exception applies: meaningful
unit and real-consumer component evidence satisfies parity without a new
Playwright case or browser build. The plan's UI-01 preview describes the same
data within both existing wrappers.

Independently extend the backend bucket/summarizer tables and a disposable
provider `httptest` API regression exercising real `GetMRStatus` and
`GetMRFeedback`. Prove 2/2 for success plus optional manual, retain raw manual
metadata, and use mixed jobs to prove real failures and active/blocking jobs
survive classification. Extend frontend bucket/count/stage tests with the
same policy controls and dominant-state URL selection.

Add `apps/web/components/gitlab/mr-ci-popover.optional-manual.test.tsx` using
real `StateProvider`, locales, Tooltip/Toast providers, `MRCIPopover`,
`useMRFeedback`, API functions and classifier helpers. Stub only fetch
transport, supplying production-shaped feedback/files/commits and automation
reads. Assert the actual pass bar, stage counts, icon classes and native hrefs,
including live precedence, aggregate fallback and mixed pending/failing
stages. Settle held fetches and clean up timers/providers. Existing mocked
popover tests do not establish the live read/consumer contract by themselves.

No new dependencies, configuration, state ownership, persistence, permissions,
telemetry, races, provider actions or public API shapes are introduced.
Protected discovery source files are evidence only; permanent tests must be
independently authored without reading, copying, importing or replaying them.

## Discussion reply draft settlement

This correction extends the existing provider review contract, owned
by Integrations. The
[reply-draft work order](../../../plans/gitlab-reply-draft-preservation/task-01-preserve-reply-draft.md)
contains implementation and verification scope. No independent UI owner or
incident specification is introduced.
Local implementation and targeted checks are complete; hosted review, CI and
merge remain pending. The unrelated migrated design retains its draft status.

### Local state boundary

`apps/web/components/gitlab/mr-discussions-section.tsx` renders one
`Discussion` per stable `discussion.id`. Each owns its `reply` state;
`submitReply` captures that state before trimming and awaiting `onReply`.
The textarea remains editable during the await.

Capture the raw `reply` before dispatch and derive the existing trimmed body
from that snapshot. Keep the nonblank gate. On successful settlement use a
functional state update that compares the latest value with the captured raw
snapshot: clear an exact match, otherwise return the latest value intact.
Compare raw strings, not their trimmed bodies: a whitespace edit is unsent
work even when the sent body would be identical. Exact restoration to the
snapshot can clear; no edit history, sequence counter, shared draft store,
request framework, or new pending policy is needed. A false result performs
no draft write. State updaters remain pure.

### Production consumer and refresh

`MRDiscussionsSection` has one production consumer,
`apps/web/components/gitlab/mr-detail-panel.tsx`. Its `discussionHandlers`
binds workspace/project/IID/host and forwards the discussion ID and body to
`createMRDiscussionNote` through `useMRActions.run("reply", ...)`.
The API sends a POST to `/api/v1/gitlab/mrs/discussions/notes`, with the
existing identity and `discussion_id` payload. These contracts do not change.

`useMRActions` returns true after the POST, emits the existing translated
success toast, and schedules `useMRFeedback.refresh`; it does not await the
refresh. It returns false with an error toast after a rejected POST and
releases `pendingAction` in either case. `useMRFeedback` retains cached
feedback while refreshing the same identity. Stable discussion keys keep
local drafts attached across replaced or reordered discussion objects.
A delayed or rejected refresh is not a reply failure and does not write
textarea state. Preserve the current hook and loading/error policy.

### Verification and mobile boundary

Author independent rendered regressions in
`apps/web/components/gitlab/mr-discussions-section.reply-draft.test.tsx`.
Mount the real section with real Tooltip/Toast/locale providers and real
`useMRFeedback`, `useMRActions`, and `createMRDiscussionNote`; a small harness
uses the same callback binding as the audited detail panel. Mock only global
fetch transport. Hold POST and refresh responses independently, assert the
actual request identity/body, editable textarea and pending Reply button,
then settle responses and inspect the real textarea, notes and feedback.
Do not mock production components, hooks, API functions or draft predicates.
Use article-scoped selectors for distinct discussions and settle every held
request before cleanup. The original protected discovery fixture is read-only
evidence and must not be copied, imported, mutated or replayed.

The correction is pure component state settlement. Desktop's existing
`sm:flex-row` reply area and phone's stacked controls share the same state;
there are no changes to rendered structure, touch, scrolling, navigation,
breakpoints or copy. Targeted real-consumer component tests satisfy the
mobile-parity pure-state exception; new Playwright coverage is unnecessary.
No backend, persistence, security, telemetry, public API or operator guidance
changes are needed. Part 1 and unrelated migrated sections retain their
existing lifecycle.

## Migrated source detail

## Failure modes

- Saving an invalid host or credential fails without replacing the last known
  working connection or secret.
- A transient health or API failure keeps the saved config and linked/watch
  records. The UI shows an unavailable state and allows retry; pollers back off
  and do not create tasks from incomplete results.
- Revoked credentials mark only the affected workspace `auth_required` and
  pause API work for it until a successful probe. Other workspaces continue.
- Watch dispatch reserves the external item before creating a task. Task-create
  or auto-start failure releases or completes the reservation according to the
  shared watcher dispatcher, preventing both lost matches and duplicate tasks.
- A deleted/soft-deleted watch dependency self-disables the watch with a visible
  error instead of creating an orphan task.
- Link creation is atomic from the user's perspective: if URL validation or MR
  fetch fails, no association is written. Repeating a successful request
  returns the existing association.
- Unlink failure leaves the association and refresh watch intact. Unlink never
  closes or unsubscribes from the upstream MR.
- Watch delete is best-effort for owned-task cleanup: individual task-delete
  failures are logged, remaining owned tasks are attempted, and the watch/dedup
  rows are removed. It never schedules another poll.
- Watch reset follows the shared GitHub `watchreset` contract: after preview and
  confirmation it best-effort attempts every owned task deletion (including
  archived), logs and continues past individual task-delete failures, then
  transactionally clears dedup and `last_polled_at`. A clear failure surfaces
  together with the count already deleted; otherwise the response reports the
  successful delete count. Review watches rerun immediately and issue watches
  are eligible on their next poll/run-now.
- Reviewer, discussion, merge, and subscription failures leave the last fetched
  UI state visible and show an action error; the UI refreshes after success.
- MR creation never retries against another host. A successful push followed by
  a failed MR request is reported as partial failure with the pushed branch
  intact so the user can retry without another commit.

## Persistence guarantees

- Workspace config rows, PAT secrets, watch definitions, dedup reservations,
  task-to-MR associations, and last known MR status survive backend restarts.
- Archived tasks retain task-to-MR associations for later unarchive; hard task
  deletion removes task-owned associations and refresh watches.
- The startup migration moves the legacy global host/token to the active
  workspace, or the earliest-created workspace when no active workspace is
  available. It is idempotent and never duplicates automation watches.
- `glab` login and `GITLAB_TOKEN` remain host-process state and are not copied or
  backed up by Kandev.
- In-flight HTTP requests and poll iterations do not resume after restart. The
  next health/poll cycle re-runs safely against durable dedup state.
- GitLab notification subscriptions survive because GitLab owns them; Kandev
  re-reads their state after reload.

## Scenarios

- **GIVEN** two workspaces connected to different GitLab hosts, **WHEN** each
  opens its GitLab page, **THEN** each sees only data fetched with its own host
  and credential.
- **GIVEN** a workspace without a GitLab connection, **WHEN** the user enters a
  public `gitlab.com` repository URL in Remote task creation, **THEN** Kandev
  lists its branches anonymously without making any other GitLab browse or
  write capability available.
- **GIVEN** a legacy global GitLab host and token, **WHEN** Kandev starts after
  upgrade, **THEN** one deterministic workspace receives the config and secret
  and other workspaces remain unconfigured.
- **GIVEN** a self-managed workspace connection, **WHEN** a user links a valid
  MR URL from that host, **THEN** the task shows the linked MR and its live
  review details after reload.
- **GIVEN** a task with no linked MR in a GitLab-configured workspace, **WHEN**
  the task detail opens, **THEN** the top bar has no `Link MR` action and the
  task's contextual `Link` submenu offers `GitLab Merge Request`.
- **GIVEN** a touch viewport and a task with no linked MR in a GitLab-configured
  workspace, **WHEN** the user opens the task row's visible actions menu and
  chooses `Link` then `GitLab Merge Request`, **THEN** the GitLab MR link dialog
  opens without relying on right-click or long press.
- **GIVEN** a task with a linked GitLab MR, **WHEN** the task detail opens,
  **THEN** the top bar shows the linked MR status control rather than a generic
  link action.
- **GIVEN** an MR URL from a different host, **WHEN** it is linked in the current
  workspace, **THEN** the request is rejected and no association is written.
- **GIVEN** a linked MR, **WHEN** the user unlinks it, **THEN** it disappears
  from the task and GitLab list indicator while the upstream MR is unchanged.
- **GIVEN** a GitLab MR or issue search row, **WHEN** the user launches a preset,
  **THEN** the task-create dialog is prefilled with the matching project and
  context and successful creation navigates to the task.
- **GIVEN** a linked MR with discussions, approvals, conflicts, and a pipeline,
  **WHEN** the user opens its task review panel, **THEN** all states and thread
  actions are available without leaving Kandev.
- **GIVEN** an eligible project member, **WHEN** the user selects them as a
  reviewer, **THEN** GitLab and the refreshed MR panel both show that reviewer.
- **GIVEN** a linked MR with auto-fix enabled, an open state, a settled
  failing pipeline, and a promptable session, **WHEN** automation evaluates
  the MR, **THEN** it sends or queues exactly one `@mr-auto-fix` prompt and
  `auto_fix_round_count` increments by one; an unchanged snapshot on the next
  evaluation dispatches nothing.
- **GIVEN** a linked MR with auto-merge enabled and every readiness gate
  satisfied, **WHEN** automation evaluates the MR, **THEN** Kandev merges it
  exactly once, routed through the workspace client resolved with the linked
  row's own host; a host mismatch never merges.
- **GIVEN** a linked MR reaches `merged`, `closed`, or `locked`, **WHEN**
  auto-fix is still enabled, **THEN** no further auto-fix prompt is
  dispatched for that MR.
- **GIVEN** a task with exactly one linked MR on desktop, **WHEN** the user
  hovers the topbar button past the open delay, **THEN** a preview popover
  shows the pass-rate bar, approval row, unresolved-discussions row,
  Automation controls, a merge action when ready, and link/open/unlink
  actions, all without a click.
- **GIVEN** a task with exactly one linked MR on desktop, **WHEN** the user
  clicks the topbar button, **THEN** the MR detail panel opens directly (no
  dropdown) and any open hover popover closes.
- **GIVEN** a task with two or more linked MRs, or any touch/coarse-pointer
  viewport regardless of MR count, **WHEN** the user interacts with the
  topbar button, **THEN** no hover popover is ever rendered and
  clicking/tapping opens the existing per-MR dropdown.
- **GIVEN** a task with two linked MRs, one merged and one open with a failing
  pipeline, **WHEN** the Kanban card renders its MR badge, **THEN** the badge
  shows a count of two and the open MR's (red) colour, not the merged MR's.
- **GIVEN** an MR or issue the user does not subscribe to, **WHEN** they enable
  notifications, **THEN** GitLab reports it subscribed and no Kandev watch or
  task is created.
- **GIVEN** an enabled review watch, **WHEN** a new matching MR requests the
  user as reviewer, **THEN** one linked task is created in the configured step
  and auto-starts with the configured profiles when requested.
- **GIVEN** an enabled issue watch, **WHEN** a matching issue appears, **THEN**
  one task is created with the issue URL and interpolated issue context.
- **GIVEN** a paused watch, **WHEN** scheduled polling or run-now occurs,
  **THEN** no task is created and existing dedup state is retained.
- **GIVEN** a watch with owned tasks, **WHEN** the user previews and confirms
  reset, **THEN** all shown tasks are attempted, the response reports successful
  deletions, dedup state is cleared, the watch remains enabled, and currently
  matching items can create new tasks immediately (review) or next poll (issue).
- **GIVEN** a watch with owned tasks, **WHEN** the user deletes the watch,
  **THEN** Kandev best-effort deletes those tasks and removes the watch/dedup
  state without recreating tasks from current matches.
- **GIVEN** a watch whose bound profile or repository was deleted, **WHEN** a
  match is dispatched, **THEN** the watch disables with a visible error and no
  orphan task is created.
- **GIVEN** a GitLab task branch with committed changes, **WHEN** the user
  creates a draft merge request, **THEN** the branch is pushed to the configured
  host, the returned MR URL is linked to that task repository, and the UI uses
  merge-request terminology on desktop and mobile.
- **GIVEN** a revoked token in Workspace A, **WHEN** health polling runs,
  **THEN** Workspace A shows `auth_required`, its watches stop dispatching, and
  Workspace B continues using its own connection.

## Out of scope

- GitLab webhook ingestion; this iteration uses polling.
- Durable GitLab issue-to-task linking, issue state synchronization, or
  Jira/Linear-style structured issue import.
- Editing GitLab approval rules, protected branches, CI configuration, pipeline
  jobs/logs, or merge request templates.
- Group-wide dashboards beyond results visible to the configured user.
- OAuth-based Kandev sign-in, GitLab Duo, repository migration between hosts,
  and Bitbucket parity.
- Multiple named GitLab connections inside one Kandev workspace.
- A multi-MR aggregate hover preview (the popover only covers the single-MR
  topbar case; a task with 2+ linked MRs keeps the existing click-only
  dropdown).
- GitLab's per-reviewer `requested_changes` states as a three-way review
  label; "awaiting review" is approvals-count-based only (version/tier
  dependent otherwise).
- Per-job CI log tailing in the auto-fix prompt.
