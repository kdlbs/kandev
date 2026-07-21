import type { DockviewReadyEvent } from "dockview-react";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { createDebugLogger, isDebug } from "@/lib/debug/log";
import { consumeSessionTabUserActivationIntent } from "./session-tab-activation-intent";
import { resolveSessionTabSyncTarget } from "./dockview-session-tabs";

const debug = createDebugLogger("dockview:session-tabs");

function restoreActiveSessionPanel(
  api: DockviewReadyEvent["api"],
  activeSessionId: string | null,
): void {
  if (!activeSessionId) return;
  api.getPanel(`session:${activeSessionId}`)?.api.setActive();
}

function isDifferentSessionPanel(panelId: string, activeSessionId: string | null): boolean {
  return panelId.startsWith("session:") && panelId !== `session:${activeSessionId}`;
}

/**
 * Sync `activeSessionId` in the store when the user explicitly activates a
 * session tab. Dockview can also activate panels internally while restoring
 * layout or reconciling tabs; those activations must not pin a different
 * session or they create an app-level feedback loop.
 */
export function setupSessionTabSync(api: DockviewReadyEvent["api"], appStore: StoreApi<AppState>) {
  return api.onDidActivePanelChange((panel) => {
    if (!panel) return;
    const isRestoring = useDockviewStore.getState().isRestoringLayout;
    if (isDebug()) {
      debug("setupSessionTabSync: onDidActivePanelChange", {
        panelId: panel.id,
        isRestoring,
        currentActiveSessionId: appStore.getState().tasks.activeSessionId,
        currentActiveTaskId: appStore.getState().tasks.activeTaskId,
        livePanelIds: api.panels.map((p) => p.id),
      });
    }
    if (isRestoring) return;
    const state = appStore.getState();
    const target = resolveSessionTabSyncTarget({
      panelId: panel.id,
      activeTaskId: state.tasks.activeTaskId,
      activeSessionId: state.tasks.activeSessionId,
      taskSessionsById: state.taskSessions.items,
      environmentIdBySessionId: state.environmentIdBySessionId,
    });
    if (!target) {
      const shouldRestoreActiveSession = isDifferentSessionPanel(
        panel.id,
        state.tasks.activeSessionId,
      );
      if (isDebug() && panel.id.startsWith("session:")) {
        debug("setupSessionTabSync: skip (stale or cross-task panel)", {
          panelId: panel.id,
          activeTaskId: state.tasks.activeTaskId,
        });
      }
      if (shouldRestoreActiveSession) restoreActiveSessionPanel(api, state.tasks.activeSessionId);
      return;
    }
    if (!consumeSessionTabUserActivationIntent(target.sessionId)) {
      if (isDebug()) {
        debug("setupSessionTabSync: skip (no user activation intent)", {
          panelId: panel.id,
          activeSessionId: state.tasks.activeSessionId,
        });
      }
      restoreActiveSessionPanel(api, state.tasks.activeSessionId);
      return;
    }
    if (isDebug()) {
      debug("setupSessionTabSync: setActiveSession", {
        taskId: target.taskId,
        newSessionId: target.sessionId,
      });
    }
    state.setActiveSession(target.taskId, target.sessionId);
  });
}
