import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { MessageAddedPayload } from "@/lib/types/backend";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import { sessionId, taskId, type Message, type MessageType } from "@/lib/types/http";

type MessagePayload = MessageAddedPayload;
type MessageUpdate = Omit<Message, "session_id" | "task_id"> & {
  session_id: ReturnType<typeof sessionId>;
  task_id: ReturnType<typeof taskId>;
};

export type MessageFrameSchedulerOptions = {
  schedule?: (callback: () => void) => number;
  cancel?: (handle: number) => void;
};

export type MessageUpdateScheduler = {
  enqueue: (payload: MessagePayload) => void;
  flush: () => void;
  dispose: () => void;
};

export type MessageHandlerRegistration = {
  handlers: WsHandlers;
  scheduler: MessageUpdateScheduler;
  dispose: () => void;
};

function toMessage(payload: MessagePayload): MessageUpdate {
  return {
    id: payload.message_id,
    session_id: sessionId(payload.session_id),
    task_id: taskId(payload.task_id),
    turn_id: payload.turn_id,
    author_type: payload.author_type,
    author_id: payload.author_id,
    content: payload.content,
    raw_content: payload.raw_content,
    type: (payload.type as MessageType) || "message",
    metadata: payload.metadata,
    requests_input: payload.requests_input,
    created_at: payload.created_at,
    updated_at: payload.updated_at,
  };
}

function messageKey(payload: MessagePayload): string {
  return `${payload.session_id}:${payload.message_id}`;
}

function isOlderThanCurrentSnapshot(store: StoreApi<AppState>, payload: MessagePayload): boolean {
  const current = store
    .getState()
    .messages.bySession[
      sessionId(payload.session_id)
    ]?.find((message) => message.id === payload.message_id);
  if (!current?.updated_at) return false;
  return !payload.updated_at || current.updated_at > payload.updated_at;
}

function defaultSchedule(callback: () => void): number {
  if (typeof requestAnimationFrame === "function") {
    return requestAnimationFrame(() => callback());
  }
  return globalThis.setTimeout(callback, 16) as unknown as number;
}

function defaultCancel(handle: number): void {
  if (typeof cancelAnimationFrame === "function") {
    cancelAnimationFrame(handle);
    return;
  }
  globalThis.clearTimeout(handle);
}

export function createMessageUpdateScheduler(
  store: StoreApi<AppState>,
  options: MessageFrameSchedulerOptions = {},
): MessageUpdateScheduler {
  const schedule = options.schedule ?? defaultSchedule;
  const cancel = options.cancel ?? defaultCancel;
  const pending = new Map<string, MessagePayload>();
  let scheduledHandle: number | null = null;
  let disposed = false;

  const flush = () => {
    if (scheduledHandle !== null) {
      cancel(scheduledHandle);
      scheduledHandle = null;
    }
    if (disposed || pending.size === 0) {
      pending.clear();
      return;
    }
    const updates = [...pending.values()];
    pending.clear();
    for (const payload of updates) {
      if (!payload.session_id || !payload.message_id) continue;
      if (isOlderThanCurrentSnapshot(store, payload)) continue;
      store.getState().updateMessage(toMessage(payload));
    }
  };

  const enqueue = (payload: MessagePayload) => {
    if (disposed || !payload.session_id || !payload.message_id) return;
    pending.set(messageKey(payload), payload);
    if (scheduledHandle !== null) return;
    scheduledHandle = schedule(() => {
      scheduledHandle = null;
      flush();
    });
  };

  const dispose = () => {
    if (disposed) return;
    disposed = true;
    if (scheduledHandle !== null) {
      cancel(scheduledHandle);
      scheduledHandle = null;
    }
    pending.clear();
  };

  return { enqueue, flush, dispose };
}

export function createMessagesHandlerRegistration(
  store: StoreApi<AppState>,
  options?: MessageFrameSchedulerOptions,
): MessageHandlerRegistration {
  const scheduler = createMessageUpdateScheduler(store, options);
  const handlers: WsHandlers = {
    "session.message.added": (message) => {
      const payload = message.payload;
      if (!payload.session_id) return;
      scheduler.flush();
      store.getState().addMessage(toMessage(payload));
    },
    "session.message.updated": (message) => {
      scheduler.enqueue(message.payload);
    },
    "session.message.deleted": (message) => {
      const payload = message.payload;
      if (!payload.session_id) return;
      // A delete is a semantic barrier. Applying pending updates before the
      // removal prevents a stale animation-frame callback from resurrecting
      // the deleted row.
      scheduler.flush();
      store.getState().removeMessage(sessionId(payload.session_id), payload.message_id);
    },
  };
  return { handlers, scheduler, dispose: scheduler.dispose };
}
