---
status: current
system: agents
requirements:
  - REQ-AGENTS-NATIVE-CODE-REVIEW-001
---

# Native review finding action publication

## Purpose and boundaries

Agents owns native code-review findings and their disposition contract. This
design extends the existing [requirement](../requirements/native-code-review.md)
through the shared client action path. The [status atomicity design](native-code-review-status-atomicity.md)
continues to own the persisted transition. No protocol, database, request
cancellation, component lifetime, layout, copy, or touch contract changes.

The guarantee concerns completions of overlapping `useFindingActions` calls in
one app store. Local initiation order determines which acknowledged local action
may publish. It does not describe server execution order, chronological database
state, live-event order, or agreement across browsers. Independent writers keep
their existing behavior and can replace the locally owned display.

## Requirement mapping

| Criteria under `REQ-AGENTS-NATIVE-CODE-REVIEW-001` | Design section |
| --- | --- |
| `.4`, `.9`, `.11` | Shared publication owner |
| `.10`, `.12` | Admission and settlement |
| `.13` | Existing writers and ownership loss |

## Consumer path and corrected defect

- `InlineReviewFinding` selects the active task and creates `useFindingActions`.
  `ReviewFindingCard` allows Resolve, then Undo, then Dismiss while prior
  requests remain pending. It does not await or disable those callbacks.
- `use-diff-annotation-renderer.tsx` and `UnanchoredFindingsBanner` mount that
  container. Multiple surfaces can therefore create separate hook instances
  for the same store/finding. `ReviewFindingsOverview` reads the resulting rows
  and filters open findings; it does not own disposition actions.
- Before the correction, `useFindingActions` published the input row with an
  optimistic status, then unconditionally published the returned or original row.
  It now delegates synchronous publication ownership to `beginFindingAction`.
- `updateReviewFindingStatus` uses the actual `task.review.finding.update`
  WebSocket action with `{finding_id, status}` and extracts `response.finding`.
- `createReviewSlice` owns the task collections. `setTaskReview` replaces a
  snapshot, `addReviewFindings` merges and removes superseded IDs,
  `updateReviewFinding` replaces or inserts an unseen row, and
  `clearTaskReviewState` removes task review state. `registerReviewHandlers`
  calls these existing actions for live events; `useTaskReview` backfills them.

## Shared publication owner

Add the dependency-neutral `apps/web/lib/review/finding-action-publication.ts`
helper and call it from the shared hook. Use a module-local
`WeakMap<StoreApi<AppState>, ...>` with nested task-ID and finding-ID maps.
Do not use a per-hook ref, one global finding-ID map, string concatenation for
identity, or a new persisted/store slice field.

Each overlapping group retains only:

- Its unique group identity, a monotonically increasing local action ordinal,
  and the latest action's pending/settled condition.
- The highest successful action ordinal and its complete acknowledged row;
  initialize this rollback baseline from the current stored row at admission.
- The outstanding request count and the exact row object this group passes
  to its publication setter. An independent synchronous replacement must never
  become action-owned merely because a subsequent store read returns it.

The count keeps the group available when newer actions finish before older
ones. Once all its requests settle, delete it only if the registry still points
to that group; an obsolete completion must not delete a replacement group.
Remove empty nested maps. The weak store key allows ordinary store collection.
No subscriptions, timers, queues, request retries, or lifecycle service are needed.

## Admission and settlement

1. Capture the store, task, and finding identity at call time. A missing task
   keeps the existing no-op. Read the current stored finding rather than trust
   an older card prop. A finding already absent cannot admit a display mutation.
2. Join an existing group only while the current row is its last owned row.
   Otherwise start a fresh group from the present row and leave old completions
   unable to publish. Increment the ordinal/count and mark this action newest
   and pending. Publish a copy of the current row with the requested status.
3. Call `updateReviewFindingStatus` through the existing API. The helper owns
   synchronous publication bookkeeping, not transport, translation, or toast.
4. On success, if the group still owns the row, retain the complete response as
   the baseline only when its ordinal exceeds the highest successful ordinal.
   An older success cannot replace a newer acknowledged response. Use the
   existing API response contract without adding wire validation behavior.
