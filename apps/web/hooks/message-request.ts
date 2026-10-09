import type { MessageAttachment } from "@/components/task/chat/chat-input-container";
import { MessageSendError } from "@/lib/chat/message-send-error";
import { listTaskSessions } from "@/lib/api/domains/session-api";
import type { EntityReference } from "@/lib/types/entity-reference";
import type { Message, TaskPlanCommentRef, TaskPreviewFeedbackRef } from "@/lib/types/http";
import { generateUUID } from "@/lib/utils";
import { getWebSocketClient } from "@/lib/ws/connection";

type SendMessagePayload = {
  taskId: string;
  resolvedSessionId: string;
  clientMessageId?: string;
  finalMessage: string;
  modelToSend: string | undefined;
  planMode: boolean;
  hasReviewComments?: boolean;
  attachments?: MessageAttachment[];
  contextFilesMeta?: Array<{ path: string; name: string; is_directory?: boolean }>;
  entityReferences?: EntityReference[];
  planCommentRefs?: TaskPlanCommentRef[];
  previewFeedbackRefs?: TaskPreviewFeedbackRef[];
  requirePrimarySession?: boolean;
};

type MessageListResponse = { messages?: Message[] };

function isUncertainMessageTransportError(error: unknown): boolean {
  if (!(error instanceof Error)) return false;
  const message = error.message.toLowerCase();
  return (
    message.includes("websocket request timed out") || message === "websocket connection closed"
  );
}

export async function findMessageByID(
  client: ReturnType<typeof getWebSocketClient>,
  taskId: string,
  sessionId: string,
  messageId: string,
): Promise<Message | undefined> {
  if (!client) return undefined;
  const sessionIds = [sessionId];
  try {
    const response = await listTaskSessions(taskId);
    for (const session of response.sessions ?? []) {
      if (session.id && !sessionIds.includes(session.id)) sessionIds.push(session.id);
    }
  } catch {
    // The submitted session remains a useful reconciliation fallback.
  }
  try {
    for (const candidateSessionId of sessionIds) {
      const response = await client.request<MessageListResponse>(
        "message.list",
        { session_id: candidateSessionId, limit: 100, sort: "desc" },
        5000,
      );
      const found = response.messages?.find((message) => message.id === messageId);
      if (found) return found;
    }
    return undefined;
  } catch {
    return undefined;
  }
}

async function waitForConnected(client: NonNullable<ReturnType<typeof getWebSocketClient>>) {
  const getStatus = client.getStatus?.bind(client);
  if (!getStatus || getStatus() === "connected") return true;
  const deadline = Date.now() + 3000;
  while (Date.now() < deadline) {
    await new Promise((resolve) => setTimeout(resolve, 100));
    if (getStatus() === "connected") return true;
  }
  return false;
}

type MessageReconciliation = {
  client: NonNullable<ReturnType<typeof getWebSocketClient>>;
  taskId: string;
  sessionId: string;
  messageId: string;
  request: () => Promise<Message | undefined>;
  originalError: unknown;
};

async function reconcileUncertainMessage({
  client,
  taskId,
  sessionId,
  messageId,
  request,
  originalError,
}: MessageReconciliation) {
  const committed = await findMessageByID(client, taskId, sessionId, messageId);
  if (committed) return committed;
  if (!(await waitForConnected(client))) throw originalError;

  try {
    return await request();
  } catch (retryError) {
    const retriedMessage = await findMessageByID(client, taskId, sessionId, messageId);
    if (retriedMessage) return retriedMessage;
    throw retryError;
  }
}

function buildMessageRequestPayload(payload: SendMessagePayload, stableMessageId: string) {
  const {
    taskId,
    resolvedSessionId,
    finalMessage,
    modelToSend,
    planMode,
    hasReviewComments,
    attachments,
    contextFilesMeta,
    entityReferences,
    planCommentRefs,
    previewFeedbackRefs,
    requirePrimarySession,
  } = payload;
  return {
    task_id: taskId,
    session_id: resolvedSessionId,
    client_message_id: stableMessageId,
    content: finalMessage,
    ...(modelToSend && { model: modelToSend }),
    ...(planMode && { plan_mode: true }),
    ...(hasReviewComments && { has_review_comments: true }),
    ...(attachments?.length && { attachments }),
    ...(contextFilesMeta && { context_files: contextFilesMeta }),
    ...(entityReferences && { entity_references: entityReferences }),
    ...(planCommentRefs?.length && { plan_comment_refs: planCommentRefs }),
    ...(previewFeedbackRefs?.length && { preview_feedback_refs: previewFeedbackRefs }),
    ...(requirePrimarySession && { require_primary_session: true }),
  };
}

export async function sendMessageRequest(
  payload: SendMessagePayload,
): Promise<Message | undefined> {
  const client = getWebSocketClient();
  if (!client) {
    throw new MessageSendError(
      "connection-unavailable",
      "Connection unavailable. Reconnect and try again.",
    );
  }

  const stableMessageId = payload.clientMessageId ?? generateUUID();
  const requestPayload = buildMessageRequestPayload(payload, stableMessageId);
  const request = () =>
    client.request<Message | undefined>(
      "message.add",
      requestPayload,
      payload.attachments?.length ? 30000 : 10000,
    );

  try {
    return await request();
  } catch (error) {
    if (!isUncertainMessageTransportError(error)) throw error;
    return reconcileUncertainMessage({
      client,
      taskId: payload.taskId,
      sessionId: payload.resolvedSessionId,
      messageId: stableMessageId,
      request,
      originalError: error,
    });
  }
}
