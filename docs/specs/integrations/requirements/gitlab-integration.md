---
status: active
system: integrations
created: 2026-05-04
updated: 2026-10-22
owners:
  - tbd
---
# GitLab Integration Requirements

## Overview

Teams whose code lives on GitLab cannot complete the same task, review, and automation workflows available for GitHub without leaving Kandev. Existing GitLab support can browse merge requests and issues and contains partial watch and review plumbing, but its connection is installation-wide and the main workflows are not usable end to end.

## Requirements

### REQ-INTEGRATIONS-GITLAB-INTEGRATION-001: GitLab Integration

**Intent:** Teams whose code lives on GitLab cannot complete the same task, review, and automation workflows available for GitHub without leaving Kandev. Existing GitLab support can browse merge requests and issues and contains partial watch and review plumbing, but its connection is installation-wide and the main workflows are not usable end to end.

#### Acceptance criteria

- **AC-INTEGRATIONS-GITLAB-INTEGRATION-001.1:** GitLab and GitHub can be connected at the same time. Each integration only reads or mutates its own provider.
- **AC-INTEGRATIONS-GITLAB-INTEGRATION-001.2:** Each Kandev workspace owns exactly one GitLab connection: one normalized host URL, one authentication method, one credential, and one health record.
- **AC-INTEGRATIONS-GITLAB-INTEGRATION-001.3:** The default host is `https://gitlab.com`; self-managed `http://` and `https://` origins are supported for API calls, web links, clone URLs, and merge request creation. Kandev preserves the configured scheme.
- **AC-INTEGRATIONS-GITLAB-INTEGRATION-001.4:** A workspace can authenticate with a personal access token or a `glab` login for its configured host. `GITLAB_TOKEN` remains an explicit deployment fallback, but it is never persisted and only applies to workspaces configured to use that fallback.
- **AC-INTEGRATIONS-GITLAB-INTEGRATION-001.5:** GitLab browse, task-link, review, watch, and write endpoints require an authoritative `workspace_id` and resolve that workspace's connection. Data or credentials from another workspace are never used as fallback.
- **AC-INTEGRATIONS-GITLAB-INTEGRATION-001.10:** Merge-request and issue project choices shall accumulate across pages within the same workspace and browse context. Changing workspace shall discard projects accumulated in the previous workspace, including when the new workspace returns no results. An explicitly selected project filter remains represented without being reset; it does not authorize retaining other projects from the previous workspace.
- **AC-INTEGRATIONS-GITLAB-INTEGRATION-001.6:** Task creation is the narrow unauthenticated exception: branch discovery for an explicitly entered public `gitlab.com` repository URL works without a saved workspace connection. It does not expose private projects, browse results, merge requests, issues, or write actions.
- **AC-INTEGRATIONS-GITLAB-INTEGRATION-001.7:** GitLab repository matching uses provider, normalized provider host, and full subgroup project path. Repositories with unknown or mismatched provider hosts are not eligible for GitLab linking or merge-request actions. Decision: ADR-2026-07-20-repository-provider-origin-identity.
- **AC-INTEGRATIONS-GITLAB-INTEGRATION-001.8:** Users can browse and search merge requests and issues, then launch a task from either row with the same configurable action presets used by GitHub.
- **AC-INTEGRATIONS-GITLAB-INTEGRATION-001.9:** When the selected workspace changes during merge-request or issue browsing, the result page shall reset to page 1 of the new workspace. A populated first page shall remain reachable even when the new workspace has fewer pages than the previous workspace. Ordinary page navigation and unchanged search inputs within the same workspace shall preserve the selected page. Existing preset, custom-query, and result-kind changes shall continue to reset the page; workspace switching shall retain current-workspace result visibility, latest-request response handling, and the existing disabled, refresh, empty, and failure behavior.

