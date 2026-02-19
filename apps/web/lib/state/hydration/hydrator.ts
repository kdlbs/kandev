import type { Draft } from "immer";
import type { AppState } from "../store";
import { deepMerge, mergeSessionMap, mergeLoadingState } from "./merge-strategies";

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

/** Deep-merge a field with optional loading state preservation. */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
function mergeWithLoading(draft: any, source: any | undefined): void {
  if (!source) return;
  deepMerge(draft, source);
  mergeLoadingState(draft, source);
}

/** Hydrate kanban and workspace slices. */
function hydrateKanbanAndWorkspace(draft: Draft<AppState>, state: Partial<AppState>): void {
  if (state.kanban) deepMerge(draft.kanban, state.kanban);
  if (state.kanbanMulti) deepMerge(draft.kanbanMulti, state.kanbanMulti);
  if (state.workflows) deepMerge(draft.workflows, state.workflows);
  if (state.tasks) deepMerge(draft.tasks, state.tasks);
  if (state.workspaces) deepMerge(draft.workspaces, state.workspaces);
  if (state.repositories) deepMerge(draft.repositories, state.repositories);
  if (state.repositoryBranches) deepMerge(draft.repositoryBranches, state.repositoryBranches);
}

/** Hydrate settings slices, preserving loading states. */
function hydrateSettings(draft: Draft<AppState>, state: Partial<AppState>): void {
  if (state.executors) deepMerge(draft.executors, state.executors);
  if (state.environments) deepMerge(draft.environments, state.environments);
  if (state.settingsAgents) deepMerge(draft.settingsAgents, state.settingsAgents);
  if (state.agentDiscovery) deepMerge(draft.agentDiscovery, state.agentDiscovery);
  mergeWithLoading(draft.availableAgents, state.availableAgents);
  if (state.agentProfiles) deepMerge(draft.agentProfiles, state.agentProfiles);
  mergeWithLoading(draft.editors, state.editors);
  mergeWithLoading(draft.prompts, state.prompts);
  mergeWithLoading(draft.notificationProviders, state.notificationProviders);
  if (state.settingsData) deepMerge(draft.settingsData, state.settingsData);
  if (state.userSettings && !draft.userSettings.loaded) {
    deepMerge(draft.userSettings, state.userSettings);
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
  if (state.pendingModel) deepMerge(draft.pendingModel, state.pendingModel);
  if (state.activeModel) deepMerge(draft.activeModel, state.activeModel);
}

/** Hydrate session runtime slices (volatile state). */
function hydrateSessionRuntime(
  draft: Draft<AppState>,
  state: Partial<AppState>,
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
  if (state.gitStatus) {
    mergeSessionMap(
      draft.gitStatus.bySessionId,
      state.gitStatus?.bySessionId,
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
  if (state.agents) deepMerge(draft.agents, state.agents);
}

/** Hydrate UI slices without overwriting active connection state. */
function hydrateUI(draft: Draft<AppState>, state: Partial<AppState>): void {
  if (state.previewPanel) deepMerge(draft.previewPanel, state.previewPanel);
  if (state.rightPanel) deepMerge(draft.rightPanel, state.rightPanel);
  if (state.diffs) deepMerge(draft.diffs, state.diffs);
  if (state.connection) {
    // eslint-disable-next-line @typescript-eslint/no-unused-vars
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

  hydrateKanbanAndWorkspace(draft, state);
  hydrateSettings(draft, state);
  hydrateSession(draft, state, activeSessionId, forceMergeSessionId);

  if (!skipSessionRuntime) {
    hydrateSessionRuntime(draft, state, activeSessionId, forceMergeSessionId);
  }

  hydrateUI(draft, state);
}
