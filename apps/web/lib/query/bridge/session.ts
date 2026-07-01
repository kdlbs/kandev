import type { QueryClient } from "@tanstack/react-query";
import type { QueueStatus, QueuedMessage } from "@/lib/state/slices/session/types";
import type { BackendMessageMap } from "@/lib/types/backend";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type Message,
  type MessageType,
  type Task,
  type TaskPlan,
  type TaskPlanRevision,
  type Turn,
  type WorkflowSnapshot,
} from "@/lib/types/http";
import { qk } from "../keys";
import type { SessionMessagesLatestData } from "../query-options/session";
import { updateWorkflowSnapshotQueries } from "../workflow-snapshot-cache";
import { registerBridgeHandlers, type QueryBridgeRegistration } from "./registrar";

type MessageEvent =
  | BackendMessageMap["session.message.added"]
  | BackendMessageMap["session.message.updated"];
type MessageDeletedEvent = BackendMessageMap["session.message.deleted"];
type TurnEvent =
  | BackendMessageMap["session.turn.started"]
  | BackendMessageMap["session.turn.completed"];
type SessionStateEvent = BackendMessageMap["session.state_changed"];
type QueueEvent = BackendMessageMap["message.queue.status_changed"];
type PlanEvent = BackendMessageMap["task.plan.created"] | BackendMessageMap["task.plan.updated"];
type PlanRevisionEvent =
  | BackendMessageMap["task.plan.revision.created"]
  | BackendMessageMap["task.plan.reverted"];

export function registerSessionBridge(
  ws: Parameters<typeof registerBridgeHandlers>[0],
  queryClient: QueryClient,
): QueryBridgeRegistration {
  return registerBridgeHandlers(ws, queryClient, {
    "session.message.added": (message) => {
      const row = messageFromPayload(message.payload);
      if (row) upsertMessageCaches(queryClient, row);
    },
    "session.message.updated": (message) => {
      const row = messageFromPayload(message.payload);
      if (row) upsertMessageCaches(queryClient, row);
    },
    "session.message.deleted": (message) => {
      removeMessageCaches(queryClient, message);
    },
    "session.turn.started": (message) => {
      const row = turnFromPayload(message.payload);
      if (row) upsertTurn(queryClient, row, "started");
    },
    "session.turn.completed": (message) => {
      const row = turnFromPayload(message.payload);
      if (row) upsertTurn(queryClient, row, "completed");
      if (message.payload.session_id) {
        queryClient.invalidateQueries({
          exact: true,
          queryKey: qk.session.messages(message.payload.session_id),
        });
      }
    },
    "session.state_changed": (message) => {
      patchTaskSession(queryClient, message);
      patchPrimaryTaskSessionState(queryClient, message);
    },
    "message.queue.status_changed": (message) => {
      patchQueueStatus(queryClient, message);
    },
    "task.plan.created": (message) => {
      upsertTaskPlan(queryClient, message);
    },
    "task.plan.updated": (message) => {
      upsertTaskPlan(queryClient, message);
    },
    "task.plan.deleted": (message) => {
      deleteTaskPlan(queryClient, message);
    },
    "task.plan.revision.created": (message) => {
      upsertTaskPlanRevision(queryClient, message);
    },
    "task.plan.reverted": (message) => {
      upsertTaskPlanRevision(queryClient, message);
      queryClient.invalidateQueries({
        exact: true,
        queryKey: qk.taskPlan.detail(message.payload.task_id),
      });
    },
  });
}

function removeMessageCaches(queryClient: QueryClient, message: MessageDeletedEvent): void {
  const payload = message.payload;
  if (!payload.session_id || !payload.message_id) return;
  const sid = payload.session_id;
  const messageId = payload.message_id;
  queryClient.setQueryData(qk.session.messages(sid), (current: unknown) =>
    removeMessageFromLatest(current, messageId),
  );
  queryClient.setQueriesData({ queryKey: ["session", sid, "messagesPage"] }, (current: unknown) =>
    removeMessageFromPage(current, messageId),
  );
  queryClient.setQueriesData(
    { queryKey: ["session", sid, "messagesInfinite"] },
    (current: unknown) => removeMessageFromPages(current, messageId),
  );
}

function removeMessageFromLatest(current: unknown, messageId: string): unknown {
  if (!isRecord(current) || !Array.isArray(current.messages)) return current;
  return { ...current, messages: removeMessage(current.messages, messageId) };
}

function removeMessageFromPage(current: unknown, messageId: string): unknown {
  if (!isRecord(current) || !Array.isArray(current.messages)) return current;
  return { ...current, messages: removeMessage(current.messages, messageId) };
}

