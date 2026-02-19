import type { QueueStatus, QueuedMessage } from "@/lib/state/slices/session/types";
import { getWebSocketClient } from "@/lib/ws/connection";

const WS_CLIENT_UNAVAILABLE = "WebSocket client not available";

export type QueueMessageParams = {
  session_id: string;
  task_id: string;
  content: string;
  model?: string;
  plan_mode?: boolean;
  attachments?: Array<{ type: string; data: string; mime_type: string }>;
  user_id?: string;
};

// Queue a message for auto-execution
export async function queueMessage(params: QueueMessageParams): Promise<QueuedMessage> {
  const client = getWebSocketClient();
  if (!client) {
    throw new Error(WS_CLIENT_UNAVAILABLE);
  }

  return client.request<QueuedMessage>("message.queue.add", params);
}

// Cancel a queued message
export async function cancelQueuedMessage(sessionId: string): Promise<QueuedMessage> {
  const client = getWebSocketClient();
  if (!client) {
    throw new Error(WS_CLIENT_UNAVAILABLE);
  }

  return client.request<QueuedMessage>("message.queue.cancel", { session_id: sessionId });
}

// Get queue status for a session
export async function getQueueStatus(sessionId: string): Promise<QueueStatus> {
  const client = getWebSocketClient();
  if (!client) {
    throw new Error(WS_CLIENT_UNAVAILABLE);
  }

  return client.request<QueueStatus>("message.queue.get", { session_id: sessionId });
}

// Update queued message content (for arrow up editing)
export async function updateQueuedMessage(
  sessionId: string,
  content: string,
): Promise<QueueStatus> {
  const client = getWebSocketClient();
  if (!client) {
    throw new Error(WS_CLIENT_UNAVAILABLE);
  }

  return client.request<QueueStatus>("message.queue.update", { session_id: sessionId, content });
}
