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

/** Merge kanban tasks by ID, keeping the version with the newer updatedAt timestamp. */
function mergeKanbanTasks(
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  draft: Draft<any>,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  source: any[] | undefined,
): void {
  if (!source || source.length === 0) return;
  const draftTasks = draft.tasks as Array<{ id: string; updatedAt?: string }>;
  const existingById = new Map(draftTasks.map((t) => [t.id, t]));

  for (const incoming of source) {
    const existing = existingById.get(incoming.id);
    if (!existing) {
      draftTasks.push(incoming);
    } else {
      const existingTime = existing.updatedAt ? new Date(existing.updatedAt).getTime() : 0;
      const incomingTime = incoming.updatedAt ? new Date(incoming.updatedAt).getTime() : 0;
      if (incomingTime >= existingTime) {
        const idx = draftTasks.findIndex((t) => t.id === incoming.id);
        if (idx >= 0) draftTasks[idx] = incoming;
      }
    }
  }
}

/** Hydrate kanban and workspace slices. */
function hydrateKanbanAndWorkspace(draft: Draft<AppState>, state: Partial<AppState>): void {
  if (state.kanban) {
    // Merge tasks by ID with timestamp comparison to avoid overwriting fresher WS data
    const { tasks, ...kanbanRest } = state.kanban;
    if (Object.keys(kanbanRest).length > 0) deepMerge(draft.kanban, kanbanRest);
    mergeKanbanTasks(draft.kanban, tasks);
  }
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
  if (state.agents) deepMerge(draft.agents, state.agents);
}

/** Hydrate UI slices without overwriting active connection state. */
function hydrateUI(draft: Draft<AppState>, state: Partial<AppState>): void {
  if (state.previewPanel) deepMerge(draft.previewPanel, state.previewPanel);
  if (state.rightPanel) deepMerge(draft.rightPanel, state.rightPanel);
  if (state.diffs) deepMerge(draft.diffs, state.diffs);
  if (state.quickChat) {
    // Merge quick chat sessions, preserving isOpen from client
    if (state.quickChat.sessions) {
      draft.quickChat.sessions = state.quickChat.sessions;
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

  hydrateGitHub(draft, state);
  hydrateUI(draft, state);
}

/** Hydrate GitHub slices, preserving loading states. */
function hydrateGitHub(draft: Draft<AppState>, state: Partial<AppState>): void {
  if (state.githubStatus) mergeWithLoading(draft.githubStatus, state.githubStatus);
  if (state.taskPRs) mergeWithLoading(draft.taskPRs, state.taskPRs);
  if (state.prWatches) mergeWithLoading(draft.prWatches, state.prWatches);
  if (state.reviewWatches) mergeWithLoading(draft.reviewWatches, state.reviewWatches);
}
