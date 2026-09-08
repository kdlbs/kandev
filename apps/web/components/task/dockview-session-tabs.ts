import { useEffect, useRef, type MutableRefObject } from "react";
import type { DockviewApi, DockviewReadyEvent, AddPanelOptions } from "dockview-react";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { releaseLayoutToDefault, useDockviewStore } from "@/lib/state/dockview-store";
import {
  CENTER_GROUP,
  isCenterCandidateGroupId,
  RIGHT_TOP_GROUP,
} from "@/lib/state/layout-manager";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { sessionId as toSessionId } from "@/lib/types/ids";
import { createDebugLogger, isDebug } from "@/lib/debug/log";
import type { TaskSession } from "@/lib/types/http";
import {
  activateChatReplacement,
  isChatPlaceholderSelected,
  restorePreservedActivePanel,
  shouldActivateSessionPanel,
  shouldPreserveActivePanel,
} from "./dockview-session-tab-activation";
import { anchorIncomingSessionPanel, ensureSessionPanel } from "./dockview-session-handoff";
import { t } from "@/lib/i18n";

const debug = createDebugLogger("dockview:session-tabs");

/**
 * Decide whether `onDidActivePanelChange` should write `setActiveSession`.
 *
 * The activated session must belong to the currently-active task. During a
 * task switch dockview can briefly fire activation for a stale `session:<sid>`
 * panel that still belongs to the previous task (panels are torn down async).
 * Writing it would poison `lastSessionByTaskId[newTaskId]` with a session from
 * a different task, which the next task-re-entry then prefers over the
 * primary session, restoring the wrong layout.
 *
 * Two ownership gates:
 * 1. Reject when the session is hydrated and known to belong to a different
 *    task (the primary leak path).
 * 2. Reject when the session has no `environmentIdBySessionId` entry. The
 *    session slice clears `taskSessions.items[sid]` and
 *    `environmentIdBySessionId[sid]` together on `removeTaskSession`, so a
 *    missing env mapping means the session has been deleted or never existed.
 *    Writing `activeSessionId = sid` in that window briefly points every
 *    activeSessionId-consumer (chat / file editor / shell / pr-detail / ...)
 *    at a dead session.
 */
export function resolveSessionTabSyncTarget(args: {
  panelId: string;
  activeTaskId: string | null;
  activeSessionId: string | null;
  taskSessionsById: Record<string, TaskSession>;
  environmentIdBySessionId: Record<string, string>;
}): { taskId: string; sessionId: string } | null {
  const { panelId, activeTaskId, activeSessionId, taskSessionsById, environmentIdBySessionId } =
    args;
  if (!panelId.startsWith("session:")) return null;
  const sid = panelId.slice("session:".length);
  if (!sid) return null;
  if (sid === activeSessionId) return null;
  if (!activeTaskId) return null;
  if (!environmentIdBySessionId[sid]) return null;
  const sessionTaskId = taskSessionsById[sid]?.task_id;
  if (sessionTaskId && sessionTaskId !== activeTaskId) return null;
  return { taskId: activeTaskId, sessionId: sid };
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
    if (isDebug()) {
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
        const position = undefined;
        // Only recreate a panel if there's still an active session.
        // If all sessions were deleted, leave the layout empty — the user
        // can create a new session via the "+" menu.
        if (!activeSessionId) {
          if (isDebug()) debug("setupChatPanelSafetyNet: skip recreate (no active session)");
          return;
        }
        // Don't recreate a panel for a session that no longer exists in the
        // store — this guards against handleDelete racing with the safety net.
        const activeTaskId = appStore.getState().tasks.activeTaskId;
        const knownSessions = activeTaskId
          ? (appStore.getState().taskSessionsByTask.itemsByTaskId[activeTaskId] ?? [])
          : [];
        if (!knownSessions.some((s) => s.id === activeSessionId)) {
          if (isDebug()) {
            debug("setupChatPanelSafetyNet: skip recreate (session not in store)", {
              activeSessionId,
              activeTaskId,
              knownSessionIds: knownSessions.map((s) => s.id),
            });
          }
          return;
        }
        if (isDebug()) {
          debug("setupChatPanelSafetyNet: recreating session panel", {
            activeSessionId,
            activeTaskId,
            anchor: "auto",
          });
        }
        api.addPanel({
          id: `session:${activeSessionId}`,
          component: "chat",
          tabComponent: "sessionTab",
          title: t("common:agent"),
          params: { sessionId: activeSessionId },
          position,
        });
        const nc = api.getPanel(`session:${activeSessionId}`);
        if (nc) {
          useDockviewStore.setState({
            centerGroupId: isCenterCandidateGroupId(nc.group.id) ? nc.group.id : CENTER_GROUP,
          });
        }
      });
    });
  });
}

