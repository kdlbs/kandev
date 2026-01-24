import { useCallback } from 'react';
import { getWebSocketClient } from '@/lib/ws/connection';

export function useMessageHandler(
  resolvedSessionId: string | null,
  taskId: string | null,
  sessionModel: string | null,
  activeModel: string | null,
  planMode: boolean = false
) {
  const handleSendMessage = useCallback(
    async (message: string) => {
      if (!taskId || !resolvedSessionId) {
        console.error('No active task session. Start an agent before sending a message.');
        return;
      }
      const client = getWebSocketClient();
      if (!client) return;

      // Include active model in the request if it differs from the session model
      const modelToSend = activeModel && activeModel !== sessionModel ? activeModel : undefined;

      await client.request(
        'message.add',
        {
          task_id: taskId,
          session_id: resolvedSessionId,
          content: message,
          ...(modelToSend && { model: modelToSend }),
          ...(planMode && { plan_mode: true }),
        },
        10000
      );
    },
    [resolvedSessionId, taskId, activeModel, sessionModel, planMode]
  );

  return { handleSendMessage };
}
