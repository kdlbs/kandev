/**
 * Pinned-column target enforcement primitives.
 *
 * Lives in the state layer (next to `dockview-store`) so the store can
 * call enforcement without crossing into the component layer. Sash-drag
 * handlers in `components/task/dockview-layout-setup.ts` toggle the
 * `sashDragging` flag via `setSashDragging` and consume `enforcePinnedTargets`
 * for the reactive `onDidLayoutChange` callback - both directions now
 * import from this module, breaking the previous
 * state-imports-component-imports-state cycle.
 *
 * Dockview's splitview rebalances proportionally on any `api.layout` call,
 * which would otherwise grow pinned columns past their initial defaults on
 * container expansion and shrink them on container contraction. We treat
 * sidebar/right as having a *target width* (stored in
 * `layout-manager/pinned-targets.ts`) that is updated only by explicit user
 * actions (drag, initial layout, restore from saved); after every
 * layout-change event we force the live columns back to their targets via
 * `sv.resizeView`.
 */
import type { DockviewApi } from "dockview-react";
import { getRootSplitview, getPinnedTarget, computeSidebarMaxPx } from "./layout-manager";
import { createDebugLogger, IS_DEBUG } from "@/lib/debug/log";

const debugWidths = createDebugLogger("dockview:widths");

/** Enforcement-in-progress guard to prevent infinite loops when our own
 *  `sv.resizeView` triggers `onDidLayoutChange`. */
let enforcing = false;

/** True while the user is actively dragging a `.dv-sash`. We pause target
 *  enforcement during the drag so the in-progress resize doesn't get
 *  reverted to the previous target on every intermediate layout change. */
let sashDragging = false;

/** Setter for the sash-drag flag. Sash mousedown/mouseup handlers in
 *  `components/task/dockview-layout-setup.ts` call this to gate enforcement
 *  during user drags. Reading from outside isn't useful because the value
 *  flips synchronously with the drag - call sites that need the state
 *  check it indirectly via `enforcePinnedTargets`'s short-circuit. */
export function setSashDragging(v: boolean): void {
  sashDragging = v;
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function restoreColumnToTarget(sv: any, idx: number, target: number | undefined): void {
  if (target === undefined) return;
  const cur = sv.getViewSize(idx);
  if (Math.abs(cur - target) <= 1) return;
  if (IS_DEBUG) {
    debugWidths(`enforce-restore idx=${idx} cur=${Math.round(cur)} target=${Math.round(target)}`);
  }
  try {
    sv.resizeView(idx, target);
  } catch {
    /* dockview rejects unreachable sizes — ignore */
  }
}

/** Visibility/maximize state needed by `enforcePinnedTargets`. Passed in by
 *  callers (instead of read via `useDockviewStore.getState()`) so this module
 *  doesn't depend on the store and the import graph stays one-way. */
export type EnforcePinnedTargetsCtx = {
  sidebarVisible: boolean;
  rightPanelsVisible: boolean;
  /** Non-null when we are in (or restoring) a maximized-group state; skip
   *  enforcement so we don't fight the 2-column maximize overlay. */
  maximized: boolean;
};

/**
 * Snap the pinned columns back to their recorded targets.
 *
 * Two call modes:
 *   - Reactive: wired in `setupSashDragCapToggle` via `onDidLayoutChange`.
 *     The subscription pre-checks `isRestoringLayout` and skips during
 *     restore so this function does not fight an in-progress programmatic
 *     layout.
 *   - Proactive: called synchronously by programmatic layout paths
 *     (build-default, env-switch, preset/custom-layout apply, sidebar/right
 *     toggle, maximize/exit) *inside* the post-`api.layout` rAF, before
 *     `isRestoringLayout` flips false. Dockview's proportional rebalance
 *     after `api.layout` can grow pinned columns up to their loose
 *     `setConstraints.maximumWidth`; without this synchronous snap the
 *     correction would only happen on the next user-triggered layout-change
 *     event, producing a visible jerk after env prep settles.
 *
 * The `enforcing` re-entry guard remains so the `sv.resizeView` call we emit
 * does not feed back through the layout-change subscription. `sashDragging`
 * still gates the whole thing - mid-drag, we want dockview to follow the
 * user's mouse, not snap to the previous target.
 */
export function enforcePinnedTargets(api: DockviewApi, ctx: EnforcePinnedTargetsCtx): void {
  if (enforcing || sashDragging) return;
  if (api.hasMaximizedGroup() || ctx.maximized) return;
  const sv = getRootSplitview(api);
  if (!sv || sv.length < 2) return;
  enforcing = true;
  try {
    if (ctx.sidebarVisible) {
      // Clamp the (global, raw) sidebar target to the current screen so a width
      // set on a wide monitor fits a narrower one. Storage keeps the raw value,
      // so returning to the wide monitor restores it.
      const raw = getPinnedTarget("sidebar");
      // Guard against an unmeasured container (api.width === 0): passing 0 to
      // computeSidebarMaxPx bypasses its window.innerWidth fallback and clamps
      // the target down to LAYOUT_PINNED_MIN_PX. undefined lets it fall back.
      const safeWidth = api.width > 0 ? api.width : undefined;
      const clamped = raw === undefined ? undefined : Math.min(raw, computeSidebarMaxPx(safeWidth));
      restoreColumnToTarget(sv, 0, clamped);
    }
    if (ctx.rightPanelsVisible) {
      restoreColumnToTarget(sv, sv.length - 1, getPinnedTarget("right"));
    }
  } finally {
    enforcing = false;
  }
}
