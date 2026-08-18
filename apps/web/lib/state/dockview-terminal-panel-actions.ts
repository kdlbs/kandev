import type { DockviewApi } from "dockview-react";
import { focusOrAddPanel } from "./dockview-layout-builders";
import { DEV_SERVER_PANEL_ID, TERMINAL_DEFAULT_ID } from "./layout-manager/constants";
import { panelTitle } from "./layout-manager/panel-title";

type TerminalStoreGet = () => {
  api: DockviewApi | null;
  rightBottomGroupId: string;
};

/** Terminal-group panels: user shells and the read-only dev-script output view. */
export function buildTerminalPanelActions(get: TerminalStoreGet) {
  return {
    addTerminalPanel: (
      terminalId?: string,
      groupId?: string,
      environmentId?: string,
      taskID?: string,
      title?: string,
    ) => {
      const { api, rightBottomGroupId } = get();
      if (!api) return;
      const id = terminalId ?? `terminal-${Date.now()}`;
      // Stamp env id + task id into the panel's params so cleanup
      // (dockview-layout-setup.onDidRemovePanel) can call destroyUserShell
      // with the correct task scope even after the user switches tasks.
      // task_id is what the backend uses to verify ownership now — without
      // it `requireOwnership` rejects with ErrTaskMismatch.
      focusOrAddPanel(api, {
        id,
        component: "terminal",
        // terminalTab is a custom dockview tab that adds the `#N` badge
        // when there's more than one ordinary terminal in the task and
        // exposes a context menu for rename / park / destroy.
        tabComponent: "terminalTab",
        title: title ?? panelTitle(TERMINAL_DEFAULT_ID),
        params: { terminalId: id, environmentId, taskID },
        position: { referenceGroup: groupId ?? rightBottomGroupId },
      });
    },
    /**
     * Read-only view of the dev script's output.
     *
     * It reuses the `terminal` component but deliberately carries no
     * `terminalId`: it owns no PTY, and the panel-close handler keys off that
     * param to park or destroy the user shell a tab represents.
     */
    addDevServerPanel: (groupId?: string) => {
      const { api, rightBottomGroupId } = get();
      if (!api) return;
      focusOrAddPanel(api, {
        id: DEV_SERVER_PANEL_ID,
        component: "terminal",
        title: panelTitle(DEV_SERVER_PANEL_ID),
        params: { type: DEV_SERVER_PANEL_ID },
        position: { referenceGroup: groupId ?? rightBottomGroupId },
      });
    },
  };
}
