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

function adoptRestoredSessionTabSelection(
  api: DockviewReadyEvent["api"],
  appStore: StoreApi<AppState>,
): void {
  const state = appStore.getState();
  const activeTaskId = state.tasks.activeTaskId;
  const currentEnvironmentId = useDockviewStore.getState().currentLayoutEnvId;
  if (!activeTaskId || !currentEnvironmentId) return;

  const currentSessionIds = new Set<string>(
    (state.taskSessionsByTask.itemsByTaskId[activeTaskId] ?? []).map((session) => session.id),
  );
  const selectedSessionIds = api.groups.flatMap((group) => {
    const panelId = group.activePanel?.id;
    if (!panelId?.startsWith("session:")) return [];
    const sessionId = panelId.slice("session:".length);
    if (state.taskSessions.items[sessionId]?.task_id !== activeTaskId) return [];
    if (!currentSessionIds.has(sessionId)) return [];
    if (state.environmentIdBySessionId[sessionId] !== currentEnvironmentId) return [];
    return [sessionId];
  });
  if (selectedSessionIds.length !== 1) return;

  const sessionId = selectedSessionIds[0];
  if (sessionId === state.tasks.activeSessionId) return;
  state.setActiveSessionAuto(activeTaskId, sessionId);
}

/**
 * When the activated panel is not a session panel and the previous active
 * session panel is no longer visible (closed by the user), find any remaining
 * visible session panel and activate it. This prevents runAutoSessionTabEffect
 * from recreating the closed panel. Returns true when handled.
 */
function handleNonSessionSuccessor(
  api: DockviewReadyEvent["api"],
  appStore: StoreApi<AppState>,
  panel: { id: string },
): boolean {
  const state = appStore.getState();
  const isActiveSessionPanelGone =
    state.tasks.activeSessionId && !api.getPanel(`session:${state.tasks.activeSessionId}`);
  if (!panel.id.startsWith("session:") && isActiveSessionPanelGone) {
    const remainingSessionPanel = api.panels.find((p) => p.id.startsWith("session:"));
    if (remainingSessionPanel) {
      const target = resolveSessionTabSyncTarget({
        panelId: remainingSessionPanel.id,
        activeTaskId: state.tasks.activeTaskId,
        activeSessionId: state.tasks.activeSessionId,
        taskSessionsById: state.taskSessions.items,
        environmentIdBySessionId: state.environmentIdBySessionId,
      });
      if (!target) return false;
      if (isDebug()) {
        debug("setupSessionTabSync: activating remaining session panel", {
          remainingPanelId: remainingSessionPanel.id,
          closedActiveSessionId: state.tasks.activeSessionId,
        });
      }
      state.setActiveSession(target.taskId, target.sessionId);
      return true;
    }
  }
  return false;
}

/**
 * Sync `activeSessionId` in the store when the user explicitly activates a
 * session tab. Dockview can also activate panels internally while restoring
 * layout or reconciling tabs; those activations must not pin a different
 * session or they create an app-level feedback loop.
 */
export function setupSessionTabSync(api: DockviewReadyEvent["api"], appStore: StoreApi<AppState>) {
  adoptRestoredSessionTabSelection(api, appStore);
  const unsubscribeLayoutRestore = useDockviewStore.subscribe((state, previousState) => {
    if (previousState.isRestoringLayout && !state.isRestoringLayout) {
      adoptRestoredSessionTabSelection(api, appStore);
    }
  });
  const activePanelDisposable = api.onDidActivePanelChange((panel) => {
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
      if (handleNonSessionSuccessor(api, appStore, panel)) return;
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
    const activeSessionPanelIsGone = !api.getPanel(`session:${state.tasks.activeSessionId}`);
    const hasUserActivationIntent = consumeSessionTabUserActivationIntent(target.sessionId);
    if (!activeSessionPanelIsGone && !hasUserActivationIntent) {
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
  return {
    dispose: () => {
      unsubscribeLayoutRestore();
      activePanelDisposable.dispose();
    },
  };
}
