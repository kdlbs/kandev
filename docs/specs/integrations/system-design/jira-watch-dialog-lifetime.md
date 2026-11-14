---
status: current
system: integrations
requirements:
  - REQ-INTEGRATIONS-JIRA-WATCH-DIALOG-001
---

# Jira Watch Dialog Lifetime System Design

## Purpose and boundaries

Integrations owns this local Jira form boundary. Reuse the shipped opening/mount
pattern without introducing a shared framework, coordinator, queue or ADR.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-INTEGRATIONS-JIRA-WATCH-DIALOG-001 | Existing path; Opening ownership; Workspace changes; Completion; Desktop and phone |

## Existing path

The sole production caller, `JiraIssueWatchersSection`, is install-wide and
retains a selected watch snapshot. It does not key the section by active
workspace. `useJiraIssueWatches()` uses the actual Zustand list and Jira API
client through `fetchJson`. Create sends POST `/api/v1/jira/watches/issue`;
update sends PATCH `/api/v1/jira/watches/issue/:id` with the selected row's stored
`workspace_id`. The section wraps these actions in success/error toasts.
Accepted results publish regardless of the form owner's lifetime.

## Opening ownership

Render a local opening only while open, keyed by scalar selected watch ID and
stored workspace plus the optional caller-bound workspace. Seed fields once per
opening. Do not key existing edits or unlocked create forms to the unrelated
global active workspace; do not use watch object identity as a reset signal.

A layout-effect mount guard invalidates authority only on committed unmount and
restores liveness during StrictMode replay. Dismissal invalidates immediately
before notifying the parent. A request token belongs to this opening and is
captured before awaiting the existing action. An uncommitted suspended render
cannot invalidate the visible owner. Busy admission uses the same immediate
token so duplicate callbacks cannot admit a second save before React commits.

## Workspace changes

Existing edits lock workspace to their row; an optional caller workspace locks
the create picker. Unlocked creation seeds from the active workspace once.
If initially absent, the first default may fill an empty workspace through
`clearWorkspaceScopedForm`, retaining the user's other raw fields. Later active
workspace hydration does not replace a user selection or reset fields.

An explicit create-picker change invokes `clearWorkspaceScopedForm` and retires
any pending request token, clearing only that old form busy state. Reselecting
the same workspace is a no-op. Preserve JQL, prompt, profiles, polling, enabled
and throttle values. Workflow, step, repository and base branch follow the
existing shared reset contract. No shared helper changes are needed.

## Completion

Capture ID, workspace and payload at admission; preserve existing raw JQL and
all payload/default normalization. The frontend sends raw JQL; the existing
backend trims its persisted JQL. Transport fixtures may return raw accepted data,
while browser assertions use the actual normalized persistence. Await real actions to retain store/list and
truthful toast behavior. Only the live opening holding the same request token
may dismiss or clear busy state. Current failure preserves fields and permits
retry; stale failure remains reported by the caller without mutating the form.
Cancel does not abort or retarget accepted transport. Backend persistence,
authorization, polling and write ordering remain unchanged.

## Desktop and phone

Retain the shipped dialog and its scrolling settings flow on desktop and Pixel
5. No layout or touch primitive changes. Both paths use the same local owner;
phone verification uses native taps from actual Jira settings, including Cancel,
reopening, focus/selection and the next Save.

## Verification boundary

Permanent transport tests mount the actual install-wide section, domain hook,
API, store and State/Toast/Tooltip/SettingsSave providers with held HTTP. Only
unrelated Monaco rendering may be adapted for jsdom. Additional real-domain
consumers cover passive hydration, committed selection/bound scope changes,
StrictMode, unmount and suspended uncommitted renders. Browser controls hold the
response after actual disposable backend persistence and verify owned fixture
cleanup, including a deliberate failure path.
