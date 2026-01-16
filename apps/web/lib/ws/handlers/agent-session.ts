import type { StoreApi } from 'zustand';
import type { AppState } from '@/lib/state/store';
import type { WsHandlers } from '@/lib/ws/handlers/types';
import type { TaskSessionState } from '@/lib/types/http';

export function registerTaskSessionHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    'task_session.state_changed': (message) => {
      const payload = message.payload;
      if (!payload?.task_id || !payload?.new_state) {
        return;
      }
      const taskId = payload.task_id;
      const newState = payload.new_state as TaskSessionState;
      const sessionId = payload.task_session_id;

      // Update task session state by task ID
      store.getState().setTaskSessionState(taskId, newState);

      // Also update or create the session object if we have the session ID
      if (sessionId) {
        const existingSession = store.getState().taskSessions.items[sessionId];
        if (existingSession) {
          // Update existing session
          store.getState().setTaskSession({
            ...existingSession,
            state: newState,
          });
        } else {
          // Create partial session object
          store.getState().setTaskSession({
            id: sessionId,
            task_id: taskId,
            state: newState,
            progress: 0,
            started_at: '',
            updated_at: '',
          });
        }
      }
    },
  };
}
