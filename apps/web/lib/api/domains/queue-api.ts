import type { QueueStatus, QueuedMessage } from "@/lib/state/slices/session/types";
import type { EntityReference } from "@/lib/types/entity-reference";
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

/** Error thrown when a merge would push the combined entity references past
 * the per-message cap; the server rejects the merge atomically instead of
 * dropping references that were already persisted. */
export class MergeReferenceOverflowError extends Error {
  readonly code = "merge_reference_overflow";
  constructor() {
    super("Merging would exceed the per-message entity reference limit.");
    this.name = "MergeReferenceOverflowError";
  }
}

export type QueueSendNowErrorCode =
  | "queue_empty"
  | "queue_changed"
  | "send_now_conflict"
  | "turn_changed"
  | "send_now_attachment_overflow"
  | "send_now_reference_overflow";

const QUEUE_SEND_NOW_ERROR_CODES: ReadonlySet<QueueSendNowErrorCode> = new Set([
  "queue_empty",
  "queue_changed",
  "send_now_conflict",
  "turn_changed",
  "send_now_attachment_overflow",
  "send_now_reference_overflow",
]);

/** Error returned when Send Now cannot safely claim its click-time selection. */
export class QueueSendNowError extends Error {
  readonly code: QueueSendNowErrorCode;

  constructor(code: QueueSendNowErrorCode, message?: string) {
    super(message ?? "Queued messages could not be sent now.");
    this.name = "QueueSendNowError";
    this.code = code;
  }
}

/** Error thrown when a reorder's submitted id set no longer matches the
 * visible pending queue (an entry was drained, removed, merged, or newly
 * queued since the client's snapshot). The reorder was rejected atomically;
 * callers refetch the authoritative queue. */
export class QueueReorderError extends Error {
  readonly code = "queue_changed" as const;

  constructor(message?: string) {
    super(message ?? "The queue changed before the reorder could be applied.");
    this.name = "QueueReorderError";
  }
}

type WSError = {
  code?: string;
  message?: string;
  details?: { queue_size?: number; max?: number; [k: string]: unknown };
};

function asWSError(err: unknown): WSError | undefined {
  // Some WS error payloads omit `code` and only carry `message`/`details`;
  // narrowing on `code` alone would drop those and stringify the whole object
  // as the eventual Error message ("[object Object]"). Real Error instances
  // are skipped so they pass through unchanged in `rethrowQueueError`.
  if (typeof err !== "object" || err === null || err instanceof Error) {
    return undefined;
  }
  if ("code" in err || "message" in err || "details" in err) {
    return err as WSError;
  }
  return undefined;
}

function knownQueueError(wsErr: WSError): Error | undefined {
  switch (wsErr.code) {
    case "queue_full": {
      const size = typeof wsErr.details?.queue_size === "number" ? wsErr.details.queue_size : 0;
      const max = typeof wsErr.details?.max === "number" ? wsErr.details.max : 0;
      return new QueueFullError(size, max);
    }
    case "entry_not_found":
      return new QueueEntryNotFoundError();
    case "merge_reference_overflow":
      return new MergeReferenceOverflowError();
    default:
      if (wsErr.code && QUEUE_SEND_NOW_ERROR_CODES.has(wsErr.code as QueueSendNowErrorCode)) {
        return new QueueSendNowError(wsErr.code as QueueSendNowErrorCode, wsErr.message);
      }
      return undefined;
  }
}

export function rethrowQueueError(err: unknown): never {
  const wsErr = asWSError(err);
  if (wsErr) {
    const known = knownQueueError(wsErr);
    if (known) throw known;
    if (wsErr.message) throw new Error(wsErr.message);
  }
  throw err instanceof Error ? err : new Error(String(err));
}

export type QueueMessageParams = {
  session_id: string;
  task_id: string;
  content: string;
  model?: string;
  plan_mode?: boolean;
  attachments?: Array<{
    type: string;
    data?: string;
    attachment_id?: string;
    mime_type: string;
    name?: string;
    size_bytes?: number;
    delivery_mode?: "prompt" | "path";
  }>;
  context_files?: Array<{ path: string; name: string; is_directory?: boolean }>;
  entity_references?: EntityReference[];
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

/** Dispatch one queued entry now when the session is ready for input. */
export async function drainQueuedMessage(sessionId: string): Promise<{ drained: boolean }> {
  const client = getWebSocketClient();
  if (!client) {
    throw new Error(WS_CLIENT_UNAVAILABLE);
  }
  return client.request<{ drained: boolean }>("message.queue.drain", { session_id: sessionId });
}

export type SendQueuedNowParams = {
  session_id: string;
  scope: "entry" | "all";
  entry_id?: string;
};

/** Interrupt the active turn and replace it with an exact queue selection. */
export async function sendQueuedNow(
  params: SendQueuedNowParams,
): Promise<{ session_id: string; dispatched: boolean; sent_count: number }> {
  const client = getWebSocketClient();
  if (!client) {
    throw new Error(WS_CLIENT_UNAVAILABLE);
  }
  try {
    return await client.request<{
      session_id: string;
      dispatched: boolean;
      sent_count: number;
    }>("message.queue.send_now", params);
  } catch (err) {
    rethrowQueueError(err);
  }
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
  session_id: string;
  entry_id: string;
  content: string;
  attachments?: Array<{
    type: string;
    data?: string;
    attachment_id?: string;
    mime_type: string;
    name?: string;
    size_bytes?: number;
    delivery_mode?: "prompt" | "path";
  }>;
  entity_references: EntityReference[];
  user_id?: string;
}): Promise<{ entry_id: string }> {
  const client = getWebSocketClient();
  if (!client) {
    throw new Error(WS_CLIENT_UNAVAILABLE);
  }
  try {
    return await client.request<{ entry_id: string }>("message.queue.update", {
      ...params,
      entity_references: params.entity_references ?? [],
    });
  } catch (err) {
    rethrowQueueError(err);
  }
}

/** Remove a single queued entry by id. Throws QueueEntryNotFoundError if drained. */
export async function removeQueuedEntry(params: {
  session_id: string;
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

/** Fold a queued entry into the entry directly above it. Throws QueueEntryNotFoundError if drained. */
export async function mergeQueuedEntry(params: {
  session_id: string;
  entry_id: string;
  user_id?: string;
}): Promise<{ entry_id: string }> {
  const client = getWebSocketClient();
  if (!client) {
    throw new Error(WS_CLIENT_UNAVAILABLE);
  }
  try {
    return await client.request<{ entry_id: string }>("message.queue.merge", params);
  } catch (err) {
    rethrowQueueError(err);
  }
}

/** Rewrite the visible pending order of a session's queue. Throws
 * QueueReorderError when the queue changed since the client's snapshot; the
 * reorder was rejected atomically and the caller should refetch. */
export async function reorderQueuedEntries(params: {
  session_id: string;
  ordered_ids: string[];
}): Promise<{ session_id: string; reordered: number }> {
  const client = getWebSocketClient();
  if (!client) {
    throw new Error(WS_CLIENT_UNAVAILABLE);
  }
  try {
    return await client.request<{ session_id: string; reordered: number }>(
      "message.queue.reorder",
      params,
    );
  } catch (err) {
    // The shared queue_changed mapping targets Send Now; reorder drift needs
    // its own error class so callers reconcile silently instead of toasting.
    if (asWSError(err)?.code === "queue_changed") {
      throw new QueueReorderError();
    }
    rethrowQueueError(err);
  }
}
