---
status: current
system: integrations
requirements:
  - REQ-INTEGRATIONS-SENTRY-WATCH-DIALOG-001
---

# Sentry Watch Dialog Lifetime System Design

## Purpose and boundaries

This local form repair belongs to Integrations. It follows the existing
opening/mount boundary used by GitLab watch dialogs; it creates no shared
framework, durable authority or architectural decision.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-INTEGRATIONS-SENTRY-WATCH-DIALOG-001 | Existing path; Opening ownership; Completion and recovery; Desktop and phone |

## Existing path

`SentryIntegrationPage` keys its workspace subtree by the resolved workspace.
A route or active-workspace replacement therefore unmounts the entire section.
`SentryIssueWatchersSection` retains the selected watch snapshot separately from
its hook's list; accepted list updates do not replace that selected snapshot.
`useSentryIssueWatches` owns local list state, not an app-store watcher list.
Its create/update calls reach `lib/api/domains/sentry-api.ts` and `fetchJson`
(POST `/api/v1/sentry/watches/issue`, PATCH `/api/v1/sentry/watches/issue/:id`).
The section wraps them with truthful success/error toasts. These contracts and
the backend's persistence and ordering remain unchanged.

## Opening ownership

`SentryIssueWatchDialog` renders a form owner only while open. Key that owner
by resolved form scope plus selected watch ID, workspace and instance IDs.
Use scalar semantic identity, never watch object reference or serialized draft.
Initialize fields once per owner from the selected watch or create defaults.
Same-ID passive hydration preserves fields, selection and pending state.
The optional workspace prop takes precedence over the active workspace for
create defaults. The actual settings parent supplies workspace lifetime by
keyed unmount; preserve that existing boundary rather than infer a list scope
from a stale selected watch object.

A layout-effect mount guard becomes inactive on committed unmount and is restored
by development lifecycle replay. Cancel/dismiss invalidates it immediately before
calling the parent. A suspended, uncommitted alternative render cannot retire the
visible owner. Busy state belongs to this mounted form only.

## Completion and recovery

Save builds its payload and captures the original watch/workspace/instance before
awaiting the existing action. Always let that action publish accepted results and
truthful toasts. Only a live opening may close or clear its own busy state.
A stale error remains reported by the section while the later form is untouched.
Current errors preserve the draft and enable retry. Closing does not abort, replay
or retarget a write. Scope replacement and remount cannot revive an old owner.
Existing lookup cancellation, sole-instance/org defaults, project selection and
repository defaults remain local to each form.

## Desktop and phone

Reuse the shipped Sentry settings page, tappable watcher rows and scrollable
watch dialog with its existing Cancel/Update/Create footer. No composition,
copy, viewport branch or geometry changes are introduced. Both viewports share
the same form owner. Browser checks use desktop clicks and emulated Pixel 5 native
taps through the real settings route and fresh production assets, disposable
backend/mock Sentry, and a held response after a real accepted write. They prove
browser interaction, not physical-device, OS-keyboard or operator-incident behavior.

## Implementation record

[Plan](../../../plans/sentry-watch-dialog-ownership/plan.md).
