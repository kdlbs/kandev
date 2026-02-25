import type { DockviewApi, AddPanelOptions } from "dockview-react";
import {
  SIDEBAR_LOCK,
  LAYOUT_SIDEBAR_MAX_PX,
  LAYOUT_RIGHT_MAX_PX,
  RIGHT_TOP_GROUP,
  RIGHT_BOTTOM_GROUP,
  resolveGroupIds,
} from "./layout-manager";
import type { LayoutGroupIds } from "./layout-manager";

// Re-export for consumers that import from this module
export { getRootSplitview } from "./layout-manager";

/** After fromJSON() restores a session layout, apply fixups and return group IDs. */
export function applyLayoutFixups(api: DockviewApi): LayoutGroupIds {
  const sb = api.getPanel("sidebar");
  if (sb) {
    sb.group.locked = SIDEBAR_LOCK;
    sb.group.header.hidden = false;
    sb.group.api.setConstraints({ maximumWidth: LAYOUT_SIDEBAR_MAX_PX });
  }
  const oldChanges = api.getPanel("diff-files");
  if (oldChanges) oldChanges.api.setTitle("Changes");
  const oldFiles = api.getPanel("all-files");
  if (oldFiles) oldFiles.api.setTitle("Files");

  // Constrain right column groups by their well-known IDs.
  // Groups created from presets carry stable IDs (e.g. "group-right-top"),
  // so this works regardless of which panels are in them.
  for (const gid of [RIGHT_TOP_GROUP, RIGHT_BOTTOM_GROUP]) {
    const group = api.groups.find((g) => g.id === gid);
    if (group) {
      group.api.setConstraints({ maximumWidth: LAYOUT_RIGHT_MAX_PX });
    }
  }

  return resolveGroupIds(api);
}

export function focusOrAddPanel(
  api: DockviewApi,
  options: AddPanelOptions & { id: string },
  quiet = false,
): void {
  const existing = api.getPanel(options.id);
  if (existing) {
    if (!quiet) existing.api.setActive();
    return;
  }
  const prev = quiet ? api.activePanel : null;
  api.addPanel(options);
  if (prev) prev.api.setActive();
}
