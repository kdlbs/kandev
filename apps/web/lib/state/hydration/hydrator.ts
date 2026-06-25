import type { Draft } from "immer";
import type { AppState } from "../store";
import { migrateView } from "../slices/ui/ui-slice";
import { getStoredQuickChatNames } from "@/lib/local-storage";
import { deepMerge, mergeSessionMap } from "./merge-strategies";

/**
 * Hydration options for controlling merge behavior
 */
export type HydrationOptions = {
  /** Active session ID to avoid overwriting live data */
  activeSessionId?: string | null;
  /** Whether to skip hydrating session runtime state (shell, processes, git) */
  skipSessionRuntime?: boolean;
  /** Force merge this session even if it's active (for navigation refresh) */
  forceMergeSessionId?: string | null;
};

/** Hydrate navigation and workspace slices. */
function hydrateNavigationAndWorkspace(draft: Draft<AppState>, state: Partial<AppState>): void {
  if (state.workflows) {
    draft.workflows.activeId = state.workflows.activeId ?? draft.workflows.activeId;
  }
  if (state.tasks) deepMerge(draft.tasks, state.tasks);
  if (state.workspaces) deepMerge(draft.workspaces, state.workspaces);
}

/** Hydrate settings slices, preserving loading states. */
function hydrateSettings(draft: Draft<AppState>, state: Partial<AppState>): void {
  if (state.userSettings && !draft.userSettings.loaded) {
    deepMerge(draft.userSettings, state.userSettings);
    bridgeSidebarViewsFromUserSettings(draft, state.userSettings);
  }
}

function bridgeSidebarViewsFromUserSettings(
  draft: Draft<AppState>,
  userSettings: Partial<AppState["userSettings"]>,
): void {
  const serverViews = userSettings.sidebarViews;
  const normalized = serverViews?.map(migrateView) ?? [];
  if (normalized.length > 0) {
    draft.sidebarViews.views = normalized;
  }
  if (
    userSettings.sidebarActiveViewId &&
    draft.sidebarViews.views.some((v) => v.id === userSettings.sidebarActiveViewId)
  ) {
    draft.sidebarViews.activeViewId = userSettings.sidebarActiveViewId;
  } else if (
    draft.sidebarViews.views.length > 0 &&
    !draft.sidebarViews.views.some((v) => v.id === draft.sidebarViews.activeViewId)
  ) {
    draft.sidebarViews.activeViewId = draft.sidebarViews.views[0].id;
  }
  if (userSettings.sidebarDraft !== undefined) {
    draft.sidebarViews.draft = userSettings.sidebarDraft;
  }
  if (userSettings.sidebarTaskPrefs) {
    if (draft.sidebarTaskPrefs.syncPending) return;
    const nextPrefs = { ...userSettings.sidebarTaskPrefs };
    if (draft.sidebarTaskPrefs.syncError) nextPrefs.syncError = draft.sidebarTaskPrefs.syncError;
    draft.sidebarTaskPrefs = nextPrefs;
  }
}

/** Hydrate session slices, protecting active sessions. */
function hydrateSession(
  draft: Draft<AppState>,
  state: Partial<AppState>,
  activeSessionId: string | null,
  forceMergeSessionId: string | null,
): void {
  if (state.messages) {
    if (state.messages.bySession)
      mergeSessionMap(
        draft.messages.bySession,
        state.messages.bySession,
        activeSessionId,
        forceMergeSessionId,
      );
    if (state.messages.metaBySession)
      mergeSessionMap(
        draft.messages.metaBySession,
        state.messages.metaBySession,
        activeSessionId,
        forceMergeSessionId,
      );
  }
  if (state.turns) {
    if (state.turns.bySession)
      mergeSessionMap(
        draft.turns.bySession,
        state.turns.bySession,
        activeSessionId,
        forceMergeSessionId,
      );
    if (state.turns.activeBySession)
      mergeSessionMap(
        draft.turns.activeBySession,
        state.turns.activeBySession,
        activeSessionId,
        forceMergeSessionId,
      );
  }
  if (state.taskSessions) deepMerge(draft.taskSessions, state.taskSessions);
  if (state.taskSessionsByTask) deepMerge(draft.taskSessionsByTask, state.taskSessionsByTask);
  if (state.sessionAgentctl) {
    mergeSessionMap(
      draft.sessionAgentctl.itemsBySessionId,
      state.sessionAgentctl?.itemsBySessionId,
      activeSessionId,
      forceMergeSessionId,
    );
  }
  if (state.worktrees) deepMerge(draft.worktrees, state.worktrees);
  if (state.sessionWorktreesBySessionId)
    deepMerge(draft.sessionWorktreesBySessionId, state.sessionWorktreesBySessionId);
  if (state.activeModel) deepMerge(draft.activeModel, state.activeModel);
}

