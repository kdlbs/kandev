import type { StoreApi } from 'zustand';
import type { AppState } from '@/lib/state/store';
import type { WsHandlers } from '@/lib/ws/handlers/types';
import type { TaskSessionState } from '@/lib/types/http';
import type { QueuedMessage } from '@/lib/state/slices/session/types';

/** Build a session update object from the state_changed payload. */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
function buildSessionUpdate(payload: any): Record<string, unknown> {
  const update: Record<string, unknown> = {};
  if (payload.new_state) update.state = payload.new_state;
  if (payload.review_status !== undefined) update.review_status = payload.review_status;
  if (payload.workflow_step_id !== undefined) update.workflow_step_id = payload.workflow_step_id;
  if (payload.agent_profile_snapshot) update.agent_profile_snapshot = payload.agent_profile_snapshot;
  if (payload.is_passthrough !== undefined) update.is_passthrough = payload.is_passthrough;
  return update;
}

/** Upsert the session in the per-task sessions list. */
function upsertTaskSessionList(
  store: StoreApi<AppState>, taskId: string, sessionId: string,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  payload: any, sessionUpdate: Record<string, unknown>
): void {
  const sessionsByTask = store.getState().taskSessionsByTask.itemsByTaskId[taskId];
  if (!sessionsByTask) return;

  const hasSession = sessionsByTask.some((s) => s.id === sessionId);
  const newState = payload.new_state as TaskSessionState | undefined;

  if (!hasSession && newState) {
    store.getState().setTaskSessionsForTask(taskId, [...sessionsByTask, {
      id: sessionId, task_id: taskId, state: newState, started_at: '', updated_at: '',
      ...(payload.agent_profile_id ? { agent_profile_id: payload.agent_profile_id } : {}),
      ...sessionUpdate,
    }]);
  } else if (hasSession) {
    store.getState().setTaskSessionsForTask(taskId, sessionsByTask.map((s) =>
      s.id === sessionId ? { ...s, ...sessionUpdate } : s
    ));
  }
}

/** Extract context window data from payload metadata and store it. */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
function extractContextWindow(store: StoreApi<AppState>, sessionId: string, payload: any): void {
  const metadata = payload.metadata;
  if (!metadata || typeof metadata !== 'object') return;
  const contextWindow = (metadata as Record<string, unknown>).context_window;
  if (!contextWindow || typeof contextWindow !== 'object') return;
  const cw = contextWindow as Record<string, unknown>;
  store.getState().setContextWindow(sessionId, {
    size: (cw.size as number) ?? 0,
    used: (cw.used as number) ?? 0,
    remaining: (cw.remaining as number) ?? 0,
    efficiency: (cw.efficiency as number) ?? 0,
    timestamp: new Date().toISOString(),
  });
}

/** Handle the agentctl_ready event: update session worktree info. */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
function handleAgentctlReady(store: StoreApi<AppState>, payload: any): void {
  const existingSession = store.getState().taskSessions.items[payload.session_id];
  if (!existingSession) return;

  const sessionUpdate: Record<string, unknown> = {};
  if (payload.worktree_id) sessionUpdate.worktree_id = payload.worktree_id;
  if (payload.worktree_path) sessionUpdate.worktree_path = payload.worktree_path;
  if (payload.worktree_branch) sessionUpdate.worktree_branch = payload.worktree_branch;

  if (Object.keys(sessionUpdate).length > 0) {
    store.getState().setTaskSession({ ...existingSession, ...sessionUpdate });
  }

  if (payload.worktree_id) {
    store.getState().setWorktree({
      id: payload.worktree_id, sessionId: payload.session_id,
      repositoryId: existingSession.repository_id ?? undefined,
      path: payload.worktree_path ?? existingSession.worktree_path ?? undefined,
      branch: payload.worktree_branch ?? existingSession.worktree_branch ?? undefined,
    });
    const existing = store.getState().sessionWorktreesBySessionId.itemsBySessionId[payload.session_id] ?? [];
    if (!existing.includes(payload.worktree_id)) {
      store.getState().setSessionWorktrees(payload.session_id, [...existing, payload.worktree_id]);
    }
  }
}

export function registerTaskSessionHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    'message.queue.status_changed': (message) => {
      const payload = message.payload;
      if (!payload?.session_id) {
        console.warn('[Queue] Missing session_id in queue status change event');
        return;
      }
      const sessionId = payload.session_id;
      const isQueued = payload.is_queued as boolean;
      const queuedMessage = payload.message as QueuedMessage | null | undefined;
      console.log('[Queue] Status changed:', { sessionId, isQueued, hasMessage: !!queuedMessage });
      store.getState().setQueueStatus(sessionId, { is_queued: isQueued, message: queuedMessage });
    },
    'session.state_changed': (message) => {
      const payload = message.payload;
      if (!payload?.task_id) return;
      const { task_id: taskId, session_id: sessionId, new_state: newState } = payload;

      if (!sessionId) return;

      const sessionUpdate = buildSessionUpdate(payload);
      const existingSession = store.getState().taskSessions.items[sessionId];

      if (existingSession) {
        store.getState().setTaskSession({ ...existingSession, ...sessionUpdate });
      } else if (newState) {
        store.getState().setTaskSession({
          id: sessionId, task_id: taskId, state: newState as TaskSessionState,
          started_at: '', updated_at: '',
          ...(payload.agent_profile_id ? { agent_profile_id: payload.agent_profile_id } : {}),
          ...sessionUpdate,
        });
      }

      upsertTaskSessionList(store, taskId, sessionId, payload, sessionUpdate);
      extractContextWindow(store, sessionId, payload);
    },
    'session.agentctl_starting': (message) => {
      const payload = message.payload;
      if (!payload?.session_id) return;
      store.getState().setSessionAgentctlStatus(payload.session_id, {
        status: 'starting', agentExecutionId: payload.agent_execution_id, updatedAt: message.timestamp,
      });
    },
    'session.agentctl_ready': (message) => {
      const payload = message.payload;
      if (!payload?.session_id) return;
      store.getState().setSessionAgentctlStatus(payload.session_id, {
        status: 'ready', agentExecutionId: payload.agent_execution_id, updatedAt: message.timestamp,
      });
      handleAgentctlReady(store, payload);
    },
    'session.agentctl_error': (message) => {
      const payload = message.payload;
      if (!payload?.session_id) return;
      store.getState().setSessionAgentctlStatus(payload.session_id, {
        status: 'error', agentExecutionId: payload.agent_execution_id,
        errorMessage: payload.error_message, updatedAt: message.timestamp,
      });
    },
  };
}
