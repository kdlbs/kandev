---
status: current
system: ui
requirements:
  - REQ-UI-PR-ONLY-COMMIT-DETAILS-001
---

# Commit Detail Target Types System Design

## Context and ownership

The Changes panel uses a shared source-aware contract when opening commit
details. Its pure TypeScript definitions live in
`apps/web/lib/state/diff-target-types.ts`, below the component layer. The
module defines `CommitDetailTarget`, `OpenDiffOptions`, `DiffSheetMode`,
`DiffSource`, and the `ChangeLayer` alias. It owns type shapes and source
identity; it does not implement navigation or request behavior.

## Requirement mapping

| Criteria for REQ-UI-PR-ONLY-COMMIT-DETAILS-001 | Design section                   |
| ---------------------------------------------- | -------------------------------- |
| .1                                             | Source-aware target contract     |
| .2, .3                                         | Local and GitHub target identity |
| .6                                             | Shared detail consumers          |
| .8, .9                                         | Committed reader lifetime        |

## Source-aware target contract

`CommitDetailTarget` is a discriminated union with `source: "local"` and
`source: "github"` variants. Both retain the exact `sha`. The GitHub variant
also retains `workspaceId`, `owner`, and `repo`; optional `repositoryName` is a
local display/group identity. The local variant may carry the optional local
repository subpath. This shape preserves one explicit source identity across a
commit row and its detail view.

`DiffSource`, `OpenDiffOptions`, and `DiffSheetMode` carry the existing diff
selection and sheet modes. `ChangeLayer` aliases `GitChangeLayer` from
`lib/state/slices/session-runtime/types.ts` through a type-only import. That
runtime type module does not import this contract, keeping the dependency
acyclic.

## Local and GitHub target identity

The target factories in `components/task/changes-panel-helpers.ts` retain
local-versus-GitHub source selection. A GitHub target carries the selected
workspace, owner, repository, and SHA. The shared types constrain the payload.
The factories and consumers preserve the existing behavior in
[REQ-UI-PR-ONLY-COMMIT-DETAILS-001](../requirements/pr-only-commit-details.md),
AC .1-.3.

## Shared detail consumers

State actions and the dockview store import these contracts as types. Desktop
and mobile consumers share `CommitDetailTarget` and keep source-aware data, as
required by AC .6. The definitions in `lib/state` remove the
component-to-state reverse dependency. They add no runtime module edge.

## Committed reader lifetime

`hooks/domains/session/use-commit-detail.ts` owns request admission and
publication for each mounted reader. `CommitRowFiles`, `CommitDetailPanel`,
and `CommitDiffView` consume it; `mobile/mobile-diff-sheet.tsx` embeds
`CommitDiffView`. These consumers share the same retirement behavior. A hidden
inline list can keep its reader mounted; hiding alone does not retire it.

The hook retains its existing target key and loaded-key projection. GitHub
request identity uses workspace, owner, repository, and SHA. Local identity
uses repository and SHA, with the existing active session, resolved session
task (active-task fallback), and agentctl readiness passed to the transport.
Readiness remains a request/retry input, not a new commit-data identity. Local
context updates must preserve their existing fetch dependencies; unrelated
local readiness changes must not refetch a GitHub target. Display metadata and
file-navigation requests do not become transport identity.

Use a small per-instance lifetime associated with the committed fetch inputs,
following the layout-bound lifetime pattern in `hooks/use-file-upload.ts`.
Create an inactive owner without changing shared refs during render. Layout
setup admits the committed owner; layout cleanup retires it on replacement or
unmount and invalidates requests admitted before cleanup. Passive cleanup alone
leaves a publication window after the replacement has committed.

The returned fetch callback captures its owner. Before changing state or
dispatching transport, it verifies that owner is still admitted. A retained
callback cannot revive an owner after replacement, unmount, or an A -> B -> A
target cycle. On each admitted request, capture the existing monotonic request
sequence. Success, catch (including protocol failures and toast publication),
and finally may publish only while both the owner and sequence remain current.
Latest-request ordering remains local to the hook instance.

StrictMode layout setup/cleanup/setup must admit the replayed current reader
while invalidating requests from the earlier setup. A request captures its
generation before awaiting; reopening a live flag alone must not make an old
request current again. Normal same-owner refetch still works. Preserve passive
initial fetching, local readiness retry, current protocol-error localization,
current error toasts, and the returned result shape. GitHub errors never fall
back to local data.

Real-provider React DOM integration tests keep `StateProvider` and
`ToastProvider` mounted while removing or replacing the reader. Mock only
`requestCommitDetail` and the frontend-error-report transport sink, retaining
the real `CommitDetailProtocolError`. Assert the hook's rendered state, actual
toast DOM, report sink, and transport admissions. A layout-bound retained
callback invocation must prove retirement before passive cleanup. Use deferred
successes and failures, StrictMode replay, independent instances, overlap,
closed/reopened readers, and current failure/retry controls. Clean all owned
fake timers without changing provider timer behavior.

This extension changes no transport, store, global reporter, persistence, or
provider contracts. Desktop panels, phone sheets, touch targets, copy,
navigation, scrolling, and breakpoints keep their existing composition. The
repair is state-only; provider-rendering DOM tests establish its lifecycle
boundary without a browser or E2E run. No ADR is needed for this local reuse of
an established owner-lifetime pattern.

## Related delivery and decision

- [Frontend state/UI ownership refactor](../../../plans/frontend-state-ui-ownership/plan.md)
- [Commit-detail reader lifetime](../../../plans/commit-detail-reader-lifetime/plan.md)
- [ADR 2026-08-01: Architecture lint budgets](../../../decisions/2026-08-01-architecture-lint-budgets.md)
