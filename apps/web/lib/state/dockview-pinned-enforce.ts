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
import {
  getRootOrientation,
  getRootSplitview,
  getPinnedTarget,
  computeSidebarMaxPx,
} from "./layout-manager";
import { getPinnedRightColumn } from "./layout-manager/pinned-right-column";
import { getRootSidebarWidth } from "./layout-manager/applier";
import { getReusableRightWidth, setPinnedTarget } from "./layout-manager/pinned-targets";
import { restoreReusableRootGeometry } from "./layout-manager/reusable-root-geometry";
import { computeRightMaxPx, LAYOUT_PINNED_MIN_PX } from "./layout-manager/caps";
import { resolveResponsiveRightWidth } from "./layout-manager/right-width";
import { getManualRightWidth } from "@/lib/local-storage";
import type { HiddenRightPane } from "./dockview-right-pane";
import { createDebugLogger, isDebug } from "@/lib/debug/log";
import { measureDockviewGridWidth } from "./dockview-measure";

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
  if (isDebug()) {
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
  envId?: string | null;
  /** A hidden pane retains the complete root source until its full structure returns. */
  hiddenRightPane?: HiddenRightPane | null;
};

type RightSplitview = {
  length: number;
  getViewSize: (index: number) => number;
  resizeView: (index: number, size: number) => void;
};

function resolveRightTarget(
  ctx: EnforcePinnedTargetsCtx,
  sidebarWidth: number,
  width: number | undefined,
  manualWidth: number | null,
): number {
  const persistedTarget =
    manualWidth ?? getReusableRightWidth(ctx.envId, width) ?? getPinnedTarget("right") ?? null;
  return resolveResponsiveRightWidth(width, sidebarWidth, persistedTarget);
}

function enforceRightColumn(
  api: DockviewApi,
  ctx: EnforcePinnedTargetsCtx,
  sv: RightSplitview,
): void {
  const width = measureDockviewGridWidth(api);
  const right = getPinnedRightColumn(api);
  const sidebarWidth = getRootSidebarWidth(api);
  const manualWidth = getManualRightWidth(ctx.envId ?? null);
  const target = resolveRightTarget(ctx, sidebarWidth, width, manualWidth);
  if (right) {
    const maximumWidth = computeRightMaxPx(width, sidebarWidth);
    for (const group of right.groups) {
      group.api.setConstraints({ maximumWidth, minimumWidth: LAYOUT_PINNED_MIN_PX });
    }
    setPinnedTarget("right", target);
    if (
      restoreReusableRootGeometry(api, ctx.envId, width, target, {
        manualWidth,
        hiddenRightPane: ctx.hiddenRightPane,
      })
    )
      return;
  }
  restoreColumnToTarget(sv, right?.index ?? sv.length - 1, target);
}

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
  // Pinned targets are horizontal widths; a vertical root's children are rows.
  if (getRootOrientation(api) === "VERTICAL") return;
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
    if (ctx.rightPanelsVisible) enforceRightColumn(api, ctx, sv);
  } finally {
    enforcing = false;
  }
}