Criterion .9 is the pagination clarification delivered by the
[workspace-pagination package](../../../plans/gitlab-workspace-pagination/plan.md).
Its local implementation and behavioral verification are complete; hosted
review, CI, and merge remain pending. Unrelated criteria retain their lifecycle.

#### Discussion reply drafts

The following clarification extends the existing GitLab review contract.
Local implementation and targeted checks are complete in the
[reply-draft package](../../../plans/gitlab-reply-draft-preservation/plan.md).
Hosted review, CI and merge remain pending; unrelated criteria retain their
existing lifecycle.

- **AC-INTEGRATIONS-GITLAB-INTEGRATION-001.11:** When a user submits a nonblank discussion reply, only the trimmed text captured at submission shall be sent to that discussion. Empty or whitespace-only input shall not be submitted. The textarea shall remain editable while the reply is pending, with submission unavailable during the pending action.
- **AC-INTEGRATIONS-GITLAB-INTEGRATION-001.12:** When the reply succeeds, the textarea shall clear only if its current exact text equals the raw text captured at submission. A different current value shall be preserved exactly, including whitespace-only differences and an intentional clear. Text edited and then restored exactly to the submitted raw value is eligible to clear.
- **AC-INTEGRATIONS-GITLAB-INTEGRATION-001.13:** When the reply fails, the current textarea value shall remain intact, whether unchanged, edited, or cleared. After the pending action settles, a user can retry nonblank current text; retry shall send the current trimmed text, and its success shall obey the same clearing rule.
- **AC-INTEGRATIONS-GITLAB-INTEGRATION-001.14:** A successful reply shall keep the existing success feedback and refresh the review. While refreshed data retains the same discussion, its newer unsent text shall survive a delayed, successful, or failed refresh. A refresh failure shall not turn a successful reply into a failed submission or restore a successfully cleared draft.
- **AC-INTEGRATIONS-GITLAB-INTEGRATION-001.15:** Reply settlement shall affect only the submitted discussion's draft. Other discussion drafts shall remain intact, including when refreshed discussions are reordered. Desktop and phone shall share these draft semantics within the existing reply surface.

This clarification excludes draft persistence after unmount, reload, or
discussion removal, changes to MR/workspace switching, and changes to layout,
copy, other review actions, or provider APIs.

#### Pipeline job summary classification

This clarification of the existing GitLab CI summary contract is locally
implemented and verified in the
[optional-manual classification package](../../../plans/gitlab-optional-manual-ci/plan.md).
Hosted CI, review and merge remain pending; the existing requirement and
unrelated criteria retain their lifecycle. Integrations owns the provider semantics across the
status API, feedback, cached MR summaries, and desktop/phone disclosures.

The existing `passed` bucket means accepted for pipeline progress. It includes
skipped jobs and allowed failures, so it does not assert that every job ran or
that every test succeeded. An optional manual job has raw status `manual` and
`allow_failure=true`; a blocking manual job has `allow_failure=false`.

- **AC-INTEGRATIONS-GITLAB-INTEGRATION-001.16:** When an untriggered optional manual job is included in a pipeline's job list, Kandev shall count it in the existing accepted/pass bucket and retain it in the total. A successful test plus one optional manual deployment shall report 2/2 through status and feedback summaries, cached MR summaries after their next ordinary refresh, and the first-party live and fallback CI displays on desktop and phone.
- **AC-INTEGRATIONS-GITLAB-INTEGRATION-001.17:** Job and stage summaries shall agree for mixed states. A stage shall be accepted only when every included job is in the accepted/pass bucket. At least one non-allowed failed or canceled job shall keep the stage failed; otherwise at least one in-progress job shall keep it in progress, even alongside optional manual jobs. Whole-pipeline counts shall retain every job and the separate accepted, in-progress, and failed totals where exposed.
- **AC-INTEGRATIONS-GITLAB-INTEGRATION-001.18:** Blocking manual jobs shall remain in progress. Running, pending, created, scheduled, and unknown-status jobs shall remain in progress even with `allow_failure=true`. Success and skipped jobs shall retain their existing accepted classification; failed and canceled jobs shall remain failed unless allowed to fail, in which case they retain their existing accepted classification.
- **AC-INTEGRATIONS-GITLAB-INTEGRATION-001.19:** Summary classification shall preserve provider-reported job status, `allow_failure`, pipeline status, and job links. Accepted counts shall not substitute for pipeline status or change merge readiness, auto-merge gates, or the failing-job set used by auto-fix. An optional manual job shall not conceal a real failure, claim successful execution, or initiate an upstream action.

