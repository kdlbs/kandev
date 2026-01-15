import type { StoreApi } from 'zustand';
import type { AppState } from '@/lib/state/store';
import type { WsHandlers } from '@/lib/ws/handlers/types';

export function registerMessagesHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    'message.added': (message) => {
      const payload = message.payload;
      console.log('[WS] message.added payload:', JSON.stringify(payload, null, 2));
      const state = store.getState();
      console.log('[WS] store state:', {
        hasAddMessage: typeof state.addMessage,
        messagesSessionId: state.messages?.sessionId,
      });
      state.addMessage({
        id: payload.message_id,
        agent_session_id: payload.agent_session_id,
        task_id: payload.task_id,
        author_type: payload.author_type,
        author_id: payload.author_id,
        content: payload.content,
        type: (payload.type as 'message' | 'content' | 'tool_call' | 'progress' | 'error' | 'status') || 'message',
        metadata: payload.metadata,
        requests_input: payload.requests_input,
        created_at: payload.created_at,
      });
      console.log('[WS] addMessage called');
    },
    'message.updated': (message) => {
      const payload = message.payload;
      console.log('[WS] message.updated payload:', JSON.stringify(payload, null, 2));
      const state = store.getState();
      state.updateMessage({
        id: payload.message_id,
        agent_session_id: payload.agent_session_id,
        task_id: payload.task_id,
        author_type: payload.author_type,
        author_id: payload.author_id,
        content: payload.content,
        type: (payload.type as 'message' | 'content' | 'tool_call' | 'progress' | 'error' | 'status') || 'message',
        metadata: payload.metadata,
        requests_input: payload.requests_input,
        created_at: payload.created_at,
      });
      console.log('[WS] updateMessage called');
    },
  };
}
