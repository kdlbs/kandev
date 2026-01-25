import type { StoreApi } from 'zustand';
import type { AppState } from '@/lib/state/store';
import type { WsHandlers } from '@/lib/ws/handlers/types';
import type { TaskSessionState } from '@/lib/types/http';

export function registerTaskSessionHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    'session.state_changed': (message) => {
      const payload = message.payload;
      if (!payload?.task_id || !payload?.new_state) {
        return;
      }
      const taskId = payload.task_id;
      const newState = payload.new_state as TaskSessionState;
      const sessionId = payload.session_id;

      // Also update or create the session object if we have the session ID
      if (sessionId) {
        const existingSession = store.getState().taskSessions.items[sessionId];
        const agentProfileId = payload.agent_profile_id;
        const agentProfileSnapshot = payload.agent_profile_snapshot;

        if (existingSession) {
          // Update existing session - preserve all existing fields, but update snapshot if provided
          store.getState().setTaskSession({
            ...existingSession,
            state: newState,
            // Update snapshot if provided in the event
            ...(agentProfileSnapshot ? { agent_profile_snapshot: agentProfileSnapshot } : {}),
          });
        } else {
          // Create partial session object
          // Include agent_profile_id and agent_profile_snapshot from the payload if provided
          store.getState().setTaskSession({
            id: sessionId,
            task_id: taskId,
            state: newState,
            started_at: '',
            updated_at: '',
            ...(agentProfileId ? { agent_profile_id: agentProfileId } : {}),
            ...(agentProfileSnapshot ? { agent_profile_snapshot: agentProfileSnapshot } : {}),
          });
        }

        const sessionsByTask = store.getState().taskSessionsByTask.itemsByTaskId[taskId];
        if (sessionsByTask) {
          const hasSession = sessionsByTask.some((session) => session.id === sessionId);
          if (!hasSession) {
            store.getState().setTaskSessionsForTask(taskId, [...sessionsByTask, {
              id: sessionId,
              task_id: taskId,
              state: newState,
              started_at: '',
              updated_at: '',
              ...(agentProfileId ? { agent_profile_id: agentProfileId } : {}),
              ...(agentProfileSnapshot ? { agent_profile_snapshot: agentProfileSnapshot } : {}),
            }]);
          } else {
            const nextSessions = sessionsByTask.map((session) =>
              session.id === sessionId ? {
                ...session,
                state: newState,
                ...(agentProfileSnapshot ? { agent_profile_snapshot: agentProfileSnapshot } : {}),
              } : session
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
