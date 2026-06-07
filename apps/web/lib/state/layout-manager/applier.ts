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
import { getPinnedWidth } from "./sizing";
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

function isCenterCandidateGroupId(groupId: string): boolean {
  return groupId !== SIDEBAR_GROUP && groupId !== RIGHT_TOP_GROUP && groupId !== RIGHT_BOTTOM_GROUP;
}

/** Find the center group, preferring the well-known ID, then "chat", then any
 *  "session:*" panel's group. When a session is active, "chat" is removed and
 *  replaced with per-session tabs — without the session fallback the returned
 *  ID would be a stale constant that doesn't match any live group. */
function findCenterGroupId(api: DockviewApi): string {
  if (api.groups.some((g) => g.id === CENTER_GROUP)) return CENTER_GROUP;
  const chat = api.getPanel("chat");
  if (chat?.group?.id && isCenterCandidateGroupId(chat.group.id)) return chat.group.id;
  const sessionPanel = api.panels.find((p) => p.id.startsWith("session:"));
  if (sessionPanel?.group?.id && isCenterCandidateGroupId(sessionPanel.group.id)) {
    return sessionPanel.group.id;
  }
  const centerish = api.groups.find((g) => isCenterCandidateGroupId(g.id));
  if (centerish) return centerish.id;
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
  configureSidebarPinned(api, state, sv, w, pinnedWidths);
  configureRightPinned(api, state, sv, w, pinnedWidths);

  return resolveGroupIds(api);
}

/** Set the pinned target for a column.
 *
 *  Resolve the intended width as `override ?? defaultWidth` — an explicit
 *  user-resized width if we have one (from `pinnedWidths`), otherwise the
 *  column's computed default. We resize dockview to that width and pin it as
 *  the target. We deliberately do NOT fall back to reading `sv.getViewSize`:
 *  fromJSON can produce a transient clamped/rebalanced size during a layout
 *  switch (the panel's NEW group mounts with default constraints, or the grid
 *  is briefly laid out at a narrower width before our setConstraints/api.layout
 *  settle), and capturing that would pin the column too narrow and then persist
 *  it — e.g. toggling plan mode off left the right column stuck at the
 *  transient width instead of its default. `getViewSize` is only a last resort
 *  when neither an override nor a default is available.
 */
function syncPinnedColumnTarget(
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  sv: any,
  idx: number,
  columnId: "sidebar" | "right",
  override: number | undefined,
  defaultWidth: number | undefined,
): void {
  const target = override !== undefined && override > 0 ? override : defaultWidth;
  if (target !== undefined && target > 0) {
    const live = sv?.getViewSize?.(idx);
    if (typeof live === "number" && live > 0 && Math.abs(live - target) > 1) {
      try {
        sv.resizeView(idx, target);
      } catch {
        /* dockview rejects unreachable sizes — ignore */
      }
    }
    setPinnedTarget(columnId, target);
    return;
  }
  const live = sv?.getViewSize?.(idx);
  if (typeof live === "number" && live > 0) setPinnedTarget(columnId, live);
}

function configureSidebarPinned(
  api: DockviewApi,
  state: LayoutState,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  sv: any,
  viewportWidth: number,
  pinnedWidths: Map<string, number>,
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
  const cap = sidebarCol?.maxWidth ?? computePinnedMaxPxFor("sidebar", viewportWidth);
  sb.group.api.setConstraints({
    maximumWidth: cap,
    minimumWidth: LAYOUT_PINNED_MIN_PX,
  });
  if (!sidebarCol) return;
  const defaultWidth = getPinnedWidth(sidebarCol, viewportWidth, undefined);
  syncPinnedColumnTarget(sv, 0, "sidebar", pinnedWidths.get("sidebar"), defaultWidth);
}

function configureRightPinned(
  api: DockviewApi,
  state: LayoutState,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  sv: any,
  viewportWidth: number,
  pinnedWidths: Map<string, number>,
): void {
  for (let i = 0; i < state.columns.length; i++) {
    const col = state.columns[i];
    if (col.id === "sidebar" || !col.pinned) continue;
    // Same rationale as `configureSidebarPinned`: derive the cap from the
    // measured dockview width, not `window.innerWidth`.
    const cap = col.maxWidth ?? computePinnedMaxPxFor(col.id, viewportWidth);
    applyConstraintsToAllPanelGroups(api, col, cap);
    if (col.id !== "right") continue;
    const defaultWidth = getPinnedWidth(col, viewportWidth, undefined);
    syncPinnedColumnTarget(sv, i, "right", pinnedWidths.get("right"), defaultWidth);
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
