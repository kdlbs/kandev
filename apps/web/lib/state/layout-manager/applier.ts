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

  // Lock sidebar group and enforce max/min-width constraints.
  // Constraints are not serialized with layouts, so we must reapply after fromJSON.
  // column.maxWidth wins so presets like `compact` (maxWidth=260) keep their
  // tight cap; otherwise inherit the runtime sidebar cap.
  const sidebarCol = state.columns.find((c) => c.id === "sidebar");
  const sidebarCap = sidebarCol?.maxWidth ?? computePinnedMaxPxFor("sidebar");
  const sb = api.getPanel("sidebar");
  if (sb) {
    sb.group.locked = SIDEBAR_LOCK;
    sb.group.header.hidden = false;
    sb.group.api.setConstraints({
      maximumWidth: sidebarCap,
      minimumWidth: LAYOUT_PINNED_MIN_PX,
    });
  }

  // Enforce constraints on other pinned columns (e.g. right panel group).
  // Column.maxWidth (when set) wins over the runtime cap so presets like
  // compact can pin the sidebar tight; otherwise we inherit the viewport-
  // proportional cap so wide screens get a roomy right panel.
  for (const col of state.columns) {
    if (col.id === "sidebar" || !col.pinned) continue;
    const cap = col.maxWidth ?? computePinnedMaxPxFor(col.id);
    for (const group of col.groups) {
      for (const p of group.panels) {
        const pnl = api.getPanel(p.id);
        if (pnl) {
          pnl.group.api.setConstraints({
            maximumWidth: cap,
            minimumWidth: LAYOUT_PINNED_MIN_PX,
          });
          break;
        }
      }
    }
  }

  return resolveGroupIds(api);
}
