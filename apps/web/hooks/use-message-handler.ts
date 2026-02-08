import { useCallback } from 'react';
import { getWebSocketClient } from '@/lib/ws/connection';
import { useQueue } from './domains/session/use-queue';
import type { MessageAttachment } from '@/components/task/chat/chat-input-container';
import type { ActiveDocument, DocumentComment } from '@/lib/state/slices/ui/types';

function buildDocumentContext(activeDocument: ActiveDocument | null, comments?: DocumentComment[]): string {
  if (!activeDocument) return '';

  if (activeDocument.type === 'plan') {
    let context = `\n\n<kandev-system>\nACTIVE DOCUMENT: The user is editing the task plan side-by-side with this chat.\nRead the current plan using the plan_get MCP tool to understand the context before responding.\nAny plan modifications should use the plan_update MCP tool.`;

    if (comments && comments.length > 0) {
      context += `\n\nUser comments on the plan:`;
      comments.forEach((c, i) => {
        context += `\nComment ${i + 1}:\n- Selected text: "${c.selectedText}"\n- Comment: "${c.comment}"`;
      });
    }

    context += `\n</kandev-system>`;
    return context;
  }

  return `\n\n<kandev-system>\nACTIVE DOCUMENT: The user is editing "${activeDocument.name}" (${activeDocument.path}) side-by-side with this chat.\nRead this file to understand the context before responding.\n</kandev-system>`;
}

export function useMessageHandler(
  resolvedSessionId: string | null,
  taskId: string | null,
  sessionModel: string | null,
  activeModel: string | null,
  planMode: boolean = false,
  isAgentBusy: boolean = false,
  activeDocument: ActiveDocument | null = null,
  documentComments: DocumentComment[] = []
) {
  const { queue } = useQueue(resolvedSessionId);

  const handleSendMessage = useCallback(
    async (message: string, attachments?: MessageAttachment[], hasReviewComments?: boolean) => {
      if (!taskId || !resolvedSessionId) {
        console.error('No active task session. Start an agent before sending a message.');
        return;
      }
      const client = getWebSocketClient();
      if (!client) return;

      const trimmedMessage = message.trim();

      // Append document context if active
      const documentContext = buildDocumentContext(activeDocument, documentComments);
      const finalMessage = documentContext ? trimmedMessage + documentContext : trimmedMessage;

      // Include active model in the request if it differs from the session model
      const modelToSend = activeModel && activeModel !== sessionModel ? activeModel : undefined;

      // Use longer timeout when sending attachments (images can be large)
      const hasAttachments = attachments && attachments.length > 0;
      const timeoutMs = hasAttachments ? 30000 : 10000;

      // If agent is busy, queue the message instead of sending immediately
      if (isAgentBusy) {
        // Convert MessageAttachment[] to queue API format
        const queueAttachments = attachments?.map(att => ({
          type: att.type,
          data: att.data,
          mime_type: att.mime_type,
        }));

        await queue(taskId, finalMessage, modelToSend, planMode, queueAttachments);
        return;
      }

      // Agent not busy - send message normally
      await client.request(
        'message.add',
        {
          task_id: taskId,
          session_id: resolvedSessionId,
          content: finalMessage,
          ...(modelToSend && { model: modelToSend }),
          ...(planMode && { plan_mode: true }),
          ...(hasReviewComments && { has_review_comments: true }),
          ...(hasAttachments && { attachments }),
        },
        timeoutMs
      );
    },
    [resolvedSessionId, taskId, activeModel, sessionModel, planMode, isAgentBusy, queue, activeDocument, documentComments]
  );

  return { handleSendMessage };
}
