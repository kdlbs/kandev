import { useEffect, useRef } from "react";
import type { DockviewReadyEvent, AddPanelOptions } from "dockview-react";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { useDockviewStore } from "@/lib/state/dockview-store";

/**
 * Sync `activeSessionId` in the store when the user clicks a session tab.
 * This ensures global panels (changes, files, plan) switch context.
 */
export function setupSessionTabSync(
  api: DockviewReadyEvent["api"],
  appStore: StoreApi<AppState>,
) {
  api.onDidActivePanelChange((panel) => {
    if (!panel) return;
    // Parse sessionId from panel ID (format: "session:{sessionId}")
    if (!panel.id.startsWith("session:")) return;
    const sid = panel.id.slice("session:".length);
    if (sid && sid !== appStore.getState().tasks.activeSessionId) {
      const taskId = appStore.getState().tasks.activeTaskId;
      if (taskId) {
        // Pre-set currentLayoutSessionId so useSessionSwitchCleanup's
        // performLayoutSwitch guard prevents a full layout teardown/rebuild.
        // Clicking a tab just switches context — the layout stays intact.
        useDockviewStore.setState({ currentLayoutSessionId: sid });
        appStore.getState().setActiveSession(taskId, sid);
      }
    }
  });
}

/**
 * Re-create a chat or session panel if the last one is removed.
 * Prevents the user from ending up with no chat panel at all.
 */
export function setupChatPanelSafetyNet(
  api: DockviewReadyEvent["api"],
  appStore: StoreApi<AppState>,
) {
  api.onDidRemovePanel((panel) => {
    if (useDockviewStore.getState().isRestoringLayout) return;
    const isChatPanel = panel.id === "chat" || panel.id.startsWith("session:");
    if (!isChatPanel) return;
    requestAnimationFrame(() => {
      const hasChatPanel = api.panels.some(
        (p) => p.id === "chat" || p.id.startsWith("session:"),
      );
      if (hasChatPanel) return;
      const activeSessionId = appStore.getState().tasks.activeSessionId;
      const sb = api.getPanel("sidebar");
      const position = sb
        ? { direction: "right" as const, referencePanel: "sidebar" }
        : undefined;
      if (activeSessionId) {
        api.addPanel({
          id: `session:${activeSessionId}`,
          component: "chat",
          tabComponent: "sessionTab",
          title: "Agent",
          params: { sessionId: activeSessionId },
          position,
        });
        const nc = api.getPanel(`session:${activeSessionId}`);
        if (nc) useDockviewStore.setState({ centerGroupId: nc.group.id });
      } else {
        api.addPanel({
          id: "chat",
          component: "chat",
          tabComponent: "permanentTab",
          title: "Agent",
          position,
        });
        const nc = api.getPanel("chat");
        if (nc) useDockviewStore.setState({ centerGroupId: nc.group.id });
      }
    });
  });
}

/**
 * Auto-create a session tab when a session becomes active.
 * Replaces the generic "chat" panel with a per-session tab on first use.
 */
export function useAutoSessionTab(effectiveSessionId: string | null) {
  const sessionTabCreatedRef = useRef<Set<string>>(new Set());
  useEffect(() => {
    if (!effectiveSessionId) return;
    const api = useDockviewStore.getState().api;
    if (!api) return;
    if (api.getPanel(`session:${effectiveSessionId}`)) {
      sessionTabCreatedRef.current.add(effectiveSessionId);
      return;
    }
    // Always remove the generic "chat" panel — it's replaced by per-session tabs
    const chatPanel = api.getPanel("chat");
    if (chatPanel) {
      api.removePanel(chatPanel);
    }
    // Resolve position: prefer centerGroupId if the group still exists,
    // fall back to placing right of sidebar, or omit position entirely.
    const { centerGroupId } = useDockviewStore.getState();
    const centerGroupExists = centerGroupId && api.groups.some((g) => g.id === centerGroupId);
    let position: AddPanelOptions["position"];
    if (centerGroupExists) {
      position = { referenceGroup: centerGroupId };
    } else {
      const sb = api.getPanel("sidebar");
      if (sb) {
        position = { direction: "right" as const, referencePanel: "sidebar" };
      }
    }
    api.addPanel({
      id: `session:${effectiveSessionId}`,
      component: "chat",
      tabComponent: "sessionTab",
      title: "Agent",
      params: { sessionId: effectiveSessionId },
      position,
    });
    const panel = api.getPanel(`session:${effectiveSessionId}`);
    if (panel) {
      panel.api.setActive();
      useDockviewStore.setState({ centerGroupId: panel.group.id });
    }
    sessionTabCreatedRef.current.add(effectiveSessionId);
  }, [effectiveSessionId]);
}
