import { useEffect, useRef, type MutableRefObject } from "react";
import type { DockviewApi, DockviewReadyEvent, AddPanelOptions } from "dockview-react";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { releaseLayoutToDefault, useDockviewStore } from "@/lib/state/dockview-store";
import { focusOrAddPanel } from "@/lib/state/dockview-layout-builders";
import {
  CENTER_GROUP,
  isCenterCandidateGroupId,
  RIGHT_TOP_GROUP,
} from "@/lib/state/layout-manager";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { wasPRPanelOffered, markPRPanelOffered } from "@/lib/local-storage";
import { sessionId as toSessionId } from "@/lib/types/ids";
import { createDebugLogger, isDebug } from "@/lib/debug/log";
import type { TaskSession } from "@/lib/types/http";
import type { TaskPR } from "@/lib/types/github";
import { getPrimaryTaskPR } from "@/hooks/domains/github/use-task-pr";
import { prTaskKey } from "@/components/github/pr-utils";

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
          title: "Agent",
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
  const sessionGroupId = sessionPanel?.group?.id;
  if (sessionGroupId && isCenterCandidateGroupId(sessionGroupId)) return sessionGroupId;
  return isCenterCandidateGroupId(centerGroupId) ? centerGroupId : CENTER_GROUP;
}

/**
 * Derive the auto-PR-panel decision inputs from one task's live PR list.
 *
 * @param taskPRs - The task's associated PRs, in creation order; `undefined`
 *   when the task has none loaded.
 * @returns `hasPR` — whether the task has any linked PR; `defaultPRKey` —
 *   the key of the primary/first PR (matches `PRDetailPanelComponent`'s
 *   fallback when no explicit `prKey` param is set), or `undefined` when
 *   there's no PR.
 */
function resolveAutoPRPanelState(taskPRs: TaskPR[] | undefined): {
  hasPR: boolean;
  defaultPRKey: string | undefined;
} {
  const primary = getPrimaryTaskPR(taskPRs);
  return {
    hasPR: !!taskPRs && taskPRs.length > 0,
    defaultPRKey: primary ? prTaskKey(primary) : undefined,
  };
}

/**
 * Pure effect logic for `useAutoPRPanel`: decides whether to add, remove,
 * or leave alone the auto-shown PR detail panel, and mutates the given
 * dockview `api` accordingly. Extracted for unit testing.
 *
 * @param api - The live dockview API to add/remove/update panels on.
 * @param sessionId - The active session, used for the offered/dismissed
 *   sessionStorage flag and to resolve the panel's target group.
 * @param params.hasPR - Whether the active task has any linked PR.
 * @param params.defaultPRKey - Key of the PR the legacy unkeyed
 *   "pr-detail" panel should render (and stay resynced to).
 * @param params.isRestoringLayout - Suppresses auto-add while a saved
 *   layout is being restored.
 * @param params.isMaximized - Suppresses auto-add while a group is
 *   maximized.
 * @param params.centerGroupId - Fallback group when no live session panel
 *   can anchor the new PR panel.
 */