5. A success of the newest action publishes its acknowledged row. An older
   success while the newest action is pending updates only the baseline. If
   the newest action has already failed, an older success that advances the
   baseline may publish it. Older pending actions never resume their optimistic
   display after a newer failure.
6. On failure, only the newest action can roll the display back, and it uses the
   acknowledged baseline at settlement time. An older rejection does not write
   a row. Keep the hook's existing error toast for every rejection, with the
   current title, description/fallback, and error reporting path.
7. Settlement always releases its own outstanding count, including after
   ownership loss. Keep the baseline while any older request can still settle.

The returned row is accepted as an acknowledgement; timestamps are metadata,
not a universal ordering authority. Do not compare status strings as a fence:
Resolve, Undo, Resolve can have equal statuses with distinct action identities.

### Required overlap examples

| Local actions and completions | Display and baseline |
| --- | --- |
| Resolve pending; Undo acknowledged; Dismiss acknowledged; Resolve rejects or succeeds late | Dismissed throughout the late completion; full Dismiss acknowledgement retained. |
| Resolve pending; Undo pending; Resolve acknowledged; Undo rejects | Undo stays optimistically open until rejection; rollback to acknowledged Resolve. |
| Resolve pending; Undo rejects; Resolve succeeds later | Undo failure restores the pre-overlap row first; later Resolve acknowledgement becomes visible. |
| Resolve pending; Undo rejects; Resolve rejects later | Pre-overlap row retained; both failures use the existing toast path. |
| Resolve acknowledged; Undo pending; Dismiss pending; Undo acknowledged; Dismiss rejects | Roll back to acknowledged Undo, never the pending Dismiss or an older acknowledgement. |
| Two views issue successive actions on one store/finding | One shared group governs both, regardless of which hook instance settles. |

## Existing writers and ownership loss

Before admission to an existing group and before settlement publication, compare
the current stored row reference with the group's last owned row and check
registry group identity. If absent or replaced, relinquish that group's write
authority. A later callback can still complete and report an error, but cannot
insert or restore the old row. New actions on a present replacement start from
that replacement's state.

This is a local publication guard, not a journal of all store changes. A live
`task.review.finding_updated` event, even an echo, can replace the row and take
ownership. It then follows the existing event contract. The design does not
reorder those events, infer their originating request, or promise to detect an
unobserved remove-and-restore of the exact same row object. Unrelated collection
writes preserve the target row reference and must not suppress its action.

Leave review slice methods and WS handlers unchanged, including insertion of
unseen event rows. Test that snapshots, clear, and supersession actually retain
their semantics; do not redefine those writers as action acknowledgements.

## Rendered evidence and mobile parity

Use permanent `hooks/domains/review/use-finding-actions.test.tsx` tests with real
`StateProvider`/`createAppStore`, `ToastProvider`, `InlineReviewFinding`, its real
`ReviewFindingCard`, and `ReviewFindingsOverview`. Mock only the WS request and
frontend-error-report boundaries. Use a stable empty array for selectors and
initialize actual task/review state. Control the transport with deferred
promises; assert exact action/payloads, card disposition, overview membership,
full returned/rollback row data, and actual toast/report outcomes. Mount two
separate inline containers for the same finding and scope their controls.

Mobile parity uses the skill's pure state/data exception: existing phone diff
cards use the same hook; this correction changes no geometry, navigation,
scrolling, controls, touch behavior, or breakpoint logic. Rendered integration
tests exercise the shared state contract. Browser, build, and full-suite checks
are excluded from this work package, and no visual verification is claimed.

## Documentation and decisions

The public-doc audit found no published native finding disposition timing
contract to amend in `docs/public`, root `README.md`, or `docs/screenshots.md`.
This restores existing Resolve/Undo/Dismiss semantics with no new user action,
public API, terminology, or configuration. Update internal specs and this
[bounded delivery package](../../../plans/native-review-disposition-publication/plan.md).
Do not add a public page for this internal timing correction.

No new ADR is needed: this is a local action-publication correction within the
existing review owner, and this design preserves sufficient rationale. A
backend versioning or all-writer reconciliation decision would require a
separate scope and review.
