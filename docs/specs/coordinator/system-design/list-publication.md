---
id: coordinator-list-publication-design
title: Coordinator settings list read publication design
status: draft
system: coordinator
owners:
  - kandev
created: 2026-10-06
last_updated: 2026-10-06
requirements:
  - REQ-COORDINATOR-COORDINATORS-004
---

# Coordinator settings list read publication System Design

## Settings list read publication

`AC-COORDINATOR-COORDINATORS-004.8` is implemented locally in
`apps/web/hooks/domains/settings/use-coordinators.ts`. Its production consumers
are `CoordinatorsListPage` (rows, loaded/loading, error and refresh) and
`CoordinatorAddPage` (create only, though mounting the hook also starts its list
read). The flat `coordinators` slice and its actions remain the existing store
contract. The settings route admits these pages through
`WorkspaceCoordinatorRoute` and `useFeature("coordinator")`; this correction
does not alter admission or the flag defaults.

Initial load and explicit refresh share one local read path. Each admitted read
captures a unique request token, workspace and owning store action identity.
Every success, failure and finally publication requires that token to remain
current in the live hook lifetime. A local memoized lifetime identifies each
workspace/store-action owner. Layout-effect setup activates it; layout cleanup
retires it at the committed workspace/store/unmount boundary before passive
effects or successor layout callbacks. This precedes the null-workspace or
already-loaded early return and invalidates every read, including refreshes.
Refresh captures that lifetime and refuses admission after retirement, even if
a retained callback names the same workspace as a later visit. No global/store
state is mutated during render. A later same-ID visit or remount cannot revive
a token. Cancellation may save transport work,
but publication safety does not depend on the transport honoring cancellation.

Keep loaded-cache identity bound to the same owner as the accepted rows.
Expose rows only when that accepted workspace/store-action identity matches the
current render, so a workspace commit cannot construct foreign links before
passive loading starts. This is an identity gate, not a store rewrite.
Returning A-loaded -> B-pending -> A retains A's accepted rows and loaded state
without allowing B's callbacks to publish. A's cached branch must settle the
abandoned loading state itself; obsolete finally callbacks cannot do so. An
uncached workspace starts its normal load. Overlapping reads for one workspace
have separate tokens: an older completion cannot replace newer rows, clear the
newer pending state, erase a newer error or restore a stale loaded marker.
Owner replacement discards the old owner's local loaded/error identity and
performs the new owner's ordinary load even for the same workspace ID. Cleanup
may synchronously clear loading owned by its abandoned read, before a successor
starts; it must not clear a successor's loading.

Current failures keep the established inline error and Retry path, leave an
initial load unaccepted and retain any previously accepted rows. Current
success replaces rows in server order, clears error and accepts the loaded
workspace; its finally settles loading. Same-workspace cached rerenders do not
add requests, null workspaces do not fetch, and refresh remains explicit. CRUD
callbacks and read-versus-mutation ordering are outside this correction.

Validation uses the real hook, `StateProvider`/`AppStore`, API list client and
rendered `CoordinatorsListPage`, replacing only `fetchJson` transport with
deferred replies. Assert rows and loaded/loading/error state as well as rendered
name, Open and Configure link identity. Existing mocked-hook tests remain
compatibility checks, not evidence for the publication race. The correction is
state-only: phone and desktop share the hook, and no markup, copy, layout,
navigation or touch behavior changes. Targeted hook/component tests satisfy
the mobile-parity state-only exception; browser/build/E2E expansion requires
ROOT's causal-scope decision. Delivery is tracked by the
[list publication plan](../../../plans/coordinator-list-publication/plan.md).
