import type { DockviewApi, AddPanelOptions } from "dockview-react";
import { SIDEBAR_LOCK, resolveGroupIds } from "./layout-manager";
import type { LayoutGroupIds } from "./layout-manager";

// Re-export for consumers that import from this module
export { getRootSplitview } from "./layout-manager";

/** After fromJSON() restores a session layout, apply fixups and return group IDs. */
export function applyLayoutFixups(api: DockviewApi): LayoutGroupIds {
  const sb = api.getPanel("sidebar");
  if (sb) {
    sb.group.locked = SIDEBAR_LOCK;
    sb.group.header.hidden = false;
  }
  const oldChanges = api.getPanel("diff-files");
  if (oldChanges) oldChanges.api.setTitle("Changes");
  const oldFiles = api.getPanel("all-files");
  if (oldFiles) oldFiles.api.setTitle("Files");

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