function removeMessageFromPages(current: unknown, messageId: string): unknown {
  if (!isRecord(current) || !Array.isArray(current.pages)) return current;
  return {
    ...current,
    pages: current.pages.map((page) =>
      isRecord(page) && Array.isArray(page.messages)
        ? { ...page, messages: removeMessage(page.messages, messageId) }
        : page,
    ),
  };
}

function removeMessage(messages: unknown[], messageId: string): Message[] {
  return messages.filter(
    (message) => !(isRecord(message) && message.id === messageId),
  ) as Message[];
}

function messageFromPayload(payload: MessageEvent["payload"]): Message | null {
  if (!payload.session_id || !payload.message_id) return null;
  return {
    id: payload.message_id,
    session_id: toSessionId(payload.session_id),
    task_id: toTaskId(payload.task_id),
    turn_id: payload.turn_id,
    author_type: payload.author_type,
    author_id: payload.author_id,
    content: payload.content,
    raw_content: payload.raw_content,
    type: (payload.type as MessageType | undefined) ?? "message",
    metadata: payload.metadata,
    requests_input: payload.requests_input,
    created_at: payload.created_at,
    updated_at: payload.updated_at,
  };
}

function turnFromPayload(payload: TurnEvent["payload"]): Turn | null {
  if (!payload.session_id || !payload.id) return null;
  return {
    id: payload.id,
    session_id: toSessionId(payload.session_id),
    task_id: toTaskId(payload.task_id),
    started_at: payload.started_at,
    completed_at: payload.completed_at,
    metadata: payload.metadata,
    created_at: payload.created_at,
    updated_at: payload.updated_at,
  };
}

function upsertMessageCaches(queryClient: QueryClient, row: Message): void {
  const sid = row.session_id;
  const latestMessagesKey = qk.session.messages(sid);
  if (queryClient.getQueryData(latestMessagesKey) === undefined) {
    queryClient.invalidateQueries({ exact: true, queryKey: latestMessagesKey });
  } else {
    queryClient.setQueryData(latestMessagesKey, (current: unknown) =>
      patchLatestMessages(current, row),
    );
  }
  queryClient.setQueriesData({ queryKey: ["session", sid, "messagesPage"] }, (current: unknown) =>
    patchMessagePage(current, row),
  );
  queryClient.setQueriesData(
    { queryKey: ["session", sid, "messagesInfinite"] },
    (current: unknown) => patchMessagePages(current, row),
  );
}

function patchLatestMessages(current: unknown, row: Message): SessionMessagesLatestData {
  const currentRecord = isRecord(current) ? current : {};
  const messages = Array.isArray(currentRecord.messages) ? currentRecord.messages : [];
  const next = upsertMessage(messages, row);
  return {
    messages: next,
    hasMore: Boolean(currentRecord.hasMore),
    oldestCursor:
      typeof currentRecord.oldestCursor === "string"
        ? currentRecord.oldestCursor
        : (next[0]?.id ?? null),
  };
}

function patchMessagePage(current: unknown, row: Message): unknown {
  if (!isRecord(current) || !Array.isArray(current.messages)) return current;
  return { ...current, messages: upsertMessage(current.messages, row) };
}

function patchMessagePages(current: unknown, row: Message): unknown {
  if (!isRecord(current) || !Array.isArray(current.pages)) return current;
  return {
    ...current,
    pages: current.pages.map((page) =>
      isRecord(page) && Array.isArray(page.messages)
        ? { ...page, messages: upsertMessage(page.messages, row) }
        : page,
    ),
  };
}

