import type { Page } from "@playwright/test";

type DroppedMessage = {
  action: string;
  content: string;
};

type PromptDropController = {
  dropPrompt: (prompt: string) => void;
  droppedCount: () => number;
  recoveryResponseCount: () => number;
};

type MessageAddResponseDropController = {
  dropNextMessageAddResponse: () => void;
  droppedCount: () => number;
};

type ConversationChangeDropController = {
  dropChange: (content: string) => void;
  droppedCount: () => number;
  pluginSubscribeCount: () => number;
};

export type QueueAdmissionDropController = {
  dropNextQueueAddRequest: () => void;
  dropNextQueueAddResponse: (count?: number) => void;
  dropQueueAdmissionReconciliation: () => void;
  queueAddRequestCount: () => number;
  droppedRequestCount: () => number;
  droppedResponseCount: () => number;
};

function asRecord(value: unknown): Record<string, unknown> | null {
  return typeof value === "object" && value !== null ? (value as Record<string, unknown>) : null;
}

function parseJSONFrames(message: string | Buffer): Array<Record<string, unknown>> {
  const text = typeof message === "string" ? message : message.toString("utf8");
  const frames: Array<Record<string, unknown>> = [];
  for (const part of text.split("\n")) {
    if (!part.trim()) continue;
    try {
      const parsed = asRecord(JSON.parse(part));
      if (parsed) frames.push(parsed);
    } catch {
      // Preserve non-JSON frames; the app's WS client handles parse failures too.
    }
  }
  return frames;
}

function targetMessagePayload(message: unknown, prompt: string): Record<string, unknown> | null {
  const envelope = asRecord(message);
  const payload = asRecord(envelope?.payload);
  const isLegacy = envelope?.action === "session.message.added" && payload?.author_type === "user";
  const isOrdered =
    envelope?.type === "session.event" &&
    envelope?.event_type === "message.added" &&
    payload?.author_type === "user";
  if (
    (!isLegacy && !isOrdered) ||
    typeof payload?.content !== "string" ||
    !payload.content.includes(prompt)
  ) {
    return null;
  }
  return payload;
}

function isTargetUserMessageAdded(
  message: unknown,
  prompt: string,
): message is { payload: { content: string } } {
  return targetMessagePayload(message, prompt) !== null;
}

function targetAction(message: unknown): string {
  const envelope = asRecord(message);
  return envelope?.type === "session.event" ? "session.event" : "session.message.added";
}

function filterServerFrame(
  message: string | Buffer,
  prompt: string | null,
  dropped: DroppedMessage[],
): string | Buffer | null {
  if (!prompt || typeof message !== "string") return message;

  const kept: string[] = [];
  let didDrop = false;
  for (const part of message.split("\n")) {
    const trimmed = part.trim();
    if (!trimmed) {
      kept.push(part);
      continue;
    }
    try {
      const parsed = JSON.parse(trimmed) as unknown;
      if (isTargetUserMessageAdded(parsed, prompt)) {
        didDrop = true;
        dropped.push({ action: targetAction(parsed), content: parsed.payload.content });
        continue;
      }
      if (hasConversationChangeContent(parsed, prompt)) {
        didDrop = true;
        dropped.push({ action: "session.conversation.changed", content: prompt });
        continue;
      }
    } catch {
      // Preserve non-JSON frames; the app's WS client handles parse failures too.
    }
    kept.push(part);
  }

  if (!didDrop) return message;
  const filtered = kept.join("\n");
  return filtered.trim() ? filtered : null;
}

export async function routeMainWebSocketWithPromptDrop(page: Page): Promise<PromptDropController> {
  let promptToDrop: string | null = null;
  const dropped: DroppedMessage[] = [];
  const recoveryRequestIDs = new Set<string>();
  const orderedRecoveryRequestIDs = new Set<string>();
  let recoveryResponses = 0;

  await page.routeWebSocket(/\/ws$/, (ws) => {
    const server = ws.connectToServer();
    ws.onMessage((message) => {
      for (const frame of parseJSONFrames(message)) {
        const payload = asRecord(frame.payload);
        if (
          promptToDrop !== null &&
          frame.type === "request" &&
          frame.action === "message.list" &&
          typeof frame.id === "string"
        ) {
          recoveryRequestIDs.add(frame.id);
        }
        if (
          promptToDrop !== null &&
          frame.type === "request" &&
          frame.action === "session.subscribe" &&
          payload?.consumer_kind === "core" &&
          payload.replace_cursor === true &&
          typeof frame.id === "string"
        ) {
          orderedRecoveryRequestIDs.add(frame.id);
        }
      }
      server.send(message);
    });
    server.onMessage((message) => {
      for (const frame of parseJSONFrames(message)) {
        if (
          frame.type === "response" &&
          frame.action === "message.list" &&
          typeof frame.id === "string" &&
          recoveryRequestIDs.delete(frame.id)
        ) {
          if (promptToDrop !== null && JSON.stringify(frame).includes(promptToDrop)) {
            recoveryResponses += 1;
          }
        }
        if (
          frame.type === "response" &&
          frame.action === "session.subscribe" &&
          typeof frame.id === "string" &&
          orderedRecoveryRequestIDs.delete(frame.id)
        ) {
          recoveryResponses += 1;
        }
      }
      const filtered = filterServerFrame(message, promptToDrop, dropped);
      if (filtered !== null) ws.send(filtered);
    });
  });

  return {
    dropPrompt: (prompt: string) => {
      promptToDrop = prompt;
      dropped.length = 0;
      recoveryRequestIDs.clear();
      orderedRecoveryRequestIDs.clear();
      recoveryResponses = 0;
    },
    droppedCount: () => dropped.length,
    recoveryResponseCount: () => recoveryResponses,
  };
}

