---
status: current
system: integrations
created: 2026-10-27
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-DETAIL-DECISION-001
owners:
  - kandev
---

# GitHub PR Detail Review Decision System Design

## Purpose and boundaries

The GitHub integration supplies feedback history to the task's detail view.
The detail header derives a read-only summary; it does not change GitHub's
review or merge authority. No new persistence, transport, metric, copy, layout,
or provider policy is introduced. The existing review-request interaction is
separate and must remain intact.

## Requirement mapping

| Criteria | Design |
| --- | --- |
| AC .1-.3 | Binding history and detail fallback below |
| AC .4 | Consumers and verification below |

## Binding history and detail fallback

`PRDetailContent` obtains `PRFeedback` through the actual
`hooks/domains/github/use-pr-feedback.ts` resource and passes a model from
`derivePanelMetrics` to shared `ChangeRequestDetail`. Today
`computeLiveReviewState` takes the latest row of every author regardless of
state and requires every selected row to be approved. An `APPROVED` at 10:00
followed by `COMMENTED` at 11:00 by the same author becomes pending even though
the comment body renders. This loss was observed in the actual component in
HappyDOM at main `8f9b531d416950223f9d3037b6be48a9dcf93ff2`; it is not a
Chromium observation or evidence of a response freshness failure.

Use the existing `latestReviewStateByAuthor` and `reduceReviewSummary` in
`apps/backend/internal/github/client_helpers.go`, used by GraphQL
`summarizeReviewState`, as the binding decision reference. States are
uppercased there; author keys are exact strings, not case-folded. Each sample
uses the provider's submission time: GH and PAT REST adapters map GitHub
`submitted_at` into `PRReview.created_at`, while GraphQL uses `SubmittedAt`.
The reducer processes source order, retains a previously seen author entry on a
nonbinding row, and accepts a binding row whose time is at least the retained
time. Equal-time binding rows thus favor the later encountered row. Empty
names use the backend's timestamp-derived anonymous identity. The supplied
history is normally chronological; do not assume array order alone determines
which binding decision is newer. These edge semantics are reference behavior,
not a new GitHub identity or timestamp policy.

A remaining `CHANGES_REQUESTED` wins across authors; otherwise at least one
`APPROVED` yields approved; otherwise the binding summary is empty. In detail,
keep the existing no-reviews fallback: current requested reviewers yield pending,
otherwise use the linked PR's fallback state or empty. For nonempty history with
no binding result preserve the current pending presentation, including
neutral-only and dismissed-only histories; do not infer approval from the
fallback in this branch. Requested reviewers and individual review rows retain
their existing independent logic. The frontend implementation may use a small
pure helper close to this GitHub domain, only for this detail derivation. It
must not mutate or filter feedback history.

The REST `computeOverallReviewState` in `service_pr_status.go` currently has a
separate latest-row/all-approved rule. This design does not claim it agrees
with the GraphQL reducer or authorize changing it, counts, `has_issues`, status
synchronization, watchers, or persisted task PRs. Existing detail fallback is
not a guarantee of a fresh or authoritative GitHub merge decision.

## Consumers and verification

Desktop uses dockview `ReviewDetailPanelComponent` and the GitHub detail panel.
On phone, `SessionMobileBottomNav` opens Review; `SessionMobileLayout` renders
`MobileReviewPanel`, its existing selector, and `ReviewDetailPanelComponent`,
which resolves the selected GitHub PR to the same detail component. The phone
surface and scroll ownership remain unchanged. This is state/data normalization
only: no viewport-specific layout, interaction, touch target, or navigation
changes. The mobile-parity pure-state exception is appropriate only with a
focused rendered phone-consumer check in the work order; it does not claim the
original desktop HappyDOM proof exercised the phone path.

An independently authored regression renders the real detail, shared model,
state provider and feedback hook with only the domain feedback HTTP boundary
(and necessary WS/panel-title transport) mocked. It asserts the real header
and review body, with an approval-only control before the correction. Cover
binding transitions, mixed authors, neutral and dismissed history, requested
reviewers, no reviews, ordering and casing against the reference. Preserve
existing review-request tests. HappyDOM is a component environment, not a
Chromium run. Any inability to execute the real phone consumer check must be
reported rather than called a mobile pass.

Public documentation assessment: `docs/public/sessions-and-review.md` already
describes the existing detail and phone entry without specifying the incorrect
reduction. No public documentation or screenshot change is needed for this
bounded display correction; reassess if implementation changes a documented
workflow or wording.
