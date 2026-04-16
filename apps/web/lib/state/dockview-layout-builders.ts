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

/**
 * Resolve a fallback group position when the intended reference is stale.
 *
 * Tries to land in the center column, in this order:
 *   1. Well-known CENTER_GROUP id.
 *   2. Group containing the `chat` panel (post-drag, the well-known id may be
 *      gone but the chat panel still marks the center column).
 *   3. Group containing any `session:*` panel (active session: chat is removed
 *      and replaced with per-session tabs).
 *   4. Any group that is NOT the sidebar AND NOT a right-column group
 *      (Changes/Files/Terminal). Returning a right-column group would leak the
 *      panel into the narrow tools column — same UX bug as the sidebar leak.
 *
 * Returns undefined if no center-like group exists. The caller drops the
 * position so dockview picks a default. Never returns the sidebar.
 */
export function fallbackGroupPosition(api: DockviewApi): { referenceGroup: string } | undefined {
  const centerGroup = api.groups.find((g) => g.id === CENTER_GROUP);
  if (centerGroup) return { referenceGroup: centerGroup.id };

  const chatGroupId = api.getPanel("chat")?.group?.id;
  if (chatGroupId) return { referenceGroup: chatGroupId };

  const sessionGroupId = api.panels.find((p) => p.id.startsWith("session:"))?.group?.id;
  if (sessionGroupId) return { referenceGroup: sessionGroupId };

  const centerish = api.groups.find(
    (g) => g.id !== SIDEBAR_GROUP && g.id !== RIGHT_TOP_GROUP && g.id !== RIGHT_BOTTOM_GROUP,
  );
  if (centerish) return { referenceGroup: centerish.id };

  return undefined;
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
  // Guard: if the referenced group or panel no longer exists (stale ID after
  // layout transition), fall back to a known group. Avoid the active panel's
  // group because the user may have just clicked in the sidebar.
  const pos = options.position;
  if (pos && "referenceGroup" in pos) {
    const groupExists = api.groups.some((g) => g.id === pos.referenceGroup);
    if (!groupExists) {
      const fallback = fallbackGroupPosition(api);
      options = fallback
        ? { ...options, position: { ...pos, ...fallback } }
        : (Object.fromEntries(
            Object.entries(options).filter(([k]) => k !== "position"),
          ) as typeof options);
    }
  }

  if (pos && "referencePanel" in pos) {
    const refPanel = api.getPanel(pos.referencePanel as string);
    if (!refPanel) {
      const fallback = fallbackGroupPosition(api);
      options = fallback
        ? { ...options, position: fallback }
        : (Object.fromEntries(
            Object.entries(options).filter(([k]) => k !== "position"),
          ) as typeof options);
    }
  }

  const prev = quiet ? api.activePanel : null;
  api.addPanel(options);
  if (prev) prev.api.setActive();
}