export function resolveInitialPosition(
  api: DockviewApi,
  chatWasSelected = false,
): AddPanelOptions["position"] {
  // Prefer the live "chat" placeholder's group. The session panel must be added
  // INTO that group (so it's a tab beside chat) BEFORE chat is removed —
  // otherwise removing chat empties and destroys the center group, the stored
  // centerGroupId goes stale, and the session panel gets appended as a new row,
  // collapsing the horizontal default layout into a vertical stack.
  const chatGroupId = api.getPanel("chat")?.group?.id;
  if (chatGroupId) {
    // Put the replacement before selected Chat so its group remains focused
    // through removal. Otherwise retain the saved tab order for an inactive
    // Chat placeholder.
    return {
      referenceGroup: chatGroupId,
      ...(chatWasSelected ? { index: 0 } : {}),
    };
  }
  const { centerGroupId } = useDockviewStore.getState();
  const centerGroupExists =
    centerGroupId &&
    isCenterCandidateGroupId(centerGroupId) &&
    api.groups.some((g) => g.id === centerGroupId);
  if (centerGroupExists) return { referenceGroup: centerGroupId };
  const rightTopExists = api.groups.some((g) => g.id === RIGHT_TOP_GROUP);
  if (rightTopExists) return { referenceGroup: RIGHT_TOP_GROUP, direction: "left" };
  return undefined;
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
  const stalePanels = api.panels.filter((panel) => {
    if (!panel.id.startsWith("session:")) return false;
    const sid = panel.id.slice("session:".length);
    return sid !== keepSessionId && !currentIds.has(sid);
  });

  if (keepSessionId) {
    anchorIncomingSessionPanel(api, currentIds, stalePanels, keepSessionId, createdSet);
  }

  for (const panel of stalePanels) {
    const sid = panel.id.slice("session:".length);
    try {
      panel.api.close();
      removed.push(panel.id);
    } catch {
      /* already gone */
    }
    createdSet.delete(sid);
  }
  if (isDebug()) {
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
 * Whether the layout is maximized — session panels are intentionally absent
 * then (restored on exit-maximize). Returns true when callers should continue
 * ensuring panels, false to skip.
 */
function shouldEnsureSessionPanels(): boolean {
  return useDockviewStore.getState().preMaximizeLayout === null;
}

/**
 * Drop the generic "chat" placeholder once a real session panel exists.
 *
 * MUST run AFTER the session panel has been added to chat's group — removing
 * chat while it's the only panel in the center group destroys that group and
 * collapses the horizontal default layout into a vertical stack.
 */
function removeChatPlaceholder(api: DockviewApi): void {
  if (useDockviewStore.getState().preMaximizeLayout) return;
  const chatPanel = api.getPanel("chat");
  if (chatPanel) api.removePanel(chatPanel);
}

/**
 * Keep agent/session tabs before contextual siblings in their tab group.
 *
 * Dockview restores saved tab order verbatim. If a task previously saved
 * `pr-detail` before `session:<id>`, the restored agent panel already exists,
 * so `resolveInitialPosition(... index: 0)` never runs. Move session siblings
 * as a stable block so multi-session ordering remains stable.
 */
export function ensureSessionTabPrecedesNonSessionTabs(api: DockviewApi, sessionId: string): void {
  const panel = api.getPanel(`session:${sessionId}`);
  const groupPanels = panel?.group?.panels;
  if (!panel || !groupPanels) return;

  const firstNonSessionIndex = groupPanels.findIndex((p) => !p.id.startsWith("session:"));
  if (firstNonSessionIndex === -1) return;

  const sessionPanelsToMove = groupPanels
    .slice(firstNonSessionIndex + 1)
    .filter((groupPanel) => groupPanel.id.startsWith("session:"));

  for (const sessionPanel of sessionPanelsToMove.reverse()) {
    sessionPanel.api.moveTo({
      group: panel.group,
      position: "center",
      index: firstNonSessionIndex,
      skipSetActive: true,
    });
  }
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
  activation: {
    sessionPanelExistedBefore: boolean;
    activePanelIdBeforeEnsure: string | null;
  },
  refs: AutoSessionTabRefs,
  tid: string | null,
): ReturnType<DockviewApi["getPanel"]> {
  const activePanel = api.getPanel(`session:${effectiveSessionId}`);
  if (!activePanel) return activePanel;

  const activePanelIdAfterReorder = api.activePanel?.id ?? null;
  const shouldActivate = shouldActivateSessionPanel({
    sessionPanelExistedBefore: activation.sessionPanelExistedBefore,
    prevTaskId: refs.prevTaskIdRef.current,
    prevSessionId: refs.prevSessionIdRef.current,
    currentTaskId: tid,
    currentSessionId: effectiveSessionId,
    currentActivePanelId: activation.activePanelIdBeforeEnsure,
  });
  if (isDebug()) {
    debug("useAutoSessionTab: activation decision", {
      effectiveSessionId,
      shouldActivate,
      sessionPanelExistedBefore: activation.sessionPanelExistedBefore,
      prevTaskId: refs.prevTaskIdRef.current,
      prevSessionId: refs.prevSessionIdRef.current,
      currentTaskId: tid,
      activePanelIdBeforeEnsure: activation.activePanelIdBeforeEnsure,
      activePanelIdAfterReorder,
      activeGroupId: activePanel.group.id,
    });
  }
  if (shouldActivate) activePanel.api.setActive();
  useDockviewStore.setState({
    centerGroupId: isCenterCandidateGroupId(activePanel.group.id)
      ? activePanel.group.id
      : CENTER_GROUP,
  });
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
    if (isDebug() && !api.getPanel(`session:${sid}`)) created.push(sid);
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
    if (isDebug()) {
      debug("useAutoSessionTab: skip (session not in store yet)", {
        effectiveSessionId,
        currentSessionIds,
      });
    }
    return true;
  }
  if (!shouldEnsureSessionPanels()) {
    if (isDebug())
      debug("useAutoSessionTab: skip body (maximized - panels suppressed)", { effectiveSessionId });
    createdSet.add(effectiveSessionId);
    return true;
  }
  return false;
}

export function shouldRebuildDefaultForPendingSession(
  api: DockviewApi,
  effectiveSessionId: string | null,
  currentSessionIds: string[],
): boolean {
  if (!effectiveSessionId) return false;
  if (currentSessionIds.includes(toSessionId(effectiveSessionId))) return false;
  if (api.getPanel("chat")) return false;
  return true;
}

function logAutoSessionTabEffectEntry(
  api: DockviewApi,
  effectiveSessionId: string | null,
  tid: string | null,
  currentSessionIds: string[],
  refs: AutoSessionTabRefs,
): void {
  if (!isDebug()) return;
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

function logPendingSessionReset(
  api: DockviewApi,
  effectiveSessionId: string | null,
  currentSessionIds: string[],
): void {
  if (!isDebug()) return;
  debug("useAutoSessionTab: release default for pending session", {
    effectiveSessionId,
    currentSessionIds,
    currentLayoutEnvId: useDockviewStore.getState().currentLayoutEnvId,
    livePanelIds: api.panels.map((panel) => panel.id),
  });
}

function logSessionPanelEnsure(
  effectiveSessionId: string,
  sessionPanelExistedBefore: boolean,
  initialPosition: AddPanelOptions["position"],
): void {
  if (!isDebug()) return;
  debug("useAutoSessionTab: ensuring active session panel", {
    effectiveSessionId,
    sessionPanelExistedBefore,
    initialPosition: JSON.stringify(initialPosition),
  });
}

function logAutoSessionTabEffectExit(
  api: DockviewApi,
  effectiveSessionId: string,
  siblingsCreated: string[],
  activePanel: ReturnType<DockviewApi["getPanel"]>,
): void {
  if (!isDebug()) return;
  debug("useAutoSessionTab: effect exit", {
    effectiveSessionId,
    siblingsCreated,
    livePanelIdsAfter: api.panels.map((panel) => panel.id),
    activeGroupId: activePanel?.group.id ?? null,
    liveActivePanelId: api.activePanel?.id ?? null,
  });
}

function updateAutoSessionTabRefs(
  refs: AutoSessionTabRefs,
  tid: string | null,
  effectiveSessionId: string | null,
): void {
  refs.prevTaskIdRef.current = tid;
  refs.prevSessionIdRef.current = effectiveSessionId;
}

/**
 * Core effect body for useAutoSessionTab — extracted to reduce complexity of
 * the hook itself.
 */
export function runAutoSessionTabEffect(
  effectiveSessionId: string | null,
  appStore: ReturnType<typeof useAppStoreApi>,
  refs: AutoSessionTabRefs,
): void {
  const api = useDockviewStore.getState().api;
  if (!api) return;

  const { tid, currentSessionIds } = resolveCurrentSessionIds(appStore);

  logAutoSessionTabEffectEntry(api, effectiveSessionId, tid, currentSessionIds, refs);

  if (shouldRebuildDefaultForPendingSession(api, effectiveSessionId, currentSessionIds)) {
    logPendingSessionReset(api, effectiveSessionId, currentSessionIds);
    releaseLayoutToDefault(useDockviewStore.getState().currentLayoutEnvId);
    updateAutoSessionTabRefs(refs, tid, effectiveSessionId);
    return;
  }

  reconcileRemovedSessionPanels(
    api,
    refs.sessionTabCreatedRef.current,
    currentSessionIds,
    effectiveSessionId ?? "",
  );

  if (!effectiveSessionId) {
    if (isDebug()) debug("useAutoSessionTab: no effectiveSessionId, returning");
    updateAutoSessionTabRefs(refs, tid, effectiveSessionId);
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
    updateAutoSessionTabRefs(refs, tid, effectiveSessionId);
    return;
  }

  const chatWasSelected = isChatPlaceholderSelected(api);
  const initialPosition = resolveInitialPosition(api, chatWasSelected);
  const sessionPanelExistedBefore = !!api.getPanel(`session:${effectiveSessionId}`);
  // Preserve the restored/user-selected panel before adding and reordering the
  // session tab. Dockview's same-group move briefly activates a sibling even
  // with skipSetActive, so reading api.activePanel afterward loses this intent.
  const activePanelIdBeforeEnsure = api.activePanel?.id ?? null;
  const preserveActivePanel = shouldPreserveActivePanel(
    sessionPanelExistedBefore,
    activePanelIdBeforeEnsure,
  );

  logSessionPanelEnsure(effectiveSessionId, sessionPanelExistedBefore, initialPosition);

  ensureSessionPanel(
    api,
    effectiveSessionId,
    initialPosition,
    preserveActivePanel,
    refs.sessionTabCreatedRef.current,
  );

  // Chat is a placeholder for the real Agent panel. When it was selected and
  // this effect created its replacement, transfer that group-local selection
  // before removal so Dockview never chooses a neighboring Plan tab. A session
  // anchored during task switching already has its restored selection, which
  // must not be overwritten here.
  activateChatReplacement(api, effectiveSessionId, chatWasSelected && !sessionPanelExistedBefore);

  // Now that the session panel occupies the center group, drop the generic
  // "chat" placeholder. Order matters: removing chat first would empty and
  // destroy the center group, collapsing the horizontal layout.
  removeChatPlaceholder(api);
  ensureSessionTabPrecedesNonSessionTabs(api, effectiveSessionId);

  const activePanel = activateSessionPanel(
    api,
    effectiveSessionId,
    { sessionPanelExistedBefore, activePanelIdBeforeEnsure },
    refs,
    tid,
  );

  restorePreservedActivePanel(api, activePanelIdBeforeEnsure, preserveActivePanel);

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

  logAutoSessionTabEffectExit(api, effectiveSessionId, siblingsCreated, activePanel);

  updateAutoSessionTabRefs(refs, tid, effectiveSessionId);
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
  const dockviewApi = useDockviewStore((state) => state.api);

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
  }, [appStore, dockviewApi, effectiveSessionId, sessionIdsKey]);
}
