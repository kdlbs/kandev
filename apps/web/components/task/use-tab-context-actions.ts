"use client";

import { useCallback } from "react";
import type { IDockviewPanelHeaderProps } from "dockview-react";

/**
 * The close actions every dockview tab context menu offers.
 *
 * `Close` matters beyond convenience: a group narrow enough to push a tab's
 * close button toward the edge of the strip leaves the context menu as the
 * reachable way to close it.
 *
 * `Close Others` deliberately spares the chat and session panels — they are
 * the task's permanent tabs and closing them is a session action, not a panel
 * one.
 */
export function useTabContextActions(
  api: IDockviewPanelHeaderProps["api"],
  containerApi: IDockviewPanelHeaderProps["containerApi"],
) {
  const handleClose = useCallback(() => {
    const panel = containerApi.getPanel(api.id);
    if (panel) containerApi.removePanel(panel);
  }, [api, containerApi]);

  const handleCloseOthers = useCallback(() => {
    const toClose = api.group.panels.filter(
      (p) => p.id !== api.id && p.id !== "chat" && !p.id.startsWith("session:"),
    );
    for (const panel of toClose) containerApi.removePanel(panel);
  }, [api, containerApi]);

  return { handleClose, handleCloseOthers };
}