export function runAutoPRPanelEffect(
  api: DockviewApi,
  sessionId: string,
  params: {
    hasPR: boolean;
    /** Key of the PR the legacy unkeyed "pr-detail" panel should render. */
    defaultPRKey: string | undefined;
    isRestoringLayout: boolean;
    isMaximized: boolean;
    centerGroupId: string;
  },
): void {
  const decision = shouldAutoAddPRPanel({
    hasPR: params.hasPR,
    panelExists: !!api.getPanel("pr-detail"),
    isRestoringLayout: params.isRestoringLayout,
    isMaximized: params.isMaximized,
    wasOffered: wasPRPanelOffered(sessionId),
  });
  if (decision === "remove") {
    api.getPanel("pr-detail")?.api.close();
    return;
  }

  if (decision === "add") {
    const targetGroupId = resolvePRPanelTargetGroup(api, sessionId, params.centerGroupId);
    focusOrAddPanel(api, {
      id: "pr-detail",
      component: "pr-detail",
      title: "Pull Request",
      position: { referenceGroup: targetGroupId },
      inactive: true,
      // Stamp the panel's params so addPRPanel can tell a matching menu
      // click (reuse this tab) apart from a different PR's click (open a
      // new tab) — see addPRPanel in dockview-panel-actions.ts.
      params: params.defaultPRKey ? { prKey: params.defaultPRKey } : undefined,
    });
    markPRPanelOffered(sessionId);
    return;
  }

  // "none" — panel already present or conditions not met.
  // Mark as offered if the panel exists (e.g. restored from saved layout).
  const legacy = api.getPanel("pr-detail");
  if (params.hasPR && legacy) {
    markPRPanelOffered(sessionId);
    // Keep the legacy tab's stamped key in sync with the CURRENT default PR.
    // Nothing else ever writes a different key onto this specific panel — a
    // manual "+" menu pick of a different PR always creates its own keyed
    // `pr-detail|<key>` tab instead (see addPRPanel in
    // dockview-panel-actions.ts) — so unconditionally resyncing here is safe
    // and fixes staleness both when the primary PR changes for this task and
    // when this panel is reused across a task switch (see Greptile/cubic
    // review on PR #1636).
    if (params.defaultPRKey && legacy.params?.prKey !== params.defaultPRKey) {
      legacy.api.updateParameters({ prKey: params.defaultPRKey });
    }
  }
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
    return resolveAutoPRPanelState(tid ? s.taskPRs.byTaskId[tid] : undefined).hasPR;
  });
  // Key of the PR the legacy unkeyed "pr-detail" panel renders — mirrors
  // PRDetailPanelComponent's fallback of the primary/first TaskPR.
  const defaultPRKey = useAppStore((s) => {
    const tid = s.tasks.activeTaskId;
    return resolveAutoPRPanelState(tid ? s.taskPRs.byTaskId[tid] : undefined).defaultPRKey;
  });
  const hasApi = useDockviewStore((s) => !!s.api);
  const appStore = useAppStoreApi();

  useEffect(() => {
    if (!taskId || !hasApi || !sessionId) return;

    requestAnimationFrame(() => {
      requestAnimationFrame(() => {
        const api = useDockviewStore.getState().api;
        if (!api) return;

        // Re-read live task/session/PR state before mutating dockview — a
        // task or session switch during this two-frame delay must not stamp
        // the panel with a stale task's PR key (cubic-dev-ai review on PR
        // #1636). If the active task/session moved on, bail: the effect
        // instance already scheduled for the new task/session (its deps
        // changed) will handle it correctly.
        const liveTasks = appStore.getState().tasks;
        if (liveTasks.activeTaskId !== taskId || liveTasks.activeSessionId !== sessionId) return;
        const live = resolveAutoPRPanelState(appStore.getState().taskPRs.byTaskId[taskId]);

        runAutoPRPanelEffect(api, sessionId, {
          hasPR: live.hasPR,
          defaultPRKey: live.defaultPRKey,
          isRestoringLayout: useDockviewStore.getState().isRestoringLayout,
          isMaximized: useDockviewStore.getState().preMaximizeLayout !== null,
          centerGroupId: useDockviewStore.getState().centerGroupId,
        });
      });
    });
  }, [taskId, hasPR, hasApi, sessionId, defaultPRKey, appStore]);
}

/**
 * Panels that are added co-tabbed with a session panel (see `useAutoPRPanel`).
 * When a saved layout's session was stripped (phantom-session sanitize on
 * page load, or stale removal during env switch), these siblings end up alone
 * in a group with no session. Prefer joining that group when adding the new
 * active session — without this fallback we'd add the session as a fresh
 * split next to the sidebar, breaking the user's grouping.
 *
 * Each entry matches either the bare id (e.g. `pr-detail`) or a keyed
 * variant `<id>|<key>` (multi-repo PR panels use `pr-detail|owner/repo/N`,
 * see `addPRPanel` in dockview-panel-actions.ts).
 */
const SESSION_ANCHOR_PANEL_IDS = ["pr-detail"];

export function findSessionAnchorGroupId(api: DockviewApi): string | null {
  for (const id of SESSION_ANCHOR_PANEL_IDS) {
    const exact = api.getPanel(id);
    if (exact) return exact.group.id;
    const keyedPrefix = `${id}|`;
    const keyed = api.panels.find((p) => p.id.startsWith(keyedPrefix));
    if (keyed) return keyed.group.id;
  }
  return null;
}

