import type { StoreApi } from 'zustand';
import type { AppState } from '@/lib/state/store';
import type { WsHandlers } from '@/lib/ws/handlers/types';

export function registerCommentsHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    'comment.added': (message) => {
      const payload = message.payload;
      console.log('[WS] comment.added payload:', JSON.stringify(payload, null, 2));
      const state = store.getState();
      console.log('[WS] store state:', {
        hasAddComment: typeof state.addComment,
        commentsTaskId: state.comments?.taskId,
      });
      state.addComment({
        id: payload.comment_id,
        task_id: payload.task_id,
        author_type: payload.author_type,
        author_id: payload.author_id,
        content: payload.content,
        type: (payload.type as 'message' | 'content' | 'tool_call' | 'progress' | 'error' | 'status') || 'message',
        metadata: payload.metadata,
        requests_input: payload.requests_input,
        created_at: payload.created_at,
      });
      console.log('[WS] addComment called');
    },
    'comment.updated': (message) => {
      const payload = message.payload;
      console.log('[WS] comment.updated payload:', JSON.stringify(payload, null, 2));
      const state = store.getState();
      state.updateComment({
        id: payload.comment_id,
        task_id: payload.task_id,
        author_type: payload.author_type,
        author_id: payload.author_id,
        content: payload.content,
        type: (payload.type as 'message' | 'content' | 'tool_call' | 'progress' | 'error' | 'status') || 'message',
        metadata: payload.metadata,
        requests_input: payload.requests_input,
        created_at: payload.created_at,
      });
      console.log('[WS] updateComment called');
    },
  };
}
