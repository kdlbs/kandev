import type { DockviewApi } from "dockview-react";
import { PANEL_REGISTRY, type ReusablePanelId } from "@/lib/state/layout-manager";

export type LayoutEditorDirection = "left" | "right" | "above" | "below";
export type LayoutEditorSizeAxis = "width" | "height";

const MIN_GROUP_SIZE = 120;

function getGroup(api: DockviewApi, groupId: string) {
  return api.groups.find((group) => group.id === groupId);
}

function toDockviewPosition(direction: LayoutEditorDirection) {
  if (direction === "above") return "top";
  if (direction === "below") return "bottom";
  return direction;
}

export function addReusablePanel(
  api: DockviewApi,
  panelId: ReusablePanelId,
  groupId?: string,
): boolean {
  if (api.getPanel(panelId)) return false;
  const referenceGroup = groupId ? getGroup(api, groupId) : (api.activeGroup ?? api.groups[0]);
  api.addPanel({
    id: panelId,
    ...PANEL_REGISTRY[panelId],
    ...(referenceGroup ? { position: { referenceGroup } } : {}),
  });
  return true;
}

export function removeReusablePanel(api: DockviewApi, panelId: string): boolean {
  if (panelId === "chat") return false;
  const panel = api.getPanel(panelId);
  if (!panel) return false;
  api.removePanel(panel);
  return true;
}

export function reorderTab(api: DockviewApi, panelId: string, delta: -1 | 1): boolean {
  const panel = api.getPanel(panelId);
  if (!panel) return false;
  const currentIndex = panel.group.panels.findIndex((candidate) => candidate.id === panelId);
  const nextIndex = currentIndex + delta;
  if (currentIndex < 0 || nextIndex < 0 || nextIndex >= panel.group.panels.length) return false;
  panel.api.moveTo({
    group: panel.group,
    position: "center",
    index: nextIndex,
    skipSetActive: true,
  });
  return true;
}

export function movePanelToGroup(
  api: DockviewApi,
  panelId: string,
  targetGroupId: string,
): boolean {
  const panel = api.getPanel(panelId);
  const target = getGroup(api, targetGroupId);
  if (!panel || !target || panel.group.id === targetGroupId) return false;
  panel.api.moveTo({ group: target, position: "center" });
  return true;
}

export function splitPanel(
  api: DockviewApi,
  panelId: string,
  direction: LayoutEditorDirection,
): boolean {
  const panel = api.getPanel(panelId);
  if (!panel || panel.group.panels.length < 2) return false;
  panel.api.moveTo({ group: panel.group, position: toDockviewPosition(direction) });
  return true;
}

export function moveGroup(
  api: DockviewApi,
  sourceGroupId: string,
  targetGroupId: string,
  direction: LayoutEditorDirection,
): boolean {
  const source = getGroup(api, sourceGroupId);
  const target = getGroup(api, targetGroupId);
  if (!source || !target || source === target) return false;
  source.api.moveTo({ group: target, position: toDockviewPosition(direction) });
  return true;
}

export function mergeGroup(
  api: DockviewApi,
  sourceGroupId: string,
  targetGroupId: string,
): boolean {
  const source = getGroup(api, sourceGroupId);
  const target = getGroup(api, targetGroupId);
  if (!source || !target || source === target) return false;
  source.api.moveTo({ group: target, position: "center" });
  return true;
}

export function resizeGroup(
  api: DockviewApi,
  groupId: string,
  axis: LayoutEditorSizeAxis,
  delta: number,
): boolean {
  const group = getGroup(api, groupId);
  if (!group) return false;
  const size = Math.max(MIN_GROUP_SIZE, group.api[axis] + delta);
  group.api.setSize({ [axis]: size });
  return true;
}