function upsertMessage(messages: unknown[], row: Message): Message[] {
  const next = messages.map((message) =>
    isRecord(message) && message.id === row.id
      ? ({ ...message, ...definedFields(row) } as Message)
      : (message as Message),
  );
  if (!next.some((message) => message.id === row.id)) next.push(row);
  return next.sort((a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime());
}

function upsertTurn(queryClient: QueryClient, row: Turn, phase: "started" | "completed"): void {
  const queryKey = qk.session.turns(row.session_id);
  let patchedExistingList = false;
  queryClient.setQueryData(queryKey, (current: unknown) => {
    if (!isRecord(current) || !Array.isArray(current.turns)) return current;
    patchedExistingList = true;
    const currentRecord = current;
    const turns = currentRecord.turns as unknown[];
    const next = turns.map((turn) =>
      isRecord(turn) && turn.id === row.id ? { ...turn, ...definedFields(row) } : turn,
    );
    if (!next.some((turn) => isRecord(turn) && turn.id === row.id)) next.push(row);
    const currentActiveTurnId =
      typeof currentRecord.activeTurnId === "string" ? currentRecord.activeTurnId : null;
    return {
      ...currentRecord,
      turns: next.sort(
        (a, b) =>
          new Date(String((a as Turn).started_at)).getTime() -
          new Date(String((b as Turn).started_at)).getTime(),
      ),
      total: typeof currentRecord.total === "number" ? currentRecord.total : next.length,
      activeTurnId: nextActiveTurnId(phase, row.id, currentActiveTurnId),
    };
  });
  if (!patchedExistingList) {
    queryClient.invalidateQueries({ exact: true, queryKey });
  }
}

function nextActiveTurnId(
  phase: string | undefined,
  rowId: string,
  currentActiveTurnId: string | null,
): string | null {
  if (phase === "started") return rowId;
  if (currentActiveTurnId === rowId) return null;
  return currentActiveTurnId;
}

function patchTaskSession(queryClient: QueryClient, message: SessionStateEvent): void {
  const payload = message.payload;
  if (!payload.session_id) return;
  const byIdKey = qk.taskSession.byId(payload.session_id);
  const existing = queryClient.getQueryData(byIdKey);
  if (isStaleSessionStateEvent(existing, payload.updated_at)) return;
  queryClient.setQueryData(byIdKey, (current: unknown) => {
    const existing = isRecord(current) ? current : {};
    return applySessionStatePayload(
      {
        ...existing,
        id: payload.session_id,
        task_id: payload.task_id ?? existing.task_id,
      },
      payload,
    );
  });
  if (!hasFullSessionDetail(existing)) {
    queryClient.invalidateQueries({ exact: true, queryKey: byIdKey });
  }
  if (payload.task_id) {
    patchTaskSessionList(queryClient, payload.task_id, payload.session_id, payload);
    queryClient.invalidateQueries({
      exact: true,
      queryKey: qk.taskSession.byTask(payload.task_id),
    });
  }
}

function hasFullSessionDetail(session: unknown): boolean {
  return isRecord(session) && typeof session.started_at === "string";
}

function patchPrimaryTaskSessionState(queryClient: QueryClient, message: SessionStateEvent): void {
  const payload = message.payload;
  if (!payload.task_id || !payload.session_id || !payload.new_state) return;
  const existing = queryClient.getQueryData(qk.taskSession.byId(payload.session_id));
  if (isStaleSessionStateEvent(existing, payload.updated_at)) return;
  queryClient.setQueryData(qk.tasks.detail(payload.task_id), (current: unknown) =>
    patchPrimaryTask(current, payload),
  );
  updateWorkflowSnapshotQueries(queryClient, (snapshot) =>
    patchSnapshotPrimaryTask(snapshot, payload),
  );
}

function patchSnapshotPrimaryTask(
  snapshot: WorkflowSnapshot,
  payload: SessionStateEvent["payload"],
): WorkflowSnapshot {
  let changed = false;
  const tasks = snapshot.tasks.map((task) => {
    const nextTask = patchPrimaryTask(task, payload) as Task;
    if (nextTask !== task) changed = true;
    return nextTask;
  });
  return changed ? { ...snapshot, tasks } : snapshot;
}

function patchPrimaryTask(current: unknown, payload: SessionStateEvent["payload"]): unknown {
  if (!isRecord(current) || current.id !== payload.task_id) return current;
  if (current.primary_session_id !== payload.session_id) return current;
  if (current.primary_session_state === payload.new_state) return current;
  return {
    ...current,
    primary_session_state: payload.new_state as Task["primary_session_state"],
    updated_at: payload.updated_at ?? current.updated_at,
  };
}

function isStaleSessionStateEvent(
  existing: unknown,
  payloadUpdatedAt: string | undefined,
): boolean {
  if (!payloadUpdatedAt || !isRecord(existing) || typeof existing.updated_at !== "string") {
    return false;
  }
  const payloadTime = Date.parse(payloadUpdatedAt);
  const existingTime = Date.parse(existing.updated_at);
  if (Number.isNaN(payloadTime) || Number.isNaN(existingTime)) return false;
  return payloadTime < existingTime;
}

function patchTaskSessionList(
  queryClient: QueryClient,
  taskId: string,
  sessionId: string,
  payload: SessionStateEvent["payload"],
): void {
  queryClient.setQueryData(qk.taskSession.byTask(taskId), (current: unknown) => {
    if (!isRecord(current) || !Array.isArray(current.sessions)) return current;
    let found = false;
    const sessions = current.sessions.map((session) => {
      if (isRecord(session) && session.id === sessionId) {
        found = true;
        return applySessionStatePayload(session, payload);
      }
      return session;
    });
    if (!found) sessions.push(sessionRowFromStatePayload(queryClient, taskId, sessionId, payload));
    return { ...current, sessions };
  });
}

function sessionRowFromStatePayload(
  queryClient: QueryClient,
  taskId: string,
  sessionId: string,
  payload: SessionStateEvent["payload"],
): Record<string, unknown> {
  const byId = queryClient.getQueryData(qk.taskSession.byId(sessionId));
  const existing = isRecord(byId) ? byId : {};
  const timestamp =
    payload.updated_at ??
    (typeof existing.updated_at === "string" ? existing.updated_at : new Date(0).toISOString());
  return applySessionStatePayload(
    {
      ...existing,
      id: sessionId,
      task_id: taskId,
      state: payload.new_state ?? existing.state ?? "STARTING",
      started_at: typeof existing.started_at === "string" ? existing.started_at : timestamp,
      updated_at: typeof existing.updated_at === "string" ? existing.updated_at : timestamp,
    },
    payload,
  );
}

function applySessionStatePayload(
  existing: Record<string, unknown>,
  payload: SessionStateEvent["payload"],
): Record<string, unknown> {
  return {
    ...existing,
    state: payload.new_state ?? existing.state,
    updated_at: payload.updated_at ?? existing.updated_at,
    error_message: payload.error_message ?? existing.error_message,
    metadata: mergeSessionMetadata(existing.metadata, payload),
    agent_profile_id: payload.agent_profile_id ?? existing.agent_profile_id,
    agent_profile_snapshot: payload.agent_profile_snapshot ?? existing.agent_profile_snapshot,
    is_passthrough: payload.is_passthrough ?? existing.is_passthrough,
    review_status: payload.review_status ?? existing.review_status,
    task_environment_id: payload.task_environment_id ?? existing.task_environment_id,
  };
}

function mergeSessionMetadata(
  existingMetadata: unknown,
  payload: SessionStateEvent["payload"],
): unknown {
  const incoming = payload.session_metadata ?? payload.metadata;
  if (incoming === undefined) return existingMetadata;
  if (!isRecord(incoming)) return incoming;
  const existing = isRecord(existingMetadata) ? existingMetadata : {};
  return { ...existing, ...incoming };
}

function patchQueueStatus(queryClient: QueryClient, message: QueueEvent): void {
  const payload = message.payload;
  if (!payload.session_id) return;
  const entries = Array.isArray(payload.entries) ? (payload.entries as QueuedMessage[]) : [];
  const status: QueueStatus = {
    entries,
    count: typeof payload.count === "number" ? payload.count : entries.length,
    max: typeof payload.max === "number" ? payload.max : 0,
  };
  queryClient.setQueryData(qk.session.queue(payload.session_id), status);
}

function upsertTaskPlan(queryClient: QueryClient, message: PlanEvent): void {
  const payload = message.payload;
  const plan: TaskPlan = {
    id: payload.id,
    task_id: payload.task_id,
    title: payload.title,
    content: payload.content,
    created_by: payload.created_by,
    created_at: payload.created_at,
    updated_at: payload.updated_at,
  };
  queryClient.setQueryData(qk.taskPlan.detail(payload.task_id), plan);
}

function deleteTaskPlan(
  queryClient: QueryClient,
  message: BackendMessageMap["task.plan.deleted"],
): void {
  queryClient.setQueryData(qk.taskPlan.detail(message.payload.task_id), null);
  queryClient.invalidateQueries({
    exact: true,
    queryKey: qk.taskPlan.revisions(message.payload.task_id),
  });
}

function upsertTaskPlanRevision(queryClient: QueryClient, message: PlanRevisionEvent): void {
  const payload = message.payload;
  const revisionsKey = qk.taskPlan.revisions(payload.task_id);
  const revision: TaskPlanRevision = {
    id: payload.id,
    task_id: payload.task_id,
    revision_number: payload.revision_number,
    title: payload.title,
    author_kind: payload.author_kind,
    author_name: payload.author_name,
    revert_of_revision_id: payload.revert_of_revision_id ?? null,
    created_at: payload.created_at,
    updated_at: payload.updated_at,
  };
  queryClient.setQueryData(revisionsKey, (current: unknown) => {
    if (!Array.isArray(current)) return current;
    const next = current.map((item) =>
      isRecord(item) && item.id === revision.id ? { ...item, ...revision } : item,
    );
    if (!next.some((item) => isRecord(item) && item.id === revision.id)) next.unshift(revision);
    return next.sort(
      (a, b) =>
        Number((b as TaskPlanRevision).revision_number) -
        Number((a as TaskPlanRevision).revision_number),
    );
  });
  queryClient.invalidateQueries({
    exact: true,
    queryKey: revisionsKey,
  });
  queryClient.invalidateQueries({
    exact: true,
    queryKey: qk.taskPlan.revision(payload.task_id, payload.id),
  });
}

function definedFields<T extends Record<string, unknown>>(value: T): Partial<T> {
  return Object.fromEntries(
    Object.entries(value).filter(([, field]) => field !== undefined),
  ) as Partial<T>;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