export function resolveInitialPosition(api: DockviewApi): AddPanelOptions["position"] {
  // Prefer the live "chat" placeholder's group. The session panel must be added
  // INTO that group (so it's a tab beside chat) BEFORE chat is removed —
  // otherwise removing chat empties and destroys the center group, the stored
  // centerGroupId goes stale, and the session panel gets appended as a new row,
  // collapsing the horizontal default layout into a vertical stack.
  const chatGroupId = api.getPanel("chat")?.group?.id;
  if (chatGroupId && isCenterCandidateGroupId(chatGroupId)) return { referenceGroup: chatGroupId };
  const { centerGroupId } = useDockviewStore.getState();
  const centerGroupExists =
    centerGroupId &&
    isCenterCandidateGroupId(centerGroupId) &&
    api.groups.some((g) => g.id === centerGroupId);
  if (centerGroupExists) return { referenceGroup: centerGroupId };
  const anchorGroupId = findSessionAnchorGroupId(api);
  // index:0 matches the project's session-on-the-left convention (agent tab
  // first, pr-detail/etc. to the right). Worth noting: if a user had
  // rearranged a previous layout to put pr-detail first, that ordering is
  // lost here — but the alternative (appending) would put the agent tab
  // to the right of pr-detail, which contradicts the default placement
  // every other code path produces. Pick the consistent default.
  if (anchorGroupId && isCenterCandidateGroupId(anchorGroupId)) {
    return { referenceGroup: anchorGroupId, index: 0 };
  }
  const rightTopExists = api.groups.some((g) => g.id === RIGHT_TOP_GROUP);
  if (rightTopExists) return { referenceGroup: RIGHT_TOP_GROUP, direction: "left" };
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

/**
 * Decide whether to force-activate the session panel after it (and any
 * sibling tabs) have been ensured.
 *
 * - It was just created by `ensureSessionPanel` (no prior dockview state
 *   to honor), or
 * - The hook is mounting for the first time (initial page load) AND
 *   dockview did not restore a different active panel from the saved
 *   layout — fall back to focusing the agent tab so chat is visible, or
 * - The user switched sessions within the same task (intra-task switch
 *   where dockview hasn't re-activated the new session for us).
 *
 * After an env (task) switch the prev refs are populated and the task
 * changed — the saved layout's active panel has already been restored for
 * the incoming task, so calling setActive here would override it and force
 * the agent tab on top of whatever the user had focused.
 *
 * On first mount when the session panel was already in the restored layout,
 * respect dockview's restored active panel (e.g. a file diff the user had
 * focused before refresh) instead of forcing the agent tab on top.
 */
export function shouldActivateSessionPanel(args: {
  sessionPanelExistedBefore: boolean;
  prevTaskId: string | null;
  prevSessionId: string | null;
  currentTaskId: string | null;
  currentSessionId: string;
  currentActivePanelId: string | null;
}): boolean {
  const {
    sessionPanelExistedBefore,
    prevTaskId,
    prevSessionId,
    currentTaskId,
    currentSessionId,
    currentActivePanelId,
  } = args;
  if (!sessionPanelExistedBefore) return true;
  const isFirstMount = prevTaskId === null && prevSessionId === null;
  if (isFirstMount) {
    const sessionPanelId = `session:${currentSessionId}`;
    if (!currentActivePanelId || currentActivePanelId === sessionPanelId) return true;
    return false;
  }
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

  const currentActivePanelId = api.activePanel?.id ?? null;
  const shouldActivate = shouldActivateSessionPanel({
    sessionPanelExistedBefore,
    prevTaskId: refs.prevTaskIdRef.current,
    prevSessionId: refs.prevSessionIdRef.current,
    currentTaskId: tid,
    currentSessionId: effectiveSessionId,
    currentActivePanelId,
  });
  if (isDebug()) {
    debug("useAutoSessionTab: activation decision", {
      effectiveSessionId,
      shouldActivate,
      sessionPanelExistedBefore,
      prevTaskId: refs.prevTaskIdRef.current,
      prevSessionId: refs.prevSessionIdRef.current,
      currentTaskId: tid,
      currentActivePanelId,
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
    if (isDebug()) {
      debug("useAutoSessionTab: release default for pending session", {
        effectiveSessionId,
        currentSessionIds,
        currentLayoutEnvId: useDockviewStore.getState().currentLayoutEnvId,
        livePanelIds: api.panels.map((p) => p.id),
      });
    }
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

  const initialPosition = resolveInitialPosition(api);
  const sessionPanelExistedBefore = !!api.getPanel(`session:${effectiveSessionId}`);

  if (isDebug()) {
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

  // Now that the session panel occupies the center group, drop the generic
  // "chat" placeholder. Order matters: removing chat first would empty and
  // destroy the center group, collapsing the horizontal layout.
  removeChatPlaceholder(api);
  ensureSessionTabPrecedesNonSessionTabs(api, effectiveSessionId);

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

  if (isDebug()) {
    debug("useAutoSessionTab: effect exit", {
      effectiveSessionId,
      siblingsCreated,
      livePanelIdsAfter: api.panels.map((p) => p.id),
      activeGroupId: activePanel?.group.id ?? null,
      liveActivePanelId: api.activePanel?.id ?? null,
    });
  }

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
