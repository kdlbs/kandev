---
status: current
system: ui
requirements:
  - REQ-UI-REVIEW-VIEWED-001
---

# Review viewed toggling design

## Boundary and requirement mapping

`REQ-UI-REVIEW-VIEWED-001` is implemented by per-file scroll crossing state in
`ReviewDiffList`. AC-001.1 through AC-001.3 map to crossing detection; AC-001.4
to eligibility and suppression; AC-001.5 to shared state and presentation below.
The AC suffixes here use the full `AC-UI-REVIEW-VIEWED` prefix.

## Current failure

`useAutoMarkOnScroll` in `components/review/review-diff-list.tsx` disconnects
while reviewed and resets `autoMarkedRef`. Clearing the checkbox reconnects
because `isReviewed` is an effect dependency. The observer's initial sample
already satisfies non-intersecting and above-root geometry. After prior user
scroll input, `suppressAutoMark.current` is false, so the callback immediately
writes reviewed=true without another crossing.

## Crossing detection

Observe geometry across review state changes rather than treating each observer
subscription as a reading pass. Retain a per-file baseline of whether the start
sentinel is above the root. The first sample establishes a baseline only.
A transition from not-above to above can mark a currently eligible file once,
provided existing user-scroll suppression allows marking. Update the baseline
even when the file is reviewed or suppression blocks a mark; this prevents a
later uncheck or release of suppression from replaying a consumed crossing.

Keep the observer stable across checkbox changes and callback identity changes,
using the latest review eligibility and handler without reconnecting solely for
those changes. If reconnection is required by root or sentinel replacement,
the first sample is a new baseline, never proof of a crossing. An observer
lifetime must not leak queued callbacks after cleanup.

Manual clearing while already above leaves the baseline above. Further samples
above cannot mark. Returning the sentinel into or below the viewport changes
the baseline to not-above. Passing above again under manual scroll can mark.
Observe while reviewed so an uncheck can use the existing geometry baseline.
Turning automatic marking back on establishes a baseline rather than marking
already-passed files. Preserve pending/stale eligibility gates and do not add a
permanent manual veto or a timing debounce.

Use the existing root and start sentinel, preserving the shipped marking
threshold. `useEffectEvent` supplies current eligibility and the handler without
observer reconnection. A passive root scroll listener reads the marker and root
geometry together with current `scrollTop`. Only that listener can certify a
crossing. The marker displacement must agree with the root scroll delta within
one CSS pixel of rounding; layout changes cannot borrow earlier input.
Intersection observations refresh the current geometry baseline without marking.
This avoids comparing an older observer snapshot against a later touch-scroll
position. Remove the listener and disconnect the observer together on cleanup.
Preserve `useReviewDiffScrollInteraction` and `useReviewScrollAnchor` navigation
suppression. Input cancellation first restores the previous suppression value
and cancels the queued anchor frame. Then wheel/touch movement, scroll keys,
a pointer press on the scroll root, and touch contact anywhere inside the
scroller explicitly allow automatic marking. That explicit input must win over
the cancelled restoration's saved value. Chromium can scroll a touch gesture
on the compositor without delivering `touchmove`; touch contact only permits
a subsequent certified crossing and never marks by itself. Mouse and pen
presses on descendants cancel stale restoration without allowing marking.
Unrelated keys do neither. This changes no crossing geometry or eligibility gate.

## Shared state, identity, and persistence

Continue invoking `onToggleReviewed` with `reviewFileKey(file)` and the latest
handler. `useSessionFileReviews` remains the sole existing persistence path.
Desktop tree and header controls continue deriving from the shared reviewed set.
Keep transient crossing state local to the file section and reset it when its
session/source/file identity changes or the section unmounts. No schema, API,
settings, telemetry, or locale changes are required.

## Desktop and phone

Retain `ReviewDialogSurface`: desktop has a fixed file tree beside the diff
scroller; phone hides the tree and exposes the viewed control in each file
header. The nearest live exemplar is `review-dialog-surface.tsx` with
`ReviewDiffHeader`; the curated dense-viewer pattern is the full-height
`mobile/mobile-diff-sheet.tsx`. Sustained reading remains in the existing dialog,
with one diff scroll owner. Keep viewport bounds, safe-area handling, current
navigation, and existing touch hit areas. Business logic is shared.

## Verification

Extend `review-diff-list-auto-mark.test.tsx` with a controlled reviewed-set
harness and active-observer semantics. Prove uncheck/rerender stability, genuine
return-and-pass rearming, reviewed-state tracking, reconnection baseline,
suppression, disabled/stale/pending content, identity isolation and cleanup.
Add desktop and mobile browser regressions using real scroll geometry and real
checkbox mutations; do not force observer callbacks in browser tests. Include
a below-viewport trailing file so the target start can pass the viewport top.
Cover touch contact on diff descendants followed by a scroll crossing without
`touchmove`, and preserve suppression for descendant mouse and pen clicks.
Contact without a certified crossing must not mark a file.

## Implementation plan

[Plan and work order](../../../plans/review-viewed-toggling/plan.md).
