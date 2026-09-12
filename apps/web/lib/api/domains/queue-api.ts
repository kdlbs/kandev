import type { QueueStatus, QueuedMessage } from "@/lib/state/slices/session/types";
import type { EntityReference } from "@/lib/types/entity-reference";
import type { Message, TaskPlanCommentRef } from "@/lib/types/http";
import { getWebSocketClient } from "@/lib/ws/connection";
// i18n-exempt: precondition diagnostic for a programmer error; callers branch
// on the error type, never render this message.
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

/** Error thrown when a queued edit lost its lease or expected revision. */
export class QueueEditConflictError extends Error {
  readonly code = "edit_conflict" as const;

  constructor() {
    super("The queued entry changed before the edit could be saved.");
    this.name = "QueueEditConflictError";
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
// i18n-exempt: transport/API diagnostic. Callers branch on the error code and
// render translated copy; this text only ever appears in a console or as an
// interpolated English diagnostic (see docs/i18n.md on interpolated values).
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
// i18n-exempt: transport/API diagnostic. Callers branch on the error code and
// render translated copy; this text only ever appears in a console or as an
// interpolated English diagnostic (see docs/i18n.md on interpolated values).
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
    case "edit_conflict":
    case "queue_conflict":
      return new QueueEditConflictError();
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

export type QueueSessionIdentity = {
  task_id: string;
  session_id: string;
  session_incarnation_id: string;
};

export type QueueMessageParams = {
  session_id: string;
  session_incarnation_id: string;
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
  client_queue_id?: string;
  plan_comment_refs?: TaskPlanCommentRef[];
  require_primary_session?: boolean;
};

function isUncertainQueueTransportError(error: unknown): boolean {
  if (!(error instanceof Error)) return false;
  const message = error.message.toLowerCase();
  return (
    message.includes("websocket request timed out") || message === "websocket connection closed"
  );
}

function hasQueueAdmissionMetadata(metadata: Message["metadata"], clientQueueId: string): boolean {
  if (metadata?.client_queue_id === clientQueueId) return true;
  const sources = metadata?.send_now_sources;
  if (!Array.isArray(sources)) return false;
  return sources.some((source) => {
    if (!source || typeof source !== "object") return false;
    const sourceMetadata = (source as { metadata?: unknown }).metadata;
    return (
      !!sourceMetadata &&
      typeof sourceMetadata === "object" &&
      (sourceMetadata as { client_queue_id?: unknown }).client_queue_id === clientQueueId
    );
  });
}

async function findAcceptedQueueAdmission(
  client: NonNullable<ReturnType<typeof getWebSocketClient>>,
  params: QueueMessageParams & { client_queue_id: string },
): Promise<QueuedMessage | undefined> {
  try {
    const status = await client.request<QueueStatus>("message.queue.get", {
      task_id: params.task_id,
      session_id: params.session_id,
      session_incarnation_id: params.session_incarnation_id,
    });
    const queued = status.entries?.find((entry) => entry.id === params.client_queue_id);
    if (queued) return queued;
  } catch {
    // Continue to transcript reconciliation in case the queue already drained.
  }
  try {
    let before: string | undefined;
    const seenCursors = new Set<string>();
    do {
      const response = await client.request<{
        messages?: Message[];
        has_more?: boolean;
        cursor?: string;
      }>(
        "message.list",
        {
          session_id: params.session_id,
          limit: 100,
          sort: "desc",
          ...(before ? { before } : {}),
        },
        5000,
      );
      const recorded = response.messages?.find((message) =>
        hasQueueAdmissionMetadata(message.metadata, params.client_queue_id),
      );
      if (recorded) {
        return {
          id: params.client_queue_id,
          session_id: params.session_id,
          task_id: params.task_id,
          content: recorded.content,
          model: params.model,
          plan_mode: params.plan_mode ?? false,
          attachments: params.attachments,
          metadata: recorded.metadata,
          queued_at: recorded.created_at,
        };
      }
      const cursor = response.cursor;
      if (!response.has_more || !cursor || seenCursors.has(cursor)) return undefined;
      seenCursors.add(cursor);
      before = cursor;
    } while (before);
    return undefined;
  } catch {
    return undefined;
  }
}

async function waitForQueueConnection(client: NonNullable<ReturnType<typeof getWebSocketClient>>) {
  const getStatus = client.getStatus?.bind(client);
  if (!getStatus || getStatus() === "connected") return true;
  const deadline = Date.now() + 3000;
  while (Date.now() < deadline) {
    await new Promise((resolve) => setTimeout(resolve, 100));
    if (getStatus() === "connected") return true;
  }
  return false;
}

async function reconcileUncertainQueueAdmission(
  client: NonNullable<ReturnType<typeof getWebSocketClient>>,
  params: QueueMessageParams & { client_queue_id: string },
  originalError: unknown,
) {
  const accepted = await findAcceptedQueueAdmission(client, params);
  if (accepted) return accepted;
  if (!(await waitForQueueConnection(client))) throw originalError;
  try {
    return await client.request<QueuedMessage>("message.queue.add", params);
  } catch (retryError) {
    const acceptedAfterRetry = await findAcceptedQueueAdmission(client, params);
    if (acceptedAfterRetry) return acceptedAfterRetry;
    throw retryError;
  }
}

/** Append a new entry to the session's FIFO queue. Throws QueueFullError on overflow. */
export async function queueMessage(params: QueueMessageParams): Promise<QueuedMessage> {
  const client = getWebSocketClient();
  if (!client) {
    throw new Error(WS_CLIENT_UNAVAILABLE);
  }
  try {
    return await client.request<QueuedMessage>("message.queue.add", params);
  } catch (err) {
    if (params.client_queue_id && isUncertainQueueTransportError(err)) {
      try {
        return await reconcileUncertainQueueAdmission(
          client,
          params as QueueMessageParams & { client_queue_id: string },
          err,
        );
      } catch (reconcileError) {
        rethrowQueueError(reconcileError);
      }
    }
    rethrowQueueError(err);
  }
}

/** Clear every pending entry for the session. */
export async function clearQueue(identity: QueueSessionIdentity): Promise<{ removed: number }> {
  const client = getWebSocketClient();
  if (!client) {
    throw new Error(WS_CLIENT_UNAVAILABLE);
  }
  return client.request<{ removed: number }>("message.queue.cancel", identity);
}

/** Dispatch one queued entry now when the session is ready for input. */
export async function drainQueuedMessage(
  identity: QueueSessionIdentity,
): Promise<{ drained: boolean }> {
  const client = getWebSocketClient();
  if (!client) {
    throw new Error(WS_CLIENT_UNAVAILABLE);
  }
  return client.request<{ drained: boolean }>("message.queue.drain", identity);
}

export type SendQueuedNowParams = QueueSessionIdentity & {
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

/** Persist the per-session queue Auto-run policy and start the queue when enabling it. */
export async function setQueueAutoRun(
  identity: QueueSessionIdentity,
  enabled: boolean,
): Promise<{ session_id: string; auto_run: boolean; dispatched: boolean }> {
  const client = getWebSocketClient();
  if (!client) {
    throw new Error(WS_CLIENT_UNAVAILABLE);
  }
  return client.request<{ session_id: string; auto_run: boolean; dispatched: boolean }>(
    "message.queue.auto_run.set",
    { ...identity, enabled },
  );
}
export type QueueAutoMergePolicyResponse = QueueSessionIdentity & {
  auto_merge_enabled: boolean;
  auto_merge_source: "global" | "session";
  auto_merge_revision: number;
};

/** Persist an explicit automatic-merge policy for one immutable session. */
export async function setQueueAutoMerge(
  identity: QueueSessionIdentity,
  enabled: boolean,
): Promise<QueueAutoMergePolicyResponse> {
  const client = getWebSocketClient();
  if (!client) {
    throw new Error(WS_CLIENT_UNAVAILABLE);
  }
  return client.request<QueueAutoMergePolicyResponse>("message.queue.auto_merge.set", {
    ...identity,
    enabled,
  });
}

/** Fetch the full queue snapshot for one immutable session identity. */
export async function getQueueStatus(identity: QueueSessionIdentity): Promise<QueueStatus> {
  const client = getWebSocketClient();
  if (!client) {
    throw new Error(WS_CLIENT_UNAVAILABLE);
  }
  return client.request<QueueStatus>("message.queue.get", identity);
}

/** Append content onto the tail entry when the same caller authored it; otherwise insert a new entry. */
export async function appendToQueue(
  params: QueueSessionIdentity & {
    content: string;
    model?: string;
    plan_mode?: boolean;
    user_id?: string;
  },
): Promise<{ entry_id: string; was_append: boolean }> {
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
export type QueueEditLease = {
  session_id: string;
  entry_id: string;
  lease_id: string;
  target_revision: number;
  lease_generation?: number;
  expires_at?: string;
};

export async function beginQueuedMessageEdit(
  sessionId: string,
  entryId: string,
): Promise<QueueEditLease> {
  const client = getWebSocketClient();
  if (!client) throw new Error(WS_CLIENT_UNAVAILABLE);
  try {
    return await client.request<QueueEditLease>("message.queue.edit.begin", {
      session_id: sessionId,
      entry_id: entryId,
    });
  } catch (err) {
    rethrowQueueError(err);
  }
}

export async function renewQueuedMessageEdit(
  lease: Pick<QueueEditLease, "session_id" | "entry_id" | "lease_id">,
): Promise<QueueEditLease> {
  const client = getWebSocketClient();
  if (!client) throw new Error(WS_CLIENT_UNAVAILABLE);
  try {
    return await client.request<QueueEditLease>("message.queue.edit.renew", lease);
  } catch (err) {
    rethrowQueueError(err);
  }
}

export async function endQueuedMessageEdit(
  lease: Pick<QueueEditLease, "session_id" | "entry_id" | "lease_id">,
  dispatchIfAutoRun = false,
): Promise<void> {
  const client = getWebSocketClient();
  if (!client) throw new Error(WS_CLIENT_UNAVAILABLE);
  try {
    await client.request("message.queue.edit.end", {
      ...lease,
      ...(dispatchIfAutoRun ? { dispatch_if_auto_run: true } : {}),
    });
  } catch (err) {
    rethrowQueueError(err);
  }
}

/** Replace a queued entry through its live target-bound edit lease. */
export async function updateQueuedMessage(
  params: QueueSessionIdentity & {
    entry_id: string;
    lease_id?: string;
    operation_id?: string;
    expected_target_revision?: number;
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
    entity_references?: EntityReference[];
    user_id?: string;
  },
): Promise<{ entry_id: string; operation_id?: string; target_revision?: number }> {
  const client = getWebSocketClient();
  if (!client) throw new Error(WS_CLIENT_UNAVAILABLE);
  try {
    return await client.request("message.queue.update", {
      ...params,
      entity_references: params.entity_references ?? [],
    });
  } catch (err) {
    rethrowQueueError(err);
  }
}

/** Remove a single queued entry by id. Throws QueueEntryNotFoundError if drained. */
export async function removeQueuedEntry(
  params: QueueSessionIdentity & { entry_id: string },
): Promise<{ entry_id: string }> {
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
export async function mergeQueuedEntry(
  params: QueueSessionIdentity & { entry_id: string; user_id?: string },
): Promise<{ entry_id: string }> {
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
export async function reorderQueuedEntries(
  params: QueueSessionIdentity & { ordered_ids: string[] },
): Promise<{ session_id: string; reordered: number }> {
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
