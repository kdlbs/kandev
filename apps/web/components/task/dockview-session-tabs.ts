import { useEffect, useRef } from "react";
import type { DockviewApi, DockviewReadyEvent, AddPanelOptions } from "dockview-react";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { focusOrAddPanel } from "@/lib/state/dockview-layout-builders";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { wasPRPanelOffered, markPRPanelOffered } from "@/lib/local-storage";

/**
 * Sync `activeSessionId` in the store when the user clicks a session tab.
 * Layouts are env-keyed, so switching between sessions of the same task is
 * a no-op at the layout level — the env switch action short-circuits when
 * old==new env. No manual skip flag needed.
 */
export function setupSessionTabSync(api: DockviewReadyEvent["api"], appStore: StoreApi<AppState>) {
  return api.onDidActivePanelChange((panel) => {
    if (!panel) return;
    if (useDockviewStore.getState().isRestoringLayout) return;
    if (!panel.id.startsWith("session:")) return;
    const sid = panel.id.slice("session:".length);
    if (sid && sid !== appStore.getState().tasks.activeSessionId) {
      const taskId = appStore.getState().tasks.activeTaskId;
      if (taskId) {
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

// ---------------------------------------------------------------------------
// Auto-show PR detail panel
// ---------------------------------------------------------------------------

/** Pure decision function for whether the PR panel should be auto-added or removed. */
export function shouldAutoAddPRPanel(params: {
  hasPR: boolean;
  panelExists: boolean;
  isRestoringLayout: boolean;
  isMaximized: boolean;
  wasOffered: boolean;
}): "add" | "remove" | "none" {
  if (!params.hasPR && params.panelExists) return "remove";
  if (!params.hasPR) return "none";
  if (params.panelExists) return "none";
  if (params.isRestoringLayout) return "none";
  if (params.isMaximized) return "none";
  if (params.wasOffered) return "none";
  return "add";
}

/**
 * Resolve the group ID to anchor the PR detail panel to.
 *
 * Preference: the live session chat panel's group. It's the group the user is
 * actively looking at, and reading it directly avoids the stale-id window the
 * store's centerGroupId has across layout transitions (which caused the PR
 * panel to land in a split instead of as a tab next to the session).
 */
export function resolvePRPanelTargetGroup(
  api: DockviewApi,
  sessionId: string,
  centerGroupId: string,
): string {
  const sessionPanel = api.getPanel(`session:${sessionId}`);
  const resolved = sessionPanel?.group?.id ?? centerGroupId;
  return resolved;
}

/**
 * Auto-add the PR detail panel to the center group when the active task
 * has an associated pull request. The panel is added as a background tab
 * (the session/agent tab stays focused).
 *
 * Dismissal is persisted to sessionStorage: if the user closes the PR panel,
 * it won't be re-added for that session — even after a page refresh.
 */
export function useAutoPRPanel() {
  const taskId = useAppStore((s) => s.tasks.activeTaskId);
  const sessionId = useAppStore((s) => s.tasks.activeSessionId);
  const hasPR = useAppStore((s) => {
    const tid = s.tasks.activeTaskId;
    return tid ? (s.taskPRs.byTaskId[tid]?.length ?? 0) > 0 : false;
  });
  const hasApi = useDockviewStore((s) => !!s.api);

  useEffect(() => {
    if (!taskId || !hasApi || !sessionId) return;

    requestAnimationFrame(() => {
      requestAnimationFrame(() => {
        const api = useDockviewStore.getState().api;
        if (!api) return;

        const decisionParams = {
          hasPR,
          panelExists: !!api.getPanel("pr-detail"),
          isRestoringLayout: useDockviewStore.getState().isRestoringLayout,
          isMaximized: useDockviewStore.getState().preMaximizeLayout !== null,
          wasOffered: wasPRPanelOffered(sessionId),
        };
        const decision = shouldAutoAddPRPanel(decisionParams);
        if (decision === "remove") {
          api.getPanel("pr-detail")?.api.close();
          return;
        }

        if (decision === "add") {
          const targetGroupId = resolvePRPanelTargetGroup(
            api,
            sessionId,
            useDockviewStore.getState().centerGroupId,
          );
          focusOrAddPanel(api, {
            id: "pr-detail",
            component: "pr-detail",
            title: "Pull Request",
            position: { referenceGroup: targetGroupId },
            inactive: true,
          });
          markPRPanelOffered(sessionId);
          return;
        }

        // "none" — panel already present or conditions not met.
        // Mark as offered if the panel exists (e.g. restored from saved layout).
        if (hasPR && api.getPanel("pr-detail")) {
          markPRPanelOffered(sessionId);
        }
      });
    });
  }, [taskId, hasPR, hasApi, sessionId]);
}

function resolveInitialPosition(api: DockviewApi): AddPanelOptions["position"] {
  const { centerGroupId } = useDockviewStore.getState();
  const centerGroupExists = centerGroupId && api.groups.some((g) => g.id === centerGroupId);
  if (centerGroupExists) return { referenceGroup: centerGroupId };
  const sb = api.getPanel("sidebar");
  if (sb) return { direction: "right" as const, referencePanel: "sidebar" };
  return undefined;
}

function ensureSessionPanel(
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

/** Close panels we previously created for sessions no longer in the task's list. */
function reconcileRemovedSessionPanels(
  api: DockviewApi,
  createdSet: Set<string>,
  currentSessionIds: string[],
  keepSessionId: string,
): void {
  const currentIds = new Set(currentSessionIds);
  for (const createdId of [...createdSet]) {
    if (createdId === keepSessionId) continue;
    if (currentIds.has(createdId)) continue;
    const stalePanel = api.getPanel(`session:${createdId}`);
    if (stalePanel) {
      try {
        stalePanel.api.close();
      } catch {
        /* already gone */
      }
    }
    createdSet.delete(createdId);
  }
}

const EMPTY_SESSION_IDS_KEY = "";

/**
 * Open a dockview tab for every session of the active task and keep them in sync
 * with the store.
 *
 * - On mount / session-list change: create a panel for each session if one does
 *   not exist yet. Siblings are added adjacent to the active session's group so
 *   they show up as tabs in the center area.
 * - The panel for `effectiveSessionId` is the active tab; the rest are added
 *   inactive so switching the active session doesn't blow focus out of the
 *   already-open layout.
 * - Deleted sessions have their panels closed.
 */
export function useAutoSessionTab(effectiveSessionId: string | null) {
  const sessionTabCreatedRef = useRef<Set<string>>(new Set());
  const appStore = useAppStoreApi();

  // Key-based dependency so the effect re-runs when the task's session list
  // changes (add/remove). Inside the effect we re-read the real array from
  // the store so we don't capture a stale reference.
  const sessionIdsKey = useAppStore((s) => {
    const tid = s.tasks.activeTaskId;
    if (!tid) return EMPTY_SESSION_IDS_KEY;
    const list = s.taskSessionsByTask.itemsByTaskId[tid];
    if (!list || list.length === 0) return EMPTY_SESSION_IDS_KEY;
    return list.map((ss) => ss.id).join(",");
  });

  useEffect(() => {
    const api = useDockviewStore.getState().api;
    if (!api) return;

    // Re-read the current session list straight from the store so we iterate
    // the live array (sessionIdsKey change is what gets us here).
    const tid = appStore.getState().tasks.activeTaskId;
    const currentSessions = tid
      ? (appStore.getState().taskSessionsByTask.itemsByTaskId[tid] ?? [])
      : [];
    const currentSessionIds = currentSessions.map((s) => s.id);

    // Always reconcile removed panels — even when effectiveSessionId is null
    // (e.g. all sessions deleted) so orphaned tabs are closed.
    reconcileRemovedSessionPanels(
      api,
      sessionTabCreatedRef.current,
      currentSessionIds,
      effectiveSessionId ?? "",
    );

    if (!effectiveSessionId) return;

    // Remove the generic "chat" placeholder as soon as a real session is
    // active — per-session tabs replace it. Skip in maximized state to
    // preserve the saved maximize layout.
    const chatPanel = api.getPanel("chat");
    if (chatPanel && !useDockviewStore.getState().preMaximizeLayout) {
      api.removePanel(chatPanel);
    }

    // In maximized state, session panels are intentionally absent from the
    // layout — they'll be restored when the user exits maximize.
    if (useDockviewStore.getState().preMaximizeLayout !== null) {
      sessionTabCreatedRef.current.add(effectiveSessionId);
      return;
    }

    const initialPosition = resolveInitialPosition(api);

    // Active panel first so its group becomes the anchor for siblings.
    ensureSessionPanel(
      api,
      effectiveSessionId,
      initialPosition,
      false,
      sessionTabCreatedRef.current,
    );
    const activePanel = api.getPanel(`session:${effectiveSessionId}`);
    if (activePanel) {
      activePanel.api.setActive();
      useDockviewStore.setState({ centerGroupId: activePanel.group.id });
    }

    const siblingAnchor: AddPanelOptions["position"] = activePanel
      ? { referenceGroup: activePanel.group.id }
      : initialPosition;

    for (const sid of currentSessionIds) {
      if (sid === effectiveSessionId) continue;
      ensureSessionPanel(api, sid, siblingAnchor, true, sessionTabCreatedRef.current);
    }
  }, [effectiveSessionId, sessionIdsKey, appStore]);
}
