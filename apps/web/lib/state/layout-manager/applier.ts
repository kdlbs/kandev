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

  // Lock sidebar / right column widths at their just-computed defaults.
  // Without a tight max, dockview's proportional rebalance on the next
  // `api.layout` call grows pinned columns past the legacy initial cap.
  // The sash-drag handler widens the cap on mousedown so the user can
  // still drag past the default; on mouseup the cap snaps back to the
  // new current width. column.maxWidth (e.g. compact's 260) overrides
  // the lock when set so presets stay tight.
  const readWidth = makeWidthReader(api);
  lockSidebarPinnedCap(api, state, readWidth);
  lockRightPinnedCaps(api, state, readWidth);

  return resolveGroupIds(api);
}

type WidthReader = (index: number, columnId: string) => number;

function makeWidthReader(api: DockviewApi): WidthReader {
  const sv = getRootSplitview(api);
  return (index, columnId) => {
    const live = sv?.getViewSize?.(index);
    return typeof live === "number" ? live : computePinnedMaxPxFor(columnId);
  };
}

function lockSidebarPinnedCap(api: DockviewApi, state: LayoutState, readWidth: WidthReader): void {
  const sidebarCol = state.columns.find((c) => c.id === "sidebar");
  const sb = api.getPanel("sidebar");
  if (!sb) return;
  sb.group.locked = SIDEBAR_LOCK;
  sb.group.header.hidden = false;
  const currentW = readWidth(0, "sidebar");
  const cap = sidebarCol?.maxWidth ?? Math.max(currentW, LAYOUT_PINNED_MIN_PX);
  sb.group.api.setConstraints({ maximumWidth: cap, minimumWidth: LAYOUT_PINNED_MIN_PX });
}

function lockRightPinnedCaps(api: DockviewApi, state: LayoutState, readWidth: WidthReader): void {
  for (let i = 0; i < state.columns.length; i++) {
    const col = state.columns[i];
    if (col.id === "sidebar" || !col.pinned) continue;
    const currentW = readWidth(i, col.id);
    const cap = col.maxWidth ?? Math.max(currentW, LAYOUT_PINNED_MIN_PX);
    applyConstraintsToFirstPanelGroup(api, col, cap);
  }
}

function applyConstraintsToFirstPanelGroup(
  api: DockviewApi,
  col: LayoutState["columns"][number],
  cap: number,
): void {
  for (const group of col.groups) {
    for (const p of group.panels) {
      const pnl = api.getPanel(p.id);
      if (pnl) {
        pnl.group.api.setConstraints({
          maximumWidth: cap,
          minimumWidth: LAYOUT_PINNED_MIN_PX,
        });
        return;
      }
    }
  }
}