function filterMessageAddResponses(
  message: string | Buffer,
  requestIDs: Set<string>,
  armed: { value: boolean },
  dropped: { value: number },
): string | Buffer | null {
  if (typeof message !== "string" || !armed.value) return message;

  const kept: string[] = [];
  let didDrop = false;
  for (const part of message.split("\n")) {
    const trimmed = part.trim();
    if (!trimmed) {
      kept.push(part);
      continue;
    }

    let responseID: string | undefined;
    try {
      const frame = asRecord(JSON.parse(trimmed));
      if (
        frame?.type === "response" &&
        frame.action === "message.add" &&
        typeof frame.id === "string" &&
        requestIDs.has(frame.id)
      ) {
        responseID = frame.id;
      }
    } catch {
      // Preserve non-JSON frames.
    }

    if (responseID) {
      requestIDs.delete(responseID);
      armed.value = false;
      dropped.value += 1;
      didDrop = true;
      continue;
    }
    kept.push(part);
  }

  if (!didDrop) return message;
  const filtered = kept.join("\n");
  return filtered.trim() ? filtered : null;
}

/**
 * Drops one correlated `message.add` response while preserving the request,
 * notification, and every unrelated frame. This exercises the UI's
 * stable-ID reconciliation path without disconnecting the whole socket.
 */
export async function routeMainWebSocketWithMessageAddResponseDrop(
  page: Page,
): Promise<MessageAddResponseDropController> {
  const requestIDs = new Set<string>();
  const armed = { value: false };
  const dropped = { value: 0 };

  await page.routeWebSocket(/\/ws$/, (ws) => {
    const server = ws.connectToServer();
    ws.onMessage((message) => {
      for (const frame of parseJSONFrames(message)) {
        if (
          armed.value &&
          frame.type === "request" &&
          frame.action === "message.add" &&
          typeof frame.id === "string"
        ) {
          requestIDs.add(frame.id);
        }
      }
      server.send(message);
    });
    server.onMessage((message) => {
      const filtered = filterMessageAddResponses(message, requestIDs, armed, dropped);
      if (filtered !== null) ws.send(filtered);
    });
  });

  return {
    dropNextMessageAddResponse: () => {
      requestIDs.clear();
      armed.value = true;
      dropped.value = 0;
    },
    droppedCount: () => dropped.value,
  };
}

/**
 * Injects one pre-server request loss or one post-admission response loss for
 * `message.queue.add`. Every other gateway frame continues through the proxy.
 */
type QueueAdmissionProxyState = {
  responseRequestIDs: Set<string>;
  dropRequest: { value: boolean };
  dropResponseCount: { value: number };
  dropReconciliation: { value: boolean };
  queueAddRequests: { value: number };
  droppedRequests: { value: number };
  droppedResponses: { value: number };
};

function parseQueueAdmissionFrame(part: string): Record<string, unknown> | undefined {
  if (!part.trim()) return undefined;
  try {
    return asRecord(JSON.parse(part.trim())) ?? undefined;
  } catch {
    return undefined;
  }
}

function filterQueueAdmissionFrames(
  message: string,
  shouldDrop: (frame: Record<string, unknown>) => boolean,
): string {
  return message
    .split("\n")
    .filter((part) => {
      const frame = parseQueueAdmissionFrame(part);
      return frame === undefined || !shouldDrop(frame);
    })
    .join("\n");
}

function inspectQueueAdmissionRequest(
  frame: Record<string, unknown>,
  state: QueueAdmissionProxyState,
): boolean {
  if (frame.type !== "request" || frame.action !== "message.queue.add") return false;
  state.queueAddRequests.value += 1;
  if (state.dropRequest.value) {
    state.dropRequest.value = false;
    state.droppedRequests.value += 1;
    return true;
  }
  if (state.dropResponseCount.value > 0 && typeof frame.id === "string") {
    state.responseRequestIDs.add(frame.id);
  }
  return false;
}

