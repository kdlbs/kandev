import type { Draft } from 'immer';
import type { AppState } from '../store';
import { deepMerge, mergeSessionMap, mergeLoadingState } from './merge-strategies';

/**
 * Hydration options for controlling merge behavior
 */
export type HydrationOptions = {
  /** Active session ID to avoid overwriting live data */
  activeSessionId?: string | null;
  /** Whether to skip hydrating session runtime state (shell, processes, git) */
  skipSessionRuntime?: boolean;
};

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
  options: HydrationOptions = {}
): void {
  const { activeSessionId = null, skipSessionRuntime = false } = options;

  // Kanban slice - always safe to hydrate
  if (state.kanban) deepMerge(draft.kanban, state.kanban);
  if (state.boards) deepMerge(draft.boards, state.boards);
  if (state.tasks) deepMerge(draft.tasks, state.tasks);

  // Workspace slice - always safe to hydrate
  if (state.workspaces) deepMerge(draft.workspaces, state.workspaces);
  if (state.repositories) deepMerge(draft.repositories, state.repositories);
  if (state.repositoryBranches) deepMerge(draft.repositoryBranches, state.repositoryBranches);

  // Settings slice - always safe to hydrate, but preserve loading states
  if (state.executors) deepMerge(draft.executors, state.executors);
  if (state.environments) deepMerge(draft.environments, state.environments);
  if (state.settingsAgents) deepMerge(draft.settingsAgents, state.settingsAgents);
  if (state.agentDiscovery) deepMerge(draft.agentDiscovery, state.agentDiscovery);
  if (state.availableAgents) {
    deepMerge(draft.availableAgents, state.availableAgents);
    mergeLoadingState(draft.availableAgents, state.availableAgents);
  }
  if (state.agentProfiles) deepMerge(draft.agentProfiles, state.agentProfiles);
  if (state.editors) {
    deepMerge(draft.editors, state.editors);
    mergeLoadingState(draft.editors, state.editors);
  }
  if (state.prompts) {
    deepMerge(draft.prompts, state.prompts);
    mergeLoadingState(draft.prompts, state.prompts);
  }
  if (state.notificationProviders) {
    deepMerge(draft.notificationProviders, state.notificationProviders);
    mergeLoadingState(draft.notificationProviders, state.notificationProviders);
  }
  if (state.settingsData) deepMerge(draft.settingsData, state.settingsData);
  if (state.userSettings) deepMerge(draft.userSettings, state.userSettings);

  // Session slice - careful with active sessions
  if (state.messages) {
    if (state.messages.bySession) {
      mergeSessionMap(draft.messages.bySession, state.messages.bySession, activeSessionId);
    }
    if (state.messages.metaBySession) {
      mergeSessionMap(draft.messages.metaBySession, state.messages.metaBySession, activeSessionId);
    }
  }
  if (state.turns) {
    if (state.turns.bySession) {
      mergeSessionMap(draft.turns.bySession, state.turns.bySession, activeSessionId);
    }
    if (state.turns.activeBySession) {
      mergeSessionMap(draft.turns.activeBySession, state.turns.activeBySession, activeSessionId);
    }
  }
  if (state.taskSessions) deepMerge(draft.taskSessions, state.taskSessions);
  if (state.taskSessionsByTask) deepMerge(draft.taskSessionsByTask, state.taskSessionsByTask);
  if (state.sessionAgentctl) {
    mergeSessionMap(draft.sessionAgentctl.itemsBySessionId, state.sessionAgentctl?.itemsBySessionId, activeSessionId);
  }
  if (state.worktrees) deepMerge(draft.worktrees, state.worktrees);
  if (state.sessionWorktreesBySessionId) {
    deepMerge(draft.sessionWorktreesBySessionId, state.sessionWorktreesBySessionId);
  }
  if (state.pendingModel) deepMerge(draft.pendingModel, state.pendingModel);
  if (state.activeModel) deepMerge(draft.activeModel, state.activeModel);

  // Session Runtime slice - skip if requested (volatile state)
  if (!skipSessionRuntime) {
    if (state.terminal) deepMerge(draft.terminal, state.terminal);
    if (state.shell) {
      mergeSessionMap(draft.shell.outputs, state.shell?.outputs, activeSessionId);
      mergeSessionMap(draft.shell.statuses, state.shell?.statuses, activeSessionId);
    }
    if (state.processes) deepMerge(draft.processes, state.processes);
    if (state.gitStatus) {
      mergeSessionMap(draft.gitStatus.bySessionId, state.gitStatus?.bySessionId, activeSessionId);
    }
    if (state.contextWindow) {
      mergeSessionMap(draft.contextWindow.bySessionId, state.contextWindow?.bySessionId, activeSessionId);
    }
    if (state.agents) deepMerge(draft.agents, state.agents);
  }

  // UI slice - merge but don't overwrite active state
  if (state.previewPanel) deepMerge(draft.previewPanel, state.previewPanel);
  if (state.rightPanel) deepMerge(draft.rightPanel, state.rightPanel);
  if (state.diffs) deepMerge(draft.diffs, state.diffs);
  if (state.connection) {
    // Don't overwrite connection status from SSR - it's always stale
    // eslint-disable-next-line @typescript-eslint/no-unused-vars
    const { status: _status, ...rest } = state.connection || {};
    if (Object.keys(rest).length > 0) {
      Object.assign(draft.connection, rest);
    }
  }
}
