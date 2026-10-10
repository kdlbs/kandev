import { type QueuedMessage, type QueueStatus } from "@/lib/state/slices/session/types";
import { type Task, type TaskSession } from "@/lib/types/http";
import { generateUUID } from "@/lib/uuid";
import { appendMessage, enqueueReply } from "./agent-run-runtime";
import {
  acceptedQueueMessages,
  findTask,
  notify,
  persist,
  queuedMessages,
  respond,
  runTimers,
  state,
} from "./runtime-state";
import { makeMessage } from "./scenario";

// i18n-exempt: browser demo transport diagnostics use literal fixture text
export function handleFollowUp(
  socketId: string,
  id: string,
  action: string,
  payload: Record<string, unknown>,
) {
  const task = findTask(String(payload.task_id));
  const session = state.sessions.find((candidate) => candidate.id === payload.session_id);
  const content = String(payload.content ?? "");
  if (!task || !session || session.task_id !== task.id || !content.trim()) {
    respond(socketId, id, { message: "A matching task, session, and message are required" }, true);
    return;
  }
  if (action === "message.queue.add") {
    if (payload.session_incarnation_id !== session.queue_incarnation_id) {
      respond(
        socketId,
        id,
        { message: "Queue session is no longer available", code: "queue_session_unavailable" },
        true,
      );
      return;
    }
    queueFollowUp(socketId, id, task, session, payload);
    return;
  }
  const messageId = String(payload.client_message_id || generateUUID());
  const existing = state.messagesBySession[session.id]?.find((message) => message.id === messageId);
  if (existing) {
    respond(
      socketId,
      id,
      existing,
      existing.author_type !== "user" || existing.content !== content,
    );
    return;
  }
  const user = makeMessage(messageId, session.id, task.id, "user", content);
  appendMessage(user);
  respond(socketId, id, user);
  enqueueReply(task, session, user);
  persist();
}

export function queueFollowUp(
  socketId: string,
  id: string,
  task: Task,
  session: TaskSession,
  payload: Record<string, unknown>,
) {
  const content = String(payload.content ?? "");
  const queueId = String(payload.client_queue_id || generateUUID());
  const accepted = state.messagesBySession[session.id]?.find(
    (message) => message.metadata?.client_queue_id === queueId,
  );
  const existing =
    acceptedQueueMessages.get(queueId) ??
    (accepted
      ? {
          id: queueId,
          task_id: task.id,
          session_id: session.id,
          content: accepted.content,
          plan_mode: payload.plan_mode === true,
          queued_at: accepted.created_at,
        }
      : undefined);
  if (existing) {
    respond(
      socketId,
      id,
      existing,
      existing.session_id !== session.id || existing.content !== content,
    );
    return;
  }
  const entry: QueuedMessage = {
    id: queueId,
    task_id: task.id,
    session_id: session.id,
    content,
    plan_mode: payload.plan_mode === true,
    queued_at: new Date().toISOString(),
  };
  acceptedQueueMessages.set(queueId, entry);
  queuedMessages.set(queueId, entry);
  respond(socketId, id, entry);
  notify("message.queue.status_changed", demoQueueStatus(session));
  runTimers.schedule(
    session.id,
    () => {
      if (!state.tasks.includes(task) || !state.sessions.includes(session)) return;
      queuedMessages.delete(queueId);
      const user = makeMessage(`${queueId}-user`, session.id, task.id, "user", content, {
        metadata: { client_queue_id: queueId },
      });
      appendMessage(user);
      notify("message.queue.status_changed", demoQueueStatus(session));
      enqueueReply(task, session, user);
      persist();
    },
    100,
  );
}

export function demoQueueStatus(session: TaskSession): QueueStatus {
  const entries = [...queuedMessages.values()].filter((entry) => entry.session_id === session.id);
  return {
    task_id: session.task_id,
    session_id: session.id,
    session_incarnation_id: session.queue_incarnation_id,
    entries,
    count: entries.length,
    max: 10,
    merge_enabled: false,
    auto_run: true,
  };
}
