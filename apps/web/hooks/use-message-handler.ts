import { useCallback } from 'react';
import { getWebSocketClient } from '@/lib/ws/connection';
import type { MessageAttachment } from '@/components/task/chat/chat-input-container';

export function useMessageHandler(
  resolvedSessionId: string | null,
  taskId: string | null,
  sessionModel: string | null,
  activeModel: string | null,
  planMode: boolean = false
) {
  const handleSendMessage = useCallback(
    async (message: string, attachments?: MessageAttachment[]) => {
      if (!taskId || !resolvedSessionId) {
        console.error('No active task session. Start an agent before sending a message.');
        return;
      }
      const client = getWebSocketClient();
      if (!client) return;

      // Include active model in the request if it differs from the session model
      const modelToSend = activeModel && activeModel !== sessionModel ? activeModel : undefined;

      // Use longer timeout when sending attachments (images can be large)
      const hasAttachments = attachments && attachments.length > 0;
      const timeoutMs = hasAttachments ? 30000 : 10000;

      await client.request(
        'message.add',
        {
          task_id: taskId,
          session_id: resolvedSessionId,
          content: message,
          ...(modelToSend && { model: modelToSend }),
          ...(planMode && { plan_mode: true }),
          ...(hasAttachments && { attachments }),
        },
        timeoutMs
      );
    },
    [resolvedSessionId, taskId, activeModel, sessionModel, planMode]
  );

  return { handleSendMessage };
}