/** Hydrate session runtime slices (volatile state). */
function hydrateSessionRuntime(
  draft: Draft<AppState>,
  state: Partial<AppState>,
  activeSessionId: string | null,
  forceMergeSessionId: string | null,
): void {
  if (state.shell) {
    mergeSessionMap(
      draft.shell.outputs,
      state.shell?.outputs,
      activeSessionId,
      forceMergeSessionId,
    );
    mergeSessionMap(
      draft.shell.statuses,
      state.shell?.statuses,
      activeSessionId,
      forceMergeSessionId,
    );
  }
  if (state.processes) deepMerge(draft.processes, state.processes);
  if (state.gitStatus) {
    mergeSessionMap(
      draft.gitStatus.byEnvironmentId,
      state.gitStatus?.byEnvironmentId,
      activeSessionId,
      forceMergeSessionId,
    );
  }
  if (state.contextWindow) {
    mergeSessionMap(
      draft.contextWindow.bySessionId,
      state.contextWindow?.bySessionId,
      activeSessionId,
      forceMergeSessionId,
    );
  }
  if (state.environmentIdBySessionId) {
    Object.assign(draft.environmentIdBySessionId, state.environmentIdBySessionId);
  }
  if (state.prepareProgress) {
    mergeSessionMap(
      draft.prepareProgress.bySessionId,
      state.prepareProgress?.bySessionId,
      activeSessionId,
      forceMergeSessionId,
    );
  }
}

/** Hydrate UI slices without overwriting active connection state. */
export function hydrateUI(draft: Draft<AppState>, state: Partial<AppState>): void {
  if (state.previewPanel) deepMerge(draft.previewPanel, state.previewPanel);
  if (state.rightPanel) deepMerge(draft.rightPanel, state.rightPanel);
  if (state.diffs) deepMerge(draft.diffs, state.diffs);
  if (state.quickChat) {
    // Merge quick chat sessions, preserving isOpen from client
    if (state.quickChat.sessions) {
      // Local renames live in localStorage and override the SSR-provided name
      // (which derives from the backend task title). Apply on every hydration
      // so a renamed chat keeps its local name across reloads and tab switches.
      const storedNames = getStoredQuickChatNames();
      draft.quickChat.sessions = state.quickChat.sessions.map((s) => {
        const local = storedNames[s.sessionId];
        return local ? { ...s, name: local } : s;
      });
      // Validate activeSessionId exists in sessions after merge
      if (
        draft.quickChat.activeSessionId &&
        !draft.quickChat.sessions.some((s) => s.sessionId === draft.quickChat.activeSessionId)
      ) {
        draft.quickChat.activeSessionId = draft.quickChat.sessions[0]?.sessionId ?? null;
      }
      // Close quick chat if no sessions remain
      if (draft.quickChat.sessions.length === 0) {
        draft.quickChat.isOpen = false;
      }
    }
  }
  if (state.connection) {
    const { status: _status, ...rest } = state.connection || {};
    if (Object.keys(rest).length > 0) {
      Object.assign(draft.connection, rest);
    }
  }
}

/**
 * Hydrates the app state with SSR data using smart merge strategies.
 *
 * Features:
 * - Deep merge for nested objects
 * - Avoids overwriting active sessions
 * - Preserves loading states to prevent flickering
 * - Partial hydration support
 */
export function hydrateState(
  draft: Draft<AppState>,
  state: Partial<AppState>,
  options: HydrationOptions = {},
): void {
  const {
    activeSessionId = null,
    skipSessionRuntime = false,
    forceMergeSessionId = null,
  } = options;

  hydrateNavigationAndWorkspace(draft, state);
  hydrateSettings(draft, state);
  hydrateSession(draft, state, activeSessionId, forceMergeSessionId);

  if (!skipSessionRuntime) {
    hydrateSessionRuntime(draft, state, activeSessionId, forceMergeSessionId);
  }

  hydrateGitHub(draft, state);
  hydrateUI(draft, state);

  // Office slice — shallow merge SSR-provided data into the store.
  if (state.office) {
    Object.assign(draft.office, state.office);
  }

  // Feature flags — overwrite whole map. SSR is authoritative; the backend
  // is the only source of truth for what's enabled in this deployment.
  if (state.features) {
    Object.assign(draft.features, state.features);
  }
}

/** Hydrate GitHub slices, preserving loading states. */
function hydrateGitHub(draft: Draft<AppState>, state: Partial<AppState>): void {
  if (state.pendingPrUrlByTaskId) {
    deepMerge(draft.pendingPrUrlByTaskId, state.pendingPrUrlByTaskId);
  }
  if (state.prFeedbackCache) deepMerge(draft.prFeedbackCache, state.prFeedbackCache);
}
