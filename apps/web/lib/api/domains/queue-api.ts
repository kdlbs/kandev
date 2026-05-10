import type { QueueStatus, QueuedMessage } from "@/lib/state/slices/session/types";
import { getWebSocketClient } from "@/lib/ws/connection";

const WS_CLIENT_UNAVAILABLE = "WebSocket client not available";

/** Error thrown when the queue would exceed its per-session cap. */
export class QueueFullError extends Error {
  readonly code = "queue_full";
  readonly queueSize: number;
  readonly max: number;

  constructor(queueSize: number, max: number) {
    super(`Queue is full (${queueSize}/${max} pending). Wait for the next turn to drain.`);
    this.name = "QueueFullError";
    this.queueSize = queueSize;
    this.max = max;
  }
}

/** Error thrown when the targeted entry was already drained or does not exist. */
export class QueueEntryNotFoundError extends Error {
  readonly code = "entry_not_found";
  constructor() {
    super("Queue entry was already drained or no longer exists.");
    this.name = "QueueEntryNotFoundError";
  }
}

type WSError = {
  code?: string;
  message?: string;
  details?: { queue_size?: number; max?: number; [k: string]: unknown };
};

function asWSError(err: unknown): WSError | undefined {
  if (typeof err === "object" && err !== null && "code" in err) {
    return err as WSError;
  }
  return undefined;
}

export function rethrowQueueError(err: unknown): never {
  const wsErr = asWSError(err);
  if (wsErr?.code === "queue_full") {
    const size = typeof wsErr.details?.queue_size === "number" ? wsErr.details.queue_size : 0;
    const max = typeof wsErr.details?.max === "number" ? wsErr.details.max : 0;
    throw new QueueFullError(size, max);
  }
  if (wsErr?.code === "entry_not_found") {
    throw new QueueEntryNotFoundError();
  }
  if (wsErr?.message) {
    throw new Error(wsErr.message);
  }
  throw err instanceof Error ? err : new Error(String(err));
}

export type QueueMessageParams = {
  session_id: string;
  task_id: string;
  content: string;
  model?: string;
  plan_mode?: boolean;
  attachments?: Array<{ type: string; data: string; mime_type: string }>;
  user_id?: string;
};

/** Append a new entry to the session's FIFO queue. Throws QueueFullError on overflow. */
export async function queueMessage(params: QueueMessageParams): Promise<QueuedMessage> {
  const client = getWebSocketClient();
  if (!client) {
    throw new Error(WS_CLIENT_UNAVAILABLE);
  }
  try {
    return await client.request<QueuedMessage>("message.queue.add", params);
  } catch (err) {
    rethrowQueueError(err);
  }
}

/** Clear every pending entry for the session. */
export async function clearQueue(sessionId: string): Promise<{ removed: number }> {
  const client = getWebSocketClient();
  if (!client) {
    throw new Error(WS_CLIENT_UNAVAILABLE);
  }
  return client.request<{ removed: number }>("message.queue.cancel", { session_id: sessionId });
}

/** Fetch the full queue snapshot (entries + capacity). */
export async function getQueueStatus(sessionId: string): Promise<QueueStatus> {
  const client = getWebSocketClient();
  if (!client) {
    throw new Error(WS_CLIENT_UNAVAILABLE);
  }
  return client.request<QueueStatus>("message.queue.get", { session_id: sessionId });
}

/** Append content onto the tail entry when the same caller authored it; otherwise insert a new entry. */
export async function appendToQueue(params: {
  session_id: string;
  task_id: string;
  content: string;
  model?: string;
  plan_mode?: boolean;
  user_id?: string;
}): Promise<{ entry_id: string; was_append: boolean }> {
  const client = getWebSocketClient();
  if (!client) {
    throw new Error(WS_CLIENT_UNAVAILABLE);
  }
  try {
    return await client.request<{ entry_id: string; was_append: boolean }>(
      "message.queue.append",
      params,
    );
  } catch (err) {
    rethrowQueueError(err);
  }
}

/** Replace the content/attachments of a queued entry. Throws QueueEntryNotFoundError if drained. */
export async function updateQueuedMessage(params: {
  session_id?: string;
  entry_id: string;
  content: string;
  attachments?: Array<{ type: string; data: string; mime_type: string }>;
  user_id?: string;
}): Promise<{ entry_id: string }> {
  const client = getWebSocketClient();
  if (!client) {
    throw new Error(WS_CLIENT_UNAVAILABLE);
  }
  try {
    return await client.request<{ entry_id: string }>("message.queue.update", params);
  } catch (err) {
    rethrowQueueError(err);
  }
}

/** Remove a single queued entry by id. Throws QueueEntryNotFoundError if drained. */
export async function removeQueuedEntry(params: {
  session_id?: string;
  entry_id: string;
}): Promise<{ entry_id: string }> {
  const client = getWebSocketClient();
  if (!client) {
    throw new Error(WS_CLIENT_UNAVAILABLE);
  }
  try {
    return await client.request<{ entry_id: string }>("message.queue.remove", params);
  } catch (err) {
    rethrowQueueError(err);
  }
}
