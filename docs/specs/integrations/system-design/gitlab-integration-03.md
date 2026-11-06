---
status: draft
system: integrations
requirements:
  - REQ-INTEGRATIONS-GITLAB-INTEGRATION-001
created: 2026-11-04
updated: 2026-11-04
owners:
  - tbd
---
# GitLab Integration System Design Part 3

This design carries the technical source for `REQ-INTEGRATIONS-GITLAB-INTEGRATION-001` AC .31-.35. Parts [1](gitlab-integration-01.md) and [2](gitlab-integration-02.md) hold the rest.

## Requirement mapping

| Requirement | Design |
| --- | --- |
| `REQ-INTEGRATIONS-GITLAB-INTEGRATION-001` (AC .31-.35) | [Draft merge requests](#draft-merge-requests) |

## Draft merge requests

GitLab's REST API has no draft parameter: a draft is a leading title marker
(see the [merge request API](https://docs.gitlab.com/api/merge_requests/) and
[drafts](https://docs.gitlab.com/user/project/merge_requests/drafts/)), and the
merge endpoint answers 405 or 406 for a MR it cannot merge, which includes a
draft (observed on a GitLab 18.1.1 draft MR; read-only). `glab mr update --ready`
rewrites the title the same way. Kandev's GitLab client is REST-only with no
GraphQL path, so Mark as ready rewrites the title through
`PUT /projects/:id/merge_requests/:iid` rather than the `mergeRequestSetDraft`
mutation or the `/ready` quick action.

**Draft predicate.** One web helper `isMRDraft({draft, detailed_merge_status})`
(true when `draft` or status `draft_status`) and one helper
`isMergeStatusTransient(status)` (`checking`, `unchecked`, `preparing`). The web
`MR` type gains `detailed_merge_status`. The detail panel feeds the helpers from
`feedback.mr` only (it renders nothing but `FeedbackPlaceholder` before the
feedback loads, so there is no cached fallback); the popover and phone drawer
(`MRMergeButton`, defined in `mr-merge-button.tsx` and rendered by
`mr-ci-popover.tsx`) feed it the `TaskMR` only.

**Controls (AC .32).** Detail-panel Merge button and `MergeConfirmation` confirm
action: disabled for a draft, with an inline text hint (`aria-describedby` on the
button, same text on touch). `MRMergeButton`: renders the disabled button plus
the hint for an open draft before it applies the `isMRReadyToMerge` check, which
keeps hiding it for other reasons. The confirmation action re-evaluates on every render and is
disabled by exactly the Merge button's conditions (draft, open state, conflicts,
pending action, and the post-ready hold), so a MR that turns draft or enters the
hold while the dialog is open disables its confirm. The auto-merge setting is not changed. The detail-panel
Merge button keeps `!isOpenMRState`, `hasConflicts` and `busy` as today and adds
the draft rule and, only while a post-ready wait is active (see UI flow), the
transient hold; it does not add a `mergeable` condition, and a panel that opens
on a `checking` MR does not hold Merge.

**Server guard (AC .31).** Order in `mergeMRWithClient` is unchanged: project
merge-method read, method validation, then `client.MergeMR`. Inside
`PATClient.MergeMR` the existing head GET also decodes `draft`, `work_in_progress`
and `detailed_merge_status`; for a draft it returns the sentinel `ErrMRIsDraft`
before the empty-SHA check and before the PUT. `MockClient.MergeMR` applies the
same rule. `writeMergeActionError` maps `ErrMRIsDraft` to `409` with the fixed
message "This merge request is a draft; mark it ready first". Automation reaches
the same `MergeMR`, so it is covered; its existing draft skip is unchanged.

**Refusal messages (AC .33).** `PATClient.MergeMR` wraps an `*APIError` returned by
the merge PUT (only) with status 405, 406, 409 or 422 in `MergeRefusedError{Status}`,
whose `Error()` and `Unwrap()` delegate to the wrapped error, so
`APIError.Error()` is unchanged and a failed head GET is never wrapped.
`writeMergeActionError`, used by the merge route only and falling through to
`writeWorkspaceClientActionError` for everything else, maps by status:
405 and 406 to `409` "GitLab cannot merge this merge request yet", 409 to `409`
"The source branch changed after it was read; refresh and retry", 422 to `422`
"GitLab rejected the merge options". `APIError.Body` is never read for output, so
`TestAPIErrorNeverIncludesProviderResponseBody` and
`TestHostileProviderBodyDoesNotReachHTTPStatusOrLogs` keep holding; a new case
adds a hostile body on a 405 merge response. The web toast shows the response
`error` field as it does today.

**Mark as ready (AC .34, .35).** Route
`PUT /api/v1/gitlab/mrs/ready?workspace_id=&expected_host=`, body
`{"project": string, "iid": int}`, response the re-read `MR` as JSON. The handler
rejects a blank project or `iid < 1` with `400` "project and a positive iid are
required", then runs `Client.MarkMRReady(ctx, project, iid) (*MR, error)` through
`runWorkspaceClientAction` (same workspace, host and credential checks as merge)
and maps errors through `writeMarkReadyError`, falling through to
`writeWorkspaceClientActionError`. `PATClient` order: `GetMR` (state is the
normalized `open`, not GitLab's `opened`); a nil first read without an error is `502` with
no write; state other than `open` -> `ErrMRNotOpen`; not a draft -> return the read
MR, no write; `stripDraftMarkers(title)` (pure) -> `ErrDraftMarkerNotFound` or
`ErrDraftTitleEmpty` when nothing is stripped or the result is empty; PUT
`{"title": stripped}`; `GetMR` again, whose failure, or nil MR without an error, is `ErrMRReadAfterReady`;
a re-read whose state is not `open` is returned with `200` first (even when it is
still flagged draft); otherwise a re-read that is still a draft is
`ErrMRStillDraft`; any other result is returned with `200`. A failed first `GetMR`
or title PUT is not wrapped and goes to `writeWorkspaceClientActionError` with the
action label `mark merge request ready` (the existing "GitLab action failed" text
and 401/403/404/502 mapping). Statuses and fixed messages:

| Error | Status | Message |
| --- | --- | --- |
| `ErrMRNotOpen` | 409 | This merge request is not open; reopen it in GitLab before marking it ready |
| `ErrDraftMarkerNotFound` | 422 | The draft marker is not at the start of the title; edit the title in GitLab |
| `ErrDraftTitleEmpty` | 422 | The title would be empty without the draft marker; edit the title in GitLab |
| `ErrMRReadAfterReady` (failed or nil re-read) | 502 | Merge request title updated, but its status could not be refreshed; refresh the panel |
| `ErrMRStillDraft` | 409 | GitLab still reports this merge request as a draft; refresh and try again, or check it in GitLab |
| nil first read without an error (`ErrMRNilResult`) | 502 | GitLab returned no merge request; try again |

Stripping removes leading whitespace, then repeats removing a case-insensitive
`Draft:`, `[Draft]` or `(Draft)` plus the whitespace after it until none remains.
Title race: the title is read, then written, with no conditional write (GitLab
offers none); an edit made in that window, or by a concurrent call that read a
different title, is overwritten, which is accepted. Two concurrent calls on the
same title are both safe: the second sees a non-draft or writes the same title.
`MockClient` strips with the same helper and clears `Draft`, then keeps a per-MR
counter of `GetMRFeedback` calls made after the write. Every read reports
`detailed_merge_status` `checking` when the counter is below 2 at that moment and
`mergeable` otherwise; only `GetMRFeedback` increments it, after answering. So
feedback reads 1 and 2 report `checking` and read 3 onward `mergeable`, while
`GetMR` and `GetMRStatus` (`MarkMRReady`'s own re-read, the poller and the
task-MR link request) never advance it. The counter is per MR and shared by every
`GetMRFeedback` caller, so any open panel or popover for that MR also advances
it; the poller, which uses `GetMRStatus`, cannot consume the transition. The
Playwright spec opens one panel for the MR and issues no other feedback read
for it. `NoopClient` returns `ErrNoClient`.

**UI flow.** `markMRReady` in `gitlab-api.ts`; `MRActionKind` gains `ready` with
catalog keys. `useMRActions.run` calls `onRefresh` after a success today; for
`ready` the panel passes no `onRefresh` and instead starts the post-ready wait
itself after every outcome (success or failure), so a `502` "title updated" or
`409` still-draft failure also starts it, and a success never refreshes twice.
The Mark as ready button is shown only for an open draft, beside the disabled
Merge. The request is not aborted when the panel unmounts, its identity changes,
`refresh()` runs or a new Mark as ready starts: it completes and its outcome is
reported by toast, but it starts a wait only when the panel it was issued from
is still mounted on the same identity and no newer Mark as ready has begun; a
newer click is blocked while the first is pending, so the last case arises only
after an identity change and back.

*Reload contract.* `useMRFeedback` keeps `refresh()` (fire and forget, wired to
the header button and the retry action) and gains `reload(): Promise<ReloadResult>`
used only by the wait. `reload` runs the same three requests (feedback, files,
commits) under the hook's `requestGeneration`: it resolves
`{outcome: "ok", mr}` when the feedback request succeeded (a files or commits
failure still sets the hook `error` but does not fail the reload),
`{outcome: "failed"}` when the feedback request failed (the previous `feedback`
stays), and `{outcome: "superseded"}` when the generation changed before it
settled (unmount, identity change, `refresh()`, a newer `reload`), in which case
it dispatches nothing. A superseded reload therefore cannot write state or restart
the wait.

*Wait.* After the Mark as ready outcome the panel runs: reload 1 immediately;
while the outcome is `ok` with a transient `detailed_merge_status`, or `failed`,
and fewer than 5 reloads have run, schedule the next reload 2 s after the
previous one settled. It stops at the first `ok` non-transient result, at reload
5, or on `superseded`. A manual `refresh()` or a new Mark as ready supersedes the
in-flight reload and clears the pending timer; unmount and an identity change do
the same. The loop owns one timer. The hold ("Checking merge status", Merge
disabled) is a panel state set when the wait starts and cleared when it ends for
any reason, so no other path holds Merge.

*Cached task MR.* The server refreshes the cached rows, not the UI. After
`MarkMRReady` reached its title write (result `200`, `ErrMRStillDraft` or
`ErrMRReadAfterReady`; a no-op on a non-draft also refreshes), the service
method behind the ready route lists the workspace's task MRs
(`ListTaskMRsByWorkspace`), keeps rows whose host matches the workspace host
(`sameConfiguredOrigin`) and whose `ProjectPath` and `MRIID` equal the request,
and for each runs `syncTaskMRWithClient` with the client already resolved for the
request, then `publishTaskMRUpdated`; the existing handler applies the row with
`setTaskMR`. This is the sync path (`reconcileComparisonTargetFromSync`): it
honours a manual base-branch override and never replaces or clears a comparison
target, unlike `AssociateExistingMRByURL`, which is not used. Rows refresh
sequentially, each failure is logged at Warn without provider text and skipped,
and nothing changes the HTTP response. The refresh reads `GetMRStatus`, so it
does not advance the mock counter. It happens right after the write, so the
cached row may still show `checking`; the existing poller brings it to
`mergeable`, which is when the popover and drawer merge button appear, and the
detail panel does not depend on the cache. The UI sends nothing for this, so no
`repository_id` is involved. The unregistered `POST /tasks/:id/mrs/sync` route
and the unused web `syncTaskMR` are not used, and
`TestRegisterHTTPRoutesDoesNotExposeUnscopedTaskMRSync` stays valid. New copy
goes through `t()` in seven languages; zh-hant via `pnpm run i18n:zh-hant`.
