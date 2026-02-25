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
  LAYOUT_SIDEBAR_MAX_PX,
} from "./constants";

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

export function resolveGroupIds(api: DockviewApi): LayoutGroupIds {
  return {
    sidebarGroupId: findGroupId(api, SIDEBAR_GROUP, "sidebar"),
    centerGroupId: findGroupId(api, CENTER_GROUP, "chat"),
    rightTopGroupId: findGroupId(api, RIGHT_TOP_GROUP, "changes"),
    rightBottomGroupId: findGroupId(api, RIGHT_BOTTOM_GROUP, TERMINAL_DEFAULT_ID),
  };
}

/**
 * Apply a LayoutState to DockviewApi via fromJSON.
 * Computes sizes, serializes, applies, and returns group IDs.
 */
export function applyLayout(
  api: DockviewApi,
  state: LayoutState,
  pinnedWidths: Map<string, number>,
): LayoutGroupIds {
  const serialized = toSerializedDockview(state, api.width, api.height, pinnedWidths);

  api.fromJSON(serialized);

  // Lock sidebar group and enforce max-width constraint.
  // Constraints are not serialized with layouts, so we must reapply after fromJSON.
  const sb = api.getPanel("sidebar");
  if (sb) {
    sb.group.locked = SIDEBAR_LOCK;
    sb.group.header.hidden = false;
    sb.group.api.setConstraints({ maximumWidth: LAYOUT_SIDEBAR_MAX_PX });
  }

  // Enforce max-width on other pinned columns (e.g. right panel group).
  for (const col of state.columns) {
    if (col.id === "sidebar" || !col.pinned || !col.maxWidth) continue;
    for (const group of col.groups) {
      for (const p of group.panels) {
        const pnl = api.getPanel(p.id);
        if (pnl) {
          pnl.group.api.setConstraints({ maximumWidth: col.maxWidth });
          break;
        }
      }
    }
  }

  return resolveGroupIds(api);
}
