---
created: 2026-10-27
status: implemented
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-DETAIL-DECISION-001
system_design:
  - ../../specs/integrations/system-design/github-pr-detail-review-decision.md
legacy_specs: []
---

# Implementation Plan: GitHub PR Detail Binding Decision

## Overview and scope

Correct only the read-only GitHub PR detail summary derived from feedback
reviews. First independently establish the rendered defect and approval-only
control, then apply the existing GraphQL binding-summary semantics at this
consumer and verify desktop and actual phone consumption. One sequential work
order owns the full slice. ROOT must read the complete package before a later,
separate implementation instruction and exclusive heavy-work grant. This design
checkpoint grants no production or permanent test changes, installation,
staging, commit, push, PR, or merge.

Exclude backend status, `has_issues`, counts, watchers, GitLab, review-request
lifecycle, persisted task PR mutations, merge/approval authority, copy, layout,
and query synchronization. Preserve raw review bodies and requested/re-request
controls. The REST status rule differs from the binding GraphQL reducer and is
outside this fix.

## Technical approach

The relevant seam is `computeLiveReviewState` and `derivePanelMetrics` in
`apps/web/components/github/pr-detail-panel.tsx`, with a narrowly scoped pure
GitHub detail helper only if it improves clarity. Reuse the existing contract
in `apps/backend/internal/github/client_helpers.go`; do not change the resource,
hook, transport, raw reviews, or shared review actions. Explicit fallback and
source-order, timestamp, author and state casing semantics are in the design.

## ASCII UI preview

UI-01: existing desktop PR detail header and review history (AC .1-.3). The
baseline's missing badge is observed; exact spacing is illustrative.

```text
Before: PR detail                  After: PR detail
[PR title]                         [PR title] [Approved]
Reviews                            Reviews
  reviewer: approved                 reviewer: approved
  reviewer: follow-up comment        reviewer: follow-up comment
```

UI-02: phone task bottom navigation -> Review -> selected GitHub PR (AC .4).
Existing selector and full-height scroll surface remain; no new controls.

```text
[PR selector]
[PR title] [Approved]
Reviews
  reviewer: approved
  reviewer: follow-up comment
      (existing content scrolls)
[Chat] [Changes] [Review] ...
```

## Tests and E2E assessment

The new `components/github/pr-detail-review-decision.test.tsx` uses real
`PRDetailContent`, shared `ChangeRequestDetail`, `StateProvider`, and
`usePRFeedback`; mock only the domain `getPRFeedback` HTTP response and
necessary WS/panel-title transport. Before production edits, collect both the
causal failure and approval-only pass in the same run; assert the actual
header badge and follow-up body, not helper outputs alone. Independently author
fixtures, with no private ROOT reproduction source. Add meaningful controls
for all criteria and reference edge behavior, then GREEN. Add a rendered phone
consumer assertion through the shipped `MobileReviewPanel` and
`ReviewDetailPanelComponent` path, including real selected GitHub identity and
the same header and body, without substituting desktop-only markup. The
existing mobile layout test mocks the detail, so it alone is insufficient.

The change is purely local data derivation: no layout, touch, scrolling,
navigation, or viewport-sensitive interaction changes. The mobile-parity
pure-state exception applies with that actual rendered phone-consumer check;
no new Playwright test is required. HappyDOM does not establish Chromium or
physical phone behavior. If the phone consumer check cannot be performed,
record the blocker and do not claim it passed. Existing `pr-detail-panel.test.ts`,
`pr-reviews-section.test.tsx`, and mobile layout tests protect existing actions;
inspect their actual Vitest project mapping before invocation. New unreviewed
files belong to `browser-locales`, existing reviewed files to their configured
project; require nonzero discovery.

## Work orders

- [x] [Task 01: Correct the detail binding decision](task-01-detail-binding-decision.md)

## Verification results

Implementation checks: the independently authored regression collected two tests
with the expected causal failure and passing approval-only control before the
fix. After implementation, the locale-project decision and existing detail
suites passed 40 tests; the browser-project review section and mobile layout
suites passed 33 tests. Scoped ESLint passed and web typecheck passed at 6 GiB
after correcting a missing fixture workspace ID. The actual selected phone
consumer was exercised in HappyDOM, not Chromium. Qualification receipt: at main `8f9b531d416950223f9d3037b6be48a9dcf93ff2`,
HappyDOM actual handle 5975 joined exit 1, two tests collected, causal failure
and control pass. The current checkout is separately based at
`24e76dfa9c4bb73e2172a75cfe196e9f30e6d2a3`; no local implementation run is
claimed. Full commands and conditions are in the work order.

## Risks and documentation

- Binding reducer and REST status logic diverge; the latter is explicitly out
  of scope and may still display a different status elsewhere.
- Anonymous identity, nonbinding row ordering and equal timestamps have existing
  reference semantics; tests must distinguish them from invented policy.
- `docs/public/sessions-and-review.md` already documents the existing entry and
  generic review view; no public copy or screenshot update is planned because
  neither workflow nor terminology changes. Reassess actual implementation.
