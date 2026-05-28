import type { DockviewApi } from "dockview-react";
import type { LayoutState } from "./types";
import { toSerializedDockview } from "./serializer";
import {
  SIDEBAR_LOCK,
  SIDEBAR_GROUP,
  CENTER_GROUP,
  RIGHT_TOP_GROUP,
  RIGHT_BOTTOM_GROUP,
  TERMINAL_DEFAULT_ID,
} from "./constants";
import { computePinnedMaxPxFor, LAYOUT_PINNED_MIN_PX } from "./caps";
import { setPinnedTarget } from "./pinned-targets";

export type LayoutGroupIds = {
  centerGroupId: string;
  rightTopGroupId: string;
  rightBottomGroupId: string;
  sidebarGroupId: string;
};

// eslint-disable-next-line @typescript-eslint/no-explicit-any
export function getRootSplitview(api: DockviewApi): any | null {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const sv = (api as any).component?.gridview?.root?.splitview;
  return sv?.resizeView && sv?.getViewSize ? sv : null;
}

/** Find a group by well-known ID, falling back to panel-based lookup. */
function findGroupId(api: DockviewApi, knownId: string, fallbackPanelId: string): string {
  if (api.groups.some((g) => g.id === knownId)) return knownId;
  const pnl = api.getPanel(fallbackPanelId);
  return pnl?.group?.id ?? knownId;
}

/** Find the center group, preferring the well-known ID, then "chat", then any
 *  "session:*" panel's group. When a session is active, "chat" is removed and
 *  replaced with per-session tabs — without the session fallback the returned
 *  ID would be a stale constant that doesn't match any live group. */
function findCenterGroupId(api: DockviewApi): string {
  if (api.groups.some((g) => g.id === CENTER_GROUP)) return CENTER_GROUP;
  const chat = api.getPanel("chat");
  if (chat?.group?.id) return chat.group.id;
  const sessionPanel = api.panels.find((p) => p.id.startsWith("session:"));
  if (sessionPanel?.group?.id) return sessionPanel.group.id;
  return CENTER_GROUP;
}

export function resolveGroupIds(api: DockviewApi): LayoutGroupIds {
  return {
    sidebarGroupId: findGroupId(api, SIDEBAR_GROUP, "sidebar"),
    centerGroupId: findCenterGroupId(api),
    // Always use the well-known constant — do NOT fall back to the "changes"
    // panel's current group. In plan mode the "changes" panel moves into the
    // center group; a panel-based fallback would return the center group ID and
    // defeat the auto-focus guard in changes-tab.tsx.
    rightTopGroupId: RIGHT_TOP_GROUP,
    rightBottomGroupId: findGroupId(api, RIGHT_BOTTOM_GROUP, TERMINAL_DEFAULT_ID),
  };
}

/**
 * Apply a LayoutState to DockviewApi via fromJSON.
 * Computes sizes, serializes, applies, and returns group IDs.
 *
 * `totalWidth` / `totalHeight` default to `api.width` / `api.height`, but
 * callers should pass measured container dimensions when available — relying
 * on `api.width` causes a proportional rescale on the next `api.layout` call
 * (the pinned-column max widths no longer enforce the legacy hard caps, so
 * the rescale grows sidebar/right past their intended defaults).
 */
export function applyLayout(
  api: DockviewApi,
  state: LayoutState,
  pinnedWidths: Map<string, number>,
  totalWidth?: number,
  totalHeight?: number,
): LayoutGroupIds {
  const w = totalWidth ?? api.width;
  const h = totalHeight ?? api.height;
  const serialized = toSerializedDockview(state, w, h, pinnedWidths);

  api.fromJSON(serialized);

  // Apply loose constraints (runtime cap) so the user can drag freely; the
  // actual pinning happens via `setPinnedTarget` + `enforcePinnedTargets`
  // (wired in `setupSashDragCapToggle`) — after every layout-change event
  // we force the live column back to its target width via `sv.resizeView`.
  // This avoids the "lock to current" ratchet bug where transient container
  // shrinks would permanently pin the sidebar at the smaller size.
  const sv = getRootSplitview(api);
  configureSidebarPinned(api, state, sv, w);
  configureRightPinned(api, state, sv, w);

  return resolveGroupIds(api);
}

function configureSidebarPinned(
  api: DockviewApi,
  state: LayoutState,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  sv: any,
  viewportWidth: number,
): void {
  const sidebarCol = state.columns.find((c) => c.id === "sidebar");
  const sb = api.getPanel("sidebar");
  if (!sb) return;
  sb.group.locked = SIDEBAR_LOCK;
  sb.group.header.hidden = false;
  // Use the measured dockview width (passed via `applyLayout`'s `totalWidth`)
  // rather than letting `computePinnedMaxPxFor` fall back to `window.innerWidth`.
  // The window-derived path is racy during a layout toggle: a stale
  // `innerWidth` of ~601 produces cap=301 (= 601 - VIEWPORT_RESERVE_PX), and
  // setConstraints({max:301}) then squeezes a 430px sidebar down to 301 —
  // reproducible as the rare `pane-resize-sidebar.spec.ts:41` failure in CI.
  sb.group.api.setConstraints({
    maximumWidth: sidebarCol?.maxWidth ?? computePinnedMaxPxFor("sidebar", viewportWidth),
    minimumWidth: LAYOUT_PINNED_MIN_PX,
  });
  if (!sidebarCol) return;
  const live = sv?.getViewSize?.(0);
  if (typeof live === "number" && live > 0) setPinnedTarget("sidebar", live);
}

function configureRightPinned(
  api: DockviewApi,
  state: LayoutState,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  sv: any,
  viewportWidth: number,
): void {
  for (let i = 0; i < state.columns.length; i++) {
    const col = state.columns[i];
    if (col.id === "sidebar" || !col.pinned) continue;
    // Same rationale as `configureSidebarPinned`: derive the cap from the
    // measured dockview width, not `window.innerWidth`.
    const cap = col.maxWidth ?? computePinnedMaxPxFor(col.id, viewportWidth);
    applyConstraintsToAllPanelGroups(api, col, cap);
    if (col.id !== "right") continue;
    const live = sv?.getViewSize?.(i);
    if (typeof live === "number" && live > 0) setPinnedTarget("right", live);
  }
}

/** Constrain every dockview group in the column. The default right column
 *  has separate top (files+changes) and bottom (terminal) groups — applying
 *  the cap to only the first group would leave the bottom unbounded and let
 *  the column grow on rebalance via the bottom group. */
function applyConstraintsToAllPanelGroups(
  api: DockviewApi,
  col: LayoutState["columns"][number],
  cap: number,
): void {
  const seen = new Set<string>();
  for (const group of col.groups) {
    for (const p of group.panels) {
      const pnl = api.getPanel(p.id);
      if (!pnl) continue;
      if (seen.has(pnl.group.id)) break;
      seen.add(pnl.group.id);
      pnl.group.api.setConstraints({
        maximumWidth: cap,
        minimumWidth: LAYOUT_PINNED_MIN_PX,
      });
      break;
    }
  }
}
