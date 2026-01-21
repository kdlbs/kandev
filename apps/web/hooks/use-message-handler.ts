import { useCallback } from 'react';
import { getWebSocketClient } from '@/lib/ws/connection';

export function useMessageHandler(
  resolvedSessionId: string | null,
  taskId: string | null,
  sessionModel: string | null,
  pendingModel: string | null,
  setActiveModel: (sessionId: string, modelId: string) => void,
  clearPendingModel: (sessionId: string) => void
) {
  const handleSendMessage = useCallback(
    async (message: string) => {
      if (!taskId || !resolvedSessionId) {
        console.error('No active task session. Start an agent before sending a message.');
        return;
      }
      const client = getWebSocketClient();
      if (!client) return;

      // Include pending model in the request if one is selected
      const modelToSend = pendingModel && pendingModel !== sessionModel ? pendingModel : undefined;

      await client.request(
        'message.add',
        {
          task_id: taskId,
          session_id: resolvedSessionId,
          content: message,
          ...(modelToSend && { model: modelToSend }),
        },
        10000
      );

      // Move pending model to active model after sending (it's now the model in use)
      if (modelToSend && resolvedSessionId) {
        setActiveModel(resolvedSessionId, modelToSend);
        clearPendingModel(resolvedSessionId);
      }
    },
    [resolvedSessionId, taskId, pendingModel, sessionModel, setActiveModel, clearPendingModel]
  );

  return { handleSendMessage };
}
