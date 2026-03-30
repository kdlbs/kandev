import type { DockviewApi, AddPanelOptions } from "dockview-react";
import {
  SIDEBAR_LOCK,
  SIDEBAR_GROUP,
  CENTER_GROUP,
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
  // Guard: if the referenced group no longer exists (stale ID after layout
  // transition), try the well-known center group, then the first non-sidebar
  // group. Avoid falling back to the active panel's group because the user
  // may have just clicked in the sidebar, which would place the new panel there.
  const pos = options.position;
  if (pos && "referenceGroup" in pos) {
    const groupExists = api.groups.some((g) => g.id === pos.referenceGroup);
    if (!groupExists) {
      const centerGroup = api.groups.find((g) => g.id === CENTER_GROUP);
      const nonSidebarGroup = api.groups.find((g) => g.id !== SIDEBAR_GROUP);
      if (centerGroup) {
        options = { ...options, position: { ...pos, referenceGroup: centerGroup.id } };
      } else if (nonSidebarGroup) {
        options = { ...options, position: { ...pos, referenceGroup: nonSidebarGroup.id } };
      } else if (api.groups.length > 0) {
        options = { ...options, position: { ...pos, referenceGroup: api.groups[0].id } };
      } else {
        options = Object.fromEntries(
          Object.entries(options).filter(([k]) => k !== "position"),
        ) as typeof options;
      }
    }
  }

  const prev = quiet ? api.activePanel : null;
  api.addPanel(options);
  if (prev) prev.api.setActive();
}
