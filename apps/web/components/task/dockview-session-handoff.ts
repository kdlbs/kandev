import type { AddPanelOptions, DockviewApi } from "dockview-react";

type StaleSessionPanel = {
  id: string;
  group: { id: string; panels: Array<{ id: string }> };
};

export function ensureSessionPanel(
  api: DockviewApi,
  sessionId: string,
  position: AddPanelOptions["position"],
  inactive: boolean,
  createdSet: Set<string>,
): void {
  if (api.getPanel(`session:${sessionId}`)) {
    createdSet.add(sessionId);
    return;
  }
  api.addPanel({
    id: `session:${sessionId}`,
    component: "chat",
    tabComponent: "sessionTab",
    title: "Agent",
    params: { sessionId },
    position,
    inactive,
  });
  createdSet.add(sessionId);
}

/**
 * Add the incoming task session before closing its stale predecessor. Dockview
 * destroys a group when its last panel closes, so the replacement must happen
 * at the predecessor's live group and tab index.
 */
export function anchorIncomingSessionPanel(
  api: DockviewApi,
  currentIds: Set<string>,
  stalePanels: StaleSessionPanel[],
  keepSessionId: string,
  createdSet: Set<string>,
): void {
  const firstStale = stalePanels[0];
  if (
    !currentIds.has(keepSessionId) ||
    api.getPanel(`session:${keepSessionId}`) ||
    !firstStale ||
    !api.groups.some((group) => group.id === firstStale.group.id)
  ) {
    return;
  }

  const index = firstStale.group.panels.findIndex((panel) => panel.id === firstStale.id);
  ensureSessionPanel(
    api,
    keepSessionId,
    { referenceGroup: firstStale.group.id, ...(index >= 0 ? { index } : {}) },
    true,
    createdSet,
  );
}