This clarification excludes CI configuration edits, job execution, credentials,
new fetching or cache policies, persistence changes, race repairs, other
providers, new copy, and layout or touch-interaction changes.

#### Watch dialog ownership

Integrations owns the create/edit lifetime of GitLab review and issue watches,
including the same behavior on desktop and phone. Implementation and evidence
belong to the [watch-dialog package](../../../plans/gitlab-watch-dialog-ownership/plan.md).

- **AC-INTEGRATIONS-GITLAB-INTEGRATION-001.20:** Closing a pending create or update shall remain available. Its eventual result shall apply to the originally submitted workspace, kind, watch and normalized payload, with the existing accepted store publication and truthful success or failure feedback.
- **AC-INTEGRATIONS-GITLAB-INTEGRATION-001.21:** After closing, reopening or replacing a watch dialog, an earlier submission shall not dismiss the current dialog, change its raw draft or focus/caret, publish an inline error there, or release its pending Save state. This applies to the same watch, another watch, create mode, another kind or workspace, and unmount/remount.
- **AC-INTEGRATIONS-GITLAB-INTEGRATION-001.22:** A current opening's successful Save shall dismiss it; a failed Save shall retain its draft, show its inline error, release its pending state and allow retry. A new opening shall start with usable Save/Cancel controls independent of an earlier pending submission.
- **AC-INTEGRATIONS-GITLAB-INTEGRATION-001.23:** Each opening shall initialize from the selected watch or create defaults. Same-identity watch data refreshes shall preserve an existing raw draft and pending ownership; changing workspace, kind or selected watch shall initialize the newly selected form. Development lifecycle replay shall preserve these guarantees.

This clarification excludes persistent drafts after closure, watch-list cache
ownership, new polling or API contracts, and changes to other integrations.


#### Merge API source-head compatibility

The existing merge action and auto-merge shall work with GitLab instances or
groups that require a commit SHA on the merge API. The
[repair plan](../../../plans/gitlab-required-merge-sha/plan.md) records the
implementation and verification; unrelated criteria retain their lifecycle.

- **AC-INTEGRATIONS-GITLAB-INTEGRATION-001.24:** When a valid merge is requested through the existing action or automation, Kandev shall bind the merge to the current source-head SHA obtained for that same merge request and workspace connection, including when GitLab requires the SHA. Hosts that do not require it shall remain supported.
- **AC-INTEGRATIONS-GITLAB-INTEGRATION-001.25:** When the source head changes between the read and merge, the provider's mismatch shall remain a failure; Kandev shall not automatically retry that merge with the newer head. Obtaining a current head does not assert that a user reviewed it.
- **AC-INTEGRATIONS-GITLAB-INTEGRATION-001.26:** When the source-head SHA cannot be obtained or is empty or unusable, the operation shall fail without issuing a merge write. Existing provider failures shall remain failures.
- **AC-INTEGRATIONS-GITLAB-INTEGRATION-001.27:** The operation shall retain its workspace, configured host and credential, full project path and MR IID, method validation, squash and message behavior, and existing source-branch removal behavior.

This clarification excludes review-freshness UX, new permissions or settings,
source-branch deletion, and changes to other GitLab features or providers.

## System design

The migrated technical source is split into [part 1](../system-design/gitlab-integration-01.md), [part 2](../system-design/gitlab-integration-02.md).
