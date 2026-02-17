import type { StoreApi } from 'zustand';
import type { AppState } from '@/lib/state/store';
import type { WsHandlers } from '@/lib/ws/handlers/types';
import type { TaskSessionState } from '@/lib/types/http';
import type { QueuedMessage } from '@/lib/state/slices/session/types';

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

      store.getState().setQueueStatus(sessionId, {
        is_queued: isQueued,
        message: queuedMessage,
      });
    },
    'session.state_changed': (message) => {
      const payload = message.payload;
      if (!payload?.task_id) {
        return;
      }
      const taskId = payload.task_id;
      const newState = payload.new_state as TaskSessionState | undefined;
      const sessionId = payload.session_id;
      const reviewStatus = payload.review_status as string | undefined;
      const workflowStepId = payload.workflow_step_id as string | undefined;

      // Also update or create the session object if we have the session ID
      if (sessionId) {
        const existingSession = store.getState().taskSessions.items[sessionId];
        const agentProfileId = payload.agent_profile_id;
        const agentProfileSnapshot = payload.agent_profile_snapshot as Record<string, unknown> | undefined;

        const isPassthrough = payload.is_passthrough as boolean | undefined;

        // Build the update object with all possible fields
        const sessionUpdate: Record<string, unknown> = {};
        if (newState) sessionUpdate.state = newState;
        if (reviewStatus !== undefined) sessionUpdate.review_status = reviewStatus;
        if (workflowStepId !== undefined) sessionUpdate.workflow_step_id = workflowStepId;
        if (agentProfileSnapshot) sessionUpdate.agent_profile_snapshot = agentProfileSnapshot;
        if (isPassthrough !== undefined) sessionUpdate.is_passthrough = isPassthrough;

        if (existingSession) {
          // Update existing session - preserve all existing fields
          store.getState().setTaskSession({
            ...existingSession,
            ...sessionUpdate,
          });
        } else if (newState) {
          // Create partial session object only if we have a state
          store.getState().setTaskSession({
            id: sessionId,
            task_id: taskId,
            state: newState,
            started_at: '',
            updated_at: '',
            ...(agentProfileId ? { agent_profile_id: agentProfileId } : {}),
            ...sessionUpdate,
          });
        }

        const sessionsByTask = store.getState().taskSessionsByTask.itemsByTaskId[taskId];
        if (sessionsByTask) {
          const hasSession = sessionsByTask.some((session) => session.id === sessionId);
          if (!hasSession && newState) {
            store.getState().setTaskSessionsForTask(taskId, [...sessionsByTask, {
              id: sessionId,
              task_id: taskId,
              state: newState,
              started_at: '',
              updated_at: '',
              ...(agentProfileId ? { agent_profile_id: agentProfileId } : {}),
              ...sessionUpdate,
            }]);
          } else if (hasSession) {
            const nextSessions = sessionsByTask.map((session) =>
              session.id === sessionId
                ? { ...session, ...sessionUpdate }
                : session
            );
            store.getState().setTaskSessionsForTask(taskId, nextSessions);
          }
        }

        // Extract context window data from metadata if present
        const metadata = payload.metadata;
        if (metadata && typeof metadata === 'object') {
          const contextWindow = (metadata as Record<string, unknown>).context_window;
          if (contextWindow && typeof contextWindow === 'object') {
            const cw = contextWindow as Record<string, unknown>;
            store.getState().setContextWindow(sessionId, {
              size: (cw.size as number) ?? 0,
              used: (cw.used as number) ?? 0,
              remaining: (cw.remaining as number) ?? 0,
              efficiency: (cw.efficiency as number) ?? 0,
              timestamp: new Date().toISOString(),
            });
          }
        }
      }
    },
    'session.agentctl_starting': (message) => {
      const payload = message.payload;
      if (!payload?.session_id) {
        return;
      }
      store.getState().setSessionAgentctlStatus(payload.session_id, {
        status: 'starting',
        agentExecutionId: payload.agent_execution_id,
        updatedAt: message.timestamp,
      });
    },
    'session.agentctl_ready': (message) => {
      const payload = message.payload;
      if (!payload?.session_id) {
        return;
      }
      store.getState().setSessionAgentctlStatus(payload.session_id, {
        status: 'ready',
        agentExecutionId: payload.agent_execution_id,
        updatedAt: message.timestamp,
      });

      const existingSession = store.getState().taskSessions.items[payload.session_id];
      if (!existingSession) {
        return;
      }

      const sessionUpdate: Record<string, unknown> = {};
      if (payload.worktree_id) sessionUpdate.worktree_id = payload.worktree_id;
      if (payload.worktree_path) sessionUpdate.worktree_path = payload.worktree_path;
      if (payload.worktree_branch) sessionUpdate.worktree_branch = payload.worktree_branch;

      if (Object.keys(sessionUpdate).length > 0) {
        store.getState().setTaskSession({
          ...existingSession,
          ...sessionUpdate,
        });
      }

      if (payload.worktree_id) {
        store.getState().setWorktree({
          id: payload.worktree_id,
          sessionId: payload.session_id,
          repositoryId: existingSession.repository_id ?? undefined,
          path: payload.worktree_path ?? existingSession.worktree_path ?? undefined,
          branch: payload.worktree_branch ?? existingSession.worktree_branch ?? undefined,
        });
        const existing = store.getState().sessionWorktreesBySessionId.itemsBySessionId[payload.session_id] ?? [];
        if (!existing.includes(payload.worktree_id)) {
          store.getState().setSessionWorktrees(payload.session_id, [...existing, payload.worktree_id]);
        }
      }
    },
    'session.agentctl_error': (message) => {
      const payload = message.payload;
      if (!payload?.session_id) {
        return;
      }
      store.getState().setSessionAgentctlStatus(payload.session_id, {
        status: 'error',
        agentExecutionId: payload.agent_execution_id,
        errorMessage: payload.error_message,
        updatedAt: message.timestamp,
      });
    },
  };
}
