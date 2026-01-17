import type { StoreApi } from 'zustand';
import type { AppState } from '@/lib/state/store';
import type { WsHandlers } from '@/lib/ws/handlers/types';

export function registerMessagesHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    'message.added': (message) => {
      const payload = message.payload;
      if (!payload.task_session_id) {
        return;
      }
      store.getState().addMessage({
        id: payload.message_id,
        task_session_id: payload.task_session_id,
        task_id: payload.task_id,
        author_type: payload.author_type,
        author_id: payload.author_id,
        content: payload.content,
        type: (payload.type as 'message' | 'content' | 'tool_call' | 'progress' | 'error' | 'status') || 'message',
        metadata: payload.metadata,
        requests_input: payload.requests_input,
        created_at: payload.created_at,
      });
    },
    'message.updated': (message) => {
      const payload = message.payload;
      if (!payload.task_session_id) {
        return;
      }
      store.getState().updateMessage({
        id: payload.message_id,
        task_session_id: payload.task_session_id,
        task_id: payload.task_id,
        author_type: payload.author_type,
        author_id: payload.author_id,
        content: payload.content,
        type: (payload.type as 'message' | 'content' | 'tool_call' | 'progress' | 'error' | 'status') || 'message',
        metadata: payload.metadata,
        requests_input: payload.requests_input,
        created_at: payload.created_at,
      });
    },
  };
}
