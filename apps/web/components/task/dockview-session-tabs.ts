import { useEffect, useRef } from "react";
import type { DockviewReadyEvent, AddPanelOptions } from "dockview-react";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { useDockviewStore } from "@/lib/state/dockview-store";

/**
 * Sync `activeSessionId` in the store when the user clicks a session tab.
 * This ensures global panels (changes, files, plan) switch context.
 */
export function setupSessionTabSync(api: DockviewReadyEvent["api"], appStore: StoreApi<AppState>) {
  return api.onDidActivePanelChange((panel) => {
    if (!panel) return;
    // Ignore panel activations during layout operations (e.g. drag-to-split,
    // layout restore) to avoid cascading layout switches.
    if (useDockviewStore.getState().isRestoringLayout) return;
    // Parse sessionId from panel ID (format: "session:{sessionId}")
    if (!panel.id.startsWith("session:")) return;
    const sid = panel.id.slice("session:".length);
    if (sid && sid !== appStore.getState().tasks.activeSessionId) {
      const taskId = appStore.getState().tasks.activeTaskId;
      if (taskId) {
        // Skip the next layout switch for this session — clicking a tab
        // just switches context, the layout stays intact.
        useDockviewStore.setState({ _skipLayoutSwitchForSession: sid });
        appStore.getState().setActiveSession(taskId, sid);
      }
    }
  });
}

/**
 * Re-create a chat or session panel if the last one is removed.
 * Prevents the user from ending up with no chat panel at all.
 *
 * Uses a delayed check to avoid racing with dockview drag-to-split
 * operations, which temporarily remove and re-add panels.
 */
export function setupChatPanelSafetyNet(
  api: DockviewReadyEvent["api"],
  appStore: StoreApi<AppState>,
) {
  return api.onDidRemovePanel((panel) => {
    if (useDockviewStore.getState().isRestoringLayout) return;
    const isChatPanel = panel.id === "chat" || panel.id.startsWith("session:");
    if (!isChatPanel) return;
    // Double rAF gives dockview time to finish internal operations like
    // drag-to-split moves (remove from old group → add to new group).
    requestAnimationFrame(() => {
      requestAnimationFrame(() => {
        if (useDockviewStore.getState().isRestoringLayout) return;
        const hasChatPanel = api.panels.some((p) => p.id === "chat" || p.id.startsWith("session:"));
        if (hasChatPanel) return;
        const activeSessionId = appStore.getState().tasks.activeSessionId;
        const sb = api.getPanel("sidebar");
        const position = sb
          ? { direction: "right" as const, referencePanel: "sidebar" }
          : undefined;
        // Only recreate a panel if there's still an active session.
        // If all sessions were deleted, leave the layout empty — the user
        // can create a new session via the "+" menu.
        if (!activeSessionId) return;
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
      });
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

    // Always remove the generic "chat" panel when a session is active —
    // it's replaced by per-session tabs. Must run before the early return
    // so restored layouts with both "chat" and session panels get cleaned up.
    // Skip removal in maximized state to avoid triggering the safety net
    // which could disrupt the saved maximize layout.
    const chatPanel = api.getPanel("chat");
    if (chatPanel && !useDockviewStore.getState().preMaximizeLayout) {
      api.removePanel(chatPanel);
    }
    if (api.getPanel(`session:${effectiveSessionId}`)) {
      sessionTabCreatedRef.current.add(effectiveSessionId);
      return;
    }
    // In maximized state the session panel is intentionally absent from the layout;
    // it will be restored when the user exits maximize (via preMaximizeLayout).
    // Adding it here would destroy the saved maximize layout.
    if (useDockviewStore.getState().preMaximizeLayout !== null) {
      sessionTabCreatedRef.current.add(effectiveSessionId);
      return;
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
