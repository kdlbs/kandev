import { useEffect, useRef, type MutableRefObject } from "react";
import type { DockviewApi, DockviewReadyEvent, AddPanelOptions } from "dockview-react";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { focusOrAddPanel } from "@/lib/state/dockview-layout-builders";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { wasPRPanelOffered, markPRPanelOffered } from "@/lib/local-storage";
import { sessionId as toSessionId } from "@/lib/types/ids";
import { createDebugLogger, IS_DEBUG } from "@/lib/debug/log";

const debug = createDebugLogger("dockview:session-tabs");

/**
 * Sync `activeSessionId` in the store when the user clicks a session tab.
 * Layouts are env-keyed, so switching between sessions of the same task is
 * a no-op at the layout level — the env switch action short-circuits when
 * old==new env. No manual skip flag needed.
 */
export function setupSessionTabSync(api: DockviewReadyEvent["api"], appStore: StoreApi<AppState>) {
  return api.onDidActivePanelChange((panel) => {
    if (!panel) return;
    const isRestoring = useDockviewStore.getState().isRestoringLayout;
    if (IS_DEBUG) {
      debug("setupSessionTabSync: onDidActivePanelChange", {
        panelId: panel.id,
        isRestoring,
        currentActiveSessionId: appStore.getState().tasks.activeSessionId,
        currentActiveTaskId: appStore.getState().tasks.activeTaskId,
        livePanelIds: api.panels.map((p) => p.id),
      });
    }
    if (isRestoring) return;
    if (!panel.id.startsWith("session:")) return;
    const sid = panel.id.slice("session:".length);
    if (sid && sid !== appStore.getState().tasks.activeSessionId) {
      const taskId = appStore.getState().tasks.activeTaskId;
      if (taskId) {
        if (IS_DEBUG) {
          debug("setupSessionTabSync: setActiveSession", { taskId, newSessionId: sid });
        }
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
    if (IS_DEBUG) {
      debug("setupChatPanelSafetyNet: chat panel removed", {
        removedPanelId: panel.id,
        livePanelIds: api.panels.map((p) => p.id),
      });
    }
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
        if (!activeSessionId) {
          debug("setupChatPanelSafetyNet: skip recreate (no active session)");
          return;
        }
        // Don't recreate a panel for a session that no longer exists in the
        // store — this guards against handleDelete racing with the safety net.
        const activeTaskId = appStore.getState().tasks.activeTaskId;
        const knownSessions = activeTaskId
          ? (appStore.getState().taskSessionsByTask.itemsByTaskId[activeTaskId] ?? [])
          : [];
        if (!knownSessions.some((s) => s.id === activeSessionId)) {
          if (IS_DEBUG) {
            debug("setupChatPanelSafetyNet: skip recreate (session not in store)", {
              activeSessionId,
              activeTaskId,
              knownSessionIds: knownSessions.map((s) => s.id),
            });
          }
          return;
        }
        if (IS_DEBUG) {
          debug("setupChatPanelSafetyNet: recreating session panel", {
            activeSessionId,
            activeTaskId,
            anchor: sb ? "rightOfSidebar" : "auto",
          });
        }
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

/**
 * Close session panels that no longer belong to the active task.
 *
 * Iterates `api.panels` (not `createdSet`) as the source of truth — `createdSet`
 * is unreliable because session panels can enter dockview via `tryRestoreLayout`
 * /`fromJSON` (never going through `ensureSessionPanel`) and can leave via
 * external removals like the right-click delete handler. Trusting it caused
 * tabs from a previous task to leak into the current task's view.
 */
export function reconcileRemovedSessionPanels(
  api: DockviewApi,
  createdSet: Set<string>,
  currentSessionIds: string[],
  keepSessionId: string,
): void {
  const currentIds = new Set(currentSessionIds);
  const removed: string[] = [];
  // Snapshot before iterating: closing a panel can mutate `api.panels`
  // synchronously, which would skip elements in a `for...of` over the live
  // array. Matches the pattern in `removeEphemeralPanels`.
  for (const panel of [...api.panels]) {
    if (!panel.id.startsWith("session:")) continue;
    const sid = panel.id.slice("session:".length);
    if (sid === keepSessionId) continue;
    if (currentIds.has(sid)) continue;
    try {
      panel.api.close();
      removed.push(panel.id);
    } catch {
      /* already gone */
    }
    createdSet.delete(sid);
  }
  if (IS_DEBUG) {
    const sessionPanels = api.panels.filter((p) => p.id.startsWith("session:"));
    debug("reconcileRemovedSessionPanels", {
      keepSessionId,
      currentSessionIds,
      liveSessionPanelIds: sessionPanels.map((p) => p.id),
      removed,
      createdSetAfter: Array.from(createdSet),
    });
  }
  // Drop any remaining stale entries (panel already removed externally, e.g.
  // by the right-click delete handler) so the ref stays in sync with reality.
  for (const sid of [...createdSet]) {
    if (sid === keepSessionId) continue;
    if (currentIds.has(sid)) continue;
    createdSet.delete(sid);
  }
}

const EMPTY_SESSION_IDS_KEY = "";

/**
 * Drop the generic "chat" placeholder once a real session is active.
 * Skips removal in maximized state to preserve the saved maximize layout.
 * Returns true when callers should continue ensuring session panels;
 * false when the maximized state intentionally suppresses them.
 */
function prepareLayoutForSessionPanels(api: DockviewApi): boolean {
  const preMaximizeLayout = useDockviewStore.getState().preMaximizeLayout;
  const chatPanel = api.getPanel("chat");
  if (chatPanel && !preMaximizeLayout) {
    api.removePanel(chatPanel);
  }
  // In maximized state, session panels are intentionally absent from the
  // layout — they'll be restored when the user exits maximize.
  return preMaximizeLayout === null;
}

/**
 * Decide whether to force-activate the session panel after it (and any
 * sibling tabs) have been ensured.
 *
 * - It was just created by `ensureSessionPanel` (no prior dockview state
 *   to honor), or
 * - The hook is mounting for the first time (initial page load) — preserve
 *   the long-standing behavior of focusing the agent tab so the chat is
 *   visible immediately, even when a saved layout had a different center
 *   tab active, or
 * - The user switched sessions within the same task (intra-task switch
 *   where dockview hasn't re-activated the new session for us).
 *
 * After an env (task) switch the prev refs are populated and the task
 * changed — `restoreSavedActiveViews` has already applied the saved active
 * panel for the incoming task, so calling setActive here would override it
 * and force the agent tab on top of whatever the user had focused.
 */
function shouldActivateSessionPanel(args: {
  sessionPanelExistedBefore: boolean;
  prevTaskId: string | null;
  prevSessionId: string | null;
  currentTaskId: string | null;
  currentSessionId: string;
}): boolean {
  const { sessionPanelExistedBefore, prevTaskId, prevSessionId, currentTaskId, currentSessionId } =
    args;
  if (!sessionPanelExistedBefore) return true;
  const isFirstMount = prevTaskId === null && prevSessionId === null;
  if (isFirstMount) return true;
  const taskChanged = prevTaskId !== currentTaskId;
  const sessionChanged = prevSessionId !== currentSessionId;
  return sessionChanged && !taskChanged;
}

type AutoSessionTabRefs = {
  sessionTabCreatedRef: MutableRefObject<Set<string>>;
  prevTaskIdRef: MutableRefObject<string | null>;
  prevSessionIdRef: MutableRefObject<string | null>;
};

/**
 * Activate the newly-ensured session panel and update the center-group store
 * entry. Returns the resolved active panel for sibling anchoring.
 */
function activateSessionPanel(
  api: DockviewApi,
  effectiveSessionId: string,
  sessionPanelExistedBefore: boolean,
  refs: AutoSessionTabRefs,
  tid: string | null,
): ReturnType<DockviewApi["getPanel"]> {
  const activePanel = api.getPanel(`session:${effectiveSessionId}`);
  if (!activePanel) return activePanel;

  const shouldActivate = shouldActivateSessionPanel({
    sessionPanelExistedBefore,
    prevTaskId: refs.prevTaskIdRef.current,
    prevSessionId: refs.prevSessionIdRef.current,
    currentTaskId: tid,
    currentSessionId: effectiveSessionId,
  });
  if (IS_DEBUG) {
    debug("useAutoSessionTab: activation decision", {
      effectiveSessionId,
      shouldActivate,
      sessionPanelExistedBefore,
      prevTaskId: refs.prevTaskIdRef.current,
      prevSessionId: refs.prevSessionIdRef.current,
      currentTaskId: tid,
      activeGroupId: activePanel.group.id,
    });
  }
  if (shouldActivate) activePanel.api.setActive();
  useDockviewStore.setState({ centerGroupId: activePanel.group.id });
  return activePanel;
}

/**
 * Add sibling session panels (inactive) into the same group as the active
 * panel so they appear as tabs. Returns the list of newly-created sibling IDs
 * for debug logging.
 */
function ensureSiblingPanels(
  api: DockviewApi,
  currentSessionIds: string[],
  effectiveSessionId: string,
  siblingAnchor: AddPanelOptions["position"],
  createdSet: Set<string>,
): string[] {
  const created: string[] = [];
  for (const sid of currentSessionIds) {
    if (sid === effectiveSessionId) continue;
    if (!api.getPanel(`session:${sid}`)) created.push(sid);
    ensureSessionPanel(api, sid, siblingAnchor, true, createdSet);
  }
  return created;
}

/** Resolve the current session ID list from the store for the active task. */
function resolveCurrentSessionIds(appStore: ReturnType<typeof useAppStoreApi>): {
  tid: string | null;
  currentSessionIds: string[];
} {
  const tid = appStore.getState().tasks.activeTaskId;
  const currentSessions = tid
    ? (appStore.getState().taskSessionsByTask.itemsByTaskId[tid] ?? [])
    : [];
  return { tid: tid ?? null, currentSessionIds: currentSessions.map((s) => s.id) };
}

/**
 * Check early-exit conditions after reconciliation. Returns true when the
 * caller should bail out before ensuring panels.
 */
function shouldSkipPanelEnsure(
  api: DockviewApi,
  effectiveSessionId: string,
  currentSessionIds: string[],
  createdSet: Set<string>,
): boolean {
  if (!currentSessionIds.includes(toSessionId(effectiveSessionId))) {
    if (IS_DEBUG) {
      debug("useAutoSessionTab: skip (session not in store yet)", {
        effectiveSessionId,
        currentSessionIds,
      });
    }
    return true;
  }
  if (!prepareLayoutForSessionPanels(api)) {
    debug("useAutoSessionTab: skip body (maximized - panels suppressed)", { effectiveSessionId });
    createdSet.add(effectiveSessionId);
    return true;
  }
  return false;
}

/**
 * Core effect body for useAutoSessionTab — extracted to reduce complexity of
 * the hook itself.
 */
function runAutoSessionTabEffect(
  effectiveSessionId: string | null,
  appStore: ReturnType<typeof useAppStoreApi>,
  refs: AutoSessionTabRefs,
): void {
  const api = useDockviewStore.getState().api;
  if (!api) return;

  const { tid, currentSessionIds } = resolveCurrentSessionIds(appStore);

  if (IS_DEBUG) {
    debug("useAutoSessionTab: effect entry", {
      effectiveSessionId,
      activeTaskId: tid,
      prevTaskId: refs.prevTaskIdRef.current,
      prevSessionId: refs.prevSessionIdRef.current,
      currentSessionIds,
      livePanelIdsBefore: api.panels.map((p) => p.id),
      createdSet: Array.from(refs.sessionTabCreatedRef.current),
    });
  }

  reconcileRemovedSessionPanels(
    api,
    refs.sessionTabCreatedRef.current,
    currentSessionIds,
    effectiveSessionId ?? "",
  );

  if (!effectiveSessionId) {
    debug("useAutoSessionTab: no effectiveSessionId, returning");
    return;
  }

  if (
    shouldSkipPanelEnsure(
      api,
      effectiveSessionId,
      currentSessionIds,
      refs.sessionTabCreatedRef.current,
    )
  ) {
    return;
  }

  const initialPosition = resolveInitialPosition(api);
  const sessionPanelExistedBefore = !!api.getPanel(`session:${effectiveSessionId}`);

  if (IS_DEBUG) {
    debug("useAutoSessionTab: ensuring active session panel", {
      effectiveSessionId,
      sessionPanelExistedBefore,
      initialPosition: JSON.stringify(initialPosition),
    });
  }

  ensureSessionPanel(
    api,
    effectiveSessionId,
    initialPosition,
    false,
    refs.sessionTabCreatedRef.current,
  );

  const activePanel = activateSessionPanel(
    api,
    effectiveSessionId,
    sessionPanelExistedBefore,
    refs,
    tid,
  );

  const siblingAnchor: AddPanelOptions["position"] = activePanel
    ? { referenceGroup: activePanel.group.id }
    : initialPosition;

  const siblingsCreated = ensureSiblingPanels(
    api,
    currentSessionIds,
    effectiveSessionId,
    siblingAnchor,
    refs.sessionTabCreatedRef.current,
  );

  if (IS_DEBUG) {
    debug("useAutoSessionTab: effect exit", {
      effectiveSessionId,
      siblingsCreated,
      livePanelIdsAfter: api.panels.map((p) => p.id),
      activeGroupId: activePanel?.group.id ?? null,
      liveActivePanelId: api.activePanel?.id ?? null,
    });
  }

  refs.prevTaskIdRef.current = tid;
  refs.prevSessionIdRef.current = effectiveSessionId;
}

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
  const prevTaskIdRef = useRef<string | null>(null);
  const prevSessionIdRef = useRef<string | null>(null);
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
    runAutoSessionTabEffect(effectiveSessionId, appStore, {
      sessionTabCreatedRef,
      prevTaskIdRef,
      prevSessionIdRef,
    });
  }, [effectiveSessionId, sessionIdsKey, appStore]);
}
