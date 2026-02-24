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

export function resolveGroupIds(api: DockviewApi): LayoutGroupIds {
  const sidebar = api.getPanel("sidebar");
  const changes = api.getPanel("changes") ?? api.getPanel("diff-files");
  const term = api.panels.find((p) => p.id.startsWith("terminal-") || p.id === TERMINAL_DEFAULT_ID);

  return {
    sidebarGroupId: sidebar?.group?.id ?? SIDEBAR_GROUP,
    centerGroupId: api.getPanel("chat")?.group?.id ?? CENTER_GROUP,
    rightTopGroupId: changes?.group?.id ?? RIGHT_TOP_GROUP,
    rightBottomGroupId: term?.group?.id ?? RIGHT_BOTTOM_GROUP,
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

  return resolveGroupIds(api);
}
