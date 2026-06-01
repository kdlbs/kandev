import type { Draft } from "immer";
import type { AppState } from "../store";
import type { SsrInitialState } from "@/lib/ssr/initial-state";
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

/**
 * Hydrate the client-only kanban + workspace selection slices. Server kanban
 * data (single/multi snapshots, workflows list) is seeded into the TanStack
 * Query cache by `StateHydrator`, NOT Zustand — so only the active-workflow
 * selection and active task/session selection are merged here. `workflows.items`
 * from the SSR payload is intentionally ignored (it lives in TQ).
 */
function hydrateKanbanAndWorkspace(draft: Draft<AppState>, state: SsrInitialState): void {
  if (state.workflows?.activeId !== undefined) {
    draft.workflows.activeId = state.workflows.activeId ?? null;
  }
  if (state.tasks) deepMerge(draft.tasks, state.tasks);
  if (state.workspaces?.activeId !== undefined) {
    draft.workspaces.activeId = state.workspaces.activeId ?? null;
  }
}

/**
 * Bridge the server-provided sidebar views (now seeded into the TanStack Query
 * `userSettings` cache, not a Zustand slice) into the client-only `sidebarViews`
 * UI slice. The rest of user settings lives in TQ and is read via
 * `useUserSettings`.
 */
function hydrateSidebarViews(draft: Draft<AppState>, state: SsrInitialState): void {
  const serverViews = state.userSettings?.sidebarViews;
  if (!state.userSettings?.loaded || !serverViews || serverViews.length === 0) return;
  const normalized = serverViews.map(migrateView);
  draft.sidebarViews.views = normalized;
  if (!normalized.some((v) => v.id === draft.sidebarViews.activeViewId)) {
    draft.sidebarViews.activeViewId = normalized[0].id;
  }
}

/** Hydrate session slices, protecting active sessions. */
function hydrateSession(
  draft: Draft<AppState>,
  state: SsrInitialState,
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
  if (state.pendingModel) deepMerge(draft.pendingModel, state.pendingModel);
  if (state.activeModel) deepMerge(draft.activeModel, state.activeModel);
}

/** Hydrate session runtime slices (volatile state). */
function hydrateSessionRuntime(
  draft: Draft<AppState>,
  state: SsrInitialState,
  activeSessionId: string | null,
  forceMergeSessionId: string | null,
): void {
  if (state.terminal) deepMerge(draft.terminal, state.terminal);
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
  // gitStatus is now hydrated into the TanStack Query cache (see StateHydrator),
  // not the Zustand mirror.
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
  if (state.agents) deepMerge(draft.agents, state.agents);
  // prepareProgress removed from Zustand — now seeded into the TanStack Query
  // cache from SSR by StateHydrator (seedPrepareProgressQueries) and fed live by
  // bridge/session-runtime.ts.
}

/** Hydrate UI slices without overwriting active connection state. */
export function hydrateUI(draft: Draft<AppState>, state: SsrInitialState): void {
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
  state: SsrInitialState,
  options: HydrationOptions = {},
): void {
  const {
    activeSessionId = null,
    skipSessionRuntime = false,
    forceMergeSessionId = null,
  } = options;

  hydrateKanbanAndWorkspace(draft, state);
  hydrateSidebarViews(draft, state);
  hydrateSession(draft, state, activeSessionId, forceMergeSessionId);

  if (!skipSessionRuntime) {
    hydrateSessionRuntime(draft, state, activeSessionId, forceMergeSessionId);
  }

  hydrateUI(draft, state);
}
