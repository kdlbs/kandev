---
status: current
system: integrations
requirements:
  - REQ-INTEGRATIONS-LINEAR-WATCH-DIALOG-001
---

# Linear Watch Dialog Lifetime System Design

## Purpose and boundaries

This is a local form lifetime repair within Integrations, using existing React
mount and event ownership. It adds no global generation, framework or ADR.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-INTEGRATIONS-LINEAR-WATCH-DIALOG-001 | Existing path; Opening and scope; Completion; Desktop and phone |

## Existing path

`LinearIssueWatchersSection` lists every workspace, retains the editing watch
snapshot and wraps `useLinearIssueWatches` actions in truthful toasts. The domain
hook publishes accepted writes to the real app store through `linear-api.ts`
and `fetchJson`: POST `/api/v1/linear/watches/issue` and PATCH
`/api/v1/linear/watches/issue/:id?workspace_id=...`. The section forwards each
edited row's stored workspace. Preserve this path without changing shared cache
or backend ordering.

## Opening and scope

Render a form owner only while open, keyed by selected watch ID and stored
workspace plus optional locked workspace prop. Initialize its draft once.
Do not key the install-wide edit form to global active workspace, object
reference or draft values. Same-ID hydration leaves draft and pending state
untouched. A suspended alternative render cannot retire a committed owner.
A layout-effect live guard retires on committed unmount and reactivates during
StrictMode replay; dismiss retires it synchronously.

The active workspace seeds create defaults at opening and may fill a still-empty
workspace. It is not ongoing scope authority. Explicit create selection uses
`switchWatchWorkspace` on the existing draft, preserving portable fields and
clearing exactly the existing workspace-bound fields. A local submission token
is invalidated synchronously on an actual workspace switch, clearing local busy
state. Same-workspace selection is a no-op. Passive active/default hydration
cannot overwrite a chosen workspace or retire its save.

## Completion

Save captures payload, selected ID and create workspace before awaiting the
unchanged action. Busy admission and completion belong to its local submission
and mounted owner. Only that owner/token may dismiss or clear busy. Accepted
writes and caller feedback always settle. Current errors preserve draft/retry;
old errors leave the later form untouched. No request is aborted or retargeted.

## Desktop and phone

Reuse the shipped scrollable Linear watch dialog, picker, query input and
Cancel/Update/Create footer on both viewports. This repair changes state only,
not geometry, navigation, copy or touch composition. The existing settings
watcher row and dialog are the nearest mobile exemplar. Fresh production-browser
checks use desktop clicks and Pixel 5 native taps, a real isolated backend with
mock Linear, owned workspace/repository/offline origin and Worktree profile.
Held HTTP responses follow real backend acceptance. These are emulated browser
checks, not physical-device or operator-runtime claims.

## Implementation record

[Plan](../../../plans/linear-watch-dialog-ownership/plan.md).