function inspectQueueAdmissionResponse(
  frame: Record<string, unknown>,
  state: QueueAdmissionProxyState,
): boolean {
  const isReconciliationResponse =
    state.dropReconciliation.value &&
    frame.type === "response" &&
    (frame.action === "message.queue.get" || frame.action === "message.list");
  if (isReconciliationResponse) return true;

  const isAdmissionResponse =
    frame.type === "response" &&
    frame.action === "message.queue.add" &&
    typeof frame.id === "string" &&
    state.responseRequestIDs.has(frame.id) &&
    state.dropResponseCount.value > 0;
  if (!isAdmissionResponse) return false;

  state.responseRequestIDs.delete(frame.id as string);
  state.dropResponseCount.value -= 1;
  state.droppedResponses.value += 1;
  return true;
}

export async function routeMainWebSocketWithQueueAdmissionDrops(
  page: Page,
): Promise<QueueAdmissionDropController> {
  const state: QueueAdmissionProxyState = {
    responseRequestIDs: new Set<string>(),
    dropRequest: { value: false },
    dropResponseCount: { value: 0 },
    dropReconciliation: { value: false },
    queueAddRequests: { value: 0 },
    droppedRequests: { value: 0 },
    droppedResponses: { value: 0 },
  };

  await page.routeWebSocket(/\/ws$/, (ws) => {
    const server = ws.connectToServer();
    ws.onMessage((message) => {
      if (typeof message !== "string") {
        server.send(message);
        return;
      }
      const filtered = filterQueueAdmissionFrames(message, (frame) =>
        inspectQueueAdmissionRequest(frame, state),
      );
      if (filtered.trim()) server.send(filtered);
    });
    server.onMessage((message) => {
      if (typeof message !== "string") {
        ws.send(message);
        return;
      }
      const filtered = filterQueueAdmissionFrames(message, (frame) =>
        inspectQueueAdmissionResponse(frame, state),
      );
      if (filtered.trim()) ws.send(filtered);
    });
  });

  return {
    dropNextQueueAddRequest: () => {
      state.dropRequest.value = true;
    },
    dropNextQueueAddResponse: (count = 1) => {
      state.responseRequestIDs.clear();
      state.dropResponseCount.value = Math.max(1, count);
      state.dropReconciliation.value = false;
      state.droppedResponses.value = 0;
    },
    dropQueueAdmissionReconciliation: () => {
      state.dropReconciliation.value = true;
    },
    queueAddRequestCount: () => state.queueAddRequests.value,
    droppedRequestCount: () => state.droppedRequests.value,
    droppedResponseCount: () => state.droppedResponses.value,
  };
}

function hasConversationChangeContent(message: unknown, content: string): boolean {
  const envelope = asRecord(message);
  if (envelope?.action !== "session.conversation.changed") {
    return false;
  }
  return JSON.stringify(envelope).includes(content);
}

function filterConversationChange(
  message: string | Buffer,
  content: string | null,
  state: { value: number },
): string | Buffer {
  if (content === null) return message;
  const text = typeof message === "string" ? message : message.toString("utf8");
  const kept: string[] = [];
  let didDrop = false;
  for (const part of text.split("\n")) {
    const trimmed = part.trim();
    if (!trimmed) {
      kept.push(part);
      continue;
    }
    let frame: unknown;
    try {
      frame = JSON.parse(trimmed);
    } catch {
      kept.push(part);
      continue;
    }
    if (hasConversationChangeContent(frame, content)) {
      state.value += 1;
      didDrop = true;
      continue;
    }
    kept.push(part);
  }
  if (!didDrop) return message;
  const filtered = kept.join("\n");
  return typeof message === "string" ? filtered : Buffer.from(filtered, "utf8");
}

/**
 * Drops one durable plugin conversation change while preserving the socket.
 * The following change creates a revision gap and exercises source recovery.
 */
export async function routeMainWebSocketWithConversationChangeDrop(
  page: Page,
): Promise<ConversationChangeDropController> {
  let contentToDrop: string | null = null;
  const dropped = { value: 0 };
  let pluginSubscribeRequests = 0;

  await page.routeWebSocket(/\/ws$/, (ws) => {
    const server = ws.connectToServer();
    ws.onMessage((message) => {
      for (const frame of parseJSONFrames(message)) {
        const payload = asRecord(frame.payload);
        if (
          frame.type === "request" &&
          frame.action === "session.conversation.subscribe" &&
          payload?.consumer_kind === "plugin"
        ) {
          pluginSubscribeRequests += 1;
        }
      }
      server.send(message);
    });
    server.onMessage((message) => {
      const filtered = filterConversationChange(message, contentToDrop, dropped);
      if (dropped.value > 0 && filtered !== message) contentToDrop = null;
      ws.send(filtered);
    });
  });

  return {
    dropChange: (content: string) => {
      contentToDrop = content;
      dropped.value = 0;
    },
    droppedCount: () => dropped.value,
    pluginSubscribeCount: () => pluginSubscribeRequests,
  };
}
